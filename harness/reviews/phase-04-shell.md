# Phase 04 shell implementation review

Contract accepted by user reply `ACCEPT` on 2026-11-22 (Asia/Jakarta).
Scope: `shell.exec` version 1 and its existing CLI/runtime/configuration boundaries.

## Architecture and security

- Compose HTTP and shell registrations through one CLI factory with exact-version
  bindings and only enabled descriptor permissions. Existing injected runtimes
  stay isolated; no test executable can be enabled without explicit configuration.
- Profiles are copied, strictly decoded and revalidated on invocation. No PATH
  search, dynamic executable, shell string, arbitrary environment or stdin input.
  Arguments and capture have explicit bounds. Failures expose generic action
  errors and retain no failed stdout/stderr. Existing engine policies own retries,
  execution state, concurrency and cancellation.
- Linux/macOS use owned process groups under the reviewed trusted-command rule.
  Windows starts suspended, creates a non-breakaway kill-on-close job, binds the
  child before resuming, checks the initial thread PID, and propagates handle
  cleanup failures. Native cancellation and cleanup share one bounded budget.
- All os/exec capture owners are joined before buffer inspection and native handle
  release. No global mutable process state or detached waiter is introduced. No
  new dependency, schema migration, public API or workflow DSL change.

## Test mapping

| Area                                               | Observable tests                                                                                                            |
| -------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| Defaults, validation, version and grants           | `TestProfileValidationAndPermissions`, `TestShellConfiguration`                                                             |
| Direct argv, minimum environment, fixed directory  | `TestArgumentPreservationAndEnvironment`                                                                                    |
| Immutable snapshots and argument boundaries        | `TestSnapshotAndArgumentLimits`, `FuzzShellArguments`                                                                       |
| Output limits, UTF-8, escaping and failure secrecy | `TestCaptureFailureAndOutputLimits`                                                                                         |
| Cancellation/deadlines and descendants             | `TestCancellationDeadlineAndChildCleanup`, `TestExitedRootCannotLeaveCapturedChildRunning`                                  |
| Native ownership before execution                  | `TestWindowsSuspendedNonBreakawayOwnership`, `TestWindowsAssignmentFailureCannotRunExecutable`, `TestUnixOwnedProcessGroup` |
| Start failure and changed paths                    | `TestProcessStartFailureAndProfileRecheck`                                                                                  |
| CLI to durable history and revocation              | `TestProductionShellWorkflowAndRevocation`                                                                                  |
| Queued execution and joined shutdown               | `TestProductionQueuedShellShutdown`                                                                                         |
| Performance                                        | `BenchmarkShellExecution`, asserted in benchmark CI                                                                         |

Fixtures are built into isolated temporary directories and use loopback handshakes
for process readiness, cancellation and peer closure. Windows may close a killed
process's TCP connection with WSAECONNRESET rather than EOF; tests accept that
specific native reset after the handshake while retaining timeout/survival failure
assertions. No assertion is removed to bypass surviving descendants.

See [shell reference](../../docs/shell-action.md) and the append-only
[build log](../build-log.md) for exact commands/outcomes, native CI evidence,
configuration, trust boundary and compatibility. Requirement status remains
in-progress/implemented until all required acceptance gates pass. Docker,
filesystem and notification adapters are separate unimplemented work units.

## Local verification before native CI

Final full ordinary/race suites, vet, build, module verification, tidy diff and
latest vulnerability scan passed on Go 1.27.4. Coverage is shell 82.7%, HTTP 94.9%,
engine 82.7%, execution 86.2%, workflow 94.3%. Final review balanced each native
System32 library load/free, then repeated full checks. Fuzzing passed 83,022 inputs.
The final Windows process benchmark measured 453-494 ms/op in three short
single-iteration samples; no optimization or SLA is claimed. Native Linux/macOS
execution remains a CI gate; local cross-compilation alone does not close it.

## Shutdown acceptance follow-up

Native shell tests passed on all three supported OS families in the first shell
CI run. A separate pre-existing immediate server-stop test failed intermittently
on Ubuntu/stable. Startup and scheduler scans now follow the dispatcher's existing
five-second independent durable-metadata budget; a completed admission may remain
queued during shutdown. `TestDispatcherShutdownDuringDurableScan` covers both
scans, context release, preserved storage errors, no invocation after stop, and
committed startup intent. The scheduler integration fixture explicitly releases
its scan and verifies joined shutdown plus cancellation of an already running
action. No assertion was removed. The regression fails with the original scan
contexts and passes with the fix. Exact local/remote outcomes are appended to the
build log; all-check acceptance remains pending until fresh CI passes.

The subsequent Windows/stable CI run exposed a fixture clock crossing a cron
minute (:58 plus two seconds). The scheduler correctly admitted another intent.
The first real scan retains actual admission time. The second fake scan, whose
only purpose is the shutdown handshake, now returns an empty completed result
after its explicit release. It does not admit a second cron occurrence when the
simulated clock crosses a minute. All execution-count, first real admission,
cancellation and join assertions remain. This test-only correction does not
change the final dispatcher source.

## Combined-source acceptance — 2026-11-22

Final combined source `3bd23de` passed every required PR workflow:

| Workflow           | Successful run                                                               | Evidence                                                                                                           |
| ------------------ | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| CI                 | [44976628487](https://github.com/PHPDevsr/opsweave/actions/runs/44976628487) | Linux/macOS/Windows on Go 1.27.4 and stable, Linux race, formatting, module consistency, vet/build and smoke tests |
| Vulnerability scan | [44976628507](https://github.com/PHPDevsr/opsweave/actions/runs/44976628507) | govulncheck@latest on three native OS families                                                                     |
| Benchmarks         | [44976628467](https://github.com/PHPDevsr/opsweave/actions/runs/44976628467) | Required benchmark families, including ShellExecution                                                              |
| Markdown           | [44976628468](https://github.com/PHPDevsr/opsweave/actions/runs/44976628468) | Bun formatting                                                                                                     |
| CodeQL             | [44976628471](https://github.com/PHPDevsr/opsweave/actions/runs/44976628471) | Static security analysis                                                                                           |
| Release            | [44976628571](https://github.com/PHPDevsr/opsweave/actions/runs/44976628571) | Linux/macOS/Windows amd64/arm64 packages and assembly; publish skipped                                             |

All six were pull_request runs; no duplicate push workflow was triggered. No
release was published. Native tests close the platform gate; local cross-compile
results alone were never counted as native acceptance.

Local final production source passed full ordinary/race suites, vet, build,
module verification and vulnerability scan. Core coverage: engine 85.6%,
execution 86.2%, workflow 94.3%; shell 82.7%, HTTP 94.9%. The subsequent scheduler
fixture-only correction passed 30 ordinary and 10 race repetitions, then the full
native matrix above. Formatting and scoped ledger reconciliation also passed.
The recorded failed intermediate runs remain in the build log.

Implementation, tests, self-review, English documentation, compatibility and
security evidence are complete for `G-5.5-016`, `G-5.15-007` and `G-5.15-017`;
these three rows are now COMPLETED. Existing four HTTP rows stay completed.
No dependency or schema migration is required. This status does not complete
Phase 04: the next Docker contract still requires human review, and the phase PR
remains draft targeting main. Filesystem/notification and Phase 05 remain open.
