<!--
  SYNC IMPACT REPORT
  ==================
  Version change: 0.0.0 → 1.0.0 (Initial adoption)
  Modified principles:
    - Added I. Clear, Idiomatic Go Packages
    - Added II. WebSocket Contract & Configuration Discipline
    - Added III. Secrets, Security & Least Privilege
    - Added IV. Test-First Quality Gates
    - Added V. Observability & Operational Resilience
    - Added VI. Release & Infrastructure Alignment
  Added sections:
    - Engineering Standards
    - Delivery Workflow
    - Governance
  Removed sections: None
  Templates requiring updates:
    - .specify/templates/plan-template.md ✅ verified (Constitution Check present)
    - .specify/templates/spec-template.md ✅ verified (requirements and success criteria present)
    - .specify/templates/tasks-template.md ✅ verified (phased task structure with test-first support)
    - .cursor/commands/speckit.specify.md ✅ verified
    - .cursor/commands/speckit.tasks.md ✅ verified
    - .specify/templates/commands/*.md ⚠ not present in this repository; equivalent guidance checked in .cursor/commands/
  Follow-up TODOs: None
-->

# Weni WebChat Socket Constitution

## Core Principles

### I. Clear, Idiomatic Go Packages

Production code MUST be readable without extensive commentary. Exported
functions MUST have GoDoc comments, packages MUST have a single
responsibility, and the WebSocket handler layer MUST delegate non-trivial
logic to focused service or helper packages. Rationale: a long-running
WebSocket server is debugged under connection-pressure scenarios, so
clarity reduces mean time to resolution.

- Go code MUST favor explicit control flow, descriptive names, and small
  composable functions.
- Exported types and functions MUST include GoDoc comments in imperative
  mood describing behavior and constraints.
- Comments SHOULD explain intent, trade-offs, or infrastructure-specific
  caveats (Redis, MongoDB, gRPC), not restate obvious code.
- Debug code, dead branches, and commented-out implementations MUST not
  be committed.
- Files exceeding roughly 500 lines MUST be justified in the plan or
  refactored into separate packages.

### II. WebSocket Contract & Configuration Discipline

Message behavior MUST be deterministic from the incoming payload, server
configuration, and approved external sources. Event contracts,
environment variables, and connection lifecycle rules MUST be explicit in
specs and documentation. Rationale: WebSocket servers fail unpredictably
when message contracts drift or configuration is implicit.

- Handlers MUST validate or normalize incoming messages at the boundary
  before dispatching to internal logic.
- Business logic MUST live outside the handler body when it can be tested
  independently (e.g., in service or helper packages).
- Configuration MUST come from environment variables loaded through the
  `config` package or approved sources (Redis, MongoDB), never from
  hardcoded values.
- Connection lifecycle operations (register, unregister, reconnect) MUST
  be explicitly documented with their expected message shapes.
- Network calls to external services (Flows API, Redis, MongoDB, S3)
  MUST set explicit timeouts and retries appropriate to the operation
  context.

### III. Secrets, Security & Least Privilege

Sensitive data MUST be protected across code, logs, and infrastructure
interactions. Rationale: this project handles JWT tokens, S3 credentials,
Redis and MongoDB connection strings, and user session data, so
accidental exposure or over-privileged access is high risk.

- Secrets MUST never be hardcoded, committed, or written to logs.
- Credential and secret access MUST flow through dedicated configuration
  modules or clearly isolated adapters (e.g., `config`, `jwt` packages).
- IAM and resource permission assumptions MUST be documented when a
  feature needs new access to AWS, Redis, or MongoDB resources.
- External responses MUST be handled defensively, with actionable errors
  that do not leak sensitive payloads or internal state.
- Dependencies that affect authentication (JWT), transport (HTTP/gRPC),
  or cryptography MUST be introduced deliberately and documented in the
  plan or research.

### IV. Test-First Quality Gates

Every behavior change MUST be backed by automated tests before review,
and no feature is complete until quality gates pass. Rationale:
regressions in a real-time WebSocket server are cheap to introduce and
expensive to detect in production.

- New behavior MUST include tests that fail before implementation and
  pass afterward.
- Shared logic MUST have unit tests; handler, WebSocket message flow, or
  integration tests MUST exist when message contracts, external service
  interactions, or connection lifecycle changes.
- Changed modules SHOULD maintain at least 80% line and branch coverage
  unless the plan records an approved exception.
- Linting, formatting (`gofmt`/`golangci-lint`), and tests MUST pass
  locally before code review and in CI before merge.
- Bug fixes MUST include a regression test whenever technically feasible.

### V. Observability & Operational Resilience

Production behavior MUST be diagnosable from logs, metrics, and explicit
failure paths. Rationale: WebSocket servers maintain persistent
connections; if telemetry is weak, connection drops, message loss, and
resource leaks are difficult to reproduce.

- Logs MUST include enough context to trace the operation (connection ID,
  client identifier, message type) but MUST exclude secrets, tokens, and
  personal data.
- Error handling MUST distinguish retriable upstream failures (Redis
  timeouts, external API errors) from permanent validation or contract
  errors.
- Prometheus metrics MUST be maintained for key operations: active
  connections, message throughput, error rates, and Redis pool health.
- Features affecting reconnection, concurrency, pool management, or
  graceful shutdown MUST document their operational impact.
- Silent exception swallowing is forbidden unless explicit recovery
  behavior and logging are present.

### VI. Release & Infrastructure Alignment

Application changes MUST ship in a way that preserves semantic versioning
and downstream infrastructure automation. Rationale: this repo produces
Docker container images consumed by Kubernetes deployments and
infrastructure repositories.

- Release-impacting changes MUST document whether they require a new
  Docker image tag, an infrastructure repository update, or a coordinated
  environment rollout.
- Tags and release notes MUST follow the documented semantic version
  workflow.
- Breaking runtime or contract changes (message format, environment
  variables, Redis key schemas) MUST trigger a MAJOR version discussion
  before merge.
- Required follow-up in deployment configuration MUST be captured in the
  plan, quickstart, or pull request notes.
- Changes to the Go version, Docker base image, or runtime dependencies
  MUST include compatibility verification steps.

## Engineering Standards

- Runtime code targets Go 1.24 and MUST remain compatible with the
  version declared in `go.mod` and `docker/Dockerfile`.
- Dependencies MUST be minimal, justified, and added through `go mod`.
- External integrations (Redis, MongoDB, S3, Flows API, gRPC) MUST
  prefer thin adapters behind interfaces so business logic remains
  testable with mocks.
- Documentation for new environment variables, message contracts,
  connection lifecycle changes, and rollout steps MUST be updated
  alongside code.
- Formatting and linting conventions MUST follow `gofmt` and
  `golangci-lint` standards; imports MUST be organized (stdlib, external,
  internal).

## Delivery Workflow

- Specs MUST capture user scenarios, edge cases, functional requirements,
  non-functional or operational requirements, and measurable success
  criteria before planning.
- Plans MUST include a Constitution Check covering readability, WebSocket
  contract boundaries, security and secrets, test coverage,
  observability, and release impact.
- Tasks MUST include mandatory test work, configuration or security work,
  and any infrastructure follow-up required for release.
- Pull requests MUST explain runtime impact, rollback considerations, and
  any manual deployment coordination.
- Complexity exceptions MUST be documented explicitly in the plan with
  the simpler alternative that was rejected.

## Governance

This constitution is the authoritative engineering policy for the Weni
WebChat Socket repository. All specifications, plans, tasks, and code
reviews MUST enforce it.

**Amendment Process**:
1. Propose changes in a pull request that updates
   `.specify/memory/constitution.md` and any affected templates or
   command docs.
2. Record the semantic version bump rationale in the Sync Impact Report.
3. Obtain approval from the maintainers responsible for application code
   and deployment workflow before merge.
4. Update downstream guidance when a principle changes behavior expected
   in specs, plans, tasks, CI, or release operations.

**Versioning Policy**:
- MAJOR: Remove or materially redefine a principle or governance rule.
- MINOR: Add a principle or section, or expand requirements in a way
  that changes expected workflow.
- PATCH: Clarify wording, examples, or non-semantic guidance.

**Compliance Review**:
- Every plan MUST pass the Constitution Check before design begins and
  after design is complete.
- Every pull request MUST show how tests, observability, and release
  impact were addressed.
- Reviewers MUST reject changes that bypass required tests, secret
  handling rules, or release coordination.
- Open exceptions MUST be documented in the plan or pull request and
  approved explicitly.

**Version**: 1.0.0 | **Ratified**: 2026-03-07 | **Last Amended**: 2026-03-07
