# Phase 04 HTTP implementation review

Scope: `http.request`, `http.health_check`, SSRF policy and production composition.
Contract accepted by user reply `ACCEPT` on 2026-11-22 (Asia/Jakarta).

## Architecture and security review

- `internal/httpaction` composes exact-version action registrations and an
  immutable policy snapshot. Configuration validates its policy without network
  I/O. CLI foundation uses the catalog only for preflight; run/replay/server inject
  the runtime into the existing engine. Existing test injection remains available.
- No engine, transition, persistence, migration or workflow DSL changes. No new
  dependencies. Disabled HTTP yields no registrations or grants. No wildcard
  grants, TLS bypass, proxy support or configurable test transports are exposed.
- Review checked DNS validation and literal-IP pinning on every new connection,
  original Host/TLS names, private subnet containment, metadata/special-address
  rejection, output bounds, generic errors, body cleanup and cancellation.
- Response-body cleanup was corrected during review so compressed-response
  rejection still closes the owned body. Bounded header/body processing does not
  emit raw input or transport diagnostics. Input values remain ordinary persisted
  workflow data, as documented; authenticated credentials are outside this delivery.

## Observable test mapping

| Area                                                             | Tests                                                                                   |
| ---------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| Exact versions, defaults, grants, configuration validation       | `TestPolicyValidation`, `TestHTTPConfiguration`                                         |
| URL parsing, origin normalization, restricted IPs/private grants | `TestURLPolicy`, `TestIPPolicy`, `FuzzHTTPURLPolicy`                                    |
| DNS rebinding, mixed answers, pinned dialing and cancellation    | `TestDNSPinningRebindingAndCancellation`, `TestPinnedHostnameAndHeaders`                |
| Success/status/redirect behavior and byte limits                 | `TestHTTPBehaviorAndBounds`, `TestEscapedOutputAndTruncatedResponse`, `TestHeaderRules` |
| Cancellation, deadlines, TLS rejection and disabled proxies      | `TestHTTPContextTLSAndProxy`, `TestResponseAndConnectionDeadlines`                      |
| Production CLI preflight/execution, durable history, revocation  | `TestProductionHTTPWorkflow`, `TestProductionHTTPServerCancellation`                    |
| Performance measurement                                          | `BenchmarkHTTPPolicy`, asserted by benchmark CI                                         |

Verification results and limitations are recorded in the append-only
[build log](../build-log.md). See the [HTTP reference](../../docs/http-actions.md)
for configuration, CLI example, semantics, security bounds and troubleshooting.
Remote CI must be observed before claiming its quality gates passed. This review
does not complete later Phase 04 actions or authorize Phase 05.

## Rebased verification on Go 1.27.4

The user confirmed the separate security patch was merged. HTTP was rebased onto
origin/main `0033480`; the only conflict was append-only log history, preserved
from both branches. Application source/tests are unchanged from `4bf30d6`.
Full ordinary/race suites, vet, build, module verification and govulncheck@latest
passed on the combined source. HTTP coverage is 95.3%; engine 82.6%, execution
86.2%, workflow 94.3%. Bounded fuzzing passed 71,138 executions. Fresh PR checks
are still required before completing the four HTTP/SSRF ledger rows. See the
build log for exact benchmark values and command outcomes. Shell has no runtime
implementation and awaits its separate reviewed contract.
