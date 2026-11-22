# Local Docker actions

`docker.start`, `docker.stop`, `docker.restart` and `docker.inspect`, action version
`1`, support `opsweave.io/v1` through the existing executor, engine and dispatcher.
All are disabled by default. They require an explicitly configured local daemon
endpoint, full container IDs and individual action grants.

## Configuration and deployment

```yaml
actions:
  docker:
    enabled: true
    endpoint: unix:///var/run/docker.sock
    allowed_containers:
      - aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    allowed_actions:
      - docker.inspect
```

Replace the illustrative ID with an existing container's full ID, for example
from `docker inspect --format '{{.Id}}' container-name` using an administrator's
Docker CLI. OpsWeave does not invoke that CLI or discover Docker configuration.
Enable mutation permissions only when required. `allowed_actions` accepts the
four exact action identifiers, with no duplicates or wildcards. An inspect-only
configuration cannot validate or invoke a workflow requiring `docker.start`.

On Linux/macOS use `unix:///absolute/path/to/socket`. Paths must be canonical,
valid UTF-8 and at most 4096 bytes, without control characters, URL escaping,
query/fragment text or backslashes. Native Unix socket path limits still apply
(typically much shorter); failure to connect is reported as an action failure.
On Windows use `npipe:////./pipe/docker_engine` or another explicit local pipe.
The pipe name is 1-128 ASCII letters/digits/dots/hyphens/underscores, excluding
`.` and `..`. Other spellings, remote pipes, TCP, HTTP/HTTPS and SSH are rejected.
An endpoint must use the transport native to the host OS.

`allowed_containers` contains 1-128 unique full lowercase 64-hex container IDs.
Names, shortened IDs, path/query text and wildcards are rejected. Recreating a
container changes its ID; update policy and workflow definitions deliberately.
Workflow inputs cannot select an endpoint, API version, Docker context or grant.
Unknown configuration fields and invalid supplied policies fail even when disabled.
Configuration validation contacts no daemon. Existing configurations remain valid
with Docker disabled. HTTP, shell and Docker grants are independent.

`OPSWEAVE_DOCKER_ENABLED=true` or `false` overrides only enablement. Enabling
without a complete valid policy fails. No other Docker policy environment
override is supported. `DOCKER_HOST`, `DOCKER_CONTEXT`, proxy variables and ambient
Docker credential/config files do not affect these actions.

## Workflow and CLI

```yaml
apiVersion: opsweave.io/v1
kind: Workflow
metadata:
  name: inspect-maintenance-container
spec:
  steps:
    - id: inspect
      uses: docker.inspect
      with:
        container: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
```

```sh
opsweave --config opsweave.yaml validate workflow.yaml
opsweave --config opsweave.yaml plan --json workflow.yaml
opsweave --config opsweave.yaml apply workflow.yaml
opsweave --config opsweave.yaml run --json inspect-maintenance-container
opsweave --config opsweave.yaml execution get --json EXECUTION_ID
```

Queue execution with `run --enqueue`; `server` owns dispatch, cancellation and
shutdown. All four actions accept only the required string `container`. Actions
are pinned to version `1` in immutable revisions. Revoking an action/container
grant blocks future invocations, including existing revisions; durable history
remains readable.

| Action           | Fixed Engine API operation               | Output                                                               |
| ---------------- | ---------------------------------------- | -------------------------------------------------------------------- |
| `docker.start`   | POST `/v1.48/containers/ID/start`        | `container_id`, `changed`                                            |
| `docker.stop`    | POST `/v1.48/containers/ID/stop?t=10`    | `container_id`, `changed`                                            |
| `docker.restart` | POST `/v1.48/containers/ID/restart?t=10` | `container_id`, `changed`                                            |
| `docker.inspect` | GET `/v1.48/containers/ID/json`          | `container_id`, `name`, `image_id`, `status`, `running`, `exit_code` |

Start/stop accept 204 (`changed: true`) or 304 (`changed: false`). Restart accepts
204 (`changed: true`). Inspect requires 200, exact matching container ID, image
ID `sha256:` plus 64 lowercase hex digits, a slash-prefixed ASCII container name
of at most 256 bytes, a known status, a boolean running flag and integer exit code
0-2147483647. Required fields cannot be missing/null. Known statuses are created,
running, paused, restarting, removing, exited and dead. Invalid responses fail.
Only these fields are returned. Environment, labels, mounts, credentials, unknown
fields and raw daemon JSON/error bodies never enter outputs or diagnostic text.

## Security, limits and execution behavior

The daemon endpoint is privileged. Administrators must trust the daemon, endpoint
ownership and allowlisted containers, including their mounts/privileges. These
grants restrict OpsWeave's action surface; they do not sandbox Docker or reduce
the service account's underlying socket rights. Apply least-privileged service
permissions and local endpoint access controls. Windows clients use anonymous
security quality of service to prevent the pipe server impersonating the service
token. Handles are non-inheritable and opened for overlapped cancellable I/O.

Each invocation owns one IPC connection with no pool, proxy, redirects, cookies,
decompression or reuse. It sends one fixed HTTP request, with no API retries or
negotiation. Windows waits for a busy pipe only within the connection budget.
The [Engine API version matrix](https://docs.docker.com/reference/api/engine/)
must include API 1.48 for the chosen daemon; unsupported versions fail rather than
downgrade. The [API specification](https://raw.githubusercontent.com/moby/moby/v28.0.0/api/swagger.yaml)
defines the pinned operations.

- Total action ceiling: 30 seconds, shortened by an earlier engine deadline.
- Connection and response-header waits: at most five seconds each, within total.
- Response headers: at most 16 KiB including status/header framing.
- Response body: at most 1 MiB and valid UTF-8; no content encoding is accepted.
- Serialized output: existing 64 KiB runtime cap. Inspect captures a much smaller
  fixed field set; oversized/malformed responses produce no output.

Cancellation closes IPC and joins its cancellation callback. Oversize rejection
closes the connection before closing the response body, so cleanup cannot drain
an unbounded response. No transport goroutine, pooled connection or retry owner
is left behind. The execution engine owns workflow/step timeouts, retries,
concurrency and persistence. Existing structured lifecycle logs include action,
execution and step identities, without endpoint paths or daemon bodies.
An IPC timeout at the engine deadline retains the deadline outcome, including
when native I/O reports its timeout before the context timer finishes signaling.

Cancellation or a connection error does not roll back a daemon operation.
An interrupted request may already have changed a container. Retrying restart
can repeat effects; no exactly-once or external rollback guarantee is provided.
No create/delete/image/exec/attach operation, arbitrary API path or configuration
mutation is exposed.

## Upgrade, backup and troubleshooting

No dependency, database migration or management API change is required. Rebuild
or install a binary containing these registrations, then explicitly enable the
required policy. Keep one documentation source; released docs derive from tags.
Protect configuration backups according to service account/endpoint access and
retain the existing SQLite backup procedure in the [foundation guide](foundation.md).

If validation reports an unavailable action, check enablement and the exact
`allowed_actions` subset. An action failure can indicate a rejected ID, missing
container, socket/pipe permission, unavailable endpoint, unsupported API version,
daemon error, malformed response or resource limit. Inspect daemon diagnostics
directly under administrator control; OpsWeave deliberately omits raw errors.
After an ambiguous mutation failure, inspect the container before retrying.
Following container recreation, update the full-ID policy and workflow together.

Tests use private fake Unix sockets/named pipes and fixed Engine API responses,
including production CLI/history and queued shutdown. They do not access a real
Docker daemon or prove the operational safety of administrator-chosen containers.
See [benchmarks](benchmarks.md) for the local IPC baseline and CI artifact limits.
