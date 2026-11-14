package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/workflow"
)

// Clock supplies cancellation-aware due-work waits. Repository timestamps remain UTC wall time.
type Clock interface {
	WaitUntil(context.Context, time.Time) error
}
type systemClock struct{}

func (systemClock) WaitUntil(ctx context.Context, at time.Time) error {
	timer := time.NewTimer(time.Until(at))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// RunOptions describes manual admission; idempotency keys are never copied into result DTOs.
type RunOptions struct {
	IdempotencyKey, Parent string
	AcknowledgedUncertain  bool
}

// Retry replays the exact original revision as a new execution, never rewriting history.
func (e Engine) Retry(ctx context.Context, id string, options RunOptions) (execution.Snapshot, error) {
	if e.Executions == nil || !execution.ValidID(id) {
		return execution.Snapshot{}, execution.ErrInvalid
	}
	original, err := e.Executions.Execution(ctx, id)
	if err != nil {
		return execution.Snapshot{}, safeRepositoryError(err)
	}
	options.Parent = id
	return e.RunReliable(ctx, original.Workflow, options)
}

// RunReliable executes only newly admitted serial work. It does not recover or steal old work.
func (e Engine) RunReliable(ctx context.Context, name string, options RunOptions) (execution.Snapshot, error) {
	var empty execution.Snapshot
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if e.Reliable == nil || e.Workflows == nil || e.Executions == nil {
		return empty, execution.ErrInvalid
	}
	revision := ""
	if options.Parent != "" {
		parent, err := e.Executions.Execution(ctx, options.Parent)
		if err != nil {
			return empty, safeRepositoryError(err)
		}
		if parent.Workflow != name {
			return empty, execution.ErrInvalid
		}
		revision = parent.Revision
	}
	record, err := e.Workflows.Get(ctx, name, revision)
	if err != nil {
		return empty, safeRepositoryError(err)
	}
	if record.Archived {
		return empty, registry.ErrArchived
	}
	bindings := map[string]string{}
	if json.Unmarshal(record.Bindings, &bindings) != nil {
		return empty, execution.ErrInvalid
	}
	catalog, err := e.Actions.Catalog(bindings)
	if err != nil {
		return empty, err
	}
	candidates, err := registry.PrepareReliable(ctx, []registry.Source{{Data: record.Source}}, catalog)
	if err != nil {
		return empty, err
	}
	if candidates[0].Name != name || !bytes.Equal(candidates[0].Normalized, record.Definition) || !reflect.DeepEqual(bindings, candidates[0].Bindings) {
		return empty, execution.ErrInvalid
	}
	definition, err := workflow.Parse(ctx, record.Source)
	if err != nil {
		return empty, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return empty, &execution.Failure{Cause: err}
	}
	owner := hex.EncodeToString(random[:])
	admission, err := e.Reliable.CreateReliable(ctx, record, execution.AdmissionOptions{Owner: owner, Parent: options.Parent, IdempotencyKey: options.IdempotencyKey, AcknowledgedUncertain: options.AcknowledgedUncertain})
	if err != nil {
		return admission.Snapshot, safeRepositoryError(err)
	}
	if !admission.Created {
		return e.waitReceipt(ctx, admission.Snapshot)
	}
	return e.coordinate(ctx, admission.Snapshot, owner, definition)
}

func (e Engine) waitReceipt(ctx context.Context, snap execution.Snapshot) (execution.Snapshot, error) {
	if !execution.Terminal(snap.State) {
		if snap.Deadline == nil {
			return snap, execution.ErrInvalid
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, *snap.Deadline)
		defer cancel()
	}
	clock := e.Clock
	if clock == nil {
		clock = systemClock{}
	}
	for !execution.Terminal(snap.State) {
		if err := clock.WaitUntil(ctx, time.Now().Add(100*time.Millisecond)); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return snap, err
			}
			return snap, &execution.Failure{Cause: err}
		}
		next, err := e.Executions.Execution(ctx, snap.ID)
		if err != nil {
			return snap, safeRepositoryError(err)
		}
		snap = next
	}
	return snap, resultError(snap.State)
}

func resultError(state execution.State) error {
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

type activeStep struct {
	sync.Mutex
	id     string
	cancel context.CancelFunc
}

func (a *activeStep) set(id string, cancel context.CancelFunc) {
	a.Lock()
	a.id, a.cancel = id, cancel
	a.Unlock()
}
func (a *activeStep) cancelRequested(control execution.Cancellation) {
	a.Lock()
	id, cancel := a.id, a.cancel
	a.Unlock()
	if control.Steps[id] && cancel != nil {
		cancel()
	}
}

func (e Engine) watch(ctx context.Context, id, owner string, cancel context.CancelCauseFunc, active *activeStep, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			control, err := e.Reliable.ReliableControl(ctx, id, owner)
			if err != nil {
				if ctx.Err() == nil {
					cancel(&execution.Failure{Cause: err})
				}
				return
			}
			if execution.Terminal(control.State) {
				return
			}
			if control.Execution {
				cancel(context.Canceled)
				return
			}
			active.cancelRequested(control)
		}
	}
}

func (e Engine) coordinate(parent context.Context, snap execution.Snapshot, owner string, definition *workflow.Definition) (execution.Snapshot, error) {
	if snap.Deadline == nil {
		return snap, execution.ErrInvalid
	}
	deadlineCtx, deadlineCancel := context.WithDeadline(parent, *snap.Deadline)
	defer deadlineCancel()
	ctx, cancel := context.WithCancelCause(deadlineCtx)
	active := &activeStep{}
	done := make(chan struct{})
	go e.watch(ctx, snap.ID, owner, cancel, active, done)
	defer func() { cancel(context.Canceled); <-done }()
	clock := e.Clock
	if clock == nil {
		clock = systemClock{}
	}
	var cleanup context.Context
	var cleanupCancel context.CancelFunc
	defer func() {
		if cleanupCancel != nil {
			cleanupCancel()
		}
	}()
	writeContext := func() context.Context {
		if ctx.Err() == nil {
			return ctx
		}
		if cleanup == nil {
			cleanup, cleanupCancel = context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		}
		return cleanup
	}
	fault := func() error {
		var failure *execution.Failure
		if errors.As(context.Cause(ctx), &failure) {
			return failure
		}
		return nil
	}
	refresh := func() error {
		next, err := e.Reliable.OwnedReliable(writeContext(), snap.ID, owner)
		if err != nil {
			return safeRepositoryError(err)
		}
		snap = next
		return nil
	}
	change := func(m execution.ReliableMutation) error {
		for range 3 {
			effective := m
			if m.Operation == execution.StartStep || m.Operation == execution.StartAttempt {
				stop := snap.CancelRequestedAt != nil || (snap.Deadline != nil && !time.Now().Before(*snap.Deadline))
				for _, step := range snap.Steps {
					if step.ID == m.Step {
						stop = stop || step.CancelRequestedAt != nil
					}
				}
				if stop {
					effective.Operation = execution.CancelIdleStep
				}
			}
			effective.ID, effective.Owner, effective.Version = snap.ID, owner, snap.Version
			next, err := e.Reliable.ChangeReliable(writeContext(), effective)
			if err != nil && ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				next, err = e.Reliable.ChangeReliable(writeContext(), effective)
			}
			if err == nil {
				snap = next
				return nil
			}
			if !errors.Is(err, execution.ErrConflict) {
				return safeRepositoryError(err)
			}
			before := snap.Version
			if err := refresh(); err != nil {
				return err
			}
			if snap.Version == before || snap.State != execution.Running {
				return execution.ErrConflict
			}
		}
		return execution.ErrConflict
	}
	requestCancellation := func() error {
		if err := fault(); err != nil {
			return err
		}
		if errors.Is(ctx.Err(), context.Canceled) && snap.CancelRequestedAt == nil {
			next, err := e.Reliable.RequestReliableCancellation(writeContext(), snap.ID, "")
			if err != nil {
				return safeRepositoryError(err)
			}
			snap = next
		}
		return nil
	}
	definitions := map[string]workflow.Step{}
	for _, step := range definition.Spec.Steps {
		definitions[step.ID] = step
	}
	for index := range snap.Steps {
		if err := requestCancellation(); err != nil {
			return snap, err
		}
		if err := refresh(); err != nil {
			return snap, err
		}
		if ctx.Err() != nil || snap.CancelRequestedAt != nil {
			if err := change(execution.ReliableMutation{Operation: execution.CancelIdleSteps}); err != nil {
				return snap, err
			}
			break
		}
		if snap.Steps[index].CancelRequestedAt != nil {
			if err := change(execution.ReliableMutation{Step: snap.Steps[index].ID, Operation: execution.CancelIdleStep}); err != nil {
				return snap, err
			}
			continue
		}
		if err := change(execution.ReliableMutation{Step: snap.Steps[index].ID, Operation: execution.StartStep}); err != nil {
			return snap, err
		}
		if snap.Steps[index].State != execution.Running {
			continue
		}
		step := snap.Steps[index]
		stepCtx, stepCancel := context.WithDeadline(ctx, *step.Deadline)
		active.set(step.ID, stepCancel)
		inputs, inputErr := ResolveInputs(stepCtx, definitions[step.ID].Inputs, snap.Steps)
		if inputErr == nil {
			inputErr = e.Actions.ValidateInputs(stepCtx, step.Action, step.ActionVersion, inputs)
		}
		if inputErr != nil && stepCtx.Err() == nil {
			active.set("", nil)
			stepCancel()
			reason := "input_invalid"
			if errors.Is(inputErr, execution.ErrReference) {
				reason = "reference_unavailable"
			}
			if errors.Is(inputErr, execution.ErrPermission) {
				reason = "permission_denied"
			}
			if err := change(execution.ReliableMutation{Step: step.ID, Operation: execution.FailResolution, Reason: reason}); err != nil {
				return snap, err
			}
			continue
		}
		for snap.Steps[index].State == execution.Running {
			if err := requestCancellation(); err != nil {
				active.set("", nil)
				stepCancel()
				return snap, err
			}
			if err := refresh(); err != nil {
				active.set("", nil)
				stepCancel()
				return snap, err
			}
			step = snap.Steps[index]
			if step.CancelRequestedAt != nil || snap.CancelRequestedAt != nil || ctx.Err() != nil {
				if err := change(execution.ReliableMutation{Step: step.ID, Operation: execution.CancelIdleStep}); err != nil {
					active.set("", nil)
					stepCancel()
					return snap, err
				}
				break
			}
			if step.NextAttemptAt != nil {
				waitErr := clock.WaitUntil(stepCtx, *step.NextAttemptAt)
				if waitErr != nil && !errors.Is(waitErr, context.Canceled) && !errors.Is(waitErr, context.DeadlineExceeded) {
					active.set("", nil)
					stepCancel()
					return snap, &execution.Failure{Cause: waitErr}
				}
				if err := fault(); err != nil {
					active.set("", nil)
					stepCancel()
					return snap, err
				}
				if err := refresh(); err != nil {
					active.set("", nil)
					stepCancel()
					return snap, err
				}
				step = snap.Steps[index]
			}
			if stepCtx.Err() != nil && errors.Is(stepCtx.Err(), context.Canceled) {
				continue
			}
			if err := change(execution.ReliableMutation{Step: step.ID, Operation: execution.StartAttempt}); err != nil {
				active.set("", nil)
				stepCancel()
				return snap, err
			}
			if snap.Steps[index].State != execution.Running {
				break
			}
			output, actionErr := e.Actions.Invoke(stepCtx, step.Action, step.ActionVersion, inputs)
			if err := fault(); err != nil {
				active.set("", nil)
				stepCancel()
				return snap, err
			}
			if err := requestCancellation(); err != nil {
				active.set("", nil)
				stepCancel()
				return snap, err
			}
			total := len(output)
			for _, previous := range snap.Steps {
				total += len(previous.Output)
			}
			state, reason := stepOutcome(ctx.Err(), stepCtx.Err(), actionErr, total > 1<<20)
			if state != execution.Succeeded {
				output = nil
			}
			if err := change(execution.ReliableMutation{Step: step.ID, Operation: execution.FinishAttempt, Outcome: state, Reason: reason, Output: output}); err != nil {
				active.set("", nil)
				stepCancel()
				return snap, err
			}
		}
		active.set("", nil)
		stepCancel()
	}
	if err := requestCancellation(); err != nil {
		return snap, err
	}
	if err := change(execution.ReliableMutation{Operation: execution.FinishExecution}); err != nil {
		return snap, err
	}
	return snap, resultError(snap.State)
}
