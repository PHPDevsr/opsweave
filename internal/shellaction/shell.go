// Package shellaction executes explicitly configured trusted command profiles.
package shellaction

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/registry"
)

const outputLimit = 8192

// Profile is an administrator-controlled executable, directory and fixed argv prefix.
type Profile struct {
	Name             string
	Executable       string
	WorkingDirectory string
	Args             []string
}

// Config defaults to disabled; no executable is implicitly permitted.
type Config struct {
	Enabled  bool
	Commands []Profile
}

var errConfig = errors.New("invalid shell action configuration")
var errCommand = errors.New("shell action failed")

// cleanupBudget is shared by cancellation and final cleanup, never renewed.
type cleanupBudget struct {
	once     sync.Once
	deadline time.Time
}

func (b *cleanupBudget) until() time.Time {
	b.once.Do(func() { b.deadline = time.Now().Add(2 * time.Second) })
	return b.deadline
}

func validName(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return s != "." && s != ".."
}

func validArgs(args []string) bool {
	if len(args) > 64 {
		return false
	}
	size := 0
	for _, arg := range args {
		size += len(arg)
		if size > 8192 || !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
			return false
		}
	}
	return true
}

func validPath(s string) bool {
	return filepath.IsAbs(s) && utf8.ValidString(s) && !strings.ContainsRune(s, 0) && !strings.HasPrefix(s, `\\`) && !strings.HasPrefix(s, "//") && !strings.Contains(strings.TrimPrefix(s, filepath.VolumeName(s)), ":")
}

func (p Profile) validate() error {
	if !validName(p.Name) || !validPath(p.Executable) || !validPath(p.WorkingDirectory) || !validArgs(p.Args) {
		return errConfig
	}
	file, err := os.Stat(p.Executable)
	if err != nil || !file.Mode().IsRegular() || !executable(file) {
		return errConfig
	}
	dir, err := os.Stat(p.WorkingDirectory)
	if err != nil || !dir.IsDir() {
		return errConfig
	}
	return nil
}

// Validate checks profiles without launching any process. Paths must exist locally.
func (c Config) Validate() error {
	if len(c.Commands) > 128 || (c.Enabled && len(c.Commands) == 0) {
		return errConfig
	}
	seen := map[string]bool{}
	for _, p := range c.Commands {
		if p.validate() != nil || seen[p.Name] {
			return errConfig
		}
		seen[p.Name] = true
	}
	return nil
}

// Registrations snapshots enabled profiles for the existing action executor.
func Registrations(c Config) ([]action.Registration, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !c.Enabled {
		return nil, nil
	}
	profiles := map[string]Profile{}
	for _, p := range c.Commands {
		p.Args = append([]string(nil), p.Args...)
		profiles[p.Name] = p
	}
	d := registry.Descriptor{ID: "shell.exec", Version: "1", APIVersion: "opsweave.io/v1", Permissions: []string{"shell.exec"}, Inputs: registry.Field{Type: "object", Properties: map[string]registry.Field{
		"command": {Type: "string", Required: true}, "args": {Type: "array", Items: &registry.Field{Type: "string"}},
	}}, Outputs: map[string]registry.Field{"exit_code": {Type: "integer", Required: true}, "stdout": {Type: "string", Required: true}, "stderr": {Type: "string", Required: true}}}
	return []action.Registration{{Descriptor: d, Run: func(ctx context.Context, in map[string]any) (map[string]any, error) {
		name, ok := in["command"].(string)
		if !ok {
			return nil, errCommand
		}
		p, ok := profiles[name]
		if !ok {
			return nil, errCommand
		}
		args := append([]string(nil), p.Args...)
		if value, exists := in["args"]; exists {
			values, ok := value.([]any)
			if !ok || len(values) > 64 {
				return nil, errCommand
			}
			for _, value := range values {
				arg, ok := value.(string)
				if !ok {
					return nil, errCommand
				}
				args = append(args, arg)
			}
		}
		if !validArgs(args) || p.validate() != nil {
			return nil, errCommand
		}
		return run(ctx, p, args)
	}}}, nil
}

// capture has exactly one os/exec copy owner. Results are read only after Wait
// has joined both stream-copy goroutines, including its WaitDelay failure path.
type capture struct {
	data     []byte
	overflow bool
	cancel   context.CancelFunc
}

func (c *capture) Write(p []byte) (int, error) {
	remaining := outputLimit - len(c.data)
	if len(p) > remaining {
		c.data = append(c.data, p[:remaining]...)
		c.overflow = true
		c.cancel()
		return remaining, errCommand
	}
	c.data = append(c.data, p...)
	return len(p), nil
}

func run(parent context.Context, p Profile, args []string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.Executable, args...)
	cmd.Dir = p.WorkingDirectory
	env, err := environment()
	if err != nil {
		return nil, errCommand
	}
	cmd.Env = env
	cmd.WaitDelay = 2 * time.Second
	stdout, stderr := capture{cancel: cancel}, capture{cancel: cancel}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	owner, err := prepare(cmd)
	if err != nil {
		return nil, errCommand
	}
	// All ownership resources are closed only after os/exec has joined its owners.
	if err := cmd.Start(); err != nil {
		return nil, errors.Join(errCommand, owner.close())
	}
	if err := owner.bind(cmd); err != nil {
		cancel()
		killErr := cmd.Process.Kill()
		waitErr := cmd.Wait()
		return nil, errors.Join(errCommand, killErr, waitErr, owner.close())
	}
	waitErr := cmd.Wait()
	cleanupErr := owner.close()
	if parent.Err() != nil {
		return nil, parent.Err()
	}
	if waitErr != nil || cleanupErr != nil || ctx.Err() != nil || stdout.overflow || stderr.overflow || !utf8.Valid(stdout.data) || !utf8.Valid(stderr.data) {
		return nil, errCommand
	}
	return map[string]any{"exit_code": int64(0), "stdout": string(stdout.data), "stderr": string(stderr.data)}, nil
}
