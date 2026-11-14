# Event triggers

Workflows can declare startup, internal, and webhook triggers alongside cron
triggers. The workflow trigger limit is 16. Trigger IDs are unique within the
workflow. A workflow still needs steps backed by the server's action catalog.

```yaml
spec:
  triggers:
    - id: ready
      type: startup
    - id: rebuild
      type: internal
      event: source-changed
    - id: delivery
      type: webhook
```

Use lowercase ASCII names of up to 63 characters. Names start with a letter and
may contain digits and single hyphens. Use `workflow disable` to stop automatic
admission for the workflow. Queued automatic jobs from prior enablement periods
settle as cancelled. Manual admission remains available for a disabled workflow.

## Startup and internal events

After dispatcher recovery, `opsweave server` evaluates the startup routes for
its new ownership generation. The current route set is bounded by a persisted
scan position. When the queue fills, the next eligible startup route retains its
position until capacity becomes available. A later server generation starts a
new scan. Workflow changes during the scan can invalidate an older route.

```sh
opsweave event emit --id source-change-123 --json source-changed
opsweave event history --limit 100 --json workflow-name
```

Internal emission uses access to the local database directory. It can queue
events before the server starts. An emission addresses up to 64 enabled matching
destinations. All new admissions in a batch commit together. An event ID is
1–128 printable ASCII characters without spaces. Reuse the same ID for retries.

## Signed webhooks

Set `OPSWEAVE_WEBHOOK_SECRET` in the server environment to a dedicated signing
secret of 32–1024 bytes. Start ingress with `opsweave server --webhook-port 8080`.
The server binds `127.0.0.1:8080`. For remote delivery, terminate TLS at a reverse
proxy and preserve the exact request path, signed headers and request body.

Send a UTF-8 JSON object to `POST /v1/hooks/{workflow}/{trigger}` using
`Content-Type: application/json`. The path is lowercase literal ASCII with no
query or percent encoding. The body limit is 64 KiB. Content encoding is
rejected. Ingress enforces an 8 KiB request-line and header budget and five-second read,
write and idle timeouts. At most 16 requests process ingress concurrently.

| Header                 | Value                                                            |
| ---------------------- | ---------------------------------------------------------------- |
| `X-OpsWeave-Timestamp` | Unix seconds in decimal, within five minutes of the server clock |
| `X-OpsWeave-Event-ID`  | Stable event ID using the same rules as internal events          |
| `X-OpsWeave-Signature` | 64 hexadecimal characters representing HMAC-SHA256               |

Compute the HMAC over these bytes in order: the timestamp header, LF, the event
ID header, LF, the literal request path, LF, and the raw body. The verifier
checks the signature before workflow lookup. A client can sign a retry with a
fresh timestamp while keeping its original body and event ID.

| HTTP status           | Result                                                                                   |
| --------------------- | ---------------------------------------------------------------------------------------- |
| 202                   | Durable admission or an existing receipt                                                 |
| 400 / 413 / 415 / 431 | Invalid JSON object, oversized body, unsupported media or encoding, or oversized headers |
| 401                   | Invalid or expired signature headers                                                     |
| 404                   | Unavailable path or enabled route                                                        |
| 409                   | Existing event ID with a different body fingerprint                                      |
| 503                   | Busy ingress, queue capacity, or an unavailable admission transaction                    |

The response body for admission is a JSON array of receipts with `execution`,
`workflow`, and `trigger`. The database keeps a hash of the event ID and the
body fingerprint for seven days. An enabled route with the same workflow, ID,
and source type can replay its existing receipt after a revision change. JSON
bodies are used for authentication and deduplication. Workflow inputs come from
the pinned definition.

## History and operations

`event history` reads the latest admission outcomes for one workflow. Use an
execution link from an admitted event to inspect its run and action result.
Capacity and conflict failures have safe reason codes. Payload and signing
secret values stay outside the history. Cron outcomes use `workflow schedule-logs`.

Migration 006 adds event routes, origins, deduplication and history. Stop the
dispatcher and back up the SQLite database and configuration before upgrading.
Rollback uses that backup with its matching binary. During shutdown the server
cancels ingress requests, closes the listener and joins handlers and workers
before releasing dispatcher ownership.
