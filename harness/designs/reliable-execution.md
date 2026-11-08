# Phase 03 reliable execution proposal

Status: `ACCEPTED_FOR_IMPLEMENTATION`, 2026-11-08 (Asia/Jakarta). The user replied
"ACCEPT" to the four decisions below after authorizing Phase 03. The first serial
reliability unit implements these execution policies; bounded dispatcher ownership,
parallel capacity and automatic recovery remain gated by the next
[single-node dispatcher proposal](single-node-dispatcher.md). See the
[phase plan](../phases/phase-03.md) and [implementation review](../reviews/phase-03-core.md).

The [workflow v1 contract](workflow-v1.md) already defines condition syntax,
dependency order, retry math and deadline scope. The [manual execution contract](manual-execution.md)
defines existing durable states. Preserve those rules except for the explicit
Phase 03 extensions below. Contract acceptance alone does not complete a requirement.

## Four accepted decisions

1. Keep a step running across its bounded retry attempts and backoff. Persist a
   separate append-only attempt history before every action invocation. Extend
   aggregate completion to support conditional skips and individual step
   cancellation; never reopen a terminal step or execution. New execution format
   2 permits 16,384 bounded lifecycle events rather than Phase 02's 1,024, so the
   supported 256-step/10-attempt limits cannot exhaust normal history capacity.
2. Recover queued/unstarted work and durable backoff without resetting deadlines
   or attempts. An action whose outcome was not durably recorded before a crash
   is uncertain: fail that execution with a safe `uncertain_effect` classification,
   retain all history, and never automatically repeat the action. Resubmission
   after such a failure requires an explicit acknowledgement of duplicate-effect
   risk. No action currently provides a verified recovery-idempotency mechanism.
3. `execution retry` creates a new execution of the exact original revision and
   action versions, replaying the whole workflow. It does not patch old history or
   resume at an arbitrary failed step. Submission idempotency keys deduplicate
   execution creation for seven days; they do not make external effects exactly
   once. Historical archived workflows remain unavailable for new execution.
4. Cancellation is a durable request, acknowledged separately from terminal
   completion. Whole-execution cancellation stops new work and reaches every
   owned action/backoff; step cancellation targets only that step. Terminal
   state is committed after owned work has stopped. Parallel execution and
   non-default concurrency remain unavailable until bounded worker capacity and
   atomic claims are implemented and verified together.

## Dependencies and conditions

Use the existing deterministic topological order, with authored order for ready
ties. A step becomes eligible only after all dependencies are terminal. With no
condition, every dependency must have succeeded; otherwise persist skipped with
`dependency_failed`. Evaluate an explicit condition against durable terminal
outcomes using the reviewed grammar and short-circuit rules. False produces
skipped with `condition_false`. An evaluation error fails the step without
invoking its action. Referenced steps must remain transitive dependencies.

Only successful final outputs can satisfy output references. Resolve recursively
into independent values and validate the resolved input schema before invocation.
Missing output, an unsuccessful source or wrong type fails the step with a safe
classification, never an empty-string substitute. Sensitive output references
and all secret references remain rejected. No new expression language, event
payload reference, environment interpolation or arbitrary code evaluation is added.

The coordinator has a bounded ready set of at most 256 steps. Active steps include
input resolution, action attempts and their retry backoff; they consume a workflow
parallelism slot until terminal. Backoff releases an action-worker slot and uses
the injected clock/due-work mechanism rather than one sleeping goroutine per retry.
Do not start a dependency early, hold SQL locks during actions, or silently abandon
owned goroutines when a slot or context is cancelled.

## State and attempt contract

Execution states remain pending, queued, running, succeeded, failed, cancelled and
timed_out. Step states remain pending, running, succeeded, failed, skipped,
cancelled and timed_out. Add only this step-state edge to the reviewed table:
`pending -> failed` for a condition-evaluation failure before action invocation.
All other existing edges and terminal immutability remain enforced. A false
condition uses the existing pending-to-skipped edge; there is no running-to-skipped
shortcut and no failed-to-running retry transition.

Once an eligible step starts, persist its start time and absolute deadline before
input resolution. A validation/reference failure can terminate it without an
action attempt. Each actual attempt has an increasing number, start/finish times,
safe result class and next retry time where applicable. Persist attempt-start
before invocation and attempt-finish together with the step's next durable state.
Do not persist action error strings, failed output payloads or resolved inputs.

An action failure can finish an attempt while the step remains running and waits
for its next due attempt. Retry only the action-failure category already allowed
by the accepted workflow contract. Input/reference validation, permission denial,
invalid/oversized output, cancellation, step/execution deadline and infrastructure
failure are non-retryable. No attempt number is reused. Exhaustion produces failed.
The existing attempts range 1–10 and overflow-safe capped exponential backoff are
unchanged; no implicit extra attempt or jitter is introduced.

Aggregate outcomes, after all owned work is terminal:

| Situation                                                    | Execution outcome                                         |
| ------------------------------------------------------------ | --------------------------------------------------------- |
| Whole-execution cancellation/deadline accepted               | cancelled/timed_out; first persisted terminal result wins |
| Any failed or timed-out step                                 | failed, even if a failure handler succeeded               |
| No failed/timed-out step, but an individually cancelled step | cancelled, even if its cancellation handler succeeded     |
| Every step succeeded or was legitimately skipped             | succeeded, including an all-false conditional workflow    |

Skipped never becomes step success. An explicit condition may handle a cancelled
dependency; an ordinary dependent is skipped. Independent branches may finish after
a step failure or individual cancellation within configured limits. Whole-execution
cancellation or deadline prevents any new action start.

## Deadlines and cancellation

Execution timeout starts at entry into running, excluding queue wait. Step timeout
starts before the first input resolution and includes attempts/backoff. Persist
absolute UTC deadlines and enforce the earlier parent/step deadline. Recovery
does not extend either deadline. Clock/timer dependencies are injected for tests;
contexts carry cancellation across engine, worker, action and repository boundaries.

`execution cancel <id> [--step <step-id>] [--json]` records an idempotent request.
For queued work, cancellation and claim compete in one transaction: no action
starts if cancellation wins. For running work, return a safe acknowledgement
containing `cancellation_requested` and current identifying metadata. This is not
a claim that the execution is already terminal; callers inspect get/logs for the
durable result. Repeated requests do not append unlimited duplicate events.
Unknown IDs fail; already-terminal targets return their unchanged state.

The coordinator observes cancellation before ready selection, attempt creation,
invocation and retry waits, and signals active contexts. It waits for owned work
before committing final state or releasing capacity. Handlers must cooperate;
there is no detached goroutine workaround for uncooperative Go code. Shutdown and
cleanup have bounded contexts and report failures; an uncertain durable outcome
is preserved visibly rather than reported as terminal success.

## Recovery and explicit replay

Recovery requires exclusive, fenced single-node dispatcher ownership before any
action can start. The bounded worker/claim design in the next phase task must
provide that prerequisite; a stale row or lock-file presence alone is insufficient.
No distributed ownership, PostgreSQL lease or cross-host SQLite guarantee is added.

Classify persisted work without changing historical successful steps:

| Durable evidence at restart                                        | Policy                                                                                       |
| ------------------------------------------------------------------ | -------------------------------------------------------------------------------------------- |
| Queued execution, no action started                                | Claim only after validating current availability, ownership and capacity                     |
| Pending step or due backoff with a durably failed finished attempt | Continue from the stored attempt/deadline after safe claim                                   |
| Deadline already elapsed                                           | Persist timeout/cancellation of unfinished work without another invocation                   |
| An attempt was running and no result committed                     | Record uncertain_effect and fail the execution; do not automatically invoke any further step |
| Terminal execution                                                 | Read-only history; never restart it                                                          |

If any attempt is uncertain, terminalize the execution conservatively: fail
uncertain running steps, explicitly skip dependents, and cancel other unfinished
work with `recovery_aborted`. Previously terminal steps/outputs remain unchanged.
Legacy Phase 02 running actions without an attempt ledger are also uncertain;
do not invent proof that they were safe to repeat. Ambiguous commits require
re-reading durable state and ownership before a decision, never automatic replay.

`execution retry <id> [--ack-uncertain] [--json]` accepts only failed, cancelled or
timed-out originals. It creates a new ID with a parent-execution link, original
immutable revision and exact action bindings, with fresh policy deadlines and
attempt history. The original is untouched. All steps run again, including earlier
successful ones. Reject missing/incompatible action versions, archived workflows
and unsupported capabilities. Require `--ack-uncertain` if the source contains an
uncertain effect; retain that acknowledgement in safe audit metadata. It does not
prove external effects are safe or idempotent. Use ordinary run for a current
revision; retry never silently switches versions. Replaying succeeded executions
is not exposed as a hidden flag in this first work unit.

## Submission idempotency

Add `run --idempotency-key <key>` and the same option to explicit retry. Accept
1–128 printable ASCII characters without whitespace/control characters, hash the
key before persistence, and never log or return its plaintext. This key is for
execution admission; it is not passed as an action credential or expression.

Scope uniqueness to workflow plus operation; retry additionally scopes to the
original execution ID. The fingerprint includes the pinned revision, action
bindings and replay acknowledgement. The same key/fingerprint returns the original
execution ID and its current result rather than creating work. A different
fingerprint is a conflict. Publication of the admission record and execution is
atomic; parallel duplicate requests have exactly one durable execution winner.

Deduplication lasts seven days from first admission and is not extended on reads.
After expiry, reuse may create a new execution; document this explicitly. Retain
the admission tombstone until expiry even if an independently approved history
retention policy removes the execution payload, returning an expired-history
result rather than silently recreating it. General retention defaults and trigger
event deduplication need the later storage/trigger contract before exposure.

## Persistence and compatibility

Use a new additive/checksummed migration, preserving migration 001/002 contents.
Add attempt history, absolute deadlines, cancellation requests, admission records,
parent linkage and execution format/event limits. Rebuild constrained event storage
transactionally where necessary, preserving every existing event and its sequence.
Keep old terminal snapshots immutable and readable. Upgrade old nonterminal work
only through the explicit recovery classification, not by guessing missing attempts.

Format 1 keeps its 1,024-event limit; new format 2 permits 16,384 events, each
bounded to 1 KiB with engine-owned codes/identifiers. This accommodates the worst
supported retry graph plus cancellation metadata without unbounded log capture.
The output limits remain 64 KiB per step and 1 MiB per execution; attempt failures
store no output. Existing 64 MiB response and bounded-list limits remain in force.
Do not silently truncate structured history or accept unknown formats.

All claims, attempts, results, cancellation acknowledgements and admission records
use transactions plus expected state/version/owner checks. SQL is confined to the
adapter; policy remains in domain/engine packages. Preserve existing SQLite
durability settings and rollback errors. Back up the stopped database before
upgrade; rollback requires the previous backup and compatible binary, without
down-migrating history in place. No release tags exist at this design baseline.

## Implementation order and acceptance

1. Extend the domain/attempt contract, safe classifications and migrations with
   exhaustive state and upgrade/rollback tests. Build on current packages rather
   than replacing the architecture.
2. Add bounded condition evaluation/output resolution and retry/deadline policy
   tests using deterministic actions and clocks. Test missing outputs, schema
   mismatch, invalid conditions, retries, exhaustion and cancellation at every
   boundary. Keep the production action catalog empty until Phase 04.
3. Integrate policy execution with bounded worker capacity and atomic ownership;
   enable DAG parallelism/non-default concurrency only when those guarantees are
   implemented together. Test cancellation/shutdown without lost owners, duplicate
   claims or goroutine leaks, including multiple local handles and restart crashes.
4. Wire cancel/retry/idempotency commands and document acknowledgement, whole-run
   replay and seven-day deduplication semantics. Verify JSON/text/output failures,
   exact revision/action binding and uncertain-effect acknowledgement.

Run all mandatory Go checks, core coverage at least 80%, race/integration/E2E and
bounded parser/input fuzzing. Add policy/parallelism/retry and durable claim
benchmarks to existing CI before making performance claims. Verify old history,
capture redaction and output bounds under retries and simultaneous completion.
No requirement becomes implemented/completed from this proposal alone.

Scheduler/cron/DST/missed-run behavior, startup/internal/webhook trigger variants,
listener authentication/configuration, queue ownership/resource settings and
general retention are subsequent Phase 03 work units. They require their concrete
contract/security review before exposure, as the phase plan specifies. This
proposal does not finalize an unreviewed trigger YAML shape or management API.
Phases 04–10 remain unapproved except the previously authorized release packaging
extension; no new release, deployment or background automation is started.
