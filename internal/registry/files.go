package registry

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/PHPDevsr/opsweave/internal/workflow"
)

// ReadSources loads one file or a flat directory without following symlinks.
func ReadSources(ctx context.Context, path string) ([]Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("workflow input is unavailable or a symlink")
	}
	paths := []string{path}
	if info.IsDir() {
		paths, err = directoryPaths(ctx, path)
		if err != nil {
			return nil, err
		}
	}
	if len(paths) == 0 {
		return nil, errors.New("workflow directory contains no definitions")
	}
	result := make([]Source, 0, len(paths))
	total := 0
	for _, name := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Recheck file identity after open to reject replacement symlinks.
		before, err := os.Lstat(name)
		if err != nil || !before.Mode().IsRegular() {
			return nil, errors.New("workflow input must be a regular file")
		}
		file, err := os.Open(name)
		if err != nil {
			return nil, errors.New("open workflow input failed")
		}
		after, statErr := file.Stat()
		if statErr != nil || !os.SameFile(before, after) {
			closeErr := file.Close()
			if closeErr != nil {
				return nil, errors.New("close changed workflow input failed")
			}
			return nil, errors.New("workflow input changed while opening")
		}
		data, readErr := io.ReadAll(io.LimitReader(file, workflow.MaxSourceBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return nil, errors.New("read workflow input failed")
		}
		total += len(data)
		if len(data) > workflow.MaxSourceBytes || total > 16<<20 {
			return nil, errors.New("workflow input exceeds size limit")
		}
		result = append(result, Source{data})
	}
	return result, nil
}

func directoryPaths(ctx context.Context, path string) (result []string, err error) {
	dir, err := os.Open(path)
	if err != nil {
		return nil, errors.New("open workflow directory failed")
	}
	defer func() {
		if closeErr := dir.Close(); closeErr != nil {
			err = errors.Join(err, errors.New("close workflow directory failed"))
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, readErr := dir.ReadDir(128)
		if readErr != nil && readErr != io.EOF {
			return nil, errors.New("read workflow directory failed")
		}
		for _, entry := range entries {
			ext := filepath.Ext(entry.Name())
			if ext != ".yaml" && ext != ".yml" {
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil, errors.New("workflow symlinks are unsupported")
			}
			if !entry.IsDir() {
				result = append(result, filepath.Join(path, entry.Name()))
			}
			if len(result) > 256 {
				return nil, errors.New("too many workflow files")
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	sort.Strings(result)
	return result, nil
}
