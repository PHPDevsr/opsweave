package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/PHPDevsr/opsweave/internal/dispatchlock"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
	urfave "github.com/urfave/cli/v3"
)

func pruneCommand(out, diagnostics io.Writer) *urfave.Command {
	return &urfave.Command{Name: "prune", Usage: "Preview or delete eligible execution history", OnUsageError: usageError, ArgValidator: noArguments,
		Flags: []urfave.Flag{jsonFlag(), &urfave.BoolFlag{Name: "dry-run", Usage: "Preview eligibility"}, &urfave.BoolFlag{Name: "apply", Usage: "Delete one batch of up to 100 executions"}},
		Action: func(ctx context.Context, cmd *urfave.Command) error {
			dry := cmd.Bool("dry-run")
			if dry == cmd.Bool("apply") {
				return errors.New("select exactly one of --dry-run and --apply")
			}
			cfg, err := loadConfig(ctx, cmd, diagnostics)
			if err != nil {
				return err
			}
			if cfg.RetentionAge == 0 {
				return errors.New("execution retention requires a configured retention_age")
			}
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			var report sqlite.PruneReport
			if dry {
				report, err = sqlite.PreviewPrune(ctx, cfg.StoragePath, cfg.RetentionAge)
			} else {
				err = withStore(ctx, cfg.StoragePath, func(s *sqlite.Store) error {
					var applyErr error
					report, applyErr = s.PruneExecutions(ctx, cfg.RetentionAge, false)
					return applyErr
				})
			}
			if err != nil {
				return executionDiagnostic(err)
			}
			if cmd.Bool("json") {
				return json.NewEncoder(out).Encode(report)
			}
			_, err = fmt.Fprintf(out, "eligible=%d deleted=%d active=%d within_retention=%d protected_by_receipt=%d referenced_by_replay=%d\n",
				report.Eligible, report.Deleted, report.Active, report.WithinRetention, report.Protected, report.Referenced)
			return err
		}}
}

func serverStatusCommand(out, diagnostics io.Writer) *urfave.Command {
	return &urfave.Command{Name: "status", Usage: "Check local dispatcher readiness and worker pool", OnUsageError: usageError, ArgValidator: noArguments,
		Flags: []urfave.Flag{jsonFlag()}, Action: func(ctx context.Context, cmd *urfave.Command) error {
			cfg, err := loadConfig(ctx, cmd, diagnostics)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			status, checkErr := localServerStatus(ctx, cfg.StoragePath)
			if cmd.Bool("json") {
				err = json.NewEncoder(out).Encode(status)
			} else {
				_, err = fmt.Fprintln(out, status.State)
				if err == nil && status.Pool != nil {
					_, err = fmt.Fprintf(out, "workers=%d busy=%d queue_depth=%d\n", status.Pool.Capacity, status.Pool.Busy, status.Pool.QueueDepth)
				}
			}
			if err != nil {
				return err
			}
			if checkErr != nil {
				if errors.Is(checkErr, context.Canceled) || errors.Is(checkErr, context.DeadlineExceeded) {
					return checkErr
				}
				return errors.New("server status check failed; check database compatibility and dispatcher lock access")
			}
			if status.State != sqlite.ServerReady {
				return fmt.Errorf("dispatcher server is %s", status.State)
			}
			return nil
		}}
}

func localServerStatus(ctx context.Context, path string) (sqlite.ServerStatus, error) {
	unavailable := sqlite.ServerStatus{State: sqlite.ServerUnavailable}
	if err := ctx.Err(); err != nil {
		return unavailable, err
	}
	canonical, held, err := dispatchlock.ProbeServer(path)
	if err != nil || !held {
		return unavailable, err
	}
	status, err := sqlite.ReadServerStatus(ctx, canonical)
	if err != nil {
		return unavailable, err
	}
	_, held, err = dispatchlock.ProbeServer(canonical)
	if err != nil || !held {
		return unavailable, err
	}
	if err = ctx.Err(); err != nil {
		return unavailable, err
	}
	return status, nil
}
