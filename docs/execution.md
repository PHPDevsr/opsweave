# Durable manual execution in development

Phase 02 adds the internal action executor, sequential engine, transactional
SQLite history and local execution commands. The [accepted contract](../harness/designs/manual-execution.md)
defines its scope. The production action catalog remains empty until built-in
integrations ship. Tests inject deterministic trusted actions; there is no CLI
flag, configuration setting, public SDK or plugin loader to enable those fixtures.
Consequently, production cannot register or run a usable workflow yet. Successful
parser validation alone never implies an available executable action.

The [Phase 03 serial reliability core](reliability.md) extends new runs with
conditions, references, retry/deadline policies, cancellation, replay, idempotency
and format-2 attempt history. The remaining Phase 02 description below records the
foundation behavior; use the reliability guide for those reviewed extensions.

## Build, configuration and commands

Follow [installation and configuration](foundation.md). The same Go binary and
SQLite path serve execution commands; no server, Redis or RabbitMQ is required.

```sh
opsweave execution list
opsweave execution list --json
opsweave run <workflow>
opsweave run --json <workflow>
opsweave execution get --json <execution-id>
opsweave execution logs --json <execution-id>
```

Replace placeholders with a registered workflow name and the returned opaque
32-character hexadecimal execution ID. With the current empty production catalog,
`run` reports an unavailable action version before creating an execution. Empty
history lists succeed. Help and completion do not load configuration or storage.

`run` waits synchronously and returns identifying metadata, including the exact
revision, execution ID and final state. Exit 0 requires durable success. Action
failure, cancellation, deadline expiry, infrastructure errors and output errors
exit 1. A durably failed run still writes its identifying result to stdout before
the safe stderr diagnostic. Use `execution get` for the exact historical definition,
ordered step states and redacted outputs; `logs` returns ordered structured
lifecycle events. Reading a failed execution successfully returns exit 0.

All four commands support `--json`, one JSON value plus newline, strict arguments
and non-interactive operation. Lists contain summaries without definitions or
outputs and sort by creation time then ID, newest first. Reads are bounded to
10,000 rows and 64 MiB; exceeding a limit fails without partial success. Exact
get/logs remain available for individual bounded histories. Store ownership tokens
are private and never returned.

## Execution behavior

Each submission pins the current immutable workflow revision and exact registered
action versions. Preflight checks stored source/normalization, input schema,
runtime compatibility, supported policies and explicit permission grants. A
concurrent update between preflight and submission returns a revision conflict
without invoking an action. Disabled workflows permit manual execution; archived
workflows reject new submissions and retain previous history.

Phase 02 supports the existing sequential chain with literal inputs. Each step
requires its predecessor to have succeeded. A failed or invalid-output step is
recorded as failed; remaining dependents become skipped with `dependency_failed`.
Skipped is never success. There is one attempt, one running step, and one active
execution per workflow across all revisions. Competing submissions fail rather
than creating an unowned background queue.

Execution states are pending, queued, running, succeeded, failed, cancelled and
timed_out. Step states are pending, running, succeeded, failed, skipped, cancelled
and timed_out. Unknown, repeated-terminal and otherwise invalid transitions fail.
Submission records pending/queued/running execution events atomically; it creates
all pending steps before action invocation. Each running step is committed before
its action starts. Completion/output/event writes are atomic. Execution success
requires every step to have succeeded; no storage failure becomes a successful
execution result.

The default execution deadline is one hour and each step has a five-minute child
deadline; an earlier caller deadline is inherited. Caller cancellation propagates
to supported handlers. A step deadline alone fails the execution; an execution
deadline produces timed_out. Simultaneous cancellation/deadline outcomes use the
first persisted terminal transition. Cleanup recording after cancellation has a
separate five-second deadline and reports failures. Handlers must cooperate with
contexts; no goroutine is abandoned to simulate forcibly stopping arbitrary Go.

Configurable retries/timeouts/concurrency, conditions, output references,
independent branches, parallel steps, management cancellation and recovery remain
Phase 03 work. Unsupported definition capabilities are rejected before submission.
Built-in HTTP, shell, Docker, file and notification adapters remain Phase 04;
database adapters remain Phase 09. Their common identifier/schema/version,
permission, structured-result, cancellation and error obligations are defined now.
No external side-effect or exactly-once guarantee is claimed by test fixtures.

## Security and capture limits

Only trusted compiled handlers can be injected at application composition. Each
registration is immutable metadata for an exact action version; unavailable or
incompatible versions never silently fall back. Required permissions need exact
explicit grants; wildcard grants are rejected. Dangerous integrations still need
their future action-specific configuration. Secret references remain unavailable
until encrypted secret resolution in Phase 05. Handlers shared between executions
must be concurrency-safe and bound their own memory and external operations.

Inputs are copied before invocation and outputs copied during capture. Validate
required/unknown fields, scalar/map/list types, finite numbers and UTF-8 strings.
Bound each capture traversal to depth 32, 20,000 nodes and 64 KiB of scalar/key
content. Encoded output is limited to 64 KiB per step and 1 MiB per execution;
overflow fails rather than silently truncating JSON. Recursively omit sensitive
and credential output fields, including sensitive array items. Raw action error
text and returned error payloads are discarded; failed outputs are not persisted.

Persist at most 1,024 engine-owned lifecycle events with timestamp, sequence,
execution/step IDs, safe event code and validated state/reason. These structured
events are the execution log in Phase 02. Arbitrary action text, raw inputs,
payloads and private tokens are excluded. Future action-specific logging requires
reviewed redaction and bounds. Schema declarations cannot identify a secret that
an incorrectly implemented trusted handler places in an ordinary public field;
action authors must classify outputs correctly.

## Upgrade, recovery and troubleshooting

Migration `002_executions.sql` adds execution, step and append-only event tables
without rewriting foundation migration 001 or historical revisions. The same
checksummed atomic migration mechanism applies. Back up the stopped process's
database before upgrade. There is no automatic down migration; rollback requires
restoring the pre-upgrade backup with its compatible binary. SQLite remains
single-node; ownership checks do not implement distributed leases or PostgreSQL
claiming. Existing backups preserve execution history alongside revisions.

After a crash, history remains readable but unfinished executions stay active and
block another run of that workflow. Phase 02 performs no automatic replay, unsafe
reset or implicit state repair. Phase 03 must supply reviewed recovery behavior.
Do not edit database rows to bypass this guard.

A persistence/commit error can leave the durable result uncertain. Inspect
`execution list`, `get` and `logs` using the reported ID before deciding what to
do; never automatically repeat a potentially non-idempotent action. Output failure
after commit also preserves the durable result. An action failure is distinguished
from a persistence error by safe error categories and step reasons. Permission or
version errors fail preflight with no action invocation. Database access/schema
errors keep their detailed causes internal to avoid leaking paths or data.

See the [test and review evidence](../harness/reviews/phase-02.md) and
[benchmarks](benchmarks.md). This development document is one source; future
release documentation must be generated from exact Git tags.
