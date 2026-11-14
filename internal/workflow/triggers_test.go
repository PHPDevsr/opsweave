package workflow

import (
	"strings"
	"testing"
)

func TestCronTriggerValidationAndCapabilityGates(t *testing.T) {
	trigger := "  triggers:\n    - {id: daily, type: cron, schedule: '0 2 * * *', timezone: UTC}\n"
	doc := strings.Replace(minimal, "spec:", "spec:\n"+strings.TrimSuffix(trigger, "\n"), 1)
	parsed, err := Parse(t.Context(), []byte(doc))
	if err != nil || len(parsed.Spec.Triggers) != 1 || parsed.Spec.Triggers[0].MissedRun != "skip" {
		t.Fatal(parsed, err)
	}
	if parsed.Phase01() == nil || parsed.SerialReliability() == nil || parsed.DispatcherCapabilities() != nil {
		t.Fatal("legacy capability gate changed")
	}
	for _, replacement := range []string{
		"  triggers: [{id: hook, type: webhook, event: build}]", "  triggers: [{id: boot, type: startup, timezone: UTC}]", "  triggers: [{id: bus, type: internal}]",
		"  triggers: []", "  triggers: {}", "  triggers: [{id: daily, type: unsupported}]",
		"  triggers: [{id: daily, type: cron, schedule: '* * * * *'}]",
		"  triggers: [{id: daily, type: cron, schedule: 1, timezone: UTC}]",
		"  triggers: [{id: daily, type: cron, schedule: '* * * * *', timezone: true}]",
		"  triggers: [{id: daily, type: cron, schedule: '* * * * *', timezone: UTC, missed_run: invalid}]",
		"  triggers: [{id: daily, type: cron, schedule: '* * * * *', timezone: UTC, unknown: private-marker}]",
		"  triggers: [{id: daily, type: cron, schedule: '@daily', timezone: UTC}]",
		"  triggers: [{id: daily, type: cron, schedule: '* * * * *', timezone: Local}]",
		"  triggers:\n" + strings.Repeat("    - {id: daily, type: cron, schedule: '* * * * *', timezone: UTC}\n", 2),
		"  triggers:\n" + strings.Repeat("    - {}\n", 17),
	} {
		source := strings.Replace(minimal, "spec:", "spec:\n"+replacement, 1)
		if _, err := Parse(t.Context(), []byte(source)); err == nil || strings.Contains(err.Error(), "private-marker") {
			t.Fatal(replacement, err)
		}
	}
}
