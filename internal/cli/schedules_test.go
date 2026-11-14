package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/PHPDevsr/opsweave/internal/scheduler"
)

func TestScheduleCLIInspection(t *testing.T) {
	cfg, input, _ := setupFoundation(t)
	source, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.Replace(string(source), "spec:", "spec:\n  triggers: [{id: daily, type: cron, schedule: '0 1 * * *', timezone: Asia/Jakarta}]", 1)
	if err := os.WriteFile(input, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCatalog(t, testCatalog(), cfg, "apply", input); err != nil {
		t.Fatal(err)
	}
	out, err := runCatalog(t, testCatalog(), cfg, "workflow", "schedules", "--json", "example")
	var entries []scheduler.Entry
	if err != nil || json.Unmarshal([]byte(out), &entries) != nil || len(entries) != 1 || entries[0].Trigger.MissedRun != "skip" || !entries[0].Enabled {
		t.Fatal(out, err)
	}
	if _, err := runCatalog(t, testCatalog(), cfg, "workflow", "disable", "example"); err != nil {
		t.Fatal(err)
	}
	out, err = runCatalog(t, testCatalog(), cfg, "workflow", "schedules", "example")
	if err != nil || !strings.Contains(out, "enabled=false") {
		t.Fatal(out, err)
	}
	out, err = runCatalog(t, testCatalog(), cfg, "workflow", "schedule-logs", "--json", "example")
	var history scheduler.History
	if err != nil || json.Unmarshal([]byte(out), &history) != nil || history.Events == nil || len(history.Events) != 0 {
		t.Fatal(out, err)
	}
	if _, err = runCatalog(t, testCatalog(), cfg, "workflow", "schedule-logs", "--limit", "1001", "example"); err == nil {
		t.Fatal("accepted oversized history request")
	}
}
