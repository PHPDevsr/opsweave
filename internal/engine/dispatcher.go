package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/workflow"
)

// Dispatcher owns one bounded pool. Its caller holds the native lock until Run returns.
type Dispatcher struct {
	Engine     Engine
	Repository execution.DispatchRepository
	Fence      execution.Fence
	Capacity   execution.Capacity
	Logger     *slog.Logger
}

func (e Engine) preflightDispatcher(ctx context.Context, name, revision string) (registry.Record, *workflow.Definition, error) {
	return e.preflightPinned(ctx, name, revision, false)
}
func (e Engine) preflightPinned(ctx context.Context, name, revision string, allowArchived bool) (registry.Record, *workflow.Definition, error) {
	r, err := e.Workflows.Get(ctx, name, revision)
	if err != nil {
		return r, nil, safeRepositoryError(err)
	}
	if r.Archived && !allowArchived {
		return r, nil, registry.ErrArchived
	}
	bindings := map[string]string{}
	if json.Unmarshal(r.Bindings, &bindings) != nil {
		return r, nil, execution.ErrInvalid
	}
	catalog, err := e.Actions.Catalog(bindings)
	if err != nil {
		return r, nil, err
	}
	candidates, err := registry.PrepareDispatcher(ctx, []registry.Source{{Data: r.Source}}, catalog)
	if err != nil {
		return r, nil, err
	}
	if candidates[0].Name != name || !bytes.Equal(candidates[0].Normalized, r.Definition) || !reflect.DeepEqual(bindings, candidates[0].Bindings) {
		return r, nil, execution.ErrInvalid
	}
	d, err := workflow.Parse(ctx, r.Source)
	return r, d, err
}

// Enqueue validates exact action bindings, then atomically admits without invoking work.
func (e Engine) Enqueue(ctx context.Context, repo execution.DispatchRepository, capacity execution.Capacity, name string, options RunOptions) (execution.AdmissionResult, error) {
	if e.Workflows == nil || e.Executions == nil || repo == nil {
		return execution.AdmissionResult{}, execution.ErrInvalid
	}
	revision := ""
	if options.Parent != "" {
		original, err := e.Executions.Execution(ctx, options.Parent)
		if err != nil {
			return execution.AdmissionResult{}, safeRepositoryError(err)
		}
		if name == "" {
			name = original.Workflow
		}
		if original.Workflow != name {
			return execution.AdmissionResult{}, execution.ErrInvalid
		}
		revision = original.Revision
	}
	r, _, err := e.preflightDispatcher(ctx, name, revision)
	if err != nil {
		return execution.AdmissionResult{}, err
	}
	result, err := repo.Enqueue(ctx, r, execution.QueueAdmission{Options: execution.AdmissionOptions{Parent: options.Parent, IdempotencyKey: options.IdempotencyKey, AcknowledgedUncertain: options.AcknowledgedUncertain}, Capacity: capacity})
	return result, safeRepositoryError(err)
}

type dispatchTask struct {
	id, step, action, version string
	attempt                   int
	ctx                       context.Context
	inputs                    map[string]any
}
type dispatchResult struct {
	task   dispatchTask
	output json.RawMessage
	err    error
}
type invocation struct {
	task   dispatchTask
	cancel context.CancelFunc
}

// Run drains eligible queue work for a server, or only the given standalone ID.
// No goroutine is allocated per queued execution, retry, or logical active step.
// An uncooperative handler retains ownership until it actually exits.
func (d Dispatcher) Run(ctx context.Context, only string) (final execution.Snapshot, resultErr error) {
	if d.Repository == nil || d.Engine.Workflows == nil || d.Engine.Executions == nil || d.Capacity.Validate() != nil || !execution.ValidID(d.Fence.Owner) || d.Fence.Generation < 1 || (only != "" && !execution.ValidID(only)) {
		return final, execution.ErrInvalid
	}
	if only != "" {
		final.ID = only
		budget, cancel := dispatchContext(ctx, 5*time.Second)
		initial, err := d.Engine.Executions.Execution(budget, only)
		cancel()
		if err != nil {
			return final, safeRepositoryError(err)
		}
		final = initial
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	jobs := make(chan dispatchTask)
	results := make(chan dispatchResult, d.Capacity.Workers)
	poolCtx, poolCancel := context.WithCancel(context.WithoutCancel(ctx))
	var workers sync.WaitGroup
	for range d.Capacity.Workers {
		workers.Go(func() {
			for {
				select {
				case <-poolCtx.Done():
					return
				case task, ok := <-jobs:
					if !ok {
						return
					}
					output, err := d.Engine.Actions.Invoke(task.ctx, task.action, task.version, task.inputs)
					results <- dispatchResult{task, output, err}
				}
			}
		})
	}
	active := map[string]invocation{}
	var shutdown context.Context
	var stop context.CancelFunc
	defer func() {
		poolCancel()
		for _, call := range active {
			call.cancel()
		}
		close(jobs)
		if len(active) == 0 {
			workers.Wait()
			return
		}
		joined := make(chan struct{})
		go func() { workers.Wait(); close(joined) }()
		remaining := d.Capacity.ShutdownTimeout
		if shutdown != nil {
			if deadline, ok := shutdown.Deadline(); ok {
				remaining = time.Until(deadline)
			}
		}
		timer := time.NewTimer(max(0, remaining))
		defer timer.Stop()
		select {
		case <-joined:
			return
		case <-timer.C:
			logger.ErrorContext(context.WithoutCancel(ctx), "dispatcher shutdown timed out; retaining ownership until invocations exit")
			resultErr = errors.Join(resultErr, context.DeadlineExceeded)
			<-joined
		}
	}()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	var operationCtx context.Context
	var operationCancel context.CancelFunc
	defer func() {
		if operationCancel != nil {
			operationCancel()
		}
	}()
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	writeCtx := func() context.Context {
		if shutdown != nil {
			return shutdown
		}
		return operationCtx
	}
	change := func(snap execution.Snapshot, m execution.ReliableMutation) (execution.Snapshot, error) {
		m.ID, m.Owner = snap.ID, d.Fence.Owner
		for range 3 {
			if m.Operation == execution.StartStep || m.Operation == execution.StartAttempt {
				cancelled := snap.CancelRequestedAt != nil || (snap.Deadline != nil && !time.Now().Before(*snap.Deadline))
				for _, step := range snap.Steps {
					if step.ID == m.Step && step.CancelRequestedAt != nil {
						cancelled = true
					}
				}
				if cancelled {
					m.Operation = execution.CancelIdleStep
				}
			}
			m.Version = snap.Version
			next, err := d.Repository.ChangeDispatch(writeCtx(), d.Fence, m)
			if err == nil {
				if next.ID == only {
					final = next
				}
				return next, nil
			}
			if !errors.Is(err, execution.ErrConflict) {
				return snap, safeRepositoryError(err)
			}
			next, err = d.Engine.Executions.Execution(writeCtx(), snap.ID)
			if err != nil {
				return snap, safeRepositoryError(err)
			}
			if next.Version == snap.Version || next.State != execution.Running {
				return next, execution.ErrConflict
			}
			snap = next
		}
		return snap, execution.ErrConflict
	}
	handleResult := func(r dispatchResult) error {
		key := r.task.id + ":" + r.task.step
		call, ok := active[key]
		if !ok {
			return execution.ErrInvalid
		}
		delete(active, key)
		defer call.cancel()
		snap, err := d.Engine.Executions.Execution(writeCtx(), r.task.id)
		if err != nil {
			return safeRepositoryError(err)
		}
		found := false
		total := len(r.output)
		for _, step := range snap.Steps {
			total += len(step.Output)
			if step.ID == r.task.step {
				found = step.Attempt == r.task.attempt && step.State == execution.Running
			}
		}
		if !found {
			return execution.ErrConflict
		}
		parentErr := error(nil)
		if shutdown != nil {
			parentErr = context.Canceled
		}
		state, reason := stepOutcome(parentErr, r.task.ctx.Err(), r.err, total > 1<<20)
		if state != execution.Succeeded {
			r.output = nil
		}
		next, err := change(snap, execution.ReliableMutation{Operation: execution.FinishAttempt, Step: r.task.step, Outcome: state, Reason: reason, Output: r.output})
		if next.ID == only {
			final = next
		}
		return err
	}
	for {
		if operationCancel != nil {
			operationCancel()
		}
		operationCtx, operationCancel = dispatchContext(ctx, 5*time.Second)
		if shutdown == nil && ctx.Err() != nil {
			shutdown, stop = dispatchContext(ctx, d.Capacity.ShutdownTimeout)
			logger.InfoContext(shutdown, "dispatcher stopping")
			if only != "" {
				own, err := d.Engine.Executions.Execution(shutdown, only)
				if err != nil {
					return final, safeRepositoryError(err)
				}
				if own.State == execution.Queued {
					if _, err = d.Repository.RequestReliableCancellation(shutdown, only, ""); err != nil {
						return final, safeRepositoryError(err)
					}
					final, err = d.Repository.ClaimExecution(shutdown, d.Fence, only, registry.Record{}, d.Capacity)
					if err != nil {
						return final, safeRepositoryError(err)
					}
				}
			}

			for _, call := range active {
				call.cancel()
			}
		}
		if shutdown != nil && shutdown.Err() != nil {
			return final, context.DeadlineExceeded
		}
		// Drain completed invocations before making any new dispatch decision.
		for drain := true; drain; {
			select {
			case r := <-results:
				if err := handleResult(r); err != nil {
					return final, err
				}
			default:
				drain = false
			}
		}
		running, err := d.Repository.DispatchExecutions(writeCtx(), d.Fence)
		if err != nil {
			return final, safeRepositoryError(err)
		}
		if shutdown != nil {
			for i, snap := range running {
				if snap.CancelRequestedAt == nil {
					next, err := d.Repository.RequestReliableCancellation(writeCtx(), snap.ID, "")
					if err != nil {
						return final, safeRepositoryError(err)
					}
					running[i] = next
				}
			}
		}
		slots := 0
		for _, snap := range running {
			for _, step := range snap.Steps {
				if step.State == execution.Running {
					slots++
				}
			}
		}
		for _, snap := range running {
			if snap.CancelRequestedAt != nil || (snap.Deadline != nil && !time.Now().Before(*snap.Deadline)) {
				inflight := false
				logical := 0
				for key, call := range active {
					if call.task.id == snap.ID {
						call.cancel()
						active[key] = call
						inflight = true
					}
				}
				if inflight {
					continue
				}
				for _, step := range snap.Steps {
					if step.State == execution.Running {
						logical++
					}
				}
				snap, err = change(snap, execution.ReliableMutation{Operation: execution.CancelIdleSteps})
				if err != nil {
					return final, err
				}
				slots -= logical
				snap, err = change(snap, execution.ReliableMutation{Operation: execution.FinishExecution})
				if err != nil {
					return final, err
				}
				if snap.ID == only {
					final = snap
				}
				continue
			}
			// Revalidate the pinned revision on recovery; never swap to the latest revision.
			_, definition, err := d.Engine.preflightPinned(writeCtx(), snap.Workflow, snap.Revision, true)
			if err != nil {
				return final, err
			}
			definitions := map[string]workflow.Step{}
			for _, step := range definition.Spec.Steps {
				definitions[step.ID] = step
			}
			roundEnd := time.Now().Add(time.Second)
			for i := range snap.Steps {
				if shutdown == nil && (ctx.Err() != nil || !time.Now().Before(roundEnd)) {
					break
				}
				step := snap.Steps[i]
				if execution.Terminal(step.State) {
					continue
				}
				key := snap.ID + ":" + step.ID
				cancelled := snap.CancelRequestedAt != nil || step.CancelRequestedAt != nil || shutdown != nil
				expired := snap.Deadline == nil || !time.Now().Before(*snap.Deadline) || (step.Deadline != nil && !time.Now().Before(*step.Deadline))
				if call, ok := active[key]; ok {
					if cancelled || expired {
						call.cancel()
					}
					continue
				}
				if cancelled || expired {
					// Pending steps and persisted backoff have no owned invocation to join.
					if step.State == execution.Running && len(step.Attempts) > 0 && step.Attempts[len(step.Attempts)-1].State == execution.Running {
						return final, execution.ErrUncertain
					}
					if !cancelled && step.State == execution.Running {
						snap, err = change(snap, execution.ReliableMutation{Step: step.ID, Operation: execution.StartAttempt})
					} else {
						snap, err = change(snap, execution.ReliableMutation{Step: step.ID, Operation: execution.CancelIdleStep})
					}
					if err != nil {
						return final, err
					}
					if step.State == execution.Running {
						slots--
					}
					continue
				}
				if step.State == execution.Pending {
					ready := true
					for _, id := range step.Policy.Needs {
						for _, dependency := range snap.Steps {
							if dependency.ID == id && !execution.Terminal(dependency.State) {
								ready = false
							}
						}
					}
					if !ready || slots >= d.Capacity.ActiveSteps {
						continue
					}
					logical := 0
					for _, v := range snap.Steps {
						if v.State == execution.Running {
							logical++
						}
					}
					if logical >= snap.Parallelism {
						continue
					}
					snap, err = change(snap, execution.ReliableMutation{Step: step.ID, Operation: execution.StartStep})
					if err != nil {
						return final, err
					}
					step = snap.Steps[i]
					if step.State != execution.Running {
						continue
					}
					slots++
				}
				if len(active) >= d.Capacity.Workers {
					continue
				}
				if step.NextAttemptAt != nil && time.Now().Before(*step.NextAttemptAt) {
					continue
				}
				if len(step.Attempts) > 0 && step.Attempts[len(step.Attempts)-1].State == execution.Running {
					return final, execution.ErrUncertain
				}
				child, childCancel := context.WithDeadline(poolCtx, *step.Deadline)
				inputs, inputErr := ResolveInputs(child, definitions[step.ID].Inputs, snap.Steps)
				if inputErr == nil {
					inputErr = d.Engine.Actions.ValidateInputs(child, step.Action, step.ActionVersion, inputs)
				}
				if inputErr != nil {
					childErr := child.Err()
					childCancel()
					if childErr != nil {
						continue
					}
					reason := "input_invalid"
					if errors.Is(inputErr, execution.ErrReference) {
						reason = "reference_unavailable"
					}
					if errors.Is(inputErr, execution.ErrPermission) {
						reason = "permission_denied"
					}
					if len(step.Attempts) > 0 {
						return final, inputErr
					}
					snap, err = change(snap, execution.ReliableMutation{Step: step.ID, Operation: execution.FailResolution, Reason: reason})
					if err != nil {
						return final, err
					}
					slots--
					continue
				}
				next, err := change(snap, execution.ReliableMutation{Step: step.ID, Operation: execution.StartAttempt})
				if err != nil {
					childCancel()
					return final, err
				}
				snap = next
				if snap.Steps[i].State != execution.Running {
					childCancel()
					slots--
					continue
				}
				task := dispatchTask{id: snap.ID, step: step.ID, action: step.Action, version: step.ActionVersion, attempt: snap.Steps[i].Attempt, ctx: child, inputs: inputs}
				active[key] = invocation{task, childCancel}
				logger.DebugContext(writeCtx(), "attempt dispatched", "execution", task.id, "step", task.step, "attempt", task.attempt)
				if ctx.Err() != nil {
					childCancel()
				}
				jobs <- task
			}
			allDone := true
			for _, step := range snap.Steps {
				allDone = allDone && execution.Terminal(step.State)
			}
			if allDone {
				snap, err = change(snap, execution.ReliableMutation{Operation: execution.FinishExecution})
				if err != nil {
					return final, err
				}
			}
			if snap.ID == only {
				final = snap
			}
		}
		if only != "" {
			snap, err := d.Engine.Executions.Execution(writeCtx(), only)
			if err != nil {
				return final, safeRepositoryError(err)
			}
			final = snap
			if execution.Terminal(snap.State) {
				return snap, resultError(snap.State)
			}
		}
		if shutdown != nil && len(running) == 0 && len(active) == 0 {
			return final, ctx.Err()
		}
		if shutdown == nil && ctx.Err() == nil && len(running) < d.Capacity.RunningExecutions {
			candidates, err := d.Repository.QueuedExecutions(writeCtx(), only)
			if err != nil {
				return final, safeRepositoryError(err)
			}
			requestDeadline := time.Time{}
			if deadline, ok := ctx.Deadline(); ok {
				requestDeadline = deadline
			}
			for _, candidate := range candidates {
				if ctx.Err() != nil {
					break
				}
				r, _, err := d.Engine.preflightDispatcher(writeCtx(), candidate.Workflow, candidate.Revision)
				if err != nil {
					// Archived/cancelled/expired work still goes through guarded queue settlement.
					if errors.Is(err, registry.ErrArchived) {
						r, err = d.Engine.Workflows.Get(writeCtx(), candidate.Workflow, candidate.Revision)
					}
					if err != nil {
						if _, writeErr := d.Repository.RejectQueued(writeCtx(), d.Fence, candidate.ID, "action_unavailable"); writeErr != nil {
							return final, safeRepositoryError(writeErr)
						}
						continue
					}
				}
				_, err = d.Repository.ClaimExecution(writeCtx(), d.Fence, candidate.ID, r, d.Capacity, requestDeadline)
				if errors.Is(err, execution.ErrBusy) || errors.Is(err, execution.ErrConflict) {
					continue
				}
				if err != nil {
					return final, safeRepositoryError(err)
				}
				// Refresh capacity before another claim; SQL independently checks every limit.
				break
			}
		}
		select {
		case r := <-results:
			if err := handleResult(r); err != nil {
				return final, err
			}
		case <-tick.C:
		case <-ctx.Done():
		}
	}
}

// dispatchContext returns a detached, bounded operation budget. Run owns and
// cancels each budget before replacing it and on every exit, including shutdown.
func dispatchContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}
