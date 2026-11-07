# Phase 08 — PostgreSQL and distributed workers

Milestone: v0.7.x. Requirement status: `PLANNED`. Review state: NOT_APPROVED.

## Goal, ownership and prerequisites

Deliver postgresql and distributed workers with evidence for every owned row in the [requirement ledger](../requirements.md). Ownership includes the exact source text and context; shared rules in phases 00 and 02 apply wherever relevant.

Prerequisites: All phases 00, 01, 02, 03, 04, 05, 06, 07; immediate gate: phase 07 accepted.

Affected boundaries: PostgreSQL storage adapter, distributed claiming/leases, worker membership and monitoring.

Owned IDs: `G-5.2-053`, `G-5.10-002`, `G-5.10-018`, `G-5.11-004`, `G-5.11-005`, `G-5.11-006`, `G-5.11-011`, `G-5.11-013`, `G-5.11-014`, `G-11-008`

## Ordered implementation tasks

1. Review SQL backend contract differences, migrations, row-locking claims, lease identity/fencing, heartbeat expiry and recovery before implementation; use pgx.
2. Implement PostgreSQL repositories and safe transactional claims using FOR UPDATE SKIP LOCKED or a justified equivalent.
3. Add worker registration/heartbeats, renewable leases, stale-owner rejection, capacity accounting and failure recovery without exactly-once claims.
4. Run shared repository suites on both backends and publish topology/backup/migration guidance; SQLite stays single-node.

## Acceptance scenarios

- Isolated PostgreSQL integration tests prove no conflicting ownership under simultaneous claims and updates.
- Test lease renewal/expiry, stale completion rejection, worker death, database failure, restart recovery and shutdown under active work.
- Verify bounded capacity and race safety; test migration rollback/errors and historical revision persistence on both backends.

## Security, documentation and compatibility

- Follow [AGENTS.md](../../AGENTS.md) and [GOALS.md](../../GOALS.md); keep domain logic independent of HTTP and SQL adapters. Introduce only packages needed for delivered behavior.
- Review new external inputs, permissions, resource bounds and secret handling before exposing each feature. Inject infrastructure dependencies, propagate contexts and persist validated state changes atomically.
- Update English reference documentation, examples and troubleshooting with the feature. Record schema migrations, rollback/data implications and compatibility evidence; if no migration/API change is needed, state that with a reason in the build log.
- Review contracts listed in the tasks before their implementations. No illustrative YAML or unreviewed API/schema/plugin design becomes a released promise. Inspect relevant release tags once they exist; keep one documentation source.

## Verification and review checklist

Run `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go mod verify`, `go build ./...`, `npm run check:markdown` and relevant phase acceptance suites. Add bounded fuzz runs for introduced parsers and isolated integration resources. Use deterministic synchronization and context deadlines. Record exact test/benchmark commands when introduced.

- [ ] Every owned row has exact test/review and documentation evidence attached; shared policy obligations audited.
- [ ] All applicable checks passed; unavailable checks remain explicit open gates.
- [ ] Security, compatibility and migration implications reviewed; source changes scoped.
- [ ] Append commands, exit codes, evidence, limitations and next steps to [build log](../build-log.md).
- [ ] Present results and obtain user acceptance before advancing. Do not treat code presence or this checklist as acceptance evidence.
