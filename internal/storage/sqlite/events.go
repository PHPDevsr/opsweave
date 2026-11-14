package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/PHPDevsr/opsweave/internal/event"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/schedule"
)

type eventRoute struct {
	sequence, epoch                   int64
	workflow, revision, trigger, kind string
}

func reconcileEventRoutes(ctx context.Context, tx *sql.Tx, name, revision string, triggers []schedule.Trigger) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM event_routes WHERE workflow_name=?", name); err != nil {
		return err
	}
	for _, t := range triggers {
		if t.Type == "cron" {
			continue
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO event_routes(workflow_name,revision_id,trigger_id,source_type,event_name) VALUES(?,?,?,?,?)", name, revision, t.ID, t.Type, t.Event); err != nil {
			return err
		}
	}
	return nil
}

const enabledRoutes = `SELECT r.sequence,w.automatic_generation,r.workflow_name,r.revision_id,r.trigger_id,r.source_type
 FROM event_routes r JOIN workflows w ON w.name=r.workflow_name AND w.current_revision=r.revision_id
 WHERE w.enabled=1 AND w.archived=0`

func routeBatch(ctx context.Context, tx *sql.Tx, query string, args ...any) (routes []eventRoute, err error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var r eventRoute
		if err := rows.Scan(&r.sequence, &r.epoch, &r.workflow, &r.revision, &r.trigger, &r.kind); err != nil {
			return nil, err
		}
		routes = append(routes, r)
	}
	return routes, rows.Err()
}

// EmitEvent atomically admits all enabled internal routes or one authenticated webhook route.
// Webhook callers supply the listener's fence. Local internal emissions use filesystem authorization.
func (s *Store) EmitEvent(ctx context.Context, request event.Request, cap execution.Capacity, fence *execution.Fence) (receipts []event.Receipt, err error) {
	if !event.ValidID(request.ID) || cap.Validate() != nil {
		return nil, execution.ErrInvalid
	}
	query := enabledRoutes
	var args []any
	switch request.Type {
	case "internal":
		if !event.ValidName(request.Name) || fence != nil {
			return nil, execution.ErrInvalid
		}
		request.Fingerprint = event.Digest([]byte(request.Name))
		query += " AND r.source_type='internal' AND r.event_name=? ORDER BY r.sequence LIMIT 65"
		args = []any{request.Name}
	case "webhook":
		if !event.ValidName(request.Workflow) || !event.ValidName(request.Trigger) || !event.ValidDigest(request.Fingerprint) || fence == nil {
			return nil, execution.ErrInvalid
		}
		query += " AND r.source_type='webhook' AND r.workflow_name=? AND r.trigger_id=? LIMIT 1"
		args = []any{request.Workflow, request.Trigger}
	default:
		return nil, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rollback(tx, &err)
	if fence != nil {
		if err = checkFence(ctx, tx, *fence, ""); err != nil {
			return nil, err
		}
	}
	routes, err := routeBatch(ctx, tx, query, args...)
	if err != nil {
		return nil, err
	}
	if len(routes) == 0 {
		return nil, event.ErrRoute
	}
	if len(routes) > 64 {
		return nil, event.ErrDestinations
	}
	now := time.Now().UTC()
	digest := event.Digest([]byte(request.ID))
	owner, err := opaqueID()
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "SAVEPOINT event_batch"); err != nil {
		return nil, err
	}
	for _, route := range routes {
		id, existing, err := existingEvent(ctx, tx, route, digest, request.Fingerprint, now)
		if err != nil {
			if errors.Is(err, event.ErrConflict) {
				return nil, rejectEventBatch(ctx, tx, route, "event_conflict", err, now)
			}
			return nil, err
		}
		if !existing {
			id, err = admitEvent(ctx, tx, route, owner, cap, now)
			if err != nil {
				if errors.Is(err, execution.ErrQueueFull) {
					return nil, rejectEventBatch(ctx, tx, route, "queue_full", err, now)
				}
				return nil, err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO event_dedup VALUES(?,?,?,?,?,?,?)
 ON CONFLICT(source_type,workflow_name,trigger_id,event_digest) DO UPDATE SET fingerprint=excluded.fingerprint,expires_at=excluded.expires_at,execution_id=excluded.execution_id`, route.kind, route.workflow, route.trigger, digest, request.Fingerprint, now.Add(7*24*time.Hour).Unix(), id)
			if err != nil {
				return nil, err
			}
		}
		receipts = append(receipts, event.Receipt{Execution: id, Workflow: route.workflow, Trigger: route.trigger})
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return receipts, nil
}

func rejectEventBatch(ctx context.Context, tx *sql.Tx, route eventRoute, code string, reason error, now time.Time) error {
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO event_batch"); err != nil {
		return err
	}
	if err := appendEventHistory(ctx, tx, route, code, "", now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return reason
}

// EventHistory returns recent immutable event outcomes for a workflow.
func (s *Store) EventHistory(ctx context.Context, workflow string, limit int) (history []event.HistoryEntry, err error) {
	if !event.ValidName(workflow) || limit < 1 || limit > 1000 {
		return nil, execution.ErrInvalid
	}
	if _, err = s.Get(ctx, workflow, ""); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT sequence,revision_id,trigger_id,source_type,occurred_at,code,coalesce(execution_id,'') FROM event_history WHERE workflow_name=? ORDER BY sequence DESC LIMIT ?", workflow, limit)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	history = []event.HistoryEntry{}
	for rows.Next() {
		var row event.HistoryEntry
		var at string
		if err = rows.Scan(&row.Sequence, &row.Revision, &row.Trigger, &row.Type, &at, &row.Code, &row.Execution); err != nil {
			return nil, err
		}
		row.OccurredAt, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, err
		}
		history = append(history, row)
	}
	return history, rows.Err()
}

func existingEvent(ctx context.Context, tx *sql.Tx, r eventRoute, digest, fingerprint string, now time.Time) (string, bool, error) {
	var id, stored string
	var expires int64
	err := tx.QueryRowContext(ctx, "SELECT execution_id,fingerprint,expires_at FROM event_dedup WHERE source_type=? AND workflow_name=? AND trigger_id=? AND event_digest=?", r.kind, r.workflow, r.trigger, digest).Scan(&id, &stored, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if expires <= now.Unix() {
		return "", false, nil
	}
	if stored != fingerprint {
		return "", false, event.ErrConflict
	}
	return id, true, nil
}

func admitEvent(ctx context.Context, tx *sql.Tx, r eventRoute, owner string, cap execution.Capacity, now time.Time) (string, error) {
	record, err := get(ctx, tx, r.workflow, r.revision)
	if err != nil {
		return "", err
	}
	if !record.Enabled || record.Archived {
		return "", event.ErrRoute
	}
	result, err := admitInTransaction(ctx, tx, record, execution.AdmissionOptions{Owner: owner}, &cap, now)
	if err != nil {
		return "", err
	}
	snap := result.Snapshot
	if _, err = tx.ExecContext(ctx, "INSERT INTO event_executions VALUES(?,?,?,?,?,?)", snap.ID, r.workflow, r.revision, r.trigger, r.kind, r.epoch); err != nil {
		return "", err
	}
	snap.Trigger = &execution.TriggerOrigin{Type: r.kind, ID: r.trigger}
	body, err := snapshotBody(snap)
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE executions SET body=? WHERE id=?", body, snap.ID); err != nil {
		return "", err
	}
	return snap.ID, appendEventHistory(ctx, tx, r, "admitted", snap.ID, now)
}

func appendEventHistory(ctx context.Context, tx *sql.Tx, r eventRoute, code, id string, now time.Time) error {
	var linked any
	if id != "" {
		linked = id
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO event_history(workflow_name,revision_id,trigger_id,source_type,occurred_at,code,execution_id) VALUES(?,?,?,?,?,?,?)", r.workflow, r.revision, r.trigger, r.kind, now.Format(time.RFC3339Nano), code, linked)
	return err
}

// BeginStartup fixes the scan boundary for a recovered server generation.
func (s *Store) BeginStartup(ctx context.Context, f execution.Fence) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx, &err)
	if err = checkFence(ctx, tx, f, ""); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO startup_scan(singleton,generation,cursor,upper_bound,started_at)
 SELECT 1,?,0,coalesce(max(sequence),0),? FROM event_routes WHERE source_type='startup'
 ON CONFLICT(singleton) DO UPDATE SET generation=excluded.generation,cursor=0,upper_bound=excluded.upper_bound,started_at=excluded.started_at,deferred_sequence=0 WHERE startup_scan.generation<>excluded.generation`, f.Generation, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}

// AdvanceStartup admits at most one startup occurrence and durably advances the cursor.
func (s *Store) AdvanceStartup(ctx context.Context, f execution.Fence, cap execution.Capacity) (done bool, err error) {
	if cap.Validate() != nil {
		return false, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer rollback(tx, &err)
	if err = checkFence(ctx, tx, f, ""); err != nil {
		return false, err
	}
	var cursor, upper, deferred int64
	var started string
	if err = tx.QueryRowContext(ctx, "SELECT cursor,upper_bound,deferred_sequence,started_at FROM startup_scan WHERE generation=?", f.Generation).Scan(&cursor, &upper, &deferred, &started); err != nil {
		return false, err
	}
	if cursor >= upper {
		return true, tx.Commit()
	}
	routes, err := routeBatch(ctx, tx, enabledRoutes+" AND r.source_type='startup' AND r.sequence>? AND r.sequence<=? AND julianday(w.updated_at)<=julianday(?) ORDER BY r.sequence LIMIT 1", cursor, upper, started)
	if err != nil {
		return false, err
	}
	if len(routes) == 0 {
		cursor = upper
	} else {
		r := routes[0]
		now := time.Now().UTC()
		if _, err = admitEvent(ctx, tx, r, f.Owner, cap, now); errors.Is(err, execution.ErrQueueFull) {
			if deferred != r.sequence {
				if err = appendEventHistory(ctx, tx, r, "queue_full", "", now); err != nil {
					return false, err
				}
				if _, err = tx.ExecContext(ctx, "UPDATE startup_scan SET deferred_sequence=? WHERE generation=?", r.sequence, f.Generation); err != nil {
					return false, err
				}
			}
			return false, tx.Commit()
		} else if err != nil {
			return false, err
		}
		cursor = r.sequence
	}
	if _, err = tx.ExecContext(ctx, "UPDATE startup_scan SET cursor=?,deferred_sequence=0 WHERE generation=?", cursor, f.Generation); err != nil {
		return false, err
	}
	return cursor >= upper, tx.Commit()
}
