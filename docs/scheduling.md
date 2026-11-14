# Scheduled workflows

Use an explicit cron trigger under `spec.triggers`. A workflow can declare up to
16 triggers with unique IDs. The example below is a workflow fragment. Choose
actions available in the catalog used to validate and deploy the workflow.

```yaml
spec:
  triggers:
    - id: daily-maintenance
      type: cron
      schedule: '30 2 * * *'
      timezone: Asia/Jakarta
      missed_run: skip
```

The five fields are minute (0–59), hour (0–23), day of month (1–31), month
(1–12), and day of week (0–6, Sunday is 0). Fields support numeric lists, inclusive
ranges, `*`, and positive steps up to the field's width. A value followed by a
step runs from that value to the field maximum. When both day fields are restricted,
either can match. A field containing `*` follows wildcard day matching, including
stepped wildcards. The parser rejects names, macros, seconds, and inline timezone
prefixes. `timezone` is an explicit IANA name or `UTC`, with Go timezone data
embedded as a fallback on every platform.

Evaluation matches local calendar fields against UTC minutes. The spring DST gap
has no matching instant. A repeated fall DST minute has two distinct UTC instants
and each can produce an execution. The missed-run policy applies when the server
misses either instant.

| Policy           | Behavior                                                                                          |
| ---------------- | ------------------------------------------------------------------------------------------------- |
| `skip` (default) | Admit a matching current minute after the stored cursor. Skip earlier unprocessed minutes.        |
| `catch_up_once`  | Admit the newest eligible minute within the last 24 hours. Coalesce older matches in that window. |

## Running and inspecting schedules

```sh
opsweave apply workflow.yaml
opsweave server
opsweave workflow schedules --json workflow-name
opsweave workflow schedule-logs --limit 100 --json workflow-name
opsweave execution get --json execution-id
```

The same server owns the scheduler and the bounded worker pool. It polls once per
second and processes batches of up to 64 cursors with a one-second scan budget
between entries. Additional due cursors are processed on subsequent ticks. A
standalone `run` only processes its manual admission. The development binary's
production action catalog is empty, so deployments require an available action
implementation. See [dispatcher operations](dispatcher.md).

The cursor starts at the stored deployment or re-enable timestamp. `workflow
disable` pauses all schedules of the workflow. Re-enable resets their cursors and
clears prior pending intents. Previously queued automatic jobs from an invalidated
enablement period settle as `workflow_disabled`. Manual jobs can still be claimed
while the workflow is disabled. Publishing a revision replaces future schedules.
Previously admitted executions retain their pinned revision and action versions.

Queue-full and unavailable-action failures retain one pending UTC occurrence per
schedule. The server retries the same identity after one minute for up to the
24-hour horizon. History records the failure when the reason or pending identity
changes. Expired pending intents and coalesced or skipped matches have separate
history events. Infrastructure errors stop the dispatcher and preserve any
uncommitted cursor for recovery.

Each occurrence is uniquely identified by workflow, immutable revision, trigger
ID, and UTC minute. Cursor, history, identity, and queued admission commit in one
transaction. Restart and backward clock movement preserve the cursor and cannot
re-admit the same occurrence. This guarantee applies to queue admission. External
effects still follow the dispatcher recovery rules.

`workflow schedules` lists current definitions, enablement, cursor, and pending
state. `workflow schedule-logs` returns the newest events across revisions, with
the associated execution ID for admissions. JSON includes `hasMore` when history
exceeds the requested limit. The available limit is 1–1000. Scheduled executions
include trigger ID and UTC occurrence. Replay creates an explicitly requested
manual execution using the original revision.

## Upgrade and shutdown

Migration 005 adds current scheduler state, immutable schedule history, and
automatic admission identities. Stop executors and back up the database and
configuration before upgrading. Restore the backup with its compatible binary
if rollback is necessary. See [dispatcher migration instructions](dispatcher.md#upgrade-backup-and-troubleshooting).

An interrupt cancels the scan and then lets the dispatcher settle its active
invocations. Unclaimed jobs remain in the queue. For a missing occurrence, inspect
enablement, the UTC cursor, the configured timezone, and schedule history first.
