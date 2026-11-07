# AGENTS.md — OpsWeave Development Instructions

This document defines mandatory development instructions for AI coding agents and contributors working on OpsWeave.

All contributors MUST follow these instructions when modifying the repository.

## 1. General Instructions

Before implementing any task:

1. Read `GOALS.md`.
2. Understand the relevant product requirements.
3. Inspect the existing project architecture.
4. Identify affected packages and interfaces.
5. Check existing tests before modifying behavior.
6. Follow existing repository conventions.
7. Avoid unrelated changes.

Do not assume that planned features are already implemented.

Do not introduce functionality outside the documented project scope without a clear technical justification.

## 2. Go Toolchain Requirements

- Use the Go version declared in `go.mod`.
- Do not increase the minimum supported Go version without justification.
- Use Go Modules.
- Keep `go.mod` and `go.sum` consistent.
- Avoid unnecessary dependencies.
- Prefer the Go standard library when practical.
- Verify dependencies before introducing them.
- Do not use deprecated APIs when supported alternatives exist.
- Follow official Go conventions.

## 3. Code Quality

All Go code MUST:

- Follow `gofmt`.
- Use meaningful package and identifier names.
- Keep functions focused.
- Avoid unnecessary complexity.
- Return errors explicitly.
- Wrap errors with context using `%w` where appropriate.
- Avoid ignoring errors.
- Avoid panic for ordinary application failures.
- Avoid global mutable state.
- Avoid unnecessary interfaces.
- Avoid premature abstractions.
- Keep public APIs minimal and documented.
- Avoid circular package dependencies.

Prefer simple, readable, maintainable code over clever implementations.

## 4. Project Architecture

OpsWeave uses a modular monolith architecture initially.

Maintain clear separation between:

- Domain logic.
- Workflow definitions.
- Workflow parsing and validation.
- Execution engine.
- Scheduling.
- Triggers.
- Actions.
- Worker management.
- API.
- CLI.
- Storage.
- Secrets.
- Observability.

Architecture rules:

1. Business logic must not depend directly on HTTP handlers.
2. Domain logic must remain independent of storage implementations.
3. Infrastructure dependencies must be injected where appropriate.
4. Avoid circular dependencies.
5. Avoid exposing internal implementation details through public packages.
6. Keep package responsibilities explicit.
7. Do not introduce microservices without a documented architectural requirement.
8. Do not add Redis or RabbitMQ as mandatory dependencies for the initial deployment.
9. Keep SQLite suitable for single-node operation.
10. Keep distributed execution concerns isolated from single-node execution.

## 5. Error Handling

- Return errors instead of hiding failures.
- Add meaningful context when propagating errors.
- Use `%w` when wrapping errors.
- Use `errors.Is` and `errors.As` when inspecting wrapped errors.
- Define sentinel errors only when they provide meaningful behavior.
- Use typed errors when callers need structured error information.
- Never expose credentials or secrets in error messages.
- Never silently convert infrastructure failures into successful results.

## 6. Context and Resource Management

- Use `context.Context` for cancellable operations.
- Propagate context through API, engine, worker, scheduler, and action boundaries.
- Do not store request contexts in long-lived structs.
- Use context cancellation for graceful shutdown.
- Close files, database rows, response bodies, and other resources.
- Handle cleanup errors where appropriate.
- Avoid unbounded blocking operations.
- Define timeouts for external operations.

## 7. Concurrency Requirements

Concurrency safety is mandatory.

Rules:

1. Every goroutine must have a clear owner.
2. Every goroutine must have a termination condition.
3. Use bounded worker pools.
4. Avoid unbounded channel buffering.
5. Protect shared mutable state.
6. Avoid holding locks during blocking I/O.
7. Prevent goroutine leaks.
8. Ensure shutdown waits for owned goroutines where appropriate.
9. Handle concurrent cancellation safely.
10. Use synchronization primitives correctly.

Concurrency-sensitive changes MUST pass:

```bash
go test -race -count=1 ./...
```

## 8. Workflow Engine Requirements

The workflow engine is a critical subsystem.

Implementation MUST preserve:

- Explicit execution states.
- Explicit step states.
- Valid state transitions.
- Immutable workflow revisions.
- Step dependency validation.
- Retry policies.
- Timeout policies.
- Cancellation propagation.
- Concurrency limits.
- Execution persistence.
- Recovery behavior.
- Idempotency where supported.

Never:

- Execute an invalid workflow.
- Change historical workflow revisions.
- Silently skip failed dependencies.
- Retry indefinitely.
- Ignore context cancellation.
- Claim exactly-once external side effects without a valid technical guarantee.
- Lose durable execution state without reporting an error.

Every state transition must be validated and persisted according to the storage contract.

## 9. Action Development Requirements

Every action must provide:

- A unique identifier.
- Input validation.
- Output definition.
- Execution implementation.
- Error handling.
- Context support.
- Timeout handling.
- Structured logging.
- Security requirements.
- Tests.

Actions must not bypass execution engine policies.

Actions must respect:

- Execution cancellation.
- Configured timeouts.
- Resource limits.
- Secret redaction.
- Permission restrictions.

Dangerous actions must require explicit configuration.

### Shell Actions

- Never construct shell commands from untrusted string concatenation.
- Prefer argument arrays and direct process execution.
- Use `exec.CommandContext`.
- Apply execution timeouts.
- Bound stdout and stderr.
- Avoid exposing sensitive command arguments in logs.
- Do not enable unrestricted execution by default.

### HTTP Actions

- Apply request timeouts.
- Restrict response sizes.
- Validate URLs.
- Prevent SSRF.
- Handle redirects according to security policy.
- Do not expose authorization headers in logs.
- Respect context cancellation.

### Docker Actions

- Docker socket access must be opt-in.
- Use explicit container allowlists.
- Avoid unrestricted host-level control.
- Validate container identifiers.
- Apply least-privilege principles.

## 10. Storage Requirements

- Use repository boundaries where appropriate.
- Keep domain logic independent of SQL implementations.
- Use database transactions for atomic state changes.
- Use migrations for schema changes.
- Handle rollback correctly.
- Avoid unsafe concurrent state updates.
- Test persistence across restarts.
- Ensure SQLite and PostgreSQL behavior is explicitly documented.
- Never assume both databases provide identical concurrency guarantees.

PostgreSQL distributed job claiming must use safe concurrency control, such as transactional row locking and `FOR UPDATE SKIP LOCKED`, where appropriate.

## 11. Secrets and Security

Security is a mandatory requirement.

- Never hardcode credentials.
- Never commit real secrets.
- Never log secret values.
- Encrypt secrets at rest.
- Validate external inputs.
- Enforce authentication and authorization.
- Use least-privilege permissions.
- Redact sensitive information.
- Avoid unsafe deserialization.
- Prevent path traversal.
- Prevent command injection.
- Prevent SSRF.
- Apply request and response size limits.
- Review dependency vulnerabilities.

Do not weaken security controls merely to simplify implementation or tests.

## 12. API Requirements

- Validate every external request.
- Use consistent response structures.
- Use appropriate HTTP status codes.
- Support request context cancellation.
- Apply timeouts.
- Use request IDs.
- Avoid leaking internal errors.
- Enforce authentication and authorization.
- Document API changes.
- Maintain API version compatibility.

## 13. Configuration Requirements

- Validate configuration during startup.
- Provide sensible defaults.
- Support environment-based configuration.
- Document configuration options.
- Reject invalid configurations with useful errors.
- Never log secret configuration values.
- Avoid silently ignoring unknown critical settings.
- Keep configuration backward compatibility where practical.

## 14. Logging and Observability

Use structured logging.

Logging MUST:

- Include useful contextual information.
- Include execution IDs where applicable.
- Include step IDs where applicable.
- Avoid sensitive information.
- Avoid excessive log volume.
- Distinguish errors from informational events.
- Support appropriate log levels.

Metrics MUST avoid unbounded cardinality.

Do not use workflow IDs, execution IDs, or arbitrary user-controlled values as unrestricted Prometheus labels.

## 15. Testing Requirements

Every new feature MUST include automated tests.

Required verification commands:

```bash
gofmt -l .
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go mod verify
```

### Unit Tests

- Test normal behavior.
- Test invalid inputs.
- Test error conditions.
- Test boundary conditions.
- Prefer table-driven tests.
- Keep tests deterministic.
- Avoid unnecessary external dependencies.

### Integration Tests

- Test database interactions.
- Test migrations.
- Test transactions.
- Test storage recovery.
- Test scheduler interactions.
- Test worker execution.
- Test API and storage integration.

Integration tests must use isolated test resources.

### End-to-End Tests

Test critical workflows from definition to completion.

At minimum:

- Valid workflow execution.
- Invalid workflow rejection.
- Conditional step execution.
- Retry after failure.
- Retry exhaustion.
- Execution timeout.
- Execution cancellation.
- Execution recovery after restart.
- Concurrent execution limits.

### Race Tests

Test:

- Concurrent execution.
- Concurrent job claiming.
- Simultaneous state updates.
- Concurrent cancellation.
- Worker shutdown.
- Scheduler shutdown.

### Fuzz Tests

Use Go fuzzing for:

- YAML workflow parsing.
- Workflow validation.
- Expression parsing.
- Input validation.
- Other parsers exposed to untrusted input.

Fuzz tests must not panic, hang, or consume unbounded resources on malformed input.

### Security Tests

Test:

- Secret redaction.
- Authentication failures.
- Authorization boundaries.
- Shell argument injection.
- SSRF protection.
- Request size limits.
- Response size limits.
- Docker allowlists.
- Unauthorized API access.

### Test Quality

- Do not write tests solely to increase coverage.
- Test observable behavior.
- Test failure scenarios.
- Avoid arbitrary sleep-based synchronization.
- Use context deadlines.
- Use `t.Cleanup` for cleanup.
- Use temporary directories and isolated databases.
- Use `t.Parallel` only when tests are independent and safe.
- Never depend on production credentials or services.

## 16. Coverage Requirements

Target coverage:

- Core workflow engine, parser, and state machine: at least 80% statement coverage.
- Security-critical paths: all important success and failure paths must be tested.
- Storage: cover transactions, errors, and persistence behavior.
- Integration: cover important interactions between components.

Do not sacrifice test quality to reach an arbitrary global coverage percentage.

## 17. Performance Requirements

- Benchmark before optimizing.
- Avoid speculative optimization.
- Measure execution engine performance.
- Measure scheduler overhead.
- Measure worker throughput.
- Measure storage performance.
- Keep memory and goroutine usage bounded.
- Avoid unbounded queues.
- Avoid unnecessary allocations in critical paths when benchmarks justify optimization.

Performance changes must preserve correctness and concurrency safety.

## 18. Documentation Requirements

All project documentation must be written in English.

Every feature must update the relevant documentation.

Required documentation areas:

- Installation.
- Quick start.
- Configuration.
- CLI reference.
- Workflow syntax.
- REST API.
- Built-in actions.
- Triggers.
- Execution behavior.
- Security.
- Deployment.
- Backup and restore.
- Upgrade and migration.
- Troubleshooting.
- Contribution guidelines.

### Release-Based Documentation Versioning

OpsWeave uses Git release tags as the source of documentation versions.

Mandatory rules:

1. Maintain a single documentation source in the repository.
2. Do not manually create version-specific source directories.
3. Generate released documentation from Git release tags.
4. Build documentation from the exact commit associated with each tag.
5. Preserve previously published documentation versions.
6. Never overwrite an existing released version with content from a newer release.
7. The latest stable release must be the default documentation version.
8. Development documentation must be distinguishable from stable documentation.
9. Documentation builds must be reproducible.
10. Update version routing automatically during deployment.

When changing released behavior, inspect the relevant Git tag before modifying version-specific documentation.

## 19. Git and Pull Request Requirements

Before submitting a pull request:

- Ensure changes are scoped to the task.
- Add or update tests.
- Update documentation.
- Verify formatting.
- Run static analysis.
- Run unit tests.
- Run race tests.
- Run relevant integration tests.
- Explain any tests that could not be executed.
- Document breaking changes.
- Include migration instructions when needed.

Never:

- Remove tests just to make CI pass.
- Weaken assertions without justification.
- Claim tests passed without executing them.
- Introduce unrelated dependencies.
- Commit generated secrets or local configuration.
- Modify historical release documentation unintentionally.

## 20. Definition of Done

A task is complete only when:

1. Requirements are understood.
2. Implementation is complete.
3. Relevant tests are written.
4. Tests pass.
5. Static analysis passes.
6. Concurrency safety is verified where applicable.
7. Security implications are reviewed.
8. Documentation is updated.
9. Compatibility is verified.
10. No known regression is introduced.

If a requirement cannot be completed, report the limitation explicitly rather than claiming completion.

## 21. Agent Behavior

AI coding agents MUST:

- Read `GOALS.md` before implementing product features.
- Inspect existing source code before creating new abstractions.
- Follow the established project structure.
- Implement the smallest maintainable solution.
- Avoid unnecessary refactoring.
- Preserve existing behavior unless a change is explicitly required.
- Add tests for new behavior.
- Update documentation.
- Run appropriate verification commands.
- Report what was changed and which tests were actually executed.

Do not mark product requirements as completed merely because implementation code exists.

**Primary engineering principle:** Correctness, reliability, security, and maintainability take priority over implementation speed.