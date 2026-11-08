# Phase 04 — Built-in actions

Milestone: v0.3.x. Requirement status: `PLANNED`. Review state: NOT_APPROVED.

## Goal, ownership and prerequisites

Deliver built-in actions with evidence for every owned row in the [requirement ledger](../requirements.md). Ownership includes the exact source text and context; shared rules in phases 00 and 02 apply wherever relevant.

Prerequisites: All phases 00, 01, 02, 03; immediate gate: phase 03 accepted.

Affected boundaries: Built-in HTTP, shell, Docker, file and notification action packages behind the engine action contract.

Owned IDs: `G-5.5-013`, `G-5.5-014`, `G-5.5-016`, `G-5.5-018`, `G-5.5-019`, `G-5.5-020`, `G-5.5-021`, `G-5.5-023`, `G-5.5-024`, `G-5.5-025`, `G-5.5-031`, `G-5.5-032`, `G-5.5-033`, `G-5.5-034`, `G-5.15-007`, `G-5.15-008`, `G-5.15-009`, `G-5.15-017`, `G-5.15-018`, `G-5.15-019`, `G-5.15-020`, `G-11-004`

## Ordered implementation tasks

1. Specify/review input/output/version/security contracts for HTTP request/health, shell exec, Docker start/stop/restart/inspect, file exists/cleanup/copy and Telegram/Slack/email/webhook adapters.
2. Implement actions through the executor with context, timeout, bounded resources, structured errors and redacted logs. Inject transports/process/filesystem/container dependencies for isolated tests.
3. Enforce HTTP URL/DNS/redirect/network policy, direct shell argv and restricted executable configuration, Docker socket opt-in/container allowlists, and permitted filesystem roots.
4. Keep dangerous actions disabled by default; notification adapters accept securely supplied credentials without persisting plaintext. Secret-reference resolution waits for phase 05.

## Acceptance scenarios

- Test every action success, input rejection, cancellation, timeout, execution failure, output schema, version compatibility and engine-policy enforcement.
- SSRF tests include restricted destinations, DNS changes/redirects and response limits; no authorization headers leak.
- Shell tests prove metacharacters remain arguments and output is bounded; Docker tests prove opt-in and allowlist enforcement.
- File tests cover traversal, symlink escape and destructive-operation boundaries; notification adapter tests use isolated fake endpoints and redact credentials.

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
