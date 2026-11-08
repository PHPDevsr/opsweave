# Phase 03 — Reliable orchestration

Milestone: v0.2.x. Requirement status: `IN_PROGRESS` (serial core locally verified; dispatcher review required). Review state: APPROVED_FOR_PHASE_03; execution contract ACCEPTED (user reply "ACCEPT", 2026-11-08, Asia/Jakarta); dispatcher contract AWAITING_REVIEW.

Authorization: the user confirmed merging Phase 02 and requested "git fetch origin dan git rebase origin/main. Lanjut phase 3" on 2026-11-08 (Asia/Jakarta). Phase 02 user acceptance is separate from the unresolved CodeQL configuration check. The [reliable execution proposal](../designs/reliable-execution.md) was accepted in a subsequent "ACCEPT" reply. The serial core is described in the [core guide](../../docs/reliability.md) and [review](../reviews/phase-03-core.md). The next [dispatcher proposal](../designs/single-node-dispatcher.md) requires its own ownership/capacity review before activation. It does not finalize scheduler/trigger/listener or dispatcher ownership contracts. No later phase is authorized.

## Goal, ownership and prerequisites

Deliver reliable orchestration with evidence for every owned row in the [requirement ledger](../requirements.md). Ownership includes the exact source text and context; shared rules in phases 00 and 02 apply wherever relevant.

Prerequisites: All phases 00, 01, 02; immediate gate: phase 02 accepted.

Affected boundaries: Engine policies, dispatcher, bounded single-node workers, scheduler, trigger manager, listener and persistence.

Owned IDs: `G-5.1-022`, `G-5.2-004`, `G-5.2-006`, `G-5.2-009`, `G-5.2-010`, `G-5.2-011`, `G-5.2-012`, `G-5.2-013`, `G-5.2-014`, `G-5.2-018`, `G-5.2-019`, `G-5.2-021`, `G-5.2-022`, `G-5.2-023`, `G-5.2-041`, `G-5.2-042`, `G-5.2-043`, `G-5.2-044`, `G-5.2-045`, `G-5.2-048`, `G-5.2-049`, `G-5.2-050`, `G-5.2-051`, `G-5.3-002`, `G-5.3-003`, `G-5.3-004`, `G-5.3-005`, `G-5.3-007`, `G-5.3-008`, `G-5.3-009`, `G-5.3-010`, `G-5.3-011`, `G-5.3-012`, `G-5.3-013`, `G-5.3-014`, `G-5.3-015`, `G-5.3-016`, `G-5.3-017`, `G-5.3-018`, `G-5.3-019`, `G-5.4-001`, `G-5.4-002`, `G-5.4-003`, `G-5.4-004`, `G-5.4-005`, `G-5.4-006`, `G-5.4-007`, `G-5.4-008`, `G-5.4-009`, `G-5.4-010`, `G-5.4-011`, `G-5.4-012`, `G-5.4-013`, `G-5.4-014`, `G-5.9-004`, `G-5.9-016`, `G-5.9-017`, `G-5.10-007`, `G-5.10-008`, `G-5.10-012`, `G-5.10-013`, `G-5.11-001`, `G-5.11-002`, `G-5.11-003`, `G-5.11-007`, `G-5.11-008`, `G-5.11-009`, `G-5.11-010`, `G-5.11-012`, `G-5.11-015`, `G-5.15-012`, `G-5.16-007`, `G-5.16-008`, `G-5.16-009`, `G-5.16-011`, `G-11-003`

## Ordered implementation tasks

1. Review condition/failed-dependency semantics, parallel limits, retry/backoff deadlines, timeout/cancellation ownership and retry/replay identity before coding.
2. Implement bounded job claiming, configurable capacity, controlled shutdown, idempotency/deduplication, retention and restart recovery with explicit handling of uncertain non-idempotent outcomes.
3. Implement persisted cron/timezone schedules and configurable missed-run behavior; internal/startup/webhook triggers with validation, authentication, event history, failure reporting and concurrency limits.
4. Add server lifecycle, execution cancel/retry commands, health/readiness, resource limits and trigger/schedule storage; authenticate and size/time-bound webhooks before exposure. No unauthenticated management API.

## Acceptance scenarios

- Valid/invalid/conditional/parallel workflows, retry success/exhaustion, execution and step deadlines, concurrent cancellation and dependency failure paths.
- Deterministic clock tests cover timezone/DST, missed runs, enable/disable, duplicate prevention and restart recovery; avoid arbitrary sleeps.
- Crash/restart tests preserve durable transitions and reject silent duplicate non-idempotent effects; race tests cover state updates, capacity and worker/scheduler shutdown.
- Webhook tests reject invalid, unauthorized, oversized and duplicate events; disabled triggers/workflows cannot create automatic executions.

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
