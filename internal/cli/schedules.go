package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
	urfave "github.com/urfave/cli/v3"
)

func scheduleCommands(out, diagnostics io.Writer) []*urfave.Command {
	result := []*urfave.Command{}
	for _, verb := range []string{"schedules", "schedule-logs"} {
		flags := []urfave.Flag{jsonFlag()}
		if verb == "schedule-logs" {
			flags = append(flags, &urfave.IntFlag{Name: "limit", Value: 100, Usage: "Newest schedule events (1–1000)"})
		}
		result = append(result, &urfave.Command{Name: verb, Usage: "Inspect durable workflow " + verb, OnUsageError: usageError, ArgValidator: exactArguments(1), Flags: flags, Action: func(ctx context.Context, cmd *urfave.Command) error {
			if verb == "schedule-logs" && (cmd.Int("limit") < 1 || cmd.Int("limit") > 1000) {
				return execution.ErrInvalid
			}
			cfg, err := loadConfig(ctx, cmd, diagnostics)
			if err != nil {
				return err
			}
			return withStore(ctx, cfg.StoragePath, func(store *sqlite.Store) error {
				name := cmd.Args().First()
				if verb == "schedules" {
					entries, err := store.Schedules(ctx, name)
					if err != nil {
						return storageDiagnostic(err)
					}
					if cmd.Bool("json") {
						return json.NewEncoder(out).Encode(entries)
					}
					for _, entry := range entries {
						if _, err = fmt.Fprintf(out, "%s %s %s %s enabled=%t cursor=%s\n", entry.Trigger.ID, entry.Trigger.Schedule, entry.Trigger.Timezone, entry.Trigger.MissedRun, entry.Enabled, entry.Cursor.Format(time.RFC3339Nano)); err != nil {
							return err
						}
					}
					return nil
				}
				history, err := store.ScheduleEvents(ctx, name, int(cmd.Int("limit")))
				if err != nil {
					return executionDiagnostic(err)
				}
				if cmd.Bool("json") {
					return json.NewEncoder(out).Encode(history)
				}
				for _, event := range history.Events {
					if _, err = fmt.Fprintf(out, "%d %s %s %s %s\n", event.Sequence, event.Time.Format(time.RFC3339), event.Trigger, event.Code, event.Execution); err != nil {
						return err
					}
				}
				if history.HasMore {
					_, err = fmt.Fprintln(out, "More schedule events are retained; increase --limit (maximum 1000).")
				}
				return err
			})
		}})
	}
	return result
}
