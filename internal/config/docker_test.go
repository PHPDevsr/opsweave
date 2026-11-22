package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDockerConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	endpoint := "unix:///tmp/opsweave-test.sock"
	if runtime.GOOS == "windows" {
		endpoint = "npipe:////./pipe/opsweave-test"
	}
	policy := fmt.Sprintf("endpoint: %q, allowed_containers: [%q], allowed_actions: [docker.inspect]", endpoint, strings.Repeat("a", 64))
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("actions: {docker: {enabled: true, " + policy + "}}")
	c, err := Load(t.Context(), path, dir, nil)
	if err != nil || !c.Docker.Enabled || c.Docker.Endpoint != endpoint || len(c.Docker.AllowedActions) != 1 || len(c.Docker.AllowedContainers) != 1 || c.HTTP.Enabled || c.Shell.Enabled {
		t.Fatal(c, err)
	}
	c, err = Load(t.Context(), path, dir, []string{"OPSWEAVE_DOCKER_ENABLED=false", "DOCKER_HOST=tcp://secret-marker:2375"})
	if err != nil || c.Docker.Enabled || c.Docker.Endpoint != endpoint {
		t.Fatal(c, err)
	}
	write("actions: {docker: {enabled: false, " + policy + "}}")
	if c, err = Load(t.Context(), path, dir, []string{"OPSWEAVE_DOCKER_ENABLED=true"}); err != nil || !c.Docker.Enabled {
		t.Fatal(c, err)
	}
	for _, text := range []string{
		"actions: {docker: []}", "actions: {docker: {enabled: yes}}", "actions: {docker: {enabled: true}}",
		"actions: {docker: {endpoint: 42}}", "actions: {docker: {allowed_containers: 42}}", "actions: {docker: {allowed_actions: 42}}",
		"actions: {docker: {allowed_actions: [42]}}", "actions: {docker: {allowed_containers: [true]}}",
		"actions: {docker: {endpoint: 'tcp://secret-marker:2375'}}", "actions: {docker: {unknown: secret-marker}}",
		"actions: {docker: {allowed_containers: [secret-marker]}}", "actions: {docker: {allowed_actions: [docker.exec]}}",
		"actions: {docker: {allowed_actions: [docker.inspect,docker.inspect]}}",
		"actions: {docker: {enabled: true, enabled: false}}",
		"actions: {docker: {allowed_actions: [docker.start,docker.stop,docker.restart,docker.inspect,docker.exec]}}",
	} {
		write(text)
		if _, err := Load(t.Context(), path, dir, nil); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatal(text, err)
		}
	}
	write("actions: {docker: {}}")
	for _, value := range []string{"yes", "TRUE", "1", ""} {
		if _, err := Load(t.Context(), path, dir, []string{"OPSWEAVE_DOCKER_ENABLED=" + value}); err == nil {
			t.Fatal("invalid override accepted", value)
		}
	}
	if _, err := Load(t.Context(), path, dir, []string{"OPSWEAVE_DOCKER_ENDPOINT=secret-marker"}); err == nil || strings.Contains(err.Error(), "secret-marker") {
		t.Fatal(err)
	}
}
