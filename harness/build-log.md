# OpsWeave Build Log

Append-only evidence record. Dates use Asia/Jakarta. Follow [operating instructions](README.md); requirement ownership is in the [ledger](requirements.md).

## 2026-11-08 — Initial repository assessment

- Baseline commit: `59c5b3f02a155f5979698bfde53c944063222be5`; worktree clean; no release tags. GOALS/AGENTS, source/tests, manifests, documentation and all existing CI configuration inspected. No PLANS or harness files existed.
- Scope: phase 00 and policy/traceability requirements assigned there. Existing CLI behavior assessed as implemented, not product-complete. No workflow/configuration/engine/storage/server/action/trigger/worker/secrets/API/telemetry/dashboard/plugin subsystem exists.
- Environment: PowerShell on Windows amd64; `go version` reports `go1.27.3 windows/amd64`. `go env CGO_ENABLED` reports `0`; GCC and Clang were not found on PATH.
- Existing CI is configuration evidence only; remote runs, repository settings, CodeQL uploads and branch protection were not verified. README/contribution CI prose is narrower than the configured three-OS matrix.
- Default Git status emitted an fsmonitor IPC diagnostic; `git -c core.fsmonitor=false status --short` established clean state without changing Git configuration.

| Command                        | Result              | Evidence/limitation                                                                                      |
| ------------------------------ | ------------------- | -------------------------------------------------------------------------------------------------------- |
| `gofmt -l .`                   | Exit 0              | No files reported                                                                                        |
| `go vet ./...`                 | Exit 0              | No diagnostics                                                                                           |
| `go test -count=1 ./...`       | Exit 0              | CLI tests passed; cmd package has no tests                                                               |
| `go test -race -count=1 ./...` | Failed before tests | `-race requires cgo; enable cgo by setting CGO_ENABLED=1`; numeric exit was not separately captured      |
| `go mod verify`                | Exit 0              | All modules verified                                                                                     |
| `go build ./...`               | Exit 0              | Warning: unable to write module stat cache due to access denied; does not establish a warning-free build |
| `npm run check:markdown`       | Passed              | All matched Markdown used Prettier style; numeric exit was not separately captured                       |

Open gates: race verification requires a supported CGO/compiler environment or actual passing supported CI evidence. No product acceptance established. User selected documentation-only harness and review before advancing each phase. Initial plan was presented for review without repository changes.

## 2026-11-08 — Harness generation authorized

- User explicitly requested implementation of the presented initialization plan. This authorizes documentation generation; it does not authorize phase 01 product work.
- Reconfirmed the same commit, clean starting worktree, no tags and no prior harness. Re-read requirements/instructions and inspected actual CLI source/tests and toolchain declaration.
- Added master plan, exhaustive source ledger, phase 00–10 plans, operating instructions and this log. Requirements with foundation CLI code remain `IMPLEMENTED`; all other ledger rows begin `PLANNED`. No product requirement is `COMPLETED`.
- No product source, dependency, API, migration or runtime configuration changes. No compatibility or data migration impact. Generated plans preserve security prerequisites and one-source release documentation policy.
- Review state: `AWAITING_INITIAL_REVIEW`; generated artifacts will be presented for review. Phase 01 remains `NOT_APPROVED`.
- Validation results follow in an appended entry after checks execute.

## 2026-11-08 — Planning artifact validation

- Created 15 Markdown artifacts: master plan, requirement ledger, eleven phase plans, operating instructions and build log. Changes are confined to `PLANS.md` and `harness/`.
- An inline Python reconciliation command independently enumerated GOALS content blocks, bullets, check/CLI commands and release rows, treating the illustrative YAML as one reference. It compared all 561 source entries with ledger source references: no missing or duplicate entries. Exit 0.
- Verified 561 unique stable IDs, one phase owner per ID, presence in exactly one phase plan and prerequisites restricted to earlier phases. All 1,192 local Markdown links resolved. All 15 expected artifacts exist. Exit 0.
- Ownership counts: phase 00 = 161; 01 = 70; 02 = 68; 03 = 76; 04 = 22; 05 = 55; 06 = 37; 07 = 19; 08 = 10; 09 = 20; 10 = 23. Status counts: 552 `PLANNED`, nine existing CLI rows `IMPLEMENTED`, zero `COMPLETED`.
- Formatted only the new artifacts using the existing locked local Prettier. `npm run check:markdown` reported all Markdown formatted. `git -c core.fsmonitor=false diff --check` exited 0. No new dependency or retained validation runner was needed.
- Baseline Go results above were carried forward, not rerun for documentation-only changes. The race-test environment limitation and build warning remain open; no integration, E2E or remote CI results are claimed.
- Review outcome: artifacts ready for initial review, acceptance pending. Next action is review of the generated documents and explicit phase 01 authorization; no product implementation has begun.

## 2026-11-08 — Phase authorization and workflow contract design

- Baseline: `d7b89f6` (merged harness initialization); starting worktree clean, no release tags. Re-read GOALS, phase 01 plan, actual CLI implementation/tests, README and manifests. No additional nested AGENTS instructions found.
- Authorization: user replied "Ya, ACCEPT" to the explicit question accepting phase 00 and authorizing phase 01. Recorded phase 00 artifact acceptance and phase 01 approval in master/operating/phase documents. Phases 02–10 remain unapproved; recurring policy and verification obligations remain open.
- Work unit: phase 01 task 1, `G-6-001`–`G-6-016`. Added [workflow v1 contract proposal](designs/workflow-v1.md) covering schema, bounded parsing, dependency/condition semantics, typed references, retries, deadlines, concurrency, errors, secrets, action compatibility, immutable revisions, planning and phase capability rejection. Added proposed acceptance cases and design self-review. These rows are `IN_PROGRESS`, with draft evidence linked from the ledger; no requirement is completed.
- Design review findings: ordinary inputs cannot interpolate code; conditions are bounded; unknown capabilities fail explicitly; descriptor-declared credentials cannot be persisted as inline literals. Phase 01 has no built-in action implementation, so production registration requires a future compatible action catalog; fixture catalogs remain test-only. This usability trade-off is an explicit review decision, not a hidden implementation assumption.
- Documentation-only changes: no Go source, dependencies, database schema, migrations, runtime behavior or released documentation changed. No product tests added; proposed behavior tests are listed in the design and remain unexecuted. Existing Go/race evidence is historical, not rerun or claimed as current product acceptance.
- Verification: scoped locked Prettier formatting exited 0; `npm run check:markdown` exited 0; `git -c core.fsmonitor=false diff --check` exited 0. Inline Python validation exited 0: 561 unique ledger IDs with exactly one phase owner each, exactly 16 draft rows in progress, zero completed rows, 1,202 local links resolved, approval states consistent. The first validator attempt exited 1 because its regex did not account for table padding; corrected whitespace-independent cell parsing passed without changing ledger identities. Final formatting/diff checks follow after this entry is written.
- Self-review covered changed documentation and the full draft for scope, security, compatibility and truthful status. Runtime acceptance and the previous race-toolchain limitation remain open. Per harness operating instructions, the concrete contract now needs user review before parser implementation. Next action: accept or amend the four review decisions in the draft, then proceed within phase 01 only.

## 2026-11-08 — Workflow contract accepted and candidate parser implemented

- Authorization: user replied "setuju" to the explicit review question for the four workflow-v1 decisions. Recorded `ACCEPTED_FOR_IMPLEMENTATION`; phase 01 remains the only approved product phase. Baseline commit remains `d7b89f6`; preceding planning/design edits were preserved. No release tags or additional nested AGENTS files found.
- Implemented `internal/workflow`: candidate domain values, original-source copying, normalized defaults, strict YAML/schema validation, bounded condition syntax, dependency DAG/topological ordering, transitive reference validation, symbolic secret/output references and phase 01 definition capability rejection. SQL, action invocation, network access and secret resolution are absent. Candidates still need catalog/schema/credential validation before registration; no new CLI command is exposed.
- Added observable table-driven tests: `TestParseDefaultsAndSource`, `TestParseLiteralsAndMetadata`, `TestParseRejectsInvalidDefinitions`, `TestDurationValidation`, `TestLimitsAndCancellation`, `TestGraphAndReferences`, `TestIdentifier`, `TestSafeErrors`, `TestCollectionBoundaries`, `TestValidateCondition`; fuzz targets `FuzzParse` and `FuzzValidateCondition`. Tests cover boundaries, safe diagnostics, malformed YAML/expressions, cycle/reference errors, policy defaults and unsupported features. No production actions or external credentials used.
- Dependency: added direct `go.yaml.in/yaml/v3`, initially cached v3.0.5, then verified upstream metadata and updated to v3.0.6 before final checks. [Upstream source](https://github.com/yaml/go-yaml) and [version/API documentation](https://pkg.go.dev/go.yaml.in/yaml/v3@v3.0.6) identify the maintained YAML implementation. The module requires Go 1.16; OpsWeave stays on Go 1.27.3. `go get` and final `go mod tidy` exited 0; checksums remain enabled. No SQLite dependency introduced before contract review.
- Added [implemented parser documentation](../docs/workflow-parsing.md), updated README/architecture and state documents. `G-5.1-002` and `G-5.1-004` are `IMPLEMENTED`; schema/catalog and registry metadata/label requirements remain `IN_PROGRESS`. No row marked `COMPLETED`. Runtime retry, timeout, cancellation, action security and persistence semantics in the accepted design remain unimplemented.
- Code self-review: checked bounds and overflow handling, safe errors, graph/reference ordering, original-source ownership, context propagation, absence of mutable globals/goroutines and clear separation from storage/HTTP. Added cancellation checks during reference graph traversal and collection-boundary tests. No existing test removed or weakened. No released API, migration or data compatibility change.
- Review finding: the accepted action-name grammar excludes underscores, conflicting with GOALS names such as `http.health_check`. Current implementation retains the accepted grammar; correction is explicitly proposed in the [remaining foundation contracts](designs/foundation-storage-cli.md). That proposal also completes task 2's repository/migration/revision, configuration and CLI design, including archive semantics, atomic batches and acceptance cases. It is `DRAFT_FOR_REVIEW`; no persistence or configuration implementation is claimed.

Final checks below ran with YAML v3.0.6. Go commands that need temporary files used workspace `.cache/go-tmp` via `GOTMPDIR`; ignored caches are not product artifacts.

| Command                                                                                           | Exit | Result                                                                                          |
| ------------------------------------------------------------------------------------------------- | ---- | ----------------------------------------------------------------------------------------------- |
| `gofmt -l .`                                                                                      | 0    | No files reported                                                                               |
| `go vet ./...`                                                                                    | 0    | No diagnostics                                                                                  |
| `go test -count=1 ./...`                                                                          | 0    | CLI and workflow tests passed; cmd has no tests                                                 |
| `go test -count=1 '-coverprofile=.cache/workflow.cover' ./internal/workflow`                      | 0    | 97.7% statement coverage                                                                        |
| `go test ./internal/workflow -run '^$' -fuzz '^FuzzParse$' -fuzztime=10s -parallel=1`             | 0    | 38,701 executions; four seeds plus 106 interesting inputs; no failure                           |
| `go test ./internal/workflow -run '^$' -fuzz '^FuzzValidateCondition$' -fuzztime=10s -parallel=1` | 0    | 283,517 executions; four seeds plus 38 interesting inputs; no failure                           |
| `go test -race -count=1 ./...`                                                                    | 2    | Blocked before tests: CGO disabled; GCC/Clang absent from PATH and common local toolchain paths |
| `go mod verify`                                                                                   | 0    | All modules verified                                                                            |
| `go build ./...`                                                                                  | 0    | Build passed with module stat-cache access-denied warning for the root module                   |
| `govulncheck ./...`                                                                               | 0    | No vulnerabilities found using official database access                                         |
| `npm run check:markdown`                                                                          | 0    | All matched Markdown formatted before this log append                                           |
| `git -c core.fsmonitor=false diff --check`                                                        | 0    | No whitespace errors; Git emitted CRLF-to-LF normalization warning for ledger                   |

- Earlier failed attempts, not final acceptance: coverage in the default Windows temp directory failed writing metadata (exit 1); workspace `GOTMPDIR` resolved it. An unquoted PowerShell coverage-profile argument was split and treated as an extra package (exit 1); the quoted command above passed. Initial sandboxed vulnerability lookup failed DNS (exit 1); rerunning with official database network access passed (exit 0). Early v3.0.5 coverage/fuzz results were superseded by final v3.0.6 runs above.
- Ledger/link validation exited 0: 561 unique IDs with exactly one phase owner each, zero completed rows, 1,225 local links resolved and phases 02–10 still unapproved. Final documentation formatting/diff checks run after this entry. No SQLite/integration/E2E or remote CI result is claimed; those components do not yet exist. Race verification remains a mandatory open gate and configured CI alone does not close it.
- Next action requiring a decision: review the five decisions in the remaining foundation proposal, then implement approved registry/storage/configuration/CLI work within phase 01. No phase 02 work, commit, PR, deployment or unattended process was started.

## 2026-11-08 — Phase 01 foundation implementation and verification

- Authorization: the latest user reply "setuju" accepted all five decisions in the foundation storage/configuration/CLI proposal, including action underscores, atomic batches, archive name reservation, strict config/exclusive init and the additional revision/export/plan commands. Both contracts are accepted; phase 01 remains the only approved product phase.
- Baseline: `d7b89f6` on local `main`, with the preceding planning/parser worktree changes preserved. Origin is the OpsWeave GitHub repository. No release tags, commit, push, branch publication or PR were created. No background orchestrator/automation installed.
- Implementation: shared bounded `internal/yamlutil`; strict YAML/environment `internal/config`; action descriptor/schema/credential preflight, directory loading and deterministic normalization in `internal/registry`; single-node `internal/storage/sqlite` with embedded/checksummed transactional migrations, immutable revisions, atomic batch publication, lifecycle persistence and health checks. CLI exposes init/validate/plan/apply and workflow list/get/revisions/export/delete/enable/disable while preserving help/version/completion and injected output streams. Production supplies no action descriptors or implementations; no execution/trigger/server behavior is claimed.
- Security/reliability review: source/config/graph/expression/batch/result bounds; no raw values in config/storage diagnostics; credential literals and nested sensitive output references rejected; parameterized SQL; adapter-owned DSN/pragmas; private new file/directory modes where supported; archive/history and same-owner foreign-key invariants; original source retained; context cancellation and cleanup errors propagated; structured debug events omit settings and detect short/failed writes. Rejected preflight leaves no database. Output failure after commit reports failure without losing the durable revision. Listings are bounded to 10,000 records/64 MiB and fail without partial results. No tests removed or assertions weakened.
- Added registry, configuration, SQLite, CLI integration and YAML utility tests. Observable suites include `TestRegistryPersistenceAndHistory`, `TestBatchRollbackAndRevisionConstraints`, `TestConcurrentApplyAndCancellation`, `TestMigrationFailurePreservesState`, `TestStorageErrorsAndListing`, `TestInsertFailureRollsBackAndRestore`, `TestCorruptStoredDefinitionIsReported`, `TestListingLimitIsReportedWithoutPartialResult`, `TestPrepareCatalogValidation`, `TestCredentialsAndDescriptors`, `TestSensitiveOutputReferences`, `TestNormalizationPreservesReferenceObjects`, `TestLoadDefaultsAndPrecedence`, `TestConfigRejectsUnsafeInput`, `TestInitExclusiveAndCanceled`, `TestFoundationPreflightHasNoStorageSideEffects`, `TestFoundationCLIEndToEnd`, `TestFoundationOutputFailureAfterCommit`, `TestDebugDiagnosticsAndEmptyConfig` and `TestStrictLoad`. All use isolated resources; concurrent tests own/join bounded goroutines and use context deadlines rather than sleeps.
- Added [foundation guide](../docs/foundation.md) covering installation/quick start, config/env, CLI, workflow/phase limits, security, SQLite operation, backup/restore, upgrades/rollback and troubleshooting. Updated parser docs, README and contribution instructions. Added [manual self-review](reviews/phase-01.md). Added migration paths to both Go CI event filters so SQL-only changes run tests; `actionlint` passed. Remote CI is still unverified for this worktree.
- Dependency: pinned [modernc.org/sqlite v1.62.0](https://pkg.go.dev/modernc.org/sqlite@v1.62.0), CGO-free backend with Go 1.26.0 requirement. Its exact `modernc.org/libc v1.80.1` dependency is retained; OpsWeave remains Go 1.27.3. Download/tidy/checksum verification completed with permitted official proxy/checksum access after a sandbox DNS failure. Runtime does not require the portable C compiler, Redis or RabbitMQ.
- Race environment repair: downloaded the [official LLVM-MinGW 20261020 UCRT x86_64 release](https://github.com/mstorsjo/llvm-mingw/releases/tag/20261020) into ignored `.cache`, verified the release API SHA-256 `fd5462b7f5d93b2ff981fcd3f9c7e427e47f3203c64ea91c27bcc444eebd74cd` (190,612,575-byte archive), and confirmed clang 23.1.4. Only test-process `CC`, `CGO_ENABLED` and `PATH` were changed; no system installation/settings or Go version change. Sandbox race execution failed ThreadSanitizer address allocation (Windows error 87, exit 1). Final supported unsandboxed race execution passed, resolving the old missing-CGO/compiler gate.

Final verification used workspace `GOTMPDIR=.cache/go-tmp`. Database-heavy checks were run sequentially for final acceptance evidence. Test commands returning 0 still skip the two symlink cases listed below.

| Command/check                                                                                     | Exit | Evidence                                                                                                                                                                |
| ------------------------------------------------------------------------------------------------- | ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `gofmt -l .`                                                                                      | 0    | No files reported                                                                                                                                                       |
| `go vet ./...`                                                                                    | 0    | No diagnostics, including final CLI diagnostic writer change                                                                                                            |
| `go test -count=1 ./...`                                                                          | 0    | CLI, config, registry, SQLite, workflow and YAML utility suites passed; two symlink cases skipped                                                                       |
| `go test -count=1 '-coverprofile=.cache/foundation.cover' ./internal/...`                         | 0    | CLI 93.4%, config 94.7%, registry 85.3%, SQLite 81.1%, workflow 98.1%, YAML utility 95.8%                                                                               |
| `go test -race -count=1 ./...` with verified compiler, outside sandbox                            | 0    | All packages passed; two symlink cases still skipped                                                                                                                    |
| `go mod tidy`                                                                                     | 0    | Module files consistent after intentional dependency changes                                                                                                            |
| `go mod verify`                                                                                   | 0    | All modules verified                                                                                                                                                    |
| `go build ./...` and smoke binary build                                                           | 0    | Both built; root-module stat-cache write warning (access denied) remains a local environment limitation                                                                 |
| `govulncheck ./...` with official database access                                                 | 0    | No vulnerabilities found                                                                                                                                                |
| `go test ./internal/workflow -run '^$' -fuzz '^FuzzParse$' -fuzztime=10s -parallel=1`             | 0    | 56,099 executions; four seeds + 134 interesting inputs; no failure                                                                                                      |
| `go test ./internal/workflow -run '^$' -fuzz '^FuzzValidateCondition$' -fuzztime=10s -parallel=1` | 0    | 154,835 executions; four seeds + 37 interesting inputs; no failure                                                                                                      |
| `go test ./internal/config -run '^$' -fuzz '^FuzzConfig$' -fuzztime=10s -parallel=1`              | 0    | 25,568 executions; four seeds + 88 interesting inputs; no failure                                                                                                       |
| `go test ./internal/registry -run '^$' -fuzz '^FuzzPrepare$' -fuzztime=10s -parallel=1`           | 0    | 83,834 executions; three seeds + 117 interesting inputs; no failure                                                                                                     |
| Built-binary smoke script                                                                         | 0    | Text help, JSON version/init, unavailable-action rejection with no database, and empty SQLite listing verified                                                          |
| `actionlint .github/workflows/ci.yml`                                                             | 0    | Migration path filters valid                                                                                                                                            |
| Ledger/link validation                                                                            | 0    | 561 unique IDs/owners; phase 01 has 68 implemented and two in-progress rows, zero completed; 1,455 local links resolved before this log append; phases 02–10 unapproved |

- Intermediate failed checks were not treated as passes: simultaneous heavy unit/coverage/race/fuzz commands caused `TestConcurrentApplyAndCancellation` to exceed SQLite's five-second busy timeout in unit and coverage runs (exit 1, `SQLITE_BUSY`). Sequential final commands passed without changing that timeout, bypassing locking or weakening assertions. The test context deadline is 30 seconds, while each storage operation retains the documented five-second busy timeout. Initial `go mod tidy` failed official proxy DNS in the sandbox (exit 1); permitted network retry passed (exit 0).
- Open security gate: `TestInitRefusesSymlinkAndConfigDirectory` and `TestReadSourcesRejectsSymlinks` skipped because Windows denied `os.Symlink`. Targeted verbose checks reproduced the skip both inside and outside the sandbox (commands exit 0, tests not executed to completion). Docker is unavailable; unsandboxed `wsl --list --quiet` returned no installed distribution. Windows security settings were not changed. Linux CI evidence is required before marking these paths/phase completed.
- Status: phase 01 implementation and self-review prepared, `PHASE_01_AWAITING_LINUX_VERIFICATION`. All 70 phase-owned rows have scoped tests/docs/review/log evidence: 68 `IMPLEMENTED`, two broad security rows `IN_PROGRESS`; no requirement marked `COMPLETED`. Later runtime semantics remain gated, not claimed shipped. The initial migration introduces a new foundation schema; there is no previous supported database to convert, no down migration and no released documentation rewrite.
- Next human decision: publish the concrete reviewed worktree to `codex/phase-01-foundation` and a draft PR so existing Linux CI can verify symlink cases and the full quality gates. No remote publication occurred. Remain within phase 01 until verification/acceptance; do not start phase 02. Final Markdown/diff checks run after this append.

## 2026-11-08 — Phase 01 publication authorized

- User explicitly requested a commit and PR targeting `main` before the next phase. This authorizes creating `codex/phase-01-foundation`, committing the reviewed Phase 01 worktree, pushing it and opening a PR against `main`. It does not authorize merge or Phase 02 implementation.
- Reconfirmed baseline `d7b89f6`, branch `main`, scoped tracked/untracked source/doc/test files and migration CI changes; ignored compiler/cache/smoke artifacts remain excluded. `git diff --check` exited 0 with the existing ledger newline-normalization warning. Earlier executed checks remain attributed to the implementation log; no source or dependency changes since those final checks.
- Updated current authorization notes. Markdown/diff/staged-file checks precede the commit. Next: publish the branch/PR, inspect actual CI results and resolve relevant failures within Phase 01. Linux symlink evidence and phase acceptance remain open until verified; no merge or next-phase advancement is implied.

## 2026-11-08 — Phase 01 branch and PR published

- Created commit `eb97043ff145ad022a5f5931ef375f3b0edd48a9` (`feat: implement phase 01 workflow foundation`), pushed `codex/phase-01-foundation` and opened [draft PR #3](https://github.com/PHPDevsr/opsweave/pull/3) targeting `main`, as explicitly requested. Local HEAD and origin branch SHA matched; worktree was clean after publication. PR is not merged; Phase 02 remains unapproved.
- Initial PR workflows started: CI, Markdown and CodeQL. During CI evidence review, strengthened the two symlink tests so only Windows may skip a creation failure; Linux/macOS now fail if setup cannot create a symlink. Added an explicit verbose Linux symlink CI step to record real execution of both tests. This closes a verification ambiguity without changing product behavior or relaxing any assertion.
- Local affected config/registry suites passed (`go test -count=1 ./internal/config ./internal/registry`, exit 0); Windows still skips the privileged symlink setup. `actionlint` and gofmt reported no diagnostics. Markdown/diff checks precede the follow-up commit. Actual CI results are pending; the initial CodeQL failure is being investigated rather than treated as a passing gate.

- CodeQL investigation: [initial PR run](https://github.com/PHPDevsr/opsweave/actions/runs/42437308206), job `128598818711`, completed build/analysis and failed result upload with "Advanced Security must be enabled for this repository to use code scanning". This is an open repository configuration gate requiring an owner decision; no security settings, upload enforcement or workflow failure handling were weakened. Linux verification continues on the updated branch.

## 2026-11-08 — Remote Phase 01 verification and owner gate

- PR #3 targets `main`, remains draft/unmerged and contains the approved Phase 01 implementation plus stricter Linux verification. Tested commit: `f10f59678f8ad59b129b28bd16b882ef82ea50c8` (`test: require symlink security checks on Linux`).
- [CI run 42437553488](https://github.com/PHPDevsr/opsweave/actions/runs/42437553488) completed successfully: formatting, module consistency, race, and all six Linux/Windows/macOS minimum/stable Go jobs. Both Ubuntu jobs passed the explicit Linux symlink step. Job `128599556834` logs explicitly show `--- PASS` for `TestInitRefusesSymlinkAndConfigDirectory` and `TestReadSourcesRejectsSymlinks`; the earlier Windows privilege gate is closed by actual execution, not configured CI alone. [Markdown run 42437553511](https://github.com/PHPDevsr/opsweave/actions/runs/42437553511) succeeded.
- [CodeQL run 42437553495](https://github.com/PHPDevsr/opsweave/actions/runs/42437553495), job `128599556787`, failed result upload with the confirmed error "Advanced Security must be enabled for this repository to use code scanning". This is a repository configuration gate requiring an owner decision. No settings or failure enforcement were changed, and no clean CodeQL result is claimed.
- Updated current master/phase/operating/review notes and the two security rows with actual Linux evidence. All 70 Phase 01 owned rows are `IMPLEMENTED`; none are `COMPLETED` pending the remaining quality gate and phase review. State: `PHASE_01_AWAITING_CODE_SCANNING`. All later phases remain unapproved and unstarted.
- These follow-up changes are documentation/evidence only; no product/dependency/test behavior changes after `f10f596`. Markdown, ledger/link and diff checks precede the evidence commit, which remains on the same authorized PR branch. Record the final PR status in its description; do not merge or begin Phase 02.

## 2026-11-08 — Foundation benchmarks and benchmark CI

- User requested benchmark tests and their CI before continuing. Scope remains Phase 01 and existing PR #3; no engine/scheduler/worker implementation, optimization, dependency change or phase advancement is authorized by this extension. Starting commit `c5e8022`, branch `codex/phase-01-foundation`, clean worktree.
- Added seven Go benchmark families with 17 cases: `BenchmarkParse` (1/32/256 steps), `BenchmarkValidateCondition` (literal/predicates/128 terms), `BenchmarkPrepare` (1/32/256 steps and 16 workflows), `BenchmarkSQLiteApply` (1/16 revisions per transaction), `BenchmarkSQLiteGet` (current/historical from 128 revisions), `BenchmarkSQLiteList` (1/128 workflows) and `BenchmarkSQLiteMigrate` (fresh database creation/migrations/close). Fixtures use isolated temporary resources and validation-only catalog descriptors. Allocation reporting is enabled; Go `b.Loop` and observable result checks prevent dead-code elimination without shared mutable sinks.
- SQLite uses production durability/foreign-key settings. Apply databases are capped at 128 total revisions; resets close/remove only the owned temporary file and migrate outside the apply timer. The separate migration benchmark includes open/migrate/close. A forced `Batch16` nine-iteration run crossed the reset boundary and verified revision numbers restart correctly. Read datasets are fixed before timing. No source or security behavior was changed.
- Added `.github/workflows/benchmarks.yml`: relevant PRs, pushes to `main` and manual dispatch; Linux/declared Go; `-cpu=1`, `-benchtime=200ms`, `-count=3`, `-benchmem`. All seven family names must occur. Pipe failures propagate and partial reports are uploaded even if benchmarks fail, without masking that failure. Reports contain timing/allocations plus selected commit/Go/kernel/CPU metadata; no environment dump, database, credentials or PR comments. Artifact retention requests 14 days. Actions are SHA-pinned, token permissions are read-only and runner duration is capped at 15 minutes.
- Verified official [upload-artifact v7.0.2](https://github.com/actions/upload-artifact/releases/tag/v7.0.2) resolves to commit `cf430e030ddbb5b0abf93d22962f4752f3646cd9` through GitHub's release-tag API; existing checkout/setup-go pins were reused. No Go/npm dependency added. Timing values establish a baseline without a hardware-dependent absolute failure threshold; benchmark errors/missing families still fail CI.
- Added [benchmark guide](../docs/benchmarks.md), README/contribution links, phase notes and benchmark self-review. Recurring policy rows `G-8-019`/`G-9-010` are `IN_PROGRESS` with this scoped evidence; future component measurements remain pending. Phase 01 stays awaiting the pre-existing CodeQL repository configuration gate; no requirement is marked completed.

Local verification used Go 1.27.3, Windows/amd64, AMD Ryzen 5 5600X, one benchmark logical CPU setting, workspace GOTMPDIR and no performance instrumentation/concurrent heavy suites. These are local observations, not Linux CI or production guarantees.

| Command/check                                                                                                                                        | Exit | Evidence                                                                                                                |
| ---------------------------------------------------------------------------------------------------------------------------------------------------- | ---- | ----------------------------------------------------------------------------------------------------------------------- |
| `go test -run '^$' -bench '^Benchmark' -benchmem -benchtime=1x -count=1 -cpu=1 ./internal/workflow ./internal/registry ./internal/storage/sqlite`    | 0    | All 17 cases executed successfully                                                                                      |
| `go test -run '^$' -bench '^BenchmarkSQLiteApply/Batch16$' -benchmem -benchtime=9x -count=1 -cpu=1 ./internal/storage/sqlite`                        | 0    | Reset boundary exercised and validated with production storage settings                                                 |
| `go test -run '^$' -bench '^Benchmark' -benchmem -benchtime=200ms -count=3 -cpu=1 ./internal/workflow ./internal/registry ./internal/storage/sqlite` | 0    | Final 17 cases / 51 timing samples; allocation metrics emitted; ignored local report `.cache/benchmark-local-final.txt` |
| `gofmt -l .`                                                                                                                                         | 0    | No files reported                                                                                                       |
| `go vet ./...`                                                                                                                                       | 0    | No diagnostics                                                                                                          |
| `go test -count=1 ./...`                                                                                                                             | 0    | All package suites passed; Windows symlink privilege skips remain as documented, with prior Linux execution evidence    |
| `go test -race -count=1 ./...` with verified compiler outside sandbox                                                                                | 0    | All packages passed                                                                                                     |
| `go mod verify`                                                                                                                                      | 0    | All modules verified; module files unchanged                                                                            |
| `go build ./...`                                                                                                                                     | 0    | Existing root-module stat-cache access-denied warning remains                                                           |
| `actionlint .github/workflows/benchmarks.yml`                                                                                                        | 0    | Workflow syntax/actions/expressions checked                                                                             |
| `npm run check:markdown` and `git diff --check`                                                                                                      | 0    | Formatted; existing LF/CRLF normalization warnings only                                                                 |

Illustrative local median observations (three samples, ns/op): Parse/Steps1 23,144; Parse/Steps256 6,357,862; Prepare/Batch16 540,025; SQLiteApply/Batch1 108,330,750; SQLiteApply/Batch16 118,229,850; SQLiteGet/Current 204,071; SQLiteMigrate 123,879,500. SQLite timing includes this Windows filesystem's durable commit latency. Compare like environments and repeated samples; no optimization decision or runtime SLA follows from these values.

- Next: commit/push this tested extension to the already authorized PR branch, verify the new benchmark workflow and uploaded reports, and attach actual CI evidence in PR #3. Keep CodeQL's owner gate visible and do not start Phase 02 or merge the PR. Final docs/link/diff checks precede publication.

## 2026-11-08 — Markdown CI migrated to native Bun formatter

- User explicitly requested replacing Markdown CI with Bun using `bun fmt` before continuing. Scope stays on PR #3 / Phase 01; starting commit `c208634`, clean worktree. No next-phase work, runtime behavior, Go source, module dependency or historical release documentation changes.
- Replaced setup-node/npm installation/check scripts with SHA-pinned `oven-sh/setup-bun v2.2.0` and `bun fmt --check "**/*.md"`. Added `.bun-version` pin `1.4.4` and included it in both workflow event path filters. Check mode retains the existing non-writing formatting gate. Read-only workflow permissions and timeout/cancellation remain in place.
- Removed the formatter-only `package.json`/`package-lock.json` and their obsolete npm Dependabot entry. No `bun install`, `bunx` or external Prettier package is needed. Kept `.prettierrc.json`/`.prettierignore` as native Bun-compatible style/ignore configuration. `bun fmt --doctor` confirmed 31 Markdown files are formatted natively; no repository-wide style rewrite was needed. Updated README, contribution and harness verification commands; previous npm/Prettier results in this append-only log retain their original attribution.
- Verified [official setup-bun release](https://github.com/oven-sh/setup-bun/releases/tag/v2.2.0) resolves to commit `0c5077e51419868618aeaa5fe8019c62421857d6`. Downloaded the [official Bun 1.4.4 Windows x64 release](https://github.com/oven-sh/bun/releases/tag/bun-v1.4.4) into ignored `.cache`, checked SHA-256 `cd2c0fccc78f3ede1f50ca827a15f063eda13d4d150a3d0674f4f6e323d864a5` and confirmed version 1.4.4. No global install or shell profile change. Native check mode behavior follows the [formatter documentation](https://bun.sh/docs/format).
- Verification: `bun fmt --check "**/*.md"` exited 0; formatter doctor exited 0. An isolated temporary Markdown probe intentionally failed check mode with exit 1, retained its exact hash, was corrected by `bun fmt` (exit 0), then passed check mode (exit 0); the probe was removed in guaranteed cleanup. `actionlint .github/workflows/markdown.yml` exited 0; diff/link checks precede publication. No local Go suite rerun is claimed for this tooling/docs-only change; its last actual passing results remain attributed above, and normal PR CI can reverify them.
- Manual review: no package script can intercept the native command after manifest removal; pinned Bun/action versions make the toolchain explicit, Markdown-only globs preserve Go/YAML formatter scope, and existing formatting rules match natively. No security gate or assertion was weakened. Next: commit/push to the already authorized PR branch, verify a fresh Markdown job without npm dependencies, and attach actual CI evidence in PR #3. The separate repository CodeQL owner gate and phase acceptance remain unchanged.

- Updated only the Markdown verification command in current phase 00–10 checklists so they no longer reference removed npm scripts. Requirement ownership, approval states and future phase scope remain unchanged; historical assessment/log entries retain their original tool references.

## 2026-11-08 — Merged foundation synchronized; Phase 02 contract prepared

- Authorization: the user stated the branch was merged and requested "git fetch origin & rebase origin/main. Next phase". This accepts Phase 01 and authorizes Phase 02. It does not authorize Phase 03 or mark unavailable checks passed. PR #3 is merged at `401935d1ff13624aa7f02cf40669bf49bb4067f3`. CodeQL's repository Advanced Security upload configuration remains an independent open gate; no settings or enforcement changed.
- Ran `git -c core.fsmonitor=false fetch origin` (exit 0), then `git -c core.fsmonitor=false rebase origin/main` (exit 0), without conflicts. Created `codex/phase-02-manual-execution` (exit 0). Verified HEAD and `origin/main` both identify the merge commit. The worktree was clean before Phase 02 documentation edits. A plain status command emitted the existing fsmonitor IPC diagnostic; disabling fsmonitor per command avoids it without changing repository configuration.
- Re-read GOALS/AGENTS and phase/contract instructions, inspected workflow capability checks, registry descriptors, CLI composition, SQLite migration and existing tests. No action executor or execution store exists yet. The first Phase 02 task requires state/action contract review before runtime implementation; prepared [manual execution proposal](designs/manual-execution.md) with four concrete decisions, full transition tables, transaction/ownership invariants, error categories, capture/redaction bounds, CLI behavior and acceptance tests to implement.
- Design self-review: retain the existing sequential-chain/literal-input capability boundary; preserve exact revisions/action versions and empty production catalog. Commit state before action invocation, reject unowned concurrent submissions, and preserve unfinished state after crash without automatic side-effect replay. Match the accepted workflow contract's first-persisted terminal outcome for simultaneous cancellation/deadline cases. Default context deadlines and cooperative cancellation are mandatory now; configurable reliability policies and recovery remain Phase 03. No external integration, public SDK, runtime dependency or historical migration changed.
- Updated master/harness/phase review states together and the Phase 01 review to reflect actual user acceptance and next-phase authorization. Marked only 24 state/action design rows `IN_PROGRESS` with proposal evidence; runtime acceptance remains pending. All 561 stable IDs, source/context, priorities, owners and prerequisite assignments are unchanged. Other Phase 02 rows remain planned; no requirement was marked implemented or completed by this work.
- Verification: native Bun formatting exited 0. A read-only Python reconciliation exited 0: 561 unique stable IDs; all phase ownership lists match the ledger; exactly 24 planned-to-in-progress design changes; source/ownership unchanged; all changed-document local links resolve; HEAD equals `origin/main`. Final repository-wide `bun fmt --check "**/*.md"` and `git diff --check` results follow below. This is design/documentation work only; no new Go test, benchmark, race or CodeQL success is claimed. Existing test evidence retains its original commit attribution.
- Next required human decision: review the four choices in the proposal. Harness operating instructions require: "If a required contract is unresolved, complete its design task and seek the relevant decision before implementing it." Phase 02 authorization is already recorded and is not being requested again. After contract review, implement state machine, persistence, engine and CLI with the listed tests, benchmarks, documentation and review before feature completion. Stop before Phase 03.
- Final checks: repository-wide native `bun fmt --check "**/*.md"` exited 0; `git -c core.fsmonitor=false diff --check` exited 0 with existing CRLF-to-LF normalization warnings only. Bun realigned ledger tables around changed evidence/status widths; reconciliation confirms unchanged IDs, source text and ownership. Changes remain uncommitted on the Phase 02 branch pending contract review.

## 2026-11-08 — Accepted Phase 02 implemented and locally verified

- Authorization: user reply "ACCEPT" approves all four manual execution contract decisions. Scope remains Phase 02 on `codex/phase-02-manual-execution`, based on merged commit `401935d`. No Phase 03 work, new action integration, public SDK, server or configuration security bypass is authorized or implemented.
- Added SQL-independent execution/step state tables and aggregate validation; trusted immutable action registrations with exact versions/grants, schema validation, copied inputs and bounded/redacted outputs; synchronous manual dispatcher/engine; additive SQLite migration 002 and execution/step/event repositories; and `run` plus `execution list/get/logs`. Submission and step starts commit before action invocation; completion/output/events commit atomically. Active workflow uniqueness, private ownership/version guards and append-only events protect durable state. Existing migration 001 and module files are unchanged.
- Added exhaustive state-pair tests, engine E2E/history/reopen tests, actual SQL failure/rollback tests, concurrent submissions through separate store handles, stale ownership, output redaction/limits, context/deadline and cleanup tests, preflight corruption/version/permission tests, ambiguous commit tests and CLI text/JSON/output failure tests. Production still supplies no action implementation; deterministic handlers are test-only. Restart history is readable; an unfinished run remains active and is never automatically replayed.
- Initial targeted tests caught two issues that were corrected before final checks: SQL driver strings cannot scan directly into Go 1.27's raw JSON value type, so historical definitions now scan into a string before conversion; the old unknown-schema fixture must alter only migration version 1 after migration 002 exists, avoiding an unrelated duplicate-version error. Assertions and durability settings were retained. An initial unquoted PowerShell coverage option was split into an extra package argument; the correctly quoted command below passed. Interim `gofmt -l .` calls while Go tests/benchmarks were running included generated ignored `.cache/go-tmp` files; final formatting is checked after processes finish.
- Added transition, durable two-step manual execution and execution-history/log benchmarks. The existing pinned read-only benchmark workflow now executes five packages and requires ten families. Full local run produced 63 samples across 21 cases. Temporary write history is capped at 128 runs per database; reset/migration is outside execution timing. No optimization, timing threshold or SLA is claimed.
- Updated [execution guide](../docs/execution.md), foundation/parser guides, README, benchmark guide, accepted contract, phase authorization states, [self-review](reviews/phase-02.md) and ledger evidence. All 68 Phase 02 rows are `IMPLEMENTED`, none `COMPLETED`; all 561 IDs, source/context, ownership, priority and prerequisites remain unchanged. Phases 03–10 remain unapproved. CodeQL's independent repository Advanced Security configuration issue is still open.

| Verification                                                                                           | Exit | Evidence                                                                                                  |
| ------------------------------------------------------------------------------------------------------ | ---- | --------------------------------------------------------------------------------------------------------- |
| `go vet ./...`                                                                                         | 0    | All packages; no diagnostics                                                                              |
| `go test -count=1 ./...`                                                                               | 0    | Unit, SQLite integration, engine E2E, CLI and concurrency suites passed                                   |
| `go test -count=1 '-coverprofile=.cache/phase02-final-coverage.out' ./...`                             | 0    | Engine 94.5%, state machine 95.5%, parser 97.5%, action 96.7%, CLI 93.5%, storage 80.3%                   |
| `go test -race -count=1 ./...`                                                                         | 0    | All packages, using previously verified portable Clang outside sandbox                                    |
| `go test -race -count=1 ./internal/action` after new fuzz seeds                                        | 0    | New capture seeds included; same verified compiler                                                        |
| `go test -run '^$' -fuzz '^FuzzOutputCapture$' -fuzztime=3s -parallel=2 ./internal/action`             | 0    | 85,886 executions, 129 new interesting inputs; no failure                                                 |
| Full ten-family benchmarks, `-run '^$' -bench '^Benchmark' -benchmem -benchtime=200ms -count=3 -cpu=1` | 0    | Five documented packages; ignored report `.cache/phase02-benchmarks.txt`, 63 samples/21 cases             |
| `go mod verify`                                                                                        | 0    | All modules verified; dependency files unchanged                                                          |
| `go build ./...`                                                                                       | 0    | Existing module stat-cache access-denied warning remains; build succeeded                                 |
| `govulncheck ./...`                                                                                    | 0    | No vulnerabilities found using current advisory database                                                  |
| `actionlint .github/workflows/benchmarks.yml`                                                          | 0    | Workflow checked; existing action pins/permissions retained                                               |
| Ledger/phase/local-link reconciliation                                                                 | 0    | 561 stable IDs, exact phase ownership, 68 implemented Phase 02 rows; unchanged dependencies/migration 001 |

- Illustrative medians from the final local benchmark run: transition 50.28 ns/op; durable two-step manual execution 633,510,600 ns/op; exact execution get 252,508 ns/op; logs 380,896 ns/op. These include this Windows machine/filesystem and synchronous FULL commits, and are not comparable to real external action latency.
- Manual code/security review is recorded in the self-review: no raw action error/log capture, token exposure, unsafe replay, dependency weakening or scope expansion found. Handler memory/I/O and cooperative cancellation remain trusted implementation obligations. Final Markdown/diff/format checks and publication/actual remote evidence follow. Do not infer CI success from local checks, or mark requirements completed before applicable gates and phase review close.

- Final `gofmt -l .` exited 0 with no files after Go processes finished; native Bun repository-wide check and `git diff --check` exited 0 (existing line-ending warnings only). Prepared for the previously requested commit/PR targeting `main`; no Phase 03 work will begin.
- Publication is waiting for human GPG/pinentry interaction: Git has `commit.gpgsign=true`, and the commit process has launched its signer and pinentry but has not returned. All implementation changes are staged; signing has not been disabled or bypassed. The user was asked to complete the native signing dialog without sending a passphrase in chat. Push, PR creation and remote CI have not run yet. Resume after signing completes, inspect the commit outcome, include this publication note in subsequent evidence, then push/create the authorized PR targeting `main` and verify actual CI. This note was added after the pending commit's staging step.
- Signing gate resolved: the user confirmed completing the signing dialog. The pending commit returned exit 0 and created `a7f425d` (`feat: add durable sequential manual execution and history`), containing the 32 scoped implementation/test/documentation/CI files. Signing configuration was preserved. This follow-up records the interaction; next push the branch, open the authorized PR targeting `main`, and inspect actual remote checks. CI results will be attached to that PR, without inferring success from local evidence.

## 2026-11-08 — User-authorized multi-OS release packaging extension

- The user explicitly requested release CI with an OS matrix before Phase 03 so users can download ready-to-run binaries. This authorizes a narrow early Phase 10 packaging extension, not Phase 03 or general production qualification. Continued on the clean Phase 02 branch at `7204871`, with existing draft PR #4 still open. Its prior CI/benchmark/Markdown runs passed; CodeQL upload remained blocked by repository Advanced Security configuration, as attached to the PR.
- Added `.github/workflows/release.yml`: six OS/CPU archive targets, tested build hosts, CGO-free versioned binaries, extracted-archive checksum/target checks, four native version/help/SQLite smoke paths, complete six-archive assembly and a tag-only publication job. Repository metadata confirmed it is private; standard Linux/Windows ARM runners are unavailable for this case, so those two targets are cross-built on amd64 hosts. Both macOS targets and Linux/Windows amd64 use native runners. ARM runtime qualification is explicitly unclaimed; no paid runner or billing setting was introduced.
- Added standard-library `tools/release` with bounded tag validation, argument-array Go invocation, fixed archive metadata/modes, installation instructions, SHA-256 sidecars and exclusive output creation. Archive tests check content, executable mode, deterministic bytes and writer failures; target/version tests and fuzzing reject path/argument injection and malformed identifiers. Source CLI builds still report dev; a linker-assigned version symbol is never mutated at runtime. No module dependency or migration changed.
- Publication requires a valid intentional version tag on a commit reachable from main, successful matrix/assembly jobs, matching remote tag/built commit and verified archive hashes. Only that job receives contents write. PR/manual builds are read-only previews and cannot create releases. Existing releases are not overwritten; no tag, public release, signing secret or notarization credential was created during this work. Concurrency does not cancel publication. Source/ref values are validated and passed through environment variables rather than interpolated as shell code.
- Pinned new `actions/download-artifact v8.0.2` to verified official commit `9000827ccba6bdab643e8b6fd33ac0654aef8333`; existing checkout/setup-go/upload pins remain. Runner availability and `gh release create` behavior were checked against [GitHub runner documentation](https://docs.github.com/en/actions/reference/runners/github-hosted-runners), the [official action release](https://github.com/actions/download-artifact/releases/tag/v8.0.2) and [GitHub CLI manual](https://cli.github.com/manual/gh_release_create). Updated installation/release instructions, README/contribution guide, master plan, Phase 10 exception, review and the release-pipeline ledger row. All 561 IDs and ownership remain unchanged; only G-13-011 moves from planned to in progress, with full qualification pending.

| Verification                                                                               | Exit | Evidence                                                                                                                  |
| ------------------------------------------------------------------------------------------ | ---- | ------------------------------------------------------------------------------------------------------------------------- |
| `go test -count=1 ./...`                                                                   | 0    | Full existing suite plus release packager tests passed                                                                    |
| `go test -race -count=1 ./...`                                                             | 0    | All packages, including release tool, using verified portable compiler outside sandbox                                    |
| `go vet ./...`, `go mod verify`                                                            | 0    | No diagnostics; modules verified and files unchanged                                                                      |
| `go build ./...`                                                                           | 0    | Build succeeded with existing local module stat-cache permission warning                                                  |
| `go test -run '^$' -fuzz '^FuzzVersionFilename$' -fuzztime=3s -parallel=2 ./tools/release` | 0    | 78,256 executions, 46 new interesting inputs, no failure                                                                  |
| Windows preview packaging/extraction                                                       | 0    | Real archive hash matched; extracted binary reported v0.0.0-preview; SQLite creation/history listing succeeded            |
| Linux arm64 cross-package/extraction                                                       | 0    | Archive hash matched; Go binary metadata confirmed linux/arm64 and CGO_ENABLED=0; not executed on this Windows host       |
| `actionlint .github/workflows/release.yml`                                                 | 0    | Workflow checked after final tag/publication guards                                                                       |
| `gofmt -l .`, native Bun Markdown check, diff check                                        | 0    | No unformatted Go; Markdown passed; existing CRLF normalization warnings only                                             |
| Ledger/local-link/dependency reconciliation                                                | 0    | 561 stable IDs; only authorized release-pipeline status/evidence changed; links resolve; modules and migrations unchanged |

- Manual code/security review: no untrusted shell concatenation, secret dump, public plugin/integration change, arbitrary output overwrite or permission escalation in PR jobs. Production catalog stays empty. Fixed archive metadata is tested for identical input; no cross-toolchain reproducibility/signing guarantee is claimed. Tag publication itself is intentionally not exercised without a chosen release tag. Next: commit/push to the existing authorized PR, verify actual six-target Release CI and bundle assets, attach fresh evidence, and stop before Phase 03.

## 2026-11-08 — Phase 02 accepted; Phase 03 synchronized and execution design prepared

- Authorization: the user confirmed "phase 2 sudah saya merge" and requested "git fetch origin dan git rebase origin/main. Lanjut phase 3". This accepts Phase 02 and explicitly authorizes Phase 03; approval is not being requested again. PR #4 is confirmed merged at `e7d7177fe4eb028c91a8dae98322bfbb7ca2daf3`. The worktree was clean before this work unit. No release tags exist after fetching.
- `git -c core.fsmonitor=false fetch origin` exited 0 and moved origin/main from `401935d` to `e7d7177`. `git -c core.fsmonitor=false rebase origin/main` exited 0 without conflicts. Created `codex/phase-03-reliable-orchestration` with `git switch -c` (exit 0); HEAD equals origin/main. Preserve the old merged branch; no force push, merge or tag publication was performed.
- Recorded actual pre-merge evidence from PR #4: [CI 42478643637](https://github.com/PHPDevsr/opsweave/actions/runs/42478643637), [benchmarks 42478643623](https://github.com/PHPDevsr/opsweave/actions/runs/42478643623), [native Bun Markdown 42478643651](https://github.com/PHPDevsr/opsweave/actions/runs/42478643651), and [six-target release preview/assembly 42478643656](https://github.com/PHPDevsr/opsweave/actions/runs/42478643656) passed on `7aa93ed`. The [preview bundle](https://github.com/PHPDevsr/opsweave/actions/runs/42478643656/artifacts/13616627733) contains six archives and checksums; publish correctly skipped for a PR. [CodeQL 42478643613](https://github.com/PHPDevsr/opsweave/actions/runs/42478643613) failed upload because Advanced Security must be enabled. User acceptance/advancement does not resolve that check, qualify ARM runtime or prove live tag publication; those claims remain unmade.
- Read GOALS, AGENTS, phase plans, operating rules, accepted workflow/manual contracts, engine/domain/configuration code, migration 002 and existing condition/engine tests. The first ordered Phase 03 task requires review of execution policies and retry/replay identity. Prepared [reliable execution proposal](designs/reliable-execution.md) with four concrete decisions: durable attempt history and conditional aggregate outcomes; conservative uncertain-effect recovery; new-ID exact-revision replay plus seven-day admission deduplication; and durable cancellation requests distinct from terminal completion.
- Design self-review identified necessary changes that must not be treated as ordinary Phase 02 behavior: condition evaluation failure needs a guarded pending-to-failed edge; successful conditional workflows may contain legitimately skipped steps; individual cancellation needs an aggregate policy; 256 steps with ten attempts cannot fit the legacy 1,024-event cap. Proposed execution format 2 uses 16,384 bounded events while retaining old format limits/history. Terminal states remain immutable, deadlines/attempts survive recovery, and no uncertain external action is automatically repeated. Parallelism/non-default concurrency remain gated until bounded capacity and safe claims are implemented together. Scheduler/trigger/listener and ownership contracts are later work units requiring their own concrete review before exposure; no illustrative trigger schema was promoted into an API.
- Updated master/harness/README and phase review states together, recorded Phase 02 user acceptance independently from CodeQL, and marked only 21 Phase 03 execution-design rows `IN_PROGRESS` with proposal evidence. All 561 IDs, source/context, ownership, priorities and prerequisites are unchanged; other statuses remain as before. No row was marked implemented or completed, no runtime source/migration/configuration was edited, and no Phase 04 work is authorized.
- Verification: native `bun fmt --check "**/*.md"` exited 0; `git diff --check` exited 0 with existing CRLF-to-LF normalization warnings only. Read-only Python reconciliation exited 0: 561 unique stable IDs, exact phase ownership lists, exactly 21 planned-to-in-progress design rows, unchanged source/ownership/prerequisites, valid changed-document local links, documentation-only diff and HEAD equal to origin/main. No new Go test, race, benchmark or security-scan success is claimed for this design-only work; earlier results remain attributed to their tested commits.
- Next required human decision: review the four new execution choices in the proposal. The operating rule is: "If a required contract is unresolved, complete its design task and seek the relevant decision before implementing it." Phase 03 itself is already authorized. After contract review, implement the first scoped reliability unit with tests, code/security review, documentation and required verification, then continue eligible work only within Phase 03. Current design changes remain uncommitted pending that decision.

## 2026-11-08 — Accepted Phase 03 serial reliability core implemented and reviewed

- Authorization: the user's "ACCEPT" approves the four reliable execution decisions prepared on base `e7d7177`. Implemented only their serial scope on `codex/phase-03-reliable-orchestration`; parallel capacity and automatic recovery remain gated. No executable autonomous service, built-in fixture action, listener, release tag or Phase 04 work was introduced.
- Added bounded condition evaluation and typed copied output resolution; format-2 guarded policies, persisted attempts/deadlines and capped retry; durable execution/step cancellation; exact-revision new-ID replay; and atomic hashed seven-day admission deduplication. The production CLI uses the reliable coordinator. Legacy format-1 behavior/history remain covered. Cancellation acknowledgement is distinct from terminal completion, and deduplicated receipts cannot invoke actions.
- Added migration 003 only: atomic event-capacity extension, separate immutable attempt history, policy metadata and hashed admissions. Versions 001/002 and module files are unchanged. Upgrade/reopen tests preserve actual version-2 history; injected migration/event failures roll back schema, snapshots, attempts, admission and history. Ambiguous commit/control failures stop further invocation without claiming effects were undone.
- Manual code/security review and test mapping are in [core review](reviews/phase-03-core.md); commands, limits, compatibility, backup and rollback are in [reliability guide](../docs/reliability.md). This is a self-review, not an independent reviewer. Fixed the version-1 fixture setup to remove all later migration tables/ledger entries, replaced an incorrect event-count expectation with the exact eleven-event sequence, and corrected malformed YAML in a new test fixture; validators and assertions were not weakened. Final review corrected ownership/CLI comments to match the actual serial scope.

| Verification                                                                         | Exit | Evidence                                                                                                                                                                                                                    |
| ------------------------------------------------------------------------------------ | ---- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go test -count=1 -coverprofile=.cache/phase03-current-coverage.out ./...`           | 0    | Full unit/integration/E2E suite; engine 86.5%, state machine 85.3%, parser 94.0%, action 96.9%, CLI 91.8%, registry 83.8%, SQLite 78.5%; core target exceeded                                                               |
| `go test -race -count=1 ./...`                                                       | 0    | Full suite outside sandbox with verified portable Clang/CGO; concurrent deduplication, cancellation, ownership and owned watcher shutdown passed; later SQL owner/control negative test separately passed unit verification |
| `go vet ./...`, `go mod verify`, `go build ./...`                                    | 0    | Modules verified; existing build stat-cache access-denied warning persists despite successful exit                                                                                                                          |
| `govulncheck ./...`                                                                  | 0    | No vulnerabilities found in existing dependencies                                                                                                                                                                           |
| Three bounded fuzz runs, `-fuzztime=3s -parallel=2`                                  | 0    | FuzzEvaluateCondition: 73,804 executions; FuzzResolveOutput: 19,361; FuzzOutputCapture: 69,676; no failures                                                                                                                 |
| Five-package benchmark command from CI, `-benchmem -benchtime=200ms -count=3 -cpu=1` | 0    | All 25 cases / 75 samples passed; thirteen required families now include evaluation, reliable policy and format-2 history; ManualExecution measures production format 2                                                     |
| `actionlint .github/workflows/benchmarks.yml`                                        | 0    | Updated benchmark-family assertion is valid                                                                                                                                                                                 |

- The full verification used Go 1.27.3 and isolated test databases. Windows symlink-privilege-dependent cases remain platform skips; no Linux security-path or ARM-runtime success is inferred from this host. Benchmarks measure this workstation and do not establish an SLA or worker/scheduler throughput.
- Sixteen precise serial-core/CLI ledger rows are now `IMPLEMENTED`, with review/tests/docs evidence. Parallelism, concurrency, active recovery and other Phase 03 rows remain incomplete; all 561 stable IDs, requirement text, ownership and prerequisites are preserved. No requirement is marked `COMPLETED`; independent remote CI/CodeQL and phase acceptance gates remain open. Fixed a detected encoding error in ledger source labels by restoring unchanged static columns from HEAD and reconciling every row.
- Prepared the next [single-node dispatcher contract](designs/single-node-dispatcher.md): native single-owner lock plus fenced generations; fixed bounded durable queue/pool; conservative recovery and explicit unmanaged ownership adoption. The operating instruction requires: "If a required contract is unresolved, complete its design task and seek the relevant decision before implementing it." Stop before that dependent implementation and request review of the concrete three decisions. Phase 03 itself is already authorized.
- Next: final Markdown/format/diff/link checks, signed commit and draft PR targeting main with actual CI evidence. No force push, merge or release publication is authorized by this unit.
- Final local checks: `gofmt -l .` produced no paths; native `bun fmt --check "**/*.md"`, `git diff --check`, changed-document link checks and 561-row/static-field reconciliation exited 0. Historical migration bytes and module files match HEAD. Git reports existing CRLF normalization warnings only.

## 2026-11-08 — Accepted bounded dispatcher, queue and conservative recovery

- Authorization: the user replied "ACCEPT" to the concrete three-decision dispatcher proposal after the serial-core report. Work continues on `codex/phase-03-reliable-orchestration`, based on signed core commit `b223699` and draft PR #5. No user edits existed at start. Accepted native single-owner locking/fenced generations, bounded queue/pool and explicit unmanaged ownership adoption; Phase 03 is already approved and later phases remain unapproved.
- Implemented native canonical database locking for Linux/macOS/Windows, transactional dispatcher generations, durable queue admission/claim with exact immutable revision bindings, workflow/global concurrency and independent queue expiry. Fixed worker/result bounds and logical active-step limits permit parallel DAG traversal; backoff releases a worker while retaining its logical slot. Runtime action catalog remains empty in production.
- Added migration 004 only, preserving 001–003: queue/ownership/audit metadata and replacement of the legacy active-workflow index. Admission/claim fault injection tests roll back partial state/events/admissions; a reconstructed version-3 upgrade and migration-conflict rollback preserve history, ledger and original index. Old version-1/version-2 upgrade fixtures now remove all later tables/ledger records without weakening assertions. Promoted the existing verified `golang.org/x/sys v0.49.0` dependency from indirect to direct; no version or minimum Go change, and go.sum is unchanged.
- Server recovery preserves deadlines, consumed attempts, successful outputs and backoff due times. Unfinished invocations fail with explicit uncertain-effect metadata and are never automatically repeated. Pre-dispatcher running rows require deliberate audited adoption after all old executors stop. Whole-workflow replay has a new ID and separate uncertain-effect acknowledgement. Stale generations cannot mutate managed work or release a newer dispatcher.
- Added `run --enqueue`, `execution retry --enqueue`, and a queue-only `server --adopt-unmanaged` option. Synchronous runs require ownership before admission and never drain unrelated old queue entries. Explicit cancellation/history remain available through separate SQL handles. Optional strict execution settings and fixed named environment overrides are documented in [dispatcher operations](../docs/dispatcher.md).
- Code/security self-review and detailed tests are in [dispatcher review](reviews/phase-03-dispatcher.md). Separate real-process lock tests verify one winner, busy, crash release and aliases; hard-linked databases/sidecars are rejected. Unix sidecars enforce 0600; Windows applies/tests a protected current-user/SYSTEM DACL. Lock files are never unlinked. No native permission fallback grants a permissive acquisition.
- Integration tests exposed and fixed successful admission error wrapping and sentinel identity changes during cleanup. Shutdown tests exposed a request-cancellation/SQL race; bounded metadata budgets now survive parent cancellation long enough for durable cleanup. Explicitly owned context budgets are cancelled before replacement and on all exits. Vet verifies the final ownership arrangement. Uncooperative-handler tests demonstrate that timeout is reported without releasing ownership while a handler still runs. Source/fixture YAML errors were corrected without relaxing workflow validation.
- Full local unit/integration/E2E suite passed, with measured core coverage engine 86.2%, state machine 86.2%, parser 93.1% before final permission/validation additions. Full `go test -race -count=1 ./...` passed outside the sandbox with verified Clang/CGO; engine 135.709s and SQLite 102.437s. The final native security/alias tests separately passed `go test -race -count=1 ./internal/dispatchlock`. Final full unit/coverage refresh is running and will be recorded separately rather than inferred.
- `go vet ./...`, `go mod verify`, `go mod tidy -diff`, `go build ./...`, benchmark actionlint and `govulncheck ./...` passed on the reviewed implementation. Dependency scan found no vulnerabilities. Build retains the existing module stat-cache access-denied warning with exit 0. The initial tidy check requested only ordering the promoted direct requirement; the ordering was corrected and the repeat diff is empty. An initial vet warning about loop context ownership was resolved; no analyzer was disabled.
- Full five-package benchmark command from CI (`-run '^$' -bench '^Benchmark' -benchmem -benchtime=200ms -count=3 -cpu=1`) passed: 28 cases / 84 samples, fifteen required families including DispatcherClaim and DispatcherThroughput at 1/4 workers. Native setup/fixture settlement stay outside claim timing; throughput includes durable admission and eight no-op steps. Results measure this Windows filesystem/workstation, not real external-action scaling or an SLA. No speculative optimization was introduced.
- Fifteen further precise dispatcher/CLI rows are `IMPLEMENTED`; broader graceful-shutdown/resource rows are `IN_PROGRESS`. Scheduler, listener, triggers, general retention and worker monitoring remain incomplete. Reconciliation verifies all 561 IDs, requirement text, ownership and prerequisites are unchanged; no row is `COMPLETED`, no distributed claim guarantee is inferred, and no release tag/deployment is created.
- Prepared the next [cron scheduler proposal](designs/cron-scheduler.md) with concrete syntax, timezone/DST semantics and bounded missed-run policy. The required harness instruction remains: "If a required contract is unresolved, complete its design task and seek the relevant decision before implementing it." Stop before this new dependent contract; this is not a repeat request for Phase 03 or dispatcher authorization.
- Next: finish the final coverage/format/Markdown/link checks, make a signed commit, update the existing draft PR to describe its complete final scope and attach actual new CI/benchmark/release evidence. CodeQL still requires repository Advanced Security configuration; earlier CI evidence stays attributed to its tested source commit.

- Final refresh: `go test -count=1 -coverprofile=.cache/dispatcher-complete-coverage.out ./...` exited 0 on the complete dispatcher/security implementation: engine 86.0%, state machine 86.2%, parser 93.1%, CLI 88.1%, SQLite 75.2%. Final review then corrected an error-receipt edge: standalone failures must retain the already admitted execution ID/workflow even before the first attempt. The fault-boundary test now asserts that receipt explicitly; focused regression tests and vet are rerun for this localized change. No historical execution is lost or silently hidden by an empty receipt.
- Final local checks exited 0: targeted receipt/cancellation regression tests, final vet, `gofmt -l .` (no paths), native Bun Markdown formatting/check, diff check and changed-document local-link verification. Ledger reconciliation preserves 561 IDs/static fields with exactly 17 scoped status changes; historical migrations and go.sum remain unchanged. Git reports existing CRLF normalization warnings only.

- Remote CI amendment for signed `8f990e6`: Linux/macOS and Linux race passed, as did all 84 benchmark samples and Markdown. Both Windows jobs exposed a representation-only assertion error: Windows rendered the runner owner's SID as SDDL alias `LA` rather than the numeric SID string. The actual protected DACL was `D:PAI(A;;FA;;;LA)(A;;FA;;;SY)`. Replaced string comparison with binary SID equality plus exact two-ACE, protected-control, allow-type, no-inheritance and full-file-rights assertions. Runtime permissions were not changed or weakened. The revised security test is rerun locally before a follow-up signed commit and fresh CI. CodeQL upload again failed because Advanced Security is not enabled, independently of this test failure.

## 2026-11-14 (Asia/Jakarta) — Persisted cron scheduling

- The user accepted the cron scheduler contract and confirmed continuation. Work started from `1a0745d` on draft PR #5.
- Implemented five-field cron, explicit IANA zones with embedded Go timezone data, bounded missed-run evaluation, and durable cursor and occurrence identity. Scheduler admission uses the dispatcher queue transaction. A lifecycle generation invalidates queued automatic jobs when a workflow is disabled or archived. Revision changes replace current schedules while keeping prior admissions and history readable.
- Added migration 005, schedule inspection and history commands, integration and fault-injection tests, fuzzing, and benchmarks. The [self-review](reviews/phase-03-scheduler.md) maps the tests to requirements. The [scheduling guide](../docs/scheduling.md) records the one-second poll, batch limits, one-minute retry delay, workflow-level enablement, and CLI history bounds.
- Full unit suite with coverage passed. Engine coverage was 83.9%, execution state machine 86.2%, workflow parser 94.7%, and cron parser/calendar 98.3%. Subsequent race verification includes the added lifecycle-race, pending expiry, and migration rollback cases.
- The first full race run hit SQLite's existing busy timeout during concurrent workflow registration. Reused parsed trigger definitions from validation before the write transaction to shorten the lock. The full `go test -race -count=1 ./...` rerun passed with the verified Windows Clang toolchain.
- Cron fuzzing passed with 330,569 executions in four seconds. `go vet ./...`, `go mod verify`, `go build ./...`, and benchmark `actionlint` passed. The local build emitted the existing Go module stat-cache access-denied warning and exited zero.
- The ledger preserves all 561 requirement IDs and fields. It marks 15 cron rows implemented and records partial cron evidence for the broader trigger requirements. CodeQL and Phase 03 acceptance remain open. The [event trigger and webhook proposal](designs/event-triggers.md) specifies the next review decision before implementing external ingress.
- Local benchmark run passed across six packages with 30 cases and 90 samples. The CI workflow asserts 17 benchmark families. Go formatting, Bun Markdown, changed links and diff checks passed.

## 2026-11-14 (Asia/Jakarta) — Automatic event ingress

- The user accepted the startup, internal, and signed loopback webhook contract. Work began at `9344922` in draft PR #5.
- Migration 006 adds enabled route reconciliation, immutable event origins and history, seven-day hashed deduplication, and a persisted startup cursor. Admission shares the existing queue transaction. Internal fan-out uses a savepoint for atomic rollback. Claim rechecks automatic enablement. The server validates opt-in webhook settings before acquiring ownership and joins handlers and workers on shutdown.
- Operational choices include `server --webhook-port`, the `OPSWEAVE_WEBHOOK_SECRET` environment variable, a 250 ms startup scan interval, and local `event history` with a 1000-row maximum. See [operations](../docs/event-triggers.md) and the [self-review](reviews/phase-03-event-triggers.md).
- Focused SQLite rollback, replay, lifecycle, fencing, startup retry, and destination-limit tests passed. The local sandbox blocked the real loopback shutdown test. The same test passed with loopback access. The full local race suite passed with loopback access and two package workers, including CLI startup and joined listener shutdown.
- Route fuzzing passed with 129,883 executions. Webhook authentication benchmark passed with three samples. CI asserts 18 benchmark families. Static analysis, build, module verification, benchmark workflow lint, Markdown and ledger checks passed.
- The next approval covers retention and local readiness. The [proposal](designs/retention-readiness.md) describes deletion boundaries and a local status interface.

## 2026-11-14 (Asia/Jakarta) — macOS deadline persistence

- The push CI run 43545654478 failed on macOS with Go 1.27.3 at
  `TestCancellationAndDeadlinePersistence/deadline`. The pull request CI run
  43545660092 passed on the same commit. The push test log showed the first
  action's outcome had not persisted and hid the underlying repository error.
- Increased the legacy runner's detached cancellation persistence budget from
  five to 30 seconds. The action still stops at its deadline. The engine reports
  a persistence failure if terminal state cannot be committed. Added a regression
  for the independent write budget and improved diagnostics for the deadline test.

## 2026-11-14 (Asia/Jakarta) — Retention and local readiness

- The user replied `ACCEPT` to the retention and local readiness contract. Work started from `9c177d2` on draft PR #5 with a clean worktree.
- Added opt-in age-based pruning, an explicitly read-only preview, and an apply transaction for up to 100 terminal executions. Prune protects unexpired receipts, replay parents and workflow revisions. Cron occurrence identities continue to prevent duplicate admissions after removing the execution and its history. Migration 007 adds these storage boundaries and fenced server lifecycle state.
- Added a local `server status` check for native ownership and the SQLite migration ledger. Startup readiness follows recovery and optional webhook bind. Shutdown publishes `stopping` before joining invocations. Readiness distinguishes standalone work and stale ownership. The [maintenance review](reviews/phase-03-maintenance.md) records security and compatibility evidence.
- The full standard suite passed with native Windows locking and loopback access. The initial full race run exposed probe contention at a 200 ms SQLite busy timeout. Increased it to one second within the two-second probe budget. Six targeted race repetitions then passed. Focused prune and incompatible-schema tests passed after adding exact age boundaries, reopen and checksum coverage.
- `go test -count=1 -p 2 ./...` and `go test -race -count=1 -p 2 ./...` exited zero. The final regular-file checks and checksum regression also passed targeted race verification in the CLI, lock, and SQLite packages.
- `go vet ./...`, `go mod verify`, `go build ./...`, `actionlint .github/workflows/benchmarks.yml`, Bun Markdown, ledger reconciliation, and `git diff --check` passed. The Go build emitted the existing module stat-cache permissions warning and exited zero. No dependency changes.
- Ran `go test -run '^$' -bench '^BenchmarkExecutionPrune$' -benchmem -benchtime=200ms -count=3 -cpu=1 ./internal/storage/sqlite`. Both cases passed in all three samples. Preview was 3.08–3.16 ms per 1000 executions, and apply was 14.8–35.9 ms per 100 executions on local Windows. Benchmark CI checks both cases.
- Phase verification and acceptance remain open. The accepted status command reports overall dispatcher readiness. The worker status requirement needs a separate contract for any per-worker or pool information. CodeQL upload still requires repository Advanced Security configuration.

## 2026-11-14 (Asia/Jakarta) — Worker pool status

- The user replied `setuju` to local worker capacity, busy workers and queue depth. Work started from `53ca869` on draft PR #5.
- Added migration 008 and exposed pool counts in the existing CLI status. Lifecycle and capacity use the same fence. A read-only snapshot counts managed queued work and running attempts of the current generation. Status retains its lifecycle exit codes and exposes aggregate counts only.
- The full standard Go suite, `go vet ./...`, `go mod verify` and `go build ./...` passed. Build emitted the existing local module stat-cache permissions warning and exited zero. Tests cover active and queued work, draining shutdown, retry backoff, old schema upgrade and stale publication.
- `BenchmarkServerWorkerStatus` passed three 200 ms samples at 1.40–1.46 ms per read on local Windows. Benchmark CI checks its output. `actionlint`, Bun Markdown and the source diff checks passed. See the [maintenance guide](../docs/maintenance.md) and [implementation review](reviews/phase-03-maintenance.md).
- Phase 03 implementation is available for verification and acceptance. The release milestone and CodeQL upload configuration remain open. Later phases require user authorization.

## 2026-11-22 (Asia/Jakarta) — Isolated Go HTTP security patch

- While implementing the user-accepted HTTP contract on Phase 04, govulncheck
  reported GO-2026-6859 in Go 1.27.3 net/http, fixed in Go 1.27.4. Verified the
  official vulnerability report and Go release history. Broad reachable symbol
  matches are scan evidence, not a demonstrated exploit in OpsWeave handlers.
- The user instructed keeping the HTTP branch and making this change on a new
  branch, then rebasing HTTP onto origin/main after the patch is merged. This
  authorizes the patch-level toolchain increase and a separate review. Fetch
  confirmed origin/main at `eed2b62`. Created `codex/go-1.27.4-security-patch` from
  origin/main in an isolated ignored worktree. HTTP commit `4bf30d6` remains on
  `codex/phase-04-built-in-actions`, draft PR #8, without cherry-picking it here.
- Updated go.mod, the explicit minimum-version CI matrix, active installation and
  contribution requirements, and the current plan toolchain policy to Go 1.27.4.
  Added security rebuild/upgrade guidance. No application source, module dependency,
  go.sum, schema migration, workflow contract or requirement status is changed.
  Historical assessment and test evidence retain their original version labels.
- Next: verify with Go 1.27.4, open a separate patch PR targeting main, and leave
  merging to the user. Rebase the HTTP branch only after the patch is merged.

- Local Go 1.27.4 verification: full `go test -count=1 ./...`, `go vet ./...`,
  `go mod verify`, `go mod tidy -diff`, and `go build ./...` exited 0.
  `govulncheck ./...` exited 0 with "No vulnerabilities found." Formatting,
  Markdown and CI actionlint passed. Full race verification is recorded below
  after completion, rather than inferred from the normal suite.
- Self-review verified that only the toolchain directive and explicit CI minimum
  changed executable build inputs; all application dependencies/go.sum, source,
  migrations, requirement ledger IDs/statuses and historical evidence are unchanged.
  Existing tests supply regression coverage; no implementation-mirroring test was
  added for a version-string edit. The upstream patch is verified by the clean
  vulnerability scan and repository suites.
- Independent HTTP PR #8 checks on `4bf30d6` completed successfully: CI run
  44937235807 (all six OS/toolchain jobs and race), benchmark 44937235815,
  Markdown 44937235827, CodeQL 44937235682. These are evidence for that HTTP
  source commit, not tests of the separate patch or the future rebased combination.

- Full `go test -race -count=1 ./...` exited 0 on Go 1.27.4 with CGO and the
  existing Clang toolchain. Link verification and exact seven-file scope check
  passed. No source/test change followed these product checks. Next step is the
  separately authorized security PR and its remote quality gates; no merge or
  HTTP rebase occurs before the user merges the patch.

## 2026-11-22 (Asia/Jakarta) — Vulnerability CI on the patch branch

- The user explicitly requested govulncheck CI on this same patch branch. Added
  a read-only native Linux/Windows/macOS matrix using the declared minimum Go
  toolchain and pinned govulncheck v1.10.0. Official scanner documentation confirms
  text-mode findings fail, while JSON/SARIF output does not enforce that exit
  behavior. Use text output with no continue-on-error or exit-code suppression.
- Added Go/module/workflow path triggers, manual dispatch and a weekly default
  branch schedule, concurrency cancellation and a 15-minute job timeout. Existing
  checkout/setup-go SHA pins are reused with checkout credentials disabled.
  Application dependencies and go.sum remain unchanged. README and contribution
  instructions document commands, build-configuration limits and branch protection.
- Prior patch commit `2ff7302`: normal CI push, benchmark, Markdown, CodeQL and
  six-target release preview/assemble passed. The first PR CI Windows/stable
  attempt failed TestReliableCoordinatorCancelsMaximumGraphWithinCleanupBudget
  after its five-second persistence cleanup budget expired on a slow runner. The
  same Go 1.27.4 source passed push Windows/stable and PR Windows/minimum. Ten
  targeted local repetitions passed. Reran only the failed job, with no source or
  assertion change. This timing failure is disclosed rather than hidden.
- PR-triggered jobs initially appeared late. Two temporary manual dispatches of
  benchmark/CodeQL were cancelled once their equivalent PR jobs appeared, avoiding
  duplicate verification work. Their cancellations are not test failures or passes.

- The failed Windows/stable PR job passed its targeted rerun on unchanged
  `2ff7302`; no test or source fix is inferred from that rerun. Full initial CI
  evidence and the intermittent cleanup failure remain disclosed in PR #9.
- Verified exact CI command `go run golang.org/x/vuln/cmd/govulncheck@v1.10.0
./...` with Go 1.27.4: exit 0, "No vulnerabilities found." Installed the same
  pinned scanner into an ignored task-local tool directory for a positive control.
  An isolated, unexecuted fixture importing net/http under Go 1.27.3 returned
  native exit 3 and GO-2026-6859. This proves findings fail rather than silently
  passing. No vulnerable fixture or downloaded tool is tracked.
- Workflow self-review checked read-only permissions, disabled persisted checkout
  credentials, exact tool/action pins, bounded timeout, matrix failure reporting,
  no shell interpolation of untrusted values and no finding/error suppression.
  actionlint, pinned Bun Markdown and whitespace checks exited 0. This work unit
  changes only CI/docs, so prior full unit/race evidence remains attributed to
  `2ff7302`; no application test rerun is claimed for this workflow-only follow-up.

## 2026-11-22 (Asia/Jakarta) — Latest vulnerability scanner

- The user explicitly requested govulncheck latest. Changed the workflow and local
  contribution command to `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`.
  This supersedes the scanner version pin in `cf2f891`; historical pinned-scanner
  results above retain their attribution. Action SHA pins, minimum Go toolchain,
  native OS matrix, read-only permissions and failing text-mode gate are unchanged.

- Verified `@latest -version` resolved govulncheck v1.10.0 using Go 1.27.4 and
  the official Go vulnerability database. The exact latest scan exited 0 with
  "No vulnerabilities found." Existing positive-control evidence applies to the
  same resolved scanner version. actionlint, Markdown and whitespace checks pass.

## 2026-11-22 (Asia/Jakarta) — Avoid duplicate feature-branch CI

- Before merging, the user requested avoiding simultaneous push and pull-request
  CI runs because of excessive job volume. Restrict push events to main in CI,
  Markdown and vulnerability workflows. Benchmark and CodeQL already restrict
  pushes to main; release pushes already restrict to version tags. Keep all PR
  validation, path filters, manual dispatch and scheduled security scans.
- Updated README and contribution policy. Every feature-branch update now uses
  PR validation only, retaining separate post-merge main verification and tag
  publication. No job, security scan, test assertion or matrix coverage is removed.
- Prior latest-scanner head `dd6a966` passed PR CI 44939032093 (all six OS/Go
  jobs and race), govulncheck 44939032160 (all three OSes), benchmark 44939032224,
  Markdown 44939032156, CodeQL 44939032132 and six-target release/assemble 44939032148. Publish was skipped. This evidence remains attributed to that head;
  current changes only affect CI event routing and documentation.

- All-workflow actionlint, pinned Bun Markdown and diff whitespace checks exited 0. Audited all six event blocks: five workflows allow pushes only to main;
  release allows only version-tag pushes; every PR trigger remains. No Go source
  changed and no repeated local product tests are claimed for event-only edits.

## 2026-11-22 (Asia/Jakarta) — Phase 04 authorization and HTTP design

- The user confirmed Phase 03 was merged and requested rebase to origin/main and
  continuation. This accepts Phase 03 for progression and authorizes Phase 04.
  Independent CodeQL configuration and prior release-preview billing limitations
  remain historical open checks, not inferred passes.
- The worktree was clean. Fetch updated origin/main to `eed2b62`. Rebase completed
  without conflicts. Created `codex/phase-04-built-in-actions` at that baseline.
  Inspected GOALS, AGENTS, plans, ledger ownership, action runtime/registry,
  configuration, CLI composition and existing action/CLI tests. No release tags
  exist. No product implementation or requirement completion is claimed.
- Prepared the [HTTP action contract](designs/http-actions.md) as the first Phase
  04 work unit. It specifies descriptor/output behavior, default-disabled network
  permissions, SSRF controls, explicit private-origin access, bounds and acceptance
  tests. Existing configuration and engine/storage behavior remain unchanged.
- The harness requires: "If a required contract is unresolved, complete its design
  task and seek the relevant decision before implementing it." Phase 04 task 1
  likewise requires reviewing action contracts. Runtime implementation waits for
  this concrete security/behavior decision, rather than repeating phase approval.
- Next: accept or revise the HTTP contract, then implement both actions, tests,
  benchmark CI coverage, documentation and code/security review. Remaining Phase
  04 action contracts and Phase 05 remain unapproved.
- Planning verification exited zero: pinned Bun 1.4.4 Markdown formatting/check,
  `git diff --check`, changed-document local link validation and exact comparison
  of all 561 ledger rows. The first formatter attempt could not locate Bun on
  PATH; the pinned existing `.cache/bun-1.4.4` executable supplied the successful
  run. Git emitted existing CRLF normalization warnings. No Go source changed,
  so Go tests were not rerun for this design-only work unit. Self-review checked
  scope, permission defaults, credential handling and preservation of all ledger
  statuses. No commit, push, PR, tag or release was created in this work unit.

## 2026-11-22 (Asia/Jakarta) — HTTP action implementation and security gate

- The user replied `ACCEPT` to the HTTP contract. Implemented exact-version
  `http.request` and `http.health_check`, strict opt-in configuration/environment
  overrides, production preflight/run/replay/server composition, SSRF policy and
  bounded HTTP I/O. No new dependencies, migrations or engine/state changes.
- Added isolated policy/header/transport/security tests, production CLI history
  and server startup/shutdown tests, bounded URL/policy fuzzing, and
  `BenchmarkHTTPPolicy` asserted by benchmark CI. Updated English operational
  documentation and removed obsolete empty-production-catalog statements.
  [Self-review](reviews/phase-04-http.md) maps test names to the accepted contract.
- `go test -count=1 '-coverprofile=.cache/http-phase04-full.out' ./...` exited 0.
  Core coverage: HTTP 95.3%, engine 82.9%, execution 86.2%, workflow 94.3%.
  `go test -race -count=1 ./...` exited 0 with CGO/Clang and loopback access.
  The additional production server test subsequently passed focused normal and
  race runs (`-run '^TestProductionHTTP' ./internal/cli`).
- `go vet ./...`, `go mod verify`, `go build ./...`, and benchmark actionlint
  exited 0. Build emitted the existing module stat-cache access-denied warning.
  `gofmt -l .` initially reported ignored generated files in an abandoned build
  directory dated 2026-11-14. Verified its resolved path stayed within
  `.cache/go-tmp`, removed only that stale generated directory, and reran
  `gofmt -l .` with no paths and exit 0. Tracked Go sources were already formatted.
- Initial sandboxed loopback tests were blocked by Windows socket permissions;
  the same suites passed with isolated loopback access. An initial unquoted
  PowerShell coverage flag was split into an unintended package argument; the
  corrected quoted full-suite command above passed. No assertion was weakened.
- Fuzz command `go test -run '^$' -fuzz '^FuzzHTTPURLPolicy$' -fuzztime=4s
-parallel=2 ./internal/httpaction` exited 0 with 55,279 executions.
  Benchmark command `go test -run '^$' -bench '^BenchmarkHTTPPolicy$' -benchmem
-benchtime=200ms -count=3 -cpu=1 ./internal/httpaction` exited 0: 1205–1305 ns/op,
  267 B/op, 6 allocs/op on this Windows host. No optimization is claimed.
- `govulncheck ./...` exited 1 with GO-2026-6859 in Go 1.27.3 net/http, fixed in
  1.27.4. Verified the official Go vulnerability report/release history. No other
  module vulnerability was reported. This is a real open gate; the broad symbol
  matches are not proof of an exploit in OpsWeave. PLANS requires approval for
  the toolchain change. Prepared the [concrete patch proposal](designs/go-http-security-patch.md).
- Four HTTP/SSRF ledger rows are `IMPLEMENTED`, not `COMPLETED`. Phase 04 remains
  active. The next [shell contract](designs/shell-action.md) is prepared for later
  review, with no shell runtime implementation. Resolve the toolchain decision
  and security scan before claiming the HTTP feature complete. Phase 05 is not
  authorized. Remote CI evidence and final publication outcomes are recorded in
  subsequent amendments rather than inferred from local checks.
- Final planning/documentation checks passed: pinned Bun Markdown formatting/check,
  changed-document local links, source diff whitespace and all 561 ledger IDs,
  ownership/prerequisite/source fields. Only the four HTTP/SSRF rows changed status
  and evidence. Corrected a Windows text-encoding/newline issue in README during
  validation and restored unaffected text from the baseline before reapplying the
  scoped edit. No product assertions or security controls were relaxed.

## 2026-11-22 (Asia/Jakarta) — Rebase HTTP after the security merge

- The user confirmed PR #9 was merged. Fetch updated origin/main from `eed2b62`
  to `0033480`. Rebased `codex/phase-04-built-in-actions`; the only conflict was
  the append-only build log. Preserved the complete main log and appended all
  original HTTP-only entries. No application conflict or source edit occurred.
  The replayed HTTP implementation commit is `d3ede94`.
- The branch now inherits Go 1.27.4, govulncheck@latest, and main-only push checks
  while feature branches use PR checks. The independent security patch remains
  in main; no security-branch changes were copied back into the HTTP feature diff.
- Updated current harness state to combined HTTP verification. Earlier open
  security-decision statements are superseded by the explicit merge evidence.
  Shell remains a proposed contract, not an implemented or accepted action.
- Next: complete required Go/race/security checks on the combined source, update
  PR #8 with a lease-protected rebase push, observe its single set of PR checks,
  and close only requirements whose acceptance evidence actually passes.

- Combined-source verification with Go 1.27.4 exited 0: full ordinary suite with
  `.cache/http-rebased-coverage.out`, full race suite with CGO/Clang, vet, module
  verification, build and the exact govulncheck@latest command. Scanner reported
  "No vulnerabilities found." Coverage: HTTP 95.3%, engine 82.6%, execution
  86.2%, workflow 94.3%. No application failure or build warning was emitted.
- HTTP policy benchmark passed three 200 ms samples: 1111-1231 ns/op, 267 B/op,
  6 allocs/op on local Windows. Four-second URL/policy fuzzing passed 71,138
  executions. No optimization was introduced. Go formatting, all-workflow
  actionlint, pinned Bun Markdown and diff whitespace checks passed.
- Rebase review confirmed application source, tests, go.sum and command code are
  byte-identical to the original accepted HTTP commit `4bf30d6`; only main's
  toolchain/CI/docs and resolved harness history differ. The rewritten commit
  retains a GPG signature. HTTP rows remain IMPLEMENTED pending fresh PR evidence.

## 2026-11-22 (Asia/Jakarta) — HTTP acceptance after security rebase

- Combined HTTP source `dfc0ae5` passed all required PR checks without retries:
  CI 44940800316 (six native OS/Go jobs plus race/format/module checks), latest
  vulnerability scan 44940800291 (three native OSes), benchmark 44940800277,
  Markdown 44940800293, CodeQL 44940800290, and release preview 44940800348
  (six archives plus assemble). No tag or release was published. Observed exactly
  six PR workflows and no duplicate push workflow.
- The [HTTP self-review](reviews/phase-04-http.md) maps acceptance tests and remote
  run links. Completed only G-5.5-013, G-5.5-014, G-5.15-009 and G-5.15-020.
  Preserve every other ledger status, static requirement field and prerequisite.
  Phase 04 remains in progress; shell and later action integrations are incomplete.
- This acceptance amendment changes documentation and scoped status only. It does
  not alter the application source/tests that passed the checks above. Earlier
  intermittent Phase 03 failures are not claimed fixed; this combined run passed
  without source changes or weakened assertions.
- Next required human decision: accept or revise the concrete shell.exec contract.
  Harness authorization rules require reviewing unresolved contracts before their
  implementation. The proposal specifies operator profiles, direct argv, minimal
  environment, bounded output/deadlines and native process ownership, including
  the Unix trusted-command/no-daemonization boundary. No shell runtime was added,
  and no Phase 05 work or merge was performed.

## 2026-11-22 (Asia/Jakarta) — Configured shell action implementation

- The user replied `ACCEPT` to the shell execution proposal. Work started at
  `76387d3` on `codex/phase-04-built-in-actions`, with a clean worktree. Phase 04
  remains authorized; no Docker, filesystem, notification or Phase 05 runtime
  scope is inferred.
- Added shell.exec version 1 with strict opt-in profiles, immutable configuration
  snapshots, absolute local executable/directory checks, combined argv bounds,
  minimal native environment and bounded UTF-8 stdout/stderr capture. The existing
  CLI composes enabled HTTP and shell registrations with independent exact grants.
  HTTP behavior stays unchanged. No dependency, go.sum, schema or DSL change.
- Native Windows execution starts suspended, assigns a non-breakaway kill-on-close
  job, validates the initial thread's process ID and resumes only after assignment.
  Cleanup terminates/queries the job and checks handle closes. Linux/macOS use
  process groups under the accepted trusted-command/no-daemonization boundary.
  Capture goroutines join before output inspection and ownership release. The
  cancellation and cleanup paths share one two-second budget.
- Tests cover argument preservation, environment isolation, permission/version
  gates, fixed cwd, profile revocation/snapshots, invalid input, output/JSON limits,
  start failure, native pre-execution ownership, failed assignment, cancellation,
  deadlines, descendant cleanup and CLI-to-SQLite/queued-server shutdown. Fixtures
  are isolated compiled programs and loopback handshakes, with bounded test-owner
  cleanup. Windows job termination may produce WSAECONNRESET rather than EOF;
  corrected the initial closure assertion to accept that exact native result
  after a readiness handshake, while retaining timeout/survival failures.
- Initial integration editing had a pointer-level mismatch in decodeActions; fixed
  it before verification. A sandboxed HTTP regression run lacked loopback access;
  the same suite passed with isolated loopback permission. No assertion or runtime
  security control was weakened. Linux and Darwin shell test binaries cross-compiled
  successfully; native behavior awaits the existing CI matrix.
- Full `go test -count=1 '-coverprofile=.cache/shell-full-coverage.out' ./...`
  and full `go test -race -count=1 ./...` exited 0 on Go 1.27.4. Race used CGO and
  existing Clang. Coverage: shell 83.7%, HTTP 94.9%, engine 83.0%, execution 86.2%,
  workflow 94.3%, CLI 82.4%, config 96.6%. Added permission-composition and valid-Unicode argv cases
  subsequently passed targeted ordinary and race tests. No production source
  changed after the full suites.
- Vet, build, `go mod verify`, `go mod tidy -diff`, govulncheck@latest, all-workflow
  actionlint, Go/Markdown formatting and whitespace checks exited 0. Scanner
  reported "No vulnerabilities found." No build warning was emitted.
- `go test -run '^$' -bench '^BenchmarkShellExecution$' -benchmem -benchtime=200ms
-count=3 -cpu=1 ./internal/shellaction` passed. Three Windows samples measured
  397-439 ms/op, 75,808-83,080 B/op and 148-206 allocs/op with one iteration per
  sample. Fixture compilation is outside timing; these host/process startup
  measurements are a baseline, not a production tool latency claim or optimization.
  Benchmark CI asserts this real isolated-process case.
- `go test -run '^$' -fuzz '^FuzzShellArguments$' -fuzztime=4s -parallel=2
./internal/shellaction` exited 0 with 83,022 executions. No fixture binaries,
  cache output or test data containing real credentials are tracked.
- Updated the [shell reference](../docs/shell-action.md), configuration/benchmark
  documentation and [self-review](reviews/phase-04-shell.md). Three shell/security
  ledger rows are IMPLEMENTED pending native CI. Four completed HTTP/SSRF rows
  remain unchanged. Next: observe native OS/Go/race/security/benchmark/release
  checks on the updated draft PR, then close only the fully verified shell rows.

- Prepared the next [Docker contract](designs/docker-actions.md) with explicit
  local IPC, exact full-ID/action allowlists, bounded safe inspect output and API
  1.48 compatibility. Checked official Docker transport/version documentation
  and the pinned API specification. No Docker runtime or daemon access was added.
  The design remains PROPOSED and requires the next human contract decision.

- Final review amendment: after the earlier passing suites, replaced per-invocation
  lazy DLL lookup with balanced System32-only LoadLibraryEx/FreeLibrary ownership
  around GetProcAddress/SyscallN. This avoids accumulating library references.
  The native source change justified repeating full verification; it is not
  covered merely by the earlier "source unchanged" observation.
- Final ordinary suite (`.cache/shell-final-coverage.out`), full race suite, vet,
  build and govulncheck@latest all exited 0 on this final native implementation.
  Final coverage: shell 82.7%, HTTP 94.9%, engine 82.7%, execution 86.2%, workflow
  94.3%. No scanner finding or build warning. No production source changed after
  these final checks. Argument/environment tests now assert exact OS-specific
  variables and values, including valid Unicode argv preservation.
- Refreshed final Windows benchmark passed three single-iteration samples at
  453-494 ms/op, 75,664-83,256 B/op, 144-209 allocs/op. The same measurement limits
  apply; no performance improvement or regression conclusion is inferred from
  these short host-specific startup samples.
- Final documentation/link, all-workflow actionlint, Markdown, Go formatting and
  whitespace checks passed. Reconciliation preserves all 561 IDs/static fields,
  changes exactly the three scoped shell rows from PLANNED to IMPLEMENTED, and
  preserves all four previously completed HTTP rows. No dependency, migration,
  generated binary or secret file is included.

## 2026-11-22 (Asia/Jakarta) — bounded scan shutdown follow-up

- Baseline `c1e2b28`, authorized Phase 04 shell acceptance on PR #8. Native
  shell tests passed on Linux, macOS and Windows; latest vulnerability, Markdown,
  benchmark, CodeQL and six-target release/assemble checks passed. CI run
  [44974954486](https://github.com/PHPDevsr/opsweave/actions/runs/44974954486)
  failed only Ubuntu/stable `TestDispatcherCLIRequiresUnmanagedAdoption`, reporting
  sanitized persistence failure during immediate shutdown. No requirement was
  completed using that failed run. No push-trigger duplicate workflow was created.
- Investigation found startup/schedule transactions still inherited immediate
  parent cancellation, unlike other independently bounded dispatcher metadata.
  database/sql can automatically roll back a canceled transaction before Commit,
  returning ErrTxDone instead of the parent's cancellation error. The original
  intermittent CI error's underlying cause was sanitized, so that exact instance
  is not claimed as locally reproduced: 200 original adoption repetitions passed.
  Temporary local cause diagnostics were removed without a product change.
- Startup/schedule scans now use the existing five-second independent metadata
  budget. The dispatcher joins the scan, then enters shutdown without claiming
  waiting work. Actual scan errors remain failures. A committed startup admission
  can remain queued for the next server; docs/dispatcher.md now explains this.
  No action security policy, schema, dependency or public contract changed.
- `TestDispatcherShutdownDuringDurableScan` deterministically cancels the parent
  inside startup/schedule scans and verifies the bounded scan context survives,
  is released on return, storage failures remain visible, no waiting action runs,
  and committed startup admission remains durably queued. Replacing only the two
  fixed scan contexts with their original implementation made all four cases fail
  as expected; restored implementation passed. Ten initial regression repetitions
  and 30 adoption repetitions passed. The expanded durable-admission test passed.
- Initial full ordinary/race runs caught the old scheduler fixture intentionally
  blocking until parent cancellation, which now reaches the independent five-second
  deadline. Updated it to explicitly release its scan after cancellation and assert
  the dispatcher cannot return before joining it. The running-action cancellation
  and durable trigger assertions remain. Three targeted repetitions passed.
- Final full ordinary suite with `.cache/shell-shutdown-coverage.out` exited 0.
  Core coverage: engine 85.6%, execution 86.2%, workflow 94.3%; shell 82.7%, HTTP
  94.9%. Final vet, build, module verification, Go formatting and whitespace checks
  passed. Full race and fresh remote checks are pending at this log entry.
- Self-review: the independent budget is finite, uses existing context value
  propagation, adds no goroutine or new storage abstraction, and never interprets
  an infrastructure error as successful shutdown. Existing worker joining and
  cancellation remain in place. No release/deployment/merge was performed.
- Final full `go test -race -count=1 ./...` exited 0 with CGO/Clang on Windows,
  including engine, scheduler storage and shell process ownership. Remote CI is
  the remaining native combined-source acceptance gate.
- Final installed govulncheck scan exited 0: no vulnerabilities found. Markdown
  checks passed across all sources. Diff check passed; Git only warned that
  PowerShell-written documentation line endings would normalize to repository LF.

## 2026-11-22 (Asia/Jakarta) — scheduler fixture minute boundary

- Follow-up `7788a31` CI [44976137400](https://github.com/PHPDevsr/opsweave/actions/runs/44976137400)
  passed all Linux/macOS matrices, Windows minimum Go and Linux race. Shell and
  engine tests passed in every job. Windows/stable failed only the modified
  scheduler shutdown integration's one-execution count: its wall-clock scan at
  02:28:58 plus the simulated two seconds reached 02:29:00 and legitimately
  admitted a second cron occurrence. This is fixture time dependence, not a
  failed cancellation or surviving action; the original execution was cancelled.
- Use scheduledFixture's existing fixed :10 instant, after workflow creation,
  instead of wall-clock seconds. Both simulated scans stay in the same minute.
  Preserve exact execution count, one handler call, cancellation, joined scan and
  pinned scheduled-at assertions. No production source changed after `7788a31`.
- Vulnerability, benchmark, Markdown and CodeQL checks on `7788a31` passed.
  Six-target release/assemble remains tracked separately. Shell rows remain
  IMPLEMENTED while a fully green combined-source CI result is unavailable.
- Fixture approach amendment: using scheduledFixture's future :10 instant also
  moved queue availability into the future, so the first exploratory verification
  stalled until its deadline; it was stopped, not counted as passing. The final
  correction retains real admission time for the first real repository scan.
  After the shutdown handshake, the second fake scan completes with no entries,
  matching its synchronization-only purpose. It no longer performs a second real
  cron admission at a simulated minute boundary. The first real scheduled run,
  exact count, joined scan, action cancellation and history assertions remain.
  An exploratory verbose run timed out after 35 seconds; no such future-time
  change is retained. This supersedes the proposed fixed-:10 approach above.
- Release run 44976137445 passed all six packages and assembly, with publication
  correctly skipped for the PR. No tag or release was created.
- Final correction passed 30 ordinary and 10 race repetitions of
  TestSchedulerDispatcherCompletesAndInterrupts (exit 0). Final vet, Go/Markdown
  formatting and diff checks passed. The full source ordinary/race suites remain
  attributed to the prior final dispatcher source, which is unchanged; fresh CI
  will test this fixture correction across all native matrices.

## 2026-11-22 (Asia/Jakarta) — shell unit accepted by verification

- Baseline combined source `3bd23de`, branch codex/phase-04-built-in-actions,
  draft PR #8 targeting main. The accepted Phase 04 shell contract is implemented,
  tested, reviewed and documented. Fresh all-check acceptance is now available:
  [CI 44976628487](https://github.com/PHPDevsr/opsweave/actions/runs/44976628487),
  [vulnerability 44976628507](https://github.com/PHPDevsr/opsweave/actions/runs/44976628507),
  [benchmarks 44976628467](https://github.com/PHPDevsr/opsweave/actions/runs/44976628467),
  [Markdown 44976628468](https://github.com/PHPDevsr/opsweave/actions/runs/44976628468),
  [CodeQL 44976628471](https://github.com/PHPDevsr/opsweave/actions/runs/44976628471),
  [release 44976628571](https://github.com/PHPDevsr/opsweave/actions/runs/44976628571).
  All six workflows completed successfully as pull_request events. No duplicate
  push runs. CI includes all six OS/Go jobs and Linux race; scanner covers three
  OS families; release includes six target archives and assembly. Publish skipped.
- The self-review maps exact native, security, CLI/history, cancellation and
  process ownership tests. Previous failed runs and their corrections remain
  explicit above. Latest remote evidence covers the final test fixture correction;
  full local ordinary/race/vet/build/module/scanner results remain attributed to
  unchanged production source, with 30 ordinary/10 race repetitions for that fix.
- Mark only G-5.5-016, G-5.15-007 and G-5.15-017 COMPLETED. Reconciliation preserved
  all 561 IDs and static requirement fields, every unrelated row and all four
  previously completed HTTP rows. No dependency, schema, release-doc version,
  binary or secret change. No migration is required.
- Update master/harness/phase current state to
  PHASE_04_DOCKER_CONTRACT_REVIEW_REQUIRED and attach the final acceptance table.
  Docker proposal is already concrete and reviewable, but remains PROPOSED.
  Stop before implementation for that contract decision, as harness/README.md
  and the phase plan require. No Phase 05 work, merge, tag, deployment or external
  message is authorized by this documentation update.
- Final documentation-only acceptance update passed Bun Markdown formatting,
  added-local-link validation, diff whitespace checks and 561-row reconciliation.
  No product source changed after the fully passing combined-source CI above.

## 2026-11-22 (Asia/Jakarta) — accepted Docker adapters implemented locally

- Baseline `923831e` on codex/phase-04-built-in-actions, draft PR #8 targeting
  main. The user replied ACCEPT to the concrete local-IPC/exact-ID/action-allowlist
  Docker contract. Record the contract ACCEPTED and authorize only its four
  adapters within Phase 04. Seven completed HTTP/shell rows remain unchanged.
- Implement internal/dockeraction for docker.start/stop/restart/inspect version 1.
  Strict native endpoint/full-ID/action policy is snapshotted; dangerous access
  defaults to disabled. Strict YAML and OPSWEAVE_DOCKER_ENABLED override compose
  independently with existing HTTP/shell actions at the CLI boundary. No daemon
  discovery, Docker CLI/SDK, ambient proxy/config, dependency or migration added.
- One invocation owns one local connection and one fixed API 1.48 request. Standard
  bounded HTTP parsing has no transport goroutines, redirects, retries, pool or
  decompression. Total budget 30s, connection/header budgets 5s, headers 16 KiB,
  body 1 MiB, existing output 64 KiB. Cancellation joins its once-close callback.
  Oversize rejection closes IPC before body cleanup to prevent unbounded draining.
  Inspect captures only validated typed fields and omits raw daemon/env/mount data.
- Windows client uses non-inheritable overlapped pipe handles with anonymous SQOS.
  Native tests prove busy-pipe deadline and inability to impersonate the service
  token. Fake daemons use private current-user/SYSTEM-only local pipes; Unix
  fixtures use short private socket paths. Production never imports test utilities.
- Tests cover exact API paths/methods/status/grace, immutable grants, permission
  subsets, malformed inputs/responses, version failure, redirect/encoding rejection,
  header/body bounds, cancellation/deadline/peer closure, concurrent invocations,
  all four production CLI actions to SQLite history, redaction/revocation and queued
  server shutdown. See reviews/phase-04-docker.md for exact test names.
- Initial tests caught test expectations: run JSON is a summary, so the four-step
  assertion belongs on execution get history; Windows file-close cleanup can
  return os.ErrClosed as well as net.ErrClosed. Assertions were moved to the
  actual detailed output or retained with native equivalent close semantics.
- Initial full ordinary suite passed, but full race exposed a new fake listener
  owner failing to join. Focused repeated race and diagnostic stacks located a
  pending GetOverlappedResult; escape analysis showed the asynchronous OVERLAPPED
  address on a movable Go stack. Pin it with runtime.Pinner until cancellation
  and kernel completion join, then close the event. Join every pending-I/O exit,
  including failed waits. No timeout was expanded or join assertion discarded.
  Some diagnostic repetitions failed; one superseded diagnostic run was stopped.
- After that fix, 30 TestProductionQueuedDockerShutdown race repetitions and
  three complete Docker-package race repetitions passed. Escape analysis now
  reports OVERLAPPED moved to heap. Final full ordinary coverage suite and full
  go test -race -count=1 ./... both exited 0. Core final coverage: engine 85.5%,
  execution 86.2%, workflow 94.3%; Docker 87.6%, HTTP 94.9%, shell 82.7%, config
  97.1%, CLI 82.4%. All source is tested; no native assertion was weakened.
- Final go vet ./..., go build ./..., go mod verify and go mod tidy -diff exited 0. go.mod/go.sum unchanged. Go formatting, all six workflow actionlint and diff
  checks passed. Installed govulncheck@latest and scanned ./...: exit 0, no
  vulnerabilities found. No build warning was emitted.
- Cross-compiled Docker/CLI test binaries for Linux and Darwin successfully;
  native execution remains a CI gate. Four-second/two-worker fuzz runs passed:
  FuzzDockerInspect 113,091 executions; FuzzDockerPolicy 135,163 executions. No
  production Docker daemon or credential is accessed by these tests.
- BenchmarkDockerInspectIPC passed three 200ms/cpu=1 Windows samples:
  135052, 122476, 155917 ns/op; 19455-19461 B/op; 108 allocs/op. This measures a
  real private fake-daemon connection/request/filter/cleanup, excluding setup.
  It is a short host-specific baseline, with no optimization, SLA or actual Docker
  latency claim. Benchmark CI now requires DockerInspectIPC and uploads existing
  environment/results artifacts; native correctness CI is unchanged.
- Added English Docker reference/configuration/workflow/CLI/security/upgrade/
  troubleshooting docs, README/foundation links and benchmark guidance. Added
  code/security self-review, accepted contract and phase/master status updates.
  Reconciliation preserved all 561 IDs/static fields and changed exactly six
  owned Docker/security rows from PLANNED to IMPLEMENTED; native CI is pending.
- Prepared a concrete filesystem proposal only, using reviewed Go Root API
  boundaries and explicit destructive/copy/cancellation limits. It remains
  PROPOSED with no filesystem runtime. Notification contracts and Phase 05 remain
  unapproved. Next: push updated draft PR and observe every fresh native/scanner/
  benchmark/security/release check before closing any Docker requirement.
