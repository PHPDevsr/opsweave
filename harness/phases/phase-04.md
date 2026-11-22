# Phase 04 — Built-in actions

Milestone: v0.3.x. Requirement status: `IN_PROGRESS`. Review state: APPROVED_FOR_PHASE_04; HTTP contract ACCEPTED; HTTP unit COMPLETED; toolchain patch MERGED; shell contract ACCEPTED; runtime IMPLEMENTED; native CI OPEN.

Authorization: the user confirmed Phase 03 was merged and requested continuation
on 2026-11-22 (Asia/Jakarta). Phase approval is separate from action contract
review. The first work unit is the [HTTP proposal](../designs/http-actions.md).

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

Run `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go mod verify`, `go build ./...`, `bun fmt --check "**/*.md"` and relevant phase acceptance suites. Add bounded fuzz runs for introduced parsers and isolated integration resources. Use deterministic synchronization and context deadlines. Record exact test/benchmark commands when introduced.

- [ ] Every owned row has exact test/review and documentation evidence attached; shared policy obligations audited.
- [ ] All applicable checks passed; unavailable checks remain explicit open gates.
- [ ] Security, compatibility and migration implications reviewed; source changes scoped.
- [ ] Append commands, exit codes, evidence, limitations and next steps to [build log](../build-log.md).
- [ ] Present results and obtain user acceptance before advancing. Do not treat code presence or this checklist as acceptance evidence.

HTTP runtime/tests/docs are implemented. See the [review](../reviews/phase-04-http.md)
and [security patch decision](../designs/go-http-security-patch.md). The next
[shell contract](../designs/shell-action.md) is accepted and implemented; see the
[shell review](../reviews/phase-04-shell.md) for completed local and native CI acceptance evidence.

## HTTP unit acceptance

Combined local verification and PR checks passed on `dfc0ae5` after rebase to
main `0033480`. The [review](../reviews/phase-04-http.md) and
[build log](../build-log.md) record the evidence closing the four HTTP/SSRF rows.
The phase remains in progress: shell is completed, while Docker, filesystem and notification actions
remain unimplemented. No Phase 05 work is authorized.

The user accepted the shell contract on 2026-11-22. Implement only that reviewed
scope; Docker, filesystem and notification contracts remain unaccepted.

## Shell unit acceptance and next contract

All required checks passed on combined source `3bd23de`; see the
[shell review](../reviews/phase-04-shell.md) and append-only build log. Three scoped
shell/security rows are COMPLETED. The next work unit is review of the
[Docker proposal](../designs/docker-actions.md), covering four opt-in local-IPC
adapters with exact container/action allowlists. Do not implement that runtime
before acceptance. Filesystem/notification contracts and Phase 05 remain separate.

## Docker implementation amendment

The user accepted the concrete Docker contract on 2026-11-22. The four local-IPC
adapters and their six Docker/security rows are IMPLEMENTED, with passing local
ordinary/race/static/security/fuzz/benchmark checks. See the
[Docker review](../reviews/phase-04-docker.md) for exact test mapping and remaining
native CI gates. Prior statements that Docker is unaccepted describe the previous
state and are superseded by this amendment. The next
[filesystem proposal](../designs/filesystem-actions.md) remains PROPOSED; do not
implement it before human review. Notification contracts/Phase 05 remain separate.

## Docker unit acceptance and next contract

Combined source `3ecbe68` passed full local verification and all six PR workflows.
The [Docker review](../reviews/phase-04-docker.md) records exact native/race/
security/benchmark/release evidence. Six scoped Docker/security rows are now
COMPLETED; seven HTTP/shell rows remain completed. No dependency or migration
is required. Phase 04 remains in progress. The next
[filesystem proposal](../designs/filesystem-actions.md) is concrete but PROPOSED;
obtain its path/deletion/copy/cancellation decision before runtime implementation.
Notification contracts and Phase 05 remain separate.

## Filesystem implementation amendment

The user explicitly accepted the filesystem contract on 2026-11-22, including
its deletion/publication/cancellation limits. The three filesystem adapters are
IMPLEMENTED with [review evidence](../reviews/phase-04-filesystem.md), English
operator docs and passing local ordinary/race/static/security/fuzz/benchmark
checks. Three local Windows symlink tests are unavailable due to account privilege;
native CI must execute them before completion. The Docker traceability amendment
also completes its seventh allowlist row using existing `3ecbe68` evidence.
The next [notification proposal](../designs/notification-actions.md) remains
PROPOSED. Do not implement it before review or advance Phase 05.

## Filesystem unit acceptance and next contract

Combined source `2bb4908` passed all six PR workflows. Native Windows security
prerequisites were satisfied in both Go jobs, closing the three local privilege
limitations; Linux/macOS link/special-file tests and Linux race also passed.
See the [filesystem review](../reviews/phase-04-filesystem.md) for exact evidence.
Three filesystem rows are now COMPLETED. Phase 04 remains in progress with
notification adapters still unimplemented. Review the concrete
[notification proposal](../designs/notification-actions.md), including environment
credentials, fixed profiles/destinations and verified bounded HTTP/SMTP delivery,
before implementation. No Phase 05 or persistent secret store is authorized.
