# OpsWeave

**Automate Infrastructure. Simplify Operations.**

OpsWeave is a lightweight, self-hosted infrastructure automation platform built
with Go. The product requirements and delivery roadmap are in [GOALS.md](GOALS.md).

## Development status

The current foundation provides configuration/init, strict workflow validation,
planning, a revision registry, single-node SQLite and workflow administration CLI,
alongside help, version information, JSON output and shell completion. See the
[foundation guide](docs/foundation.md) and [workflow parsing](docs/workflow-parsing.md).
The production action catalog is empty: real workflow registration requires later
action implementations. The [manual execution engine and history CLI](docs/execution.md)
are implemented with test-only action injection; production execution still needs
those future integrations. The server and automatic triggers remain planned.
The GOALS.md example remains illustrative.

Phase 01 and both [workflow](harness/designs/workflow-v1.md) and
[storage/configuration/CLI](harness/designs/foundation-storage-cli.md) contracts
are approved. Phase 01 is merged; Phase 02 and its [manual execution contract](harness/designs/manual-execution.md)
are approved for implementation. Phase acceptance depends on verification and
review; Phases 03–10 remain unapproved. See [PLANS.md](PLANS.md) for current status.

## Requirements

For published binaries, follow [release installation](docs/install.md); users do
not need a Go toolchain. The [release workflow](docs/releases.md) produces
Linux, Windows and macOS archives for amd64/arm64 after an intentional version tag.
No release tag is created merely by merging that workflow.

- Go **1.27.3 or newer**.
- Git to clone the repository.
- Network access to the Go module proxy and checksum database for initial
  dependency downloads.

Editing Markdown uses Bun 1.4.4, pinned in `.bun-version`, with the built-in
`bun fmt` formatter. The Go binary does not require Bun or Node.js.

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

| Command                       | Behavior                                                        |
| ----------------------------- | --------------------------------------------------------------- |
| `opsweave`                    | Show root help.                                                 |
| `opsweave --help`             | Show root help.                                                 |
| `opsweave help version`       | Show version command help.                                      |
| `opsweave version`            | Print `opsweave version dev`.                                   |
| `opsweave version --json`     | Print `{"version":"dev"}` followed by a newline.                |
| `opsweave completion <shell>` | Print a completion script for `bash`, `zsh`, `fish`, or `pwsh`. |

Successful commands return exit code `0`. Invalid commands, flags, extra
positional arguments, cancellation, and output failures return `1`. Normal output
goes to stdout; failures produce a single diagnostic on stderr when writable.
No configuration file, environment settings, database, or background service is
required by the help/version/completion commands above. Administration commands
and their configuration are documented in the [foundation guide](docs/foundation.md).
Source builds report `dev`; release archives report their validated Git tag through
the linker's version assignment.

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
- `internal/workflow`: bounded YAML/schema parsing, condition syntax, dependency
  and reference validation, normalized defaults and phase capability checks.
- `internal/yamlutil`: shared bounded YAML loading with safe diagnostics.
- `internal/config`: strict YAML/environment settings and exclusive init.
- `internal/registry`: catalog/schema preflight, batch input loading, normalization
  and SQL-independent repository contracts.
- `internal/storage/sqlite`: migrations, immutable revisions, atomic batch writes
  and workflow lifecycle persistence.

Future engine, scheduler, trigger, action, worker,
secrets, API, and observability components will have explicit boundaries.
Domain logic must remain independent of HTTP handlers and storage implementations.
SQLite supports single-node operation; PostgreSQL and distributed workers
come later. Redis and RabbitMQ are not required infrastructure.

## Development and CI

See [CONTRIBUTING.md](CONTRIBUTING.md) for local checks and contribution rules.

See [foundation benchmarks](docs/benchmarks.md) for parser, preflight and SQLite
measurements. The dedicated `Benchmarks` workflow runs them for pull requests and
`main`, and uploads allocation/timing reports with environment metadata.

The workflows run the checks relevant to changed files:

| Workflow     | Push and pull request triggers                                                              | Checks                                                                                 |
| ------------ | ------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| `CI`         | Go files, `go.mod`, `go.sum`, files under `testdata` or `migrations`, or the Go CI workflow | Go format, build, vet, tests, module consistency, CLI smoke tests, and race detection. |
| `Benchmarks` | Changes under `internal`, module files, or benchmark workflow; PRs and pushes to `main`     | Parser/expression/preflight/SQLite timing, allocations and environment artifacts.      |
| `Markdown`   | Any `.md` file, formatter configuration, `.bun-version`, or the Markdown workflow           | Native `bun fmt --check` with the pinned Bun version.                                  |

All three workflows above support manual dispatch. Build, vet, and unit tests run on
Linux and Windows with Go 1.27.3 and the latest stable Go release. Race detection
runs on Linux with CGO enabled. External GitHub Actions use full commit SHA pins
with release-tag comments. Dependabot checks Actions and Go Modules weekly. Bun version updates are reviewed
through `.bun-version`.

Check or fix Markdown formatting locally:

```sh
bun --version
bun fmt --check "**/*.md"
bun fmt "**/*.md"
```

Use the version in `.bun-version`; no package installation is needed. The existing
`.prettierrc.json` and `.prettierignore` retain the formatting/ignore rules and are
read natively by Bun. The CI check reports differences without changing files. After a
documentation-only change, the Go format and test workflow is not triggered.

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
