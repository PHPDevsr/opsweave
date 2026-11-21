# Foundation benchmarks

OpsWeave measures the parser, expression validator, registry preflight, single-node
SQLite repository and Phase 02 manual execution/history. Execution uses only
deterministic in-process action doubles. These measurements do not represent real
integrations. No product optimization or
dependency change is included in this measurement baseline.

## Running locally

From the repository root, with the Go version declared in `go.mod`:

```sh
go test -run '^$' -bench '^Benchmark' -benchmem -benchtime=200ms -count=3 -cpu=1 ./internal/httpaction ./internal/webhook ./internal/schedule ./internal/workflow ./internal/registry ./internal/storage/sqlite ./internal/execution ./internal/engine
```

Use `-benchtime=1x -count=1` for a one-iteration correctness smoke run, or
`-benchtime=1s -count=5` for more samples before comparing a proposed optimization.
Keep the same settings for both sides of a comparison. For example, select only
storage writes with `-bench '^BenchmarkSQLiteApply$'` and the SQLite package path.
Benchmarks use only generated fixture workflows and isolated temporary databases;
no production resources, secrets or external integrations are required.

`ns/op` measures elapsed time per operation; `B/op` and `allocs/op` report Go heap
allocation. Parser/preflight `MB/s` uses source bytes processed, not disk bandwidth.
The `workflows/op` and `revisions/op` fields explain batch sizes so an operation
containing 16 workflows is not confused with one workflow. Setup, fixture
construction and catalog creation are outside the timed operation. Checks inside
the loop fail on errors or incomplete results; no mutable global sinks are used.

On Windows, a writable existing directory can be selected with `GOTMPDIR` when
coverage/benchmark temporary-file operations are restricted. Benchmarks do not
need CGO; race testing is a separate correctness check. Run performance samples
without race or coverage instrumentation and without competing heavy test suites.

## Cases and measurement boundaries

| Benchmark                                                            | Workload and timed operation                                                                                     |
| -------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `BenchmarkParse/Steps1`, `/Steps32`, `/Steps256`                     | YAML audit, typed validation, default normalization, DAG/reference validation and original-source copy           |
| `BenchmarkValidateCondition/Literal`, `/StepPredicates`, `/Terms128` | Boolean expression syntax validation and reference collection; no evaluation                                     |
| `BenchmarkPrepare/Steps1`, `/Steps32`, `/Steps256`, `/Batch16`       | Full source parsing, injected catalog/input schema validation, capability checks and deterministic normalization |
| `BenchmarkSQLiteApply/Batch1`, `/Batch16`                            | Public repository validation, random revision IDs, atomic inserts/current publication and durable commit         |
| `BenchmarkSQLiteGet/Current`, `/Historical`                          | Current or first historical source/normalized revision reads from a database with 128 revisions                  |
| `BenchmarkSQLiteList/Workflows1`, `/Workflows128`                    | Sorted current workflow reads including stored source/normalized payloads                                        |
| `BenchmarkSQLiteMigrate`                                             | Fresh database creation, migration/ledger transaction and connection close                                       |

The preflight catalog is validation-only test metadata and is never registered by
the production CLI. Fixture generation uses valid unique IDs and the same public
parser/registry boundaries that real definitions must pass once actions ship.

SQLite uses the production one-connection, foreign-key, DELETE journal,
synchronous FULL and five-second busy-timeout settings. The apply benchmark never
disables durability or mutates historical revisions to improve a score. It caps
each test database at 128 total revisions, then stops the timer, closes/removes
that isolated file and opens/migrates a fresh database at the same path. Thus disk
usage and the workload's history size stay bounded even for long benchmark runs.
Reset/migration costs are excluded from apply and measured by the separate fresh
migration benchmark. The read benchmarks seed fixed datasets before timing and
do not modify them. SQLite reports include API parsing/serialization overhead and
OS/filesystem latency, not just SQL statement execution time.

## CI and artifacts

Phase 03 adds condition evaluation, a deterministic retry-policy lifecycle and
format-2 attempt-history/log reads (`EvaluateCondition`, `ReliablePolicy` and
`ReliableHistory`). The dispatcher adds `DispatcherClaim` and
`DispatcherThroughput/Workers1,Workers4`: an atomic durable claim, and full
admission-to-completion of eight independent no-op steps using a bounded pool.
Native ownership/setup and claim-fixture admission/settlement are excluded from
the claim timer; throughput includes admission, policy persistence and polling.
Both workloads cap each database at 128 histories and reset only isolated files
outside timing. They do not measure real external-action speedup. `ManualExecution` now measures the production
format-2 serial coordinator, including durable attempt boundaries and owned control
polling. Migration measurements now include migration 004. These workloads differ from
earlier Phase 02 reports; compare like code/toolchains and retain commit attribution.

Phase 02 adds `BenchmarkTransition`, which validates and advances an independent
step snapshot without I/O; `BenchmarkManualExecution`, which runs the two-step
engine against SQLite with empty-output deterministic doubles; and
`BenchmarkExecutionHistory/Get` and `/Logs`, which read a seeded terminal history.
Manual execution measures preflight, durable step boundaries and final history,
not external network/process latency. It caps each temporary database at 128 runs,
resets only its owned file outside timing, and excludes setup/migration costs.
History reads use fixed data prepared before timing. Every case checks outcomes
and fails on errors. That Phase 02 baseline included additive migration 002; current measurements also
include migration 004 and must not be treated as identical migration workloads.

The `Benchmarks` workflow runs on relevant pull requests, matching pushes to
`main`, and manual dispatch. A Linux job uses the declared Go version, one logical
CPU setting (`-cpu=1`), three samples and 200 ms target measurement windows. It
verifies all twenty benchmark families, including both execution prune cases and the worker status probe. Benchmark errors or missing
families fail the job; shell pipe failures cannot be hidden by `tee`.

Each run uploads `benchmark-results.txt` plus `benchmark-environment.txt` with
the checked-out commit, Go/OS/architecture/CGO settings, kernel and CPU information.
Download `foundation-benchmarks-<sha>` from the run's Artifacts section; retention
is 14 days. Only those two generated reports are uploaded. Database files, source
workflows, caches and credentials are excluded. The workflow has read-only
repository permissions and uses full action commit pins; it does not post PR
comments or write a performance baseline back into the repository.

These results establish measurements for review. Shared hosted runners, temporary
filesystem placement, caching, CPU load, Go version and hardware affect timing,
especially fsync-heavy SQLite writes. There is no arbitrary absolute-time failure
threshold yet. Compare multiple samples on comparable environments and inspect
allocations alongside timing before proposing an optimization. Cross-platform
unit/race/security CI remains a separate correctness gate, and the existing
CodeQL repository configuration gate remains unchanged.

`BenchmarkCronScan` measures a bounded 24-hour calendar scan with a compiled IANA
schedule. `BenchmarkScheduleDueScan` measures one SQLite cursor read, calendar
calculation, and durable cursor commit for a schedule with no matching minute.
The second benchmark updates one bounded fixture row. It excludes action work.

`BenchmarkWebhookAuthentication` measures request construction, HMAC verification,
JSON validation and receipt serialization with an in-memory admission stub.

`BenchmarkExecutionPrune/Apply100` measures deletion of 100 terminal executions and their transition rows in one transaction. `/Preview1000` measures eligibility reporting over 1000 terminal executions. Fixture population is excluded from both measurements.

`BenchmarkServerWorkerStatus` measures opening SQLite read-only, checking migration compatibility and reading the pool snapshot with 16 queued executions. Native lock probing is outside this storage benchmark.

`BenchmarkHTTPPolicy` measures URL normalization, origin matching and public-IP
policy validation without network I/O. It does not measure endpoint latency.
