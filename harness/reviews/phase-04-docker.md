# Phase 04 Docker implementation review

Contract accepted by user reply `ACCEPT` on 2026-11-22 (Asia/Jakarta), answering
the concrete four-action local-IPC/allowlist question. Scope is `docker.start`,
`docker.stop`, `docker.restart`, `docker.inspect`, version `1`, within Phase 04.

## Code and security self-review

- New `internal/dockeraction` stays behind existing action descriptors/runtime.
  CLI composes only enabled registrations and exact grants; business/domain and
  storage interfaces remain unchanged. Config snapshots container/action grants.
- Strict native endpoint parsing excludes remote transports, ambiguous paths and
  ambient Docker configuration. Inputs cannot change endpoint, operation, version
  or permissions. Full container IDs are validated before connecting. Dangerous
  access is disabled by default and inspection can be granted independently.
- Each invocation opens one local IPC connection, writes one fixed API 1.48
  request and uses bounded standard HTTP parsing directly. There is no HTTP
  transport pool, proxy lookup, redirect, decompression, cookie jar or retry.
  Headers/body/timeouts are bounded; the runtime checks exact schemas/output cap.
- The cancellation callback owns the same once-closed connection and is joined.
  Rejection closes IPC before response-body cleanup, preventing unbounded draining.
  Close/deadline/read failures return generic action errors with no failed output.
  Final budget checks discard output after cancellation/deadline.
- Windows opens non-inheritable overlapped pipe handles with anonymous SQOS,
  verifies deadlines work, and bounds busy-pipe connection waits. Native tests
  attempt impersonation and verify that the service token cannot be obtained.
  See [CreateFile security flags](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew).
- Inspect decodes only a fixed typed projection, checks ID/hash/name/status and
  required fields, and never captures daemon configuration, environment or mounts.
  Existing lifecycle logs carry action/execution/step identities without endpoint
  paths, daemon bodies or credentials. No production daemon access is used in tests.
- Native fake-daemon utilities are imported only by tests/benchmarks. Their
  connection owners are bounded and joined. Windows pending `OVERLAPPED` memory
  is pinned until completion/cancellation joins; its event closes afterwards.
  This follows the [Go Pinner contract](https://pkg.go.dev/runtime#Pinner.Pin).
  No native pending pointer is left on a movable Go stack.
- No dependency, schema migration, historical revision mutation, management API,
  public workflow syntax change, Docker sandbox or exactly-once claim is added.

## Observable test mapping

| Area                                                                           | Tests                                                                                        |
| ------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------- |
| Opt-in/native endpoint/full-ID/action policy                                   | `TestDockerPolicyAndOptIn`, `TestDockerConfiguration`                                        |
| Fixed methods/API/paths/grace/statuses and filtered inspect                    | `TestDockerOperationsAndFilteredInspect`                                                     |
| Immutable grants, rejected inputs, version/permission checks before I/O        | `TestDockerAllowlistSnapshotAndPermissions`, `TestConfiguredDockerPermissionsAreIndependent` |
| Daemon/version/status errors, redirect, encoding, malformed/oversize responses | `TestDockerResponseRejections`, `TestDockerInspectValidation`                                |
| Cancellation/deadline and peer closure                                         | `TestDockerCancellationDeadlineAndPeerClosure`, `TestWindowsBusyPipeDialRespectsDeadline`    |
| Rejection cannot drain stalled oversized body                                  | `TestDockerOversizeResponseDoesNotDrain`                                                     |
| Independent concurrent invocation ownership                                    | `TestDockerConcurrentInvocations`                                                            |
| Native anonymous token security                                                | `TestWindowsPipeCannotImpersonateServiceToken`                                               |
| Four production CLI actions, SQLite history, redaction and revocation          | `TestProductionDockerWorkflowHistoryAndRevocation`                                           |
| Queued server shutdown and durable cancellation                                | `TestProductionQueuedDockerShutdown`                                                         |
| Unavailable/canceled native connection setup                                   | `TestDockerUnavailableEndpoint`                                                              |
| Bounded parsers and IPC performance                                            | `FuzzDockerInspect`, `FuzzDockerPolicy`, `BenchmarkDockerInspectIPC`                         |

## Local verification and remaining acceptance

Full ordinary and race suites passed on Go 1.27.4/Windows with CGO/Clang for race.
Vet, build, module verification, tidy diff, Go formatting and workflow actionlint
passed. govulncheck@latest was installed and reported no vulnerabilities. Final
coverage: Docker 87.6%, engine 85.5%, execution 86.2%, workflow 94.3%, HTTP 94.9%,
shell 82.7%, configuration 97.1%, CLI 82.4%.

An initial race run found a fake-daemon shutdown owner waiting on a moved stack
address retained by pending Windows I/O. Diagnostic stacks plus escape analysis
identified the lifetime issue. Pinning the structure until joined completion fixed
it; 30 queued shutdown race repetitions, three full Docker race repetitions, then
full ordinary/race suites passed. No join assertion was removed or timeout
expanded to conceal the failure. The build log retains failed and passing evidence.

Four-second/two-worker fuzz runs passed 113,091 inspect and 135,163 policy inputs.
Three short Windows IPC benchmark samples measured 122,476-155,917 ns/op,
19,455-19,461 B/op and 108 allocs/op. Listener/policy setup is excluded. Shared
host load and OS affect these measurements; no optimization or daemon latency
claim is made. Benchmark CI requires this family. Linux/Darwin native test binaries
cross-compiled; this does not replace native execution acceptance.

See [operator reference](../../docs/docker-actions.md) and [build log](../build-log.md).
Six owned Docker/security rows remain IMPLEMENTED pending fresh native multi-OS
CI, scanner, benchmark, Markdown, CodeQL and release-preview evidence. The next
[filesystem proposal](../designs/filesystem-actions.md) is design-only and still
requires human review. Notification contracts and Phase 05 remain unapproved.

## Native deadline follow-up

Initial native CI on `62dcf4a` exposed the socket-deadline/context-timer race in
`TestDockerCancellationDeadlineAndPeerClosure`: Linux/macOS sometimes classified
the deadline as ordinary action failure before the parent timer propagated Err.
The action now joins context cancellation at an elapsed action deadline, after
owned I/O cleanup, so the existing runtime sees the correct parent deadline.
The original DeadlineExceeded assertion is unchanged. Five ordinary/five race
repetitions passed locally; full suites and fresh native CI are tracked in the
build log. Other initial workflows, including six release packages plus assembly,
passed; the failed CI run is not acceptance evidence.

Final correction verification passed full ordinary/race suites, vet/build/module
checks and scanner. Coverage amendment: Docker 88.0%, engine 85.6%; other core
coverage unchanged. The earlier Windows benchmark remains attributed to source
62dcf4a; the final CI benchmark will measure corrected source separately. Native
all-check acceptance remains pending.

## Existing serial retry boundary follow-up

All Docker native tests passed after the IPC/context correction. CI on 16d86c6
found only Windows/minimum Go's existing serial backoff deadline test: an early
context wake reached storage before the stored UTC retry timestamp. The coordinator
now finishes a bounded metadata wait before timeout validation; system clock
waits recheck due time. State validation is unchanged. The new
`TestReliableCoordinatorEarlyDeadlineCannotStartRetry` fails deterministically
on original source and passes on the fix, preserving one invocation/attempt and
the timed-out step outcome. Ordinary/race repetitions passed; full/fresh CI
evidence remains tracked in the append-only log. See [reliability docs](../../docs/reliability.md).

## Combined-source acceptance — 2026-11-22

All six workflows passed on final combined source `3ecbe68`:

| Workflow           | Successful run                                                               | Acceptance evidence                                                                                       |
| ------------------ | ---------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| CI                 | [44980216309](https://github.com/PHPDevsr/opsweave/actions/runs/44980216309) | Linux/macOS/Windows on minimum Go 1.27.4 and stable; Linux race; formatting/module/vet/build/smoke checks |
| Vulnerability scan | [44980216282](https://github.com/PHPDevsr/opsweave/actions/runs/44980216282) | govulncheck@latest on all three OS families                                                               |
| Benchmarks         | [44980216287](https://github.com/PHPDevsr/opsweave/actions/runs/44980216287) | Required DockerInspectIPC plus existing benchmark families and artifact upload                            |
| Markdown           | [44980216333](https://github.com/PHPDevsr/opsweave/actions/runs/44980216333) | Bun formatting                                                                                            |
| CodeQL             | [44980216360](https://github.com/PHPDevsr/opsweave/actions/runs/44980216360) | Static security checks                                                                                    |
| Release            | [44980216283](https://github.com/PHPDevsr/opsweave/actions/runs/44980216283) | All six Linux/macOS/Windows amd64/arm64 packages and assembly; publish skipped                            |

Every run used the pull_request event. No duplicate push run, release publication,
merge or deployment was performed. Native execution closes the platform gate;
cross-compilation alone was not accepted as native evidence.

Full local ordinary/race, vet/build/module/scanner/format/workflow checks passed
on final source. Final coverage is Docker 88.0%, engine 85.0%, execution 86.2%,
workflow 94.3%; HTTP/shell/config/CLI coverage is unchanged. The final native CI
also covers both deadline corrections and their unchanged/new regression assertions.
Earlier failed results remain documented rather than relabeled as passes.

Implementation, automated tests, code/security self-review, English operator
reference and compatibility evidence are complete for `G-5.5-018`, `G-5.5-019`,
`G-5.5-020`, `G-5.5-021`, `G-5.15-008` and `G-5.15-018`. Only these six rows now
transition from IMPLEMENTED to COMPLETED. Seven completed HTTP/shell rows remain
unchanged. No dependency, schema migration or historical release-doc change is
required. This supersedes earlier pending acceptance statements in this review.

Phase 04 remains in progress at review of the concrete filesystem proposal.
Its deletion, exclusive hard-link copy publication and cooperative cancellation
limits require human acceptance before implementation. The phase PR remains
draft targeting main; notification contracts and Phase 05 remain unapproved.
