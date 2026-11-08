package registry

import (
	"strings"
	"testing"
)

func TestPrepareReliablePreservesCapabilityAndSecurityGates(t *testing.T) {
	for _, doc := range []string{fixture + "      if: 'true'\n", fixture + "      retry: {attempts: 2, backoff: 1ms, maxBackoff: 1ms}\n", strings.Replace(fixture, "spec:\n", "spec:\n  timeout: 2s\n", 1)} {
		candidate, err := PrepareReliable(t.Context(), []Source{{Data: []byte(doc)}}, fixtureCatalog())
		if err != nil || len(candidate) != 1 || len(candidate[0].Capabilities) != 1 || candidate[0].Capabilities[0] != "serial_reliability" {
			t.Fatal(candidate, err)
		}
		if _, err := Prepare(t.Context(), []Source{{Data: []byte(doc)}}, fixtureCatalog()); err == nil {
			t.Fatal("legacy prepare silently adopted reliable policies")
		}
	}
	for _, doc := range []string{strings.Replace(fixture, "spec:\n", "spec:\n  parallelism: 2\n", 1), strings.Replace(fixture, "spec:\n", "spec:\n  concurrency: {limit: 2}\n", 1), strings.Replace(fixture, "message: hello", "message: {$ref: secrets.prod.password}", 1)} {
		if _, err := PrepareReliable(t.Context(), []Source{{Data: []byte(doc)}}, fixtureCatalog()); err == nil {
			t.Fatal("unsupported capability enabled")
		}
	}
}
