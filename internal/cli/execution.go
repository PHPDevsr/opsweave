package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/engine"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
	urfave "github.com/urfave/cli/v3"
)

func executionCommands(out, diagnostics io.Writer, runtime *action.Runtime) []*urfave.Command {
	run := &urfave.Command{Name: "run", Usage: "Execute the current workflow revision synchronously", OnUsageError: usageError, ArgValidator: exactArguments(1), Flags: []urfave.Flag{jsonFlag(), &urfave.BoolFlag{Name: "enqueue", Usage: "Return durable queued admission without running work"}, &urfave.StringFlag{Name: "idempotency-key", Usage: "Deduplicate admission for seven days"}}, Action: func(ctx context.Context, cmd *urfave.Command) error {
		if cmd.IsSet("idempotency-key") && cmd.String("idempotency-key") == "" {
			return execution.ErrInvalid
		}
		cfg, err := loadConfig(ctx, cmd, diagnostics)
		if err != nil {
			return err
		}
		return managedRun(ctx, cfg, runtime, out, cmd.Bool("json"), cmd.Args().First(), engine.RunOptions{IdempotencyKey: cmd.String("idempotency-key")}, cmd.Bool("enqueue"))
	}}
	group := &urfave.Command{Name: "execution", Usage: "Inspect and manage durable executions", OnUsageError: usageError, ArgValidator: noArguments}
	for _, verb := range []string{"list", "get", "logs"} {
		argc := 1
		if verb == "list" {
			argc = 0
		}
		group.Commands = append(group.Commands, &urfave.Command{Name: verb, Usage: verb + " execution history", OnUsageError: usageError, ArgValidator: exactArguments(argc), Flags: []urfave.Flag{jsonFlag()}, Action: func(ctx context.Context, cmd *urfave.Command) error {
			id := cmd.Args().First()
			if verb != "list" && !execution.ValidID(id) {
				return execution.ErrInvalid
			}
			cfg, err := loadConfig(ctx, cmd, diagnostics)
			if err != nil {
				return err
			}
			return withStore(ctx, cfg.StoragePath, func(store *sqlite.Store) error {
				var value any
				var err error
				switch verb {
				case "list":
					value, err = store.Executions(ctx)
				case "get":
					value, err = store.Execution(ctx, id)
				case "logs":
					value, err = store.Events(ctx, id)
				}
				if err != nil {
					return executionDiagnostic(err)
				}
				return writeExecution(out, cmd.Bool("json"), value)
			})
		}})
	}
	for _, verb := range []string{"cancel", "retry"} {
		flags := []urfave.Flag{jsonFlag()}
		if verb == "cancel" {
			flags = append(flags, &urfave.StringFlag{Name: "step", Usage: "Cancel one step only"})
		} else {
			flags = append(flags, &urfave.BoolFlag{Name: "enqueue", Usage: "Return durable queued replay admission"}, &urfave.BoolFlag{Name: "ack-uncertain", Usage: "Acknowledge possible repeated external effects"}, &urfave.StringFlag{Name: "idempotency-key", Usage: "Deduplicate admission for seven days"})
		}
		group.Commands = append(group.Commands, &urfave.Command{Name: verb, Usage: verb + " an execution", OnUsageError: usageError, ArgValidator: exactArguments(1), Flags: flags, Action: func(ctx context.Context, cmd *urfave.Command) error {
			id := cmd.Args().First()
			if !execution.ValidID(id) || (cmd.IsSet("step") && cmd.String("step") == "") || (cmd.IsSet("idempotency-key") && cmd.String("idempotency-key") == "") {
				return execution.ErrInvalid
			}
			cfg, err := loadConfig(ctx, cmd, diagnostics)
			if err != nil {
				return err
			}
			if verb == "retry" {
				return managedRun(ctx, cfg, runtime, out, cmd.Bool("json"), "", engine.RunOptions{Parent: id, IdempotencyKey: cmd.String("idempotency-key"), AcknowledgedUncertain: cmd.Bool("ack-uncertain")}, cmd.Bool("enqueue"))
			}
			return withStore(ctx, cfg.StoragePath, func(store *sqlite.Store) error {
				if verb == "cancel" {
					snap, err := store.RequestReliableCancellation(ctx, id, cmd.String("step"))
					if err != nil {
						return executionDiagnostic(err)
					}
					status := "cancellation_requested"
					if execution.Terminal(snap.State) {
						status = string(snap.State)
					}
					return writeCancellation(out, cmd.Bool("json"), status, snap, cmd.String("step"))
				}
				return execution.ErrInvalid
			})
		}})
	}
	return []*urfave.Command{run, group}
}

func writeCancellation(out io.Writer, asJSON bool, status string, snap execution.Snapshot, target string) error {
	var stepState execution.State
	for _, step := range snap.Steps {
		if step.ID == target {
			stepState = step.State
			if execution.Terminal(stepState) {
				status = string(stepState)
			}
		}
	}
	value := struct {
		Status    string          `json:"status"`
		Execution string          `json:"execution"`
		State     execution.State `json:"state"`
		Step      string          `json:"step,omitempty"`
		StepState execution.State `json:"stepState,omitempty"`
	}{status, snap.ID, snap.State, target, stepState}
	if asJSON {
		return json.NewEncoder(out).Encode(value)
	}
	if target != "" {
		_, err := fmt.Fprintf(out, "%s %s step=%s state=%s execution=%s\n", status, snap.ID, target, stepState, snap.State)
		return err
	}
	_, err := fmt.Fprintf(out, "%s %s %s\n", status, snap.ID, snap.State)
	return err
}

func executionDiagnostic(err error) error {
	if err == nil {
		return nil
	}
	for _, safe := range []error{execution.ErrInvalid, execution.ErrConflict, execution.ErrNotFound, execution.ErrLimit, execution.ErrAction, execution.ErrPermission, execution.ErrUnavailable, execution.ErrOutput, execution.ErrReference, execution.ErrUncertain, execution.ErrBusy, execution.ErrQueueFull, execution.ErrUnmanaged, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, safe) {
			return safe
		}
	}
	var failure *execution.Failure
	if errors.As(err, &failure) {
		return errors.New(failure.Error())
	}
	return storageDiagnostic(err)
}

func writeExecution(out io.Writer, asJSON bool, value any) error {
	if asJSON {
		return json.NewEncoder(out).Encode(value)
	}
	switch v := value.(type) {
	case execution.Snapshot:
		if _, err := fmt.Fprintf(out, "Execution: %s\nWorkflow: %s\nRevision: %s\nState: %s\n", v.ID, v.Workflow, v.Revision, v.State); err != nil {
			return err
		}
		if v.Trigger != nil {
			if _, err := fmt.Fprintf(out, "Trigger: %s %s", v.Trigger.Type, v.Trigger.ID); err != nil {
				return err
			}
			if !v.Trigger.ScheduledAt.IsZero() {
				if _, err := fmt.Fprintf(out, " at %s", v.Trigger.ScheduledAt.Format(time.RFC3339)); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
		for _, step := range v.Steps {
			if _, err := fmt.Fprintf(out, "  %s %s %s %s\n", step.ID, step.State, step.Reason, step.Output); err != nil {
				return err
			}
			for _, attempt := range step.Attempts {
				if _, err := fmt.Fprintf(out, "    Attempt %d: %s %s\n", attempt.Number, attempt.State, attempt.Reason); err != nil {
					return err
				}
			}
		}
		if v.Parent != "" {
			if _, err := fmt.Fprintf(out, "Parent: %s\n", v.Parent); err != nil {
				return err
			}
		}
		if len(v.Definition) > 0 {
			_, err := fmt.Fprintf(out, "Definition: %s\n", v.Definition)
			return err
		}
	case []execution.Snapshot:
		for _, row := range v {
			if _, err := fmt.Fprintf(out, "%s %s %s %s\n", row.ID, row.Workflow, row.Revision, row.State); err != nil {
				return err
			}
		}
	case []execution.Event:
		for _, event := range v {
			if _, err := fmt.Fprintf(out, "%d %s %s %s %s\n", event.Sequence, event.Execution, event.Step, event.State, event.Reason); err != nil {
				return err
			}
		}
	}
	return nil
}
