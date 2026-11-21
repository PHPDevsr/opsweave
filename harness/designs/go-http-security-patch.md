# Go HTTP security patch decision

Status `PROPOSED`, 2026-11-22 (Asia/Jakarta).

## Finding and proposed change

`govulncheck ./...` exited 1 on Go 1.27.3 with
[GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859). The official report lists
Go 1.27.4 as the fixed version. It concerns concurrent HTTP/1 server header access
with MaxBytesReader/MaxBytesHandler. The scan reports reachable net/http symbols
in both HTTP client composition and existing webhook server paths. This is scan
evidence; it is not a demonstrated exploit in OpsWeave's handlers. Do not suppress
the report or label the security gate passed.

Proposed patch-only toolchain change:

- Change `go.mod` from `go 1.27.3` to `go 1.27.4`.
- Change the explicit minimum-version CI matrix entry from 1.27.3 to 1.27.4;
  keep Linux, macOS, Windows and stable coverage. Release/security/benchmark jobs
  already read the toolchain version from go.mod.
- Update active toolchain requirements in README, CONTRIBUTING and the foundation
  guide. Preserve historical assessment/log/test evidence attributed to 1.27.3.
- Install/use the verified Go 1.27.4 toolchain without changing application module
  dependencies. Run formatting, vet, full unit/race suites, module verification,
  build, Markdown and govulncheck again; observe remote CI evidence.

This raises the minimum build patch version and requires rebuilding binaries to
include the standard-library fix. No workflow DSL, database schema or runtime
configuration migration is planned. Existing installed binaries do not receive
standard-library fixes until rebuilt/replaced. No release/tag is created by this
patch. Reference [Go release history](https://go.dev/doc/devel/release#go1.27.4).

## Review gate

[PLANS.md](../../PLANS.md) states: "Retain Go 1.27.3 unless a separately justified
change is approved." Accept the changes above to authorize the patch-version
increase. Until then keep the HTTP requirement rows `IMPLEMENTED`, with the
security gate open, and do not mark the feature complete.
