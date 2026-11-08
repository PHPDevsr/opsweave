# Phase 03 bounded single-node dispatcher proposal

Status: `AWAITING_CONTRACT_REVIEW`, 2026-11-08 (Asia/Jakarta). Phase 03 and the
[reliable execution contract](reliable-execution.md) are approved. This is the
next contract required to activate parallel capacity and restart recovery; it
does not re-request phase authorization or finalize a listener/trigger API.

## Three decisions for review

1. Use a process-owned native OS advisory lock for exactly one dispatcher per
   canonical SQLite database path, plus a durable ownership generation fencing
   every claim/state write. A lock file's existence is not ownership. Implement
   the platform adapter with the already pinned `golang.org/x/sys v0.49.0`
   (`Flock` on Linux/macOS, `LockFileEx` on Windows); promote that existing indirect
   dependency to direct only when implemented. No new version, broker, service or
   distributed SQLite guarantee is introduced.
2. Add a bounded durable queue and fixed action-worker pool with the defaults
   below. Ordinary `run` still waits for a final result; `run --enqueue` and
   `execution retry --enqueue` explicitly return durable queued admission instead.
   A local synchronous run requires dispatcher ownership; when a server already
   owns the database, it fails busy before admitting work, and users may explicitly
   enqueue to that server. This avoids pretending an asynchronous receipt means
   execution success or launching a second independent worker pool.
3. Recover only while holding the OS lock and a fresh fenced generation. Managed
   in-flight uncertainty follows the accepted fail-without-replay policy. Running
   executions from the pre-dispatcher versions have no equivalent ownership proof:
   refuse automatic adoption, and require `server --adopt-unmanaged` after the
   operator stops old executors. This acknowledges ownership migration only; it
   never authorizes repeating uncertain actions. Explicit replay still requires
   the separate `--ack-uncertain` decision.

## Capacity and queue semantics

Proposed optional strict configuration, preserving existing storage/logging files:

```yaml
execution:
  workers: 4
  active_steps: 16
  running_executions: 8
  queued_executions: 1024
  queue_timeout: 1h
  shutdown_timeout: 30s
```

Ranges: workers 1–64; active_steps/running_executions 1–256; queued_executions
1–10,000; queue_timeout 1 second–24 hours; shutdown_timeout 1 second–5 minutes.
Reject unknown/invalid settings, do not clamp. Document fixed named environment
overrides with the same bounds; no wildcard acceptance of unknown OPSWEAVE keys.

The dispatcher owns at most running_executions coordinators, workers action
goroutines and active_steps logical active steps. A step in retry backoff retains
its logical parallelism slot but releases the action-worker slot. Use bounded
channels, bounded ready sets and cancellable timers, not a goroutine per queued
job or retry. Observe both authored per-execution parallelism and global capacity;
the workflow definition itself is never rewritten to fit available slots.

Admission persists pending/queued transitions and immutable revision/action
bindings before acknowledgement. No running deadline is assigned until claim.
Queued work has an independent queue-expiry time that survives restart; expiry
produces cancelled with queue_expired, not an execution timeout before running.
Queue-full is an explicit admission error, with no execution or deduplication
record partially committed. Duplicate admissions retain the accepted seven-day
idempotency contract and do not consume capacity twice.

Claim due work transactionally, ordered by available time, creation time and ID.
Check cancellation, archive/automatic-trigger enablement, queue expiry, action
availability, workflow concurrency and global capacity in the same claim boundary.
Do not silently substitute a revision/version. For revisions with different
workflow limits, use the minimum limit among active revisions and the candidate
before adding another execution. Existing running work is not killed by editing
a limit; new starts wait until capacity permits. Every running/attempt transition
compares the active dispatcher generation, private execution owner and version.

`run --enqueue` may queue while no server is running; its result says queued and
expires under queue_timeout if nobody claims it. Ordinary standalone run claims
only its own newly admitted execution, starts no automatic trigger/scheduler, and
joins owned work before exit. It does not unexpectedly drain unrelated old queue
entries. A queue-worker server drains eligible admitted work; listener and trigger
exposure remain separate reviewed units. There is no management HTTP shortcut.

## Ownership and recovery

Resolve the canonical local database path before locking. Reject unsupported
alias/hard-link arrangements rather than allow two sidecars for one database.
Keep the sidecar private and never unlink a held lock. Use non-blocking native
lock acquisition and a safe busy result, not indefinite lock waiting or a stale
PID/file-existence heuristic. Lock lifetime belongs to the dispatcher process.

After lock acquisition, transactionally advance the stored generation and fence
obsolete owners before recovery/claims. Preserve original execution/step deadlines,
consumed attempts and next retry times. Reuse successful outputs only; never
reopen terminal history. A committed backoff/pending action can resume after
validation; an unfinished invocation is uncertain even if it might have succeeded.
Record uncertainty and fail/abort that execution without invoking further steps.
Fencing protects durable writes, not exactly-once external side effects.

Unmanaged running rows block startup/adoption by default. `--adopt-unmanaged` must
be explicitly supplied after old executors stop, is durably audited, and applies
the same conservative uncertainty classification; it does not invent attempt
history for Phase 02 actions. Stop processes before database migration. Read-only
history and explicit cancellation/retry metadata operations remain available
through separate database handles; they do not acquire dispatcher ownership or
start actions themselves.

Shutdown stops admission/claims first, cancels scheduler/worker contexts as
applicable and joins every owned goroutine. Report timeout/cleanup failures;
never release ownership and claim a clean drain while a handler still runs.
Uncooperative handlers violate the action contract and may require supervisor
process termination; no detached Go goroutine simulates forced cancellation.

## Persistence, tests and boundaries

Use a new checksummed migration for queue/dispatcher ownership/capacity metadata;
never edit migrations 001–003. Upgrade and rollback require the documented
stopped-process backup procedure. Existing histories remain readable. SQLite
claim transactions retain current durability/busy-timeout settings and never
hold SQL locks across action I/O. PostgreSQL leases/claims stay Phase 08 work.

Test with actual separate processes/handles: one OS-lock winner, non-blocking busy,
release after process exit, rejected aliases, fenced stale writers, queue-full
rollback, unique concurrent admission/claim, per-workflow/global limits, queued
cancellation/expiry, managed versus unmanaged restart and uncertain effects.
Inject clocks for backoff/expiry/shutdown; use explicit synchronization, race tests
and bounded goroutine counts. Benchmark claim overhead and worker throughput in
existing CI before optimization. Keep native and ARM cross-build limitations honest.

This proposal authorizes no HTTP listener shape, webhook authentication scheme,
trigger YAML variants, cron/DST/missed-run policy or general retention default.
Those next Phase 03 contracts must be concrete and reviewed before exposure.
Production actions still arrive in Phase 04; no fixture handler ships to make a
worker demo look usable. No later phase or live release is authorized here.
