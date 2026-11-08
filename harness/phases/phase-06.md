# Phase 06 — Observability and notifications

Milestone: v0.5.x. Requirement status: `PLANNED`. Review state: NOT_APPROVED.

## Goal, ownership and prerequisites

Deliver observability and notifications with evidence for every owned row in the [requirement ledger](../requirements.md). Ownership includes the exact source text and context; shared rules in phases 00 and 02 apply wherever relevant.

Prerequisites: All phases 00, 01, 02, 03, 04, 05; immediate gate: phase 05 accepted.

Affected boundaries: Structured log capture, telemetry instrumentation, Prometheus/tracing endpoints and notification delivery policies.

Owned IDs: `G-5.7-001`, `G-5.7-002`, `G-5.7-003`, `G-5.7-004`, `G-5.7-005`, `G-5.7-006`, `G-5.7-007`, `G-5.7-008`, `G-5.7-009`, `G-5.7-010`, `G-5.7-011`, `G-5.7-012`, `G-5.7-013`, `G-5.7-014`, `G-5.7-015`, `G-5.7-016`, `G-5.7-017`, `G-5.12-001`, `G-5.12-002`, `G-5.12-003`, `G-5.12-004`, `G-5.12-005`, `G-5.12-006`, `G-5.12-007`, `G-5.12-008`, `G-5.12-009`, `G-5.12-010`, `G-5.12-011`, `G-5.12-012`, `G-5.12-013`, `G-5.12-014`, `G-5.12-015`, `G-5.12-016`, `G-5.12-017`, `G-5.12-018`, `G-5.12-019`, `G-11-006`

## Ordered implementation tasks

1. Review lifecycle-event/delivery contracts, templates/policies, persistence/retry boundaries and observability configuration.
2. Implement all required notification events and delivery status/retries/reporting on top of phase 04 adapters and phase 05 secrets.
3. Add execution/step contextual logs, duration/retry/failure/worker/scheduler/API metrics, Prometheus endpoint and OpenTelemetry tracing.
4. Implement configurable log retention and history improvements; keep labels bounded and health/readiness tied to real subsystem state.

## Acceptance scenarios

- Exercise started/succeeded/failed, step failed, retry exhausted, cancelled, recovered and worker failure events, including template and policy selection.
- Failed notification delivery is recorded/retried within bounds and does not rewrite execution outcome; shutdown leaves no owned goroutines.
- Verify trace correlation, redaction, telemetry limits, bounded label sets, duration metrics and retention across restarts.
- Verify health/readiness behavior for storage/worker/scheduler failures and protected observability exposure as documented.

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
