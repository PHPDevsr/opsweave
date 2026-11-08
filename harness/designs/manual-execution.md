# Phase 02 manual execution contract proposal

Status: `ACCEPTED_FOR_IMPLEMENTATION`, 2026-11-08 (Asia/Jakarta). The user replied
"ACCEPT" to all four decisions after Phase 02 authorization. Implementation and
verification evidence are tracked in the [phase review](../reviews/phase-02.md);
contract approval alone does not claim feature completion.

Read with the accepted [workflow contract](workflow-v1.md),
[foundation contract](foundation-storage-cli.md) and [phase plan](../phases/phase-02.md).
Internal Go names and SQL column layout can change during implementation; the
observable semantics below require review before implementation.

## Decisions for review

1. Run synchronously against the current immutable revision, with one active
   execution per workflow. A disabled workflow may be run manually; an archived
   workflow may not. Reject concurrent submissions rather than queueing work
   without an owner. A crash preserves unfinished history and blocks a new run
   for that workflow until Phase 03 supplies reviewed recovery behavior.
2. Retain the currently supported sequential chain and literal inputs in Phase 02. A failed step explicitly skips its remaining dependents. Keep configurable
   retries, conditions, output references, independent branches, parallelism,
   management cancellation and automatic recovery gated to Phase 03. Enforce
   existing default context deadlines and propagate caller cancellation now.
3. Bind trusted compiled action implementations to exact registered versions.
   Preserve the empty production catalog until built-in integrations ship;
   deterministic action doubles are test-only. Persist schema-validated,
   redacted outputs and engine-generated structured lifecycle events. Never
   persist arbitrary action error text or unrestricted action log messages.
4. Add migration 002 for execution/step/event history and the `run` and
   `execution list/get/logs` commands. Keep migration 001 unchanged. Reads are
   bounded and historical definitions always use the execution's exact revision.

## Boundaries and preflight

- `internal/execution`: SQL/CLI-independent states, transition validation,
  snapshots and repository contracts.
- Internal action executor: reuse `registry.Descriptor` schemas, identifier,
  API version, permissions and exact action-version bindings; inject trusted
  handlers through application composition. No public SDK or plugin loader.
- Internal dispatcher/engine: resolve the revision, validate runtime availability
  and permissions, submit durable state, execute steps sequentially and persist
  results. Depend on repository interfaces rather than SQLite.
- `internal/storage/sqlite`: implement atomic execution operations and bounded
  reads through the existing database handle and migration mechanism.
- `internal/cli`: synchronous invocation and safe text/JSON presentation using
  existing configuration, output streams, errors and context conventions.

Preflight reparses the stored exact source, checks recognized normalization and
bindings, verifies that the runtime supports every action version and capability,
and rejects unsupported policies before creating an execution. No fallback to a
different action version. Submission rechecks archive/current-revision identity
inside its transaction; a concurrent workflow update returns a conflict requiring
fresh preflight. Updating or archiving after submission cannot change its pinned
definition. Enablement controls automatic triggers, so manual execution is allowed
while disabled. Archive prevents new submissions and preserves existing history.

## State transitions

All state names use lower-case snake case in storage and JSON. Creation is not a
transition from an existing state. The following tables enumerate allowed edges;
all other edges, including repeated terminal transitions, are rejected.

| Execution state | Allowed next states                             |
| --------------- | ----------------------------------------------- |
| `pending`       | `queued`, `cancelled`                           |
| `queued`        | `running`, `cancelled`                          |
| `running`       | `succeeded`, `failed`, `cancelled`, `timed_out` |
| `succeeded`     | None                                            |
| `failed`        | None                                            |
| `cancelled`     | None                                            |
| `timed_out`     | None                                            |

| Step state  | Allowed next states                             |
| ----------- | ----------------------------------------------- |
| `pending`   | `running`, `skipped`, `cancelled`               |
| `running`   | `succeeded`, `failed`, `cancelled`, `timed_out` |
| `succeeded` | None                                            |
| `failed`    | None                                            |
| `skipped`   | None                                            |
| `cancelled` | None                                            |
| `timed_out` | None                                            |

State legality also depends on aggregate invariants: only a running execution
can start a step, its dependencies must have succeeded, and only one step may be
running. A skipped dependent has a safe `dependency_failed` reason. Execution
success requires every step to have succeeded; failure requires a failed or
timed-out step and terminal remaining steps. Cancellation terminates pending
steps as cancelled; it cannot convert previously terminal steps. Execution
deadline expiry produces timed_out and caller cancellation produces cancelled;
simultaneous cases use the first accepted persisted terminal transition, as in
the accepted workflow contract. A step deadline alone produces a timed-out step
and failed execution.

Manual submission creates the execution, all pending steps, and the execution's
pending/queued/running events in one transaction. Intermediate events remain
auditable, but no unowned asynchronous queue is exposed. Only after commit may
the engine invoke an action. Each step start commits before invocation. Step
completion, validated output and its lifecycle event commit atomically. Final
execution state and its event also commit atomically after all steps are terminal.

Default execution timeout is one hour and step timeout five minutes, as already
specified in the accepted workflow contract. Child contexts inherit earlier
parent deadlines. Handlers must cooperate with cancellation; do not hide an
uncooperative handler behind an abandoned goroutine or claim an in-process
timeout can forcibly terminate arbitrary Go code. Phase 02 has no retry/replay
command or automatic restart processing. Phase 03 must extend the state contract
explicitly for retry attempts and recovery rather than bypass terminal guards.

## Action, errors and capture

The internal handler accepts a context and a validated, independently owned
`map[string]any`, and returns a structured output map plus an error. Schema types
remain string, boolean, integer (`int64`), finite number (`float64` or integer),
object and array. Unknown fields, missing required fields, unsupported values,
cycles and excessive nesting are rejected. Inputs and outputs must not retain
shared mutable references across the invocation boundary.

Validate descriptor permissions against injected grants before submission and
before invocation; missing grants fail closed. No wildcard grant or dynamic
loading is introduced. Built-in dangerous-action configuration belongs to Phase
04; descriptors cannot bypass those future controls. Credential references remain
unsupported until encrypted secret resolution exists in Phase 05.

Validate output schema before redaction. Omit sensitive or credential-marked
fields recursively from persisted/public results, including nested array values;
never substitute a string that violates the declared scalar type. Sensitive
outputs cannot be referenced by another step under the accepted workflow contract.
Bound validation traversal to depth 32 and 20,000 nodes and encoded output to
64 KiB per step and 1 MiB per execution. Reject oversized results rather than
silently truncating structured data. These are capture limits: trusted handlers
must also bound their own allocations and external I/O.

Persist at most 1,024 lifecycle events per execution, with sequence, UTC timestamp,
execution/step identifiers, event code and validated state/reason. This covers the
256-step sequential limit without arbitrary free-form payloads. Structured slog
output uses the same safe vocabulary. Raw inputs, action error strings, secrets,
output payloads and private ownership tokens never appear in lifecycle logs.
Integrations can add reviewed, bounded, redacted action-specific logging later.

Use typed error categories for validation/permission conflict, action failure,
invalid output, cancellation/deadline and repository failure. Public diagnostics
contain safe fixed messages and identifiers only. Wrap infrastructure causes for
internal `errors.Is`/`errors.As` inspection without formatting them into ordinary
CLI diagnostics. Never classify a failed durable write as successful execution.

After action failure, persist its safe failure classification and dependent skips.
If persistence fails, stop invoking further actions and return an infrastructure
error with the execution ID when known. A commit error may be ambiguous: instruct
callers to inspect history, never automatically resubmit or repeat side effects.
When caller cancellation prevents recording terminal state, attempt cleanup using
a separate context bounded to five seconds; report any cleanup failure. An
unfinished durable state is preserved rather than invented as a successful end.

## Storage and ownership

Introduce new tables through an additive checksummed migration:

| Table              | Required data/invariants                                                                                     |
| ------------------ | ------------------------------------------------------------------------------------------------------------ |
| `executions`       | Random 128-bit ID, workflow/exact revision FK, state, timestamps, version, private owner token, output bytes |
| `execution_steps`  | Execution/step key, ordinal, exact action/version, state, attempt, safe reason, redacted output, timestamps  |
| `execution_events` | Execution/sequence key, timestamp, optional step ID, safe event code and state/reason                        |

The composite revision FK must enforce workflow ownership. Record the exact
action versions from immutable revision bindings. Preserve source/normalized
definition in the revision table rather than duplicating a mutable definition.
An active-workflow uniqueness constraint covers pending, queued and running
states across all revisions. Conditional writes compare expected state/version
and owner; concurrent or stale writers fail visibly. This is local ownership,
not a distributed lease or an exactly-once external-effect guarantee.

Validate domain transitions before SQL mutations and recheck persisted state and
dependencies inside the transaction. Roll back state/output/event changes
together on any error. Event rows are append-only; terminal snapshots are
immutable through repository operations. Never expose owner tokens in read DTOs.
Retain current SQLite timeout, foreign keys and synchronous durability settings.

Migration tests cover upgrading an existing foundation database without changing
its revisions, failed DDL rollback, mismatched history and reopening execution
history. There are no automatic down migrations. Before upgrade take a stopped
process backup; rollback requires restoring that backup with the matching binary.
No tags exist and no released documentation source is changed by this proposal.

## CLI contract

| Command               | Result                                                                                  |
| --------------------- | --------------------------------------------------------------------------------------- |
| `run <workflow>`      | Resolve current revision and wait for terminal result; report execution ID/state        |
| `execution list`      | Summary rows ordered by creation time then ID, newest first                             |
| `execution get <id>`  | Exact revision definition, execution metadata, ordered step states and redacted outputs |
| `execution logs <id>` | Persisted safe lifecycle events in ascending sequence                                   |

All commands accept `--json` and preserve existing help/completion, strict flags,
non-interactive operation and safe stderr conventions. `run` exits 0 only for a
durably succeeded execution; failed/cancelled/timed-out or infrastructure/output
failures exit 1. Read commands exit 0 when retrieving a failed execution. Not-found
and limit errors exit 1. JSON is one value plus newline. On a durably failed run,
stdout still contains its identifying result before the safe stderr diagnostic.

Lists omit output/definition payloads. Bound reads to 10,000 rows and 64 MiB,
returning a limit error without partial success; exact get/logs remain available
for bounded individual history. Invalid identifiers are rejected before queries.
Output errors after durable changes do not undo them or justify automatic reruns.
No management listener or unsafe history-reset command is introduced. With the
approved empty production catalog, unavailable handlers fail preflight; test
injection proves execution behavior without shipping a fake built-in action.

## Implementation acceptance and review

Implementation must include exhaustive allowed/disallowed transition tests,
repository aggregate-invariant tests, and at least 80% engine/state coverage.
Use deterministic action doubles for success, schema rejection, action failure,
dependency skipping, parent/step deadlines and caller cancellation. Use context
deadlines and explicit synchronization rather than arbitrary sleeps.

Integration/E2E tests must cover immutable revision history after update/archive,
restart reads, migration/transaction fault injection, ambiguous write handling,
concurrent submissions through separate store handles, stale ownership rejection,
permission/version mismatches, nested redaction, bounds and safe diagnostics.
Exercise CLI text/JSON, exit codes, no-listener behavior and writer failures.
Add execution/storage benchmarks to the existing benchmark CI with no arbitrary
runner timing threshold. Run all phase checks and relevant existing regression
suites before reporting implementation or completion.

Design self-review: this proposal preserves the current sequential compatibility
boundary and empty catalog. Durable-before-invocation ordering limits accidental
repeats but cannot guarantee exactly-once external side effects. Crash recovery
and action-specific logging remain explicit later contracts. No implementation,
test success or security qualification is inferred from this design alone.
