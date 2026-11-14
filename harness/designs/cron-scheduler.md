# Phase 03 persisted cron scheduler proposal

Status: `ACCEPTED_FOR_IMPLEMENTATION`. The user accepted all three decisions with "ACCEPT" and confirmed continuation on 2026-11-14 (Asia/Jakarta). The user has approved
Phase 03, the reliability core and the dispatcher. This next contract concerns
schedule syntax and missed-run semantics; it does not re-request those approvals.

## Three decisions for review

1. Expose cron schedules as an explicit typed workflow trigger list, with stable
   authored IDs. Support numeric five-field minute/hour/day-of-month/month/weekday
   cron, lists, inclusive ranges, `*` and positive steps. Weekday 0 is Sunday.
   Reject seconds, names, aliases, macros and inline `TZ=` prefixes. If both day
   fields are restricted, match either field, with all other fields still matching.
   Use a bounded standard-library parser, not arbitrary expressions or shell cron.
2. Require an explicit IANA timezone per schedule, allowing `UTC`; ship Go's
   embedded timezone data so platform installation does not change availability.
   Iterate UTC instants and test local calendar fields: nonexistent spring-forward
   times do not run; repeated fall-back wall-clock times produce two distinct
   scheduled instants. Unique admission identity includes the UTC scheduled instant.
3. Persist schedule cursor and automatic admission atomically. Default missed-run
   policy is `skip`; optional `catch_up_once` coalesces overdue occurrences into
   one admission of the newest eligible missed instant within a fixed 24-hour
   horizon. Never enqueue an unbounded backlog. Queue-full retains pending due
   intent for retry within that horizon; disabled/archived workflows do not admit
   automatic work. A disabled workflow also cannot claim previously queued
   automatic work. Manual admissions retain their existing behavior.

## Concrete syntax and identity

```yaml
spec:
  triggers:
    - id: daily-maintenance
      type: cron
      schedule: '30 2 * * *'
      timezone: Asia/Jakarta
      missed_run: skip
```

Allow 1–16 triggers per workflow, schedule strings at most 256 bytes, timezone
names at most 128 bytes and parser work bounded by the 24-hour catch-up horizon.
Missing `missed_run` means `skip`; other shown fields are required. Reject duplicate
IDs, unsupported types and unknown fields. Trigger types for startup, internal
events and authenticated webhooks remain gated until separately reviewed.

New/current workflow revisions replace future schedule definitions without
rewriting admitted executions. Identity includes workflow name, immutable revision,
trigger ID and UTC instant. First deployment begins from its committed deployment
time; it does not backfill earlier history. Workflow disable, archive and revision
changes are reconciled transactionally with cursor/admission decisions. Re-enable
starts from its committed re-enable time, without replaying disabled-period work.

The existing single fenced dispatcher owns the scheduler and its fixed polling
loop. There is no scheduler goroutine per workflow and no second independent
pool. Use injected clock/zone loading for tests, bounded due batches and bounded
SQL transactions. Queue origin metadata distinguishes automatic admissions so
claim can atomically re-check enablement. Immutable execution/action bindings and
existing worker/queue limits apply; no alternate invocation path is permitted.

Persist safe schedule/admission/failure events without logging payloads or secrets.
Capacity/storage failures must be reported, not treated as a delivered run. New
migrations preserve existing histories and use the stopped-process backup/rollback
procedure. Test calendar validation, DST gaps/repeats, wall-clock movement, exact
deduplication, restart cursors, queue-full rollback, enable/revision races and
scheduler/worker shutdown; benchmark due-scan overhead before optimization.

This proposal adds no network listener, webhook authentication model, public
management API, arbitrary script, production action or general retention default.
No later phase or live release is authorized by it.
