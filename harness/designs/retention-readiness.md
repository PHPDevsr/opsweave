# Phase 03 retention and readiness decision

Status `ACCEPTED` by the user's `ACCEPT` reply on 2026-11-14 (Asia/Jakarta).

## Retention

Keep history unless an operator configures retention. Add an optional positive
age threshold and a local `execution prune --dry-run` command. A separate apply
operation deletes terminal executions older than the threshold in batches of 100. Preserve active executions, pinned revisions, unexpired deduplication
receipts, and any receipt still referenced by a live replay. Return counts by
eligibility reason. A short transaction removes each complete execution record
and its dependent history together. Maintain occurrence identity while its
deduplication interval can still affect admission.

## Readiness

Add local `server status` output based on dispatcher ownership and a bounded
database check. Describe ready, stopping and unavailable states. A successful
status requires compatible schema and a currently held dispatcher lock. Keep
the webhook surface restricted to its signed ingress route. Tests cover stale
ownership, database failures, cancellation and concurrent events during cleanup.

See the [maintenance guide](../../docs/maintenance.md) for the implemented CLI, deletion boundaries, and readiness exit codes.
