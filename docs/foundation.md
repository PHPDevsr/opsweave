# Workflow foundation administration

Phase 01 provides strict configuration, workflow parsing/preflight, an immutable
revision registry, single-node SQLite storage and administration commands. It does
not itself execute workflows; the [Phase 02 engine](execution.md) adds that internal
boundary and execution history commands. Production currently ships an empty action catalog:
`validate`, `plan` and `apply` reject action identifiers as unavailable. Tests inject
validation-only fixture descriptors to verify the full registration lifecycle;
fixture actions cannot be enabled from configuration or CLI flags. Real action
implementations belong to later approved phases.

## Installation and quick start

Use Go 1.27.3, then build from the repository root:

```sh
go mod download
go build -o bin/opsweave ./cmd/opsweave
./bin/opsweave init
./bin/opsweave workflow list
```

On Windows, build `bin/opsweave.exe` and invoke `.\bin\opsweave.exe`. `init`
exclusively creates `opsweave.yaml`; it never overwrites an existing file or
symlink, creates a database or starts a server. The first storage administration
command creates the database and applies migrations. Root help, `version` and
shell completion do not load configuration or open storage.

Current optional worker/queue settings and server operations are documented in
the [dispatcher guide](dispatcher.md).

## Configuration

```yaml
storage:
  path: ./data/opsweave.db
logging:
  level: info
```

Use `--config <path>` before the command to select another file. An explicit file
must exist, except when `init` creates it. Without the flag, a missing default
`opsweave.yaml` uses the defaults above. Unknown fields/settings, duplicate keys,
aliases, tags, nulls, multiple YAML documents, invalid UTF-8 and files over 64 KiB
are rejected. Empty supplied paths/levels are invalid. No credentials belong in
this configuration.

| Setting         | Environment override    | Accepted values                                                                                    |
| --------------- | ----------------------- | -------------------------------------------------------------------------------------------------- |
| `storage.path`  | `OPSWEAVE_STORAGE_PATH` | Local database file path; URI/DSN options, UNC/network paths and drive-relative paths are rejected |
| `logging.level` | `OPSWEAVE_LOG_LEVEL`    | `debug`, `info`, `warn`, `error`                                                                   |

Precedence is environment, file, defaults. Unknown `OPSWEAVE_` environment
settings are errors. Relative database paths resolve against the selected config
directory, including relative environment overrides; without a config they resolve
against the working directory. Directory names may contain spaces.

Ordinary commands emit no routine logs. Debug mode emits one structured JSON
configuration-load event on stderr containing the command name, without settings
or values. A failed debug write fails the command before opening storage. Errors
produce one safe diagnostic; command output stays on stdout.

## CLI reference

| Command                                          | Result                                                                                |
| ------------------------------------------------ | ------------------------------------------------------------------------------------- |
| `init [--json]`                                  | Create configuration exclusively; JSON returns operation status                       |
| `validate <file-or-directory> [--json]`          | Validate syntax, schemas, action compatibility and supported capabilities             |
| `plan <file-or-directory> [--json]`              | Same checks and deterministic dependency order/action versions; no runtime resolution |
| `apply <file-or-directory> [--json]`             | Validate the entire batch, then atomically publish new revisions                      |
| `workflow list [--all] [--json]`                 | Sort by workflow name; `--all` includes archived workflows                            |
| `workflow get <name> [--revision <id>] [--json]` | Read current or exact historical definition and registry metadata                     |
| `workflow revisions <name> [--json]`             | Read immutable revision history in ascending revision-number order                    |
| `workflow export <name> [--revision <id>]`       | Write exact original source bytes without an added newline                            |
| `workflow delete <name> [--json]`                | Archive and disable, retaining revisions and reserving the name                       |
| `workflow enable <name> [--json]`                | Enable a non-archived workflow without editing its revisions                          |
| `workflow disable <name> [--json]`               | Disable a non-archived workflow without editing its revisions                         |

Flags may precede positional arguments, for example:

```sh
opsweave --config ./opsweave.yaml workflow get --json example
opsweave --config ./opsweave.yaml workflow export --revision REVISION_ID example
```

Text output uses stable columns or labeled fields. JSON output is one JSON value
followed by a newline; source export rejects `--json`. A normalized definition
returned by `get` is an internal semantic representation with string durations;
use `export` for a source file suitable for import through `apply`.

Success returns exit 0, including an empty list. Usage, validation, cancellation,
configuration, storage and output failures return exit 1. No command prompts for
interactive input. A write failure after apply commits returns failure while the
revision remains durable: inspect revision history before retrying. Repeating an
identical apply creates another immutable revision and identity.

Listings/history reads are bounded to 10,000 records and 64 MiB of revision
payload per call. Larger results fail explicitly without partial output; use
exact `get`/`export` reads. Pagination is future API work. This protects command
memory usage as stored history grows.

## Validation and execution planning

See [workflow parsing](workflow-parsing.md) and the [accepted v1 contract](../harness/designs/workflow-v1.md)
for schema, expressions, durations and dependency semantics. Action identifiers
have exactly two segments separated by `.`, with `_` or `-` allowed within each
segment; `http.health_check` is syntactically valid, but remains unavailable in
the Phase 01 production catalog.

Directory inputs use `.yaml`/`.yml` files in lexical order, without recursion or
following symlinks. Empty directories, duplicate workflow names, non-regular input
files, more than 256 definitions, files over 1 MiB or batches over 16 MiB fail.
Directory entries are read in bounded chunks. Validate and plan never open a
database, contact action destinations, resolve secrets or start executions. Apply
preflight errors leave no new database file and no partially published batch.

Non-default policies, independent later roots, conditions, references and triggers
are rejected until their runtime phases ship. Catalog metadata validates typed
inputs and output references; declared credentials require secret references,
which also remain unsupported until the encrypted secrets phase. Never put secret
values in workflow YAML. No environment substitution, shell interpolation or
external plugin loader is provided.

## Storage and security

SQLite uses one connection per store, foreign keys, a 5-second busy timeout,
DELETE rollback journaling, synchronous FULL and immediate write transactions.
All batch revisions and current pointers commit together. Concurrent local handles
serialize through database write locks; they cannot allocate the same revision
number. Busy, migration and persistence errors fail visibly. Random 128-bit
revision IDs are independent of source hashes. Source and normalization version 1
are retained; source is not rewritten on export.

Revision updates/deletes are prohibited, including SQL triggers enforcing
immutability. Archiving keeps every revision, disables the workflow and reserves
its name; re-apply/re-enable of an archived name fails. No restore/unarchive or
hard-delete command ships in this phase. Exact historical reads/exports remain
available. Execution bindings and disabled-trigger behavior arrive with their
runtime phases.

This backend is single-node storage. Do not share it across hosts, put it on a
network filesystem or treat its locking as distributed job ownership. PostgreSQL,
distributed claims and leases are future work. Redis and RabbitMQ are not needed.
New files use mode 0600 and new parent directories mode 0700 where supported.
Windows inherits ACLs: restrict the database/config directory to the service user
and administrators. Existing filesystem ACLs/permissions are not rewritten.

## Backup, restore and upgrade

1. Stop all commands/processes using this database. If a process crashed, reopen
   it with the compatible binary to let SQLite recover before taking a backup;
   do not delete a hot rollback journal manually.
2. Copy the database to a restricted backup directory. Retain the matching config
   separately and record the binary/commit and backup hash. This phase has no
   online backup command.
3. To restore, keep the original intact and copy the backup to a separate private
   local path. Select that path through a dedicated configuration or environment
   override, then verify listing and exact revision/source reads with a compatible
   binary before replacing the operational copy.
4. Before upgrading, retain a stopped-process backup and the previous compatible
   binary. Startup verifies migration versions/checksums and applies pending SQL
   transactionally. Newer schemas, missing history or mismatched checksums fail;
   do not alter the migration ledger to bypass the error.
5. Roll back with the pre-upgrade backup and matching binary. There are no
   automatic down migrations; never point an older incompatible binary at a
   newer operational database. Restore to another path first so rollback preserves
   the failed upgrade's original artifacts.

No release tag exists for this development feature. The initial migration creates
new foundation tables; there is no prior supported OpsWeave database schema to
convert. Future release documentation must continue to build from exact tags.

## Troubleshooting

- An unavailable-action error is expected with the empty Phase 01 production
  catalog. Do not bypass it by registering test stubs in production.
- A database error can indicate permissions, a directory/file collision, an
  incompatible schema or contention exceeding the busy timeout. Verify the local
  path and matching binary, stop conflicting processes, and use a backup for
  recovery. CLI diagnostics intentionally omit raw SQL and configured values.
- Enable debug diagnostics for command context without exposing input values.
  Use shell exit status and JSON output for automation.
- On Windows, race tests need CGO and a compatible MinGW-w64 compiler. A sandbox
  may prevent ThreadSanitizer address-space reservation even with a compiler;
  use a supported test process or passing CI evidence, never label that error a
  successful race test.
