# Phase 03 scheduler self-review

2026-11-14 (Asia/Jakarta), based on `1a0745d`. The user approved the three cron
decisions and asked to continue. The [scheduler guide](../../docs/scheduling.md)
documents the workflow syntax, CLI, and operating behavior.

- `TestCronNumericFieldsAndDaySemantics`, `TestCronDSTAndLeapDayUTCIdentities`,
  and `TestCronCursorPendingAndHorizon` cover day matching, explicit zones, DST
  gaps and folds, leap days, bounded catch-up, and backward-clock behavior.
  `FuzzCron` exercises malformed expressions and timezone validation with a bounded
  loader. The supported zone syntax rejects path traversal and `Local`.
- `TestCronTriggerValidationAndCapabilityGates` covers required fields, duplicate
  IDs, input limits, legacy capability checks, and rejection of unsupported types.
  Trigger-free workflow normalization retains its existing JSON representation.
- `TestScheduledAdmissionIdentityRestartAndHistory` and
  `TestScheduleConcurrentScansHaveOneWinner` verify the occurrence identity,
  restart deduplication, and immutable history through separate SQLite handles.
  `TestScheduledAdmissionRollbackAndRevisionRace` injects a history write failure
  after admission and verifies rollback of the cursor, job and occurrence identity.
- `TestScheduleQueueFullRetainsExactPendingIntent` verifies durable backpressure.
  `TestSchedulerDefersUnavailableActionsAndReturnsInfrastructureErrors` covers
  pending expiry, safe diagnostics and propagation of persistence failures.
  A stale cursor is a separate error from a lost dispatcher fence.
- `TestScheduleMultipleTriggersAndLifecycleChangeDuringPreflight` covers multiple
  trigger IDs and enablement changes during exact-revision preflight.
  `TestScheduleLifecycleFencesAutomaticButAllowsManualClaims` exercises old
  automatic admissions after re-enable and manual claims while disabled.
- `TestSchedulerDispatcherCompletesAndInterrupts` uses actual scheduler, SQLite,
  queue, and worker integration. It checks successful execution and cancellation
  during an active scan and invocation. The dispatcher owns polling and stops
  the scan when its context is interrupted.
- `TestMigrationFiveRollbackPreservesOldHistory` checks migration rollback and
  byte-equivalent pre-existing execution history. `TestScheduleCLIInspection`
  exercises the new commands and bounded history requests.
- `BenchmarkCronScan` exercises the full bounded calendar window.
  `BenchmarkScheduleDueScan` records SQLite read, calendar and durable cursor
  overhead. The workflow verifies both benchmark families.

Implementation choices include a one-second polling interval, batches of up to
64 cursors, a 64 MiB revision batch budget, and a one-minute delay for pending
retries. Workflow enablement controls all its cron triggers. Schedule history
shows a bounded recent window through the CLI. See the [build log](../build-log.md)
for executed checks and the PR for exact-head CI evidence.
