package sqlite

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/workflow"
)

var _ execution.Repository = (*Store)(nil)

func opaqueID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Submit atomically pins a preflighted current revision and claims single-node ownership.
func (s *Store) Submit(ctx context.Context, expected registry.Record, owner string) (result execution.Snapshot, err error) {
	if !execution.ValidID(owner) || !execution.ValidID(expected.Revision) {
		return result, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer rollback(tx, &err)
	current, err := get(ctx, tx, expected.Name, "")
	if err != nil {
		return result, err
	}
	if current.Archived {
		return result, registry.ErrArchived
	}
	if current.Revision != expected.Revision || !bytes.Equal(current.Source, expected.Source) || !bytes.Equal(current.Definition, expected.Definition) || !bytes.Equal(current.Bindings, expected.Bindings) {
		return result, execution.ErrConflict
	}
	d, err := workflow.Parse(ctx, current.Source)
	if err != nil {
		return result, err
	}
	if err := d.Phase01(); err != nil {
		return result, err
	}
	normalized, err := registry.Normalize(d)
	if err != nil {
		return result, err
	}
	if !bytes.Equal(normalized, current.Definition) {
		return result, execution.ErrInvalid
	}
	bindings := map[string]string{}
	if err := json.Unmarshal(current.Bindings, &bindings); err != nil {
		return result, err
	}
	var active int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM executions WHERE workflow_name=? AND state IN ('pending','queued','running')", current.Name).Scan(&active); err != nil {
		return result, err
	}
	if active != 0 {
		return result, execution.ErrConflict
	}
	id, err := opaqueID()
	if err != nil {
		return result, err
	}
	now := time.Now().UTC()
	result = execution.Snapshot{ID: id, Workflow: current.Name, Revision: current.Revision, State: execution.Running, Version: 0, CreatedAt: now, UpdatedAt: now, Steps: []execution.Step{}, Definition: current.Definition}
	for _, step := range d.Spec.Steps {
		version, ok := bindings[step.Uses]
		if !ok || version == "" {
			return execution.Snapshot{}, execution.ErrInvalid
		}
		result.Steps = append(result.Steps, execution.Step{ID: step.ID, Action: step.Uses, ActionVersion: version, State: execution.Pending})
	}
	body, err := snapshotBody(result)
	if err != nil {
		return execution.Snapshot{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO executions(id,workflow_name,revision_id,state,version,owner,created_at,body) VALUES(?,?,?,?,?,?,?,?)", id, current.Name, current.Revision, result.State, result.Version, owner, now.Format(time.RFC3339Nano), body); err != nil {
		return execution.Snapshot{}, err
	}
	for i, step := range result.Steps {
		b, err := json.Marshal(step)
		if err != nil {
			return execution.Snapshot{}, err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO execution_steps VALUES(?,?,?,?)", id, step.ID, i, string(b)); err != nil {
			return execution.Snapshot{}, err
		}
	}
	for _, state := range []execution.State{execution.Pending, execution.Queued, execution.Running} {
		if err := appendEvent(ctx, tx, execution.Event{Time: now, Execution: id, Code: "execution_state", State: state}); err != nil {
			return execution.Snapshot{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func snapshotBody(s execution.Snapshot) (string, error) {
	s.Steps = nil
	s.Definition = nil
	b, err := json.Marshal(s)
	return string(b), err
}

func appendEvent(ctx context.Context, tx *sql.Tx, e execution.Event) error {
	var limit int
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(sequence),0)+1,(SELECT COALESCE(MAX(event_limit),1024) FROM execution_policies WHERE execution_id=?) FROM execution_events WHERE execution_id=?", e.Execution, e.Execution).Scan(&e.Sequence, &limit); err != nil {
		return err
	}
	if e.Sequence > limit {
		return execution.ErrLimit
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if len(b) > 1024 {
		return execution.ErrLimit
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO execution_events VALUES(?,?,?)", e.Execution, e.Sequence, string(b))
	return err
}

// Change rechecks aggregate state and owner in the same transaction as state/output/event writes.
func (s *Store) Change(ctx context.Context, m execution.Mutation) (result execution.Snapshot, err error) {
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
	now := time.Now().UTC()
	if err := execution.Advance(&result, m, now); err != nil {
		return execution.Snapshot{}, err
	}
	body, err := snapshotBody(result)
	if err != nil {
		return execution.Snapshot{}, err
	}
	updated, err := tx.ExecContext(ctx, "UPDATE executions SET state=?,version=?,body=? WHERE id=? AND owner=? AND version=? AND state='running'", result.State, result.Version, body, m.ID, m.Owner, m.Version)
	if err != nil {
		return execution.Snapshot{}, err
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return execution.Snapshot{}, err
	}
	if count != 1 {
		return execution.Snapshot{}, execution.ErrConflict
	}
	if m.Step != "" {
		for _, step := range result.Steps {
			if step.ID == m.Step {
				b, err := json.Marshal(step)
				if err != nil {
					return execution.Snapshot{}, err
				}
				if _, err := tx.ExecContext(ctx, "UPDATE execution_steps SET body=? WHERE execution_id=? AND step_id=?", string(b), m.ID, m.Step); err != nil {
					return execution.Snapshot{}, err
				}
			}
		}
	}
	code := "execution_state"
	if m.Step != "" {
		code = "step_state"
	}
	if err := appendEvent(ctx, tx, execution.Event{Time: now, Execution: m.ID, Step: m.Step, Code: code, State: m.To, Reason: m.Reason}); err != nil {
		return execution.Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return execution.Snapshot{}, err
	}
	return result, nil
}

type executionQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readExecution(ctx context.Context, q executionQuerier, id string) (execution.Snapshot, error) {
	var result execution.Snapshot
	var body, workflowName, revision, state string
	var version int64
	if err := q.QueryRowContext(ctx, "SELECT body,workflow_name,revision_id,state,version FROM executions WHERE id=?", id).Scan(&body, &workflowName, &revision, &state, &version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, execution.ErrNotFound
		}
		return result, err
	}
	if len(body) > 65536 || json.Unmarshal([]byte(body), &result) != nil || result.ID != id || result.Workflow != workflowName || result.Revision != revision || string(result.State) != state || result.Version != version {
		return execution.Snapshot{}, execution.ErrInvalid
	}
	if result.Format != 0 && result.Format != 1 && result.Format != 2 {
		return execution.Snapshot{}, execution.ErrInvalid
	}
	var definition string
	if err := q.QueryRowContext(ctx, "SELECT normalized FROM workflow_revisions WHERE workflow_name=? AND id=?", workflowName, revision).Scan(&definition); err != nil {
		return execution.Snapshot{}, err
	}
	result.Definition = json.RawMessage(definition)
	rows, err := q.QueryContext(ctx, "SELECT step_id,body FROM execution_steps WHERE execution_id=? ORDER BY ordinal", id)
	if err != nil {
		return execution.Snapshot{}, err
	}
	result.Steps = []execution.Step{}
	total := len(result.Definition) + len(body)
	for rows.Next() {
		var step execution.Step
		var stepID, b string
		if err := rows.Scan(&stepID, &b); err != nil {
			return execution.Snapshot{}, errors.Join(err, rows.Close())
		}
		total += len(b)
		if len(result.Steps) >= 256 || total > 64<<20 {
			return execution.Snapshot{}, errors.Join(execution.ErrLimit, rows.Close())
		}
		if json.Unmarshal([]byte(b), &step) != nil || step.ID != stepID {
			return execution.Snapshot{}, errors.Join(execution.ErrInvalid, rows.Close())
		}
		result.Steps = append(result.Steps, step)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return execution.Snapshot{}, err
	}
	if result.Format == 2 {
		if err := readReliable(ctx, q, &result); err != nil {
			return execution.Snapshot{}, err
		}
	}
	return result, nil
}

// Execution reads one consistent snapshot and its exact historical definition.
func (s *Store) Execution(ctx context.Context, id string) (result execution.Snapshot, err error) {
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
		return execution.Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return execution.Snapshot{}, err
	}
	return result, nil
}

// Executions returns bounded summaries without definitions, outputs or ownership tokens.
func (s *Store) Executions(ctx context.Context) ([]execution.Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT body FROM executions ORDER BY created_at DESC,id DESC LIMIT 10001")
	if err != nil {
		return nil, err
	}
	result := []execution.Snapshot{}
	total := 0
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		total += len(b)
		if len(result) >= 10000 || total > 64<<20 {
			return nil, errors.Join(execution.ErrLimit, rows.Close())
		}
		var snap execution.Snapshot
		if err := json.Unmarshal([]byte(b), &snap); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		snap.Steps = nil
		snap.Definition = nil
		result = append(result, snap)
	}
	return result, errors.Join(rows.Err(), rows.Close())
}

// Events returns ordered, append-only lifecycle history without arbitrary action messages.
func (s *Store) Events(ctx context.Context, id string) ([]execution.Event, error) {
	if !execution.ValidID(id) {
		return nil, execution.ErrInvalid
	}
	var limit int
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(p.event_limit,1024) FROM executions e LEFT JOIN execution_policies p ON p.execution_id=e.id WHERE e.id=?", id).Scan(&limit); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, execution.ErrNotFound
		}
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT body FROM execution_events WHERE execution_id=? ORDER BY sequence LIMIT 16385", id)
	if err != nil {
		return nil, err
	}
	result := []execution.Event{}
	total := 0
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		total += len(b)
		if len(result) >= limit || total > 64<<20 {
			return nil, errors.Join(execution.ErrLimit, rows.Close())
		}
		var event execution.Event
		if err := json.Unmarshal([]byte(b), &event); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		if event.Execution != id || event.Sequence != len(result)+1 {
			return nil, errors.Join(execution.ErrInvalid, rows.Close())
		}
		result = append(result, event)
	}
	return result, errors.Join(rows.Err(), rows.Close())
}
