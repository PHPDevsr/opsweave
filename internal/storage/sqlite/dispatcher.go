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

var _ execution.DispatchRepository = (*Store)(nil)

// StandaloneReady prevents a synchronous invocation from adopting unrelated active work.
// The caller holds the native lock, so another managed dispatcher cannot race this check.
func (s *Store) StandaloneReady(ctx context.Context) error {
	var active int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM executions WHERE state='running'").Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return execution.ErrBusy
	}
	return nil
}

// Enqueue acknowledges durable admission only; it never invokes or claims an action.
func (s *Store) Enqueue(ctx context.Context, r registry.Record, q execution.QueueAdmission) (execution.AdmissionResult, error) {
	if err := q.Capacity.Validate(); err != nil {
		return execution.AdmissionResult{}, err
	}
	owner, err := opaqueID()
	if err != nil {
		return execution.AdmissionResult{}, err
	}
	q.Options.Owner = owner
	return s.admitReliable(ctx, r, q.Options, &q.Capacity)
}

func checkFence(ctx context.Context, tx *sql.Tx, f execution.Fence, id string) error {
	if f.Generation < 1 || !execution.ValidID(f.Owner) {
		return execution.ErrInvalid
	}
	var generation int64
	var owner string
	if err := tx.QueryRowContext(ctx, "SELECT generation,owner FROM dispatcher WHERE singleton=1").Scan(&generation, &owner); err != nil {
		return err
	}
	if generation != f.Generation || owner != f.Owner {
		return execution.ErrConflict
	}
	if id != "" {
		var claimed int64
		if err := tx.QueryRowContext(ctx, "SELECT generation FROM execution_queue WHERE execution_id=?", id).Scan(&claimed); err != nil {
			return err
		}
		if claimed != generation {
			return execution.ErrConflict
		}
	}
	return nil
}

// BeginDispatcher advances the generation only while the caller holds the native lock.
func (s *Store) BeginDispatcher(ctx context.Context, owner string, adopt bool) (f execution.Fence, err error) {
	if !execution.ValidID(owner) {
		return f, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return f, err
	}
	defer rollback(tx, &err)
	var unmanaged int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM executions e LEFT JOIN execution_queue q ON q.execution_id=e.id WHERE e.state IN ('pending','queued','running') AND q.execution_id IS NULL").Scan(&unmanaged); err != nil {
		return f, err
	}
	if unmanaged > 0 && !adopt {
		return f, execution.ErrUnmanaged
	}
	f.Owner = owner
	if err = tx.QueryRowContext(ctx, "UPDATE dispatcher SET generation=generation+1,owner=?,server_state='unavailable',worker_capacity=0 WHERE singleton=1 AND generation<9223372036854775807 RETURNING generation", owner).Scan(&f.Generation); err != nil {
		return f, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO dispatcher_audit VALUES(?,?,?)", f.Generation, adopt, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return f, err
	}
	err = tx.Commit()
	return f, err
}

// EndDispatcher releases durable ownership only after its invocations have joined.
func (s *Store) EndDispatcher(ctx context.Context, f execution.Fence) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx, &err)
	if err = checkFence(ctx, tx, f, ""); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE dispatcher SET owner='',server_state='unavailable',worker_capacity=0 WHERE singleton=1"); err != nil {
		return err
	}
	return tx.Commit()
}

// RecoverDispatcher fences stopped owners before classifying their durable attempts.
func (s *Store) RecoverDispatcher(ctx context.Context, f execution.Fence) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx, &err)
	if err = checkFence(ctx, tx, f, ""); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id FROM executions WHERE state='running' ORDER BY created_at,id LIMIT 10001")
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return errors.Join(err, rows.Close())
		}
		ids = append(ids, id)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	if len(ids) > 10000 {
		return execution.ErrLimit
	}
	now := time.Now().UTC()
	resumable := 0
	for _, id := range ids {
		snap, err := readExecution(ctx, tx, id)
		if err != nil {
			return err
		}
		before := snap.Version
		snap, events, err := execution.RecoverStopped(snap, now)
		if err != nil {
			return err
		}
		if snap.State == execution.Running {
			resumable++
			if resumable > 256 {
				return execution.ErrLimit
			}
			if !snap.Managed {
				r, err := get(ctx, tx, snap.Workflow, snap.Revision)
				if err != nil {
					return err
				}
				d, err := workflow.Parse(ctx, r.Source)
				if err != nil {
					return err
				}
				snap.Managed = true
				snap.Parallelism = d.Spec.Parallelism
				if _, err = tx.ExecContext(ctx, "INSERT INTO execution_queue VALUES(?,?,?,?,?,?)", id, snap.CreatedAt.Format(time.RFC3339Nano), snap.CreatedAt.Add(24*time.Hour).Format(time.RFC3339Nano), int64(d.Spec.Timeout), d.Spec.Concurrency, f.Generation); err != nil {
					return err
				}
			}
			snap.Version++
			snap.UpdatedAt = now
			events = append(events, execution.Event{Time: now, Execution: id, Code: "execution_recovered", State: execution.Running})
		}
		var oldOwner string
		if err = tx.QueryRowContext(ctx, "SELECT owner FROM executions WHERE id=?", id).Scan(&oldOwner); err != nil {
			return err
		}
		if err = saveReliable(ctx, tx, snap, before, oldOwner, "*"); err != nil {
			return err
		}
		if snap.Managed {
			if _, err = tx.ExecContext(ctx, "UPDATE execution_queue SET generation=? WHERE execution_id=?", f.Generation, id); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "UPDATE executions SET owner=? WHERE id=?", f.Owner, id); err != nil {
				return err
			}
		}
		for _, event := range events {
			if err = appendEvent(ctx, tx, event); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// QueuedExecutions returns bounded ordered candidates, optionally only one standalone admission.
func (s *Store) QueuedExecutions(ctx context.Context, only string) ([]execution.Snapshot, error) {
	if only != "" && !execution.ValidID(only) {
		return nil, execution.ErrInvalid
	}
	rows, err := s.db.QueryContext(ctx, "SELECT e.id,e.body FROM executions e JOIN execution_queue q ON q.execution_id=e.id JOIN workflows w ON w.name=e.workflow_name LEFT JOIN scheduled_executions cron ON cron.execution_id=e.id LEFT JOIN event_executions incoming ON incoming.execution_id=e.id WHERE e.state='queued' AND (?='' OR e.id=?) AND (json_extract(e.body,'$.cancelRequestedAt') IS NOT NULL OR julianday(q.expires_at)<=julianday(?) OR w.archived=1 OR (coalesce(cron.execution_id,incoming.execution_id) IS NOT NULL AND (w.enabled=0 OR coalesce(cron.automatic_generation,incoming.automatic_generation)<>w.automatic_generation)) OR (julianday(q.available_at)<=julianday(?) AND (SELECT count(*) FROM executions a WHERE a.workflow_name=e.workflow_name AND a.state='running')<min(q.concurrency,COALESCE((SELECT min(aq.concurrency) FROM execution_queue aq JOIN executions a ON a.id=aq.execution_id WHERE a.workflow_name=e.workflow_name AND a.state='running'),q.concurrency)))) ORDER BY q.available_at,e.created_at,e.id LIMIT 1", only, only, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	var result []execution.Snapshot
	for rows.Next() {
		var id, body string
		if err = rows.Scan(&id, &body); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		var snap execution.Snapshot
		if len(result) >= 10000 || len(body) > 65536 {
			return nil, errors.Join(execution.ErrLimit, rows.Close())
		}
		if json.Unmarshal([]byte(body), &snap) != nil || snap.ID != id || snap.State != execution.Queued || !snap.Managed {
			return nil, errors.Join(execution.ErrInvalid, rows.Close())
		}
		result = append(result, snap)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	return result, nil
}

// ClaimExecution atomically checks global/revision capacity and starts the running deadline.
// expected must already pass the caller's exact action-catalog preflight.
func (s *Store) ClaimExecution(ctx context.Context, f execution.Fence, id string, expected registry.Record, capacity execution.Capacity, requestDeadline ...time.Time) (snap execution.Snapshot, err error) {
	if !execution.ValidID(id) || capacity.Validate() != nil {
		return snap, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return snap, err
	}
	defer rollback(tx, &err)
	if err = checkFence(ctx, tx, f, ""); err != nil {
		return snap, err
	}
	snap, err = readExecution(ctx, tx, id)
	if err != nil {
		return snap, err
	}
	if snap.State != execution.Queued || !snap.Managed {
		return snap, execution.ErrConflict
	}
	var expiry, available string
	var timeout int64
	var limit int
	if err = tx.QueryRowContext(ctx, "SELECT expires_at,available_at,timeout_ns,concurrency FROM execution_queue WHERE execution_id=?", id).Scan(&expiry, &available, &timeout, &limit); err != nil {
		return snap, err
	}
	expires, err := time.Parse(time.RFC3339Nano, expiry)
	if err != nil {
		return snap, err
	}
	due, err := time.Parse(time.RFC3339Nano, available)
	if err != nil {
		return snap, err
	}
	now := time.Now().UTC()
	reason := ""
	if !now.Before(expires) {
		reason = "queue_expired"
	}
	if snap.CancelRequestedAt != nil {
		reason = "cancelled"
	}
	current, err := get(ctx, tx, snap.Workflow, snap.Revision)
	if err != nil {
		return snap, err
	}
	if current.Archived {
		reason = "workflow_archived"
	}
	{
		var original sql.NullInt64
		var currentGeneration int64
		if err = tx.QueryRowContext(ctx, "SELECT coalesce(cron.automatic_generation,incoming.automatic_generation),w.automatic_generation FROM executions e JOIN workflows w ON w.name=e.workflow_name LEFT JOIN scheduled_executions cron ON cron.execution_id=e.id LEFT JOIN event_executions incoming ON incoming.execution_id=e.id WHERE e.id=?", snap.ID).Scan(&original, &currentGeneration); err != nil {
			return snap, err
		}
		if original.Valid != (snap.Trigger != nil) {
			return snap, execution.ErrInvalid
		}
		if original.Valid && !current.Archived && (!current.Enabled || original.Int64 != currentGeneration) {
			reason = "workflow_disabled"
		}
	}

	if reason != "" {
		if err = finishQueued(ctx, tx, &snap, reason, now); err != nil {
			return snap, err
		}
		err = tx.Commit()
		return snap, err
	}
	if now.Before(due) {
		return snap, execution.ErrBusy
	}
	if expected.Name != current.Name || expected.Revision != current.Revision || !bytes.Equal(expected.Source, current.Source) || !bytes.Equal(expected.Definition, current.Definition) || !bytes.Equal(expected.Bindings, current.Bindings) {
		return snap, execution.ErrConflict
	}
	var total, active, effective int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM executions WHERE state='running'").Scan(&total); err != nil {
		return snap, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*),COALESCE(MIN(q.concurrency),?) FROM executions e JOIN execution_queue q ON q.execution_id=e.id WHERE e.workflow_name=? AND e.state='running'", limit, snap.Workflow).Scan(&active, &effective); err != nil {
		return snap, err
	}
	if total >= capacity.RunningExecutions || active >= min(limit, effective) {
		return snap, execution.ErrBusy
	}
	before := snap.Version
	snap.State = execution.Running
	snap.Version++
	snap.UpdatedAt = now
	deadline := now.Add(time.Duration(timeout))
	parent, ok := ctx.Deadline()
	if len(requestDeadline) > 1 {
		return snap, execution.ErrInvalid
	}
	if len(requestDeadline) == 1 {
		parent = requestDeadline[0]
		ok = !parent.IsZero()
	}
	if ok && parent.Before(deadline) {
		deadline = parent
	}
	snap.Deadline = &deadline
	body, err := snapshotBody(snap)
	if err != nil {
		return snap, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE executions SET body=?,state=?,version=?,owner=? WHERE id=? AND version=? AND state='queued'", body, snap.State, snap.Version, f.Owner, id, before); err != nil {
		return snap, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE execution_queue SET generation=? WHERE execution_id=?", f.Generation, id); err != nil {
		return snap, err
	}
	if err = appendEvent(ctx, tx, execution.Event{Time: now, Execution: id, Code: "execution_state", State: execution.Running}); err != nil {
		return snap, err
	}
	err = tx.Commit()
	return snap, err
}

func finishQueued(ctx context.Context, tx *sql.Tx, snap *execution.Snapshot, reason string, now time.Time) error {
	before := snap.Version
	snap.State = execution.Cancelled
	snap.Version++
	snap.UpdatedAt = now
	for i := range snap.Steps {
		step := &snap.Steps[i]
		step.State, step.Reason, step.FinishedAt = execution.Cancelled, reason, &now
	}
	var owner string
	if err := tx.QueryRowContext(ctx, "SELECT owner FROM executions WHERE id=?", snap.ID).Scan(&owner); err != nil {
		return err
	}
	if err := saveReliable(ctx, tx, *snap, before, owner, "*"); err != nil {
		return err
	}
	return appendEvent(ctx, tx, execution.Event{Time: now, Execution: snap.ID, Code: "execution_state", State: execution.Cancelled, Reason: reason})
}

// RejectQueued records a safe engine-owned admission failure without invoking actions.
func (s *Store) RejectQueued(ctx context.Context, f execution.Fence, id, reason string) (snap execution.Snapshot, err error) {
	if !execution.ValidID(id) || (reason != "action_unavailable" && reason != "input_invalid") {
		return snap, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return snap, err
	}
	defer rollback(tx, &err)
	if err = checkFence(ctx, tx, f, ""); err != nil {
		return snap, err
	}
	snap, err = readExecution(ctx, tx, id)
	if err != nil {
		return snap, err
	}
	if !snap.Managed || snap.State != execution.Queued {
		return snap, execution.ErrConflict
	}
	if err = finishQueued(ctx, tx, &snap, reason, time.Now().UTC()); err != nil {
		return snap, err
	}
	err = tx.Commit()
	return snap, err
}

// DispatchExecutions returns only running executions of the currently fenced owner.
func (s *Store) DispatchExecutions(ctx context.Context, f execution.Fence) (result []execution.Snapshot, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rollback(tx, &err)
	if err = checkFence(ctx, tx, f, ""); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT e.id FROM executions e JOIN execution_queue q ON q.execution_id=e.id WHERE e.state='running' AND e.owner=? AND q.generation=? ORDER BY e.created_at,e.id LIMIT 257", f.Owner, f.Generation)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		ids = append(ids, id)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	if len(ids) > 256 {
		return nil, execution.ErrLimit
	}
	for _, id := range ids {
		snap, err := readExecution(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, snap)
	}
	err = tx.Commit()
	return result, err
}

// ChangeDispatch compares generation, execution owner and version in the same transaction.
func (s *Store) ChangeDispatch(ctx context.Context, f execution.Fence, m execution.ReliableMutation) (execution.Snapshot, error) {
	if m.Owner != f.Owner {
		return execution.Snapshot{}, execution.ErrConflict
	}
	return s.changeFenced(ctx, m, time.Now, &f)
}
