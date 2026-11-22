# Granted local filesystem actions

`file.exists`, `file.cleanup` and `file.copy`, action version `1`, support
`opsweave.io/v1` through the existing executor, engine and dispatcher. They are
disabled by default. A workflow selects an administrator-configured root alias
and relative path; it cannot supply a new absolute root or expand its grants.

## Configuration and deployment

```yaml
actions:
  filesystem:
    enabled: true
    max_copy_bytes: 16777216
    roots:
      - name: incoming
        path: /srv/opsweave/incoming
        allowed_actions: [file.exists, file.copy]
      - name: outgoing
        path: /srv/opsweave/outgoing
        allowed_actions: [file.exists, file.copy, file.cleanup]
```

Use native absolute existing directory paths; on Windows, for example,
`path: 'C:\OpsWeave\incoming'`. Reject UNC/device/network path spellings, extra
colons beyond a Windows drive prefix, invalid UTF-8 and control characters.
Directory layout and configuration must be trusted administrator assets. There
is no implicit working-directory or home-directory root.

Configure 1-128 roots when enabled. Root names are unique, case-sensitive 1-64
ASCII letters/digits/underscores/hyphens. Each root has a nonempty unique subset
of the three action IDs. A cleanup grant permits deletion and must be explicit.
Copy requires `file.copy` on both source and destination roots; a grant on one
side does not authorize the other. Registrations expose only actions granted by
at least one root, and each invocation rechecks its selected roots' grants.
These are service-wide grants available to workflow authors with runtime access;
they do not implement per-user RBAC or an OS sandbox.

`max_copy_bytes` is a decimal byte count, default 16777216 (16 MiB), from 1048576
through 67108864 (1-64 MiB). `OPSWEAVE_FILESYSTEM_ENABLED=true` or `false` overrides
only enablement. No environment root/path override or interpolation is supported.
Unknown fields, duplicate roots/grants and invalid supplied policy are rejected
even when disabled. Validation stats directories without reading file contents
or mutating files. Existing configurations remain valid with filesystem disabled;
HTTP, shell, Docker and filesystem permissions are independent.

Use local filesystems supporting hard links within the destination directory.
Restrict roots to trusted trees without untrusted local writers, external hard
links, mount points, special devices or pseudo-filesystems. Network/FUSE/untrusted
mounts are unsupported. A root boundary does not sandbox the service account or
detect these administrator-controlled filesystem relationships. The
[Go Root contract](https://pkg.go.dev/os#Root) describes containment and limits.

## Workflow and CLI

```yaml
apiVersion: opsweave.io/v1
kind: Workflow
metadata:
  name: copy-approved-input
spec:
  steps:
    - id: check
      uses: file.exists
      with:
        root: incoming
        path: report.bin
    - id: copy
      uses: file.copy
      with:
        source_root: incoming
        source: report.bin
        destination_root: outgoing
        destination: report.bin
```

```sh
opsweave --config opsweave.yaml validate workflow.yaml
opsweave --config opsweave.yaml plan --json workflow.yaml
opsweave --config opsweave.yaml apply workflow.yaml
opsweave --config opsweave.yaml run --json copy-approved-input
opsweave --config opsweave.yaml execution get --json EXECUTION_ID
```

Use `run --enqueue` and `server` for queued dispatch. Exact version `1` bindings
are stored in immutable revisions. Changing configuration/root grants affects
future invocations, including older revisions, without rewriting history.
Revocation blocks side effects; historical typed outputs remain readable.
Each CLI invocation or server startup snapshots policy. Restart a running server
to apply configuration/grant changes; no live policy reload is provided.

| Action         | Required input fields                                      | Output                                              |
| -------------- | ---------------------------------------------------------- | --------------------------------------------------- |
| `file.exists`  | `root`, `path`                                             | `exists`, `kind` (`file`, `directory` or `missing`) |
| `file.cleanup` | `root`, `path`                                             | `removed` boolean; false when absent                |
| `file.copy`    | `source_root`, `source`, `destination_root`, `destination` | `bytes` integer                                     |

Paths are canonical slash-separated relative strings, 1-1024 UTF-8 bytes and at
most 32 components. Reject empty/dot/dot-dot components, absolute/drive/UNC/device
paths, backslashes, colon, NUL/control characters, Windows-invalid filename
characters (`< > " | ? *`), trailing dot/space, Windows reserved device names and
any component starting `.opsweave-` (case-insensitive). The root itself cannot be
selected. Path strings are literal: no globbing, expansion or directory traversal.
Windows file names retain native case-insensitive lookup; root aliases are exact.

## Operation behavior and ownership

`file.exists` reads metadata only, without listing directories or capturing file
contents. Missing paths yield `exists: false, kind: missing`. Final symlinks and
special files fail. Permission/infrastructure errors are failures, not missing.

`file.cleanup` removes exactly one regular file. It never walks a tree, removes
a directory/root, follows a final symlink, expands a pattern or sweeps temporary
files. A concurrent absent-file race returns false. Native deletion prevents
directory removal even if the final entry changes after validation.

`file.copy` reads one regular file in at most 32 KiB chunks, bounded by the
configured byte limit. Source identity/type is verified after opening; source
size/modification time is rechecked after transfer. Administrators must keep
source content stable: these checks do not provide a cross-process snapshot or
malicious-writer guarantee. No metadata, ACL, xattr, directory tree or link is
copied. Parent directories must already exist. An existing destination, including
a symlink/directory, is preserved and causes failure.

Copy writes a random `.opsweave-` temporary file in the destination directory,
flushes/closes it and publishes exclusively using a same-filesystem hard link.
It removes its own temporary name and reports cleanup errors. An unsupported
hard-link operation fails without a copy-to-final fallback. Exactly one of
concurrent copies to the same new destination can publish. New files use owner
read/write mode on Unix; Windows inherits the trusted directory's ACL. No file
contents, raw absolute paths or OS diagnostics enter outputs or lifecycle logs.

Opened parent-directory handles and `os.Root` operations provide confinement;
relative intermediate symlinks may resolve only inside their opened root. Final
file opens use native no-follow/type checks and verify descriptor identity.
Unix uses nonblocking no-follow opens and nondirectory unlink; Windows uses
relative NT opens rejecting reparse points and regular-file-only deletion by
validated handle. Every root/file/library/native handle is owned and closed.
No filesystem worker goroutine is detached or hidden behind a timeout.

## Cancellation, failures and recovery

The action budget is the earlier engine deadline or 30 seconds. Check cancellation
before filesystem work, between bounded I/O chunks and before publication/removal.
Cleanup checks use a separate five-second budget so an interrupted copy can
remove its own temporary file. Regular-file kernel operations have no hard
real-time cancellation guarantee: a stalled local filesystem can delay shutdown
until the owned syscall returns. The configured local/trusted-tree limits are
operational requirements, not claims of forced syscall termination.

Cancellation after link publication/deletion cannot undo that effect. Cleanup
failure can leave a published target or reserved temporary file; return failure
with no success output and inspect the tree before retrying. A crash can also
leave reserved files. There is no automatic sweep or destructive rollback. Restore
permissions and inspect these files under administrator control; ordinary workflow
paths cannot address reserved names. The action does not retry internally or
claim exactly-once execution, filesystem snapshots or crash-durable directories.
Engine retry/concurrency/uncertain-effect rules still apply.

## Upgrade, backup and troubleshooting

No dependency, database migration or management API change is required. Install
a binary containing these registrations, prepare trusted local roots, and enable
only the required actions. Keep configuration and existing SQLite backups under
the service's operator access policy; see the [foundation guide](foundation.md).
Back up file data separately using the filesystem's supported tools. Released
documentation continues to derive from exact tags in one documentation source.

An unavailable-action error indicates disablement or no grant for that action.
Action failure can indicate a path/root grant mismatch, invalid path, missing or
changed root/source, permission problem, unsupported hard links, source growth,
existing destination, size limit or cleanup failure. Inspect host diagnostics and
the directory under administrator control; OpsWeave omits raw OS details. Do not
automatically repeat a copy/delete after an ambiguous failure.

Tests use isolated trees, including native link-swap/escape, special-file, copy
failure cleanup, readonly-error, concurrent publication and CLI/history/shutdown
cases. Local Windows accounts without symlink privilege report unavailable tests;
CI requires these security cases and fails instead of skipping them. See the
[review](../harness/reviews/phase-04-filesystem.md) and [benchmarks](benchmarks.md).
