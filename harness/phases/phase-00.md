# Phase 00 — Assessment and harness

Milestone: Planning only. Requirement status: `PLANNED`. Review state: READY_FOR_REVIEW.

## Goal, ownership and prerequisites

Deliver assessment and harness with evidence for every owned row in the [requirement ledger](../requirements.md). Ownership includes the exact source text and context; shared rules in phases 00 and 02 apply wherever relevant.

Prerequisites: None.

Affected boundaries: Harness documents and requirement ownership; no runtime boundaries change.

Owned IDs: `G-0-001`, `G-1-001`, `G-1-002`, `G-1-003`, `G-1-004`, `G-1-005`, `G-1-006`, `G-1-007`, `G-1-008`, `G-1-009`, `G-1-010`, `G-1-011`, `G-2-001`, `G-2-002`, `G-2-003`, `G-2-004`, `G-2-005`, `G-2-006`, `G-2-007`, `G-2-008`, `G-2-009`, `G-2-010`, `G-3-001`, `G-3-002`, `G-3-003`, `G-3-004`, `G-3-005`, `G-3-006`, `G-3-007`, `G-4-001`, `G-4-002`, `G-4-003`, `G-4-004`, `G-4-005`, `G-4-006`, `G-4-007`, `G-4-008`, `G-7-001`, `G-7-002`, `G-7-003`, `G-7-004`, `G-7-005`, `G-7-006`, `G-7-007`, `G-7-008`, `G-7-009`, `G-7-010`, `G-7-011`, `G-7-012`, `G-7-013`, `G-7-014`, `G-7-015`, `G-7-016`, `G-7-017`, `G-7-018`, `G-7-019`, `G-7-020`, `G-7-021`, `G-7-022`, `G-7-023`, `G-7-024`, `G-7-025`, `G-7-026`, `G-7-027`, `G-7-028`, `G-8-001`, `G-8-002`, `G-8-003`, `G-8-004`, `G-8-005`, `G-8-006`, `G-8-007`, `G-8-008`, `G-8-009`, `G-8-010`, `G-8-011`, `G-8-012`, `G-8-013`, `G-8-014`, `G-8-015`, `G-8-016`, `G-8-017`, `G-8-018`, `G-8-019`, `G-9-001`, `G-9-002`, `G-9-003`, `G-9-004`, `G-9-005`, `G-9-006`, `G-9-007`, `G-9-008`, `G-9-009`, `G-9-010`, `G-9-011`, `G-9-012`, `G-9-013`, `G-9-014`, `G-9-015`, `G-9-016`, `G-9-017`, `G-9-018`, `G-9-019`, `G-9-020`, `G-9-021`, `G-9-022`, `G-9-023`, `G-9-024`, `G-9-025`, `G-9-026`, `G-9-027`, `G-9-028`, `G-9-029`, `G-9-030`, `G-9-031`, `G-10-001`, `G-10-002`, `G-10-003`, `G-10-004`, `G-10-005`, `G-10-006`, `G-10-007`, `G-10-008`, `G-10-009`, `G-10-010`, `G-10-011`, `G-10-012`, `G-10-013`, `G-10-014`, `G-10-015`, `G-10-016`, `G-10-017`, `G-10-018`, `G-10-019`, `G-10-020`, `G-10-021`, `G-10-022`, `G-10-023`, `G-10-024`, `G-10-025`, `G-10-026`, `G-11-001`, `G-11-012`, `G-12-001`, `G-12-002`, `G-12-003`, `G-12-004`, `G-12-005`, `G-12-006`, `G-12-007`, `G-12-008`, `G-12-009`, `G-12-010`, `G-14-001`, `G-14-002`, `G-14-003`, `G-14-004`, `G-14-005`, `G-14-006`, `G-14-007`, `G-14-008`

## Ordered implementation tasks

1. Assess source, tests, manifests, CI, tags and worktree without inferring implementation from goals.
2. Publish the master plan, exhaustive ledger, eleven phase plans, operating instructions and append-only build log.
3. Reconcile source coverage, requirement ownership, prerequisite order and all document links; format only new planning documents.
4. Present generated artifacts and limitations for review; stop before phase 01.

## Acceptance scenarios

- Every GOALS requirement and narrative constraint maps to one phase; IDs are unique and stable.
- All eleven phase plans include contracts to design, tests, dependencies, security and documentation obligations.
- Baseline results are attributed to actual runs; blocked checks stay visible; no product source changes.

## Security, documentation and compatibility

- Follow [AGENTS.md](../../AGENTS.md) and [GOALS.md](../../GOALS.md); keep domain logic independent of HTTP and SQL adapters. Introduce only packages needed for delivered behavior.
- Review new external inputs, permissions, resource bounds and secret handling before exposing each feature. Inject infrastructure dependencies, propagate contexts and persist validated state changes atomically.
- Update English reference documentation, examples and troubleshooting with the feature. Record schema migrations, rollback/data implications and compatibility evidence; if no migration/API change is needed, state that with a reason in the build log.
- Review contracts listed in the tasks before their implementations. No illustrative YAML or unreviewed API/schema/plugin design becomes a released promise. Inspect relevant release tags once they exist; keep one documentation source.

## Verification and review checklist

Reconcile every GOALS item/prose constraint/command/release row with the ledger; check unique IDs, valid owners and acyclic prerequisites; resolve local document links. Run `npm run check:markdown` and `git diff --check`. Record baseline Go checks and limitations without claiming product acceptance.

- [ ] Every owned row has exact test/review and documentation evidence attached; shared policy obligations audited.
- [ ] All applicable checks passed; unavailable checks remain explicit open gates.
- [ ] Security, compatibility and migration implications reviewed; source changes scoped.
- [ ] Append commands, exit codes, evidence, limitations and next steps to [build log](../build-log.md).
- [ ] Present results and obtain user acceptance before advancing. Do not treat code presence or this checklist as acceptance evidence.
