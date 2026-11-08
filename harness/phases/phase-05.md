# Phase 05 — Management API and secrets

Milestone: v0.4.x. Requirement status: `PLANNED`. Review state: NOT_APPROVED.

## Goal, ownership and prerequisites

Deliver management api and secrets with evidence for every owned row in the [requirement ledger](../requirements.md). Ownership includes the exact source text and context; shared rules in phases 00 and 02 apply wherever relevant.

Prerequisites: All phases 00, 01, 02, 03, 04; immediate gate: phase 04 accepted.

Affected boundaries: Public net/http management API, authorization policies, secrets manager, storage encryption and CLI adapters.

Owned IDs: `G-5.6-001`, `G-5.6-002`, `G-5.6-003`, `G-5.6-004`, `G-5.6-005`, `G-5.6-006`, `G-5.6-007`, `G-5.6-008`, `G-5.6-009`, `G-5.6-010`, `G-5.6-011`, `G-5.6-012`, `G-5.6-013`, `G-5.6-014`, `G-5.6-015`, `G-5.6-016`, `G-5.8-001`, `G-5.8-002`, `G-5.8-003`, `G-5.8-004`, `G-5.8-005`, `G-5.8-006`, `G-5.8-007`, `G-5.8-008`, `G-5.8-009`, `G-5.8-010`, `G-5.8-011`, `G-5.8-012`, `G-5.8-013`, `G-5.8-014`, `G-5.8-015`, `G-5.8-016`, `G-5.8-017`, `G-5.8-018`, `G-5.8-019`, `G-5.8-020`, `G-5.8-021`, `G-5.8-022`, `G-5.8-023`, `G-5.8-024`, `G-5.8-025`, `G-5.8-026`, `G-5.8-027`, `G-5.9-018`, `G-5.9-019`, `G-5.9-020`, `G-5.15-001`, `G-5.15-002`, `G-5.15-003`, `G-5.15-004`, `G-5.15-006`, `G-5.15-010`, `G-5.15-011`, `G-5.15-016`, `G-11-005`

## Ordered implementation tasks

1. Review versioned routes/payload/error/pagination/filter contracts, request cancellation semantics and RBAC model; define API compatibility and authentication/bootstrap configuration.
2. Design encrypted secret storage/key lifecycle, scoped references, metadata-only APIs, access policy, rotation, deletion behavior and audit events before schema changes.
3. Implement the full REST capability set and secret CLI set through shared application services; add rate limits, request IDs, input/body limits and deadlines.
4. Apply authentication/authorization before exposing any management operation; document health/readiness exposure and redact all diagnostics.

## Acceptance scenarios

- API/storage integration tests cover each operation, pagination/filtering, consistent status/errors, request IDs, rate limits, size limits and cancellation.
- Test missing/invalid credentials and every role boundary, including direct-object access and secret environment scopes.
- Inspect persisted data and responses for plaintext secrets; test rotation, unavailable/wrong keys, audit events, denied access and redaction.
- Test workflow secret references and CLI noninteractive behavior without echoing values; preserve released API/workflow compatibility.

## Security, documentation and compatibility

- Follow [AGENTS.md](../../AGENTS.md) and [GOALS.md](../../GOALS.md); keep domain logic independent of HTTP and SQL adapters. Introduce only packages needed for delivered behavior.
- Review new external inputs, permissions, resource bounds and secret handling before exposing each feature. Inject infrastructure dependencies, propagate contexts and persist validated state changes atomically.
- Update English reference documentation, examples and troubleshooting with the feature. Record schema migrations, rollback/data implications and compatibility evidence; if no migration/API change is needed, state that with a reason in the build log.
- Review contracts listed in the tasks before their implementations. No illustrative YAML or unreviewed API/schema/plugin design becomes a released promise. Inspect relevant release tags once they exist; keep one documentation source.

## Verification and review checklist

Run `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go mod verify`, `go build ./...`, `bun fmt --check "**/*.md"` and relevant phase acceptance suites. Add bounded fuzz runs for introduced parsers and isolated integration resources. Use deterministic synchronization and context deadlines. Record exact test/benchmark commands when introduced.

- [ ] Every owned row has exact test/review and documentation evidence attached; shared policy obligations audited.
- [ ] All applicable checks passed; unavailable checks remain explicit open gates.
- [ ] Security, compatibility and migration implications reviewed; source changes scoped.
- [ ] Append commands, exit codes, evidence, limitations and next steps to [build log](../build-log.md).
- [ ] Present results and obtain user acceptance before advancing. Do not treat code presence or this checklist as acceptance evidence.
