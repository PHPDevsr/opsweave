# Phase 03 event trigger review

The user accepted the event trigger contract on 2026-11-14 (Asia/Jakarta).

- Workflow validation distinguishes cron, startup, internal and webhook fields.
  Registration replaces current event routes in the same transaction as the
  revision change. Cron normalization keeps its existing fields and byte order.
- Internal routing and webhook admission use current enabled routes under the
  queue transaction. The queue savepoint rolls back every candidate on capacity
  or identity conflict. Safe failure history can commit independently of the
  rejected batch. Receipts follow the transaction commit.
- Startup progression and admission share the fenced write transaction. A
  generation keeps one durable scan cursor and deferred position. Queued event
  origins carry the same enablement generation used by cron claims.
- Webhook verification checks the literal method, route, content type, bounded
  body, timestamp and constant-time HMAC before admission. Tests exercise
  authentication, deduplication, payload bounds, and concurrent admission limits.
- CLI tests verify internal receipts and history, startup execution through the
  normal server command, and joined handler and worker shutdown. SQLite tests
  cover fan-out rollback, replay across revisions, expiry, disabled workflow
  claims, stale fences, startup retry, and destination bounds.
- The webhook package includes a parser fuzz target and an authentication
  benchmark. The benchmark workflow asserts the new family.

Operational decisions include a CLI opt-in port, a 250 ms startup scan interval,
and bounded history inspection. See the [event trigger guide](../../docs/event-triggers.md)
and [build log](../build-log.md) for behavior and verification.
