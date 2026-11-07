# Contributing to OpsWeave

Read [AGENTS.md](AGENTS.md) and [GOALS.md](GOALS.md) before making changes. Keep
changes focused, preserve existing behavior, and add tests and English
documentation for new behavior. Planned product requirements are not completed
merely because code exists.

## Toolchain and dependencies

Use Go 1.27.3 or newer, as declared in `go.mod`. CI tests the minimum supported Go
version and the latest stable version on Linux and Windows. Keep `go.mod` and
`go.sum` consistent, prefer the standard library, and justify new dependencies.
Use `go mod tidy` after dependency changes; do not hand-edit checksums or disable
checksum verification.

## Required checks

Run these commands from the repository root:

```sh
gofmt -l .
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go mod verify
go build ./...
```

`gofmt -l .` must produce no output. Apply formatting with `gofmt -w cmd internal`
when needed; the CI format check only reports differences and never rewrites files.
Race tests require CGO and a compatible C compiler. Report any check you cannot
run and its reason rather than claiming success.

Check module consistency after dependency changes:

```sh
go mod tidy
git diff --exit-code -- go.mod go.sum
```

If module files intentionally changed, review and include those changes before
running the consistency check against the committed state.

For CLI smoke tests, build the binary using the README instructions and run help,
text/JSON version output, all four shell completion generators, and an unknown
command. Verify the unknown command returns `1` and prints one diagnostic to stderr.
CI repeats these CLI smoke tests on both supported operating systems.

Validate changes to GitHub Actions workflows with
[actionlint](https://github.com/rhysd/actionlint):

```sh
actionlint
```

## Testing conventions

Test observable behavior, invalid inputs, failures, and boundaries. Prefer
deterministic, table-driven tests and standard-library assertions. Use temporary
directories, isolated resources, context deadlines, and cleanup functions. Use
parallel tests only when their state is independent. Do not weaken security
controls or remove meaningful assertions to make a check pass.

## Architecture and security

Keep the entrypoint small and application components under `internal` until a
public API is needed. Introduce packages and interfaces for implemented behavior,
not placeholders for every roadmap item. Pass contexts across cancellable
boundaries, inject infrastructure dependencies where needed, and avoid global
mutable state. Future goroutines must have ownership and a bounded lifetime.

Never commit credentials or real local configuration. Keep sensitive values out
of diagnostics and logs. Dangerous actions, persistence, and management endpoints
require their documented security controls before implementation is considered
complete.

## CI and CodeQL

The `CI` workflow contains format, cross-platform test, module consistency, and
Linux race jobs. The `CodeQL` workflow performs Go security analysis and uploads
results to GitHub Code Scanning. Both workflows have timeouts and cancel obsolete
runs for the same branch or pull request. CI has read-only repository permissions;
only the CodeQL analysis job receives `security-events: write`.

CodeQL uses advanced setup. A repository administrator must ensure code scanning
is available and disable conflicting default setup. Verify the first workflow
run and CodeQL upload after pushing. Local checks cannot establish that GitHub
permissions or repository settings allow an upload. Branch protection and rules
for blocking merges on findings are separate repository settings.

## Documentation and reviews

Maintain one English documentation source. Released documentation must be built
from exact Git release tags and preserve previously published versions; do not
create version-specific source copies. Documentation hosting and publishing are
planned work.

Before submitting a pull request, review the diff, include relevant tests and
documentation, explain compatibility or migration impacts, and list the checks
actually executed. Distinguish successful local checks from GitHub workflow
results that still need verification.
