# Phase 03 serial reliability core self-review

Date: 2026-11-08 (Asia/Jakarta). Base: Phase 02 merge `e7d7177`. The user
authorized Phase 03 and accepted the four [execution decisions](../designs/reliable-execution.md)
with "ACCEPT". This review covers the first serial unit, not completion of Phase
03 or an independent reviewer. Exact executed checks belong in the [build log](../build-log.md).

## Implementation and evidence

- The condition parser evaluates its existing bounded grammar and preserves full
  validation while short-circuiting unavailable predicates. `TestEvaluateCondition`
  and `FuzzEvaluateCondition` exercise precedence, terminal outcomes, short-circuit
  behavior and malformed input. No arbitrary evaluator or new DSL grammar was added.
- `ResolveInputs` preserves structured scalar/map/list types, copies values,
  rejects missing/unsuccessful/invalid outputs, checks cancellation and enforces
  depth/node/content/decode bounds. `TestResolveInputsPreservesTypesAndCopies`,
  `TestResolveInputsRejectsUnavailableAndBounds` and `FuzzResolveOutput` cover this.
  Catalog validation still rejects sensitive references and inline credentials.
- Format-2 policy operations validate dependencies, condition skips/failures,
  individual/whole cancellation, aggregate outcomes, deadlines and monotonic attempt
  numbers without reopening terminal states. Errors leave snapshots unchanged.
  `TestReliableAttemptsAndBackoff`, `TestReliableInvalidOperationsAreAtomic`,
  `TestReliableConditionsDependenciesAndAggregate`, `TestRetryDelayBounds` and
  cancellation/deadline tests cover boundaries and overflow-safe capped retry.
- The serial coordinator persists attempt starts before invocation, revalidates
  resolved inputs, captures only validated/redacted successful outputs and stops
  on infrastructure failure. One owned cancellation watcher uses bounded control
  metadata; per-step contexts/timers and every owned goroutine are cancelled/joined.
  Backoff holds no SQL lock or action-worker goroutine.
- `TestReliableCoordinatorRetriesReferencesAndConditions` executes retry success,
  typed reference resolution and conditional skip end to end.
  `TestReliableCoordinatorExhaustionAndFailureHandler` verifies handlers do not
  erase failures; independent-root/all-skipped tests verify aggregate semantics.
  Missing output fails before a dependent invocation. Step deadline includes
  backoff, execution deadline propagates, and caller cancellation is durable.
- `TestReliableCoordinatorDurableCancellationDuringBackoff` synchronizes through
  an injected wait and cancels execution/individual step without arbitrary sleeps.
  `TestReliableCoordinatorCancelsMaximumGraphWithinCleanupBudget` verifies all
  256 steps terminate with one invocation. Pending cancellation is batched in a
  transaction rather than requiring 255 separate durable commits.
- Change conflicts are retried only after re-reading the same owner/version and
  only for bounded metadata updates; actions are never replayed by that retry.
  `TestReliableCoordinatorPersistenceAndAmbiguousFailures` injects errors before/
  after invocation and a commit-then-error boundary. Control failure cancels the
  action, reports infrastructure failure and leaves uncertainty visible.
- Seven-day hashed admission metadata is atomic with creation. Concurrent identical
  requests have one winner and matching receipts; different fingerprints conflict.
  Receipt waiting is bounded by caller/original deadline and cannot take ownership.
  Expiry tests preserve original history. No plaintext key enters results or logs.
- Retry creates a new parent-linked execution of the exact original revision and
  bindings. `TestReliableCoordinatorReplayPinsRevision` and storage/CLI replay tests
  verify updates cannot switch it to current behavior. Uncertain metadata requires
  explicit acknowledgement; archived or succeeded sources are rejected.
- Migration 003 adds policies/attempts/admissions and rebuilds constrained events
  atomically. Migrations 001/002 are unchanged. `TestMigrationThreePreservesLegacyHistory`
  compares actual version-2 history after upgrade; conflict rollback restores the
  original 1,024-event schema/immutability triggers and migration ledger.
  Terminal attempts and events remain immutable. Old/new format limits are distinct.
- Actual SQL event-failure injection rolls back snapshots, attempts, admission
  records and history together. Stale ownership/version, concurrent admissions,
  cancellation acknowledgements, history reopen and admission expiry are tested
  through separate local handles. No SQL adapter dependency enters domain/engine.
- CLI reliable preflight is explicit; the legacy preflight/runner remains isolated
  for format-1 compatibility regression tests. Production composition uses the
  format-2 repository/coordinator. `TestReliableCLIReplayAndIdempotency` and
  `TestReliableCLICancellationRequestAndOwnedShutdown` verify text/JSON, stable IDs,
  durable acknowledgements, flags, output failures and terminal-target status.
- New policy/evaluation/history benchmarks extend CI to thirteen required families.
  ManualExecution now measures the production format-2 path; old-format history
  benchmarks remain compatibility workloads. No global-worker throughput or
  scheduler performance claim is made.

## Security and compatibility conclusions

No shipped fixture action, management listener, third-party evaluator, new module
dependency, minimum Go change, release tag or live deployment is introduced.
Raw action errors/inputs, secret references and plaintext admission keys stay out
of ordinary persistence/logs. Output capture and read bounds remain enforced.
Cancellation is a request until owned work ends; an output/persistence failure
does not undo effects or turn them into successful results. Existing terminal
history is read-only, including terminal cancellation acknowledgements.

See the [reliability guide](../../docs/reliability.md) for commands, context budgets,
replay/expiry semantics and schema-3 backup/rollback. The production action catalog
is still empty. Old unmanaged running work is not taken over, and automatic
recovery is deliberately absent; audit classification invokes no action.

Remaining required contract review: [bounded single-node dispatcher](../designs/single-node-dispatcher.md).
Parallelism/concurrency above one, durable queue/claim ownership and automatic
recovery cannot be activated before that contract and its tests are implemented.
Scheduler/trigger/listener/security and retention contracts remain later Phase 03
work. Applicable CI/CodeQL and phase acceptance gates remain open; no requirement
is completed just because these source files exist.
