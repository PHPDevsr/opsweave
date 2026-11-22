# Phase 04 filesystem implementation review

The user explicitly accepted `file.exists`, `file.cleanup` and `file.copy` on
2026-11-22 (Asia/Jakarta), including local-root grants, single-file deletion,
exclusive hard-link publication and cooperative syscall cancellation limits.
Scope is action version `1` within Phase 04; notification adapters are separate.

## Code and security self-review

- `internal/fileaction` remains behind existing action descriptors/runtime and
  CLI composition. Domain, engine and storage interfaces are unchanged. Config
  snapshots root paths/action grants; runtime maps are read-only. No dependency,
  schema migration, management API or public workflow DSL change is added.
- Default-off, strict native absolute directory policy and exact per-root grants
  prevent implicit capability. Copy requires grants on both roots. Portable paths
  reject traversal, device/drive/UNC spellings, control characters, reserved names,
  wildcards and internal temporary prefixes before use. No glob or recursion exists.
- Root/parent handles provide containment rather than string-prefix checking.
  Intermediate relative links are resolved inside `os.Root`; final files use
  native no-follow opens and verified type/identity. Unix uses `openat` with
  NONBLOCK/NOFOLLOW/CLOEXEC; Windows uses synchronous relative `NtCreateFile`,
  DONT_REPARSE, OPEN_REPARSE_POINT and NON_DIRECTORY_FILE, preserving native case
  lookup. No native asynchronous Go pointer is retained.
- Cleanup validates a regular file and cannot remove a directory: Unix uses
  unlinkat without AT_REMOVEDIR; Windows reopens the validated file by handle
  with DELETE access and applies POSIX file disposition, without a path-based
  fallback or readonly-attribute bypass. Each System32-only library load/free and
  native handle close is balanced; cleanup failures remain action failures.
- Copy uses a bounded 32 KiB transfer, byte limit, pre/post source metadata checks,
  owner-only Unix creation mode and a random O_EXCL reserved temporary file in an
  anchored destination directory. Flush/close precede one exclusive hard-link
  publication. Existing targets are never replaced. Temporary cleanup is checked;
  a failed cleanup does not trigger destructive rollback of a published target.
- Cancellation is checked before work/chunks/publication/deletion. Roots/files
  are synchronously owned/closed; no I/O goroutine is detached. Kernel filesystem
  stalls remain the explicitly accepted limitation. No external snapshot,
  crash-durable directory, malicious-writer or exactly-once guarantee is claimed.
- Outputs contain only typed booleans/kind/byte count. Raw paths, file contents
  and OS errors are omitted from errors/logs/history. Existing structured engine
  lifecycle identities remain. Operator trust excludes external hard links,
  mount/device/pseudo-filesystem relationships and untrusted local writers.
- A private transfer-function seam tests actual-root I/O failures and cleanup
  without global hooks or another filesystem interface. The queued CLI test uses
  an injected test-only gate after a real committed copy, proving shutdown cannot
  roll it back or invoke pending cleanup. That gate is absent from production config.

## Observable test mapping

| Area                                                                            | Tests                                                                                                                     |
| ------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| Default-off, portable paths, strict roots/grants/size/environment               | `TestFilesystemPolicyAndPortablePaths`, `TestFilesystemConfiguration`                                                     |
| Normal metadata, binary copy, deletion and absent-file idempotency              | `TestFilesystemExistsCopyCleanup`                                                                                         |
| Root/action/version permissions and immutable snapshots                         | `TestFilesystemGrantsSnapshotAndRevocation`, `TestConfiguredFilesystemPermissionsAreIndependent`                          |
| Existing destinations, missing/directory/empty sources and size boundary        | `TestFilesystemCopyBoundsAndExistingTargets`                                                                              |
| Read/write/short-write/no-progress/growth/cancel failures                       | `TestBoundedCopyFailureGrowthAndCancellation`                                                                             |
| Real-root failure cleanup, flush failure, source growth and publish conflict    | `TestFilesystemTransferFailureCleansTemporaryFiles`                                                                       |
| No mutation after cancellation/deadline                                         | `TestFilesystemCanceledActionsDoNotMutate`                                                                                |
| Exactly one exclusive publication and no reserved leftovers                     | `TestFilesystemExclusiveConcurrentPublication`                                                                            |
| Intermediate/final link refusal and source/destination swap confinement         | `TestFilesystemSymlinkBoundaries`, `TestFilesystemSymlinkSwapCannotEscapeRoot`, `TestNativeOpenDoesNotFollowFinalSymlink` |
| Native directory-deletion prevention and Unix special-file nonblocking open     | `TestNativeUnlinkCannotRemoveDirectory`, `TestUnixSpecialFileAndNonblockingOpen`                                          |
| Windows readonly cleanup failure preserves published target and reports failure | `TestWindowsCleanupFailureDoesNotReportCopySuccess`                                                                       |
| Windows root case behavior                                                      | `TestWindowsNativeOpenPreservesRootCaseSemantics`                                                                         |
| Changed root at invocation                                                      | `TestFilesystemRootChangedAfterRegistration`                                                                              |
| Production CLI/history/no-content capture and root-grant revocation             | `TestProductionFilesystemWorkflowHistoryAndRevocation`                                                                    |
| Queued cancellation preserves real committed copy, with no pending deletion     | `TestQueuedFilesystemShutdownPreservesCommittedCopy`                                                                      |
| Parser/resource/performance baseline                                            | `FuzzFilesystemPath`, `BenchmarkFilesystemCopy`                                                                           |

## Verification and open acceptance

Full local ordinary/race suites passed, with three Windows symlink security tests
explicitly unavailable because the local account lacks symlink privilege. Tests
fail instead of skipping under CI/GITHUB_ACTIONS if that prerequisite is missing.
Fresh native CI must execute them on Windows and execute Unix link/special-file
tests on Linux/macOS before acceptance. Local skips and cross-compilation alone
are not native security evidence.

Go 1.27.4 final production source passed vet/build/module verification/tidy diff,
latest installed govulncheck (no findings), Go formatting and all-workflow
actionlint. Linux/Darwin fileaction/CLI test binaries cross-compiled. Full local
coverage: fileaction 87.3%, engine 85.3%, execution 86.2%, workflow 94.3%, HTTP
94.9%, shell 82.7%, Docker 88.0%, configuration 97.5%, CLI 82.3%. Subsequent
test-only boundary/link-swap extensions compiled/passed locally with the same
explicit privilege gate; affected filesystem/CLI race repetitions also passed.

Four-second/two-worker path fuzzing passed 58,964 executions. Three short Windows
4 KiB copy samples measured 4.813-5.188 ms/op, 42,256-42,297 B/op and 176-177
allocs/op. Setup and final-target removal between iterations are excluded; flush,
hard-link publication and temporary cleanup are included. CI requires this family.
No optimization, SLA, network filesystem or crash durability claim is inferred.

See [operator reference](../../docs/filesystem-actions.md) and the append-only
[build log](../build-log.md) for exact commands, unavailable tests and limitations.
The three filesystem rows remain IMPLEMENTED pending all native/scanner/benchmark/
Markdown/CodeQL/release checks. The next
[notification proposal](../designs/notification-actions.md) is concrete but PROPOSED,
with no notification runtime, credential store or Phase 05 implementation.

## Combined-source acceptance — 2026-11-22

All six PR workflows passed final combined source `2bb4908`:

| Workflow           | Successful run                                                               | Acceptance evidence                                                                         |
| ------------------ | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| CI                 | [45018171115](https://github.com/PHPDevsr/opsweave/actions/runs/45018171115) | Linux/macOS/Windows on Go 1.27.4 and stable, Linux race, formatting/modules/vet/build/smoke |
| Vulnerability scan | [45018171088](https://github.com/PHPDevsr/opsweave/actions/runs/45018171088) | govulncheck@latest on three native OS families                                              |
| Benchmarks         | [45018171119](https://github.com/PHPDevsr/opsweave/actions/runs/45018171119) | Required FilesystemCopy and existing families, with environment/results artifacts           |
| Markdown           | [45018171157](https://github.com/PHPDevsr/opsweave/actions/runs/45018171157) | Bun formatting                                                                              |
| CodeQL             | [45018171204](https://github.com/PHPDevsr/opsweave/actions/runs/45018171204) | Static security checks                                                                      |
| Release            | [45018171127](https://github.com/PHPDevsr/opsweave/actions/runs/45018171127) | Six OS/architecture packages plus assembly; publication skipped                             |

The three Windows symlink tests cannot use their local missing-privilege skip
path under CI/GITHUB_ACTIONS. Both Windows jobs passed this mandatory gate, and
Linux/macOS executed their native link/special-file tests. The fresh native
results close the platform/security gate; they do not relabel the earlier local
privilege skips as passing tests. Cross-compilation alone was never acceptance.

All workflows were pull_request events, with no duplicate push runs. No merge,
tag, release publication or deployment occurred. Full local ordinary/race,
static/build/module/scanner/fuzz/benchmark/format checks and code/security review
are recorded above, with their exact platform limitations and core coverage.
Final documentation links and 561-row reconciliation are recorded in the log.

Implementation, tests, self-review, English documentation and compatibility are
complete for `G-5.5-023`, `G-5.5-024` and `G-5.5-025`. Only these three rows now
transition from IMPLEMENTED to COMPLETED. Fourteen completed HTTP/shell/Docker
rows remain unchanged. No dependency, schema migration or historical release-doc
change is required. This supersedes earlier pending statements in this review.

Phase 04 remains in progress at the notification contract decision. Its four
fixed-profile, environment-credential, verified-network adapters are PROPOSED
only; no notification runtime, persistent secret store or Phase 05 work is added.
The PR remains draft targeting main.
