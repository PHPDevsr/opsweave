package dockeraction

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/testutil/localipc"
)

const containerID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const imageID = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func inspectJSON() string {
	return fmt.Sprintf(`{"Id":%q,"Name":"/fixture","Image":%q,"State":{"Status":"running","Running":true,"ExitCode":0},"Config":{"Env":["TOKEN=secret-marker"]},"Mounts":[{"Source":"secret-marker"}]}`, containerID, imageID)
}

func response(status int, body string) string {
	return fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", status, http.StatusText(status), len(body), body)
}

func serveResponse(t testing.TB, raw string, check func(*http.Request)) string {
	t.Helper()
	return localipc.Serve(t, func(conn net.Conn) {
		req, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			t.Error(err)
			return
		}
		if check != nil {
			check(req)
		}
		if err := req.Body.Close(); err != nil {
			t.Error(err)
		}
		// Rejection/cancellation may close IPC before this deliberately bad
		// response is fully written; that is the behavior being exercised.
		_, _ = io.WriteString(conn, raw)
	})
}

func runtimeFor(t testing.TB, endpoint string, ids ...string) *action.Runtime {
	t.Helper()
	entries, err := Registrations(Config{Enabled: true, Endpoint: endpoint, AllowedContainers: []string{containerID}, AllowedActions: ids})
	if err != nil {
		t.Fatal(err)
	}
	r, err := action.New(entries, ids)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func policyEndpoint() string {
	if runtime.GOOS == "windows" {
		return "npipe:////./pipe/opsweave-fixture"
	}
	return "unix:///tmp/opsweave-fixture.sock"
}

func TestDockerPolicyAndOptIn(t *testing.T) {
	c := Config{Enabled: true, Endpoint: policyEndpoint(), AllowedContainers: []string{containerID}, AllowedActions: []string{"docker.inspect"}}
	if c.Validate() != nil {
		t.Fatal("valid policy rejected")
	}
	if entries, err := Registrations(Config{}); err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	for _, bad := range []Config{
		{Enabled: true},
		{Endpoint: "tcp://127.0.0.1:2375"}, {Endpoint: "unix://host/secret-marker"}, {Endpoint: "npipe:////remote/pipe/secret-marker"},
		{Endpoint: "unix:///tmp/../secret-marker"}, {Endpoint: "unix:///tmp/secret-marker?x=y"}, {Endpoint: "unix:///tmp/%2fsecret-marker"},
		{Endpoint: "npipe:////./pipe/../secret-marker"}, {Endpoint: "npipe:////./pipe/name\\child"},
		{AllowedContainers: []string{"fixture"}}, {AllowedContainers: []string{strings.ToUpper(containerID)}},
		{AllowedContainers: []string{containerID, containerID}}, {AllowedContainers: make([]string, 129)},
		{AllowedActions: []string{"docker.exec"}}, {AllowedActions: []string{"docker.inspect", "docker.inspect"}},
		{Enabled: true, Endpoint: policyEndpoint(), AllowedContainers: []string{containerID}},
		{Enabled: true, Endpoint: policyEndpoint(), AllowedActions: []string{"docker.inspect"}},
	} {
		if err := bad.Validate(); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatal("unsafe policy accepted or disclosed", bad, err)
		}
		if _, err := Registrations(bad); err == nil {
			t.Fatal("invalid registrations")
		}
	}
}

func TestDockerOperationsAndFilteredInspect(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:1")
	t.Setenv("DOCKER_CONTEXT", "secret-marker")
	for _, scenario := range []struct {
		id        string
		status    int
		operation string
		changed   bool
	}{
		{"docker.start", 204, "start", true}, {"docker.start", 304, "start", false},
		{"docker.stop", 204, "stop?t=10", true}, {"docker.stop", 304, "stop?t=10", false},
		{"docker.restart", 204, "restart?t=10", true}, {"docker.inspect", 200, "json", false},
	} {
		t.Run(fmt.Sprintf("%s/%d", scenario.id, scenario.status), func(t *testing.T) {
			body := ""
			if scenario.id == "docker.inspect" {
				body = inspectJSON()
			}
			var requests atomic.Int32
			endpoint := serveResponse(t, response(scenario.status, body), func(req *http.Request) {
				requests.Add(1)
				wantMethod := http.MethodPost
				if scenario.id == "docker.inspect" {
					wantMethod = http.MethodGet
				}
				if req.Method != wantMethod || req.RequestURI != "/v1.48/containers/"+containerID+"/"+scenario.operation || req.Host != "docker" || !req.Close {
					t.Error("incorrect fixed request", req.Method, req.RequestURI, req.Host, req.Close)
				}
				for _, key := range []string{"Authorization", "Cookie", "Accept-Encoding", "Proxy-Authorization"} {
					if req.Header.Get(key) != "" {
						t.Error("unexpected header", key)
					}
				}
			})
			out, err := runtimeFor(t, endpoint, scenario.id).Invoke(t.Context(), scenario.id, "1", map[string]any{"container": containerID})
			if err != nil || requests.Load() != 1 {
				t.Fatal(string(out), requests.Load(), err)
			}
			var result map[string]any
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatal(err)
			}
			if result["container_id"] != containerID || strings.Contains(string(out), "secret-marker") {
				t.Fatal(string(out))
			}
			if scenario.id == "docker.inspect" {
				if len(result) != 6 || result["name"] != "/fixture" || result["image_id"] != imageID || result["status"] != "running" || result["running"] != true || result["exit_code"] != float64(0) {
					t.Fatal(result)
				}
			} else if len(result) != 2 || result["changed"] != scenario.changed {
				t.Fatal(result)
			}
		})
	}
}

func TestDockerAllowlistSnapshotAndPermissions(t *testing.T) {
	var requests atomic.Int32
	endpoint := serveResponse(t, response(200, inspectJSON()), func(*http.Request) { requests.Add(1) })
	c := Config{Enabled: true, Endpoint: endpoint, AllowedContainers: []string{containerID}, AllowedActions: []string{"docker.inspect"}}
	entries, err := Registrations(c)
	if err != nil {
		t.Fatal(err)
	}
	c.AllowedContainers[0] = strings.Repeat("c", 64)
	c.AllowedActions[0] = "docker.start"
	c.Endpoint = "tcp://127.0.0.1:1"
	r, err := action.New(entries, []string{"docker.inspect"})
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []map[string]any{
		{"container": strings.Repeat("c", 64)}, {"container": "fixture"}, {"container": containerID[:12]},
		{"container": containerID + "/start"}, {"container": strings.ToUpper(containerID)}, {"container": 42},
		{"container": containerID, "endpoint": "tcp://127.0.0.1:1"}, {},
	} {
		if out, err := r.Invoke(t.Context(), "docker.inspect", "1", in); err == nil || len(out) != 0 {
			t.Fatal(out, err)
		}
	}
	if err := r.Check("docker.start", "1"); !errors.Is(err, execution.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := r.Check("docker.inspect", "2"); !errors.Is(err, execution.ErrUnavailable) {
		t.Fatal(err)
	}
	denied, err := action.New(entries, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := denied.Invoke(t.Context(), "docker.inspect", "1", map[string]any{"container": containerID}); !errors.Is(err, execution.ErrPermission) {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatal("rejection connected to daemon")
	}
	if _, err := r.Invoke(t.Context(), "docker.inspect", "1", map[string]any{"container": containerID}); err != nil || requests.Load() != 1 {
		t.Fatal(requests.Load(), err)
	}
}

func TestDockerResponseRejections(t *testing.T) {
	for name, raw := range map[string]string{
		"daemon_error":    response(500, `{"message":"secret-marker"}`),
		"redirect":        "HTTP/1.1 302 Found\r\nLocation: http://127.0.0.1:1/secret-marker\r\nContent-Length: 0\r\n\r\n",
		"api_unsupported": response(400, `{"message":"client API too new"}`),
		"invalid_status":  "secret-marker\r\n\r\n",
		"oversize_header": "HTTP/1.1 200 OK\r\nX-Value: " + strings.Repeat("a", 16<<10) + "\r\nContent-Length: 0\r\n\r\n",
		"oversize_body":   response(200, strings.Repeat("a", bodyLimit+1)),
		"invalid_utf8":    response(200, "\xff"),
		"encoded":         "HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: 0\r\n\r\n",
		"truncated":       "HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n{}",
	} {
		t.Run(name, func(t *testing.T) {
			endpoint := serveResponse(t, raw, nil)
			out, err := runtimeFor(t, endpoint, "docker.inspect").Invoke(t.Context(), "docker.inspect", "1", map[string]any{"container": containerID})
			if !errors.Is(err, execution.ErrAction) || len(out) != 0 || strings.Contains(err.Error(), "secret-marker") {
				t.Fatal(string(out), err)
			}
		})
	}
	t.Run("restart_304", func(t *testing.T) {
		endpoint := serveResponse(t, response(304, ""), nil)
		if out, err := runtimeFor(t, endpoint, "docker.restart").Invoke(t.Context(), "docker.restart", "1", map[string]any{"container": containerID}); !errors.Is(err, execution.ErrAction) || len(out) != 0 {
			t.Fatal(out, err)
		}
	})
}

func TestDockerInspectValidation(t *testing.T) {
	good := inspectJSON()
	for _, bad := range []string{"null", "{}", "[]", good + "{}", strings.Replace(good, containerID, strings.Repeat("c", 64), 1), strings.Replace(good, "/fixture", "/bad/path", 1), strings.Replace(good, "/fixture", "bad", 1), strings.Replace(good, imageID, "secret-marker", 1), strings.Replace(good, `"running"`, `"unknown"`, 1), strings.Replace(good, `"Running":true`, `"Running":null`, 1), strings.Replace(good, `"ExitCode":0`, `"ExitCode":-1`, 1), strings.Replace(good, `"ExitCode":0`, `"ExitCode":1.5`, 1), strings.Replace(good, `"State":{`, `"State":null,"ignored":{`, 1)} {
		if out, err := inspect([]byte(bad), containerID); err == nil || len(out) != 0 {
			t.Fatal(out, err)
		}
	}
}

func TestDockerCancellationDeadlineAndPeerClosure(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint("deadline=", deadline), func(t *testing.T) {
			ready := make(chan struct{})
			peerClosed := make(chan struct{})
			endpoint := localipc.Serve(t, func(conn net.Conn) {
				if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
					t.Error(err)
					return
				}
				close(ready)
				_, _ = io.Copy(io.Discard, conn)
				close(peerClosed)
			})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), time.Second)
			}
			defer cancel()
			done := make(chan error, 1)
			r := runtimeFor(t, endpoint, "docker.inspect")
			go func() {
				_, err := r.Invoke(ctx, "docker.inspect", "1", map[string]any{"container": containerID})
				done <- err
				close(done)
			}()
			t.Cleanup(func() { cancel(); <-done })
			select {
			case <-ready:
			case <-ctx.Done():
				t.Fatal("request never arrived", ctx.Err())
			}
			want := context.DeadlineExceeded
			if !deadline {
				cancel()
				want = context.Canceled
			}
			select {
			case err := <-done:
				if !errors.Is(err, want) {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("action did not join")
			}
			select {
			case <-peerClosed:
			case <-time.After(3 * time.Second):
				t.Fatal("owned connection survived cancellation")
			}
		})
	}
}

func TestDockerUnavailableEndpoint(t *testing.T) {
	endpoint := policyEndpoint()
	if _, err := runtimeFor(t, endpoint, "docker.inspect").Invoke(t.Context(), "docker.inspect", "1", map[string]any{"container": containerID}); !errors.Is(err, execution.ErrAction) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if conn, err := dialLocal(ctx, endpoint); err == nil {
		conn.Close()
		t.Fatal("canceled dial succeeded")
	}
}

func TestDockerOversizeResponseDoesNotDrain(t *testing.T) {
	closed := make(chan struct{})
	endpoint := localipc.Serve(t, func(conn net.Conn) {
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
			t.Error(err)
			return
		}
		_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", bodyLimit*4, strings.Repeat("x", bodyLimit+1))
		// Deliberately never provide the remainder: rejection must close IPC,
		// rather than draining the declared response until the action deadline.
		_, _ = io.Copy(io.Discard, conn)
		close(closed)
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	out, err := runtimeFor(t, endpoint, "docker.inspect").Invoke(ctx, "docker.inspect", "1", map[string]any{"container": containerID})
	if !errors.Is(err, execution.ErrAction) || len(out) != 0 || ctx.Err() != nil {
		t.Fatal("oversize response waited for deadline", out, err, ctx.Err())
	}
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("rejection left daemon connection open")
	}
}

func TestDockerConcurrentInvocations(t *testing.T) {
	endpoint := serveResponse(t, response(200, inspectJSON()), nil)
	r := runtimeFor(t, endpoint, "docker.inspect")
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	var owners sync.WaitGroup
	results := make(chan error, 16)
	for range 16 {
		owners.Go(func() {
			_, err := r.Invoke(ctx, "docker.inspect", "1", map[string]any{"container": containerID})
			results <- err
		})
	}
	owners.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Error(err)
		}
	}
}

func FuzzDockerPolicy(f *testing.F) {
	f.Add(policyEndpoint(), containerID, "docker.inspect")
	f.Add("tcp://127.0.0.1:2375", "../fixture", "docker.exec")
	f.Fuzz(func(t *testing.T, endpoint, container, id string) {
		if len(endpoint)+len(container)+len(id) > 65536 {
			return
		}
		c := Config{Enabled: true, Endpoint: endpoint, AllowedContainers: []string{container}, AllowedActions: []string{id}}
		if err := c.Validate(); err == nil && (!validEndpoint(endpoint) || !hexID(container) || !validAction(id)) {
			t.Fatal("unsafe policy accepted")
		}
	})
}

func FuzzDockerInspect(f *testing.F) {
	f.Add(inspectJSON())
	f.Add("null")
	f.Add("{\"Id\":[]}")
	f.Fuzz(func(t *testing.T, data string) {
		if len(data) > bodyLimit {
			return
		}
		out, err := inspect([]byte(data), containerID)
		if err == nil {
			encoded, err := json.Marshal(out)
			if err != nil || len(encoded) > 65536 {
				t.Fatal("unbounded output", err)
			}
		}
	})
}

func BenchmarkDockerInspectIPC(b *testing.B) {
	endpoint := serveResponse(b, response(200, inspectJSON()), nil)
	r := runtimeFor(b, endpoint, "docker.inspect")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.Invoke(b.Context(), "docker.inspect", "1", map[string]any{"container": containerID}); err != nil {
			b.Fatal(err)
		}
	}
}
