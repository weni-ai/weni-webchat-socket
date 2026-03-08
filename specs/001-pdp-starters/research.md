# Research: PDP Conversation Starters

**Branch**: `001-pdp-starters` | **Date**: 2026-03-08

## 1. AWS Lambda Invocation from Go (SDK v1)

**Decision**: Use `github.com/aws/aws-sdk-go/service/lambda` (SDK v1) for synchronous `RequestResponse` invocation.

**Rationale**: The project already depends on `github.com/aws/aws-sdk-go v1.38.8` for S3 operations (`pkg/websocket/helper.go`). Using the same SDK version for Lambda avoids adding a new major dependency (v2) and keeps the AWS session/credentials model consistent.

**Alternatives considered**:
- AWS SDK v2 (`github.com/aws/aws-sdk-go-v2`): Would require a second AWS configuration model alongside the existing v1 setup. Rejected for scope and consistency.
- HTTP invocation via Lambda Function URL: Would require a publicly exposed Lambda or VPC configuration. Rejected because `Invoke` via SDK uses IAM roles natively.

**Key implementation details**:
- `lambda.New(session)` creates the client; `Invoke(&lambda.InvokeInput{FunctionName, Payload})` calls the function synchronously.
- The session can reuse the existing AWS region from `config.Get().S3.Region` or a dedicated env var.
- `InvocationType: "RequestResponse"` (default) returns the Lambda response body.
- The Lambda response payload is JSON-decoded into a struct with `Questions []string`.

## 2. Bounded Semaphore Pattern in Go

**Decision**: Use `golang.org/x/sync/semaphore` (weighted semaphore) to limit concurrent Lambda invocations.

**Rationale**: Standard library doesn't provide a semaphore. The `golang.org/x/sync` package is the idiomatic Go solution, maintained by the Go team, and adds minimal footprint. A channel-based semaphore is also viable but `x/sync/semaphore` provides `TryAcquire` for non-blocking fail-fast behavior, which matches the spec requirement.

**Alternatives considered**:
- Buffered channel as semaphore (`make(chan struct{}, N)`): Viable but `TryAcquire` semantics require `select`+`default`, less readable. Rejected for clarity.
- Custom worker pool: Over-engineered for this use case where each request is independent. Rejected for simplicity.

**Key implementation details**:
- `semaphore.NewWeighted(int64(maxConcurrent))` at startup.
- `sem.TryAcquire(1)` before spawning goroutine: if `false`, return error immediately (fail fast).
- `defer sem.Release(1)` inside the goroutine.
- Max concurrency configurable via env var (e.g., `WWC_LAMBDA_STARTERS_MAX_CONCURRENT`, default 50).

## 3. Goroutine Safety for Client.Send

**Decision**: Call `c.Send()` directly from the Lambda goroutine, relying on the existing `sync.Mutex`.

**Rationale**: `Client.Send()` already uses `c.mu.Lock()` to serialize WebSocket writes. The streams router already calls `Send` from separate goroutines. No additional synchronization is needed.

**Alternatives considered**:
- Dedicated send channel per client: Would require refactoring the Client struct and all existing send paths. Rejected for scope.

## 4. Client Disconnection Detection

**Decision**: Check `c.Send()` error return. If sending fails, log the error and return (goroutine exits cleanly).

**Rationale**: When a client disconnects, `c.Conn.WriteJSON` returns an error (broken pipe, closed connection). The existing `isBenignConnectionError()` helper can classify these. No additional mechanism needed.

**Alternatives considered**:
- Context cancellation per client: Would require adding a `context.Context` to the Client struct and propagating cancellation on unregister. Viable but out of scope for this feature.

## 5. Configuration Approach

**Decision**: Add new fields to the `config.Configuration` struct using configor tags, consistent with existing pattern.

**Rationale**: The project uses `configor` with `WWC_` prefix for all configuration. Adding fields to the existing struct keeps the pattern uniform.

**New env vars**:
- `WWC_LAMBDA_STARTERS_ARN`: Lambda function ARN (empty = feature disabled)
- `WWC_LAMBDA_STARTERS_MAX_CONCURRENT`: Max concurrent invocations per pod (default: 50)
- `WWC_LAMBDA_STARTERS_REGION`: AWS region for Lambda client (falls back to `WWC_S3_REGION`)

## 6. Explicit Timeout on Lambda Invocation

**Decision**: Use `context.WithTimeout` with a configurable timeout (default 35s) on the `GetStarters` call inside the goroutine.

**Rationale**: Constitution principle II requires "Network calls to external services MUST set explicit timeouts." The Lambda has a 30s execution timeout, but the SDK HTTP call to the Lambda service endpoint could hang indefinitely if DNS or the endpoint itself is unreachable. A 35s timeout (5s buffer over the 30s Lambda timeout) ensures the goroutine is bounded.

**Alternatives considered**:
- Relying on Lambda's own 30s timeout: The Lambda execution timeout doesn't protect against network-level hangs (DNS, TCP connect). Rejected because the goroutine could block forever.
- Configuring HTTP client timeout on the `*lambda.Lambda` SDK client: Viable but less granular — `context.WithTimeout` is the idiomatic Go approach and enables per-request cancellation.

**Key implementation details**:
- Timeout configurable via `WWC_LAMBDA_STARTERS_TIMEOUT_SEC` (default: 35).
- `context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)` created inside the goroutine, before calling `GetStarters`.
- `defer cancel()` ensures resources are released even if Lambda responds early.

## 7. Lambda Service Abstraction

**Decision**: Define a `StartersService` interface to decouple the handler from the AWS SDK, enabling unit testing with mocks.

**Rationale**: The project uses interfaces for external services (`flows.IClient`, `history.Service`). Following this pattern for the Lambda invocation keeps the codebase consistent and testable. The interface can be mocked with `gomock` in tests.

**Interface shape**:
```go
type StartersService interface {
    GetStarters(ctx context.Context, input StartersInput) (*StartersOutput, error)
}
```
