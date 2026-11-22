# Phase 04 notification implementation review

Date: 2026-11-22 (Asia/Jakarta). Baseline `54b24cf`, existing Phase 04 branch and
draft PR #8 targeting main. The user explicitly ACCEPTED the
[four-channel contract](../designs/notification-actions.md). This review covers
`G-5.5-031`, `G-5.5-032`, `G-5.5-033`, `G-5.5-034` and the aggregate Phase 04
action milestone `G-11-004`. Native combined-source acceptance remains open.

## Implementation and security review

- `internal/notificationaction` owns strict nonsecret profiles, private environment
  snapshots, fixed provider payloads and bounded HTTPS/SMTP delivery. The CLI
  composes exact v1 registrations and independent permission grants. Unknown
  fields, cross-channel profiles and invalid inputs cannot dial. Configuration
  parsing never resolves or exports credential values.
- A small `httpaction.Destination` wrapper reuses the existing immutable origin,
  DNS-answer and numeric-IP dial policy. Ordinary `http.request` input/header
  policy is unchanged. DNS is validated before connection; fixed providers permit
  public destinations only. Private webhook/SMTP targets need explicit grants;
  metadata/link-local/multicast/unspecified addresses remain denied. No ambient
  proxy, redirect, API retry, pooling or credential-bearing workflow input exists.
- HTTPS uses one verified TLS connection and bounded raw response parsing, with
  16 KiB headers and 8 KiB body. Cleanup closes the owned socket before a rejected
  body could drain. SMTP requires implicit TLS or successful STARTTLS, verified
  hostname/certificate and AUTH PLAIN before DATA. Its 256 KiB raw receive budget
  includes TLS and bounds multi-line protocol parsing. MIME headers/destinations
  are fixed and validated; subject is encoded, body base64, outbound data bounded.
- A context callback closes the sole owned socket once and is stopped/joined on
  return. Cleanup errors propagate as sanitized action failure, even after an
  acknowledgment. No background transport owner or retry remains. Native deadline
  completion waits for context propagation before runtime classifies a timeout.
- All four actions return only `accepted: true` after acknowledgment and cleanup.
  Message/subject remain ordinary persisted input. Credentials, URLs, provider
  responses/IDs and SMTP exchanges do not enter catalog, history, logs or errors.
  Provider acceptance followed by failure remains uncertain and may duplicate on
  explicit engine retry; no rollback or exactly-once claim is made.
- Trusted Go dependencies allow isolated transport/root-CA/environment fixtures;
  production uses system trust/network and OS environment. No YAML or workflow
  switch disables TLS. Tests never contact providers or send real notifications.

Self-review found a Unicode control-character gap in the initial printable-value
validator; `unicode.IsControl` now covers C1 as well as C0/DEL, with regression
coverage. A cancellation fixture initially signaled readiness before a connection
was owned; it now synchronizes on receipt of actual TLS bytes. Assertions were
preserved and targeted/race reruns pass. Neither issue required a scope change.

## Observable acceptance mapping

| Behavior                                                                                  | Automated evidence                                                                                               |
| ----------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Telegram, Slack and webhook payloads, acknowledgment, snapshot and output                 | `TestHTTPChannels`, `TestProviderAcknowledgments`                                                                |
| Implicit TLS and mandatory STARTTLS delivery, fixed recipients and MIME                   | `TestEmailTLSDeliveryAndMIME`                                                                                    |
| Default-off, invalid credentials/profiles/references, input/permission/version limits     | `TestProfileValidation`, `TestCredentialResolutionAndRedaction`, `TestInputsPermissionsAndVersionsBeforeDial`    |
| Strict YAML/environment, nonsecret exported config and credential-free history validation | `TestNotificationConfigReferencesAndStrictEnvironment`, `TestNotificationConfigYAMLValidation`                   |
| Exact origin, DNS pinning and restricted/mixed IPv4/IPv6 answers                          | `TestDestinationExactOriginAndPinnedDNS` plus existing HTTP policy security suite                                |
| Response/body/header/TLS limits, hostname and private grant enforcement                   | `TestHTTPRejectsResponsesAndTLS`, `TestTLSHostnameAndPrivateGrantEnforcement`                                    |
| SMTP downgrade/AUTH/sender/recipient/DATA/ambiguous acknowledgment/QUIT/size failures     | `TestEmailFailures`                                                                                              |
| Cancellation/deadline and handshake/greeting ownership                                    | `TestHTTPTimeoutAndCancellation`, `TestEmailCancellationJoinsTLSAndGreeting`                                     |
| Acknowledged cleanup failure and rejected-body closure before drain                       | `TestAcknowledgedHTTPWithCleanupFailure`, `TestRejectedHTTPBodyClosesBeforeDrain`, `TestDialAndDeadlineFailures` |
| MIME/budget/Unicode/reference boundaries                                                  | `TestMailEncodingAndReferenceBoundaries`                                                                         |
| CLI composition, persisted output/redaction, queued shutdown and revocation               | `TestConfiguredNotificationOptInAndSnapshot`, `TestNotificationWorkflowAndQueuedShutdown`                        |
| Bounded parser fuzzing and real local TLS baseline                                        | `FuzzNotificationCredentialParsers`, `FuzzConfig`, `BenchmarkNotificationWebhook`                                |

## Local verification and compatibility

Go 1.27.4 full ordinary and race suites, gofmt, vet, module verification and build
passed on Windows. Final test-only extensions additionally passed notification
ordinary tests and three race repetitions. Full coverage: notification 91.8%,
HTTP 94.8%, configuration 97.2%, CLI 82.5%, engine 84.8%, execution 86.2%, workflow
94.3%; core targets remain satisfied. The existing three filesystem symlink tests
retain their explicit local missing-privilege limitation; native CI must execute
their mandatory security gate. It is not a notification skip or a passing test.

Freshly installed `govulncheck@latest ./...` found no vulnerabilities. All-workflow
actionlint passed. Four-second/two-worker notification parser fuzzing passed
51,664 executions; configuration fuzzing passed 51,555. Three short Windows
loopback HTTPS samples measured 1,164,322 / 1,119,071 / 1,105,517 ns/op,
139,134-139,432 B/op and 970 allocs/op. Setup is excluded; verified fresh TLS,
payload/acknowledgment and cleanup are included. CI requires NotificationWebhook
with existing environment/results artifacts. No optimization or SLA is inferred.

The [operator reference](../../docs/notification-actions.md) covers installation
configuration, workflow/CLI/history, credentials, delivery limits, security,
deployment, rotation, backup, upgrade and troubleshooting. No new dependency,
toolchain increase, schema migration or management API is needed. No release tags
exist to inspect; one development documentation source is retained.

The four notification rows and aggregate milestone are IMPLEMENTED pending all
fresh PR workflows. Seventeen earlier completed rows remain unchanged. Phase 04
acceptance and Phase 05 authorization remain human decisions. No merge, tag,
release publication, real provider message or deployment was performed.

## Combined-source acceptance — 2026-11-22

All six fresh PR workflows passed exact product source `963d143`:

| Workflow           | Successful run                                                               | Evidence                                                                          |
| ------------------ | ---------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| CI                 | [45024238339](https://github.com/PHPDevsr/opsweave/actions/runs/45024238339) | Six native OS/Go jobs, Linux race, formatting/modules/vet/build/smoke             |
| Vulnerability scan | [45024238286](https://github.com/PHPDevsr/opsweave/actions/runs/45024238286) | govulncheck@latest on Linux, macOS and Windows                                    |
| Benchmarks         | [45024238291](https://github.com/PHPDevsr/opsweave/actions/runs/45024238291) | Required NotificationWebhook and existing families; environment/results artifacts |
| Markdown           | [45024238392](https://github.com/PHPDevsr/opsweave/actions/runs/45024238392) | Bun formatting                                                                    |
| CodeQL             | [45024238284](https://github.com/PHPDevsr/opsweave/actions/runs/45024238284) | Go static security analysis                                                       |
| Release            | [45024238307](https://github.com/PHPDevsr/opsweave/actions/runs/45024238307) | Six OS/architecture packages and assemble; publication skipped                    |

All runs used pull_request events, without duplicate feature push workflows.
The native test jobs cover Go 1.27.4 and stable on Linux, macOS and Windows.
Both Windows jobs passed the existing mandatory symlink security prerequisite;
Linux/macOS ran their native filesystem paths and notification HTTPS/SMTP fixtures.
This closes the fresh combined-source platform gate without relabeling earlier
local privilege skips. Linux race and three-OS latest-scanner checks also passed.

Implementation, automated acceptance/security/integration tests, code review,
English documentation and compatibility evidence are complete for the four
notification rows. Only `G-5.5-031`, `G-5.5-032`, `G-5.5-033` and `G-5.5-034`
now transition to COMPLETED; seventeen earlier completed rows are preserved.
All 561 IDs/static fields and unrelated statuses remain unchanged. Final
acceptance documentation is separate from the fully tested product source.

All five Phase 04 action families are complete. `G-11-004` remains IMPLEMENTED
because human phase/release acceptance remains open, consistent with earlier
milestone accounting. Phase 04 stops at `PHASE_04_ACCEPTANCE_REQUIRED`; Phase 05
is unapproved. The existing PR remains draft targeting main. No merge, release
tag/publication, deployment or real provider message occurred. These results
supersede this review's earlier native-CI-pending statements.
