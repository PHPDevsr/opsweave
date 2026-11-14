package execution

import (
	"context"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"time"
)

// Capacity bounds all dispatcher-owned work. Zero values are invalid.
type Capacity struct {
	Workers, ActiveSteps, RunningExecutions, QueuedExecutions int
	QueueTimeout, ShutdownTimeout                             time.Duration
}

// DefaultCapacity supplies conservative single-node defaults.
func DefaultCapacity() Capacity { return Capacity{4, 16, 8, 1024, time.Hour, 30 * time.Second} }

// Validate rejects unsupported capacities rather than silently clamping them.
func (c Capacity) Validate() error {
	if c.Workers < 1 || c.Workers > 64 || c.ActiveSteps < 1 || c.ActiveSteps > 256 || c.RunningExecutions < 1 || c.RunningExecutions > 256 || c.QueuedExecutions < 1 || c.QueuedExecutions > 10000 || c.QueueTimeout < time.Second || c.QueueTimeout > 24*time.Hour || c.ShutdownTimeout < time.Second || c.ShutdownTimeout > 5*time.Minute {
		return ErrInvalid
	}
	return nil
}

// Fence is private dispatcher ownership metadata, never a public receipt.
type Fence struct {
	Generation int64
	Owner      string
}

// QueueAdmission specifies bounded durable admission without dispatcher ownership.
type QueueAdmission struct {
	Options  AdmissionOptions
	Capacity Capacity
}

// DispatchRepository isolates durable queue/ownership from the worker implementation.
// BeginDispatcher may only be called while the native database lock is held.
type DispatchRepository interface {
	ReliableRepository
	Enqueue(context.Context, registry.Record, QueueAdmission) (AdmissionResult, error)
	BeginDispatcher(context.Context, string, bool) (Fence, error)
	EndDispatcher(context.Context, Fence) error
	RecoverDispatcher(context.Context, Fence) error
	QueuedExecutions(context.Context, string) ([]Snapshot, error)
	ClaimExecution(context.Context, Fence, string, registry.Record, Capacity, ...time.Time) (Snapshot, error)
	RejectQueued(context.Context, Fence, string, string) (Snapshot, error)
	DispatchExecutions(context.Context, Fence) ([]Snapshot, error)
	ChangeDispatch(context.Context, Fence, ReliableMutation) (Snapshot, error)
}

// RecoverStopped applies conservative recovery only after an old owner is fenced.
// An unfinished invocation always fails with visible uncertainty and is never replayed.
func RecoverStopped(s Snapshot, now time.Time) (Snapshot, []Event, error) {
	classification := ClassifyRecovery(s, now)
	if classification == "invalid" || classification == "terminal" {
		return s, nil, ErrInvalid
	}
	if classification == "resumable" {
		return s, nil, nil
	}
	s.UncertainEffects = classification == "uncertain_effect"
	s.Steps = append([]Step(nil), s.Steps...)
	events := []Event{}
	for i, step := range s.Steps {
		if Terminal(step.State) {
			continue
		}
		state, reason := Cancelled, "recovery_aborted"
		if classification == "uncertain_effect" && step.State == Running {
			if s.Format != 2 || (len(step.Attempts) > 0 && step.Attempts[len(step.Attempts)-1].State == Running) {
				state, reason = Failed, "uncertain_effect"
			}
		} else if classification == "expired" && step.State == Running {
			state, reason = TimedOut, "deadline"
		}
		step.State, step.Reason, step.FinishedAt = state, reason, stamp(now)
		step.NextAttemptAt = nil
		step.Attempts = append([]Attempt(nil), step.Attempts...)
		if n := len(step.Attempts); n > 0 && step.Attempts[n-1].State == Running {
			a := &step.Attempts[n-1]
			a.State, a.Reason, a.FinishedAt = state, reason, stamp(now)
			events = append(events, Event{Time: now, Execution: s.ID, Step: step.ID, Code: "attempt_finished", State: state, Reason: reason, Attempt: a.Number})
		}
		s.Steps[i] = step
		events = append(events, Event{Time: now, Execution: s.ID, Step: step.ID, Code: "step_state", State: state, Reason: reason})
	}
	s.State = Failed
	if classification == "expired" {
		s.State = TimedOut
	}
	s.Version++
	s.UpdatedAt = now
	events = append(events, Event{Time: now, Execution: s.ID, Code: "execution_state", State: s.State, Reason: classification})
	return s, events, nil
}
