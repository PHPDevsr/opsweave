# Phase 04 HTTP action proposal

Status `ACCEPTED` by the user reply "ACCEPT" on 2026-11-22 (Asia/Jakarta). Phase 04 is authorized by the user's request on 2026-11-22
(Asia/Jakarta). This contract requires review before runtime implementation.

## First delivery

Deliver `http.request` and `http.health_check`, exact action version `1` for
`opsweave.io/v1`, through the existing action runtime and immutable revision
bindings. Keep the engine, retry ownership, persistence, cancellation and output
capture contracts unchanged. Shell, Docker, filesystem and notification actions
remain subsequent work units with their own reviewed security contracts.

The production CLI currently composes an empty catalog and no runtime. Compose
these two registrations from validated configuration for validate/apply/run/server.
Keep the existing injected runtime entry points for isolated tests. No database
migration, new dependency, credential persistence or secret interpolation is
proposed. There are currently no release tags to update.

## Inputs and observable results

| Action              | Inputs                                                                                                            | Successful output                           |
| ------------------- | ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------- |
| `http.request`      | Required `url` string; optional `method` (default GET), `body` string, `headers` array of `{name, value}` objects | `status_code` integer and `body` string     |
| `http.health_check` | Required `url` string                                                                                             | `status_code` integer and `healthy` boolean |

Reject unknown inputs, malformed URLs, duplicate header names, invalid header
bytes, GET/HEAD bodies and methods outside GET, HEAD, POST, PUT, PATCH, DELETE,
OPTIONS. A request succeeds only for 200–299. Health checks use GET and return
`healthy: false` for other final status codes so workflows can branch on the
result. Transport, policy, timeout, body-limit and invalid-text failures fail
both actions. Request failures retain the existing generic action error boundary
and do not persist response bodies. Neither action retries internally.

Request and response bodies must be valid UTF-8, at most 8 KiB each. The existing
64 KiB serialized runtime output budget remains an additional limit, including
JSON escaping. Reject excess output rather than silently truncate. Do not return
response headers. Reject URL userinfo, fragments, credentials in workflow headers
(including Authorization, Proxy-Authorization and Cookie), and overrides of Host,
Content-Length, Connection, Transfer-Encoding, Upgrade and Trailer. Bound headers
to 16 entries and 4 KiB total. Workflow source and ordinary outputs are persisted;
operators must not put secrets in URL, body or permitted headers. Persistent
secret references and authenticated adapters require a separate reviewed contract.

## Network policy and configuration

Introduce optional `actions.http` configuration. `enabled` defaults to false.
When enabled, require an explicit nonempty `allowed_origins` list of canonical
HTTP/HTTPS origins, including the effective port. No wildcards. Require an exact
origin match on every request. An empty disabled configuration adds no grants.
Enabling grants only `http.request` and `http.health_check` permissions and does
not enable other integrations. Reject unknown settings and malformed policy at
startup. Existing configurations continue to work with HTTP disabled.

Default policy permits globally routable unicast destinations only. Deny loopback,
private, link-local, unspecified, multicast, reserved and metadata destinations,
including IPv4-mapped IPv6 and special translation ranges. Resolve under the
invocation context and reject the entire DNS result if any answer is disallowed.
Connect to a validated literal IP with the original hostname retained for TLS
verification and HTTP Host. Revalidate every new connection, including after DNS
changes. Do not delegate a second unchecked hostname lookup to the dialer.

For infrastructure endpoints, an operator may explicitly list private or loopback
CIDRs in `allowed_private_cidrs`, in addition to the exact origin allowlist.
Continue to deny link-local/metadata and other special destinations even with
this setting. Workflow input cannot alter policy. Document the trust boundary:
an explicitly allowed endpoint can perform its own forwarding, so administrators
must trust it and its network. Do not infer network trust from signed webhook
ingress or from the illustrative localhost example in GOALS.md.

Disable redirects, environment proxies, cookie jars and automatic decompression.
Reject compressed responses. Retain normal TLS verification with no insecure
switch. Do not permit workflow TLS settings or arbitrary transports in production.
Use injected resolver/dial dependencies in tests without exposing a security
bypass through runtime configuration.

Each invocation uses the earlier of its engine deadline and a 30-second action
ceiling. Bound dialing/TLS/header waits to 5 seconds each and response headers to
16 KiB. Close bodies and owned transport resources on every path. No detached
network goroutines or unbounded connection pools. Cancellation must release owned
network operations before the handler returns.

## Logging, verification and completion

Use existing execution/step lifecycle logs with action identity. Never log URLs,
request/response bodies, headers, DNS answers or raw transport errors. Preserve
engine handling of uncertain external side effects, especially POST and other
mutating requests. An action timeout does not prove a remote effect was undone.

Before completion, add descriptor/runtime and CLI-to-storage execution tests,
plus isolated transport tests for success, invalid input, permission denial,
non-2xx behavior, cancellation, timeouts, exact limits, binary/escaped output,
DNS rebinding, mixed DNS answers, IPv4/IPv6, private CIDR opt-in, redirect/proxy
bypass, TLS validation and header/credential rejection. Add bounded fuzzing for
URL/policy inputs and a benchmark included in benchmark CI. Review code/security,
run all required Go/Markdown checks, and update English reference/configuration,
examples and troubleshooting documentation. Report unavailable CI gates honestly.
Do not mark either action complete before its evidence is attached to the ledger.

## Decision requested

Accept the first HTTP delivery contract, including default-disabled exact-origin
network access, explicit private/loopback CIDR opt-in, no inline credentials, and
the input/output/failure semantics above. Acceptance authorizes implementing these
two actions. It does not approve later action security contracts or Phase 05.
