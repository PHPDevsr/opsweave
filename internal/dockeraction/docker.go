// Package dockeraction exposes explicitly allowed container operations over local IPC.
package dockeraction

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/registry"
)

const bodyLimit = 1 << 20

// Config defaults to disabled and never discovers ambient Docker configuration.
type Config struct {
	Enabled           bool
	Endpoint          string
	AllowedContainers []string
	AllowedActions    []string
}

var errConfig = errors.New("invalid Docker action configuration")
var errAction = errors.New("Docker action failed")

func hexID(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func validAction(s string) bool {
	switch s {
	case "docker.start", "docker.stop", "docker.restart", "docker.inspect":
		return true
	}
	return false
}

func validEndpoint(s string) bool {
	if runtime.GOOS == "windows" {
		const prefix = "npipe:////./pipe/"
		if !strings.HasPrefix(s, prefix) {
			return false
		}
		name := strings.TrimPrefix(s, prefix)
		if len(name) == 0 || len(name) > 128 || name == "." || name == ".." {
			return false
		}
		for _, c := range name {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
				return false
			}
		}
		return true
	}
	if !strings.HasPrefix(s, "unix:///") {
		return false
	}
	p := strings.TrimPrefix(s, "unix://")
	if len(p) > 4096 || p == "/" || path.Clean(p) != p || !utf8.ValidString(p) || strings.ContainsAny(p, "%?#\\") {
		return false
	}
	for _, c := range p {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}

// Validate checks all supplied policy without contacting a daemon, even if disabled.
func (c Config) Validate() error {
	if c.Endpoint != "" && !validEndpoint(c.Endpoint) || c.Enabled && c.Endpoint == "" || len(c.AllowedContainers) > 128 || len(c.AllowedActions) > 4 || c.Enabled && (len(c.AllowedContainers) == 0 || len(c.AllowedActions) == 0) {
		return errConfig
	}
	containers := map[string]bool{}
	for _, id := range c.AllowedContainers {
		if !hexID(id) || containers[id] {
			return errConfig
		}
		containers[id] = true
	}
	actions := map[string]bool{}
	for _, id := range c.AllowedActions {
		if !validAction(id) || actions[id] {
			return errConfig
		}
		actions[id] = true
	}
	return nil
}

// Registrations snapshots the administrator's exact container and action grants.
func Registrations(c Config) ([]action.Registration, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !c.Enabled {
		return nil, nil
	}
	allowed := map[string]bool{}
	for _, id := range c.AllowedContainers {
		allowed[id] = true
	}
	var entries []action.Registration
	for _, id := range c.AllowedActions {
		outputs := map[string]registry.Field{"container_id": {Type: "string", Required: true}, "changed": {Type: "boolean", Required: true}}
		if id == "docker.inspect" {
			delete(outputs, "changed")
			for _, key := range []string{"name", "image_id", "status"} {
				outputs[key] = registry.Field{Type: "string", Required: true}
			}
			outputs["running"] = registry.Field{Type: "boolean", Required: true}
			outputs["exit_code"] = registry.Field{Type: "integer", Required: true}
		}
		entries = append(entries, action.Registration{Descriptor: registry.Descriptor{ID: id, Version: "1", APIVersion: "opsweave.io/v1", Permissions: []string{id}, Inputs: registry.Field{Type: "object", Properties: map[string]registry.Field{"container": {Type: "string", Required: true}}}, Outputs: outputs}, Run: func(ctx context.Context, in map[string]any) (map[string]any, error) {
			container, ok := in["container"].(string)
			if !ok || len(in) != 1 || !hexID(container) || !allowed[container] {
				return nil, errAction
			}
			return invoke(ctx, c.Endpoint, id, container)
		}})
	}
	return entries, nil
}

// invoke owns its one connection and cancellation callback. HTTP has no pool,
// redirects, proxy discovery, automatic decompression or background transport.
func invoke(parent context.Context, endpoint, id, container string) (output map[string]any, resultErr error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	defer func() {
		// An IPC deadline can wake before Go's context timer is scheduled.
		// Join that signal so the runtime observes the parent's deadline rather
		// than misclassifying its timeout as an ordinary action failure.
		if !time.Now().Before(deadline) {
			<-ctx.Done()
		}
		if ctx.Err() != nil {
			output = nil
			resultErr = errAction
		}
	}()
	dialCtx, stopDial := context.WithTimeout(ctx, 5*time.Second)
	conn, err := dialLocal(dialCtx, endpoint)
	stopDial()
	if err != nil {
		return nil, errAction
	}
	var once sync.Once
	var closeErr error
	closeConn := func() { once.Do(func() { closeErr = conn.Close() }) }
	closed := make(chan struct{})
	stopClose := context.AfterFunc(ctx, func() { defer close(closed); closeConn() })
	defer func() {
		if !stopClose() {
			<-closed
		}
		closeConn()
		if closeErr != nil || ctx.Err() != nil {
			output = nil
			resultErr = errAction
		}
	}()
	if conn.SetDeadline(deadline) != nil {
		return nil, errAction
	}
	operation := strings.TrimPrefix(id, "docker.")
	method := http.MethodPost
	if operation == "inspect" {
		method = http.MethodGet
		operation = "json"
	}
	u := "http://docker/v1.48/containers/" + container + "/" + operation
	if id == "docker.stop" || id == "docker.restart" {
		u += "?t=10"
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, errAction
	}
	req.Close = true
	if req.Write(conn) != nil {
		return nil, errAction
	}
	if conn.SetReadDeadline(minTime(deadline, time.Now().Add(5*time.Second))) != nil {
		return nil, errAction
	}
	limited := &io.LimitedReader{R: conn, N: 16 << 10}
	reader := bufio.NewReader(limited)
	response, err := http.ReadResponse(reader, req)
	if err != nil {
		return nil, errAction
	}
	defer func() {
		// Close IPC before Body.Close so rejection cannot drain an oversized or
		// stalled response beyond the explicit read limit.
		closeConn()
		if response.Body.Close() != nil {
			output = nil
			resultErr = errAction
		}
	}()
	limited.N = 1<<63 - 1
	if conn.SetReadDeadline(deadline) != nil {
		return nil, errAction
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, bodyLimit+1))
	if err != nil || len(data) > bodyLimit || !utf8.Valid(data) || response.Header.Get("Content-Encoding") != "" {
		return nil, errAction
	}
	if id == "docker.inspect" {
		if response.StatusCode != http.StatusOK {
			return nil, errAction
		}
		return inspect(data, container)
	}
	if response.StatusCode != http.StatusNoContent && !(response.StatusCode == http.StatusNotModified && id != "docker.restart") {
		return nil, errAction
	}
	return map[string]any{"container_id": container, "changed": response.StatusCode == http.StatusNoContent}, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func inspect(data []byte, container string) (map[string]any, error) {
	// Pointers distinguish missing/null fields from legitimate false/zero values.
	var value struct {
		ID    *string `json:"Id"`
		Name  *string
		Image *string
		State *struct {
			Status   *string
			Running  *bool
			ExitCode *int64
		}
	}
	if json.Unmarshal(data, &value) != nil || value.ID == nil || *value.ID != container || value.Name == nil || value.Image == nil || !strings.HasPrefix(*value.Image, "sha256:") || !hexID(strings.TrimPrefix(*value.Image, "sha256:")) || value.State == nil || value.State.Status == nil || value.State.Running == nil || value.State.ExitCode == nil {
		return nil, errAction
	}
	name := *value.Name
	if len(name) < 2 || len(name) > 256 || name[0] != '/' {
		return nil, errAction
	}
	for _, c := range name[1:] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
			return nil, errAction
		}
	}
	switch *value.State.Status {
	case "created", "running", "paused", "restarting", "removing", "exited", "dead":
	default:
		return nil, errAction
	}
	if *value.State.ExitCode < 0 || *value.State.ExitCode > 2147483647 {
		return nil, errAction
	}
	return map[string]any{"container_id": container, "name": name, "image_id": *value.Image, "status": *value.State.Status, "running": *value.State.Running, "exit_code": *value.State.ExitCode}, nil
}

// localAddr deliberately omits administrator-controlled IPC paths in diagnostics.
type localAddr struct{}

func (localAddr) Network() string { return "docker-local" }
func (localAddr) String() string  { return "docker-local" }

var _ net.Addr = localAddr{}
