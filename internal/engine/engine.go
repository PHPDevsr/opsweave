// Package engine dispatches synchronous manual executions through durable repository boundaries.
package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/workflow"
)

// Allow the durable cancellation writes to finish under the same default
// shutdown budget as dispatcher executions, independent of the action deadline.
const cancellationPersistenceTimeout = 30 * time.Second

// Engine composes a trusted action runtime with workflow and execution repositories.
// Run owns no background goroutines; callers own the lifetime of injected resources.
type Engine struct {
	Workflows  registry.Repository
	Executions execution.Repository
	Actions    *action.Runtime
	Reliable   execution.ReliableRepository
	Clock      Clock
}

// Run pins the current revision, commits ownership, and waits for a durable terminal result.
func (e Engine) Run(ctx context.Context, name string) (execution.Snapshot, error) {
	if e.Reliable != nil {
		return e.RunReliable(ctx, name, RunOptions{})
	}
	var snap execution.Snapshot
	if err := ctx.Err(); err != nil {
		return snap, err
	}
	if e.Workflows == nil || e.Executions == nil {
		return snap, execution.ErrInvalid
	}
	record, err := e.Workflows.Get(ctx, name, "")
	if err != nil {
		return snap, safeRepositoryError(err)
	}
	if record.Archived {
		return snap, registry.ErrArchived
	}
	bindings := map[string]string{}
	if err := json.Unmarshal(record.Bindings, &bindings); err != nil {
		return snap, execution.ErrInvalid
	}
	catalog, err := e.Actions.Catalog(bindings)
	if err != nil {
		return snap, err
	}
	candidates, err := registry.Prepare(ctx, []registry.Source{{Data: record.Source}}, catalog)
	if err != nil {
		return snap, err
	}
	if candidates[0].Name != record.Name || !bytes.Equal(candidates[0].Normalized, record.Definition) || !reflect.DeepEqual(bindings, candidates[0].Bindings) {
		return snap, execution.ErrInvalid
	}
	definition, err := workflow.Parse(ctx, record.Source)
	if err != nil {
		return snap, err
	}
	var ownerBytes [16]byte
	if _, err := rand.Read(ownerBytes[:]); err != nil {
		return snap, &execution.Failure{Cause: err}
	}
	owner := hex.EncodeToString(ownerBytes[:])
	execCtx, cancel := context.WithTimeout(ctx, definition.Spec.Timeout)
	defer cancel()
	snap, err = e.Executions.Submit(execCtx, record, owner)
	if err != nil {
		return snap, safeRepositoryError(err)
	}
	var cleanupCtx context.Context
	var cleanupCancel context.CancelFunc
	defer func() {
		if cleanupCancel != nil {
			cleanupCancel()
		}
	}()
	change := func(step string, to execution.State, reason string, output json.RawMessage) error {
		writeCtx := execCtx
		if execCtx.Err() != nil {
			if cleanupCtx == nil {
				cleanupCtx, cleanupCancel = context.WithTimeout(context.WithoutCancel(ctx), cancellationPersistenceTimeout)
			}
			writeCtx = cleanupCtx
		}
		next, err := e.Executions.Change(writeCtx, execution.Mutation{ID: snap.ID, Owner: owner, Version: snap.Version, Step: step, To: to, Reason: reason, Output: output})
		if err != nil && execCtx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) && cleanupCtx == nil {
			cleanupCtx, cleanupCancel = context.WithTimeout(context.WithoutCancel(ctx), cancellationPersistenceTimeout)
			next, err = e.Executions.Change(cleanupCtx, execution.Mutation{ID: snap.ID, Owner: owner, Version: snap.Version, Step: step, To: to, Reason: reason, Output: output})
		}
		if err != nil {
			if cleanupCtx != nil {
				return &execution.Failure{Cause: err}
			}
			return safeRepositoryError(err)
		}
		snap = next
		return nil
	}
	failed := false
	total := 0
	for _, step := range definition.Spec.Steps {
		if execCtx.Err() != nil {
			if err := change(step.ID, execution.Cancelled, "cancelled", nil); err != nil {
				return snap, err
			}
			continue
		}
		if failed {
			if err := change(step.ID, execution.Skipped, "dependency_failed", nil); err != nil {
				return snap, err
			}
			continue
		}
		if err := change(step.ID, execution.Running, "", nil); err != nil {
			return snap, err
		}
		stepCtx, stepCancel := context.WithTimeout(execCtx, step.Timeout)
		output, actionErr := e.Actions.Invoke(stepCtx, step.Uses, bindings[step.Uses], step.Inputs)
		stepErr := stepCtx.Err()
		stepCancel()
		state, reason := stepOutcome(execCtx.Err(), stepErr, actionErr, total+len(output) > 1<<20)
		if state != execution.Succeeded {
			output = nil
			failed = true
		} else {
			total += len(output)
		}
		if err := change(step.ID, state, reason, output); err != nil {
			return snap, err
		}
	}
	final := execution.Succeeded
	outcome := error(nil)
	if failed {
		final = execution.Failed
		outcome = execution.ErrAction
	}
	if execCtx.Err() != nil {
		final = execution.Cancelled
		outcome = execCtx.Err()
		if errors.Is(outcome, context.DeadlineExceeded) {
			final = execution.TimedOut
		}
	}
	if err := change("", final, "", nil); err != nil {
		return snap, err
	}
	return snap, outcome
}

func stepOutcome(execErr, stepErr, actionErr error, oversized bool) (execution.State, string) {
	if execErr != nil {
		if errors.Is(execErr, context.DeadlineExceeded) {
			return execution.TimedOut, "deadline"
		}
		return execution.Cancelled, "cancelled"
	}
	if errors.Is(stepErr, context.DeadlineExceeded) {
		return execution.TimedOut, "deadline"
	}
	if actionErr != nil {
		if errors.Is(actionErr, execution.ErrOutput) || errors.Is(actionErr, execution.ErrInvalid) {
			return execution.Failed, "invalid_output"
		}
		if errors.Is(actionErr, execution.ErrPermission) {
			return execution.Failed, "permission_denied"
		}
		return execution.Failed, "action_failed"
	}
	if oversized {
		return execution.Failed, "invalid_output"
	}
	return execution.Succeeded, ""
}

func safeRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	for _, safe := range []error{execution.ErrBusy, execution.ErrQueueFull, execution.ErrUnmanaged, execution.ErrConflict, execution.ErrNotFound, execution.ErrLimit, execution.ErrInvalid, execution.ErrReference, execution.ErrUncertain, registry.ErrNotFound, registry.ErrArchived, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, safe) {
			return safe
		}
	}
	return &execution.Failure{Cause: err}
}
