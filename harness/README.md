# OpsWeave Harness Operating Instructions

This harness is a documentation workflow. Start with [PLANS.md](../PLANS.md), [GOALS.md](../GOALS.md), [AGENTS.md](../AGENTS.md), the [ledger](requirements.md), relevant [phase plan](phases/phase-00.md) and latest [build log](build-log.md). No runner or unattended process is installed.

## Authorization and reviews

Current state: `PHASE_04_FILESYSTEM_CONTRACT_REVIEW_REQUIRED`. Phases 01-03 are merged and accepted by the user. Phase 04 is authorized. Four HTTP/SSRF, three shell/security and six Docker/security rows are COMPLETED with their implementation/test/security/documentation evidence. The accepted Docker adapters passed all six PR workflows on combined source `3ecbe68`, including all native OS/Go jobs, Linux race, latest vulnerability scanner, benchmark CI and six-target release preview/assemble. See the [Docker review](reviews/phase-04-docker.md). The next [filesystem contract](designs/filesystem-actions.md) remains PROPOSED and needs human review before implementation, including deletion, copy publication and cooperative filesystem cancellation limits. Notification contracts and Phase 05 remain unapproved. Go remains 1.27.4; feature-branch checks use PR events only. The harness is a documentation workflow with no background runner installed.

Track reviews independently from GOALS requirement status. Record explicit user approval with date, authorized phase/scope and conversation reference or a faithful summary in the build log. Update the master/phase review state together. An approval of one phase never silently approves the next.

Within an approved phase, complete scoped tasks autonomously. Before the next phase, present acceptance evidence and obtain review. If a required contract is unresolved, complete its design task and seek the relevant decision before implementing it. New scope, a breaking contract or a changed security model requires review; ordinary reversible choices within approved scope do not need repeated approval.

## Work-unit and resumption procedure

1. Read current repository instructions and plans; inspect commit, worktree, release tags, source and existing tests. Preserve unrelated user edits. Do not rely on an old log as current repository truth.
2. Identify owned requirement IDs, prerequisite acceptance and authorized phase. Compare the ledger with changed GOALS text; reconcile IDs without renumbering old rows.
3. Choose the smallest maintainable work unit. Review required contracts before implementation, add behavior/failure tests, and preserve architecture/security constraints.
4. Implement only approved work, update English docs/examples, and run required plus phase-specific verification. Do not rerun broad checks without a relevant change or unresolved concern.
5. Attach exact test names, documentation, command outcomes and log entry to affected ledger rows. Record changes, limitations and next steps; review the diff for scope and secrets.
6. Present phase results and open gates. Advance only after user acceptance. On resume, reassess source and verification freshness; rerun affected checks when code, dependencies or environment changed.

A blocker records the failing dependency/check, reproduction, impact and next action. Continue unaffected authorized work when possible. Do not bypass security, discard durable state or declare success because the environment cannot run a required check.

## Status and completion

Use only GOALS §14 statuses for requirements: `PLANNED`, `IN_PROGRESS`, `IMPLEMENTED`, `COMPLETED`, `DEFERRED`, `CANCELLED`. Deferred/cancelled entries retain their ID and require a documented scope decision. Review state is a separate field, not another requirement status.

`IMPLEMENTED` means code exists, with incomplete acceptance. Existing CLI capability rows refer only to existing commands. To mark `COMPLETED`, record implementation, observable automated tests, relevant integration and race results, security review, English docs/examples and compatibility/migration evidence. Configured CI is not remote execution evidence. Phase acceptance remains open for unavailable mandatory checks.

Phase 00 owns ongoing policy enforcement across all work. The common action contract in phase 02 applies to phase 04 and 09 integrations. Contract vocabulary in an earlier phase does not imply later runtime behavior is delivered.

## Verification and evidence

Product verification commands:

```sh
gofmt -l .
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go mod verify
go build ./...
bun fmt --check "**/*.md"
git diff --check
```

Use Go from `go.mod`. Race tests require CGO and a compatible compiler. A blocked race test is not a passing race test; a verified supported CI run can supply evidence when local tooling is unavailable. Report build warnings even when exit status is 0. Do not mutate module files merely to assess consistency; run dependency reconciliation only when authorized changes require it.

Planning-only changes require ledger reconciliation, link/ID/phase validation, Markdown formatting and diff checks. Baseline Go evidence remains attributed to its original assessment; documentation changes do not establish product acceptance.

For introduced parsers, record bounded fuzz commands and corpus results. Use isolated databases/services and context deadlines; no production credentials. Add integration, E2E, race, security, compatibility and benchmark evidence prescribed by each phase. Maintain at least 80% statement coverage for parser/engine/state machine without weakening assertions or chasing a global percentage.

## Append-only build log

Append a dated entry per meaningful work unit; correct previous entries with a new amendment instead of rewriting history. Include:

- Local date/timezone, baseline commit/worktree context and authorization/review state.
- Requirement IDs or phase reference, intended work and actual changes.
- Commands and individual exit statuses; exact tests/docs/artifact evidence where available.
- Errors, warnings, unavailable checks, security/compatibility/migration findings and unresolved gates.
- Next concrete action and any review needed.

Never store credentials, decrypted secrets or sensitive command arguments in the log. Keep generated binaries, caches and local configuration outside tracked artifacts. This harness creates no external messages, commits, PRs, deployment or release by itself.

## 2026-11-22 progression amendment

The user confirmed Phase 03 was merged and authorized continuation. Fetch and
rebase to origin/main completed at `eed2b62` without conflicts. Phase 03 is
accepted for progression; historical unavailable checks are not relabeled as
passes. Phase 04 is authorized on `codex/phase-04-built-in-actions`. Earlier
statements that Phase 04 is unapproved describe the previous state and are
superseded by this amendment. Review the [HTTP proposal](designs/http-actions.md)
before its implementation. Phases 05–10 remain unapproved except the existing
release packaging extension.

## HTTP implementation amendment

The user accepted the HTTP contract. Both actions and their local tests, race
verification, benchmark and documentation are implemented. See the [review](reviews/phase-04-http.md).
The [toolchain security decision](designs/go-http-security-patch.md)
was accepted and merged separately in PR #9. The user confirmed that merge on
2026-11-22 and authorized the HTTP rebase. Branch HTTP now includes Go 1.27.4,
latest-scanner CI and feature-branch PR-only checks from origin/main `0033480`.
Combined HTTP verification passed, including all PR checks and six-target release
preview/assemble. Four scoped HTTP/SSRF rows are completed with evidence in the
HTTP review and build log. The next
[shell contract](designs/shell-action.md) was accepted by the subsequent user reply "ACCEPT". Implementation is locally verified; native multi-OS CI is pending.
No later phase is authorized.
