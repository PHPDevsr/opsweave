# Single-node dispatcher in development

The accepted [dispatcher contract](../harness/designs/single-node-dispatcher.md)
extends the [serial reliability core](reliability.md) with a bounded action pool,
durable queue, workflow concurrency, parallel DAG steps and conservative recovery.
The production action catalog is still empty; handlers used in tests are not
installable actions. The server also evaluates persisted cron triggers. See [scheduled workflows](scheduling.md).

## Commands and configuration

```sh
opsweave run --enqueue --json workflow-name
opsweave server
opsweave execution retry --enqueue --idempotency-key replay-20261108 execution-id
opsweave execution cancel execution-id
opsweave execution cancel --step step-id execution-id
```

Enqueue returns a durable `queued` receipt, not successful execution. It works
without a running server. An ordinary synchronous `run` or `execution retry`
requires the database's dispatcher lock and processes only its own admission.
When a server owns that lock, synchronous commands fail busy before admitting
work; explicitly enqueue instead. A standalone invocation refuses unrelated
running work and requires a server to perform recovery. It does not drain an old
unrelated queue. Caller cancellation also cancels its own unclaimed admission.

`server` drains eligible queue work and prints `dispatcher ready` after ownership
and recovery succeed. The same dispatcher owns the cron scheduler and its queued work. Stop it with the process interrupt context.

Optional strict configuration:

```yaml
execution:
  workers: 4
  active_steps: 16
  running_executions: 8
  queued_executions: 1024
  queue_timeout: 1h
  shutdown_timeout: 30s
```

| Setting              | Range    | Environment override                    |
| -------------------- | -------- | --------------------------------------- |
| `workers`            | 1–64     | `OPSWEAVE_EXECUTION_WORKERS`            |
| `active_steps`       | 1–256    | `OPSWEAVE_EXECUTION_ACTIVE_STEPS`       |
| `running_executions` | 1–256    | `OPSWEAVE_EXECUTION_RUNNING_EXECUTIONS` |
| `queued_executions`  | 1–10,000 | `OPSWEAVE_EXECUTION_QUEUED_EXECUTIONS`  |
| `queue_timeout`      | 1s–24h   | `OPSWEAVE_EXECUTION_QUEUE_TIMEOUT`      |
| `shutdown_timeout`   | 1s–5m    | `OPSWEAVE_EXECUTION_SHUTDOWN_TIMEOUT`   |

Environment overrides file settings, which override defaults. Unknown fields,
unknown OpsWeave environment variables, incorrect types and out-of-range values
are rejected. Existing storage/logging configuration remains valid. These bounds
do not impose OS CPU or memory isolation on an arbitrary compiled action.

## Capacity, deadlines and ordering

Admission pins immutable revision/action bindings and commits pending/queued
history atomically with hashed seven-day deduplication metadata. Queue-full leaves
no partial execution or admission record. Queue expiry is independent of the
running timeout. It settles as cancelled with `queue_expired` when processed;
without a server, expiry does not itself launch a background process.

Eligible claims are ordered by available time, creation time and opaque ID.
SQL checks current ownership, cancellation, expiry, archive status, global running
capacity and workflow limits again in the claim transaction. Action preflight
uses the exact pinned revision. For differing workflow revisions, admission to
running respects the minimum concurrency limit of all active revisions and the
candidate. Editing a limit does not kill already running work.

Execution deadlines start at claim, capped by a synchronous caller's deadline
where present. Step deadlines include resolution, time waiting for a worker and
retry backoff. Logical active steps respect both `spec.parallelism` and
`active_steps`; actual invocations respect `workers`. Backoff retains a logical
slot but releases the worker. Independent ready steps can overlap. Dependencies,
conditions and aggregate failure semantics remain those of the core contract.

The dispatcher polls bounded metadata and uses a fixed pool, bounded result
channel and bounded running set. It allocates no goroutine per queued execution,
retry or logical step. SQL transactions never span action I/O. Metadata operations
have their own five-second budget so an interrupt can complete durable cleanup.
Infrastructure or ambiguous-commit failures stop further invocation and remain
visible for conservative recovery.

## Ownership and recovery

Exactly one native advisory lock protects a canonical local database path:
`Flock` on Linux/macOS, `LockFileEx` on Windows. Kernel lock ownership, rather than
sidecar existence or PID checks, is authoritative. A process exit releases the
lock. The private `.dispatcher.lock` sidecar is never removed during unlock. Its permissions are restricted to mode
0600 on Unix and a protected current-user/SYSTEM DACL on Windows; failure to
apply permissions prevents acquisition.
Hard-linked databases/sidecars and non-regular final paths are rejected; resolved
directory aliases share the same canonical lock name. Use a trusted local directory
and stop executors before moving or replacing database files. Network filesystems,
shared multi-host SQLite and hostile filesystem replacement are unsupported.

Each owner advances a durable generation. Claims and managed state/attempt writes
check generation, private owner and execution version transactionally. Old owners
cannot write after takeover. This fences database updates, not external effects.

On server startup, managed running history is classified before invocation:

- Successful terminal history stays immutable.
- Pending work or committed retry backoff may resume after pinned-catalog validation,
  retaining consumed attempts, outputs, absolute deadlines and next retry time.
- An unfinished invocation fails with `uncertain_effect`; remaining work aborts,
  and no handler is automatically replayed. `uncertainEffects` is visible in the
  execution receipt/history, including conservative legacy classifications.
- Expired safe work terminates without starting another attempt.

Old pre-dispatcher active rows have no equivalent ownership proof. Default server
startup refuses them. Stop **every old executor**, back up the database, then use
`opsweave server --adopt-unmanaged` deliberately. The decision is audited with the
new generation. It acknowledges ownership migration only. Replaying an uncertain
terminal execution separately requires `execution retry --ack-uncertain` and may
repeat external effects. New unmanaged admissions are rejected after ownership
migration. Read-only history and explicit cancellation metadata remain available
through separate local handles.

Shutdown stops claims, requests cancellation of running work, cancels invocation
contexts and joins every worker. Unclaimed server queue entries remain queued.
An uncooperative compiled handler cannot be forcibly stopped as a Go goroutine:
report shutdown timeout and retain ownership until it actually exits; supervisor
process termination may be required. A timeout or persistence failure is never a
clean drain or proof that an external effect did not happen.

## Upgrade, backup and troubleshooting

Migration 004 replaces the legacy active-workflow index and adds queue, generation
and ownership-audit metadata atomically. Migrations 001–003 remain unchanged and
old terminal histories remain readable. Stop all executors before upgrading or
moving files; back up the database and configuration first. Older binaries reject
the newer schema. Roll back by restoring that backup with its compatible binary;
there is no in-place down migration. Retain the sidecar while its process is live.

Busy means another owner exists or a standalone run found old active work. Do not
delete the lock file, alter rows, or start a second independent worker pool to
bypass it. Use enqueue, or stop the owner and run the server's recovery procedure.
Queue-full requires draining/cancelling existing work or explicitly increasing the
configured bound. Unavailable pinned actions are rejected without substitution.
See the [dispatcher self-review](../harness/reviews/phase-03-dispatcher.md) and
[build log](../harness/build-log.md) for measured checks and open gates.
