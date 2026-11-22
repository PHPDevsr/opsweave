# Phase 04 filesystem action proposal

Status `PROPOSED`. Review this separate path/destructive-operation contract
before implementation; Docker approval does not authorize it.

## Configuration and scope

Deliver `file.exists`, `file.cleanup` and `file.copy`, exact action version `1`
for `opsweave.io/v1`, using the existing action runtime and engine. Default
`actions.filesystem.enabled` to false. Require 1-128 explicitly configured roots
when enabled. Each root has a unique 1-64 ASCII letters/digits/underscore/hyphen
`name`, an absolute local existing directory `path`, and a nonempty unique
`allowed_actions` subset of the three action identifiers. Cleanup is destructive
and requires its own explicit root grant; enabling the subsystem grants nothing
implicitly. Copy needs `file.copy` grants on both its source and destination roots.

An optional `max_copy_bytes` defaults to 16 MiB and is bounded to 1-64 MiB.
An optional strict `OPSWEAVE_FILESYSTEM_ENABLED` boolean overrides only enablement.
Validate all supplied roots/policies even when disabled, reject unknown fields,
and perform no file-content read or mutation during configuration validation.
Do not inherit the process working directory, home directory or arbitrary
environment paths as roots. No new dependency or migration is planned.

Administrators must own/trust root configuration and tree layout. Restrict roots
to local filesystems supporting the required operations, with no untrusted local
writers, hard links to sensitive external files, mount points, device files or
pseudo-filesystems. A path root is not a filesystem or OS privilege sandbox.
The [Go Root contract](https://pkg.go.dev/os#Root) prevents symlink/path traversal
outside an opened root but does not exclude mounts, hard links or special files.

## Inputs and results

Paths are nonempty canonical slash-separated relative paths, at most 1024 UTF-8
bytes and 32 components. Reject absolute/drive/UNC/device paths, backslashes,
colon, NUL/control characters, empty/dot/dot-dot components, trailing dot/space,
Windows reserved names and any reserved `.opsweave-` component. Root itself cannot
be selected. The workflow selects only a configured root alias; it cannot supply
an absolute path, permissions, arbitrary flags, recursion or a new root.

| Action         | Required inputs                                            | Result and behavior                                                                 |
| -------------- | ---------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `file.exists`  | `root`, `path`                                             | `exists` boolean and `kind` (`file`, `directory` or `missing`); no contents/listing |
| `file.cleanup` | `root`, `path`                                             | `removed` boolean; remove exactly one regular file, return false if absent          |
| `file.copy`    | `source_root`, `source`, `destination_root`, `destination` | `bytes` integer; copy one bounded regular file to a new destination                 |

Final symlinks and special files fail; do not report their targets as safe files.
Intermediate relative symlinks may resolve only within their opened root. Use
Go's handle-relative `os.Root` operations rather than a string-prefix containment
check. Native safe-open/type checks must prevent a special-file open from blocking
the action before validation. Verify descriptors after opening. Tests must cover
symlink swaps and attempted escapes on each supported OS.

Cleanup never traverses or removes directories, follows a final link, expands a
glob, walks recursively or removes the root. A concurrent absent-file race is
idempotent false. Copy never overwrites an existing entry, creates directories,
copies metadata/ACLs/xattrs, recurses or clones links. Parent directories must
already exist. Source and destination may belong to different granted roots.

Copy reads in bounded chunks with a byte limit, writes a random reserved temporary
regular file in the destination directory, flushes/closes it and publishes a new
destination exclusively using a same-filesystem hard link. Remove the temporary
name and report cleanup errors. If hard-link publication is unsupported, fail
without a copy-to-final fallback. Existing targets remain unchanged. Output
permissions are owner read/write on Unix; Windows uses the trusted directory's
inherited ACL. Administrators must keep source content stable while copying;
there is no cross-process snapshot or malicious-writer guarantee.

## Execution and uncertainty

Check cancellation before filesystem work, between bounded read/write chunks and
before publication/removal. Use the earlier engine deadline or a 30-second action
budget. Own and close every root/file handle; do not detach a goroutine to pretend
an uninterruptible filesystem syscall has stopped. Ordinary regular-file kernel
operations do not offer a hard real-time cancellation guarantee. Operators must
exclude network/FUSE/untrusted mounts; a stalled local filesystem can still delay
shutdown until the owned syscall returns. This limitation is part of the contract
decision, not an exception silently introduced after approval.

Cancellation after publication/removal cannot undo the side effect. Flush/link
or cleanup errors can leave a published destination or reserved temporary file;
return failure, omit success output and document conservative operator inspection.
Process crashes can leave reserved temporary files; do not automatically sweep
unknown files. Existing engine retry/uncertain-effect rules apply, with no internal
retries, destructive rollback or exactly-once claim.

Keep logs/errors to existing action/execution/step identities. Do not expose raw
absolute paths, file contents or underlying OS diagnostics. Return only the
declared typed result fields; keep existing runtime output bounds.

## Acceptance and decision

Test native Linux/macOS/Windows paths, exact root/action grants, malformed paths,
symlink escapes/swaps, final links/special files, existing destination protection,
source limits/growth, copy failure cleanup, cancellation, concurrent exclusive
publication and single-file-only deletion. Use isolated temporary trees, never
user/production files. Verify production CLI/history, queued shutdown, bounded
path fuzzing, benchmark CI, race/static/security checks and English operator docs.

Accept this restricted nonrecursive, no-overwrite, local-root contract, including
hard-link publication requirements and cooperative filesystem cancellation, to
authorize implementation. Notification contracts and Phase 05 remain separate.
