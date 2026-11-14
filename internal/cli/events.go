package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/PHPDevsr/opsweave/internal/engine"
	"github.com/PHPDevsr/opsweave/internal/event"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
	"github.com/PHPDevsr/opsweave/internal/webhook"
	urfave "github.com/urfave/cli/v3"
)

func eventCommand(out, diagnostics io.Writer) *urfave.Command {
	return &urfave.Command{Name: "event", Usage: "Manage local event ingress", Commands: []*urfave.Command{{
		Name: "emit", Usage: "Queue an internal event", OnUsageError: usageError, ArgValidator: exactArguments(1),
		Flags: []urfave.Flag{&urfave.StringFlag{Name: "id", Required: true, Usage: "Stable event ID"}, jsonFlag()},
		Action: func(ctx context.Context, cmd *urfave.Command) error {
			name, id := cmd.Args().First(), cmd.String("id")
			if !event.ValidName(name) || !event.ValidID(id) {
				return execution.ErrInvalid
			}
			cfg, err := loadConfig(ctx, cmd, diagnostics)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return withStore(ctx, cfg.StoragePath, func(s *sqlite.Store) error {
				receipts, err := s.EmitEvent(ctx, event.Request{Type: "internal", Name: name, ID: id}, cfg.Execution, nil)
				if err != nil {
					for _, safe := range []error{event.ErrRoute, event.ErrConflict, event.ErrDestinations} {
						if errors.Is(err, safe) {
							return safe
						}
					}
					return executionDiagnostic(err)
				}
				if cmd.Bool("json") {
					return json.NewEncoder(out).Encode(receipts)
				}
				for _, r := range receipts {
					if _, err = fmt.Fprintf(out, "Accepted %s %s %s\n", r.Execution, r.Workflow, r.Trigger); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}, {
		Name: "history", Usage: "Read recent trigger admission history", OnUsageError: usageError, ArgValidator: exactArguments(1),
		Flags: []urfave.Flag{&urfave.IntFlag{Name: "limit", Value: 100, Usage: "Maximum entries (1–1000)"}, jsonFlag()},
		Action: func(ctx context.Context, cmd *urfave.Command) error {
			cfg, err := loadConfig(ctx, cmd, diagnostics)
			if err != nil {
				return err
			}
			return withStore(ctx, cfg.StoragePath, func(s *sqlite.Store) error {
				rows, err := s.EventHistory(ctx, cmd.Args().First(), cmd.Int("limit"))
				if err != nil {
					return executionDiagnostic(err)
				}
				if cmd.Bool("json") {
					return json.NewEncoder(out).Encode(rows)
				}
				for _, r := range rows {
					if _, err = fmt.Fprintf(out, "%s %s %s %s %s\n", r.OccurredAt.Format(time.RFC3339), r.Type, r.Trigger, r.Code, r.Execution); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}}}
}

func runServer(ctx context.Context, out io.Writer, port int, secret []byte, dispatch engine.Dispatcher, store *sqlite.Store) error {
	if len(secret) == 0 {
		if _, err := io.WriteString(out, "dispatcher ready\n"); err != nil {
			return err
		}
		_, err := dispatch.Run(ctx, "")
		return err
	}
	handler, err := webhook.New(secret, func(ctx context.Context, input event.Request) ([]event.Receipt, error) {
		return store.EmitEvent(ctx, input, dispatch.Capacity, &dispatch.Fence)
	}, nil)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("bind webhook loopback listener: %w", err)
	}
	return runWebhookServer(ctx, out, listener, handler, dispatch.Run)
}

func runWebhookServer(ctx context.Context, out io.Writer, listener net.Listener, handler http.Handler, run func(context.Context, string) (execution.Snapshot, error)) error {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	var handlerMu sync.Mutex
	var handlers sync.WaitGroup
	closing := false
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerMu.Lock()
			if closing {
				handlerMu.Unlock()
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			handlers.Add(1)
			handlerMu.Unlock()
			defer handlers.Done()
			handler.ServeHTTP(w, r)
		}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: webhook.HeaderLimit,
		BaseContext: func(net.Listener) context.Context { return child },
	}
	if _, err := io.WriteString(out, "dispatcher ready\n"); err != nil {
		return errors.Join(err, listener.Close())
	}
	served := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		served <- err
		if err != nil {
			cancel()
		}
	}()
	finished := make(chan error, 1)
	go func() { _, err := run(child, ""); finished <- err }()
	var workerErr error
	workerDone := false
	select {
	case workerErr = <-finished:
		workerDone = true
	case <-child.Done():
	}
	handlerMu.Lock()
	closing = true
	handlerMu.Unlock()
	cancel()
	shutdown, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
	defer cancelShutdown()
	shutdownErr := server.Shutdown(shutdown)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, server.Close())
	}
	serveErr := <-served
	handlers.Wait()
	if !workerDone {
		workerErr = <-finished
	}
	if errors.Is(workerErr, context.Canceled) && child.Err() != nil {
		workerErr = nil
	}
	return errors.Join(workerErr, serveErr, shutdownErr)
}
