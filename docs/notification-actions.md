# Fixed-profile notification actions

`notification.telegram`, `notification.slack`, `notification.email` and
`notification.webhook`, action version `1`, support `opsweave.io/v1` through the
existing engine, CLI and dispatcher. They default to disabled. An administrator
configures fixed destinations and credential references; workflow authors select
a profile and supply ordinary message text. Automatic lifecycle notifications,
encrypted persistent secrets and the management API belong to later phases.

## Configuration and credentials

```yaml
actions:
  notification:
    enabled: true
    profiles:
      - name: telegram_ops
        action: notification.telegram
        chat_id: '-100123456789'
        token_env: OPSWEAVE_ACTION_SECRET_TELEGRAM_TOKEN
      - name: slack_ops
        action: notification.slack
        url_env: OPSWEAVE_ACTION_SECRET_SLACK_URL
      - name: webhook_ops
        action: notification.webhook
        allowed_origin: https://notifications.example.com
        url_env: OPSWEAVE_ACTION_SECRET_WEBHOOK_URL
        bearer_env: OPSWEAVE_ACTION_SECRET_WEBHOOK_BEARER
      - name: email_ops
        action: notification.email
        host: smtp.example.com
        port: 465
        tls_mode: implicit_tls
        username_env: OPSWEAVE_ACTION_SECRET_SMTP_USER
        password_env: OPSWEAVE_ACTION_SECRET_SMTP_PASSWORD
        from: opsweave@example.com
        recipients: [operator@example.com]
```

Supply the referenced variables through the deployment's protected environment;
do not put their values in YAML, workflow inputs, shell history or tracked files.
There is no string interpolation or plaintext credential field. Only environment
names beginning `OPSWEAVE_ACTION_SECRET_`, followed by uppercase ASCII letters,
digits or underscores, are accepted (maximum 128 bytes including prefix). The
strict configuration loader rejects unreferenced `OPSWEAVE_ACTION_SECRET_*`
variables and other unknown `OPSWEAVE_*` settings. Scope each process's environment
to its configured profiles. `OPSWEAVE_WEBHOOK_SECRET` remains the separate ingress
authentication setting.

When enabled, the CLI snapshots referenced credentials privately while composing
the action runtime. Values must be nonempty valid UTF-8 without control characters,
at most 4096 bytes; complete credential URLs are limited to 2048 bytes. Missing or
invalid credentials fail runtime construction with a sanitized error. Configuration
validation and history reads do not need resolved credentials. Restart a running
server after credential rotation or policy changes; a new CLI invocation uses a
new snapshot. Protect process-environment access, configuration and workflow
authoring rights with OS permissions. This is a service-wide grant, without
per-user RBAC or a persistent secret store.

Enablement requires 1-128 unique profiles. Names are case-sensitive, 1-64 ASCII
letters, digits, underscores or hyphens. Each profile grants exactly its stated
action. Only configured action types are registered; HTTP, shell, Docker,
filesystem and notification permissions remain independent. Unknown fields,
invalid references, duplicate names and incompatible channel fields are rejected,
including when disabled. `OPSWEAVE_NOTIFICATION_ENABLED=true` or `false` overrides
only enablement; other spellings are invalid. Existing configurations remain valid
with notifications disabled.

## Channel contracts

| Action                  | Fixed destination and acknowledgment                                                                                                                                                                                                                                                                                                                                         |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `notification.telegram` | HTTPS `api.telegram.org:443`, `/bot<TOKEN>/sendMessage`; `token_env` holds a positive decimal bot ID, colon and nonempty ASCII letters/digits/underscore/hyphen token. `chat_id` is a canonical nonzero signed 64-bit decimal ID. POST plain `text` and configured `chat_id`; no parse mode or attachments. Require HTTP 200 and a JSON Boolean `ok: true`.                  |
| `notification.slack`    | `url_env` holds an HTTPS `hooks.slack.com:443/services/T/B/secret` URL with three nonempty ASCII letters/digits/underscore/hyphen segments and no query, fragment or userinfo. POST `text` with link/media unfurling disabled. Require HTTP 200 and body `ok` (surrounding whitespace allowed).                                                                              |
| `notification.webhook`  | `url_env` holds a complete HTTPS URL at exactly `allowed_origin` (scheme, canonical host and port). Path/query may contain credentials and stay private. Optional `bearer_env` supplies one validated Bearer token. POST only `{"message":"..."}`; require HTTP 2xx. No redirect following.                                                                                  |
| `notification.email`    | Fixed `host`, port 1-65535, `from` and 1-10 distinct bare ASCII `recipients`. `tls_mode` is `implicit_tls` or mandatory `starttls`; `username_env`/`password_env` provide SMTP PLAIN credentials. Require verified TLS before authentication or DATA, then successful DATA acknowledgment and QUIT/cleanup. No plaintext fallback, CC/BCC, arbitrary headers or attachments. |

For a private webhook or SMTP relay, add `allowed_private_cidrs` to that profile,
for example `[10.20.30.40/32]`. Grants must fit wholly within RFC1918, loopback or
IPv6 ULA ranges; broad public grants are invalid. Fixed Telegram/Slack profiles
cannot grant private addresses. Public origins do not require private grants.
Use a bare SMTP hostname or IP, without a URL scheme, brackets or port suffix.
Email emits a UTF-8 plain-text MIME message with encoded subject and base64 body.

## Workflow syntax and execution

```yaml
apiVersion: opsweave.io/v1
kind: Workflow
metadata: {name: notify-operator}
spec:
  steps:
    - id: webhook
      uses: notification.webhook
      with: {profile: webhook_ops, message: 'Maintenance completed'}
    - id: email
      uses: notification.email
      with:
        profile: email_ops
        subject: 'Maintenance status'
        message: 'Maintenance completed'
```

All actions require `profile` and `message`. Message text must be nonempty UTF-8,
at most 4096 bytes; multiline text is allowed. Email additionally requires
`subject`, 1-256 UTF-8 bytes without control characters. Unknown fields, missing
profiles and channel/profile mismatches fail before connecting. A workflow cannot
override recipients, origin, URL, authorization, headers, method or TLS settings.
See [CLI execution and history](execution.md) and [queued dispatch](dispatcher.md).

Success returns only `{"accepted":true}`. It means provider/transport acceptance,
without proving inbox delivery or reading. Provider bodies, IDs, addresses and
credentials are never captured as action output. Message and subject remain
ordinary **persisted workflow inputs**: never embed credentials or secret values
in them. Lifecycle logging omits payload text and adapter credentials.

Every invocation uses the earlier engine deadline or a 30-second ceiling. Dial,
TLS and initial HTTP headers/SMTP greeting are bounded by five seconds within
that ceiling. HTTP headers are limited to 16 KiB, response body to 8 KiB and
encoded outbound payload to 32 KiB. SMTP inbound traffic, including TLS, is
limited to 256 KiB per invocation; outbound MIME is limited to 32 KiB. TLS requires
1.2 or newer, verified certificate/hostname and system trust roots.

DNS answers are all validated before dialing a pinned numeric address. Redirects,
ambient proxies, cookies, connection pooling, compression and internal delivery
retries are disabled. Link-local, unspecified, multicast and prohibited metadata
addresses remain denied despite private grants. The ordinary HTTP action's
authorization-header restriction is unchanged. Network cancellation closes owned
connections and joins cleanup before returning.

Failure, cancellation or cleanup error can occur after a provider accepts a
message. Failed actions return no success output; operators must inspect the
provider before retrying an uncertain attempt. Explicit engine retry policies may
duplicate notifications. There is no rollback or exactly-once guarantee.

## Troubleshooting, upgrades and backup

For configuration errors, check reference names, process environment, exact
origins, profile/channel pairing and TLS mode without printing credential values.
For network failures, check DNS, explicit private CIDRs, outbound firewall access,
system CA trust, provider credentials and bounded acknowledgment sizes. STARTTLS
must be advertised and succeed; SMTP must advertise AUTH PLAIN after TLS. Never
disable certificate validation or downgrade to plaintext to fix delivery.

Repeated provider failures should be investigated before enabling engine retries;
revoking a profile prevents new delivery after runtime reconstruction, while
immutable execution history remains readable. Back up the existing SQLite database
and nonsecret policy configuration using the [foundation procedure](foundation.md).
Restore environment credentials separately through the deployment's protected
mechanism. No database migration, dependency, minimum Go version, management API
or existing workflow syntax change is required. Removing the opt-in configuration
restores default-off behavior; retained revisions still reference their exact
action versions and cannot execute when the corresponding grant is unavailable.
