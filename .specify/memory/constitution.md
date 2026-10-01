<!--
  SYNC IMPACT REPORT
  ==================
  Version change: 1.0.0 → 2.0.0 (MAJOR: principles redefined and
  re-scoped to adopt the VTEX CX engineering + backend base constitutions)

  Modified principles (old → new):
    - I. Clear, Idiomatic Go Packages → XIII. Explicit Over Clever
      (merged; GoDoc, file-size and dead-code rules preserved)
    - II. WebSocket Contract & Configuration Discipline → split into
      V. Never Trust the Client, VI. Versioned Contracts and
      VII. Fail Gracefully and Predictably
    - III. Secrets, Security & Least Privilege → IV. Secrets, Security
      & Least Privilege (expanded: runtime injection, dependency
      vulnerability checks)
    - IV. Test-First Quality Gates → XII. Tests Exercise Flows
      (redefined: flow-level coverage incl. failure paths is mandatory;
      test-first and regression rules preserved; golangci-lint moved
      from MUST-in-CI to SHOULD because CI does not run it today)
    - V. Observability & Operational Resilience → X. Observability and
      XI. Diagnosable Errors
    - VI. Release & Infrastructure Alignment → XVI. Changelog and
      Release Alignment
  Added principles:
    - I. Reviewed, Protected Main Branch
    - II. Atomic Conventional Commits
    - III. Contained Changes
    - VIII. Bounded Retry Over REST
    - IX. Stateless, Horizontally Scalable Instances
    - XIV. Specification Traceability
    - XV. No Silent Divergence
  Added sections: None (Engineering Standards, Delivery Workflow and
    Governance rewritten to match the new principles)
  Removed sections: None

  Templates requiring updates:
    - .specify/templates/plan-template.md ✅ no change needed
      (Constitution Check gates are derived from this file)
    - .specify/templates/spec-template.md ⚠ pending: has no
      "Inheritance from Product Spec" section required by XIV
    - .specify/templates/tasks-template.md ✅ no change needed
    - .cursor/commands/speckit.*.md ✅ no change needed

  Follow-up TODOs:
    - TODO(PRODUCT_SPEC_REPO): identify the product specification
      repository that engineering specs in specs/ MUST pin (XIV).
    - TODO(SPEC_001_INHERITANCE): specs/001-pdp-starters predates XIV
      and lacks the inheritance section; add it before amending it.
    - TODO(PEAK_LOAD): declare the service peak (concurrent WebSocket
      connections, messages/s) in the next engineering spec (IX).
    - TODO(BRANCH_PROTECTION): confirm GitHub branch protection on main
      requires 1 approval + green CI and blocks direct pushes (I).
    - TODO(HTTP_TIMEOUTS): pkg/flows, pkg/elevenlabs and the callback
      POST in pkg/websocket/client.go use HTTP clients without a timeout
      (VII).
    - TODO(CALLBACK_RETRY): the callback POST in pkg/websocket/client.go
      has no bounded retry (VIII).
    - TODO(CORRELATION_ID): no per-request/per-message correlation
      identifier is attached to error logs today (X, XI).
    - TODO(CI_GOFMT): CI runs `gofmt -d .`, which never fails; the gate
      SHOULD fail on unformatted files (e.g. `test -z "$(gofmt -l .)"`).
    - TODO(CI_VULN_SCAN): no dependency vulnerability scan in CI
      (e.g. govulncheck) (IV).
    - TODO(CI_LINT): golangci-lint is not run in CI (XII).

  Provenance:
    - Source: weni-ai/vtex-cx-engineering-constitutions (main)
    - Bases: base-constitution.md, backend/base-constitution.md
    - Domains: backend
    - Project layer: weni-webchat-socket constitution v1.0.0 (2026-03-07)
-->

# Weni WebChat Socket Constitution

## Core Principles

### I. Reviewed, Protected Main Branch

All code MUST enter `main` through a pull request. A merge MUST require at
least one approved review and a green `ci` workflow run. Direct pushes to
`main` MUST be blocked by GitHub branch protection.

**Rationale:** the policy is only real when the platform enforces it.
Peer review and a protected `main` keep history auditable and stop
unreviewed changes from reaching the image that is deployed to Kubernetes.

### II. Atomic Conventional Commits

Commits MUST follow Conventional Commits: `<type>: <description>`. Allowed
types are `feat`, `fix`, `docs`, `refactor`, `test` and `chore`. The
description MUST be imperative, specific and no longer than 50 characters.
Each commit MUST contain exactly one logical change.

**Rationale:** conventional commits feed `CHANGELOG.md` entries and
SemVer decisions; atomic commits make bisecting, reverting and reviewing
cheap.

### III. Contained Changes

A change MUST be limited to the context it was asked to address.
Refactoring, renaming, reformatting or behaviour adjustments outside that
context MUST NOT ride along; each belongs to its own pull request. A
change that stays in scope MAY span several atomic commits (Principle II).

**Rationale:** a change that reaches beyond its stated scope is a change
nobody reviewed on purpose. It hides the intended fix inside unrelated
edits and turns a revert into a choice between losing the fix and keeping
an unrelated regression.

### IV. Secrets, Security & Least Privilege

- Secrets (S3 keys, JWT signing keys, Redis/MongoDB URIs with
  credentials, ElevenLabs/Flows tokens, Sentry DSN) MUST never be
  committed, hardcoded or written to logs.
- Secrets MUST come from an external secrets manager and be injected at
  runtime as `WWC_*` environment variables, read only through the
  `config` package or an isolated adapter (e.g. `pkg/jwt`). Defaults in
  `config/config.go` MUST be safe for local development only.
- Access to AWS (S3, Lambda), Redis and MongoDB MUST follow least
  privilege; any new permission MUST be documented in the plan.
- Dependencies MUST be added through `go mod` from trusted sources and
  MUST be checked for known vulnerabilities. Dependencies that affect
  authentication, transport or cryptography MUST be justified in the plan.

**Rationale:** this service handles JWTs, cloud credentials and user
session data. Leaked credentials and vulnerable dependencies are among the
most damaging breaches, and prevention is far cheaper than remediation.

### V. Never Trust the Client

Everything that reaches the server from outside — webchat browsers over
WebSocket, gRPC callers, Redis stream payloads from producers, and
responses from Flows, VTEX, ElevenLabs or Lambda — MUST be treated as
potentially malicious, incomplete or incorrect until validated.

- Every incoming payload MUST be validated for type, format, range and
  business rules at the boundary (`pkg/websocket` handlers, `pkg/grpc`
  server, `pkg/streams` consumers) before it is dispatched to internal
  logic.
- `register` MUST remain the mandatory first message; no other event may
  be processed for an unregistered connection.
- Authorization MUST be enforced on the server for every event: origin
  and channel allowed-domain checks, and a connection MUST only act on its
  own registered session, regardless of any check made by the widget.
- Error payloads sent to clients MUST NOT leak internal state, stack
  traces or upstream response bodies.

**Rationale:** the webchat widget runs in arbitrary browsers and can be
modified or bypassed. Server-side validation is what prevents injection,
session hijacking and data corruption that client checks can never stop.

### VI. Versioned Contracts

The public interfaces of this service are: WebSocket message payloads
(documented in `README.md`), the gRPC API in `pkg/grpc/proto`, Redis key
and stream schemas shared between instances, HTTP endpoints (e.g. health
checks) and `WWC_*` environment variables.

- Any change to these interfaces MUST be versioned following SemVer.
- Changes MUST be backward compatible or ship with an announced
  deprecation path; a breaking change MUST bump the service MAJOR version
  and be discussed before merge.
- Silent breaking changes MUST NOT be introduced.
- Contract documentation (README examples, proto files, env var table)
  MUST be updated in the same pull request as the change.

**Rationale:** webchat widgets, Flows/Courier and infrastructure
manifests depend on these contracts. Explicit versioning and deprecation
give consumers a predictable path to adapt without outages, and Redis
schema drift breaks rolling deployments where old and new pods coexist.

### VII. Fail Gracefully and Predictably

- Every call to an external dependency (Redis, MongoDB, S3, Lambda,
  Flows, VTEX, ElevenLabs, callback URLs) MUST have an explicit timeout
  and MUST NOT block indefinitely, including inside goroutines spawned
  from the WebSocket read loop.
- Failures MUST be handled explicitly and surfaced as consistent,
  well-defined error responses (`type: "error"` over WebSocket, gRPC
  status codes) — never as panics, dropped connections without reason or
  leaked internals.
- Error handling MUST distinguish retriable upstream failures from
  permanent validation or contract errors.
- Silent error swallowing MUST NOT occur; every ignored error MUST have
  an explicit recovery path and a log entry.

**Rationale:** failure is a certainty, not an edge case. A long-lived
WebSocket server that blocks on one slow dependency exhausts goroutines
and connections for every client on the pod.

### VIII. Bounded Retry Over REST

When data is propagated to another service over REST (callbacks to
Courier/Flows, Flows API writes such as contact field updates):

- A failed call MUST be retried rather than dropped, but only on
  failures that could succeed on another attempt: connection error,
  timeout, HTTP 5xx or HTTP 429. It MUST NOT be retried on any other 4xx.
- Retries MUST only be applied to operations that are idempotent or
  protected by a deduplication key; a non-idempotent operation MUST be
  made idempotent rather than left without retry.
- Every retry policy MUST define a maximum number of attempts and a
  backoff strategy as named constants or configuration; unbounded retry
  MUST NOT be used.
- When attempts are exhausted, the failure MUST be logged at error level
  (reaching Sentry) and MUST remain recoverable — it MUST NOT be silently
  discarded.

**Rationale:** propagation fails for transient reasons far more often
than permanent ones, so retrying keeps services converging. Retrying
rejected or non-idempotent requests multiplies load or duplicates effects,
and unbounded retry amplifies an outage exactly when a dependency is
degraded.

### IX. Stateless, Horizontally Scalable Instances

- Every instance MUST be replaceable and horizontally scalable. State
  that outlives a single request or event — client registry, session
  ownership, pending deliveries, history — MUST live in Redis or MongoDB,
  shared by all instances.
- Live WebSocket connections are inherently pod-local; they MUST be
  addressable cluster-wide through the Redis client registry and stream
  routing (`pkg/websocket/clientmanager.go`, `pkg/streams`), so any
  instance can deliver to any client and a pod restart loses no
  undelivered message.
- In-process caches (e.g. `pkg/memcache`) are allowed only for
  recomputable data whose source of truth is external, and MUST be
  bounded by a TTL.
- The peak load the service must sustain (concurrent connections and
  message throughput) MUST be declared in the engineering spec of any
  feature that changes it, stated as peak and not average.

**Rationale:** capacity is a design input, not something discovered
during a seasonal sales peak. Statelessness is what makes adding pods a
valid answer to load; a pod holding authoritative state can only be
restarted, not scaled out. The connection exception is explicit because
sockets cannot be moved, but the routing around them can be shared.

### X. Observability

- Logs MUST be structured (`logrus` fields) and MUST include enough
  context to trace the operation (channel UUID, client identifier,
  connection ID, message type) but MUST NOT contain secrets, tokens or
  sensitive personal data.
- Errors MUST be traceable across components (WebSocket, gRPC, Redis
  streams, outbound HTTP) through a correlation identifier.
- Prometheus metrics (`pkg/metric`) MUST be maintained for key
  operations: active connections, message throughput, error rates,
  Redis pool health and health-check latency.
- Features affecting reconnection, session takeover, concurrency, pool
  management or graceful shutdown MUST document their operational impact.

**Rationale:** persistent connections make drops, message loss and
leaks hard to reproduce. Structured, privacy-safe telemetry is what makes
incidents diagnosable without creating new data-exposure risks.

### XI. Diagnosable Errors

Every error reported to Sentry — every `logrus` entry at `error`, `fatal`
or `panic` level, via the `logrus_sentry` hook — MUST carry enough context
to be located and filtered without reproducing it: at minimum the project
or channel identifier (`channel_uuid`), the account identifier when known
(e.g. VTEX account), the user identifier (opaque contact/client ID from
`register`) and the correlation identifier of the request or message.
Those identifiers MUST be opaque. Names, e-mail addresses, phone numbers,
government identifiers and free-form contact fields MUST NOT be attached
to an error report under any circumstance.

**Rationale:** an error without identifying context can be counted but
not investigated. Opaque identifiers give exactly the filtering an
investigation needs while keeping reports free of personal data, as
Principle X requires.

### XII. Tests Exercise Flows

- Every flow (a WebSocket event, gRPC method, stream consumer or
  outbound integration use case) MUST have at least one test covering
  the complete use case from input to resulting effect, e.g. a client
  sending an event and receiving the response or the expected Redis/HTTP
  side effect.
- Every flow MUST cover its success path and its failure paths; an
  error path that no test exercises MUST NOT be considered covered.
- Unit tests of single functions SHOULD be used for edge cases and
  input variations, but MUST NOT be the only coverage a flow has.
- Tests for new behavior MUST be written to fail before the
  implementation and pass afterward; bug fixes MUST include a regression
  test whenever technically feasible.
- Changed packages SHOULD keep at least 80% line coverage unless the
  plan records an approved exception.
- `gofmt` and `go test ./...` MUST pass locally and in CI before merge;
  `golangci-lint` SHOULD pass locally.

**Rationale:** a suite made only of isolated method tests can be green
while the composition is broken, because the bug lives in how the pieces
interact. Failure paths are the least exercised in development and the
most expensive in a real-time server.

### XIII. Explicit Over Clever

- What a piece of code does MUST be evident where it happens. Hidden
  side effects and implicit control flow MUST NOT be introduced to save
  lines. Handlers MUST delegate non-trivial logic to focused packages.
- Any literal that carries meaning — a timeout, TTL, limit, retry count,
  poll interval — MUST be a named constant or `config` field rather than
  an inline value. Literals with no meaning beyond their value (index 0,
  increment 1) are exempt.
- Exported types and functions MUST have GoDoc comments describing
  behavior and constraints. Other comments MUST explain why (constraint,
  trade-off, infrastructure caveat), never restate the code.
- Debug code, dead branches and commented-out implementations MUST NOT
  be committed. Files exceeding roughly 500 lines MUST be justified in
  the plan or split.

**Rationale:** code is read far more often than written, usually under
incident pressure by someone without the original context. An unnamed
literal is a decision nobody can review, and comments on the why preserve
what the code cannot carry without going stale.

### XIV. Specification Traceability

Every engineering spec under `specs/` MUST derive from exactly one
approved product spec and MUST reference it through an immutable pinned
version (commit or tag); a mutable URL or ID alone MUST NOT be used. The
product spec MUST exist and be tagged before the engineering spec is
created. An engineering spec MUST NOT redefine the problem, scope,
success criteria or binding decisions it inherits. A technical
architecture document SHOULD be produced for non-trivial features; when
it exists it MUST be linked and pinned, but its absence MUST NOT block
the engineering spec.

Every engineering spec MUST open with this section:

```
## Inheritance from Product Spec
- Product Spec: <title> — <URL>
- Pinned version: <commit/tag>
- Architecture doc: <none | URL + commit/tag>
- Inherited binding decisions: <short list>
- Scope of this spec: <slice implemented by this repo>
- Divergences: <none | link to amendment>
```

Specs created before constitution v2.0.0 (e.g. `specs/001-pdp-starters`)
MUST gain this section before they are next amended.

**Rationale:** pinning guarantees every team implements the same version
of a feature instead of divergent readings of a spec that changed
mid-flight, and a single format keeps the link machine-checkable across
repositories.

### XV. No Silent Divergence

When a technical need contradicts something inherited from the product
spec — scope, success criteria or a binding decision — it MUST NOT be
implemented silently in code. It MUST be raised as an amendment in the
product repository and recorded in the `Divergences` field, linking to
that amendment. Once the amendment is approved and tagged, `Pinned
version` MUST be updated. A technical difference that contradicts nothing
inherited is an implementation decision and MUST live in the engineering
spec.

**Rationale:** with the product spec as the single source of truth, a
silent code deviation makes intent and implementation drift apart with
no audit trail.

### XVI. Changelog and Release Alignment

- `CHANGELOG.md` MUST be updated for every production release, and every
  user-facing change MUST appear under its version. The latest changelog
  version MUST match the production tag (enforced by the
  `verify-changelog` job in `build-push-deploy.yaml`).
- Service versions and tags MUST follow SemVer (`X.Y.Z`, with
  `-develop` / `-staging` suffixes for pre-production deploys).
- This repository is a deployed service, not a public library, so the
  Keep a Changelog format is not mandatory; entries SHOULD use the
  Conventional Commit types of Principle II as categories.
- Release-impacting changes MUST state whether they need a new image
  tag, an update in `weni-ai/kubernetes-manifests-platform`, new
  environment variables or a coordinated rollout.
- Changes to the Go version, Docker base image or runtime dependencies
  MUST include compatibility verification steps.

**Rationale:** the tag drives image build and the Kubernetes manifest
patch. A changelog aligned with SemVer tells operators and consumers what
changed and how risky the upgrade is.

## Engineering Standards

- Runtime: Go 1.24, kept consistent across `go.mod`, `docker/Dockerfile`
  and `.github/workflows/ci.yml`.
- Entry points: `api/main.go` (WebSocket/HTTP) and `grpc/main.go` (gRPC);
  domain code lives in `pkg/<concern>` with a single responsibility per
  package.
- External integrations (Redis, MongoDB, S3, Lambda, Flows, VTEX,
  ElevenLabs) MUST sit behind thin interfaces so business logic is
  testable with mocks (`*_mock.go`, `golang/mock`).
- Configuration MUST be loaded through `config/config.go` from `WWC_*`
  environment variables; new variables MUST be added to the README table.
- Formatting MUST follow `gofmt`; imports MUST be grouped stdlib,
  external, internal.

## Delivery Workflow

- Specs MUST open with the inheritance section (XIV) and capture user
  scenarios, edge cases, functional and operational requirements,
  declared peak load when relevant (IX) and measurable success criteria.
- Plans MUST include a Constitution Check covering: client-input
  validation and authorization, contract versioning, timeouts and
  bounded retries, statelessness and peak load, secrets, observability
  and Sentry context, flow-level tests including failure paths, and
  release impact.
- Tasks MUST include flow tests for success and failure paths,
  configuration or security work, and any infrastructure follow-up.
- Pull requests MUST stay within their stated scope (III) and explain
  runtime impact, rollback considerations and deployment coordination.
- Complexity exceptions MUST be documented in the plan with the simpler
  alternative that was rejected.

## Governance

This constitution is the authoritative engineering policy for the Weni
WebChat Socket repository and supersedes conflicting practices. It
specializes the VTEX CX engineering and backend base constitutions;
where a project rule and a base rule conflict, the base rule prevails
unless an exception is justified in the affected principle.

**Amendment Process**:
1. Propose changes in a pull request that updates
   `.specify/memory/constitution.md`, regenerating from the base
   constitutions with `setup-engineering` when the bases change.
2. Record the SemVer bump rationale in the Sync Impact Report.
3. Obtain approval from the maintainers responsible for application code
   and deployment before merge.
4. Update affected templates and command docs when a principle changes
   what specs, plans, tasks, CI or releases are expected to contain.

**Versioning Policy**:
- MAJOR: remove or materially redefine a principle or governance rule.
- MINOR: add a principle or section, or materially expand guidance.
- PATCH: clarify wording or fix non-semantic issues.

**Compliance Review**:
- Every plan MUST pass the Constitution Check before design and again
  after design.
- `/speckit.analyze` MUST treat any conflict with a MUST statement as
  CRITICAL.
- Reviewers MUST reject changes that bypass required tests, secret
  handling, contract versioning or release coordination.
- Exceptions MUST be documented in the plan or pull request and approved
  explicitly.

**Version**: 2.0.0 | **Ratified**: 2026-03-07 | **Last Amended**: 2026-10-01
