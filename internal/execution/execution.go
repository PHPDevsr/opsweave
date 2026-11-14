// Package execution defines durable manual execution independently of SQL and CLI.
package execution

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/PHPDevsr/opsweave/internal/registry"
)

// State is an execution or step outcome; Queued is execution-only and Skipped is step-only.
type State string

const (
	Pending   State = "pending"
	Queued    State = "queued"
	Running   State = "running"
	Succeeded State = "succeeded"
	Failed    State = "failed"
	Cancelled State = "cancelled"
	TimedOut  State = "timed_out"
	Skipped   State = "skipped"
)

// CanTransition describes format-1 transitions. Format 2 uses guarded policy operations.
// Both reject unknown/self transitions and changes to terminal states.
func CanTransition(from, to State, step bool) bool {
	switch from {
	case Pending:
		if step {
			return to == Running || to == Skipped || to == Cancelled
		}
		return to == Queued || to == Cancelled
	case Queued:
		return !step && (to == Running || to == Cancelled)
	case Running:
		return to == Succeeded || to == Failed || to == Cancelled || to == TimedOut
	}
	return false
}

// Terminal reports whether a state can no longer change.
func Terminal(s State) bool {
	return s == Succeeded || s == Failed || s == Cancelled || s == TimedOut || s == Skipped
}

// Safe error categories distinguish durable storage failures from action outcomes.
var (
	ErrBusy        = errors.New("dispatcher is busy")
	ErrQueueFull   = errors.New("execution queue is full")
	ErrUnmanaged   = errors.New("unmanaged active executions require explicit adoption after stopping old executors")
	ErrInvalid     = errors.New("invalid execution request or transition")
	ErrConflict    = errors.New("execution ownership, revision or active-run conflict")
	ErrNotFound    = errors.New("execution not found")
	ErrLimit       = errors.New("execution response exceeds limit")
	ErrAction      = errors.New("action failed; inspect execution history")
	ErrPermission  = errors.New("action permission denied")
	ErrUnavailable = errors.New("action version unavailable")
	ErrOutput      = errors.New("action output invalid or exceeds limit")
	ErrReference   = errors.New("execution input reference unavailable")
	ErrUncertain   = errors.New("execution has uncertain effects; explicit acknowledgement required")
)

// Failure hides infrastructure details while retaining the cause for internal inspection.
type Failure struct{ Cause error }

func (e *Failure) Error() string {
	return "execution persistence failed; inspect history before retrying"
}
func (e *Failure) Unwrap() error { return e.Cause }

// ValidID accepts opaque random identifiers without permitting arbitrary diagnostic text.
func ValidID(id string) bool { return len(id) == 32 && idPattern.MatchString(id) }

var idPattern = regexp.MustCompile(`^[0-9a-f]+$`)

// Step is a redacted durable step snapshot.
type Step struct {
	ID                string          `json:"id"`
	Action            string          `json:"action"`
	ActionVersion     string          `json:"actionVersion"`
	State             State           `json:"state"`
	Attempt           int             `json:"attempt"`
	Reason            string          `json:"reason,omitempty"`
	Output            json.RawMessage `json:"output,omitempty,omitzero"`
	StartedAt         *time.Time      `json:"startedAt,omitempty"`
	FinishedAt        *time.Time      `json:"finishedAt,omitempty"`
	Policy            *StepPolicy     `json:"policy,omitempty"`
	Deadline          *time.Time      `json:"deadline,omitempty"`
	NextAttemptAt     *time.Time      `json:"nextAttemptAt,omitempty"`
	CancelRequestedAt *time.Time      `json:"cancelRequestedAt,omitempty"`
	Attempts          []Attempt       `json:"attempts,omitempty"`
}

// TriggerOrigin identifies an automatic admission, independently of action outputs.
type TriggerOrigin struct {
	Type        string    `json:"type"`
	ID          string    `json:"id"`
	ScheduledAt time.Time `json:"scheduledAt,omitzero"`
}

// Snapshot binds history to its original immutable workflow revision.
type Snapshot struct {
	Trigger               *TriggerOrigin  `json:"trigger,omitempty"`
	ID                    string          `json:"id"`
	Workflow              string          `json:"workflow"`
	Revision              string          `json:"revision"`
	State                 State           `json:"state"`
	Version               int64           `json:"version"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
	Steps                 []Step          `json:"steps,omitempty"`
	Definition            json.RawMessage `json:"definition,omitempty,omitzero"`
	UncertainEffects      bool            `json:"uncertainEffects,omitempty"`
	Managed               bool            `json:"managed,omitempty"`
	Parallelism           int             `json:"parallelism,omitempty"`
	Format                int             `json:"format,omitempty,omitzero"`
	Deadline              *time.Time      `json:"deadline,omitempty"`
	Parent                string          `json:"parent,omitempty"`
	AcknowledgedUncertain bool            `json:"acknowledgedUncertain,omitempty,omitzero"`
	CancelRequestedAt     *time.Time      `json:"cancelRequestedAt,omitempty"`
}

// Event contains engine-owned vocabulary only, never action-supplied text.
type Event struct {
	Sequence      int        `json:"sequence"`
	Time          time.Time  `json:"time"`
	Execution     string     `json:"execution"`
	Step          string     `json:"step,omitempty"`
	Code          string     `json:"code"`
	State         State      `json:"state"`
	Reason        string     `json:"reason,omitempty"`
	Attempt       int        `json:"attempt,omitempty,omitzero"`
	NextAttemptAt *time.Time `json:"nextAttemptAt,omitempty"`
}

// Mutation applies one validated step or execution transition with compare-and-swap ownership.
type Mutation struct {
	ID, Owner string
	Version   int64
	Step      string
	To        State
	Reason    string
	Output    json.RawMessage
}

// Repository commits state and lifecycle events atomically before acknowledging writes.
type Repository interface {
	Submit(context.Context, registry.Record, string) (Snapshot, error)
	Change(context.Context, Mutation) (Snapshot, error)
	Execution(context.Context, string) (Snapshot, error)
	Executions(context.Context) ([]Snapshot, error)
	Events(context.Context, string) ([]Event, error)
}

// Advance validates aggregate invariants and updates an independently owned snapshot.
// The adapter supplies a UTC time and persists the result atomically with its event.
func Advance(s *Snapshot, m Mutation, now time.Time) error {
	if s.Format != 0 && s.Format != 1 {
		return ErrInvalid
	}
	if s.State != Running || s.Version != m.Version {
		return ErrConflict
	}
	if m.Step == "" {
		if !CanTransition(s.State, m.To, false) || m.Reason != "" || len(m.Output) > 0 {
			return ErrInvalid
		}
		failed := false
		for _, step := range s.Steps {
			if !Terminal(step.State) {
				return ErrInvalid
			}
			failed = failed || step.State == Failed || step.State == TimedOut
			if m.To == Succeeded && step.State != Succeeded {
				return ErrInvalid
			}
		}
		if m.To == Failed && !failed {
			return ErrInvalid
		}
		s.State = m.To
	} else {
		index := -1
		total := 0
		for i, step := range s.Steps {
			if step.ID == m.Step {
				index = i
			}
			total += len(step.Output)
		}
		if index < 0 {
			return ErrInvalid
		}
		step := s.Steps[index]
		if !CanTransition(step.State, m.To, true) {
			return ErrInvalid
		}
		if m.To == Running {
			if index > 0 && s.Steps[index-1].State != Succeeded {
				return ErrInvalid
			}
			for _, other := range s.Steps {
				if other.State == Running {
					return ErrInvalid
				}
			}
		}
		validReason := m.Reason == ""
		switch m.To {
		case Failed:
			validReason = m.Reason == "action_failed" || m.Reason == "invalid_output" || m.Reason == "permission_denied"
		case Skipped:
			validReason = m.Reason == "dependency_failed" && index > 0 && Terminal(s.Steps[index-1].State) && s.Steps[index-1].State != Succeeded
		case Cancelled:
			validReason = m.Reason == "cancelled"
		case TimedOut:
			validReason = m.Reason == "deadline"
		}
		if !validReason || len(m.Output) > 65536 || total+len(m.Output) > 1<<20 {
			return ErrInvalid
		}
		if len(m.Output) > 0 && (m.To != Succeeded || !json.Valid(m.Output) || m.Output[0] != '{') {
			return ErrInvalid
		}
		step.State, step.Reason = m.To, m.Reason
		step.Output = append(json.RawMessage(nil), m.Output...)
		if m.To == Running {
			step.Attempt = 1
			t := now
			step.StartedAt = &t
		}
		if Terminal(m.To) {
			t := now
			step.FinishedAt = &t
		}
		s.Steps[index] = step
	}
	s.Version++
	s.UpdatedAt = now
	return nil
}
