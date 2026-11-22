# Phase 04 notification action proposal

Status `ACCEPTED` by user reply `ACCEPT` on 2026-11-22 (Asia/Jakarta), answering
the concrete four-channel fixed-profile/environment-credential/verified-network
contract question. This authorizes only these workflow adapters within Phase 04.

## Scope, profiles and credentials

Deliver `notification.telegram`, `notification.slack`, `notification.email` and
`notification.webhook`, exact action version `1` for `opsweave.io/v1`, as workflow
actions behind the existing engine/runtime. Default `actions.notification.enabled`
to false; an optional strict `OPSWEAVE_NOTIFICATION_ENABLED` boolean overrides
only enablement. Require 1-128 unique profiles when enabled. Each profile has a
1-64 ASCII letters/digits/underscore/hyphen name and exactly one of these action
identifiers. Register only actions with a configured profile. A workflow cannot
change that profile's destination, identity, credential or transport policy.

Credentials are environment-only references, never YAML/workflow plaintext,
database values, CLI arguments or outputs. Credential fields name variables in
the `OPSWEAVE_ACTION_SECRET_` namespace, with a nonempty uppercase-letter/digit/
underscore suffix and total name length at most 128. Accept only referenced secret
variables; reject unknown critical OpsWeave environment settings. Validate supplied
profile fields/references even when disabled; resolve required nonempty values
only for enabled profiles. Cap each credential at 4096 bytes and secret URLs at
2048 bytes. Reject control characters/injection text according to credential type.

Snapshot credentials privately at runtime construction; rotations require a new
CLI invocation or server restart. Exported config/catalog descriptors and durable
revisions retain only nonsecret profile/reference information. Never serialize
resolved values or log credential-bearing URL paths, authorization, SMTP commands,
provider bodies or authentication errors. Go memory zeroization is not promised.
Persistent encrypted secret storage/references remain Phase 05, not an implicit
prerequisite implemented early here. Existing minimal shell environments must
continue excluding these variables from child processes.

## Channel policy

| Action                  | Administrator profile fields                                                         | Fixed delivery and acceptance                                                                                       |
| ----------------------- | ------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------- |
| `notification.telegram` | `chat_id`, `token_env`                                                               | HTTPS POST `https://api.telegram.org/botTOKEN/sendMessage`; HTTP 200 plus typed `ok: true`                          |
| `notification.slack`    | `url_env`                                                                            | HTTPS POST to a validated `https://hooks.slack.com/services/...` incoming webhook; HTTP 200 with bounded `ok` reply |
| `notification.email`    | `host`, `port`, `tls_mode`, `username_env`, `password_env`, `from`, `recipients`     | Authenticated SMTP over mandatory verified TLS; accept only after DATA acknowledgment and successful owned cleanup  |
| `notification.webhook`  | `allowed_origin`, `url_env`, optional `bearer_env`, optional `allowed_private_cidrs` | HTTPS POST fixed JSON to the exact configured origin; any 2xx acknowledgment                                        |

Telegram numeric chat IDs are fixed nonzero signed 64-bit identifiers; no workflow
recipient override. Validate bot-token syntax without returning it in an error.
Use plain message text, no parse mode, keyboards, attachments or dynamic API path.
See [Telegram sendMessage](https://core.telegram.org/bots/api#sendmessage).

Slack accepts only its canonical public incoming-webhook host/HTTPS port, with a
bounded `/services/` path and no userinfo/query/fragment. The secret URL fixes its
destination; no channel, username, blocks or attachment override. Disable link/media
unfurling in the fixed payload. See [Slack incoming webhooks](https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks/).

Email requires 1-10 fixed unique recipients and one sender, validated as bare
ASCII mailbox addresses (no display names, control characters or header text).
Profiles explicitly select `implicit_tls` or mandatory `starttls` and port 1-65535.
Never authenticate or send DATA until TLS/certificate/hostname verification
succeeds; no plaintext or opportunistic downgrade. Use a bounded MIME text/plain
UTF-8 message with an encoded subject. No attachments, CC/BCC, arbitrary headers,
recipient/from overrides or shell/sendmail invocation. Use supported standard
[net/smtp](https://pkg.go.dev/net/smtp) and [net/mail](https://pkg.go.dev/net/mail)
boundaries rather than a new dependency.

Email additionally permits explicit `allowed_private_cidrs` for a trusted private
SMTP relay, following the same restricted-address rules as the generic webhook.
TLS uses system trust and a minimum TLS version of 1.2; no insecure verification
switch. Private deployments must establish host trust using the OS trust policy.
Generic webhook URLs may contain secret path/query data, but must match the
profile's canonical HTTPS origin and have no userinfo/fragment. Bearer credentials
are profile-supplied only, never arbitrary workflow headers.

## Inputs and outputs

All four actions require string `profile` and nonempty string `message`, at most
4096 UTF-8 bytes. Email also requires `subject`, 1-256 UTF-8 bytes without control
characters or CR/LF. Reject unknown fields and incompatible profile/action pairs
before connecting. Payloads are fixed channel-specific JSON or MIME; there is no
arbitrary JSON template, endpoint, recipient, method or header input.

All actions return only `accepted: true` after the provider's bounded success
acknowledgment. Other statuses, malformed responses or policy/transport/cleanup
errors fail with no output. Do not capture provider IDs, messages, recipients,
raw response content or credentials. `accepted` means transport/provider acceptance,
not inbox delivery, reading, notification-policy success or exactly-once delivery.

Workflow message/subject text remains ordinary persisted workflow input, so
operators must not embed credentials or sensitive secret values there. This
design prevents adapter credentials being persisted; it does not make arbitrary
message content encrypted. Lifecycle logs omit payload text and credentials.

## Network ownership, SSRF and bounds

Reuse the existing origin/DNS/IP/TLS security rules without relaxing ordinary
HTTP action header restrictions. Credential-bearing adapters need a separate
trusted injection boundary, not a workflow-controlled authorization bypass.
Resolve/validate addresses and pin the selected numeric destination per invocation.
Prevent DNS rebinding, redirects and IPv4/IPv6 restricted-address bypasses. Fixed
Telegram/Slack endpoints accept public addresses only. Generic webhook/SMTP private
destinations require explicit narrow CIDR grants; prohibited unspecified/multicast/
metadata destinations must not become allowed implicitly.

Every invocation has the earlier engine deadline or a 30-second ceiling. Bound
connection/TLS/initial-header or greeting waits to five seconds within total. Own
and close connections/bodies; join cancellation/transport work. No connection
reuse, ambient proxy, cookies, decompression, API retry, SMTP credential fallback
or discovery from provider/SMTP environment settings.

Limit HTTP headers to 16 KiB, response body to 8 KiB, encoded request payload to
32 KiB and serialized output to existing 64 KiB. Bound SMTP receive traffic to
256 KiB total, including TLS/protocol traffic, to prevent unbounded SMTP reply
allocation; cap outbound MIME at 32 KiB. Protocol loops must obey total deadline
and input budgets. Any partial/oversized/truncated response fails without leaking
its contents. Close before cleanup can drain a rejected response indefinitely.

Cancellation/timeout/cleanup failure can happen after a provider accepted the
message. Do not roll back, repeat internally or claim exactly-once effects.
Existing engine retries can duplicate delivery and remain an explicit workflow
choice. No listener, subscription, background notification policy, persistent
credential store, management API or Phase 06 event-delivery service is added.

## Acceptance and decision

Test all channels with isolated fake HTTPS/SMTP endpoints and synthetic credentials,
never production providers/accounts. Cover default-off and exact profile grants,
missing/invalid references, catalog/history/log/error redaction, SMTP header
injection, TLS/STARTTLS/auth failure, SSRF/DNS/redirect/IPv6 controls, input/output
bounds, overlong SMTP replies, cancellation/deadlines, ambiguous acknowledgment
and owned shutdown. Test production CLI/history and queued dispatch through the
existing dependency-injection boundary, without adding a production insecure-TLS
test switch. Include bounded parsers/fuzzing, benchmark CI, full native/race/static/
scanner checks, English operator docs and compatibility evidence.

Accept these four fixed-profile, environment-credential, verified-network workflow
adapters to authorize implementation. Automatic notification policies and
persistent secrets/management APIs remain their later approved-phase decisions.
