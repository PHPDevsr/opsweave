// Package config loads validated local file and environment settings.
package config

import (
	"context"
	"errors"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PHPDevsr/opsweave/internal/yamlutil"
	"go.yaml.in/yaml/v3"
)

// DefaultYAML is the configuration written by init, without credentials.
const DefaultYAML = "storage:\n  path: ./data/opsweave.db\nlogging:\n  level: info\n"

// Config contains only validated settings; StoragePath is absolute.
type Config struct {
	StoragePath string
	LogLevel    string
	Execution   execution.Capacity
}

// ValidStoragePath rejects driver options/URIs while permitting local paths.
func ValidStoragePath(path string) bool {
	if path == "" || strings.ContainsAny(path, "\x00?") || strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") || strings.Contains(path, "://") || strings.HasPrefix(strings.ToLower(path), "file:") || path == ":memory:" {
		return false
	}
	// A colon is allowed only in a Windows drive prefix.
	if filepath.VolumeName(path) != "" && !filepath.IsAbs(path) {
		return false
	}
	return !strings.Contains(strings.TrimPrefix(path, filepath.VolumeName(path)), ":")
}

// Load uses environment > file > defaults. A nonempty explicit path must exist.
// The caller supplies working directory and environment for invocation isolation.
func Load(ctx context.Context, path, cwd string, environ []string) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return Config{}, errors.New("resolve configuration directory failed")
	}
	c := Config{StoragePath: "./data/opsweave.db", LogLevel: "info", Execution: execution.DefaultCapacity()}
	explicit := path != ""
	if !explicit {
		path = "opsweave.yaml"
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	info, statErr := os.Stat(path)
	if statErr == nil && !info.Mode().IsRegular() {
		return Config{}, errors.New("configuration must be a regular file")
	}
	file, err := os.Open(path)
	if err == nil {
		data, readErr := io.ReadAll(io.LimitReader(file, (64<<10)+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return Config{}, errors.New("read configuration failed")
		}
		node, err := yamlutil.Load(ctx, data, 64<<10)
		if err != nil {
			return Config{}, err
		}
		if err := decode(node, &c); err != nil {
			return Config{}, err
		}
	} else if explicit || !errors.Is(err, os.ErrNotExist) {
		return Config{}, errors.New("open configuration failed")
	}
	for _, entry := range environ {
		key, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "OPSWEAVE_") {
			continue
		}
		switch key {
		case "OPSWEAVE_STORAGE_PATH":
			c.StoragePath = value
		case "OPSWEAVE_LOG_LEVEL":
			c.LogLevel = value
		case "OPSWEAVE_EXECUTION_WORKERS", "OPSWEAVE_EXECUTION_ACTIVE_STEPS", "OPSWEAVE_EXECUTION_RUNNING_EXECUTIONS", "OPSWEAVE_EXECUTION_QUEUED_EXECUTIONS", "OPSWEAVE_EXECUTION_QUEUE_TIMEOUT", "OPSWEAVE_EXECUTION_SHUTDOWN_TIMEOUT":
			if err := setExecution(&c.Execution, strings.ToLower(strings.TrimPrefix(key, "OPSWEAVE_EXECUTION_")), value); err != nil {
				return Config{}, err
			}
		default:
			return Config{}, errors.New("unknown OpsWeave environment setting")
		}
	}
	if err := c.Execution.Validate(); err != nil {
		return Config{}, errors.New("invalid execution configuration")
	}
	if !ValidStoragePath(c.StoragePath) {
		return Config{}, errors.New("invalid storage path")
	}
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return Config{}, errors.New("invalid logging level")
	}
	if !filepath.IsAbs(c.StoragePath) {
		c.StoragePath = filepath.Join(filepath.Dir(path), c.StoragePath)
	}
	return c, ctx.Err()
}

func decode(n *yaml.Node, c *Config) error {
	if n.Kind != yaml.MappingNode {
		return errors.New("configuration must be a mapping")
	}
	for i := 0; i < len(n.Content); i += 2 {
		key, group := n.Content[i].Value, n.Content[i+1]
		var field string
		switch key {
		case "storage":
			field = "path"
		case "logging":
			field = "level"
		case "execution":
			field = "execution"
		default:
			return errors.New("unknown configuration field")
		}
		if group.Kind != yaml.MappingNode {
			return errors.New("configuration section must be a mapping")
		}
		for j := 0; j < len(group.Content); j += 2 {
			v := group.Content[j+1]
			if key == "execution" {
				name := group.Content[j].Value
				tag := "!!int"
				if name == "queue_timeout" || name == "shutdown_timeout" {
					tag = "!!str"
				}
				if v.Kind != yaml.ScalarNode || v.Tag != tag {
					return errors.New("invalid execution configuration")
				}
				if err := setExecution(&c.Execution, name, v.Value); err != nil {
					return err
				}
				continue
			}
			if group.Content[j].Value != field || v.Kind != yaml.ScalarNode || v.Tag != "!!str" {
				return errors.New("invalid configuration field")
			}
			if key == "storage" {
				c.StoragePath = v.Value
			} else {
				c.LogLevel = v.Value
			}
		}
	}
	return nil
}

func setExecution(c *execution.Capacity, key, value string) error {
	if key == "queue_timeout" || key == "shutdown_timeout" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return errors.New("invalid execution duration")
		}
		if key == "queue_timeout" {
			c.QueueTimeout = duration
		} else {
			c.ShutdownTimeout = duration
		}
		return nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return errors.New("invalid execution capacity")
	}
	switch key {
	case "workers":
		c.Workers = n
	case "active_steps":
		c.ActiveSteps = n
	case "running_executions":
		c.RunningExecutions = n
	case "queued_executions":
		c.QueuedExecutions = n
	default:
		return errors.New("unknown execution configuration field")
	}
	return nil
}

// Init creates a config exclusively. Existing files and symlinks are preserved.
func Init(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("create configuration failed (target must not exist)")
	}
	_, writeErr := io.WriteString(file, DefaultYAML)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.New("write configuration failed; inspect the incomplete file")
	}
	return nil
}
