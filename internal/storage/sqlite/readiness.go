package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/PHPDevsr/opsweave/internal/config"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"modernc.org/sqlite"
)

// ServerState describes the dispatcher server lifecycle.
type ServerState string

const (
	ServerUnavailable ServerState = "unavailable"
	ServerReady       ServerState = "ready"
	ServerStopping    ServerState = "stopping"
)

// ServerStatus contains local lifecycle and capacity information from one snapshot.
type ServerStatus struct {
	State ServerState       `json:"state"`
	Pool  *WorkerPoolStatus `json:"pool,omitempty"`
}

// WorkerPoolStatus counts durable managed work for the current server.
type WorkerPoolStatus struct {
	Capacity   int `json:"capacity"`
	Busy       int `json:"busy"`
	QueueDepth int `json:"queueDepth"`
}

// PublishServerState updates the server lifecycle for the current fence only.
func (s *Store) PublishServerState(ctx context.Context, f execution.Fence, state ServerState, workers int) error {
	if f.Generation < 1 || !execution.ValidID(f.Owner) || (state != ServerReady && state != ServerStopping) || workers < 1 || workers > 64 {
		return execution.ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `UPDATE dispatcher SET server_state=?,worker_capacity=?
 WHERE singleton=1 AND generation=? AND owner=?`, state, workers, f.Generation, f.Owner)
	if err != nil {
		return fmt.Errorf("publish dispatcher lifecycle: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return execution.ErrConflict
	}
	return nil
}

// ReadServerStatus checks the exact migration ledger and reads the server status
// from one read-only snapshot. The caller verifies native lock ownership.
func ReadServerStatus(ctx context.Context, path string) (status ServerStatus, err error) {
	status.State = ServerUnavailable
	db, err := openReadOnly(path)
	if err != nil {
		return status, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return status, err
	}
	defer rollback(tx, &err)
	if err := checkSchema(ctx, tx); err != nil {
		return status, err
	}
	var owner string
	var generation int64
	var state ServerState
	var pool WorkerPoolStatus
	if err := tx.QueryRowContext(ctx, "SELECT generation,owner,server_state,worker_capacity FROM dispatcher WHERE singleton=1").Scan(&generation, &owner, &state, &pool.Capacity); err != nil {
		return status, err
	}
	if generation > 0 && execution.ValidID(owner) && (state == ServerReady || state == ServerStopping) {
		if pool.Capacity < 1 {
			return status, execution.ErrConflict
		}
		if err := tx.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM execution_attempts a JOIN execution_queue q ON q.execution_id=a.execution_id
  WHERE a.state='running' AND q.generation=?),
 (SELECT count(*) FROM executions e JOIN execution_queue q ON q.execution_id=e.id WHERE e.state='queued')`, generation).Scan(&pool.Busy, &pool.QueueDepth); err != nil {
			return status, err
		}
		if pool.Busy > pool.Capacity {
			return status, execution.ErrConflict
		}
		status = ServerStatus{State: state, Pool: &pool}
	}
	return status, tx.Commit()
}

func openReadOnly(path string) (*sql.DB, error) {
	if !config.ValidStoragePath(path) {
		return nil, execution.ErrInvalid
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, execution.ErrInvalid
	}
	uriPath := filepath.ToSlash(abs)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro&_pragma=busy_timeout(1000)"}
	connector, err := sqlite.NewConnector(uri.String())
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(connector), nil
}

func checkSchema(ctx context.Context, tx *sql.Tx) error {
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		return err
	}
	if count != len(files) {
		return execution.ErrUnavailable
	}
	for i, file := range files {
		var actual string
		if err := tx.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version=?", i+1).Scan(&actual); err != nil {
			return err
		}
		data, err := migrations.ReadFile("migrations/" + file.Name())
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		if actual != hex.EncodeToString(hash[:]) {
			return execution.ErrUnavailable
		}
	}
	return nil
}
