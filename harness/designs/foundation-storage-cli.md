# Phase 01 storage, configuration and CLI proposal

Status: `ACCEPTED_FOR_IMPLEMENTATION`, 2026-11-08 (Asia/Jakarta). User reply
"setuju" accepted all five decisions below before their implementation. Storage,
configuration and commands are now implemented; see the [foundation guide](../../docs/foundation.md)
and build-log verification. Acceptance of this contract alone completes no feature.

## Decisions for review

1. Use single-node SQLite through `database/sql` and the CGO-free
   `modernc.org/sqlite` driver. Pin and verify a stable version compatible with
   Go 1.27.3 before implementation; no PostgreSQL dependency in this phase.
2. Each apply batch is atomic. Keep every revision and archive deleted workflows;
   never reuse an archived workflow name implicitly. Re-applying that name returns
   an archived-workflow error until a future explicit restore contract exists.
3. Use a small strict YAML configuration with environment overrides; `init`
   creates only a configuration file, using exclusive creation without overwrite.
4. Add explicit `workflow revisions`, `workflow export` and `plan` commands to
   expose revision history, lossless export and side-effect-free planning already
   required by GOALS. They are Phase 01 scope, not execution commands.
5. Amend the accepted action identifier rule: each segment uses
   `[a-z][a-z0-9]*([_-][a-z0-9]+)*`, with a maximum of 63 ASCII characters.
   Workflow, step, label and reference identifiers retain their existing grammar.
   This allows documented action names such as `http.health_check`; the parser
   implements this accepted correction while other identifier grammars stay fixed.

## Package and validation boundaries

- `internal/workflow`: SQL/HTTP-independent candidates, strict YAML parser,
  condition syntax, normalized dependency graph and capability validation.
- `internal/registry`: service-level validation and domain repository contract.
  The service validates all source files, duplicate names, action schemas,
  credentials and capabilities before issuing a batch write. SQL is not imported.
- `internal/storage/sqlite`: injected repository adapter, ordered migrations,
  transactions and health check. No action invocation or domain policy in SQL.
- `internal/config`: strict file/environment configuration and defaults; no CLI
  or database dependency. Reuse bounded YAML node auditing without exposing raw
  parser errors; extract a small internal YAML helper only when shared use exists.
- `internal/cli`: injected service and output streams; production uses an empty
  action catalog until supported actions ship. No test action registration in
  production, no public plugin loader, no hidden execution stub.

An injected definition catalog describes action identifier, version, workflow API
version, recursively typed input schema, named output fields, credential inputs
and sensitive outputs. Schemas accept only the documented scalar/map/list types,
enforce required/unknown inputs and validate references against declared outputs.
Credential input positions require secret references and those references remain
unsupported in Phase 01. This is validation metadata only; the executable action
interface remains Phase 02 work. Empty production catalog errors are documented
and deterministic. Parsing success alone is never reported as deployment success.

## SQLite and revisions

Use one connection per process (`SetMaxOpenConns(1)`), foreign keys enabled,
5-second busy timeout and SQLite's default rollback journal with synchronous FULL.
Do not expose user-controlled DSN options. Database paths are local file paths,
not URLs. Single-node ownership is required; this design provides no distributed
claims, leases or cross-host lock guarantees. Busy/IO failures return errors.

Migrations are embedded numbered SQL files. Maintain `schema_migrations` with
version and checksum. On startup, a transaction verifies known checksums and
applies unapplied migrations in order; reject newer schemas, missing historical
migrations and mismatched checksums. Transactional DDL failure rolls back the
migration and version record together. Never drop or rewrite user data to recover.

Initial tables:

| Table                | Data and invariants                                                                                                                                                                                           |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `workflows`          | Unique name, enabled flag, archived flag, current revision pointer, creation/update times                                                                                                                     |
| `workflow_revisions` | Random opaque 128-bit revision ID, workflow name FK, per-workflow increasing revision number, original source BLOB, normalized definition JSON, normalization version, descriptor binding JSON, creation time |
| `schema_migrations`  | Ordered version, SQL checksum, applied time                                                                                                                                                                   |

Use `crypto/rand` for opaque IDs, UTC timestamps, parameterized SQL, foreign keys,
unique name/revision constraints and transaction-local revision allocation.
Repository reads return independent values, not shared mutable cache entries.
Revision rows are append-only at the repository boundary. Test concurrent apply
through multiple store handles to prove revision allocation/current publication
cannot diverge; use a transactional write lock before reading the current revision.

Enforce that a current-revision pointer belongs to the same workflow with a
composite foreign key. A newly inserted workflow may have a null pointer only
inside its unpublished creation transaction; commit only after inserting its first
revision and publishing the pointer. The adapter owns transaction mode and all
connection pragmas; caller-supplied strings cannot change them.

Each accepted apply creates a new revision even for identical source. Publish all
revisions/current pointers in one transaction. Roll back the complete batch if
any workflow is archived or any insert/update fails. Updating preserves enablement;
new workflows start enabled. Delete archives and disables, retaining every revision
and reserving the name. Enable/disable of an archived name is rejected. Unknown
names return a domain not-found error. Ordinary listing excludes archived entries;
`--all` includes them. Archived definitions remain readable/exportable by exact
revision for history. No execution tables are introduced before Phase 02.

Normalization version 1 uses deterministic JSON with authored step order,
normalized defaults/dependencies, structured references, duration strings and
sorted mapping keys. No resolved secrets or runtime values. Exact source bytes
remain separately preserved for export. Stored normalization versions must be
recognized; do not silently reinterpret an unknown version.

## Configuration and init

Optional default file: `opsweave.yaml` in the current working directory. An
explicit `--config <path>` is required to exist; an absent default file uses
defaults. Reject unknown keys, duplicate keys, invalid UTF-8, aliases, tags,
nulls and extra YAML documents; size limit 64 KiB. Only these fields exist:

```yaml
storage:
  path: ./data/opsweave.db
logging:
  level: info
```

Environment overrides: `OPSWEAVE_STORAGE_PATH` and `OPSWEAVE_LOG_LEVEL`.
Precedence: environment, file, defaults. Empty supplied values are invalid.
Reject unknown `OPSWEAVE_` settings to catch critical typos. Logging levels are
`debug`, `info`, `warn`, `error`; no credentials are accepted by this configuration.
Relative database paths resolve against the selected configuration file directory,
or the working directory when no file exists. Reject URI/DSN forms and NUL bytes.
Do not include configured values or sensitive environment text in diagnostics.

`opsweave init [--config <path>]` writes the documented defaults with exclusive
creation; existing files, including symlinks, fail without overwrite. It creates
no database and starts no server. Keep database parent directories private where
supported; document inherited Windows ACL limitations. Help, version and completion
remain usable without loading configuration or opening storage.

## Command and output contract

| Command                                              | Behavior                                                                                        |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `validate <file-or-directory>`                       | Full schema/catalog/capability preflight; no database                                           |
| `plan <file-or-directory>`                           | Same preflight, stable dependency order and action bindings; no database, actions or executions |
| `apply <file-or-directory>`                          | Same preflight, then atomic registry revision publication                                       |
| `workflow list [--all]`                              | Sort by workflow name; include archived only with `--all`                                       |
| `workflow get <name> [--revision <id>]`              | Current definition/registry metadata or exact matching historical revision                      |
| `workflow revisions <name>`                          | Revision metadata sorted by revision number ascending                                           |
| `workflow export <name> [--revision <id>]`           | Exact original YAML bytes to stdout; no added newline                                           |
| `workflow delete <name>`                             | Archive and disable, preserving revisions                                                       |
| `workflow enable <name>` / `workflow disable <name>` | Update registry state without creating a revision                                               |

Keep existing help, version and completion behavior. Add `--json` to new commands
except source export, which rejects it; init JSON reports the safe operation
status without echoing file contents. JSON results are one value plus newline;
mutations return identifying metadata only. Text listings use deterministic
columns; no rows is a successful empty result. Exit 0 means success; argument,
validation, configuration, storage, cancellation and output errors remain exit 1.
Diagnostics go to stderr with one safe message. Check context before reading
files, starting transactions or changing state. Closing a store or writing output
must not hide a failure; an output failure after commit does not roll back an
already committed revision and must be documented for automation callers.

Directory inputs follow the reviewed workflow contract: lexical file order,
no symlinks or recursion, duplicate-name rejection. Apply preflight completes
before opening/creating the database. Default maximum is 256 input files and
16 MiB aggregate source bytes per batch, even if each file meets its 1 MiB limit.
Empty directories are invalid input. Input errors never quote definition contents.

## Acceptance and operations

Test isolated SQLite migration checksums/newer schemas, rollback, concurrent
apply, immutable revisions, enablement preservation, archive name reservation,
restart persistence, source export/import and query failure propagation. Test all
configuration precedence/error cases, exclusive init/no overwrite, canceled
contexts, JSON/text/argument/completion behavior and writer failures. Rejected
preflight must leave no database file and no partial batch state. Run all AGENTS
checks, bounded parser fuzzing and the race suite in a supported environment.

Before the first migration ships, document stopped-process backups of the database,
restore to a separate validated path, migration failure handling and upgrade
backups. No automatic down migrations: rollback requires restoring the pre-upgrade
backup and compatible binary. PostgreSQL differences remain explicit future work.
No released tags exist; this proposal changes no historical release documentation.

Self-review: serialization inside one process is deliberately simple; transactional
locking and constraints still protect multiple local handles. Archiving every
delete favors durable history over name reuse. Empty action catalog limits early
utility but preserves approved phase boundaries. The action-name correction is a
change to the earlier draft was accepted before changing parser behavior. Additional
implementation resource guards bound listings/history to 10,000 records/64 MiB
with explicit errors, without truncation; exact revision reads remain available.
