package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
)

const pruneBatchSize = 100

// PreviewPrune checks existing history without creating or migrating storage.
func PreviewPrune(ctx context.Context, path string, age time.Duration) (report PruneReport, err error) {
	db, err := openReadOnly(path)
	if err != nil {
		return report, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	return (&Store{db}).PruneExecutions(ctx, age, true)
}

// PruneReport counts executions by the first applicable eligibility reason.
type PruneReport struct {
	Eligible        int `json:"eligible"`
	Active          int `json:"active"`
	WithinRetention int `json:"withinRetention"`
	Protected       int `json:"protectedByReceipt"`
	Referenced      int `json:"referencedByReplay"`
	Deleted         int `json:"deleted"`
}

// Stored execution timestamps use UTC RFC3339Nano with optional fractional
// digits. Pad to nine digits for exact comparisons at age and expiry boundaries.
const pruneEligibility = `WITH classification AS (
 SELECT id, CASE
  WHEN state IN ('pending','queued','running') THEN 'active'
  WHEN julianday(json_extract(body,'$.updatedAt')) IS NULL
   OR substr(rtrim(json_extract(body,'$.updatedAt'),'Z')||'.',1,20)
      ||substr(substr(rtrim(json_extract(body,'$.updatedAt'),'Z'),21)||'000000000',1,9)>=? THEN 'recent'
  WHEN EXISTS (SELECT 1 FROM execution_admissions a WHERE a.execution_id=e.id
   AND (julianday(a.expires_at) IS NULL OR substr(rtrim(a.expires_at,'Z')||'.',1,20)
      ||substr(substr(rtrim(a.expires_at,'Z'),21)||'000000000',1,9)>?))
   OR EXISTS (SELECT 1 FROM event_dedup d WHERE d.execution_id=e.id AND d.expires_at>?) THEN 'receipt'
  WHEN EXISTS (SELECT 1 FROM execution_policies p WHERE p.parent_id=e.id) THEN 'replay'
  ELSE 'eligible' END AS reason
 FROM executions e
) `

// PruneExecutions previews or deletes up to 100 complete terminal records.
// Each call uses one consistent snapshot. Concurrent admissions are serialized
// with apply by the SQLite immediate transaction.
func (s *Store) PruneExecutions(ctx context.Context, age time.Duration, dryRun bool) (report PruneReport, err error) {
	return s.pruneExecutions(ctx, age, dryRun, time.Now().UTC())
}

func (s *Store) pruneExecutions(ctx context.Context, age time.Duration, dryRun bool, now time.Time) (report PruneReport, err error) {
	if age <= 0 {
		return report, execution.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: dryRun})
	if err != nil {
		return report, fmt.Errorf("begin execution prune: %w", err)
	}
	defer rollback(tx, &err)
	if dryRun {
		if err := checkSchema(ctx, tx); err != nil {
			return report, err
		}
	}
	const timestamp = "2006-01-02T15:04:05.000000000"
	args := []any{now.Add(-age).Format(timestamp), now.Format(timestamp), now.Unix()}
	rows, err := tx.QueryContext(ctx, pruneEligibility+`SELECT reason,count(*) FROM classification GROUP BY reason`, args...)
	if err != nil {
		return PruneReport{}, fmt.Errorf("classify execution history: %w", err)
	}
	counts := map[string]*int{"eligible": &report.Eligible, "active": &report.Active, "recent": &report.WithinRetention, "receipt": &report.Protected, "replay": &report.Referenced}
	for rows.Next() {
		var reason string
		var count int
		if err := rows.Scan(&reason, &count); err != nil {
			return PruneReport{}, errors.Join(err, rows.Close())
		}
		*counts[reason] = count
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return PruneReport{}, fmt.Errorf("count execution eligibility: %w", err)
	}
	if !dryRun && report.Eligible > 0 {
		if _, err = tx.ExecContext(ctx, pruneEligibility+`INSERT INTO execution_prune_targets
 SELECT id FROM classification WHERE reason='eligible' ORDER BY id LIMIT ?`, append(args, pruneBatchSize)...); err != nil {
			return PruneReport{}, fmt.Errorf("select execution prune batch: %w", err)
		}
		report.Deleted, err = deletePruneBatch(ctx, tx)
		if err != nil {
			return PruneReport{}, err
		}
		if report.Deleted != min(report.Eligible, pruneBatchSize) {
			return PruneReport{}, execution.ErrConflict
		}
	}
	if err = tx.Commit(); err != nil {
		return PruneReport{}, fmt.Errorf("commit execution prune: %w", err)
	}
	return report, nil
}

func deletePruneBatch(ctx context.Context, tx *sql.Tx) (int, error) {
	const target = ` IN (SELECT id FROM execution_prune_targets)`
	for _, query := range []string{
		`INSERT INTO pruned_schedule_occurrences SELECT workflow_name,revision_id,trigger_id,scheduled_at FROM scheduled_executions WHERE execution_id` + target,
		`DELETE FROM event_dedup WHERE execution_id` + target,
		`DELETE FROM event_history WHERE execution_id` + target,
		`DELETE FROM event_executions WHERE execution_id` + target,
		`DELETE FROM schedule_events WHERE execution_id` + target,
		`DELETE FROM scheduled_executions WHERE execution_id` + target,
		`DELETE FROM execution_queue WHERE execution_id` + target,
		`DELETE FROM execution_admissions WHERE execution_id` + target,
		`DELETE FROM execution_policies WHERE execution_id` + target,
		`DELETE FROM execution_attempts WHERE execution_id` + target,
		`DELETE FROM execution_events WHERE execution_id` + target,
		`DELETE FROM execution_steps WHERE execution_id` + target,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return 0, fmt.Errorf("delete execution history batch: %w", err)
		}
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM executions WHERE id`+target)
	if err != nil {
		return 0, fmt.Errorf("delete execution batch: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM execution_prune_targets`); err != nil {
		return 0, fmt.Errorf("clear execution prune targets: %w", err)
	}
	return int(count), nil
}
