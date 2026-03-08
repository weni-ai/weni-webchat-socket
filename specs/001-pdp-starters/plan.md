# Implementation Plan: PDP Conversation Starters (WebSocket)

**Branch**: `001-pdp-starters` | **Date**: 2026-03-08 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-pdp-starters/spec.md`

## Summary

Add a new `get_pdp_starters` WebSocket event handler to the weni-webchat-socket server. When a registered client sends product data, the handler spawns a goroutine to invoke an AWS Lambda function (ARN from env var) that generates conversation starter questions. The response is sent back to the client as a `starters` event. A bounded semaphore limits concurrent invocations to protect server resources. When the Lambda ARN is not configured, the event is silently ignored (feature toggle).

## Technical Context

**Language/Version**: Go 1.24 (module `github.com/ilhasoft/wwcs`)  
**Primary Dependencies**: gorilla/websocket, aws-sdk-go v1, golang.org/x/sync (new), configor, logrus, testify, gomock  
**Storage**: N/A (Lambda manages its own DynamoDB cache)  
**Testing**: Go stdlib `testing`, testify/assert, gomock, table-driven tests, httptest  
**Target Platform**: Linux containers (Kubernetes)  
**Project Type**: WebSocket server (long-running service)  
**Performance Goals**: Zero blocking on read loop; Lambda goroutine up to 30s; no degradation to other events  
**Constraints**: Bounded concurrency via semaphore (configurable, default 50); explicit Lambda invocation timeout (configurable, default 35s); no new Redis/MongoDB usage  
**Scale/Scope**: Multi-pod deployment; existing 10k+ concurrent connections per pod

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Pre-design check

| Principle | Status | Notes |
| --------- | ------ | ----- |
| I. Clear, Idiomatic Go Packages | PASS | New `pkg/starters/` package with single responsibility (Lambda invocation). Handler delegates to service interface. New files stay under 500 lines. Pre-existing `client.go` (765 lines) is acknowledged in Complexity Tracking — handler addition is minimal (~40 lines). |
| II. WebSocket Contract & Configuration Discipline | PASS | New event `get_pdp_starters` documented in contracts. Config via `WWC_` env vars through configor. Validation at handler boundary. Explicit timeout on Lambda SDK call via `context.WithTimeout`. |
| III. Secrets, Security & Least Privilege | PASS | Lambda ARN is not a secret. AWS credentials flow through IAM role (existing pod role). No new secrets in env vars. IAM permission `lambda:InvokeFunction` documented in quickstart. |
| IV. Test-First Quality Gates | PASS | Interface-based design enables unit tests with mocks. Integration test plan for handler. Table-driven test pattern. |
| V. Observability & Operational Resilience | PASS | Structured logs with client_id, channel, lambda_arn. Error path distinguishes retriable (Lambda timeout/throttle) from permanent (validation). Goroutine cleanup on client disconnect. |
| VI. Release & Infrastructure Alignment | PASS | New env vars documented. Additive event type (no breaking change). Deployment notes in quickstart. No MAJOR version bump needed. |

### Post-design check

| Principle | Status | Notes |
| --------- | ------ | ----- |
| I. Clear, Idiomatic Go Packages | PASS | `pkg/starters/` contains: `service.go` (interface + Lambda impl), `service_mock.go` (generated mock). Handler in `client.go` is a thin delegation. |
| II. WebSocket Contract & Configuration Discipline | PASS | Event contracts documented in `contracts/websocket-events.md`. Config fields added to `config.Configuration` struct. Explicit `context.WithTimeout` on Lambda invocation (configurable via `WWC_LAMBDA_STARTERS_TIMEOUT_SEC`, default 35s). |
| III. Secrets, Security & Least Privilege | PASS | IAM permission requirement documented. No sensitive data in logs. Lambda response validated before forwarding. |
| IV. Test-First Quality Gates | PASS | `StartersService` interface enables gomock. Tests cover: happy path, Lambda error, validation failure, concurrency limit, ARN not configured, client disconnect. |
| V. Observability & Operational Resilience | PASS | Log fields: `client_id`, `channel`, `lambda_arn`, `error`. Silent feature toggle via empty ARN. Semaphore prevents resource exhaustion. |
| VI. Release & Infrastructure Alignment | PASS | 3 new env vars documented. No breaking changes. Deployment notes include IAM requirement. |

## Project Structure

### Documentation (this feature)

```text
specs/001-pdp-starters/
├── spec.md
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── websocket-events.md
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (via /speckit.tasks)
```

### Source Code (repository root)

```text
config/
└── config.go              # Add LambdaStarters fields to Configuration struct

pkg/
├── starters/              # NEW package
│   ├── service.go         # StartersService interface + LambdaStartersService implementation
│   ├── service_mock.go    # Generated mock (gomock)
│   └── service_test.go    # Unit tests for Lambda invocation logic
└── websocket/
    ├── application.go     # Add StartersService + semaphore to App struct
    ├── client.go          # Add "get_pdp_starters" case + GetPDPStarters handler method
    └── client_test.go     # Add tests for GetPDPStarters handler

api/
└── main.go                # Wire StartersService + semaphore into App
```

**Structure Decision**: New `pkg/starters/` package follows the existing pattern of `pkg/flows/`, `pkg/history/` — a focused package per external integration with an interface for testability. The handler method lives on `Client` in `client.go` consistent with all other event handlers.

## Design Decisions

### 1. New `pkg/starters/` package

**What**: A new package containing the `StartersService` interface and its Lambda implementation.

**Why**: Follows the project pattern (`flows.IClient`, `history.Service`). Keeps Lambda SDK usage out of the websocket package. Enables mocking in handler tests.

**Interface**:
```go
type StartersInput struct {
    Account     string            `json:"account"`
    LinkText    string            `json:"linkText"`
    ProductName string            `json:"productName,omitempty"`
    Description string            `json:"description,omitempty"`
    Brand       string            `json:"brand,omitempty"`
    Attributes  map[string]string `json:"attributes,omitempty"`
}

type StartersOutput struct {
    Questions []string `json:"questions"`
}

type StartersService interface {
    GetStarters(ctx context.Context, input StartersInput) (*StartersOutput, error)
}
```

### 2. Semaphore on App struct

**What**: `App` holds a `*semaphore.Weighted` initialized at startup with the configured max concurrency.

**Why**: The semaphore must be shared across all clients on the pod. `App` is the natural owner since it's already the shared state container. `TryAcquire(1)` provides non-blocking fail-fast behavior.

### 3. Handler flow

```
ParsePayload("get_pdp_starters")
  → return c.GetPDPStarters(payload, app)
      1. Check registration (c.ID, c.Callback) → if fail, return error
      2. Check Lambda ARN configured → if empty, debug log + return nil
      3. Validate required fields (account, linkText) → if fail, return error
      4. TryAcquire semaphore → if false, return error (capacity exceeded)
      5. go func() {
           defer sem.Release(1)
           ctx, cancel := context.WithTimeout(context.Background(), timeout)
           defer cancel()
           result, err := app.StartersService.GetStarters(ctx, input)
           if err → c.Send(error payload), log
           else → c.Send(starters payload), log
         }()
      6. return nil (read loop continues immediately)
```

**Key**: Synchronous validation errors (steps 1, 3, 4) are returned to `ParsePayload`, which sends the error payload via the standard Read loop error handler — consistent with existing handlers like `VerifyContactTimeout` and `SetCustomField`. Only after the goroutine is spawned (step 5) does the handler return `nil`. The goroutine uses `context.WithTimeout` to bound the Lambda SDK call, preventing indefinite blocking if the endpoint is unreachable.

### 4. Feature toggle via empty ARN

**What**: When `WWC_LAMBDA_STARTERS_ARN` is empty, the handler returns `nil` immediately with a debug log.

**Why**: Avoids error noise in environments where the feature isn't deployed. The frontend handles "no response" gracefully (no starters shown).

## Complexity Tracking

| Aspect | Decision | Rationale |
| ------ | -------- | --------- |
| New package (`pkg/starters/`) | Justified | Follows existing `pkg/flows/`, `pkg/history/` pattern. Single responsibility. |
| New dependency (`golang.org/x/sync`) | Justified | Official Go team package. Minimal footprint. Provides idiomatic `TryAcquire` for fail-fast. |
| `client.go` exceeds 500 lines (pre-existing) | Acknowledged | File is already 765 lines before this feature. The new `GetPDPStarters` handler adds ~40 lines of thin delegation. Refactoring `client.go` into smaller files is recommended as a separate follow-up but is out of scope for this feature. |
| Lambda invocation metrics deferred | Acknowledged | Spec explicitly defers feature-specific Prometheus metrics to Out of Scope. Lambda errors sent via `c.Send()` inside goroutines bypass the existing ParsePayload error rate counter. A follow-up should add at minimum a starters request counter (total/success/error). Structured logs (FR-009) provide interim observability. |
