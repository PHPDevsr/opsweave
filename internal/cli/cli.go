// Package cli implements the OpsWeave command-line interface.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/registry"

	urfave "github.com/urfave/cli/v3"
)

// version is assigned only by the release linker; runtime code never changes it.
var version = "dev"

// Run executes a fresh command tree and returns a process exit code.
// Args includes the executable name, as in os.Args. Output and diagnostics are
// written only to stdout and stderr, respectively.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := execute(ctx, args, stdout, stderr); err != nil {
		if _, writeErr := fmt.Fprintf(stderr, "Error: %s\n", err); writeErr != nil {
			return 1
		}
		return 1
	}
	return 0
}

func execute(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return executeWithCatalog(ctx, args, stdout, stderr, registry.Catalog{})
}

func executeWithCatalog(ctx context.Context, args []string, stdout, stderr io.Writer, catalog registry.Catalog) error {
	return executeWithRuntime(ctx, args, stdout, stderr, catalog, nil)
}

func executeWithRuntime(ctx context.Context, args []string, stdout, stderr io.Writer, catalog registry.Catalog, runtime *action.Runtime) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	output := &outputWriter{writer: stdout}
	cmd := &urfave.Command{
		Name:                  "opsweave",
		Usage:                 "Automate infrastructure. Simplify operations.",
		Writer:                output,
		ErrWriter:             stderr,
		EnableShellCompletion: true,
		OnUsageError:          usageError,
		Flags:                 []urfave.Flag{&urfave.StringFlag{Name: "config", Usage: "Configuration file for administration commands"}},
		// The caller owns diagnostics and process termination, including errors
		// returned by the library's built-in commands.
		ExitErrHandler: func(context.Context, *urfave.Command, error) {},
		Action: func(ctx context.Context, cmd *urfave.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unknown command %q", cmd.Args().First())
			}
			return urfave.ShowRootCommandHelp(cmd)
		},
		Commands: []*urfave.Command{
			{
				Name:         "version",
				Usage:        "Show the OpsWeave version",
				OnUsageError: usageError,
				ArgValidator: noArguments,
				Flags: []urfave.Flag{
					&urfave.BoolFlag{Name: "json", Usage: "Print the version as JSON"},
				},
				Action: func(ctx context.Context, cmd *urfave.Command) error {
					if cmd.Bool("json") {
						if err := json.NewEncoder(output).Encode(struct {
							Version string `json:"version"`
						}{Version: version}); err != nil {
							return fmt.Errorf("write version: %w", err)
						}
						return nil
					}
					if _, err := fmt.Fprintf(output, "opsweave version %s\n", version); err != nil {
						return fmt.Errorf("write version: %w", err)
					}
					return nil
				},
			},
		},
		ConfigureShellCompletionCommand: func(cmd *urfave.Command) {
			cmd.Hidden = false
			cmd.ArgValidator = noArguments
			cmd.OnUsageError = usageError
			for _, shell := range cmd.Commands {
				shell.OnUsageError = usageError
			}
		},
	}
	cmd.Commands = append(cmd.Commands, foundationCommands(output, stderr, catalog)...)
	cmd.Commands = append(cmd.Commands, executionCommands(output, stderr, runtime)...)
	if err := cmd.Run(ctx, args); err != nil {
		return err
	}
	if output.err != nil {
		return fmt.Errorf("write output: %w", output.err)
	}
	return nil
}

func noArguments(_ context.Context, cmd *urfave.Command) error {
	if cmd.Args().Present() {
		return fmt.Errorf("%s does not accept arguments", cmd.Name)
	}
	return nil
}

func usageError(_ context.Context, _ *urfave.Command, err error, _ bool) error {
	return err
}

// outputWriter captures errors that built-in help and completion printers may
// discard, so a failed write cannot be reported as successful execution.
type outputWriter struct {
	writer io.Writer
	err    error
}

func (w *outputWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}
