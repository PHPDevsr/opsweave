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
