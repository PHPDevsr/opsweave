// Package sqlite implements the single-node registry repository with migrations.
package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/PHPDevsr/opsweave/internal/config"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/workflow"
	"modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store owns its connection pool. Call Close when its owning command exits.
type Store struct{ db *sql.DB }

var _ registry.Repository = (*Store)(nil)

// Open validates a local path and transactionally applies embedded migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !config.ValidStoragePath(path) {
		return nil, errors.New("invalid database path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.New("resolve database path failed")
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return nil, errors.New("create database directory failed")
	}
	file, err := os.OpenFile(abs, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		if err := file.Close(); err != nil {
			return nil, errors.New("close new database file failed")
		}
	} else if !errors.Is(err, os.ErrExist) {
		return nil, errors.New("create database file failed")
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("database must be a regular local file")
	}
	// Plain paths cannot contain query delimiters; all options are adapter-owned.
	connector, err := sqlite.NewConnector(abs + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=journal_mode(DELETE)&_txlock=immediate")
	if err != nil {
		return nil, errors.New("configure database connection failed")
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db}
	if err := s.migrate(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

// Close waits for owned database operations and releases resources.
func (s *Store) Close() error { return s.db.Close() }

// Health checks connectivity and the presence of a current migration record.
func (s *Store) Health(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		return fmt.Errorf("database health check: %w", err)
	}
	if count == 0 {
		return errors.New("database has no applied migrations")
	}
	return nil
}

func rollback(tx *sql.Tx, result *error) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		*result = errors.Join(*result, fmt.Errorf("rollback transaction: %w", err))
	}
}

func (s *Store) migrate(ctx context.Context) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer rollback(tx, &err)
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, checksum TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	rows, err := tx.QueryContext(ctx, "SELECT version,checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return fmt.Errorf("read migration ledger: %w", err)
	}
	versions := make(map[int]string)
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return errors.Join(err, rows.Close())
		}
		versions[version] = checksum
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("read migration versions: %w", err)
	}
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	if len(versions) > len(files) {
		return errors.New("database schema is newer than this binary")
	}
	for i, file := range files {
		version := i + 1
		data, err := migrations.ReadFile("migrations/" + file.Name())
		if err != nil {
			return fmt.Errorf("read migration: %w", err)
		}
		hash := sha256.Sum256(data)
		checksum := hex.EncodeToString(hash[:])
		if existing, applied := versions[version]; applied {
			if existing != checksum {
				return errors.New("migration checksum mismatch")
			}
			continue
		}
		if version <= len(versions) {
			return errors.New("migration history is incomplete")
		}
		// An unknown version must not be mistaken for an unapplied migration.
		for known := range versions {
			if known < 1 || known > len(files) {
				return errors.New("unsupported migration history")
			}
		}
		if _, err := tx.ExecContext(ctx, string(data)); err != nil {
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations VALUES(?,?,?)", version, checksum, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record migration: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

// Apply publishes every candidate in one transaction; archived names are reserved.
func (s *Store) Apply(ctx context.Context, candidates []registry.Candidate) (result []registry.Record, err error) {
	if len(candidates) == 0 || len(candidates) > 256 {
		return nil, errors.New("invalid registry batch")
	}
	seen := make(map[string]bool)
	total := 0
	for _, c := range candidates {
		total += len(c.Source)
		if total > 16<<20 {
			return nil, errors.New("registry batch exceeds source limit")
		}
		d, err := workflow.Parse(ctx, c.Source)
		if err != nil {
			return nil, err
		}
		policyErr := d.Phase01()
		if len(c.Capabilities) == 1 && c.Capabilities[0] == "serial_reliability" {
			policyErr = d.SerialReliability()
		} else if len(c.Capabilities) == 1 && c.Capabilities[0] == "dispatcher" {
			policyErr = d.DispatcherCapabilities()
		} else if len(c.Capabilities) > 0 {
			return nil, errors.New("unsupported registry capability")
		}
		if d.Metadata.Name != c.Name || seen[c.Name] || policyErr != nil || !json.Valid(c.Normalized) {
			return nil, errors.New("invalid registry candidate")
		}
		normalized, err := registry.Normalize(d)
		if err != nil || string(normalized) != string(c.Normalized) {
			return nil, errors.New("candidate normalization mismatch")
		}
		for _, step := range d.Spec.Steps {
			if c.Bindings[step.Uses] == "" {
				return nil, errors.New("candidate action binding missing")
			}
		}
		seen[c.Name] = true
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin apply: %w", err)
	}
	defer rollback(tx, &err)
	for _, c := range candidates {
		r, err := applyOne(ctx, tx, c)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit workflow batch: %w", err)
	}
	return result, nil
}

func applyOne(ctx context.Context, tx *sql.Tx, c registry.Candidate) (registry.Record, error) {
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	_, err := tx.ExecContext(ctx, "INSERT INTO workflows(name,enabled,archived,created_at,updated_at) VALUES(?,1,0,?,?) ON CONFLICT(name) DO NOTHING", c.Name, stamp, stamp)
	if err != nil {
		return registry.Record{}, fmt.Errorf("create workflow: %w", err)
	}
	var archived bool
	if err := tx.QueryRowContext(ctx, "SELECT archived FROM workflows WHERE name=?", c.Name).Scan(&archived); err != nil {
		return registry.Record{}, fmt.Errorf("read workflow lifecycle: %w", err)
	}
	if archived {
		return registry.Record{}, registry.ErrArchived
	}
	var number int
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(number),0)+1 FROM workflow_revisions WHERE workflow_name=?", c.Name).Scan(&number); err != nil {
		return registry.Record{}, fmt.Errorf("allocate revision: %w", err)
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return registry.Record{}, fmt.Errorf("generate revision identity: %w", err)
	}
	revision := hex.EncodeToString(id[:])
	bindings, err := json.Marshal(c.Bindings)
	if err != nil {
		return registry.Record{}, fmt.Errorf("encode action bindings: %w", err)
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO workflow_revisions(id,workflow_name,number,source,normalized,normalization_version,bindings,created_at) VALUES(?,?,?,?,?,1,?,?)", revision, c.Name, number, c.Source, string(c.Normalized), string(bindings), stamp)
	if err != nil {
		return registry.Record{}, fmt.Errorf("persist revision: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE workflows SET current_revision=?,updated_at=? WHERE name=?", revision, stamp, c.Name); err != nil {
		return registry.Record{}, fmt.Errorf("publish revision: %w", err)
	}
	return get(ctx, tx, c.Name, revision)
}

type querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const selectRecord = `SELECT w.name,r.id,r.number,w.enabled,w.archived,r.created_at,w.updated_at,r.source,r.normalized,r.normalization_version,r.bindings
FROM workflows w JOIN workflow_revisions r ON r.workflow_name=w.name`

func scan(row interface{ Scan(...any) error }) (registry.Record, error) {
	var r registry.Record
	var created, updated, normalized, bindings string
	var version int
	if err := row.Scan(&r.Name, &r.Revision, &r.Number, &r.Enabled, &r.Archived, &created, &updated, &r.Source, &normalized, &version, &bindings); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return r, registry.ErrNotFound
		}
		return r, fmt.Errorf("read workflow revision: %w", err)
	}
	if version != 1 || !json.Valid([]byte(normalized)) || !json.Valid([]byte(bindings)) {
		return r, errors.New("unsupported or corrupt stored definition")
	}
	var err error
	if r.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return r, errors.New("invalid stored timestamp")
	}
	if r.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return r, errors.New("invalid stored timestamp")
	}
	r.Definition, r.Bindings = json.RawMessage(normalized), json.RawMessage(bindings)
	return r, nil
}

func get(ctx context.Context, q querier, name, revision string) (registry.Record, error) {
	if revision == "" {
		return scan(q.QueryRowContext(ctx, selectRecord+" WHERE w.name=? AND r.id=w.current_revision", name))
	}
	return scan(q.QueryRowContext(ctx, selectRecord+" WHERE w.name=? AND r.id=?", name, revision))
}

// Get returns the current or exact historical revision, including archived history.
func (s *Store) Get(ctx context.Context, name, revision string) (registry.Record, error) {
	return get(ctx, s.db, name, revision)
}

func (s *Store) query(ctx context.Context, query string, args ...any) (result []registry.Record, err error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query workflows: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	result = []registry.Record{}
	bytesRead := 0
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return nil, err
		}
		bytesRead += len(r.Source) + len(r.Definition) + len(r.Bindings)
		if len(result) >= 10000 || bytesRead > 64<<20 {
			return nil, registry.ErrResultLimit
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// List orders workflows by name and excludes archived entries unless all is true.
func (s *Store) List(ctx context.Context, all bool) ([]registry.Record, error) {
	return s.query(ctx, selectRecord+" WHERE r.id=w.current_revision AND (? OR w.archived=0) ORDER BY w.name", all)
}

// Revisions returns all retained revisions in ascending revision number.
func (s *Store) Revisions(ctx context.Context, name string) ([]registry.Record, error) {
	result, err := s.query(ctx, selectRecord+" WHERE w.name=? ORDER BY r.number", name)
	if err == nil && len(result) == 0 {
		return nil, registry.ErrNotFound
	}
	return result, err
}

// SetEnabled updates registry state without changing revision content.
func (s *Store) SetEnabled(ctx context.Context, name string, enabled bool) (registry.Record, error) {
	return s.lifecycle(ctx, name, enabled, false)
}

// Archive reserves a workflow name and retains all immutable revision history.
func (s *Store) Archive(ctx context.Context, name string) (registry.Record, error) {
	return s.lifecycle(ctx, name, false, true)
}

func (s *Store) lifecycle(ctx context.Context, name string, enabled, archive bool) (result registry.Record, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin lifecycle update: %w", err)
	}
	defer rollback(tx, &err)
	r, err := get(ctx, tx, name, "")
	if err != nil {
		return result, err
	}
	if r.Archived {
		return result, registry.ErrArchived
	}
	if _, err := tx.ExecContext(ctx, "UPDATE workflows SET enabled=?,archived=?,updated_at=? WHERE name=?", enabled, archive, time.Now().UTC().Format(time.RFC3339Nano), name); err != nil {
		return result, fmt.Errorf("update workflow lifecycle: %w", err)
	}
	result, err = get(ctx, tx, name, "")
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return registry.Record{}, fmt.Errorf("commit lifecycle update: %w", err)
	}
	return result, nil
}
