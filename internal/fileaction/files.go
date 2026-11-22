// Package fileaction supplies explicitly granted, handle-relative filesystem actions.
package fileaction

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/registry"
)

// Root grants actions on one administrator-controlled local directory tree.
type Root struct {
	Name           string
	Path           string
	AllowedActions []string
}

// Config defaults to disabled and a 16 MiB copy limit; zero means the default.
type Config struct {
	Enabled      bool
	Roots        []Root
	MaxCopyBytes int64
}

var errConfig = errors.New("invalid filesystem action configuration")
var errAction = errors.New("filesystem action failed")

func validName(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func validAction(s string) bool { return s == "file.exists" || s == "file.cleanup" || s == "file.copy" }

func validRoot(s string) bool {
	if !filepath.IsAbs(s) || !utf8.ValidString(s) || strings.HasPrefix(s, `\\`) || strings.HasPrefix(s, "//") || strings.Contains(strings.TrimPrefix(s, filepath.VolumeName(s)), ":") {
		return false
	}
	for _, c := range s {
		if c < 32 || c == 127 {
			return false
		}
	}
	info, err := os.Stat(s)
	return err == nil && info.IsDir()
}

func validPath(s string) bool {
	if len(s) == 0 || len(s) > 1024 || !utf8.ValidString(s) || strings.ContainsAny(s, "\\:<>\"|?*") {
		return false
	}
	parts := strings.Split(s, "/")
	if len(parts) > 32 {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.HasPrefix(strings.ToLower(part), ".opsweave-") {
			return false
		}
		for _, c := range part {
			if c < 32 || c == 127 {
				return false
			}
		}
		base, _, _ := strings.Cut(strings.ToUpper(part), ".")
		base = strings.TrimRight(base, " .")
		switch base {
		case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
			return false
		}
		if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return false
		}
		if base == "COM¹" || base == "COM²" || base == "COM³" || base == "LPT¹" || base == "LPT²" || base == "LPT³" {
			return false
		}
	}
	return true
}

// Validate checks all supplied roots/grants without reading contents or mutating files.
func (c Config) Validate() error {
	if len(c.Roots) > 128 || c.Enabled && len(c.Roots) == 0 || c.MaxCopyBytes != 0 && (c.MaxCopyBytes < 1<<20 || c.MaxCopyBytes > 64<<20) {
		return errConfig
	}
	seen := map[string]bool{}
	for _, root := range c.Roots {
		if !validName(root.Name) || !validRoot(root.Path) || seen[root.Name] || len(root.AllowedActions) == 0 || len(root.AllowedActions) > 3 {
			return errConfig
		}
		seen[root.Name] = true
		grants := map[string]bool{}
		for _, id := range root.AllowedActions {
			if !validAction(id) || grants[id] {
				return errConfig
			}
			grants[id] = true
		}
	}
	return nil
}

type rootPolicy struct {
	path   string
	grants map[string]bool
}

// Registrations snapshots root paths/grants and exposes only explicitly granted actions.
func Registrations(c Config) ([]action.Registration, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !c.Enabled {
		return nil, nil
	}
	roots := map[string]rootPolicy{}
	union := map[string]bool{}
	for _, r := range c.Roots {
		p := rootPolicy{path: r.Path, grants: map[string]bool{}}
		for _, id := range r.AllowedActions {
			p.grants[id] = true
			union[id] = true
		}
		roots[r.Name] = p
	}
	limit := c.MaxCopyBytes
	if limit == 0 {
		limit = 16 << 20
	}
	var entries []action.Registration
	for _, id := range []string{"file.exists", "file.cleanup", "file.copy"} {
		if !union[id] {
			continue
		}
		inputs := map[string]registry.Field{"root": {Type: "string", Required: true}, "path": {Type: "string", Required: true}}
		outputs := map[string]registry.Field{"exists": {Type: "boolean", Required: true}, "kind": {Type: "string", Required: true}}
		if id == "file.cleanup" {
			outputs = map[string]registry.Field{"removed": {Type: "boolean", Required: true}}
		}
		if id == "file.copy" {
			inputs = map[string]registry.Field{}
			for _, name := range []string{"source_root", "source", "destination_root", "destination"} {
				inputs[name] = registry.Field{Type: "string", Required: true}
			}
			outputs = map[string]registry.Field{"bytes": {Type: "integer", Required: true}}
		}
		entries = append(entries, action.Registration{Descriptor: registry.Descriptor{ID: id, Version: "1", APIVersion: "opsweave.io/v1", Permissions: []string{id}, Inputs: registry.Field{Type: "object", Properties: inputs}, Outputs: outputs}, Run: func(ctx context.Context, in map[string]any) (map[string]any, error) {
			return run(ctx, roots, limit, id, in)
		}})
	}
	return entries, nil
}

func selected(roots map[string]rootPolicy, id string, in map[string]any, aliasKey, pathKey string) (rootPolicy, string, error) {
	alias, ok := in[aliasKey].(string)
	if !ok {
		return rootPolicy{}, "", errAction
	}
	p, ok := roots[alias]
	name, isString := in[pathKey].(string)
	if !ok || !p.grants[id] || !isString || !validPath(name) {
		return rootPolicy{}, "", errAction
	}
	return p, name, nil
}

func run(parent context.Context, roots map[string]rootPolicy, limit int64, id string, in map[string]any) (output map[string]any, resultErr error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	defer func() {
		if resultErr != nil || ctx.Err() != nil {
			output = nil
			resultErr = errAction
		}
	}()
	aliasKey, pathKey := "root", "path"
	want := 2
	if id == "file.copy" {
		aliasKey, pathKey = "source_root", "source"
		want = 4
	}
	if len(in) != want || ctx.Err() != nil {
		return nil, errAction
	}
	p, name, err := selected(roots, id, in, aliasKey, pathKey)
	if err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(p.path)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, r.Close()) }()
	switch id {
	case "file.exists":
		info, err := r.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{"exists": false, "kind": "missing"}, nil
		}
		if err != nil {
			return nil, err
		}
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		} else if !info.Mode().IsRegular() {
			return nil, errAction
		}
		return map[string]any{"exists": true, "kind": kind}, nil
	case "file.cleanup":
		removed, err := removeRegular(ctx, r, name)
		if err != nil {
			return nil, err
		}
		return map[string]any{"removed": removed}, nil
	case "file.copy":
		destination, dest, err := selected(roots, id, in, "destination_root", "destination")
		if err != nil {
			return nil, err
		}
		dst, err := os.OpenRoot(destination.path)
		if err != nil {
			return nil, err
		}
		defer func() { resultErr = errors.Join(resultErr, dst.Close()) }()
		n, err := copyFile(ctx, r, name, dst, dest, limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"bytes": n}, nil
	}
	return nil, errAction
}

func openRegular(root *os.Root, name string) (file *os.File, resultErr error) {
	parent, err := root.OpenRoot(path.Dir(name))
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, parent.Close())
		if resultErr != nil && file != nil {
			resultErr = errors.Join(resultErr, file.Close())
			file = nil
		}
	}()
	base := path.Base(name)
	before, err := parent.Lstat(base)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errAction
	}
	f, err := openNoFollow(parent, base)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, errors.Join(err, errAction, f.Close())
	}
	return f, nil
}

func removeRegular(ctx context.Context, root *os.Root, name string) (removed bool, resultErr error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	parent, err := root.OpenRoot(path.Dir(name))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	base := path.Base(name)
	f, err := openRegular(parent, base)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { resultErr = errors.Join(resultErr, f.Close()) }()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	err = unlinkRegular(parent, base, f)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func copyFile(ctx context.Context, source *os.Root, name string, dest *os.Root, target string, limit int64) (count int64, resultErr error) {
	return copyFileWith(ctx, source, name, dest, target, limit, copyBounded)
}

// The transfer boundary permits isolated I/O failure tests with real roots,
// without a global hook or a second filesystem abstraction.
func copyFileWith(ctx context.Context, source *os.Root, name string, dest *os.Root, target string, limit int64, transfer func(context.Context, io.Writer, io.Reader, int64) (int64, error)) (count int64, resultErr error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	f, err := openRegular(source, name)
	if err != nil {
		return 0, err
	}
	defer func() { resultErr = errors.Join(resultErr, f.Close()) }()
	before, err := f.Stat()
	if err != nil || before.Size() > limit {
		return 0, errAction
	}
	parent, err := dest.OpenRoot(path.Dir(target))
	if err != nil {
		return 0, err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	base := path.Base(target)
	if _, err := parent.Lstat(base); !errors.Is(err, os.ErrNotExist) {
		return 0, errAction
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return 0, err
	}
	temp := ".opsweave-" + hex.EncodeToString(random[:])
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	out, err := parent.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return 0, err
	}
	defer func() {
		if out != nil {
			resultErr = errors.Join(resultErr, out.Close())
		}
		// Cleanup has its own finite context checks, without detaching I/O or
		// deleting a published destination when cleanup itself fails.
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer stop()
		_, err := removeRegular(cleanup, parent, temp)
		resultErr = errors.Join(resultErr, err)
	}()
	count, err = transfer(ctx, out, f, limit)
	if err != nil {
		return 0, err
	}
	after, err := f.Stat()
	if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || count != before.Size() {
		return 0, errAction
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := out.Sync(); err != nil {
		return 0, err
	}
	err = out.Close()
	out = nil
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := parent.Link(temp, base); err != nil {
		return 0, err
	}
	return count, nil
}

func copyBounded(ctx context.Context, dst io.Writer, src io.Reader, limit int64) (int64, error) {
	if limit < 0 {
		return 0, errAction
	}
	buffer := make([]byte, 32<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		read := min(int64(len(buffer)), limit-total+1)
		n, err := src.Read(buffer[:read])
		if n < 0 || int64(n) > read || int64(n) > limit-total {
			return total, errAction
		}
		if n > 0 {
			if err := ctx.Err(); err != nil {
				return total, err
			}
			written, writeErr := dst.Write(buffer[:n])
			if written < 0 || written > n {
				return total, errAction
			}
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, io.ErrNoProgress
		}
	}
}
