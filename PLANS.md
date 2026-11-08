# OpsWeave Development Plan

Assessment date: 2026-11-08 (Asia/Jakarta). Baseline commit: `59c5b3f02a155f5979698bfde53c944063222be5`.

Harness state: `PHASE_01_AWAITING_LINUX_VERIFICATION`. On 2026-11-08 (Asia/Jakarta), the user accepted phase 00 and authorized phase 01 ("Ya, ACCEPT"), then accepted the [workflow contract](harness/designs/workflow-v1.md) and the five [foundation contract](harness/designs/foundation-storage-cli.md) decisions (separate "setuju" replies). Phase 01 implementation, documentation and [self-review](harness/reviews/phase-01.md) are prepared. Local unit/integration, race, fuzz and dependency checks passed; two symlink security tests skipped for Windows privilege limitations and require supported Linux/CI evidence before completion. Phases 02–10 remain unapproved. This is a documentation workflow, not an executable autonomous runner.

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

- Follow GOALS and AGENTS throughout. Keep a modular monolith, SQL-independent domain logic, injected infrastructure, explicit boundaries and minimal public interfaces. Retain Go 1.27.3 unless a separately justified change is approved.
- Security, structured logging, resource bounds, cancellation, tests and English documentation accompany their feature. Phase 10 audits completeness rather than postponing protections. Dangerous actions default disabled; no unauthenticated management endpoints or exactly-once side-effect claims.
- Validate/persist state changes and propagate infrastructure errors. Keep revisions immutable, SQLite single-node, and PostgreSQL ownership controls isolated. No mandatory Redis or RabbitMQ.
- Product phases require `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go mod verify`, `go build ./...`, Markdown checks and relevant acceptance suites. Core parser/engine/state-machine coverage targets at least 80%; security paths need important success/failure coverage. Benchmarks precede optimization.
- Every requirement needs observable acceptance, documentation, security and compatibility evidence before `COMPLETED`. Missing checks remain open; do not weaken tests or label configured remote checks as passed.
- Keep one documentation source. Build released documentation from exact tags, preserve earlier releases, distinguish development, and generate latest-stable routing. Do not freeze illustrative DSL or invent final API/schema/plugin contracts in this initialization.

## Review and next action

Both phase 01 contracts are accepted and implemented. Read the [foundation guide](docs/foundation.md), [self-review](harness/reviews/phase-01.md) and build-log evidence. The earlier race-toolchain gate is resolved by a verified portable compiler and passing unsandboxed race run. Obtain Linux evidence for the two skipped symlink tests, then complete phase acceptance before phase 02. The user has authorized committing and publishing this worktree on `codex/phase-01-foundation` with a PR targeting `main`; Linux CI and phase review remain pending. Phase 02 remains unapproved.

See [harness operating instructions](harness/README.md) for resumption, status rules and log requirements. No runtime dependency, executable runner, background process, scheduled automation or public SDK is added here.
