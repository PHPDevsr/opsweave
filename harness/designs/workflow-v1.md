# Workflow v1 contract proposal

Status: `ACCEPTED_FOR_IMPLEMENTATION`. Prepared and accepted 2026-11-08
(Asia/Jakarta), user reply "setuju" to the four review decisions. Covers GOALS §6
(`G-6-001`–`G-6-016`). Structural candidate parsing is implemented; execution
semantics remain future work, not a released specification. A proposed correction
to action segment identifiers was accepted in the [foundation contract](foundation-storage-cli.md)
and is implemented below; other identifier rules are unchanged.

## Review decisions

The following choices were accepted before parser implementation:

1. Explicit dependencies form a DAG. Omitting `needs` means the preceding step;
   `needs: []` makes an independent root. Conditions never add implicit edges.
2. Conditions use a small boolean grammar. Inputs use explicit reference objects;
   strings never interpolate or execute code.
3. `retry.attempts` counts total attempts, including the first. Retries use bounded
   exponential backoff without jitter in v1. Failure handlers do not erase failure.
4. Unsupported fields and capabilities are errors. Phase 01 delivers definition
   management and planning, with no execution or built-in action implementation.
   A production action catalog must exist before a workflow using it can be
   registered; test catalogs do not imply shipped actions.

## Document shape and parsing limits

One UTF-8 YAML document defines one workflow. Accept a leading document marker
and UTF-8 BOM; reject invalid UTF-8, trailing documents, duplicate mapping keys,
non-string mapping keys, aliases, anchors, merge keys, explicit tags, unknown
fields and explicit nulls. Only JSON-compatible booleans, strings, signed 64-bit
integers, finite floating-point numbers, arrays and mappings are accepted in
literal action inputs. Dates are strings; no implicit timestamp conversion.

Read at most 1 MiB per definition, nesting depth 32, 20,000 YAML nodes, 256 steps,
256 labels, 64 dependencies per step and 4 KiB per expression. Check limits
before expensive traversal; reject limit violations with bounded diagnostics.
Directories contain individual `.yaml`/`.yml` files in lexical path order;
do not follow symlinks or recurse into subdirectories. A batch containing a
duplicate workflow name or invalid file fails preflight before registry writes.

| Field                  | Type and rule                                                                                  |
| ---------------------- | ---------------------------------------------------------------------------------------------- |
| `apiVersion`           | Required string, exactly `opsweave.io/v1`                                                      |
| `kind`                 | Required string, exactly `Workflow`                                                            |
| `metadata`             | Required mapping with only the fields below                                                    |
| `metadata.name`        | Required identifier, 1–63 ASCII characters, `[a-z][a-z0-9]*(-[a-z0-9]+)*`                      |
| `metadata.description` | Optional string, at most 4,096 UTF-8 bytes                                                     |
| `metadata.labels`      | Optional string-to-string mapping; keys use identifier grammar, values at most 256 UTF-8 bytes |
| `spec`                 | Required mapping with only the fields below                                                    |
| `spec.steps`           | Required nonempty ordered list of step mappings                                                |
| `spec.timeout`         | Optional positive duration; default 1 hour, maximum 24 hours                                   |
| `spec.concurrency`     | Optional mapping containing only `limit`; integer 1–256, default 1                             |
| `spec.parallelism`     | Optional integer 1–256, default 1; active steps per execution                                  |
| `spec.triggers`        | Optional list; reserved for Phase 03 trigger contracts, rejected in Phase 01                   |

Workflow enablement is registry state, defaults enabled, and is not revision
content. A disabled workflow rejects automatic triggers; explicit manual runs
remain permitted once manual execution is implemented. Archived workflows reject
all new executions. Re-enabling never edits historical revisions.

| Step field | Type and rule                                                                                                                              |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `id`       | Required identifier using the workflow-name grammar; unique in the workflow                                                                |
| `uses`     | Required action identifier, exactly two segments separated by `.`; each is 1–63 ASCII characters matching `[a-z][a-z0-9]*([_-][a-z0-9]+)*` |
| `with`     | Optional mapping, defaults empty; validated by the selected action descriptor                                                              |
| `needs`    | Optional unique list of other step IDs; no self references or cycles                                                                       |
| `if`       | Optional condition string using the grammar below                                                                                          |
| `timeout`  | Optional positive duration; default 5 minutes, maximum 24 hours                                                                            |
| `retry`    | Optional mapping containing only `attempts`, `backoff`, `maxBackoff`                                                                       |

Durations are decimal integers followed by `ms`, `s`, `m` or `h`, without spaces,
signs, fractions or compound units. Range is 1 ms–24 h. Reject overflows before
multiplication. Configuration may impose stricter deployment limits; violations
fail validation rather than silently clamping supplied values.

## Dependencies, conditions and failure

Step order supplies deterministic planning and readiness tie-breaking. A step
without `needs` depends on the previous step; the first step defaults to no
dependencies. Forward references are allowed when the resulting graph is acyclic.
An explicit empty list permits parallel roots when parallelism allows them.

All dependencies must be terminal before a step is eligible. With no `if`, all
dependencies must have succeeded; otherwise persist the step as skipped with a
dependency reason. With an explicit condition, evaluate after dependencies become
terminal, even if a dependency failed. Every referenced step must be a transitive
dependency. False means skipped; an evaluation error fails the step without
invoking its action. Skipped never means succeeded.

```text
condition = orExpr
orExpr    = andExpr { "||" andExpr }
andExpr   = unary { "&&" unary }
unary     = "!" unary | "(" condition ")" | "true" | "false" | predicate
predicate = "steps." identifier "." outcome
outcome   = "success" | "failed" | "skipped" | "cancelled" | "timed_out"
```

Allow ASCII spaces, tabs and newlines between tokens. Tokens and identifiers are
case sensitive; keywords must consume complete tokens. Precedence is `!`, `&&`,
then `||`, with left-to-right short circuit evaluation. Maximum parenthesis/unary
nesting is 32 and maximum token count 1,024. No comparisons, function calls,
arithmetic, variable assignment, environment lookup or arbitrary code evaluation.
`success` means terminal succeeded, and `failed` means terminal failed after retry
exhaustion. Timeout and cancellation each have their own predicate.

Independent branches may finish after another branch fails. At completion, any
failed or timed-out step makes the execution failed, including a step whose
failure handler succeeded. Execution cancellation or execution deadline takes
precedence and produces cancelled or timed_out respectively; simultaneous cases
use the first accepted persisted terminal transition. No new action starts after
execution cancellation/deadline. Cancel or skip unfinished steps with explicit
reasons; validate and persist each transition. Storage failures are engine errors,
never successful execution results. The detailed state machine is Phase 02 work.

## Inputs, outputs and secrets

Literal strings remain literal, including `${...}`. A mapping containing `$ref`
is exclusively a reference object and must contain exactly that one string field.
References can appear at any input value position. Action input schemas validate
the resolved value's type; unresolved values cannot bypass schema validation.

```yaml
with:
  response:
    $ref: steps.fetch.outputs.body
  token:
    $ref: secrets.production.api-token
```

Output references have grammar `steps.<id>.outputs.<key>`, with a single key
using the identifier grammar. Nested selectors, indexing, fallback coercion and
string substitution are rejected. The source step must be a transitive dependency
and the key must exist in its action output descriptor. Outputs are structured
values from the source's final successful attempt. A source without successful
output or a missing required output causes a reference-resolution step failure;
it never becomes an empty string. Redact sensitive output fields and keep them
out of ordinary persisted outputs according to the action contract.

Secret references have grammar `secrets.<environment>.<name>`, with both segments
using identifier grammar. Resolve at action invocation through authorized secret
access, never at parse, plan, export or registry time. Missing or denied access
fails without including the value. Revision storage contains references only.
Secret-derived values must not enter ordinary output persistence, errors or logs.
Secret references are rejected until Phase 05 implements encrypted storage,
authorization and redaction; this proposal does not introduce plaintext secrets.

Action inputs declared as credentials by their descriptor require secret
references; reject inline credential literals before registry persistence. They
therefore remain unavailable until Phase 05. Sensitive output values must not be
used to bypass this rule. Source preservation is not permission to persist secret
values in workflow YAML.

## Retry, timeout, concurrency and idempotency

`retry.attempts` is an integer 1–10, default 1. `backoff` defaults to 1 second;
`maxBackoff` defaults to 1 minute and must be at least `backoff`. For retry number
`n` starting at 1, delay is `min(backoff * 2^(n-1), maxBackoff)`, computed without
overflow. Retry action failures only; validation, missing references, cancellation,
deadline expiry, authorization denials and persistence errors are not retryable.
Record each attempt durably. Exhaustion persists terminal step failure.

The execution timeout starts at entry to running, excluding queue wait. The step
timeout starts before input resolution for the first attempt and includes retries
and backoff. Use the earlier of step and execution deadlines; propagate context
cancellation into actions and backoff. Recovery preserves the original absolute
deadline and consumed attempts. Never reset a deadline to extend execution.

`concurrency.limit` bounds simultaneous running executions across revisions of
the same workflow in a single-node deployment. `parallelism` bounds active steps
within each execution. Deployment worker/action limits may further reduce active
work. Queueing and job ownership must be bounded and durable before enabling
concurrent execution; Phase 01 planning starts no goroutines or jobs. Distributed
enforcement is Phase 08 work, not a SQLite distributed guarantee.

Idempotency is an execution submission option, not an arbitrary expression or
promise of exactly-once effects. Its storage and replay contract belongs to
Phase 03. Ambiguous in-flight external effects cannot be silently retried after
restart; recovery must surface them for an explicit safe decision unless the
action provides a verified idempotency mechanism.

## Action compatibility and phase capability gates

Action descriptors declare an identifier, compatible workflow API version,
implementation version, input/output schemas, sensitive fields, permissions and
capabilities. These are metadata requirements for the Phase 02 action contract,
not permission to implement Phase 04 integrations early. The registry binds a
revision to the validated action descriptor versions separately from the authored
YAML; execution fails preflight if compatible descriptors are unavailable.
Never silently replace a missing action with a stub or change an execution's
bound implementation during recovery.

Structural parsing produces a candidate definition, not authorization to register
or execute. Registration and planning additionally require an injected action
catalog and supported-capability set. Unknown actions, unavailable descriptors,
incompatible action schemas and unsupported capabilities are explicit errors.
Phase 01 production has no shipped built-in actions; only isolated acceptance
tests may inject fixture descriptors. This limitation must be visible in CLI help
and documentation until actions ship. The GOALS example remains illustrative.

| Capability                                                                                              | Earliest runtime phase     | Phase 01 treatment                                                    |
| ------------------------------------------------------------------------------------------------------- | -------------------------- | --------------------------------------------------------------------- |
| Definition metadata, DAG and literal input schema validation                                            | 01                         | Parse, validate, register and plan with a supplied compatible catalog |
| Manual execution, default sequential behavior and output capture                                        | 02                         | No run command; no action invocation                                  |
| Explicit `if`, output references, non-default timeout, retry or concurrency policies, independent roots | 03                         | Reject as unsupported capabilities at registration/planning           |
| `spec.triggers`                                                                                         | 03; filesystem triggers 09 | Reject; exact trigger variants require their phase contract review    |
| Built-in action implementations                                                                         | 04; database actions 09    | Reject identifiers absent from the supplied catalog                   |
| Secret references                                                                                       | 05                         | Reject at registration/planning                                       |

An explicitly supplied default policy is permitted if it is identical to the
omitted default; it cannot enable behavior before its runtime phase. An `if` or
reference is capability-gated even if it appears trivially resolvable. Accepted
definitions are not executable until an engine supports all required capabilities.

## Revisions, planning and compatibility

Registry operations preserve the exact validated source bytes and a normalized
semantic representation with a versioned normalization format. Each accepted
apply creates a distinct immutable revision identity in an atomic transaction;
do not use source hashes as revision identities. Identical applies can create new
revisions. Executions bind the revision and action versions at submission.
Exports return the original definition; registry identifiers and enable/archive
state are separate metadata. Deletion archives workflows with referenced history;
never cascade-delete revisions needed by executions. Atomic revision publication,
concurrent updates and migration details are Phase 01 task 2 design work.

Planning returns workflow/revision identity when available, normalized dependencies,
stable topological order, required capabilities and action versions. It validates
all branches but does not evaluate runtime conditions, resolve secrets, invoke
actions, contact action destinations or create executions. Unavailable outputs
stay unresolved references. Standalone planning writes no durable state.

Only explicitly supported API versions are accepted. Published `opsweave.io/v1`
behavior remains compatible; breaking semantics require a new API version and
migration instructions. Additive capabilities still require explicit support and
unknown-field rejection. No automatic rewrite of historical revisions or released
documentation. No tags exist at preparation time; this draft creates no release
compatibility promise.

## Proposed acceptance cases and review findings

Implementation must add table-driven tests for every field/type/limit, unknown and
duplicate keys, nulls, aliases/tags, invalid encodings, multiple documents,
durations/overflow, missing and cyclic dependencies, reference scope, grammar
precedence, short circuiting, action schema compatibility and capability gates.
Bound YAML/expression/validator fuzz runs and assert no panic or unbounded work.
Measure parser coverage against the required 80% target. Later engine phases test
the semantics above with deterministic clocks and action doubles, including failed
handlers, skipped dependencies, retries, deadline preservation and cancellation.

Registry/CLI tests must prove rejected batches cause no writes, export/import
preserves meaning, revisions remain immutable across updates, errors omit action
input values and planning has no side effects. Test catalogs must never leak into
production command wiring. Integration tests use isolated temporary databases.

Design self-review: explicit dependencies remove ambiguous condition scheduling;
reference objects avoid template injection; bounded grammars and YAML limits bound
untrusted input; capability gates avoid advertising later features. The deliberate
trade-off is that Phase 01 cannot register real integrations until their action
descriptors ship. Error messages should identify safe field paths, coordinates and
codes without echoing input scalars or raw YAML parser diagnostics.

Open gates: mandatory product verification and remaining Phase 01 integration
acceptance. See [parser documentation](../../docs/workflow-parsing.md) for the
implemented subset. No executor, security guarantee or GOALS requirement is marked
completed by this design acceptance.
