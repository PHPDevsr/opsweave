package execution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/workflow"
)

// StepPolicy is immutable execution policy copied from the pinned workflow revision.
type StepPolicy struct {
	Needs     []string       `json:"needs"`
	Condition string         `json:"condition,omitempty"`
	Timeout   time.Duration  `json:"timeout"`
	Retry     workflow.Retry `json:"retry"`
}

// Attempt records one invocation; terminal attempts cannot be changed or reused.
type Attempt struct {
	Number     int        `json:"number"`
	State      State      `json:"state"`
	Reason     string     `json:"reason,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// Operation identifies a guarded format-2 state change, not an arbitrary state assignment.
type Operation string

const (
	StartStep       Operation = "start_step"
	StartAttempt    Operation = "start_attempt"
	FinishAttempt   Operation = "finish_attempt"
	FailResolution  Operation = "fail_resolution"
	FinishExecution Operation = "finish_execution"
	CancelIdleStep  Operation = "cancel_idle_step"
	CancelIdleSteps Operation = "cancel_idle_steps"
)

// ReliableMutation is applied only by an owner holding the expected execution version.
type ReliableMutation struct {
	ID, Owner string
	Version   int64
	Step      string
	Operation Operation
	Outcome   State
	Reason    string
	Output    json.RawMessage
}

// Admission contains only hashed idempotency metadata. Plaintext keys never reach storage.
type Admission struct{ Digest, Fingerprint string }

// AdmissionOptions is a trusted admission request; Owner must never enter public DTOs.
type AdmissionOptions struct {
	Owner, Parent, IdempotencyKey string
	AcknowledgedUncertain         bool
}

// AdmissionResult distinguishes a new owned execution from a deduplicated receipt.
// Invocation additionally requires a nil error and matching execution ownership.
type AdmissionResult struct {
	Snapshot Snapshot
	Created  bool
}

// ReliableRepository stores policy changes without coupling the coordinator to SQL.
type ReliableRepository interface {
	CreateReliable(context.Context, registry.Record, AdmissionOptions) (AdmissionResult, error)
	ChangeReliable(context.Context, ReliableMutation) (Snapshot, error)
	RequestReliableCancellation(context.Context, string, string) (Snapshot, error)
	OwnedReliable(context.Context, string, string) (Snapshot, error)
	ReliableControl(context.Context, string, string) (Cancellation, error)
}

// Cancellation is bounded control metadata, without output payloads or ownership tokens.
type Cancellation struct {
	State     State
	Execution bool
	Steps     map[string]bool
}

// AdmissionFor validates and fingerprints a seven-day execution-admission key.
func AdmissionFor(key string, record registry.Record, parent string, ack bool) (Admission, error) {
	if key == "" {
		return Admission{}, nil
	}
	if len(key) > 128 {
		return Admission{}, ErrInvalid
	}
	for _, b := range []byte(key) {
		if b < 33 || b > 126 {
			return Admission{}, ErrInvalid
		}
	}
	digest := sha256.Sum256([]byte(key))
	data, err := json.Marshal(struct {
		Revision     string
		Bindings     json.RawMessage
		Parent       string
		Acknowledged bool
	}{record.Revision, record.Bindings, parent, ack})
	if err != nil {
		return Admission{}, ErrInvalid
	}
	fingerprint := sha256.Sum256(data)
	return Admission{hex.EncodeToString(digest[:]), hex.EncodeToString(fingerprint[:])}, nil
}

// NewReliable builds a format-2 snapshot without invoking actions. Execution ownership
// and action catalog validation are prerequisites for executing the resulting work.
func NewReliable(ctx context.Context, id string, record registry.Record, now time.Time) (Snapshot, error) {
	return newReliable(ctx, id, record, now, false)
}

// NewDispatcher creates bounded parallel policy without admitting or claiming work.
func NewDispatcher(ctx context.Context, id string, record registry.Record, now time.Time) (Snapshot, error) {
	return newReliable(ctx, id, record, now, true)
}

func newReliable(ctx context.Context, id string, record registry.Record, now time.Time, managed bool) (Snapshot, error) {
	now = now.UTC()
	if !ValidID(id) || !ValidID(record.Revision) {
		return Snapshot{}, ErrInvalid
	}
	definition, err := workflow.Parse(ctx, record.Source)
	if err != nil {
		return Snapshot{}, err
	}
	if definition.Metadata.Name != record.Name || (!managed && (definition.Spec.Concurrency != 1 || definition.Spec.Parallelism != 1)) {
		return Snapshot{}, ErrInvalid
	}
	normalized, err := registry.Normalize(definition)
	if err != nil || !bytes.Equal(normalized, record.Definition) {
		return Snapshot{}, ErrInvalid
	}
	bindings := map[string]string{}
	if json.Unmarshal(record.Bindings, &bindings) != nil {
		return Snapshot{}, ErrInvalid
	}
	deadline := now.Add(definition.Spec.Timeout)
	if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
		deadline = parent
	}
	if !deadline.After(now) {
		return Snapshot{}, context.DeadlineExceeded
	}
	result := Snapshot{ID: id, Workflow: record.Name, Revision: record.Revision, State: Running, Format: 2, CreatedAt: now, UpdatedAt: now, Deadline: &deadline, Definition: append(json.RawMessage(nil), record.Definition...), Steps: []Step{}}
	result.Managed = managed
	if managed {
		result.Parallelism = definition.Spec.Parallelism
	}
	steps := map[string]workflow.Step{}
	for _, step := range definition.Spec.Steps {
		steps[step.ID] = step
	}
	for _, id := range definition.Order {
		step := steps[id]
		if bindings[step.Uses] == "" || containsSecret(step.Inputs) {
			return Snapshot{}, ErrInvalid
		}
		policy := &StepPolicy{Needs: append([]string{}, step.Needs...), Condition: step.Condition, Timeout: step.Timeout, Retry: step.Retry}
		result.Steps = append(result.Steps, Step{ID: step.ID, Action: step.Uses, ActionVersion: bindings[step.Uses], State: Pending, Policy: policy})
	}
	return result, nil
}

func containsSecret(value any) bool {
	switch v := value.(type) {
	case workflow.Reference:
		return v.Kind != "output"
	case map[string]any:
		for _, child := range v {
			if containsSecret(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsSecret(child) {
				return true
			}
		}
	}
	return false
}

// RetryDelay implements capped exponential backoff without overflowing durations.
func RetryDelay(policy workflow.Retry, finished int) (time.Duration, error) {
	if policy.Attempts < 1 || policy.Attempts > 10 || finished < 1 || finished >= policy.Attempts || policy.Backoff <= 0 || policy.MaxBackoff < policy.Backoff {
		return 0, ErrInvalid
	}
	delay := policy.Backoff
	for i := 1; i < finished; i++ {
		if delay >= policy.MaxBackoff-delay {
			return policy.MaxBackoff, nil
		}
		delay *= 2
	}
	if delay > policy.MaxBackoff {
		delay = policy.MaxBackoff
	}
	return delay, nil
}

func stamp(now time.Time) *time.Time { t := now; return &t }

// ApplyReliable validates a policy operation atomically in memory. Errors leave the
// caller's snapshot unchanged. The repository persists its events and snapshot together.
func ApplyReliable(s *Snapshot, m ReliableMutation, now time.Time) ([]Event, error) {
	now = now.UTC()
	if s.Format != 2 {
		return nil, ErrInvalid
	}
	if s.State != Running || s.Version != m.Version {
		return nil, ErrConflict
	}
	if s.Deadline == nil {
		return nil, ErrInvalid
	}
	next := *s
	next.Steps = append([]Step(nil), s.Steps...)
	events := []Event{}
	emit := func(step string, code string, state State, reason string, attempt int) {
		events = append(events, Event{Time: now, Execution: s.ID, Step: step, Code: code, State: state, Reason: reason, Attempt: attempt})
	}
	if m.Operation == CancelIdleSteps {
		if m.Step != "" || m.Outcome != "" || m.Reason != "" || len(m.Output) > 0 || (next.CancelRequestedAt == nil && now.Before(*next.Deadline)) {
			return nil, ErrInvalid
		}
		for _, step := range next.Steps {
			if !Terminal(step.State) && len(step.Attempts) > 0 && step.Attempts[len(step.Attempts)-1].State == Running {
				return nil, ErrInvalid
			}
		}
		for i, step := range next.Steps {
			if !Terminal(step.State) {
				step.State, step.Reason, step.FinishedAt = Cancelled, "cancelled", stamp(now)
				step.NextAttemptAt = nil
				next.Steps[i] = step
				emit(step.ID, "step_state", Cancelled, "cancelled", 0)
			}
		}
		if len(events) == 0 {
			return nil, nil
		}
	} else if m.Operation == FinishExecution {
		if m.Step != "" || len(m.Output) > 0 || m.Reason != "" || m.Outcome != "" {
			return nil, ErrInvalid
		}
		state, err := Aggregate(next, now)
		if err != nil {
			return nil, err
		}
		next.State = state
		emit("", "execution_state", state, "", 0)
	} else {
		index := -1
		for i, step := range next.Steps {
			if step.ID == m.Step {
				index = i
			}
		}
		if index < 0 {
			return nil, ErrInvalid
		}
		step := next.Steps[index]
		if step.Policy == nil || step.Policy.Timeout <= 0 || step.Policy.Timeout > 24*time.Hour || step.Policy.Retry.Attempts < 1 || step.Policy.Retry.Attempts > 10 || step.Policy.Retry.Backoff <= 0 || step.Policy.Retry.MaxBackoff < step.Policy.Retry.Backoff || Terminal(step.State) {
			return nil, ErrInvalid
		}
		step.Attempts = append([]Attempt(nil), step.Attempts...)
		if m.Operation != FinishAttempt && (len(m.Output) > 0 || m.Outcome != "") {
			return nil, ErrInvalid
		}
		cancelled := next.CancelRequestedAt != nil || step.CancelRequestedAt != nil
		expired := !now.Before(*next.Deadline) || (step.Deadline != nil && !now.Before(*step.Deadline))
		switch m.Operation {
		case StartStep:
			if step.State != Pending || m.Reason != "" || cancelled {
				return nil, ErrInvalid
			}
			states := map[string]string{}
			for _, dependency := range next.Steps {
				states[dependency.ID] = string(dependency.State)
			}
			success := true
			for _, id := range step.Policy.Needs {
				if !Terminal(State(states[id])) {
					return nil, ErrInvalid
				}
				success = success && states[id] == string(Succeeded)
			}
			run := success
			if step.Policy.Condition != "" {
				var err error
				run, err = workflow.EvaluateCondition(step.Policy.Condition, states)
				if err != nil {
					step.State, step.Reason = Failed, "condition_invalid"
					step.FinishedAt = stamp(now)
					break
				}
			}
			if !run {
				step.State, step.Reason = Skipped, "dependency_failed"
				if step.Policy.Condition != "" {
					step.Reason = "condition_false"
				}
				step.FinishedAt = stamp(now)
				break
			}
			activeCount := 0
			limit := 1
			if next.Managed {
				limit = next.Parallelism
				if limit < 1 || limit > 256 {
					return nil, ErrInvalid
				}
			}
			for _, active := range next.Steps {
				if active.State == Running {
					activeCount++
				}
				if activeCount >= limit {
					return nil, ErrInvalid
				}
			}
			if expired {
				return nil, ErrInvalid
			}
			step.State = Running
			step.StartedAt = stamp(now)
			deadline := now.Add(step.Policy.Timeout)
			if deadline.After(*next.Deadline) {
				deadline = *next.Deadline
			}
			step.Deadline = &deadline
		case StartAttempt:
			if step.State != Running || cancelled || m.Reason != "" || step.Deadline == nil {
				return nil, ErrInvalid
			}
			if len(step.Attempts) > 0 {
				last := step.Attempts[len(step.Attempts)-1]
				if last.State != Failed || last.Reason != "action_failed" || step.NextAttemptAt == nil || now.Before(*step.NextAttemptAt) {
					return nil, ErrInvalid
				}
			}
			if expired {
				step.State, step.Reason = TimedOut, "deadline"
				step.FinishedAt = stamp(now)
				step.NextAttemptAt = nil
				break
			}
			if len(step.Attempts) >= step.Policy.Retry.Attempts {
				return nil, ErrInvalid
			}
			step.Attempt = len(step.Attempts) + 1
			step.NextAttemptAt = nil
			step.Attempts = append(step.Attempts, Attempt{Number: step.Attempt, State: Running, StartedAt: now})
			emit(step.ID, "attempt_started", Running, "", step.Attempt)
		case FinishAttempt:
			if step.State != Running || len(step.Attempts) == 0 {
				return nil, ErrInvalid
			}
			attempt := step.Attempts[len(step.Attempts)-1]
			if attempt.State != Running {
				return nil, ErrInvalid
			}
			outcome, reason := m.Outcome, m.Reason
			if !validAttemptOutcome(outcome, reason) || (outcome != Succeeded && len(m.Output) > 0) {
				return nil, ErrInvalid
			}
			cancelWins := false
			if cancelled {
				deadline := *next.Deadline
				if step.Deadline != nil && step.Deadline.Before(deadline) {
					deadline = *step.Deadline
				}
				for _, request := range []*time.Time{next.CancelRequestedAt, step.CancelRequestedAt} {
					cancelWins = cancelWins || (request != nil && request.Before(deadline))
				}
			}
			if cancelled && (!expired || cancelWins) {
				outcome, reason = Cancelled, "cancelled"
			} else if expired {
				outcome, reason = TimedOut, "deadline"
			}
			if outcome == Succeeded {
				if len(m.Output) == 0 || len(m.Output) > 65536 || !json.Valid(m.Output) || m.Output[0] != '{' {
					return nil, ErrInvalid
				}
				total := len(m.Output)
				for _, other := range next.Steps {
					total += len(other.Output)
				}
				if total > 1<<20 {
					return nil, ErrLimit
				}
				step.Output = append(json.RawMessage(nil), m.Output...)
			}
			attempt.State, attempt.Reason, attempt.FinishedAt = outcome, reason, stamp(now)
			step.Attempts[len(step.Attempts)-1] = attempt
			emit(step.ID, "attempt_finished", outcome, reason, attempt.Number)
			if outcome == Failed && reason == "action_failed" && attempt.Number < step.Policy.Retry.Attempts {
				delay, err := RetryDelay(step.Policy.Retry, attempt.Number)
				if err != nil {
					return nil, err
				}
				due := now.Add(delay)
				if due.After(*step.Deadline) {
					due = *step.Deadline
				}
				step.NextAttemptAt = &due
				emit(step.ID, "retry_scheduled", Running, "", attempt.Number)
				events[len(events)-1].NextAttemptAt = stamp(due)
			} else {
				step.State, step.Reason, step.FinishedAt = outcome, reason, stamp(now)
				emit(step.ID, "step_state", step.State, step.Reason, 0)
			}
		case FailResolution:
			if step.State != Running || len(step.Attempts) != 0 || (m.Reason != "reference_unavailable" && m.Reason != "input_invalid" && m.Reason != "permission_denied") {
				return nil, ErrInvalid
			}
			step.State, step.Reason, step.FinishedAt = Failed, m.Reason, stamp(now)
		case CancelIdleStep:
			if !cancelled && now.Before(*next.Deadline) {
				return nil, ErrInvalid
			}
			if len(step.Attempts) > 0 && step.Attempts[len(step.Attempts)-1].State == Running {
				return nil, ErrInvalid
			}
			if m.Reason != "" {
				return nil, ErrInvalid
			}
			step.State, step.Reason, step.FinishedAt = Cancelled, "cancelled", stamp(now)
			step.NextAttemptAt = nil
		default:
			return nil, ErrInvalid
		}
		if m.Operation != StartAttempt && m.Operation != FinishAttempt {
			emit(step.ID, "step_state", step.State, step.Reason, 0)
		}
		if m.Operation == StartAttempt && step.State == TimedOut {
			emit(step.ID, "step_state", step.State, step.Reason, 0)
		}
		next.Steps[index] = step
	}
	next.Version++
	next.UpdatedAt = now
	*s = next
	return events, nil
}

func validAttemptOutcome(state State, reason string) bool {
	switch state {
	case Succeeded:
		return reason == ""
	case Failed:
		return reason == "action_failed" || reason == "invalid_output" || reason == "permission_denied"
	case Cancelled:
		return reason == "cancelled"
	case TimedOut:
		return reason == "deadline"
	}
	return false
}

// Aggregate distinguishes legitimate conditional skips and individual cancellation.
func Aggregate(s Snapshot, now time.Time) (State, error) {
	failed, cancelled := false, false
	if len(s.Steps) == 0 {
		return "", ErrInvalid
	}
	for _, step := range s.Steps {
		if !Terminal(step.State) || (step.State == Skipped && step.Reason != "condition_false" && step.Reason != "dependency_failed") {
			return "", ErrInvalid
		}
		failed = failed || step.State == Failed || step.State == TimedOut
		cancelled = cancelled || step.State == Cancelled
	}
	if s.CancelRequestedAt != nil && (s.Deadline == nil || s.CancelRequestedAt.Before(*s.Deadline)) {
		return Cancelled, nil
	}
	if s.Deadline != nil && !now.Before(*s.Deadline) {
		return TimedOut, nil
	}
	if s.CancelRequestedAt != nil {
		return Cancelled, nil
	}
	if failed {
		return Failed, nil
	}
	if cancelled {
		return Cancelled, nil
	}
	return Succeeded, nil
}

// ClassifyRecovery is an audit-only decision. It does not claim ownership or run actions.
func ClassifyRecovery(s Snapshot, now time.Time) string {
	if Terminal(s.State) {
		return "terminal"
	}
	if s.Format != 0 && s.Format != 1 && s.Format != 2 {
		return "invalid"
	}
	for _, step := range s.Steps {
		if step.State == Running {
			if s.Format != 2 {
				return "uncertain_effect"
			}
			if len(step.Attempts) > 0 {
				last := step.Attempts[len(step.Attempts)-1]
				if last.State == Running {
					return "uncertain_effect"
				}
				if last.State != Failed || last.Reason != "action_failed" || step.NextAttemptAt == nil {
					return "invalid"
				}
			}
		}
	}
	if s.Deadline != nil && !now.Before(*s.Deadline) {
		return "expired"
	}
	if s.Format != 2 {
		return "uncertain_effect"
	}
	return "resumable"
}
