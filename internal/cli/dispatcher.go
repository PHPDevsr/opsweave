package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/config"
	"github.com/PHPDevsr/opsweave/internal/dispatchlock"
	"github.com/PHPDevsr/opsweave/internal/engine"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/httpaction"
	"github.com/PHPDevsr/opsweave/internal/scheduler"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
	urfave "github.com/urfave/cli/v3"
)

func dispatcherOwner() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func managedRun(ctx context.Context, cfg config.Config, runtime *action.Runtime, out io.Writer, asJSON bool, name string, options engine.RunOptions, enqueue bool) (err error) {
	if runtime == nil {
		runtime, _, err = httpaction.Runtime(cfg.HTTP)
		if err != nil {
			return err
		}
	}
	var lock *dispatchlock.Lock
	if !enqueue {
		lock, err = dispatchlock.Acquire(cfg.StoragePath)
		if err != nil {
			return executionDiagnostic(err)
		}
		defer func() {
			if closeErr := lock.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}()
		cfg.StoragePath = lock.Path()
	}
	return withStore(ctx, cfg.StoragePath, func(store *sqlite.Store) (err error) {
		e := engine.Engine{Workflows: store, Executions: store, Reliable: store, Actions: runtime}
		var fence execution.Fence
		if !enqueue {
			if err = store.StandaloneReady(ctx); err != nil {
				return executionDiagnostic(err)
			}
			owner, ownerErr := dispatcherOwner()
			if ownerErr != nil {
				return executionDiagnostic(ownerErr)
			}
			fence, err = store.BeginDispatcher(ctx, owner, false)
			if err != nil {
				return executionDiagnostic(err)
			}
			defer func() {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				if closeErr := store.EndDispatcher(cleanup, fence); closeErr != nil {
					err = errors.Join(err, executionDiagnostic(closeErr))
				}
			}()
		}
		admitted, runErr := e.Enqueue(ctx, store, cfg.Execution, name, options)
		snap := admitted.Snapshot
		if runErr == nil && !enqueue {
			if execution.Terminal(snap.State) {
				runErr = managedResultError(snap.State)
			} else {
				snap, runErr = (engine.Dispatcher{Engine: e, Repository: store, Fence: fence, Capacity: cfg.Execution}).Run(ctx, snap.ID)
			}
		}
		if snap.ID != "" {
			snap.Steps = nil
			snap.Definition = nil
			if err = writeExecution(out, asJSON, snap); err != nil {
				return err
			}
		}
		return executionDiagnostic(runErr)
	})
}

func managedResultError(state execution.State) error {
	switch state {
	case execution.Succeeded:
		return nil
	case execution.Cancelled:
		return context.Canceled
	case execution.TimedOut:
		return context.DeadlineExceeded
	default:
		return execution.ErrAction
	}
}

func serverCommand(out, diagnostics io.Writer, runtime *action.Runtime) *urfave.Command {
	return &urfave.Command{Name: "server", Usage: "Run the bounded single-node dispatcher", OnUsageError: usageError, ArgValidator: noArguments, Commands: []*urfave.Command{serverStatusCommand(out, diagnostics)}, Flags: []urfave.Flag{&urfave.IntFlag{Name: "webhook-port", Usage: "Enable authenticated IPv4 loopback webhooks on this port"}, &urfave.BoolFlag{Name: "adopt-unmanaged", Usage: "Adopt old active executions after stopping every old executor"}}, Action: func(ctx context.Context, cmd *urfave.Command) (err error) {
		var secret []byte
		if cmd.IsSet("webhook-port") {
			if cmd.Int("webhook-port") < 1 || cmd.Int("webhook-port") > 65535 {
				return errors.New("webhook port must be between 1 and 65535")
			}
			secret = []byte(os.Getenv("OPSWEAVE_WEBHOOK_SECRET"))
			if len(secret) < 32 || len(secret) > 1024 {
				return errors.New("OPSWEAVE_WEBHOOK_SECRET must contain 32 to 1024 bytes")
			}
		}
		cfg, err := loadConfig(ctx, cmd, diagnostics)
		if err != nil {
			return err
		}
		effectiveRuntime := runtime
		if effectiveRuntime == nil {
			effectiveRuntime, _, err = httpaction.Runtime(cfg.HTTP)
			if err != nil {
				return err
			}
		}
		lock, err := dispatchlock.Acquire(cfg.StoragePath)
		if err != nil {
			return executionDiagnostic(err)
		}
		defer func() {
			if closeErr := lock.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}()
		return withStore(ctx, lock.Path(), func(store *sqlite.Store) (err error) {
			owner, err := dispatcherOwner()
			if err != nil {
				return executionDiagnostic(err)
			}
			fence, err := store.BeginDispatcher(ctx, owner, cmd.Bool("adopt-unmanaged"))
			if err != nil {
				return executionDiagnostic(err)
			}
			defer func() {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				if closeErr := store.EndDispatcher(cleanup, fence); closeErr != nil {
					err = errors.Join(err, executionDiagnostic(closeErr))
				}
			}()
			if err = store.RecoverDispatcher(ctx, fence); err != nil {
				return executionDiagnostic(err)
			}

			level := slog.LevelInfo
			switch cfg.LogLevel {
			case "debug":
				level = slog.LevelDebug
			case "warn":
				level = slog.LevelWarn
			case "error":
				level = slog.LevelError
			}
			logger := slog.New(slog.NewJSONHandler(&outputWriter{writer: diagnostics}, &slog.HandlerOptions{Level: level}))
			e := engine.Engine{Workflows: store, Executions: store, Reliable: store, Actions: effectiveRuntime}
			schedules := &scheduler.Service{Repository: store, Preflight: e.ValidateScheduled}
			if err = store.BeginStartup(ctx, fence); err != nil {
				return executionDiagnostic(err)
			}
			serverLock, err := lock.Serve()
			if err != nil {
				return executionDiagnostic(err)
			}
			defer func() { err = errors.Join(err, serverLock.Close()) }()
			dispatch := engine.Dispatcher{Engine: e, Repository: store, Fence: fence, Capacity: cfg.Execution, Logger: logger, Scheduler: schedules, Startup: func(ctx context.Context) (bool, error) { return store.AdvanceStartup(ctx, fence, cfg.Execution) }}
			dispatch.Stopping = func() error {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				return store.PublishServerState(cleanup, fence, sqlite.ServerStopping, cfg.Execution.Workers)
			}
			err = runServer(ctx, out, cmd.Int("webhook-port"), secret, dispatch, store)
			if err == context.Canceled && ctx.Err() != nil {
				return nil
			}
			return executionDiagnostic(err)
		})
	}}
}
