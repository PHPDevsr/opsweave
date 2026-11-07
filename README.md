# OpsWeave

**Automate Infrastructure. Simplify Operations.**

OpsWeave is a lightweight, self-hosted infrastructure automation platform built
with Go. The product requirements and delivery roadmap are in [GOALS.md](GOALS.md).

## Development status

The current foundation provides a CLI with help, version information, JSON
version output, and shell completion. Workflow parsing, execution, storage,
configuration, the server, and `opsweave init` are planned features. The YAML
example in GOALS.md remains illustrative rather than a supported specification.

## Requirements

- Go **1.27.3 or newer**.
- Git to clone the repository.
- Network access to the Go module proxy and checksum database for initial
  dependency downloads.

The CLI uses `github.com/urfave/cli/v3` v3.14.0. Go Modules records the dependency
version and checksums in `go.mod` and `go.sum`.

## Build and quick start

```sh
git clone https://github.com/PHPDevsr/opsweave.git
cd opsweave
go mod download
go build -o bin/opsweave ./cmd/opsweave
./bin/opsweave --help
./bin/opsweave version
./bin/opsweave version --json
```

On Windows, build and run the executable from PowerShell:

```powershell
go build -o bin/opsweave.exe ./cmd/opsweave
.\bin\opsweave.exe --help
.\bin\opsweave.exe version
.\bin\opsweave.exe version --json
```

During development, commands can also run directly from source:

```sh
go run ./cmd/opsweave version
```

## CLI reference

| Command | Behavior |
| --- | --- |
| `opsweave` | Show root help. |
| `opsweave --help` | Show root help. |
| `opsweave help version` | Show version command help. |
| `opsweave version` | Print `opsweave version dev`. |
| `opsweave version --json` | Print `{"version":"dev"}` followed by a newline. |
| `opsweave completion <shell>` | Print a completion script for `bash`, `zsh`, `fish`, or `pwsh`. |

Successful commands return exit code `0`. Invalid commands, flags, extra
positional arguments, cancellation, and output failures return `1`. Normal output
goes to stdout; failures produce a single diagnostic on stderr when writable.
No configuration file, environment settings, database, or background service is
required by these commands. The development version is `dev`; a release version
injection mechanism is not implemented yet.

### Shell completion

For Bash or Zsh, source the generated script in the current shell:

```sh
source <(./bin/opsweave completion bash)
# For Zsh, use: source <(./bin/opsweave completion zsh)
```

For Fish:

```fish
./bin/opsweave completion fish > ~/.config/fish/completions/opsweave.fish
```

For PowerShell, generate and load the script:

```powershell
.\bin\opsweave.exe completion pwsh | Set-Content -Encoding utf8 .\opsweave.ps1
& .\opsweave.ps1
```

Add the applicable source/load command to your shell profile to enable completion
in future sessions. The `opsweave` binary must be on `PATH` when using completion
from an arbitrary directory.

## Architecture

OpsWeave starts as a modular monolith:

- `cmd/opsweave`: process entrypoint, interrupt context, and exit status.
- `internal/cli`: command definitions, input validation, and injected output
  streams; each invocation constructs an independent command tree.

Future domain, workflow parsing, engine, scheduler, trigger, action, worker,
storage, secrets, API, and observability components will have explicit boundaries.
Domain logic must remain independent of HTTP handlers and storage implementations.
SQLite is planned for single-node operation; PostgreSQL and distributed workers
come later. Redis and RabbitMQ are not required infrastructure.

## Development and CI

See [CONTRIBUTING.md](CONTRIBUTING.md) for local checks and contribution rules.

The `CI` workflow runs on pushes, pull requests, and manual dispatch. It checks
formatting, module consistency, builds, vet, tests, CLI smoke tests, and race
detection. Build, vet, and unit tests run on Linux and Windows with Go 1.27.3 and
the latest stable Go release. Race detection runs on Linux with CGO enabled.

The separate `CodeQL` workflow analyzes Go on pushes and pull requests targeting
`main`, manual dispatch, and Mondays at 03:00 UTC. It uses a manual Go build and
the standard CodeQL query suite, then uploads results to GitHub Code Scanning.
Repository code scanning must be available and advanced setup must be enabled;
disable an existing CodeQL default setup before using this advanced workflow.
Workflow execution and CodeQL uploads must be verified on GitHub after the files
are pushed. Repository branch protection is configured separately.

## Troubleshooting

- If dependency downloads fail with a DNS or proxy error, verify access to
  `proxy.golang.org` and `sum.golang.org` and review `go env GOPROXY`. Keep checksum
  verification enabled.
- If the race detector reports that CGO is disabled, enable CGO and provide a
  compatible C compiler on `PATH`. On Windows, a supported MinGW-w64 toolchain is
  required. Linux CI provides the race check independently of local tooling.
- Run `opsweave --help` to see the currently supported commands. Commands from the
  product roadmap are not available until their corresponding features ship.
