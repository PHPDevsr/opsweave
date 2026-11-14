# SQLite retention and local readiness

## Execution retention

OpsWeave keeps execution history by default. Set a positive Go duration in `opsweave.yaml` to make history eligible for manual pruning. The `OPSWEAVE_EXECUTION_RETENTION_AGE` environment variable overrides this setting.

```yaml
execution:
  retention_age: 720h
```

```sh
opsweave execution prune --dry-run --json
opsweave execution prune --apply --json
```

Both modes require configured retention. Preview opens the current database read-only. Apply removes at most **100** executions in a transaction, so repeat it to clear a backlog. Use a verified backup before applying. Eligibility uses the terminal execution's last state timestamp and a strict older-than comparison. Counts describe the snapshot before apply, and `deleted` reflects the committed batch. `active`, `withinRetention`, `protectedByReceipt` and `referencedByReplay` each count an execution once, using that order when conditions overlap.

Prune removes the execution, step and attempt history, transition logs, and history rows linked to that execution. Pending, queued and running executions and immutable workflow revisions are retained. An unexpired manual or event deduplication receipt protects its execution. A replay protects its parent while the replay record exists. Parents whose final reference was pruned in one batch become eligible on a later invocation. Scheduled runs leave only an occurrence key so a rewound cursor cannot repeat an already admitted occurrence. Unlinked trigger or scheduler diagnostics remain available.

The command has a 30-second operation budget. SQLite serializes prune with concurrent admissions. On a lock or deadline failure, retry the command. Prune makes deleted space available to SQLite for reuse. File size can remain unchanged, and existing backup copies retain their original history.

## Server status

```sh
opsweave server status
opsweave server status --json
```

`ready` exits `0` when the server holds its native dispatcher and server locks, recovery finished, the configured webhook listener has bound if enabled, and the SQLite migration ledger matches the binary. `stopping` exits `1` while the server is shutting down and still holds its locks. `unavailable` exits `1` for an absent server, startup in progress, stale ownership, or a failed storage or schema check. JSON output is an object with one `state` field. The local status command runs a read-only check with a two-second budget and rechecks lock ownership before returning.

Use the same configuration and OS account as the server. A synchronous `opsweave run` is a standalone dispatcher and cannot satisfy server readiness. A probe is a point-in-time observation. It checks the local server and database without running actions or testing external services. HTTP continues to expose signed loopback webhook ingestion only.

## Upgrading and restoring

Stop the dispatcher and all older OpsWeave processes. Back up the SQLite database and configuration before upgrading. Migration 007 adds dispatcher lifecycle state, prune support, and occurrence identities for future pruning. Existing execution history remains readable after migration. Preview and status require a current schema. Run `server` or another regular database-backed command to apply migrations before using either check.

Restore the pre-upgrade backup with its matching binary to roll back. To recover pruned history, use an isolated copy of the backup taken before pruning, or restore it with the dispatcher stopped. Full restoration also returns queued work and deduplication state to the backup point, so inspect that state before restarting a dispatcher.
