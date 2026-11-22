# OpsWeave Development Plan

Assessment date: 2026-11-08 (Asia/Jakarta). Baseline commit: `59c5b3f02a155f5979698bfde53c944063222be5`.

Harness state: `PHASE_04_ACCEPTANCE_REQUIRED`. Phases 01-03 are merged and accepted. Phase 04 is authorized. All five approved action families are complete: twenty-one HTTP/shell/Docker/filesystem/notification rows are COMPLETED. Final combined product source `963d143` passed all six PR workflows: native Linux/macOS/Windows on Go 1.27.4 and stable, Linux race, three-OS latest scanner, required notification benchmark, Bun Markdown, CodeQL and six-target release preview/assemble. Implementation, automated tests, code/security review, English docs and compatibility evidence are recorded in the [notification review](harness/reviews/phase-04-notification.md) and earlier family reviews. The aggregate milestone `G-11-004` remains IMPLEMENTED pending human phase/release acceptance. Stop here and present Phase 04 for acceptance. Phase 05 remains unapproved. Go remains 1.27.4; feature-branch checks use PR events only. The harness is a documentation workflow with no background runner installed.

The user additionally authorized multi-OS release CI before Phase 03 on
2026-11-08 (Asia/Jakarta). This is a narrow early packaging extension from Phase
10: six target archives, version injection, checksums and tag-based publication.
It does not approve Phase 03 or the remaining production-qualification work.
See [release procedure](docs/releases.md); no tag or release is created by this task.

## Repository assessment

This section records the original baseline assessment. Current implementation and
acceptance evidence are in the [foundation guide](docs/foundation.md) and
[phase 01 self-review](harness/reviews/phase-01.md).

Read [GOALS.md](GOALS.md), [AGENTS.md](AGENTS.md), README, contribution instructions, all source/tests, manifests, formatter configuration, and GitHub workflow/Dependabot configuration. No prior PLANS or harness files existed. The assessed worktree was clean and had no release tags.

- `cmd/opsweave` provides process entry, interrupt context and exit status; `internal/cli` builds independent urfave/cli v3 command trees with injected output streams.
- Existing behavior: help, development version text/JSON, Bash/Zsh/Fish/PowerShell completion, argument/flag validation, cancellation checks and output-error handling. Tests cover these behaviors and invocation independence.
- Configuration, init/server commands, workflow parser/registry, engine, persistence, actions, scheduler, triggers, workers, secrets, API, telemetry, dashboard and plugins are not implemented. The GOALS YAML is illustrative.
- `go.mod` declares Go 1.27.3 and urfave/cli v3.14.0. Installed Go matches. Node/Prettier are documentation tooling, not runtime infrastructure.
- CI configuration covers minimum/stable Go on Linux, Windows and macOS, formatting, build, vet, tests, CLI smoke tests, module consistency and Linux race tests. Separate Markdown and CodeQL workflows and Dependabot are configured. Remote runs, security settings and branch protection were not verified. README/contribution prose omits macOS in some CI descriptions; actual workflow configuration is the assessment source of truth.
- No executable harness, public product API, migrations, release pipeline, documentation site or deployment packaging exists. Do not infer installed product capabilities from local ignored `bin` or `node_modules` directories.

Baseline formatting, vet, unit tests, module verification, build and Markdown checks passed. Build exited 0 with a module-cache permission warning. Race testing was blocked by CGO being disabled and no GCC/Clang on PATH. Exact evidence and limitations are in the [build log](harness/build-log.md); CI configuration alone does not close this gap.

## Phase index and dependencies

Every phase requires acceptance of all preceding phases. The [ledger](harness/requirements.md) assigns one accountable phase to every GOALS requirement and reference. Phase plans list exact owned IDs, design tasks, affected boundaries and acceptance scenarios.

| Phase | Milestone     | Plan and acceptance focus                                                                                                                     |
| ----- | ------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| 00    | Planning only | [Assessment and harness](harness/phases/phase-00.md): complete traceability, operating rules and reviewed plans                               |
| 01    | v0.1.x        | [Workflow foundation](harness/phases/phase-01.md): reviewed DSL, validated configuration, lifecycle/revisions, SQLite and workflow CLI        |
| 02    | v0.1.x        | [Durable manual execution](harness/phases/phase-02.md): action contract, validated persisted states, manual execution and history             |
| 03    | v0.2.x        | [Reliable orchestration](harness/phases/phase-03.md): policies, recovery, bounded workers, authenticated triggers and scheduler               |
| 04    | v0.3.x        | [Built-in actions](harness/phases/phase-04.md): HTTP, shell, Docker, file and notification adapters with security controls                    |
| 05    | v0.4.x        | [Management API and secrets](harness/phases/phase-05.md): versioned REST, RBAC, encrypted secrets and management CLI                          |
| 06    | v0.5.x        | [Observability and notifications](harness/phases/phase-06.md): lifecycle delivery, logs, metrics, tracing and retention                       |
| 07    | v0.6.x        | [Dashboard](harness/phases/phase-07.md): every required page using the public API                                                             |
| 08    | v0.7.x        | [PostgreSQL and distributed workers](harness/phases/phase-08.md): safe claims, leases, heartbeats and backend contract tests                  |
| 09    | v0.8.x        | [Extensions and advanced integrations](harness/phases/phase-09.md): isolated plugins/SDK, database actions and filesystem triggers            |
| 10    | v0.9.x–v1.0.0 | [Production qualification](harness/phases/phase-10.md): deployment, operations, security/performance, compatibility and release documentation |

Important dependencies apply before exposure: phase 01 reviews the full DSL contract but rejects unsupported runtime capabilities; phase 02 action doubles prove engine behavior without claiming integrations ship; phase 03 authenticates webhooks before the broader phase 05 management API exists. Phase 04 notification adapters use securely supplied credentials; encrypted persistent secret references arrive in phase 05. Phase 06 builds notification policies on those adapters and secrets. Phase 08 extends existing single-node guarantees rather than coupling SQLite to distributed infrastructure.

## Quality and compatibility gates

- Follow GOALS and AGENTS throughout. Keep a modular monolith, SQL-independent domain logic, injected infrastructure, explicit boundaries and minimal public interfaces. Retain Go 1.27.4 unless a separately justified change is approved. The user approved this patch-level security increase on 2026-11-22 for GO-2026-6859; historical verification remains attributed to its original toolchain.
- Security, structured logging, resource bounds, cancellation, tests and English documentation accompany their feature. Phase 10 audits completeness rather than postponing protections. Dangerous actions default disabled; no unauthenticated management endpoints or exactly-once side-effect claims.
- Validate/persist state changes and propagate infrastructure errors. Keep revisions immutable, SQLite single-node, and PostgreSQL ownership controls isolated. No mandatory Redis or RabbitMQ.
- Product phases require `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go mod verify`, `go build ./...`, Markdown checks and relevant acceptance suites. Core parser/engine/state-machine coverage targets at least 80%; security paths need important success/failure coverage. Benchmarks precede optimization.
- Every requirement needs observable acceptance, documentation, security and compatibility evidence before `COMPLETED`. Missing checks remain open; do not weaken tests or label configured remote checks as passed.
- Keep one documentation source. Build released documentation from exact tags, preserve earlier releases, distinguish development, and generate latest-stable routing. Do not freeze illustrative DSL or invent final API/schema/plugin contracts in this initialization.

## Review and next action

Phase 04 remains active. HTTP is completed with scoped acceptance evidence.
The user accepted the [shell contract](harness/designs/shell-action.md); shell
implementation/tests/docs passed local checks and await native multi-OS CI.
See the [shell review](harness/reviews/phase-04-shell.md). Keep unrelated rows and
later phases unchanged. Review the next action contract before implementing it.

See [harness operating instructions](harness/README.md) for resumption, status rules and log requirements. The harness itself remains documentation; no executable runner, background process, scheduled automation or public SDK is installed.

## 2026-11-22 progression amendment

The user confirmed Phase 03 was merged and authorized continuation. Fetch and
rebase to origin/main completed at `eed2b62` without conflicts. Phase 03 is
accepted for progression; historical unavailable checks are not relabeled as
passes. Phase 04 is authorized on `codex/phase-04-built-in-actions`. Earlier
statements that Phase 04 is unapproved describe the previous state and are
superseded by this amendment. Review the [HTTP proposal](harness/designs/http-actions.md)
before its implementation. Phases 05–10 remain unapproved except the existing
release packaging extension.

## HTTP implementation amendment

The user accepted the HTTP contract. Both actions and their local tests, race
verification, benchmark and documentation are implemented. See the [review](harness/reviews/phase-04-http.md).
The [toolchain security decision](harness/designs/go-http-security-patch.md)
was accepted and merged separately in PR #9. The user confirmed that merge on
2026-11-22 and authorized the HTTP rebase. Branch HTTP now includes Go 1.27.4,
latest-scanner CI and feature-branch PR-only checks from origin/main `0033480`.
Combined HTTP verification passed, including all PR checks and six-target release
preview/assemble. Four scoped HTTP/SSRF rows are completed with evidence in the
HTTP review and build log. The next
[shell contract](harness/designs/shell-action.md) was accepted by the subsequent user reply "ACCEPT". Implementation is locally verified; native multi-OS CI is pending.
No later phase is authorized.
