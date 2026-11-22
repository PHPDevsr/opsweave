# Phase 04 Docker action proposal

Status `PROPOSED`. Review this separate daemon/security contract before runtime
implementation. It does not follow automatically from shell approval.

## Delivery and configuration

Deliver `docker.start`, `docker.stop`, `docker.restart` and `docker.inspect`, exact
action version `1` for `opsweave.io/v1`, through the existing runtime and engine.
Default `actions.docker.enabled` to false. When enabled require:

- `endpoint`: one explicit local Unix socket (`unix:///absolute/path`) or Windows
  local named pipe (`npipe:////./pipe/name`). No TCP, HTTP/HTTPS, SSH, remote pipes,
  Docker context lookup, implicit endpoint or DOCKER_HOST inheritance.
- `allowed_containers`: 1-128 unique full lowercase 64-hex container IDs. Workflow
  input must exactly match an allowlisted ID; names, shortened IDs, wildcards,
  aliases and path/query text are rejected before connecting.
- `allowed_actions`: a nonempty unique subset of the four exact action IDs. Grant
  only this subset, so inspection can be enabled without mutation rights.

Reject unknown fields and invalid policy even when disabled. An optional strict
OPSWEAVE_DOCKER_ENABLED boolean overrides only enablement. Workflow input cannot
change endpoint, API version, action grants or container policy. Keep profiles and
HTTP settings independent. Configuration validation performs no daemon operation.

Use the standard library and existing x/sys for local bounded IPC transport; no
new dependency, database migration or public management endpoint is planned.
The [Docker CLI transport reference](https://docs.docker.com/reference/cli/docker/)
documents local Unix/named-pipe endpoint forms. This proposal intentionally limits
its transport surface to local IPC. Named-pipe connection setup must prevent
server impersonation of the service's token and obey context/deadline bounds.

## Input, API and output contract

All actions take only required string `container`, the exact allowlisted ID.
Use explicitly versioned Docker Engine API `1.48`, with no silent downgrade.
Unsupported daemons fail. See the [version matrix](https://docs.docker.com/reference/api/engine/)
and [API 1.48 specification](https://raw.githubusercontent.com/moby/moby/v28.0.0/api/swagger.yaml).

| Action           | Fixed operation                             | Successful output                                                    |
| ---------------- | ------------------------------------------- | -------------------------------------------------------------------- |
| `docker.start`   | POST container start                        | `container_id`, boolean `changed`                                    |
| `docker.stop`    | POST container stop with 10-second grace    | `container_id`, boolean `changed`                                    |
| `docker.restart` | POST container restart with 10-second grace | `container_id`, boolean `changed`                                    |
| `docker.inspect` | GET container inspect                       | `container_id`, `name`, `image_id`, `status`, `running`, `exit_code` |

For start/stop, 204 means changed and 304 means already in the requested state.
Restart accepts 204 with changed true. Inspect requires 200 and a response ID
matching the requested ID; return only the listed bounded typed fields. Validate
names/status/hash formats and UTF-8 before capture. All other statuses, malformed
or oversized responses, unavailable containers and transport errors fail. Ignore
additional daemon fields without persisting them. Never return raw inspect JSON,
environment, labels, mount configuration, credentials or daemon error bodies.

## Ownership and security boundary

Each invocation has the earlier engine deadline or a 30-second action ceiling.
Bound connection/header waits to five seconds and response headers to 16 KiB.
Limit response bodies to 1 MiB and serialized outputs to the existing 64 KiB cap.
Disable redirects, environment proxies, cookies, decompression and connection
reuse. Close owned handles/bodies and join transport work on cancellation.

No container creation/deletion, image operations, exec/attach, arbitrary API paths,
configuration mutation or host commands are exposed. Socket access is explicitly
opt-in and only configured containers/actions are available through OpsWeave.
The underlying daemon/socket remains privileged. Administrators must trust it and
the chosen containers, including their host mounts and privileges; the allowlist
does not sandbox Docker or reduce the service account's underlying socket rights.
Use least-privileged service permissions and an endpoint policy appropriate to
the host. Workflow authors cannot broaden these grants.

Keep lifecycle logs to existing action/execution/step identities. Omit endpoint
paths, daemon bodies and sensitive inspect fields. Existing engine policies own
retries, persistence, cancellation and uncertain effects. No internal retries.
Cancelling a request does not roll back a daemon operation, and retrying restart
can repeat external effects. Never claim exactly-once mutations.

## Acceptance and decision

Test through isolated Unix-socket/named-pipe fake daemons, without production
Docker access: input and policy rejection, permission subsets, exact full-ID paths,
204/304 handling, inspect filtering, response limits, invalid JSON/UTF-8, version
failure, cancellation/deadlines, and ignored proxy/context environment. Verify
CLI-to-history and queued/server shutdown, fuzz untrusted parsing, add a bounded
IPC benchmark to CI, and run all required unit/race/security/native checks.
Update English reference, examples, upgrade and troubleshooting documentation.

Accept this local-IPC, exact-container/action allowlist and bounded-output contract
to authorize the four Docker adapters. Filesystem/notification contracts and
Phase 05 remain separate decisions.
