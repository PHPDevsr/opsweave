# OpsWeave — Product Goals & Requirements

> **Tagline:** Automate Infrastructure. Simplify Operations.

## 1. Product Vision

OpsWeave is a lightweight, self-hosted, event-driven infrastructure automation platform built with Go.

It enables developers, DevOps engineers, and system administrators to define, execute, monitor, and manage infrastructure automation workflows through declarative YAML definitions.

OpsWeave aims to simplify operational automation by combining workflow orchestration, scheduling, event triggers, execution management, observability, and extensible integrations in one platform.

### Primary Goals

- Provide a lightweight and self-hosted automation platform.
- Make infrastructure automation accessible through declarative YAML workflows.
- Support scheduled, event-driven, webhook, and manual execution.
- Provide reliable workflow execution with retries, timeouts, and recovery.
- Maintain persistent execution history and state.
- Provide secure and observable automation.
- Support extensible actions and third-party integrations.
- Build a sustainable open-source ecosystem.

## 2. Product Principles

1. Self-hosted first.
2. Go-native and resource-efficient.
3. YAML-first and Git-friendly.
4. Event-driven architecture.
5. Reliable and recoverable execution.
6. Secure by default.
7. Observable by design.
8. Extensible through well-defined interfaces.
9. Minimal mandatory infrastructure dependencies.
10. Backward compatibility for released workflow specifications.

## 3. Target Users

- Developers.
- DevOps engineers.
- System administrators.
- Infrastructure engineers.
- Self-hosting enthusiasts.
- Small and medium-sized engineering teams.
- Organizations that need operational automation without a large automation platform.

## 4. Product Scope

OpsWeave is an infrastructure automation and workflow orchestration platform.

It is not intended to replace:

- Docker or container runtimes.
- Kubernetes.
- Prometheus or Grafana.
- GitHub Actions or other CI/CD platforms.
- Dedicated monitoring systems.
- Database management systems.

Instead, OpsWeave connects these systems and automates operational tasks across them.

## 5. Functional Requirements

### 5.1 Workflow Management

**Priority: P0**

OpsWeave MUST support declarative workflow management.

Required features:

- YAML workflow definitions.
- Workflow schema validation.
- Workflow syntax validation.
- Workflow registration and deployment.
- Workflow updates.
- Workflow deletion and archival.
- Workflow listing.
- Workflow details.
- Workflow enable and disable.
- Workflow revision history.
- Immutable workflow revisions.
- Workflow import and export.
- Workflow dry-run and execution planning.
- Workflow metadata and descriptions.
- Workflow labels and categorization.

Acceptance criteria:

- Invalid workflows must be rejected before registration.
- Duplicate workflow identifiers must be handled safely.
- Every registered revision must have a unique identity.
- Existing executions must reference the exact workflow revision they started with.
- Updating a workflow must not modify historical execution definitions.
- Disabled workflows must not accept new automatic triggers.

### 5.2 Workflow Execution Engine

**Priority: P0**

The execution engine is the core component of OpsWeave.

Required features:

- Manual workflow execution.
- Sequential step execution.
- Conditional step execution.
- Step dependency management.
- Parallel step execution.
- Execution status tracking.
- Step status tracking.
- Configurable retry attempts.
- Exponential backoff.
- Configurable execution timeout.
- Configurable step timeout.
- Execution cancellation.
- Step cancellation.
- Execution output capture.
- Execution log persistence.
- Execution history.
- Execution concurrency limits.
- Idempotency support.
- Failed execution handling.
- Execution recovery after restart.
- Controlled execution retry and replay.
- Execution retention policies.

Required execution states:

- Pending.
- Queued.
- Running.
- Succeeded.
- Failed.
- Cancelled.
- Timed out.

Required step states:

- Pending.
- Running.
- Succeeded.
- Failed.
- Skipped.
- Cancelled.
- Timed out.

Acceptance criteria:

- Every execution must have a unique identifier.
- Every execution must reference an immutable workflow revision.
- State transitions must be validated.
- Failed steps must follow their configured retry policy.
- Retry exhaustion must result in a terminal failure state.
- Cancellation must propagate through supported actions.
- Timeouts must cancel the associated execution context.
- Independent steps may execute concurrently within configured limits.
- Dependent steps must not execute before their dependencies are satisfied.
- Execution state must survive application restarts.
- Recovery must not silently duplicate non-idempotent external side effects.

Delivery semantics:

- Use at-least-once processing where applicable.
- Do not claim exactly-once external side effects.
- Use idempotency keys and deduplication where supported.
- Persist state transitions before acknowledging durable jobs.
- Use leases or equivalent ownership mechanisms for distributed execution.

### 5.3 Trigger System

**Priority: P0**

Required trigger types:

- Manual trigger.
- Scheduled trigger.
- HTTP webhook trigger.
- Internal event trigger.
- Application startup trigger.
- File-system event trigger (advanced).

Required capabilities:

- Trigger registration.
- Trigger enable and disable.
- Trigger validation.
- Trigger authentication.
- Event validation.
- Event deduplication.
- Trigger execution history.
- Trigger failure reporting.
- Trigger concurrency controls.

Acceptance criteria:

- Unauthorized webhook requests must be rejected.
- Duplicate events must be handled according to configured deduplication policies.
- Invalid trigger configurations must be rejected.
- Disabled triggers must not generate new executions.

### 5.4 Scheduler

**Priority: P0**

Required features:

- Cron-based scheduling.
- Timezone-aware schedules.
- Multiple concurrent schedules.
- Schedule enable and disable.
- Schedule persistence.
- Missed schedule handling.
- Duplicate schedule prevention.
- Scheduler recovery after restart.
- Graceful scheduler shutdown.
- Schedule execution history.

Acceptance criteria:

- Schedule calculations must respect configured timezones.
- Restarting the application must not unintentionally duplicate scheduled executions.
- Scheduler shutdown must not leave uncontrolled goroutines.
- Missed execution behavior must be configurable and documented.

### 5.5 Built-in Action System

**Priority: P0**

Actions are reusable execution units invoked by workflow steps.

Every action MUST define:

- Unique action identifier.
- Input schema.
- Output schema.
- Input validation.
- Execution behavior.
- Error handling.
- Timeout behavior.
- Logging behavior.
- Security requirements.
- Version compatibility.

Planned built-in actions:

**HTTP**
- `http.request`
- `http.health_check`

**Shell**
- `shell.exec`

**Docker**
- `docker.start`
- `docker.stop`
- `docker.restart`
- `docker.inspect`

**File system**
- `file.exists`
- `file.cleanup`
- `file.copy`

**Database**
- `database.query`
- `database.backup`
- `database.restore`

**Notifications**
- `notification.telegram`
- `notification.slack`
- `notification.email`
- `notification.webhook`

Acceptance criteria:

- Actions must validate their inputs before execution.
- Actions must support context cancellation where applicable.
- Actions must have bounded execution time and resource consumption.
- Actions must return structured results.
- Actions must not expose secrets in logs.
- Dangerous actions must require explicit security configuration.
- Action failures must be distinguishable from workflow engine failures.

### 5.6 Secrets Management

**Priority: P1**

Required features:

- Secret creation.
- Secret update.
- Secret deletion.
- Secret metadata listing.
- Encrypted secret storage.
- Workflow secret references.
- Environment-specific secrets.
- Secret access control.
- Secret masking in logs.
- Secret rotation support.
- Secret audit events.

Acceptance criteria:

- Secrets must never be stored in plaintext at rest.
- Secret values must not be returned in ordinary API responses.
- Logs must redact sensitive values.
- Access must be restricted according to authorization policies.
- Encryption key management must be documented.

### 5.7 Notification System

**Priority: P1**

Required integrations:

- Telegram.
- Slack.
- Email.
- Generic HTTP webhook.

Required notification events:

- Workflow started.
- Workflow succeeded.
- Workflow failed.
- Step failed.
- Retry exhausted.
- Execution cancelled.
- Execution recovered.
- Worker failure.

Additional capabilities:

- Notification templates.
- Configurable notification policies.
- Notification delivery status.
- Retry failed notification delivery.
- Notification error reporting.

### 5.8 REST API

**Priority: P1**

The REST API MUST provide programmatic access to OpsWeave.

Required capabilities:

- API authentication.
- Authorization.
- Workflow management.
- Workflow validation.
- Workflow execution.
- Execution listing.
- Execution details.
- Execution logs.
- Execution cancellation.
- Execution retry.
- Trigger management.
- Secret management.
- Worker status.
- Health endpoint.
- Readiness endpoint.
- API versioning.
- Pagination.
- Filtering.
- Consistent error responses.
- Request ID tracking.
- Rate limiting.

Acceptance criteria:

- API endpoints must validate all external input.
- Unauthorized requests must be rejected.
- API errors must use consistent response structures.
- Long-running operations must support cancellation where applicable.
- API versioning must preserve released compatibility contracts.

### 5.9 CLI

**Priority: P0**

The CLI is the primary interface for local administration and automation scripting.

Planned commands:

```bash
opsweave init
opsweave version
opsweave server

opsweave validate ./workflows/
opsweave apply ./workflows/

opsweave workflow list
opsweave workflow get <name>
opsweave workflow delete <name>
opsweave workflow enable <name>
opsweave workflow disable <name>

opsweave run <workflow>

opsweave execution list
opsweave execution get <id>
opsweave execution logs <id>
opsweave execution cancel <id>
opsweave execution retry <id>

opsweave secret set <name>
opsweave secret list
opsweave secret delete <name>
```

Required capabilities:

- Consistent command structure.
- Command help.
- Flag validation.
- Config file support.
- Environment variable support.
- JSON output.
- Human-readable output.
- Consistent exit codes.
- Shell completion.
- Non-interactive operation for automation scripts.

### 5.10 Storage and Persistence

**Priority: P0**

Supported storage backends:

- SQLite for single-node deployments.
- PostgreSQL for production and distributed deployments.

Required capabilities:

- Workflow persistence.
- Workflow revision persistence.
- Execution persistence.
- Step state persistence.
- Trigger persistence.
- Scheduler state persistence.
- Database migrations.
- Transaction handling.
- Backup and restore.
- Data retention policies.
- Storage health checks.

Acceptance criteria:

- Database schema changes must use migrations.
- Failed transactions must roll back safely.
- Historical executions must remain readable after workflow updates.
- SQLite must be documented as single-node storage unless stronger guarantees are implemented.
- PostgreSQL must support concurrent job claiming safely.

### 5.11 Worker Management

**Priority: P1**

Required features:

- Configurable worker count.
- Bounded worker pool.
- Job claiming.
- Worker registration.
- Worker heartbeat.
- Execution leases.
- Worker shutdown.
- Worker failure recovery.
- Execution timeout enforcement.
- Worker status monitoring.
- Distributed workers using PostgreSQL.

Acceptance criteria:

- Workers must not claim jobs beyond configured capacity.
- Expired leases must be recoverable.
- Concurrent job claiming must not create conflicting ownership.
- Shutdown must allow controlled completion or cancellation of active work.

### 5.12 Observability

**Priority: P1**

Required features:

- Structured logging.
- Execution-level logs.
- Step-level logs.
- Execution metrics.
- Worker metrics.
- Scheduler metrics.
- API metrics.
- Prometheus metrics endpoint.
- OpenTelemetry tracing.
- Execution duration tracking.
- Retry metrics.
- Failure metrics.
- Health monitoring.
- Configurable log retention.

Acceptance criteria:

- Every execution must be traceable through a unique execution ID.
- Logs must include useful contextual identifiers.
- Sensitive information must be redacted.
- Metrics must avoid unbounded cardinality.
- Health and readiness checks must report meaningful system state.

### 5.13 Web Dashboard

**Priority: P2**

The web dashboard provides a graphical management interface.

Required pages:

- Overview dashboard.
- Workflow listing.
- Workflow details.
- Workflow creation and editing.
- Workflow execution history.
- Execution details.
- Step execution timeline.
- Live execution logs.
- Manual execution.
- Execution cancellation.
- Worker monitoring.
- Scheduler monitoring.
- Secrets management.
- User management.
- Role management.
- System configuration.

The dashboard must consume the public REST API rather than directly accessing internal storage.

### 5.14 Plugin and Integration System

**Priority: P2**

Required capabilities:

- Action registration interface.
- Built-in action packages.
- Public Go SDK.
- External plugin protocol.
- Plugin version compatibility.
- Plugin configuration.
- Plugin capability declaration.
- Plugin execution isolation.
- Plugin error reporting.
- Community action development.

Acceptance criteria:

- Plugin contracts must be documented.
- Plugin compatibility must be verifiable.
- External plugins must have explicit permissions.
- Arbitrary third-party code must not execute in the main process without a documented security model.

### 5.15 Security

**Priority: P0**

Required security capabilities:

- Secure API defaults.
- Authentication and authorization.
- RBAC.
- Secret encryption.
- Sensitive data redaction.
- Audit logging.
- Shell execution restrictions.
- Docker action allowlists.
- SSRF protection.
- Request size limits.
- Response size limits.
- Execution timeouts.
- Plugin isolation.
- Dependency vulnerability scanning.
- Secure configuration handling.

Security acceptance criteria:

- API management endpoints must not be exposed without authentication.
- Shell execution must use argument arrays rather than unsafe shell string concatenation.
- Docker socket access must be opt-in.
- Docker actions must enforce configured container allowlists.
- HTTP actions must prevent access to restricted internal network destinations according to policy.
- Secrets must never appear in logs or error responses.
- Untrusted input must be validated at system boundaries.

### 5.16 Deployment and Operations

**Priority: P0**

Required deployment methods:

- Single binary.
- Docker.
- Docker Compose.

Required operational capabilities:

- YAML configuration.
- Environment-based configuration.
- Persistent data storage.
- Health checks.
- Readiness checks.
- Graceful shutdown.
- Automatic database migrations.
- Configurable resource limits.
- Backup and restore.
- Upgrade instructions.
- Rollback instructions.
- Production deployment examples.

OpsWeave must not require Redis or RabbitMQ for its initial single-node deployment.

## 6. Workflow Specification

OpsWeave uses declarative YAML workflow definitions.

Illustrative example:

```yaml
apiVersion: opsweave.io/v1
kind: Workflow

metadata:
  name: api-auto-recovery
  description: Automatically recover an unhealthy API

spec:
  triggers:
    - type: schedule
      cron: "*/2 * * * *"

  concurrency:
    limit: 1

  steps:
    - id: health-check
      uses: http.request
      with:
        url: http://localhost:1999/health
        method: GET
      retry:
        attempts: 2
        backoff: 3s

    - id: restart-service
      uses: docker.restart
      if: steps.health-check.failed
      with:
        container: eclaim-api

    - id: notify
      uses: notification.telegram
      if: steps.restart-service.success
      with:
        message: "API recovery executed"
```

The final workflow specification MUST define:

- YAML schema.
- API versioning.
- Expression syntax.
- Step dependency semantics.
- Conditional execution.
- Retry semantics.
- Execution output references.
- Concurrency semantics.
- Error propagation.
- Timeout semantics.
- Secret references.
- Action compatibility.
- Workflow compatibility guarantees.

This example is illustrative and MUST NOT be treated as a finalized DSL contract until the parser, validator, and execution semantics are specified and tested.

## 7. Technical Architecture Goals

### Core architecture

Use a modular monolith initially.

Logical components:

- API server.
- CLI.
- Workflow registry.
- Workflow parser and validator.
- Trigger manager.
- Scheduler.
- Dispatcher.
- Execution engine.
- Action executor.
- Worker pool.
- Storage layer.
- Secrets manager.
- Observability layer.

Distributed workers may be introduced later without requiring a complete architectural rewrite.

### Technology preferences

- Language: Go.
- HTTP: Go standard library `net/http` initially.
- CLI: Cobra.
- Configuration: Viper or a simpler alternative when sufficient.
- Logging: `log/slog`.
- Storage: SQLite and PostgreSQL.
- PostgreSQL driver: pgx.
- Metrics: Prometheus.
- Tracing: OpenTelemetry.
- Documentation: Astro Starlight.
- Deployment: Docker and Docker Compose.
- Frontend: React and TypeScript when dashboard development begins.

These are technology preferences, not permission to introduce unnecessary dependencies.

## 8. Go Engineering Requirements

- Use Go Modules.
- Define a minimum supported Go version.
- Test against the minimum supported and latest stable Go versions.
- Follow official Go formatting and conventions.
- Prefer standard library packages when practical.
- Avoid unnecessary third-party dependencies.
- Use explicit error handling.
- Wrap errors with contextual information where appropriate.
- Use `context.Context` for cancellable operations.
- Avoid uncontrolled goroutines.
- Ensure graceful shutdown.
- Avoid unnecessary global mutable state.
- Protect shared mutable state against data races.
- Validate configuration during startup.
- Use structured logging.
- Redact sensitive information.
- Keep package boundaries explicit.
- Avoid circular dependencies.
- Benchmark before optimizing.

## 9. Testing Requirements

Testing is mandatory for all production features.

Required test categories:

- Unit tests.
- Integration tests.
- End-to-end tests.
- Race detection.
- Fuzz testing.
- Security tests.
- Database migration tests.
- Compatibility tests.
- Performance benchmarks for critical paths.

Critical components requiring comprehensive coverage:

- Workflow parser and validator.
- Execution state machine.
- Step dependency resolution.
- Retry and backoff.
- Timeout and cancellation.
- Worker pool.
- Scheduler.
- Trigger deduplication.
- Storage transactions.
- Execution recovery.
- Secret management.
- Action execution security.

### Coverage goals

- Core engine, state machine, and parser: target at least 80% statement coverage.
- Security-critical paths: test all important success and failure scenarios.
- Integration tests: cover important cross-component behavior.
- Do not optimize for arbitrary 100% global coverage.

### Required verification commands

```bash
gofmt -l .
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go mod verify
```

## 10. Documentation Requirements

OpsWeave MUST maintain documentation as part of every feature.

Documentation must cover:

- Installation.
- Quick start.
- Configuration.
- Workflow syntax.
- CLI reference.
- REST API reference.
- Built-in actions.
- Trigger configuration.
- Execution behavior.
- Security.
- Deployment.
- Backup and restore.
- Upgrade and migration.
- Troubleshooting.
- Contribution guidelines.

### Release-based documentation versioning

- Maintain a single documentation source in the repository.
- Generate released documentation from Git release tags.
- Every published documentation version must correspond to its exact tagged source commit.
- Preserve previously published versions.
- Never overwrite an older version with newer documentation.
- The latest stable release must be the default documentation version.
- Development documentation must be distinguishable from stable releases.
- Version selection and routing must be generated during deployment.
- Documentation builds must be reproducible from supported Git tags.

Do not manually maintain separate version-specific source directories in the repository.

## 11. Release Roadmap

The roadmap describes intended delivery milestones. It does not imply that features are already implemented.

| Release | Primary goals |
|---|---|
| v0.1.x | Workflow parser, validator, CLI, SQLite, manual execution, state machine |
| v0.2.x | Scheduler, webhook, worker pool, retry, timeout, cancellation, recovery |
| v0.3.x | HTTP, shell, Docker, file-system, and notification actions |
| v0.4.x | REST API, authentication, secrets management |
| v0.5.x | Observability, Prometheus, execution history improvements |
| v0.6.x | Web dashboard and workflow management |
| v0.7.x | PostgreSQL and distributed workers |
| v0.8.x | Plugin SDK, external plugin protocol, integrations |
| v0.9.x | Security hardening, performance, compatibility testing |
| v1.0.0 | Stable production release |

A feature must only be marked completed after its acceptance criteria and required tests pass.

## 12. Definition of Done

A feature is considered complete only when:

- Implementation meets the documented requirements.
- Acceptance criteria are satisfied.
- Automated tests are implemented and pass.
- Relevant integration tests pass.
- Concurrency safety is verified where applicable.
- Security implications are reviewed.
- Documentation is updated.
- Examples are updated where necessary.
- Compatibility is verified.
- CI quality gates pass.

## 13. Project Success Criteria

OpsWeave v1.0 should achieve:

- Reliable single-node workflow automation.
- Production-ready workflow execution and recovery.
- Secure built-in action execution.
- Persistent execution history.
- Scheduled and webhook-triggered automation.
- Useful CLI and REST API.
- Comprehensive observability.
- Straightforward self-hosted deployment.
- Documented upgrade and backup procedures.
- Stable workflow specification.
- Automated testing and release pipelines.
- A documented extension model for community contributions.

Long-term success is measured by reliability, usability, extensibility, community adoption, and sustainable maintenance rather than feature count alone.

## 14. Requirement Status Policy

Requirements should be tracked using these statuses:

- `PLANNED`: Defined but not implemented.
- `IN_PROGRESS`: Currently being implemented.
- `IMPLEMENTED`: Code exists but acceptance criteria are not fully verified.
- `COMPLETED`: Implementation, tests, and documentation satisfy requirements.
- `DEFERRED`: Intentionally postponed.
- `CANCELLED`: Removed from the project scope.

Never mark a requirement as completed based solely on the existence of source code.

---

**Project principle:** Build a reliable automation engine first, then expand its ecosystem without compromising simplicity, security, and operational reliability.