# Phase 02 — Durable manual execution

Milestone: v0.1.x. Requirement status: `PLANNED`. Review state: NOT_APPROVED.

## Goal, ownership and prerequisites

Deliver durable manual execution with evidence for every owned row in the [requirement ledger](../requirements.md). Ownership includes the exact source text and context; shared rules in phases 00 and 02 apply wherever relevant.

Prerequisites: All phases 00, 01; immediate gate: phase 01 accepted.

Affected boundaries: Dispatcher, domain state machine, engine, internal action executor, execution repositories and CLI.

Owned IDs: `G-5.1-020`, `G-5.1-021`, `G-5.2-001`, `G-5.2-002`, `G-5.2-003`, `G-5.2-005`, `G-5.2-007`, `G-5.2-008`, `G-5.2-015`, `G-5.2-016`, `G-5.2-017`, `G-5.2-020`, `G-5.2-024`, `G-5.2-025`, `G-5.2-026`, `G-5.2-027`, `G-5.2-028`, `G-5.2-029`, `G-5.2-030`, `G-5.2-031`, `G-5.2-032`, `G-5.2-033`, `G-5.2-034`, `G-5.2-035`, `G-5.2-036`, `G-5.2-037`, `G-5.2-038`, `G-5.2-039`, `G-5.2-040`, `G-5.2-046`, `G-5.2-047`, `G-5.2-052`, `G-5.3-001`, `G-5.5-001`, `G-5.5-002`, `G-5.5-003`, `G-5.5-004`, `G-5.5-005`, `G-5.5-006`, `G-5.5-007`, `G-5.5-008`, `G-5.5-009`, `G-5.5-010`, `G-5.5-011`, `G-5.5-012`, `G-5.5-015`, `G-5.5-017`, `G-5.5-022`, `G-5.5-026`, `G-5.5-030`, `G-5.5-035`, `G-5.5-036`, `G-5.5-037`, `G-5.5-038`, `G-5.5-039`, `G-5.5-040`, `G-5.5-041`, `G-5.9-012`, `G-5.9-013`, `G-5.9-014`, `G-5.9-015`, `G-5.10-005`, `G-5.10-006`, `G-5.10-016`, `G-5.15-005`, `G-5.15-021`, `G-5.16-001`, `G-11-002`

## Ordered implementation tasks

1. Review execution/step transition tables for every named state and design structured action input/output/errors with version, context, timeout, logging and permission obligations.
2. Persist execution identity and exact revision, validate/persist transitions transactionally and propagate storage errors before acknowledgement.
3. Implement manual trigger, sequential dependency execution, bounded outputs/logs, failed dependency reporting and durable history; use deterministic action doubles for engine tests.
4. Wire run and execution list/get/logs; expose single-binary local execution without a management listener. Reject capabilities whose reliability implementations are pending.

## Acceptance scenarios

- Execute a valid registered revision through dispatcher, engine and SQLite with deterministic action doubles; reject invalid definitions and transitions.
- Updating a workflow leaves historical execution definitions unchanged; reopen storage and read exact step/output/log history.
- Inject action and transaction failures; distinguish action errors from infrastructure errors; redact sensitive outputs and bound capture.
- Declare all required states now; prove operational cancellation/timeout/recovery behavior in phase 03 before enabling those policies.

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
