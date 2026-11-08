package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/engine"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
	urfave "github.com/urfave/cli/v3"
)

func executionCommands(out, diagnostics io.Writer, runtime *action.Runtime) []*urfave.Command {
	run := &urfave.Command{Name: "run", Usage: "Execute the current workflow revision synchronously", OnUsageError: usageError, ArgValidator: exactArguments(1), Flags: []urfave.Flag{jsonFlag()}, Action: func(ctx context.Context, cmd *urfave.Command) error {
		cfg, err := loadConfig(ctx, cmd, diagnostics)
		if err != nil {
			return err
		}
		return withStore(ctx, cfg.StoragePath, func(store *sqlite.Store) error {
			snap, runErr := (engine.Engine{Workflows: store, Executions: store, Actions: runtime}).Run(ctx, cmd.Args().First())
			if snap.ID != "" {
				snap.Steps = nil
				snap.Definition = nil
				if err := writeExecution(out, cmd.Bool("json"), snap); err != nil {
					return err
				}
			}
			return executionDiagnostic(runErr)
		})
	}}
	group := &urfave.Command{Name: "execution", Usage: "Read durable execution history", OnUsageError: usageError, ArgValidator: noArguments}
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
	return []*urfave.Command{run, group}
}

func executionDiagnostic(err error) error {
	if err == nil {
		return nil
	}
	for _, safe := range []error{execution.ErrInvalid, execution.ErrConflict, execution.ErrNotFound, execution.ErrLimit, execution.ErrAction, execution.ErrPermission, execution.ErrUnavailable, execution.ErrOutput, context.Canceled, context.DeadlineExceeded} {
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
		for _, step := range v.Steps {
			if _, err := fmt.Fprintf(out, "  %s %s %s %s\n", step.ID, step.State, step.Reason, step.Output); err != nil {
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
