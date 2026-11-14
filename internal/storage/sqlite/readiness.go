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

// PublishServerState updates the server lifecycle for the current fence only.
func (s *Store) PublishServerState(ctx context.Context, f execution.Fence, state ServerState) error {
	if f.Generation < 1 || !execution.ValidID(f.Owner) || (state != ServerReady && state != ServerStopping) {
		return execution.ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `UPDATE dispatcher SET server_state=?
 WHERE singleton=1 AND generation=? AND owner=?`, state, f.Generation, f.Owner)
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

// ReadServerState checks the exact migration ledger and reads the server state
// from one read-only snapshot. The caller verifies native lock ownership.
func ReadServerState(ctx context.Context, path string) (state ServerState, err error) {
	db, err := openReadOnly(path)
	if err != nil {
		return ServerUnavailable, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ServerUnavailable, err
	}
	defer rollback(tx, &err)
	if err := checkSchema(ctx, tx); err != nil {
		return ServerUnavailable, err
	}
	var owner string
	var generation int64
	if err := tx.QueryRowContext(ctx, "SELECT generation,owner,server_state FROM dispatcher WHERE singleton=1").Scan(&generation, &owner, &state); err != nil {
		return ServerUnavailable, err
	}
	if generation < 1 || !execution.ValidID(owner) || (state != ServerReady && state != ServerStopping) {
		state = ServerUnavailable
	}
	return state, tx.Commit()
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
