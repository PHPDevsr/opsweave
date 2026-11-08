# Phase 07 — Dashboard

Milestone: v0.6.x. Requirement status: `PLANNED`. Review state: NOT_APPROVED.

## Goal, ownership and prerequisites

Deliver dashboard with evidence for every owned row in the [requirement ledger](../requirements.md). Ownership includes the exact source text and context; shared rules in phases 00 and 02 apply wherever relevant.

Prerequisites: All phases 00, 01, 02, 03, 04, 05, 06; immediate gate: phase 06 accepted.

Affected boundaries: React/TypeScript frontend and public REST API extensions only; no browser access to internal storage.

Owned IDs: `G-5.13-001`, `G-5.13-002`, `G-5.13-003`, `G-5.13-004`, `G-5.13-005`, `G-5.13-006`, `G-5.13-007`, `G-5.13-008`, `G-5.13-009`, `G-5.13-010`, `G-5.13-011`, `G-5.13-012`, `G-5.13-013`, `G-5.13-014`, `G-5.13-015`, `G-5.13-016`, `G-5.13-017`, `G-5.13-018`, `G-11-007`

## Ordered implementation tasks

1. Review dashboard navigation and API coverage for every required page; specify necessary user/role/system configuration APIs and authorization before frontend wiring.
2. Build overview, workflow list/details/editor/history, execution details/timeline/live logs/run/cancel, worker/scheduler monitoring, secrets/users/roles/configuration.
3. Use public API contracts for all operations and handle authentication, errors, empty/loading states and bounded live-log subscriptions.
4. Document operator flows and deployment; preserve API contracts for existing CLI clients.

## Acceptance scenarios

- E2E tests cover every required page and main create/edit/run/cancel flows through the actual API.
- Authorization tests cover restricted pages and their underlying APIs; secret values remain absent from ordinary UI responses and logs.
- Test live-log disconnect/reconnect, bounded subscriptions, terminal executions, API errors and configuration validation.

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
