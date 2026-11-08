# Phase 10 — Production qualification

Milestone: v0.9.x–v1.0.0. Requirement status: `PLANNED`. Review state: NOT_APPROVED.

Explicit exception, 2026-11-08 (Asia/Jakarta): the user requested multi-OS release
CI before Phase 03 so users can download binaries. Only binary packaging,
version injection, release automation and installation documentation are
authorized early; this does not approve the rest of Phase 10 or Phase 03. See
[release procedure](../../docs/releases.md) and build-log evidence. Requirement
`G-13-011` is in progress for this scoped extension, not completed.

## Goal, ownership and prerequisites

Deliver production qualification with evidence for every owned row in the [requirement ledger](../requirements.md). Ownership includes the exact source text and context; shared rules in phases 00 and 02 apply wherever relevant.

Prerequisites: All phases 00, 01, 02, 03, 04, 05, 06, 07, 08, 09; immediate gate: phase 09 accepted.

Affected boundaries: Release/deployment pipeline, operations documentation, whole-system security/performance/compatibility qualification.

Owned IDs: `G-5.10-011`, `G-5.15-014`, `G-5.16-002`, `G-5.16-003`, `G-5.16-012`, `G-5.16-013`, `G-5.16-014`, `G-5.16-015`, `G-11-010`, `G-11-011`, `G-13-001`, `G-13-002`, `G-13-003`, `G-13-004`, `G-13-005`, `G-13-006`, `G-13-007`, `G-13-008`, `G-13-009`, `G-13-010`, `G-13-011`, `G-13-012`, `G-13-013`

## Ordered implementation tasks

1. Audit the complete ledger and close feature evidence gaps; run dependency vulnerability scanning and security review without deferring baseline controls to this phase.
2. Benchmark engine, scheduler, worker throughput and storage before any optimization; measure bounded memory/goroutines and preserve correctness.
3. Deliver/rehearse binary, Docker and Compose deployments, persistent volumes, resource limits, migration startup, health/readiness, backup/restore and upgrade/rollback.
4. Build Astro Starlight documentation from one source: exact tagged commits, preserved released outputs, distinct development docs, reproducible builds and generated latest-stable routing.
5. Verify release metadata/version injection and automated test/release pipelines; qualify v1.0 against every success criterion.

## Acceptance scenarios

- Full E2E suite covers valid/invalid/conditional workflows, retries/exhaustion, timeouts/cancellation, restart recovery and execution limits.
- Required unit/integration/race/fuzz/security/compatibility checks pass; core parser/engine/state-machine coverage meets 80% target without weakening assertions.
- Fresh install, upgrade, failed migration, restore and rollback rehearsals recover documented persistent data; no Redis/RabbitMQ required.
- Rebuild supported Git tags exactly; verify old published docs remain unchanged and default routing selects latest stable.
- Record benchmark environment/results, vulnerability dispositions and remote CI/release evidence before production completion.

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
