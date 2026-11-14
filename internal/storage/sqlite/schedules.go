package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/schedule"
	"github.com/PHPDevsr/opsweave/internal/scheduler"
)

var _ scheduler.Repository = (*Store)(nil)

func reconcileSchedules(ctx context.Context, tx *sql.Tx, name, revision string, triggers []schedule.Trigger, now time.Time) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM schedules WHERE workflow_name=?", name); err != nil {
		return err
	}
	for _, trigger := range triggers {
		if _, err := tx.ExecContext(ctx, "INSERT INTO schedules VALUES(?,?,?,?,?,?,?,?,NULL,?,0,'')", name, trigger.ID, revision, trigger.Schedule, trigger.Timezone, trigger.MissedRun, now.Format(time.RFC3339Nano), now.Truncate(time.Minute).Unix(), now.Truncate(time.Minute).Add(time.Minute).Unix()); err != nil {
			return err
		}
	}
	return nil
}

const scheduleColumns = "s.workflow_name,s.revision_id,s.trigger_id,s.expression,s.timezone,s.missed_run,s.cursor_at,s.pending_at,s.version,w.enabled,s.retry_at,s.last_failure"

func scanSchedule(row interface{ Scan(...any) error }) (entry scheduler.Entry, err error) {
	var cursor string
	var pending sql.NullString
	var retry int64
	err = row.Scan(&entry.Workflow, &entry.Revision, &entry.Trigger.ID, &entry.Trigger.Schedule, &entry.Trigger.Timezone, &entry.Trigger.MissedRun, &cursor, &pending, &entry.Version, &entry.Enabled, &retry, &entry.LastFailure)
	if errors.Is(err, sql.ErrNoRows) {
		return entry, scheduler.ErrStaleEntry
	}
	if err != nil {
		return entry, err
	}
	entry.Trigger.Type = "cron"
	entry.RetryAt = time.Unix(retry, 0).UTC()
	if entry.Cursor, err = time.Parse(time.RFC3339Nano, cursor); err != nil {
		return entry, execution.ErrInvalid
	}
	if pending.Valid {
		at, parseErr := time.Parse(time.RFC3339Nano, pending.String)
		if parseErr != nil {
			return entry, execution.ErrInvalid
		}
		entry.Pending = &at
	}
	return entry, nil
}

// DueSchedules returns at most 64 exact-revision cursors, with a bounded payload total.
func (s *Store) DueSchedules(ctx context.Context, f execution.Fence, now time.Time) (result []scheduler.Entry, err error) {
	if now.IsZero() || now.Year() < 1 || now.Year() > 9999 {
		return nil, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rollback(tx, &err)
	if err = checkFence(ctx, tx, f, ""); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT "+scheduleColumns+" FROM schedules s JOIN workflows w ON w.name=s.workflow_name WHERE w.enabled=1 AND w.archived=0 AND w.current_revision=s.revision_id AND s.retry_at<=? AND (s.checked_minute<? OR s.pending_at IS NOT NULL) ORDER BY s.retry_at,s.workflow_name,s.trigger_id LIMIT 64", now.Unix(), now.Truncate(time.Minute).Unix())
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		entry, e := scanSchedule(rows)
		if e != nil {
			return nil, errors.Join(e, rows.Close())
		}
		result = append(result, entry)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	total := 0
	for i := range result {
		record, e := get(ctx, tx, result[i].Workflow, result[i].Revision)
		if e != nil {
			return nil, e
		}
		total += len(record.Source) + len(record.Definition) + len(record.Bindings)
		if total > 64<<20 {
			if i == 0 {
				return nil, execution.ErrLimit
			}
			result = result[:i]
			break
		}
		result[i].Record = record
	}
	err = tx.Commit()
	return result, err
}

// AdvanceSchedule atomically commits a bounded scan and its pending/delivered identity.
func (s *Store) AdvanceSchedule(ctx context.Context, f execution.Fence, expected scheduler.Entry, plan schedule.Plan, preflightFailure string, capacity execution.Capacity, now time.Time) (result scheduler.Result, err error) {
	now = now.UTC()
	if capacity.Validate() != nil || (preflightFailure != "" && (preflightFailure != "action_unavailable" || plan.Due == nil)) || plan.Skipped < 0 || plan.Skipped > 1441 || now.IsZero() || now.Year() < 1 || now.Year() > 9999 || plan.Cursor.Before(expected.Cursor) || !plan.Cursor.Equal(now) {
		return result, execution.ErrInvalid
	}
	if plan.Due != nil && (plan.Due.After(now) || now.Sub(*plan.Due) > 24*time.Hour || plan.Due.Second() != 0 || plan.Due.Nanosecond() != 0) {
		return result, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer rollback(tx, &err)
	if err = checkFence(ctx, tx, f, ""); err != nil {
		return result, err
	}
	entry, err := scanSchedule(tx.QueryRowContext(ctx, "SELECT "+scheduleColumns+" FROM schedules s JOIN workflows w ON w.name=s.workflow_name WHERE s.workflow_name=? AND s.trigger_id=? AND w.archived=0 AND w.current_revision=s.revision_id", expected.Workflow, expected.Trigger.ID))
	if err != nil {
		return result, err
	}
	if !entry.Enabled || entry.Revision != expected.Revision || entry.Version != expected.Version || entry.Trigger != expected.Trigger || !entry.Cursor.Equal(expected.Cursor) {
		return result, scheduler.ErrStaleEntry
	}
	actual, err := get(ctx, tx, entry.Workflow, entry.Revision)
	if err != nil {
		return result, err
	}
	if expected.Record.Revision != actual.Revision || !bytes.Equal(expected.Record.Source, actual.Source) || !bytes.Equal(expected.Record.Definition, actual.Definition) || !bytes.Equal(expected.Record.Bindings, actual.Bindings) {
		return result, scheduler.ErrStaleEntry
	}
	pending := plan.Due
	failure := ""
	retry := now.Truncate(time.Minute).Add(time.Minute).Unix()
	appendHistory := func(code string, at *time.Time, id string, skipped int) error {
		return appendScheduleEvent(ctx, tx, scheduler.Event{Workflow: entry.Workflow, Revision: entry.Revision, Trigger: entry.Trigger.ID, Time: now, ScheduledAt: at, Code: code, Execution: id, Skipped: skipped})
	}
	if plan.Expired {
		if err = appendHistory("intent_expired", entry.Pending, "", 0); err != nil {
			return result, err
		}
	}
	if plan.Skipped > 0 {
		if err = appendHistory("schedule_skipped", nil, "", plan.Skipped); err != nil {
			return result, err
		}
	}
	if plan.Due != nil {
		if !plan.Due.After(entry.Cursor) && (entry.Pending == nil || !entry.Pending.Equal(*plan.Due)) {
			return result, execution.ErrInvalid
		}
		result, err = deliverSchedule(ctx, tx, f, entry, actual, *plan.Due, preflightFailure, capacity, now)
		if err != nil {
			return result, err
		}
		switch result.Code {
		case "admitted":
			pending = nil
			if err = appendHistory("admitted", plan.Due, result.Execution, 0); err != nil {
				return result, err
			}
		case "deduplicated":
			pending = nil
		default:
			failure = result.Code
			retry = now.Add(time.Minute).Unix()
			if entry.LastFailure != failure || entry.Pending == nil || !entry.Pending.Equal(*plan.Due) {
				if err = appendHistory(failure, plan.Due, "", 0); err != nil {
					return result, err
				}
			}
		}
	}

	if result.Code == "" {
		result.Code = "checked"
	}
	var pendingValue any
	if pending != nil {
		pendingValue = pending.UTC().Format(time.RFC3339Nano)
	}
	update, err := tx.ExecContext(ctx, "UPDATE schedules SET cursor_at=?,checked_minute=?,pending_at=?,retry_at=?,version=version+1,last_failure=? WHERE workflow_name=? AND trigger_id=? AND version=?", plan.Cursor.UTC().Format(time.RFC3339Nano), now.Truncate(time.Minute).Unix(), pendingValue, retry, failure, entry.Workflow, entry.Trigger.ID, entry.Version)
	if err != nil {
		return result, err
	}
	count, err := update.RowsAffected()
	if err != nil {
		return result, err
	}
	if count != 1 {
		return result, scheduler.ErrStaleEntry
	}
	err = tx.Commit()
	return result, err
}

// deliverSchedule shares the queue transaction and preserves an existing occurrence's identity.
func deliverSchedule(ctx context.Context, tx *sql.Tx, f execution.Fence, entry scheduler.Entry, record registry.Record, due time.Time, preflightFailure string, capacity execution.Capacity, now time.Time) (scheduler.Result, error) {
	due = due.UTC()
	var existing string
	err := tx.QueryRowContext(ctx, "SELECT execution_id FROM scheduled_executions WHERE workflow_name=? AND revision_id=? AND trigger_id=? AND scheduled_at=?", entry.Workflow, entry.Revision, entry.Trigger.ID, due.Format(time.RFC3339Nano)).Scan(&existing)
	if err == nil {
		return scheduler.Result{Execution: existing, Code: "deduplicated"}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return scheduler.Result{}, err
	}
	if preflightFailure != "" {
		return scheduler.Result{Code: preflightFailure}, nil
	}
	admitted, err := admitInTransaction(ctx, tx, record, execution.AdmissionOptions{Owner: f.Owner}, &capacity, now)
	if errors.Is(err, execution.ErrQueueFull) {
		return scheduler.Result{Code: "queue_full"}, nil
	}
	if err != nil {
		return scheduler.Result{}, err
	}
	var generation int64
	if err = tx.QueryRowContext(ctx, "SELECT automatic_generation FROM workflows WHERE name=?", entry.Workflow).Scan(&generation); err != nil {
		return scheduler.Result{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO scheduled_executions VALUES(?,?,?,?,?,?)", admitted.Snapshot.ID, entry.Workflow, entry.Revision, entry.Trigger.ID, due.Format(time.RFC3339Nano), generation); err != nil {
		return scheduler.Result{}, err
	}
	admitted.Snapshot.Trigger = &execution.TriggerOrigin{Type: "cron", ID: entry.Trigger.ID, ScheduledAt: due}
	body, err := snapshotBody(admitted.Snapshot)
	if err != nil {
		return scheduler.Result{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE executions SET body=? WHERE id=?", body, admitted.Snapshot.ID); err != nil {
		return scheduler.Result{}, err
	}
	return scheduler.Result{Execution: admitted.Snapshot.ID, Code: "admitted"}, nil
}

func appendScheduleEvent(ctx context.Context, tx *sql.Tx, event scheduler.Event) error {
	var scheduled, executionID any
	if event.ScheduledAt != nil {
		scheduled = event.ScheduledAt.UTC().Format(time.RFC3339Nano)
	}
	if event.Execution != "" {
		executionID = event.Execution
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO schedule_events(workflow_name,revision_id,trigger_id,occurred_at,scheduled_at,code,execution_id,skipped) VALUES(?,?,?,?,?,?,?,?)", event.Workflow, event.Revision, event.Trigger, event.Time.UTC().Format(time.RFC3339Nano), scheduled, event.Code, executionID, event.Skipped)
	return err
}

// Schedules returns current definitions/cursors, including disabled workflows, without action payloads.
func (s *Store) Schedules(ctx context.Context, name string) (result []scheduler.Entry, err error) {
	if _, err = s.Get(ctx, name, ""); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+scheduleColumns+" FROM schedules s JOIN workflows w ON w.name=s.workflow_name WHERE s.workflow_name=? ORDER BY s.trigger_id LIMIT 17", name)
	if err != nil {
		return nil, err
	}
	result = []scheduler.Entry{}
	for rows.Next() {
		entry, e := scanSchedule(rows)
		if e != nil {
			return nil, errors.Join(e, rows.Close())
		}
		if len(result) >= 16 {
			return nil, errors.Join(execution.ErrLimit, rows.Close())
		}
		result = append(result, entry)
	}
	err = errors.Join(rows.Err(), rows.Close())
	return result, err
}

// ScheduleEvents reads newest-first bounded immutable schedule history across revisions.
func (s *Store) ScheduleEvents(ctx context.Context, name string, limit int) (history scheduler.History, err error) {
	if limit < 1 || limit > 1000 {
		return history, execution.ErrInvalid
	}
	if _, err = s.Get(ctx, name, ""); err != nil {
		return history, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT sequence,revision_id,trigger_id,occurred_at,scheduled_at,code,execution_id,skipped FROM schedule_events WHERE workflow_name=? ORDER BY sequence DESC LIMIT ?", name, limit+1)
	if err != nil {
		return history, err
	}
	history.Events = []scheduler.Event{}
	for rows.Next() {
		if len(history.Events) >= limit {
			history.HasMore = true
			break
		}
		event := scheduler.Event{Workflow: name}
		var stamp string
		var scheduled, id sql.NullString
		if err = rows.Scan(&event.Sequence, &event.Revision, &event.Trigger, &stamp, &scheduled, &event.Code, &id, &event.Skipped); err != nil {
			return history, errors.Join(err, rows.Close())
		}
		if event.Time, err = time.Parse(time.RFC3339Nano, stamp); err != nil {
			return history, errors.Join(execution.ErrInvalid, rows.Close())
		}
		if scheduled.Valid {
			at, parseErr := time.Parse(time.RFC3339Nano, scheduled.String)
			if parseErr != nil {
				return history, errors.Join(execution.ErrInvalid, rows.Close())
			}
			event.ScheduledAt = &at
		}
		event.Execution = id.String
		history.Events = append(history.Events, event)
	}
	err = errors.Join(rows.Err(), rows.Close())
	return history, err
}

// readScheduleOrigin verifies persisted receipt metadata against its immutable admission identity.
func readScheduleOrigin(ctx context.Context, q executionQuerier, snap *execution.Snapshot) error {
	if snap.Trigger == nil {
		return nil
	}
	var id, at string
	if err := q.QueryRowContext(ctx, "SELECT trigger_id,scheduled_at FROM scheduled_executions WHERE execution_id=? AND workflow_name=? AND revision_id=?", snap.ID, snap.Workflow, snap.Revision).Scan(&id, &at); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339Nano, at)
	if err != nil || snap.Trigger.Type != "cron" || snap.Trigger.ID != id || !snap.Trigger.ScheduledAt.Equal(parsed) {
		return execution.ErrInvalid
	}
	return nil
}
