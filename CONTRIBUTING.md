# Contributing to OpsWeave

Read [AGENTS.md](AGENTS.md) and [GOALS.md](GOALS.md) before making changes. Keep
changes focused, preserve existing behavior, and add tests and English
documentation for new behavior. Planned product requirements are not completed
merely because code exists.

## Toolchain and dependencies

Use Go 1.27.4 or newer, as declared in `go.mod`. CI tests the minimum supported Go
version and the latest stable version on Linux and Windows. Keep `go.mod` and
`go.sum` consistent, prefer the standard library, and justify new dependencies.
Use `go mod tidy` after dependency changes; do not hand-edit checksums or disable
checksum verification.

## Required checks

Binary packaging and tag publication are documented in [release procedure](docs/releases.md).
Preview builds run on pull requests without publishing a release; use the
packager tests and extracted-binary smoke tests when changing that workflow.

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

Foundation tests use isolated SQLite files and injected validation-only catalogs.
Production has no built-in actions in Phase 01; do not wire test catalogs into
production to make registration succeed. See the [foundation guide](docs/foundation.md)
for configuration, capability limits, immutable revisions and stopped-process
backup/restore. Run bounded YAML/expression/configuration fuzzing when changing
these parsers; catalog and credential rejection tests must remain intact.

Validate changes to GitHub Actions workflows with
[actionlint](https://github.com/rhysd/actionlint):

```sh
actionlint
```

For Markdown changes, use the Bun version pinned in `.bun-version` (1.4.4) and
its built-in formatter to check all `.md` files:

```sh
bun --version
bun fmt --check "**/*.md"
```

Run `bun fmt "**/*.md"` to fix Markdown formatting before committing. No dependency
installation is needed. Bun reads the existing `.prettierrc.json` and
`.prettierignore` natively to preserve style; Go formatting still uses `gofmt`.
The OpsWeave binary does not require Bun or Node.js.

## Testing conventions

Run the [foundation benchmarks](docs/benchmarks.md) before performance changes.
The dedicated `Benchmarks` workflow captures timing/allocation results and runtime
metadata as downloadable artifacts. Keep durability/security settings unchanged
and compare repeated samples on comparable machines; benchmark errors fail CI,
while timing values establish an initial measurement baseline.

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
Linux race jobs. Its push and pull request path filters cover `**/*.go`, `go.mod`,
`go.sum`, `**/testdata/**`, `**/migrations/**`, and `.github/workflows/ci.yml`. Documentation-only
changes do not trigger these Go checks.

The separate `Markdown` workflow installs the pinned Bun version and runs
`bun fmt --check "**/*.md"`. It triggers when Markdown files, formatter rules,
`.bun-version`, or its own workflow change. Both workflows can also run manually.

The `CodeQL` workflow performs Go security analysis and uploads results to GitHub
Code Scanning. Its existing triggers are independent of the Go and Markdown
format checks. All workflows have timeouts and cancel obsolete runs for the same
branch or pull request. CI has read-only repository permissions; only the CodeQL
analysis job receives `security-events: write`.

Pin every external action to a full 40-character commit SHA and include its exact
release tag in a trailing comment, for example `# v7.0.1`. Verify that the SHA
belongs to the tagged release in the action's upstream repository before updating
it. Keep CodeQL init and analyze pinned to the same release.

Dependabot checks GitHub Actions and Go Modules weekly using
`.github/dependabot.yml`, with at most five open version-update pull requests per
ecosystem. It targets the default branch (`main`) and starts version-update checks
once this configuration is merged there. Review dependency updates and their CI
results before merging; preserve action SHA pins and release comments. Review
Bun runtime updates through `.bun-version` and verify native formatter output
before changing that pin.

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
