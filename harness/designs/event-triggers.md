# Phase 03 event trigger and webhook decisions

Status `ACCEPTED_FOR_IMPLEMENTATION` on 2026-11-14 (Asia/Jakarta).

## Workflow and admission

- A trigger with `type: startup` has an `id`. An enabled workflow receives one
  startup occurrence per successful server ownership generation. Startup scanning
  begins after recovery. Repeated scans within a generation use the same identity.
- A trigger with `type: internal` has an `id` and `event` name. The local command
  `opsweave event emit --id EVENT_ID EVENT_NAME` uses the database directory's
  access boundary and returns queued receipts. The name and ID are bounded ASCII
  values. Events route to currently enabled matching triggers. A batch admission
  is atomic and subject to the existing queue limit. Limit each emission to 64
  destinations and return an explicit capacity error if that bound is exceeded.
- A trigger with `type: webhook` has an `id`. Its only network route is
  `POST /v1/hooks/{workflow}/{trigger}`. Webhooks are opt-in and bind only to
  IPv4 loopback. An operator can expose the route through a TLS reverse proxy.
  Require an explicit signing secret from environment configuration, at least
  32 bytes. Apply a 16-request concurrent handler limit, 8 KiB headers, 64 KiB
  body limit, and five-second read/write timeouts. Accept a JSON object with
  `Content-Type: application/json` and reject content encodings.

## Authentication and recovery

The webhook signature is HMAC-SHA256 over the timestamp, event ID, request path,
and raw body, separated by newlines. Require `X-OpsWeave-Timestamp` (Unix seconds),
`X-OpsWeave-Event-ID` (1–128 printable ASCII characters excluding spaces), and
`X-OpsWeave-Signature` (64 hexadecimal characters). Use constant-time comparison
and a five-minute clock tolerance. Authenticate before looking up the workflow.
Return HTTP 202 only after durable admission and use consistent safe errors for
authentication, invalid input, disabled routing, and queue capacity.

Internal and webhook identities consist of source type, destination workflow and
trigger ID, and event ID. Store hashed IDs and body fingerprints for seven days.
Repeating a matching event returns the original receipt across revision changes.
Changed content under an existing event ID returns a conflict. Startup identities
use the ownership generation. Queue-full retains a bounded startup intent for
that generation and rejects external admission with a retryable capacity result.
The existing worker pool and automatic claim checks apply to all three types.

Use immutable event history with safe reason codes and execution links. Webhook
bodies are validated and fingerprinted. Persist the fingerprint and bounded
metadata. Workflow inputs continue to use the existing definition and reference
rules. Tests must cover authentication, replay, limits, lifecycle races, rollback,
and shutdown of the owned listener. Broader management API credentials and
retention policy remain separate roadmap items.
