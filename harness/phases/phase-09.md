# Phase 09 — Extensions and advanced integrations

Milestone: v0.8.x. Requirement status: `PLANNED`. Review state: NOT_APPROVED.

## Goal, ownership and prerequisites

Deliver extensions and advanced integrations with evidence for every owned row in the [requirement ledger](../requirements.md). Ownership includes the exact source text and context; shared rules in phases 00 and 02 apply wherever relevant.

Prerequisites: All phases 00, 01, 02, 03, 04, 05, 06, 07, 08; immediate gate: phase 08 accepted.

Affected boundaries: Public Go SDK and external plugin host/protocol, database actions and filesystem trigger adapter.

Owned IDs: `G-5.3-006`, `G-5.5-027`, `G-5.5-028`, `G-5.5-029`, `G-5.14-001`, `G-5.14-002`, `G-5.14-003`, `G-5.14-004`, `G-5.14-005`, `G-5.14-006`, `G-5.14-007`, `G-5.14-008`, `G-5.14-009`, `G-5.14-010`, `G-5.14-011`, `G-5.14-012`, `G-5.14-013`, `G-5.14-014`, `G-5.15-013`, `G-11-009`

## Ordered implementation tasks

1. Review SDK/protocol compatibility, capability/permission declaration, configuration, execution isolation, resource limits and error contracts before publishing public interfaces.
2. Implement action registration/built-in packaging and isolated external execution; do not load arbitrary third-party code into the main process without a reviewed security model.
3. Implement database query/backup/restore with explicit target permissions, bounded results/time, protected credentials and isolated resources.
4. Implement advanced filesystem triggers with root restrictions, event validation/deduplication, bounded watches/queues and shutdown; publish community examples.

## Acceptance scenarios

- Test plugin version/capability mismatch, permission denial, malformed protocol, crashes, hangs, cancellation and host isolation.
- SDK examples build and compatibility fixtures exercise supported contracts; plugin failures remain distinguishable from engine errors.
- Database action tests cover injection, access restrictions, bounded results and backup/restore failures without production services.
- Filesystem tests cover traversal, duplicate events, watcher errors/resource limits, restart and deterministic shutdown.

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
