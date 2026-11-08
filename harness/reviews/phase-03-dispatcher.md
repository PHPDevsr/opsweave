# Phase 03 dispatcher self-review

Date: 2026-11-08 (Asia/Jakarta). Base: signed serial-core commit `b223699` in
[draft PR #5](https://github.com/PHPDevsr/opsweave/pull/5). The user accepted all
three [dispatcher decisions](../designs/single-node-dispatcher.md) with "ACCEPT".
This review is a self-review, not an independent reviewer or Phase 03 completion.

## Implementation and acceptance mapping

- Native single-owner lock uses the already pinned `x/sys v0.49.0`, promoted from
  indirect to direct without changing version or Go minimum. Canonical file
  handles, link counts and regular-file checks reject unsafe final paths/hard links.
  `TestNativeLockProcessExitAndAlias` uses a separate real process: acquisition,
  nonblocking busy, process termination and automatic kernel release. Unix permissions and Windows protected owner/SYSTEM DACLs are tested. A sidecar's
  existence alone never grants ownership, and unlock never removes it.
- Migration 004 adds queue/generation/audit metadata and replaces the active-workflow
  index transactionally; migrations 001–003 are unchanged. Existing SQL-independent
  execution policies carry reviewed managed parallelism; the legacy serial path
  still rejects unsupported capabilities. Secret references and unavailable
  production actions remain rejected.
- `TestDispatcherAdmissionClaimAndFencing`, `TestDispatcherConcurrentAdmissionAndExpiry`
  and `TestDispatcherRevisionAndGlobalCapacity` exercise two actual SQL handles,
  unique admission/claim winners, queue-full/dedup, independent expiry, private
  ownership generations, stale writes/releases, global capacity and restrictive
  revision limits. Guarded claim checks action-preflighted exact bindings, archive,
  cancellation and capacity in its transaction. Queue reads select bounded eligible
  metadata without materializing every queued workflow's outputs/steps.
- `TestDispatcherAdmissionAndClaimRollback` injects actual SQL failures at admission
  and running-event writes, verifying no partial execution/admission/history or
  claimed deadline. `TestMigrationFourPreservesHistoryAndRollsBackIndex` starts
  from a reconstructed version-3 schema and checks unchanged history plus unique
  index/ledger rollback on an injected schema conflict.
- One dispatcher event loop owns a fixed worker pool and bounded result channel.
  No goroutine is allocated per execution/step/backoff. Logical slots, workers,
  execution concurrency and global running capacity are separate enforced bounds.
  `TestDispatcherParallelDependenciesAndWorkerBound` observes actual overlapping
  roots, copied output resolution and dependent completion; the backoff test proves
  a worker is available to an independent step while retry waits.
- `TestDispatcherResolutionAndFailureHandler` verifies missing output fails before
  invocation, retry exhaustion and successful failure handlers still aggregate to
  failure. `TestDispatcherPersistenceBoundaries` injects pre-attempt failure,
  commit-then-error and concurrent cancellation without invoking a new action on
  ambiguity. Metadata conflict retries are bounded and never repeat an invocation.
- `TestDispatcherManagedBackoffRecoveryPreservesBudget`,
  `TestDispatcherUnmanagedAndUncertainRecovery` and domain recovery tests retain
  consumed attempts/deadlines/due times, fence old owners and mark unfinished or
  conservatively classified legacy work as uncertain. No uncertain handler is
  automatically replayed. Parent-linked replay still needs acknowledgement.
  Old unmanaged admissions are rejected after the ownership generation advances.
- `TestDispatcherServerCapacityAndShutdown`, the CLI server/queue test and the
  uncooperative-handler test verify stopping claims, durable cancellation, joining
  owned work and preserving unrelated unclaimed entries. A discovered interrupt/SQL
  cancellation race was corrected with independently bounded metadata contexts;
  shutdown timeout is reported while ownership remains held until handlers exit.
  Failure never masquerades as a clean drain.
- CLI tests cover queued receipts without invocation, busy-before-admit synchronous
  calls, cross-handle cancellation, replay, explicit unmanaged adoption and native
  lock release. Server opens no HTTP listener. Ready is emitted after ownership/
  recovery; availability of a future production action is not implied.
- `TestExecutionConfiguration` checks defaults, file/environment precedence,
  strict types, all capacity/duration bounds and unknown-setting rejection.
  Source-level context budgets, bounded outputs/events and safe diagnostics are
  reviewed. No caller input becomes an unrestricted metric label or raw action log.
- New `DispatcherClaim` and `DispatcherThroughput/Workers1,Workers4` benchmarks
  measure durable claims and the eight-independent-step no-op dispatch path. CI
  requires fifteen families. They do not prove external-action scaling or an SLA.

## Compatibility, security and remaining gates

See [dispatcher operations](../../docs/dispatcher.md) for exact flags, configuration,
lock/queue semantics, supervised shutdown, stopped-process migration and rollback.
There is no distributed SQLite lease, forced goroutine termination, automatic
uncertain-effect replay, shipped fixture action, network management endpoint or
live release. Trusted local storage and stopped old executors are required for
ownership migration; fencing database writes cannot fence external side effects.

Actual executed checks and any limitations are recorded in the [build log](../build-log.md).
CodeQL upload still requires repository Advanced Security configuration. No row is
completed while applicable gates remain open. Scheduler/cron, other triggers,
authenticated listener and general retention remain incomplete Phase 03 work.
The next [cron scheduler contract](../designs/cron-scheduler.md) requires review
before implementing its syntax, calendar policy and missed-run admission semantics.
