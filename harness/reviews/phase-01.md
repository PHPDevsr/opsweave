# Phase 01 implementation self-review

Date: 2026-11-08 (Asia/Jakarta). Baseline commit: `d7b89f6`. Scope: approved
workflow/storage/configuration/CLI contracts, their implementation, tests,
documentation and migration CI path coverage. This is a manual self-review with
automated verification, not an independent reviewer or a remote CI result.

## Result

Implementation is prepared in [draft PR #3](https://github.com/PHPDevsr/opsweave/pull/3)
targeting `main`. Remote CI/Markdown passed on `f10f596`, including both explicit
Linux symlink tests, all six OS/Go matrix jobs and the race job. CodeQL result
upload is blocked by missing repository Advanced Security enablement. Phase 01
remains incomplete pending that owner decision and phase review; phases 02–10
remain unapproved.

## Reviewed behavior and evidence

- Workflow parsing audits bounded YAML before decoding typed values. Duplicate
  keys, aliases, tags, unknown fields, numeric overflow, invalid graph edges and
  reference scope fail with safe diagnostics. `TestParseRejectsInvalidDefinitions`,
  `TestCollectionBoundaries`, `TestStrictLoad` and parser/condition fuzzing cover
  these boundaries. The accepted underscore correction permits planned action
  identifiers without changing step/workflow identifier rules.
- `registry.Prepare` validates every source and injected action descriptor before
  persistence. Required/unknown inputs, output schemas and credential rules cannot
  be bypassed by symbolic references. Nested sensitive outputs are rejected.
  `TestPrepareCatalogValidation`, `TestCredentialsAndDescriptors`,
  `TestSensitiveOutputReferences` and `FuzzPrepare` cover this. The production
  command tree always supplies an empty catalog; no fixture runner is exposed.
- Source bytes and normalized version-1 JSON are stored separately. Batch
  publication, revision allocation and lifecycle updates use immediate
  transactions. Composite foreign keys bind current revisions to their owner;
  append-only SQL triggers protect historical definitions. Tests cover restart,
  exact historical reads, unique concurrent revisions, rollback after injected
  insert failure, wrong-owner references and archive name reservation.
- Migrations and checksums commit atomically. Unknown schema history/checksum
  mismatches and corrupt normalized data fail visibly. A migration conflict
  preserves existing data and rolls back the ledger. A stopped-process copy
  restores original revision/source data in isolated resources.
- Configuration uses strict shared YAML auditing and validated environment
  overrides. No values appear in ordinary configuration/storage errors. New
  database files/directories are private where permission bits are supported;
  Windows ACL limits are documented. Database options are adapter-owned and
  network/URI/DSN input is rejected.
- All product queries parameterize external values; migrations and pragmas are
  fixed application text. No new shell action, HTTP request, trigger, worker or
  executor exists. Contexts are passed through file preflight and database calls,
  never stored in the registry/store. Database pools have one connection and are
  closed on success and failure. Owned test goroutines use bounded channels and
  explicit join/cancellation.
- Input files/batches, YAML complexity, expressions and returned revision payloads
  have explicit bounds. Directory enumeration uses bounded chunks. Result-limit
  errors return no partial list. `TestListingLimitIsReportedWithoutPartialResult`
  verifies the read-limit failure path.
- CLI text/JSON, usage, help/completion, original-source export, import round trips
  and output failures are tested. Output failure after commit preserves durable
  state and reports failure. Debug output uses structured logging, includes command
  context only, and propagates short/failed writes before storage access.
- Existing CLI behavior and tests were retained. Added migration path filters
  ensure SQL-only changes trigger Go CI. `actionlint` passed; remote CI on `f10f596` passed across Linux/Windows/macOS,
  minimum/stable Go, modules, formatting and race testing. No minimum Go version increase, extra runtime
  service, release tag, deployment or historical documentation rewrite occurred.

## Open acceptance gate

The Windows symlink limitation is resolved by actual Linux evidence in
[CI run 42437553488](https://github.com/PHPDevsr/opsweave/actions/runs/42437553488).
The explicit Linux step logged `--- PASS` for both
`TestInitRefusesSymlinkAndConfigDirectory` and `TestReadSourcesRejectsSymlinks`.
Only Windows may skip setup failures; Linux/macOS fail if symlink creation fails.

[CodeQL run 42437553495](https://github.com/PHPDevsr/opsweave/actions/runs/42437553495)
failed result upload with the repository configuration error that Advanced
Security must be enabled. No upload enforcement, failure handling or security
setting was weakened. An owner decision is required; the PR stays draft and phase
02 still needs separate approval after phase review.

## Verification notes

See the append-only [build log](../build-log.md) for exact command results,
coverage, fuzz counts, dependency checks and portable compiler provenance. Local
race testing passed outside the sandbox after compiler verification; the sandbox
itself prevented ThreadSanitizer address-space allocation. Earlier simultaneous
heavy unit/coverage/race/fuzz runs exceeded SQLite's configured busy timeout;
sequential unit, coverage and race commands passed without relaxing timeouts or
assertions. These results do not promise that every contended write must succeed
beyond the documented five-second timeout.

## Benchmark extension review

Added the user-requested [foundation benchmarks](../../docs/benchmarks.md) without
product changes. Parser/preflight fixtures are generated before timing; Go
`b.Loop` retains measured results without shared mutable sinks. SQLite write
fixtures close/reset only their own temporary file, cap each history at 128 total
revisions and measure migrations separately. Production synchronous FULL and
foreign-key settings stay enabled. Errors/incomplete results fail benchmarks, and
a forced nine-iteration Batch16 run exercises the reset boundary. CI uses read-only
repository permissions, pinned actions, pipe failure propagation and explicit
benchmark family checks. Only timing/allocation and selected environment reports
are uploaded; no database or secret-bearing files are included. Hosted-runner
numbers are observations, without an arbitrary absolute timing threshold.
