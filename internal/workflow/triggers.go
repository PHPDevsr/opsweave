package workflow

import (
	"fmt"
	"github.com/PHPDevsr/opsweave/internal/schedule"
	"go.yaml.in/yaml/v3"
)

func parseTriggers(n *yaml.Node) ([]schedule.Trigger, error) {
	if n == nil {
		return nil, nil
	}
	if n.Kind != yaml.SequenceNode || len(n.Content) < 1 || len(n.Content) > 16 {
		return nil, invalid(n, "spec.triggers", "invalid_triggers")
	}
	result := make([]schedule.Trigger, 0, len(n.Content))
	seen := map[string]bool{}
	for i, node := range n.Content {
		path := fmt.Sprintf("spec.triggers[%d]", i)
		f, err := fields(node, path, "id", "type", "schedule", "timezone", "missed_run")
		if err != nil {
			return nil, err
		}
		trigger := schedule.Trigger{MissedRun: "skip"}
		if trigger.ID, err = idValue(f["id"], path+".id"); err != nil {
			return nil, err
		}
		if seen[trigger.ID] {
			return nil, invalid(node, path, "duplicate_trigger")
		}
		seen[trigger.ID] = true
		if trigger.Type, err = textValue(f["type"], path+".type"); err != nil {
			return nil, err
		}
		if trigger.Type != "cron" {
			return nil, invalid(f["type"], path+".type", "unsupported_capability")
		}
		if trigger.Schedule, err = textValue(f["schedule"], path+".schedule"); err != nil {
			return nil, err
		}
		if trigger.Timezone, err = textValue(f["timezone"], path+".timezone"); err != nil {
			return nil, err
		}
		if f["missed_run"] != nil {
			if trigger.MissedRun, err = textValue(f["missed_run"], path+".missed_run"); err != nil {
				return nil, err
			}
		}
		if trigger.MissedRun != "skip" && trigger.MissedRun != "catch_up_once" {
			return nil, invalid(node, path+".missed_run", "invalid_schedule_policy")
		}
		if _, err = schedule.Compile(trigger.Schedule, trigger.Timezone, nil); err != nil {
			return nil, invalid(node, path, "invalid_schedule")
		}
		result = append(result, trigger)
	}
	return result, nil
}
