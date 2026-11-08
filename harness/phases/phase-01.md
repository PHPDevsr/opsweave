# Phase 01 — Workflow foundation

Milestone: v0.1.x. Requirement status: `IMPLEMENTED` with CodeQL configuration/phase acceptance still open; all 70 owned rows are `IMPLEMENTED`, none `COMPLETED`. Review state: APPROVED (2026-11-08, Asia/Jakarta; user reply "Ya, ACCEPT"). Phase completion: AWAITING_CODE_SCANNING.

Both [workflow](../designs/workflow-v1.md) and [foundation](../designs/foundation-storage-cli.md) contracts were accepted in separate "setuju" replies. All four ordered implementation tasks are prepared with tests, [documentation](../../docs/foundation.md) and [self-review](../reviews/phase-01.md). Sequential unit/integration, coverage, unsandboxed race and bounded fuzz checks passed. Both symlink security tests passed in the explicit Linux CI step on `f10f596`, alongside all six OS/Go matrix jobs and race testing. [PR #3](https://github.com/PHPDevsr/opsweave/pull/3) targets `main`; CodeQL upload remains blocked by repository Advanced Security configuration. No owned requirement is completed until quality gates/phase review close; phase 02 is not authorized.

## Goal, ownership and prerequisites

Deliver workflow foundation with evidence for every owned row in the [requirement ledger](../requirements.md). Ownership includes the exact source text and context; shared rules in phases 00 and 02 apply wherever relevant.

Prerequisites: All phases 00; immediate gate: phase 00 accepted.

Affected boundaries: Domain/workflow definitions, parser/validator, registry, configuration, CLI and SQLite adapters.

Owned IDs: `G-5.1-001`, `G-5.1-002`, `G-5.1-003`, `G-5.1-004`, `G-5.1-005`, `G-5.1-006`, `G-5.1-007`, `G-5.1-008`, `G-5.1-009`, `G-5.1-010`, `G-5.1-011`, `G-5.1-012`, `G-5.1-013`, `G-5.1-014`, `G-5.1-015`, `G-5.1-016`, `G-5.1-017`, `G-5.1-018`, `G-5.1-019`, `G-5.9-001`, `G-5.9-002`, `G-5.9-003`, `G-5.9-005`, `G-5.9-006`, `G-5.9-007`, `G-5.9-008`, `G-5.9-009`, `G-5.9-010`, `G-5.9-011`, `G-5.9-021`, `G-5.9-022`, `G-5.9-023`, `G-5.9-024`, `G-5.9-025`, `G-5.9-026`, `G-5.9-027`, `G-5.9-028`, `G-5.9-029`, `G-5.9-030`, `G-5.10-001`, `G-5.10-003`, `G-5.10-004`, `G-5.10-009`, `G-5.10-010`, `G-5.10-014`, `G-5.10-015`, `G-5.10-017`, `G-5.15-015`, `G-5.15-022`, `G-5.16-004`, `G-5.16-005`, `G-5.16-006`, `G-5.16-010`, `G-5.16-016`, `G-6-001`, `G-6-002`, `G-6-003`, `G-6-004`, `G-6-005`, `G-6-006`, `G-6-007`, `G-6-008`, `G-6-009`, `G-6-010`, `G-6-011`, `G-6-012`, `G-6-013`, `G-6-014`, `G-6-015`, `G-6-016`

## Ordered implementation tasks

1. Specify and review the full versioned YAML/expressions contract: dependencies, conditions, outputs, retries, concurrency, failures, timeouts, secret references and action compatibility. Define unsupported-feature rejection until its runtime phase ships.
2. Design minimal domain/repository boundaries and transactional migration/revision contracts; select and verify only the YAML/SQLite dependencies needed. Keep SQL outside domain logic.
3. Implement validated YAML/environment configuration and init, workflow parsing/validation, registry lifecycle, immutable revisions, metadata/labels, import/export and side-effect-free execution planning.
4. Wire validate/apply and workflow list/get/delete/enable/disable commands with documented text/JSON, flags, exits and completion; preserve version behavior.

## Acceptance scenarios

- Table-driven invalid syntax/schema/input tests, bounded YAML/expression/validator fuzzing, duplicate IDs, missing/cyclic dependencies, bad references and unknown actions/capabilities.
- Isolated SQLite tests cover migrations, transaction rollback, unique immutable revision identity, updates/archive/delete and import/export round trips.
- Config tests cover defaults, environment input, invalid/unknown critical settings and secret-free errors; dry-run causes no actions or durable executions.

## Security, documentation and compatibility

- Follow [AGENTS.md](../../AGENTS.md) and [GOALS.md](../../GOALS.md); keep domain logic independent of HTTP and SQL adapters. Introduce only packages needed for delivered behavior.
- Review new external inputs, permissions, resource bounds and secret handling before exposing each feature. Inject infrastructure dependencies, propagate contexts and persist validated state changes atomically.
- Update English reference documentation, examples and troubleshooting with the feature. Record schema migrations, rollback/data implications and compatibility evidence; if no migration/API change is needed, state that with a reason in the build log.
- Review contracts listed in the tasks before their implementations. No illustrative YAML or unreviewed API/schema/plugin design becomes a released promise. Inspect relevant release tags once they exist; keep one documentation source.

## Verification and review checklist

Run `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go mod verify`, `go build ./...`, `bun fmt --check "**/*.md"` and relevant phase acceptance suites. Add bounded fuzz runs for introduced parsers and isolated integration resources. Use deterministic synchronization and context deadlines. Record exact test/benchmark commands when introduced.

- [ ] Every owned row has exact test/review and documentation evidence attached; shared policy obligations audited.
- [ ] All applicable checks passed; unavailable checks remain explicit open gates.
- [ ] Security, compatibility and migration implications reviewed; source changes scoped.
- [ ] Append commands, exit codes, evidence, limitations and next steps to [build log](../build-log.md).
- [ ] Present results and obtain user acceptance before advancing. Do not treat code presence or this checklist as acceptance evidence.

## Foundation performance measurements

The user requested benchmarks and their CI before any next phase. See the
[benchmark guide](../../docs/benchmarks.md): seven families/17 cases cover parsing,
condition syntax, registry preflight and SQLite apply/get/list/migrations. The
`Benchmarks` workflow publishes three timing/allocation samples and environment
metadata for each relevant PR/main run. Fixtures are isolated and bounded; SQLite
production durability settings remain active. No engine/scheduler/worker benchmark
is claimed before those components exist. The CodeQL owner gate remains open.
