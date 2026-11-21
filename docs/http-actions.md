# HTTP actions

`http.request` and `http.health_check` version `1` support `opsweave.io/v1`.
They are disabled by default. No extra Go dependencies or schema migrations are
needed. Existing workflow revisions retain their exact action bindings; disabling
HTTP prevents new invocations without rewriting history.

## Enable and run

Configure an exact origin allowlist in `opsweave.yaml`:

```yaml
actions:
  http:
    enabled: true
    allowed_origins:
      - https://status.example.com
```

Origins include a scheme, hostname and optional port, with no path, query,
fragment, userinfo or wildcard. Default ports are normalized to 80/443. Duplicate
normalized origins and unknown settings are rejected, even when disabled. Lists
have a maximum of 128 entries. Each invocation uses the current operator policy.

For a trusted local service, add its origin and the smallest required private
CIDR explicitly:

```yaml
actions:
  http:
    enabled: true
    allowed_origins: [http://127.0.0.1:1999]
    allowed_private_cidrs: [127.0.0.1/32]
```

Only subnets within RFC 1918 IPv4 private ranges, IPv6 unique-local ranges and
loopback ranges can be granted. Broad public CIDRs and link-local ranges cannot
be granted. Known metadata/platform service addresses remain denied, including
169.254.169.254, fd00:ec2::254, 168.63.129.16 and 100.100.100.200. Administrators
must trust allowed services, including any forwarding they perform.

Environment overrides take precedence over the file:

| Variable                              | Value                                          |
| ------------------------------------- | ---------------------------------------------- |
| `OPSWEAVE_HTTP_ENABLED`               | Exactly `true` or `false`                      |
| `OPSWEAVE_HTTP_ALLOWED_ORIGINS`       | Comma-separated origins; empty clears the list |
| `OPSWEAVE_HTTP_ALLOWED_PRIVATE_CIDRS` | Comma-separated CIDRs; empty clears the list   |

Example workflow:

```yaml
apiVersion: opsweave.io/v1
kind: Workflow
metadata:
  name: service-health
spec:
  steps:
    - id: health
      uses: http.health_check
      with:
        url: https://status.example.com/health
```

Run `opsweave validate workflow.yaml`, `opsweave apply workflow.yaml`, then
`opsweave run service-health`. Use `opsweave execution get <id>` for persisted
step results. Queued and scheduled work uses the same HTTP runtime when drained
by `opsweave server`. Validation/planning performs no network calls.

## Input and result reference

| Action              | Inputs                                               | Output                                   |
| ------------------- | ---------------------------------------------------- | ---------------------------------------- |
| `http.request`      | Required `url`; optional `method`, `body`, `headers` | Integer `status_code`, string `body`     |
| `http.health_check` | Required `url`                                       | Integer `status_code`, boolean `healthy` |

`method` defaults to GET. Supported values are GET, HEAD, POST, PUT, PATCH,
DELETE and OPTIONS. GET/HEAD cannot carry a nonempty body. Request body defaults
to empty. `headers` is an optional array of objects with required `name` and
`value` strings. Header names must be unique ignoring case. Unknown inputs,
control bytes, non-ASCII header values and invalid UTF-8 bodies are rejected.

Request succeeds only for 200–299; other statuses fail without storing the
response body. Health checks issue GET and return `healthy: false` for other
statuses. Transport errors, rejected destinations, invalid UTF-8, compressed
responses and resource-limit failures fail both actions. No response headers are
returned. Empty Content-Encoding is required. Redirects are returned as their
original status and never followed.

URLs, inputs and successful outputs are ordinary persisted data. Never supply
secrets through URLs, bodies or custom headers. Authorization, Proxy-Authorization,
Cookie, Set-Cookie, X-Api-Key, Api-Key and X-Auth-Token headers are rejected.
Host, Content-Length, Connection, Transfer-Encoding, Upgrade, Trailer,
Accept-Encoding, TE and Proxy-Connection overrides are also rejected. Authenticated
adapter credentials and secret-reference resolution require later reviewed work.

## Resource and network controls

- Request and response bodies are limited to 8192 bytes each. Oversized or binary
  responses fail, without truncation. Runtime serialized output is also limited
  to 65536 bytes, including JSON escaping.
- Input headers have a 16-entry and 4096-byte name/value total limit. Response
  headers have a 16384-byte limit.
- Each action stops at the earlier engine deadline or a 30-second ceiling.
  DNS/connect, TLS handshake and response headers each have a five-second bound.
- Every DNS answer must pass policy. Connections dial the checked literal IP while
  retaining the original HTTP hostname and TLS verification name. Every new
  connection resolves/checks again. Mixed public/restricted DNS answers fail.
- IPv4-mapped IPv6 and scoped addresses are rejected. Public routing excludes
  conservative special-purpose ranges using the [IANA IPv4 registry](https://www.iana.org/assignments/iana-ipv4-special-registry/)
  and [IANA IPv6 registry](https://www.iana.org/assignments/iana-ipv6-special-registry/).
  Some globally reachable special-purpose exceptions are deliberately denied.
- Environment proxies, redirects, cookie jars, connection reuse and automatic
  decompression are disabled. TLS certificates are verified normally.

Existing engine policies own workflow/step cancellation, retries, concurrency and
uncertain-effect recovery. These actions do not retry internally. An operator's
workflow retry can repeat a remote mutation; timeout does not undo remote effects.
Lifecycle logs retain action/execution/step identity and omit HTTP payloads, URLs,
headers and raw transport errors.

## Troubleshooting and upgrades

If deployment reports an unavailable action, enable HTTP and grant an origin in
both administration and server configuration. If an action fails, check scheme,
port, all DNS answers, the private CIDR grant, TLS trust, endpoint status and body
limits. A generic action failure deliberately hides transport details and response
payloads. Use endpoint-side diagnostics without copying secrets into workflow
history. A redirect target needs a direct URL and its own approved origin.

Existing configuration remains valid with HTTP disabled. No database migration
or data conversion is introduced by these actions. Roll back the binary only after
stopping active work; earlier binaries cannot execute these action versions.
Historical executions remain readable through existing history commands.
