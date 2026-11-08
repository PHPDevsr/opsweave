# Workflow parsing in development

The internal `workflow` package implements bounded parsing of the
[accepted workflow v1 design](../harness/designs/workflow-v1.md). The
[foundation CLI and registry](foundation.md) add catalog/schema preflight and
persistence. The production catalog is empty and no executor ships yet. This
document describes the parser; the design also covers future execution behavior.

`workflow.Parse(ctx, source)` returns a candidate definition with normalized
defaults, validated dependency/reference structure, stable topological order and
an independent copy of the original source. It does not invoke actions, perform
network requests, read secrets or persist anything. `Source()` preserves comments,
BOM and document markers for future lossless export.

The following example is structurally valid for parser development. `test.echo`
is a fictional identifier, not a shipped action or supported integration:

```yaml
apiVersion: opsweave.io/v1
kind: Workflow
metadata:
  name: parsing-example
  description: Structural parsing example only
  labels:
    team: operations
spec:
  steps:
    - id: first
      uses: test.echo
      with:
        message: Hello
    - id: second
      uses: test.echo
```

The second step depends on the first by default. Explicit `needs: []` creates an
independent root. Unknown/missing dependencies, duplicate IDs, cycles and
references outside transitive dependencies are rejected. Conditions are checked
against the boolean grammar; they are not evaluated by this package. Input strings
stay literal. Explicit `$ref` objects become structured references for later
schema validation and runtime resolution.

Limits include 1 MiB of source, 256 steps, 64 dependencies per step, 256 labels,
32 levels of YAML nesting and 20,000 YAML nodes. Conditions have 4 KiB, 1,024-token
and 32-level nesting limits. The parser rejects aliases/anchors, explicit tags,
nulls, non-finite numbers, duplicate or non-string keys, multiple documents,
invalid UTF-8 and unknown schema fields. Dates remain strings. Diagnostics report
safe schema paths, codes and available source coordinates without echoing values
or raw YAML errors. Input-size checks run before decoding; node/depth checks run
immediately after the bounded YAML document is decoded and before typed decoding.

Defaults are one-hour execution timeout, five-minute step timeout, one execution
and step at a time, one total attempt, one-second initial backoff and one-minute
maximum backoff. Positive integer durations use `ms`, `s`, `m` or `h` and cannot
exceed 24 hours. Attempts cannot exceed 10. Retry backoff cannot exceed its maximum.

`Definition.Phase01()` rejects non-default policies, independent later roots,
conditions and references. Triggers are rejected during parsing because their
variant schemas remain undesigned. Passing this check only confirms definition
capabilities: registry preflight additionally validates the injected action catalog,
input/output schemas, credentials and compatible descriptor versions.
Parsing does not authorize execution or claim that later runtime behavior ships.

Run the parser tests and bounded fuzzing from the repository root:

```sh
go test -count=1 -cover ./internal/workflow
go test ./internal/workflow -run '^$' -fuzz '^FuzzParse$' -fuzztime=10s -parallel=1
go test ./internal/workflow -run '^$' -fuzz '^FuzzValidateCondition$' -fuzztime=10s -parallel=1
```

On Windows, if Go cannot write coverage metadata in the default temporary
directory, set `GOTMPDIR` to an existing writable directory. Quote a coverage
profile argument containing a relative path in PowerShell, for example
`'-coverprofile=.cache/workflow.cover'`. Race detection still needs CGO and a
supported C compiler. No data migration or installed CLI compatibility change
is introduced by this internal parser.
