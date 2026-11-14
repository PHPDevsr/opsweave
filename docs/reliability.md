# Serial reliability core in development

The first Phase 03 unit implements the [accepted execution contract](../harness/designs/reliable-execution.md)
for a serial coordinator. It supports conditional DAG traversal, typed output
references, bounded retry/backoff, execution/step deadlines, durable cancellation
requests, seven-day admission deduplication and whole-workflow replay. New runs
use execution format 2 with separate durable attempt history.

The production CLI now uses the accepted [single-node dispatcher](dispatcher.md)
for bounded parallelism, queued claims and conservative restart recovery. This
page describes the underlying policy core; the dispatcher guide supplies current
commands, configuration and ownership/migration procedures. Scheduler, triggers
and network listeners remain unavailable. The production action catalog is still
empty until Phase 04; deterministic handlers are test-only.

## Definition and execution policies

Use the [reviewed workflow syntax](workflow-parsing.md). The existing `if`, `needs`,
`retry`, `timeout` and output-reference fields now pass reliable CLI preflight with
a compatible action catalog. Planning and apply still reject unavailable actions,
secret references, trigger definitions.
No new expression grammar or arbitrary environment/code evaluation is added.

Readiness follows stable topological order, with authored order for ties. An
ordinary step requires all dependencies to succeed; otherwise it is explicitly
skipped. An explicit condition evaluates after dependencies are terminal and can
handle failed/cancelled dependencies. False skips; evaluation errors fail without
an action invocation. Independent roots may finish after another branch fails,
but are executed serially in this unit. Legitimately skipped steps may coexist
with overall success; a skipped step itself never becomes succeeded.

Only successful final outputs resolve references. Missing output or a type mismatch
fails input resolution without an invocation, never substitutes an empty string.
Values are independently copied and revalidated against action schemas. Sensitive
output references and credential/secret resolution remain unavailable.

A step remains running across retry backoff. Persist each actual attempt start
before invocation and its result before scheduling another attempt. Retry only
action failures, with total attempts 1–10 and the accepted capped exponential
backoff. Validation/reference errors, invalid/oversized outputs, permission denial,
cancellation, deadlines and storage failures are not retried. Attempt histories
are visible in `execution get` text/JSON and ordered lifecycle logs. Failed output
payloads and arbitrary action error strings are not persisted.

The execution budget starts at running; the step budget starts before first input
resolution and includes all attempts/backoff. Earlier caller deadlines propagate.
The legacy synchronous runner allows up to 30 seconds after cancellation to
persist the terminal outcome. Actions still observe the original deadline.

Absolute deadlines are stored, not reset between attempts. Exhaustion fails the
step; a step timeout fails the execution, while an execution deadline produces
timed_out. Failure/cancellation handlers do not erase the original outcome.

## Commands and cancellation

Existing [execution commands](execution.md) retain safe diagnostics, JSON/text and
exit-code conventions. New commands/options:

```sh
opsweave run --idempotency-key <key> <workflow>
opsweave execution cancel <id>
opsweave execution cancel --step <step-id> --json <id>
opsweave execution retry --idempotency-key <key> --json <id>
opsweave execution retry --ack-uncertain <id>
```

Cancellation returns a durable `cancellation_requested` acknowledgement and current
state, not a claim that an action has stopped. Individual requests identify the
target step and its state. Already-terminal targets return their unchanged result.
Repeated requests add no duplicate request events. Check `get`/`logs` for final
completion. The active owner observes requests at invocation boundaries and via
a bounded control poll while actions/backoff are active; cancellation is cooperative.
No detached goroutine pretends to forcibly terminate arbitrary Go code.

Whole cancellation stops new action starts and propagates to active contexts.
Unstarted steps are cancelled in one transaction so large graphs do not require
one filesystem commit per cancelled step. The coordinator waits for its owned
action/control work before returning. Cleanup recording has a shared five-second
budget and reports failure rather than claiming durable completion. A step-only
cancel permits other branches to finish; absent a failed/timed-out step, overall
status is cancelled. Whole-execution cancellation/deadline takes precedence.

Cancel/retry are local database administration commands, not an unauthenticated
network API. Legacy terminal history can be inspected/acknowledged unchanged.
Cancellation of a legacy unmanaged running execution is not falsely acknowledged
as supported; dispatcher ownership/recovery is still required for that handoff.

## Idempotency and explicit replay

An admission key is 1–128 printable non-whitespace ASCII characters. Persist only
its SHA-256 digest; never return/log its plaintext. Scope keys by workflow/operation,
and additionally by original execution for retry. Identical fingerprints return
the original ID/result; a changed revision/action binding or acknowledgement is a
conflict. Concurrent duplicate admissions create one execution atomically.

Keys expire seven days after first admission; reads do not extend expiry. Reuse
after expiry may create another execution. A duplicate caller waiting on active
work is bounded by the caller/original execution deadline and cannot take over or
restart it. Cancellation of a duplicate wait does not cancel the original owner.
Admission deduplication is not exactly-once external side effects.

Retry accepts failed, cancelled or timed-out originals and creates a new execution
ID linked to its parent, using the exact original immutable revision and action
versions. Every step starts again, including previously successful ones. It does
not patch original history or switch silently to the current revision. Archived
workflows and unavailable action versions are rejected. Succeeded executions are
not replayed by this command. Uncertain-effect metadata requires `--ack-uncertain`;
that acknowledgement explicitly permits possible repeated effects, not a guarantee
they are safe. Inspect source history before choosing it.

## Restart, schema and operations

The original serial coordinator remains a compatibility path. Production CLI
execution uses the [dispatcher](dispatcher.md), whose server can recover managed
running work after acquiring native ownership and advancing its durable generation.
Unmanaged active work requires explicit adoption after every old executor stops.
Unfinished invocations are uncertain and never automatically replayed. Receipt
lookup does not itself invoke work.

Migration 003 adds policy/attempt/admission storage and transactionally rebuilds
the event table without altering migrations 001/002 or old terminal snapshots.
Format 1 retains its 1,024-event cap; new format 2 permits 16,384 events, each at
most 1 KiB. Output bounds remain 64 KiB/step and 1 MiB/execution, with existing
64 MiB read limits. Attempts become immutable when terminal. Cancellation, state,
attempt and event writes roll back together on failure.

Stop processes and back up the database/configuration before upgrading, including
when upgrading only for this core. Older binaries reject a newer schema. Rollback
requires restoring the pre-upgrade backup with its compatible binary; there is no
in-place history down migration. SQLite remains single-node, without broker,
distributed lease or PostgreSQL claims. The dispatcher adds optional execution configuration and migration 004 without
changing the minimum Go version.

See [core implementation review](../harness/reviews/phase-03-core.md) and the
[build log](../harness/build-log.md) for actual verification. Source-level core
coverage/benchmarks do not qualify the later worker/scheduler/listener components.
