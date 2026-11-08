// Package dispatchlock owns a native single-process lock for a canonical local database.
package dispatchlock

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/PHPDevsr/opsweave/internal/config"
	"github.com/PHPDevsr/opsweave/internal/execution"
)

// Lock must remain held until every dispatcher-owned invocation has ended.
type Lock struct {
	file *os.File
	path string
	once sync.Once
	err  error
}

// Acquire rejects hard links and unsafe sidecars and never waits for another owner.
// It creates a missing database, but does not open SQL or run migrations.
func Acquire(path string) (*Lock, error) {
	if !config.ValidStoragePath(path) {
		return nil, execution.ErrInvalid
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return nil, err
	}
	db, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	canonical, checkErr := canonicalFile(db, abs)
	if err = errors.Join(checkErr, db.Close()); err != nil {
		return nil, err
	}
	side := canonical + ".dispatcher.lock"
	f, err := os.OpenFile(side, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if _, err = canonicalFile(f, side); err == nil {
		err = tryLock(f)
	}
	if err == nil {
		if securityErr := privateSidecar(f, side); securityErr != nil {
			return nil, errors.Join(securityErr, unlock(f), f.Close())
		}
	}
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return &Lock{file: f, path: canonical}, nil
}

// Path returns the canonical database name protected by this lock.
func (l *Lock) Path() string { return l.path }

// Close releases ownership; callers must join their work first. The sidecar is never removed.
func (l *Lock) Close() error {
	l.once.Do(func() { l.err = errors.Join(unlock(l.file), l.file.Close()) })
	return l.err
}
