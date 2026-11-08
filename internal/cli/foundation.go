package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/PHPDevsr/opsweave/internal/config"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/storage/sqlite"
	"github.com/PHPDevsr/opsweave/internal/workflow"
	urfave "github.com/urfave/cli/v3"
)

func exactArguments(count int) func(context.Context, *urfave.Command) error {
	return func(_ context.Context, cmd *urfave.Command) error {
		if cmd.Args().Len() != count {
			return fmt.Errorf("%s requires %d arguments", cmd.Name, count)
		}
		return nil
	}
}

func jsonFlag() urfave.Flag { return &urfave.BoolFlag{Name: "json", Usage: "Print JSON"} }

func loadConfig(ctx context.Context, cmd *urfave.Command, diagnostics io.Writer) (config.Config, error) {
	if cmd.IsSet("config") && cmd.String("config") == "" {
		return config.Config{}, errors.New("configuration path must not be empty")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return config.Config{}, errors.New("resolve working directory failed")
	}
	cfg, err := config.Load(ctx, cmd.String("config"), cwd, os.Environ())
	if err == nil && cfg.LogLevel == "debug" {
		// Explicit debug mode emits one bounded structured event, never settings.
		handler := slog.NewJSONHandler(&outputWriter{writer: diagnostics}, &slog.HandlerOptions{Level: slog.LevelDebug})
		record := slog.NewRecord(time.Now(), slog.LevelDebug, "configuration loaded", 0)
		record.AddAttrs(slog.String("command", cmd.Name))
		if err := handler.Handle(ctx, record); err != nil {
			return config.Config{}, fmt.Errorf("write debug diagnostic: %w", err)
		}
	}
	return cfg, err
}

func foundationCommands(out, diagnostics io.Writer, catalog registry.Catalog) []*urfave.Command {
	commands := []*urfave.Command{{
		Name: "init", Usage: "Create configuration without overwriting an existing file",
		OnUsageError: usageError, ArgValidator: noArguments, Flags: []urfave.Flag{jsonFlag()},
		Action: func(ctx context.Context, cmd *urfave.Command) error {
			path := cmd.String("config")
			if cmd.IsSet("config") && path == "" {
				return errors.New("configuration path must not be empty")
			}
			if path == "" {
				path = "opsweave.yaml"
			}
			if err := config.Init(ctx, path); err != nil {
				return err
			}
			return writeResult(out, cmd.Bool("json"), map[string]string{"status": "created"})
		},
	}}
	for _, verb := range []string{"validate", "plan", "apply"} {
		commands = append(commands, &urfave.Command{
			Name: verb, Usage: verb + " workflow definitions",
			OnUsageError: usageError, ArgValidator: exactArguments(1), Flags: []urfave.Flag{jsonFlag()},
			Action: func(ctx context.Context, cmd *urfave.Command) error {
				cfg, err := loadConfig(ctx, cmd, diagnostics)
				if err != nil {
					return err
				}
				sources, err := registry.ReadSources(ctx, cmd.Args().First())
				if err != nil {
					return err
				}
				candidates, err := registry.PrepareReliable(ctx, sources, catalog)
				if err != nil {
					return err
				}
				if verb != "apply" {
					return writeResult(out, cmd.Bool("json"), candidates)
				}
				return withStore(ctx, cfg.StoragePath, func(store *sqlite.Store) error {
					records, err := store.Apply(ctx, candidates)
					if err != nil {
						return storageDiagnostic(err)
					}
					for i := range records {
						records[i].Definition, records[i].Bindings = nil, nil
					}
					return writeResult(out, cmd.Bool("json"), records)
				})
			},
		})
	}
	group := &urfave.Command{Name: "workflow", Usage: "Manage workflow definitions and immutable revisions", OnUsageError: usageError, ArgValidator: noArguments}
	for _, verb := range []string{"list", "get", "revisions", "export", "delete", "enable", "disable"} {
		flags := []urfave.Flag{}
		if verb != "export" {
			flags = append(flags, jsonFlag())
		}
		if verb == "get" || verb == "export" {
			flags = append(flags, &urfave.StringFlag{Name: "revision", Usage: "Exact historical revision ID"})
		}
		argc := 1
		if verb == "list" {
			argc = 0
			flags = append(flags, &urfave.BoolFlag{Name: "all", Usage: "Include archived workflows"})
		}
		group.Commands = append(group.Commands, &urfave.Command{
			Name: verb, Usage: verb + " workflow definitions", OnUsageError: usageError, ArgValidator: exactArguments(argc), Flags: flags,
			Action: func(ctx context.Context, cmd *urfave.Command) error {
				if cmd.IsSet("revision") && cmd.String("revision") == "" {
					return errors.New("revision must not be empty")
				}
				cfg, err := loadConfig(ctx, cmd, diagnostics)
				if err != nil {
					return err
				}
				return withStore(ctx, cfg.StoragePath, func(store *sqlite.Store) error {
					name := cmd.Args().First()
					var value any
					var err error
					switch verb {
					case "list":
						value, err = store.List(ctx, cmd.Bool("all"))
					case "get":
						value, err = store.Get(ctx, name, cmd.String("revision"))
					case "revisions":
						value, err = store.Revisions(ctx, name)
					case "export":
						var record registry.Record
						record, err = store.Get(ctx, name, cmd.String("revision"))
						if err == nil {
							_, err = out.Write(record.Source)
							if err != nil {
								return fmt.Errorf("write workflow export: %w", err)
							}
							return nil
						}
					case "delete":
						value, err = store.Archive(ctx, name)
					case "enable", "disable":
						value, err = store.SetEnabled(ctx, name, verb == "enable")
					}
					if err != nil {
						return storageDiagnostic(err)
					}
					if record, ok := value.(registry.Record); ok && verb != "get" {
						record.Definition, record.Bindings = nil, nil
						value = record
					}
					return writeResult(out, cmd.Bool("json"), value)
				})
			},
		})
	}
	commands = append(commands, group)
	return commands
}

func withStore(ctx context.Context, path string, fn func(*sqlite.Store) error) (err error) {
	store, err := sqlite.Open(ctx, filepath.Clean(path))
	if err != nil {
		return storageDiagnostic(err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			err = errors.Join(err, errors.New("close database failed"))
		}
	}()
	return fn(store)
}

func storageDiagnostic(err error) error {
	if errors.Is(err, registry.ErrNotFound) || errors.Is(err, registry.ErrArchived) || errors.Is(err, registry.ErrResultLimit) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var validation *workflow.ValidationError
	if errors.As(err, &validation) {
		return validation
	}
	return errors.New("database operation failed; check storage access, schema compatibility and backups")
}

func writeResult(out io.Writer, asJSON bool, value any) error {
	if asJSON {
		if err := json.NewEncoder(out).Encode(value); err != nil {
			return fmt.Errorf("write JSON output: %w", err)
		}
		return nil
	}
	writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	var err error
	switch v := value.(type) {
	case []registry.Record:
		_, err = fmt.Fprintln(writer, "NAME\tREVISION\tNUMBER\tENABLED\tARCHIVED")
		for _, row := range v {
			if err == nil {
				_, err = fmt.Fprintf(writer, "%s\t%s\t%d\t%t\t%t\n", row.Name, row.Revision, row.Number, row.Enabled, row.Archived)
			}
		}
	case registry.Record:
		_, err = fmt.Fprintf(writer, "Name: %s\nRevision: %s\nNumber: %d\nEnabled: %t\nArchived: %t\n", v.Name, v.Revision, v.Number, v.Enabled, v.Archived)
		if err == nil && len(v.Definition) > 0 {
			_, err = fmt.Fprintf(writer, "Definition: %s\n", v.Definition)
		}
	case []registry.Candidate:
		for _, candidate := range v {
			if err == nil {
				var bindings []byte
				bindings, err = json.Marshal(candidate.Bindings)
				if err == nil {
					_, err = fmt.Fprintf(writer, "%s\n  Order: %v\n  Actions: %s\n  Capabilities: %v\n", candidate.Name, candidate.Order, bindings, candidate.Capabilities)
				}
			}
		}
	default:
		_, err = fmt.Fprintln(writer, "Configuration created")
	}
	return errors.Join(err, writer.Flush())
}
