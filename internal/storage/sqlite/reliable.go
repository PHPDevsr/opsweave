package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/workflow"
)

var _ execution.ReliableRepository = (*Store)(nil)

// CreateReliable persists format-2 serial policy and admission atomically.
// It does not claim queued work or take over another execution's ownership.
func (s *Store) CreateReliable(ctx context.Context, expected registry.Record, options execution.AdmissionOptions) (result execution.AdmissionResult, err error) {
	return s.admitReliable(ctx, expected, options, nil)
}

func (s *Store) admitReliable(ctx context.Context, expected registry.Record, options execution.AdmissionOptions, queue *execution.Capacity) (result execution.AdmissionResult, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer rollback(tx, &err)
	result, err = admitInTransaction(ctx, tx, expected, options, queue, time.Now().UTC())
	if err != nil {
		return result, err
	}
	err = tx.Commit()
	return result, err
}

// admitInTransaction is shared by manual and atomic scheduler admission. It never commits.
func admitInTransaction(ctx context.Context, tx *sql.Tx, expected registry.Record, options execution.AdmissionOptions, queue *execution.Capacity, now time.Time) (result execution.AdmissionResult, err error) {
	if !execution.ValidID(options.Owner) || !execution.ValidID(expected.Revision) || (options.Parent != "" && !execution.ValidID(options.Parent)) {
		return result, execution.ErrInvalid
	}
	admission, err := execution.AdmissionFor(options.IdempotencyKey, expected, options.Parent, options.AcknowledgedUncertain)
	if err != nil {
		return result, err
	}
	if queue == nil {
		var generation int64
		if err = tx.QueryRowContext(ctx, "SELECT generation FROM dispatcher WHERE singleton=1").Scan(&generation); err != nil {
			return result, err
		}
		if generation > 0 {
			return result, execution.ErrConflict
		}
	}
	current, err := get(ctx, tx, expected.Name, "")
	if err != nil {
		return result, err
	}
	if current.Archived {
		return result, registry.ErrArchived
	}
	actual := current
	if options.Parent != "" {
		actual, err = get(ctx, tx, expected.Name, expected.Revision)
		if err != nil {
			return result, err
		}
	}
	if actual.Revision != expected.Revision || !bytes.Equal(actual.Source, expected.Source) || !bytes.Equal(actual.Definition, expected.Definition) || !bytes.Equal(actual.Bindings, expected.Bindings) {
		return result, execution.ErrConflict
	}
	if options.Parent != "" {
		parent, err := readExecution(ctx, tx, options.Parent)
		if err != nil {
			return result, err
		}
		if parent.Workflow != actual.Name || parent.Revision != actual.Revision || (parent.State != execution.Failed && parent.State != execution.Cancelled && parent.State != execution.TimedOut) {
			return result, execution.ErrInvalid
		}
		uncertain := parent.UncertainEffects
		for _, step := range parent.Steps {
			uncertain = uncertain || step.Reason == "uncertain_effect"
		}
		if uncertain && !options.AcknowledgedUncertain {
			return result, execution.ErrUncertain
		}
	}
	scope := actual.Name + ":run"
	if options.Parent != "" {
		scope = actual.Name + ":retry:" + options.Parent
	}
	if admission.Digest != "" {
		var fingerprint, id, expires string
		err := tx.QueryRowContext(ctx, "SELECT fingerprint,execution_id,expires_at FROM execution_admissions WHERE scope=? AND digest=?", scope, admission.Digest).Scan(&fingerprint, &id, &expires)
		if err == nil {
			expiry, err := time.Parse(time.RFC3339Nano, expires)
			if err != nil {
				return result, err
			}
			if now.Before(expiry) {
				if fingerprint != admission.Fingerprint {
					return result, execution.ErrConflict
				}
				snap, err := readExecution(ctx, tx, id)
				if err != nil {
					return result, err
				}
				return execution.AdmissionResult{Snapshot: snap}, nil
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM execution_admissions WHERE scope=? AND digest=?", scope, admission.Digest); err != nil {
				return result, err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
	}
	var active int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM executions WHERE workflow_name=? AND state IN ('pending','queued','running')", actual.Name).Scan(&active); err != nil {
		return result, err
	}
	if queue == nil && active != 0 {
		return result, execution.ErrConflict
	}
	if queue != nil {
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM executions WHERE state='queued'").Scan(&count); err != nil {
			return result, err
		}
		if count >= queue.QueuedExecutions {
			return result, execution.ErrQueueFull
		}
	}
	id, err := opaqueID()
	if err != nil {
		return result, err
	}
	var snap execution.Snapshot
	if queue == nil {
		snap, err = execution.NewReliable(ctx, id, actual, now)
	} else {
		snap, err = execution.NewDispatcher(ctx, id, actual, now)
	}
	if err != nil {
		return result, err
	}
	if queue != nil {
		snap.State = execution.Queued
		snap.Deadline = nil
	}
	snap.Parent = options.Parent
	snap.AcknowledgedUncertain = options.AcknowledgedUncertain
	body, err := snapshotBody(snap)
	if err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO executions VALUES(?,?,?,?,?,?,?,?)", id, actual.Name, actual.Revision, snap.State, snap.Version, options.Owner, now.Format(time.RFC3339Nano), body); err != nil {
		return result, err
	}
	var parent any
	if options.Parent != "" {
		parent = options.Parent
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO execution_policies VALUES(?,2,16384,?,?)", id, parent, options.AcknowledgedUncertain); err != nil {
		return result, err
	}
	for ordinal, step := range snap.Steps {
		b, err := stepBody(step)
		if err != nil {
			return result, err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO execution_steps VALUES(?,?,?,?)", id, step.ID, ordinal, b); err != nil {
			return result, err
		}
	}
	states := []execution.State{execution.Pending, execution.Queued, execution.Running}
	if queue != nil {
		states = states[:2]
		d, parseErr := workflow.Parse(ctx, actual.Source)
		if parseErr != nil {
			return result, parseErr
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO execution_queue VALUES(?,?,?,?,?,0)", id, now.Format(time.RFC3339Nano), now.Add(queue.QueueTimeout).Format(time.RFC3339Nano), int64(d.Spec.Timeout), d.Spec.Concurrency); err != nil {
			return result, err
		}
	}
	for _, state := range states {
		if err := appendEvent(ctx, tx, execution.Event{Time: now, Execution: id, Code: "execution_state", State: state}); err != nil {
			return result, err
		}
	}
	if admission.Digest != "" {
		if _, err := tx.ExecContext(ctx, "INSERT INTO execution_admissions VALUES(?,?,?,?,?,?)", scope, admission.Digest, admission.Fingerprint, id, now.Format(time.RFC3339Nano), now.Add(7*24*time.Hour).Format(time.RFC3339Nano)); err != nil {
			return result, err
		}
	}
	result = execution.AdmissionResult{Snapshot: snap, Created: true}
	return result, nil
}

func stepBody(step execution.Step) (string, error) {
	step.Attempts = nil
	b, err := json.Marshal(step)
	return string(b), err
}

// ChangeReliable commits attempt/state/event changes under the same expected owner/version.
func (s *Store) ChangeReliable(ctx context.Context, m execution.ReliableMutation) (result execution.Snapshot, err error) {
	return s.changeFenced(ctx, m, time.Now, nil)
}

func (s *Store) changeReliable(ctx context.Context, m execution.ReliableMutation, clock func() time.Time) (execution.Snapshot, error) {
	return s.changeFenced(ctx, m, clock, nil)
}

func (s *Store) changeFenced(ctx context.Context, m execution.ReliableMutation, clock func() time.Time, fence *execution.Fence) (result execution.Snapshot, err error) {
	if !execution.ValidID(m.ID) || !execution.ValidID(m.Owner) {
		return result, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer rollback(tx, &err)
	var owner string
	if err := tx.QueryRowContext(ctx, "SELECT owner FROM executions WHERE id=?", m.ID).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, execution.ErrNotFound
		}
		return result, err
	}
	if owner != m.Owner {
		return result, execution.ErrConflict
	}
	result, err = readExecution(ctx, tx, m.ID)
	if err != nil {
		return result, err
	}
	if result.Managed {
		if fence == nil {
			return result, execution.ErrConflict
		}
		if err = checkFence(ctx, tx, *fence, result.ID); err != nil {
			return result, err
		}
	} else if fence != nil {
		return result, execution.ErrConflict
	}
	previous := result
	events, err := execution.ApplyReliable(&result, m, clock().UTC())
	if err != nil {
		return execution.Snapshot{}, err
	}
	changedStep := m.Step
	if m.Operation == execution.CancelIdleSteps {
		changedStep = "*"
	}
	if err := saveReliable(ctx, tx, result, previous.Version, owner, changedStep); err != nil {
		return execution.Snapshot{}, err
	}
	for _, event := range events {
		if err := appendEvent(ctx, tx, event); err != nil {
			return execution.Snapshot{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func saveReliable(ctx context.Context, tx *sql.Tx, snap execution.Snapshot, expectedVersion int64, owner, changedStep string) error {
	body, err := snapshotBody(snap)
	if err != nil {
		return err
	}
	update, err := tx.ExecContext(ctx, "UPDATE executions SET body=?,state=?,version=? WHERE id=? AND owner=? AND version=? AND state IN ('queued','running')", body, snap.State, snap.Version, snap.ID, owner, expectedVersion)
	if err != nil {
		return err
	}
	count, err := update.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return execution.ErrConflict
	}
	for _, step := range snap.Steps {
		if changedStep == "" || (changedStep != "*" && step.ID != changedStep) {
			continue
		}
		b, err := stepBody(step)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE execution_steps SET body=? WHERE execution_id=? AND step_id=?", b, snap.ID, step.ID); err != nil {
			return err
		}
		for _, attempt := range step.Attempts {
			var existing string
			err := tx.QueryRowContext(ctx, "SELECT body FROM execution_attempts WHERE execution_id=? AND step_id=? AND number=?", snap.ID, step.ID, attempt.Number).Scan(&existing)
			encoded, marshalErr := json.Marshal(attempt)
			if marshalErr != nil {
				return marshalErr
			}
			if errors.Is(err, sql.ErrNoRows) {
				if _, err := tx.ExecContext(ctx, "INSERT INTO execution_attempts VALUES(?,?,?,?,?)", snap.ID, step.ID, attempt.Number, attempt.State, string(encoded)); err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else if existing != string(encoded) {
				if _, err := tx.ExecContext(ctx, "UPDATE execution_attempts SET body=?,state=? WHERE execution_id=? AND step_id=? AND number=?", string(encoded), attempt.State, snap.ID, step.ID, attempt.Number); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// RequestReliableCancellation records an idempotent request without claiming work stopped.
func (s *Store) RequestReliableCancellation(ctx context.Context, id, stepID string) (result execution.Snapshot, err error) {
	if !execution.ValidID(id) {
		return result, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer rollback(tx, &err)
	result, err = readExecution(ctx, tx, id)
	if err != nil {
		return result, err
	}
	if stepID != "" {
		found := false
		for _, step := range result.Steps {
			found = found || step.ID == stepID
		}
		if !found {
			return execution.Snapshot{}, execution.ErrInvalid
		}
	}
	if execution.Terminal(result.State) {
		return result, nil
	}
	if result.Format != 2 {
		return result, execution.ErrInvalid
	}
	now := time.Now().UTC()
	changed := false
	if stepID == "" {
		if result.CancelRequestedAt == nil {
			result.CancelRequestedAt = &now
			changed = true
		}
	} else {
		found := false
		for i := range result.Steps {
			step := &result.Steps[i]
			if step.ID == stepID {
				found = true
				if !execution.Terminal(step.State) && step.CancelRequestedAt == nil {
					step.CancelRequestedAt = &now
					changed = true
				}
			}
		}
		if !found {
			return execution.Snapshot{}, execution.ErrInvalid
		}
	}
	if !changed {
		return result, nil
	}
	previous := result.Version
	result.Version++
	result.UpdatedAt = now
	var owner string
	if err := tx.QueryRowContext(ctx, "SELECT owner FROM executions WHERE id=?", id).Scan(&owner); err != nil {
		return execution.Snapshot{}, err
	}
	if err := saveReliable(ctx, tx, result, previous, owner, stepID); err != nil {
		return execution.Snapshot{}, err
	}
	if err := appendEvent(ctx, tx, execution.Event{Time: now, Execution: id, Step: stepID, Code: "cancellation_requested", State: result.State}); err != nil {
		return execution.Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func readReliable(ctx context.Context, q executionQuerier, snap *execution.Snapshot) error {
	var format, limit int
	var parent sql.NullString
	var ack bool
	if err := q.QueryRowContext(ctx, "SELECT format,event_limit,parent_id,acknowledged_uncertain FROM execution_policies WHERE execution_id=?", snap.ID).Scan(&format, &limit, &parent, &ack); err != nil {
		return err
	}
	if format != 2 || limit != 16384 || parent.String != snap.Parent || ack != snap.AcknowledgedUncertain || (snap.Deadline == nil && !(snap.Managed && (snap.State == execution.Queued || execution.Terminal(snap.State)))) {
		return execution.ErrInvalid
	}
	if snap.Managed && (snap.Parallelism < 1 || snap.Parallelism > 256) {
		return execution.ErrInvalid
	}
	index := map[string]int{}
	for i, step := range snap.Steps {
		if step.Policy == nil || (step.State == execution.Running && step.Deadline == nil) {
			return execution.ErrInvalid
		}
		index[step.ID] = i
	}
	rows, err := q.QueryContext(ctx, "SELECT step_id,number,state,body FROM execution_attempts WHERE execution_id=? ORDER BY step_id,number LIMIT 2561", snap.ID)
	if err != nil {
		return err
	}
	count, total := 0, 0
	for rows.Next() {
		var id, state, body string
		var number int
		if err := rows.Scan(&id, &number, &state, &body); err != nil {
			return errors.Join(err, rows.Close())
		}
		count++
		total += len(body)
		i, ok := index[id]
		if !ok || count > 2560 || total > 64<<20 {
			return errors.Join(execution.ErrLimit, rows.Close())
		}
		var attempt execution.Attempt
		if json.Unmarshal([]byte(body), &attempt) != nil || attempt.Number != number || string(attempt.State) != state || number != len(snap.Steps[i].Attempts)+1 {
			return errors.Join(execution.ErrInvalid, rows.Close())
		}
		attempts := snap.Steps[i].Attempts
		if len(attempts) > 0 && attempts[len(attempts)-1].State == execution.Running {
			return errors.Join(execution.ErrInvalid, rows.Close())
		}
		snap.Steps[i].Attempts = append(attempts, attempt)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	for _, step := range snap.Steps {
		if step.Attempt != len(step.Attempts) || len(step.Attempts) > step.Policy.Retry.Attempts {
			return execution.ErrInvalid
		}
		for _, attempt := range step.Attempts {
			if (attempt.State == execution.Running) != (attempt.FinishedAt == nil) {
				return execution.ErrInvalid
			}
		}
		if execution.Terminal(step.State) && len(step.Attempts) > 0 && step.Attempts[len(step.Attempts)-1].State == execution.Running {
			return execution.ErrInvalid
		}
	}
	return nil
}

// OwnedReliable re-reads cancellation/version changes without permitting an ownership takeover.
func (s *Store) OwnedReliable(ctx context.Context, id, owner string) (result execution.Snapshot, err error) {
	if !execution.ValidID(id) || !execution.ValidID(owner) {
		return result, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer rollback(tx, &err)
	var actual string
	if err := tx.QueryRowContext(ctx, "SELECT owner FROM executions WHERE id=?", id).Scan(&actual); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, execution.ErrNotFound
		}
		return result, err
	}
	if actual != owner {
		return result, execution.ErrConflict
	}
	result, err = readExecution(ctx, tx, id)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return execution.Snapshot{}, err
	}
	return result, nil
}

// ReliableControl reads bounded cancellation metadata without copying action outputs.
func (s *Store) ReliableControl(ctx context.Context, id, owner string) (execution.Cancellation, error) {
	if !execution.ValidID(id) || !execution.ValidID(owner) {
		return execution.Cancellation{}, execution.ErrInvalid
	}
	var actual, body string
	if err := s.db.QueryRowContext(ctx, "SELECT owner,body FROM executions WHERE id=?", id).Scan(&actual, &body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return execution.Cancellation{}, execution.ErrNotFound
		}
		return execution.Cancellation{}, err
	}
	if actual != owner {
		return execution.Cancellation{}, execution.ErrConflict
	}
	var snap execution.Snapshot
	if len(body) > 65536 || json.Unmarshal([]byte(body), &snap) != nil || snap.Format != 2 {
		return execution.Cancellation{}, execution.ErrInvalid
	}
	result := execution.Cancellation{State: snap.State, Execution: snap.CancelRequestedAt != nil, Steps: map[string]bool{}}
	rows, err := s.db.QueryContext(ctx, "SELECT step_id FROM execution_steps WHERE execution_id=? AND json_extract(body,'$.cancelRequestedAt') IS NOT NULL", id)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var step string
		if err := rows.Scan(&step); err != nil {
			return result, errors.Join(err, rows.Close())
		}
		if len(result.Steps) >= 256 {
			return result, errors.Join(execution.ErrLimit, rows.Close())
		}
		result.Steps[step] = true
	}
	return result, errors.Join(rows.Err(), rows.Close())
}
