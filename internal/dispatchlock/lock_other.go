//go:build !linux && !darwin && !windows

package dispatchlock

import (
	"github.com/PHPDevsr/opsweave/internal/execution"
	"os"
)

func canonicalFile(*os.File, string) (string, error) { return "", execution.ErrUnavailable }
func tryLock(*os.File) error                         { return execution.ErrUnavailable }
func unlock(*os.File) error                          { return execution.ErrUnavailable }
func privateSidecar(*os.File, string) error          { return execution.ErrUnavailable }
