# Tasks: PDP Conversation Starters (WebSocket)

**Input**: Design documents from `/specs/001-pdp-starters/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/

**Tests**: Included per Constitution principle IV (Test-First Quality Gates).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add new dependency and configuration fields required by all user stories

- [x] T001 Add `golang.org/x/sync` dependency via `go get golang.org/x/sync`
- [x] T002 Add Lambda starters config fields (`LambdaStartersARN`, `LambdaStartersMaxConcurrent`, `LambdaStartersRegion`, `LambdaStartersTimeoutSec`) to `Configuration` struct in `config/config.go` with env tags `WWC_LAMBDA_STARTERS_ARN`, `WWC_LAMBDA_STARTERS_MAX_CONCURRENT` (default 50), `WWC_LAMBDA_STARTERS_REGION`, `WWC_LAMBDA_STARTERS_TIMEOUT_SEC` (default 35)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Create the starters service package and wire it into the application. MUST complete before any user story work.

**CRITICAL**: No user story work can begin until this phase is complete.

- [x] T003 Create `pkg/starters/service.go` with `StartersService` interface, `StartersInput`/`StartersOutput` types, and `LambdaStartersService` implementation that uses AWS SDK v1 `lambda.InvokeWithContext` with `RequestResponse` invocation type. The constructor should accept `*lambda.Lambda` client. Use `InvokeWithContext(ctx, ...)` (not `Invoke`) to respect the `context.Context` timeout/cancellation passed by the caller. Marshal `StartersInput` as JSON payload, unmarshal response into `StartersOutput`, and handle `FunctionError` in the response.
- [x] T004 Generate mock for `StartersService` interface in `pkg/starters/service_mock.go` using `mockgen` (gomock)
- [x] T005 Add `StartersService starters.StartersService` field and `StartersSem *semaphore.Weighted` field to `App` struct in `pkg/websocket/application.go`. Update `NewApp` constructor to accept and assign both. If `StartersService` is nil or semaphore is nil, the handler will treat it as feature disabled.
- [x] T006 Wire `StartersService` and semaphore into `App` in `api/main.go`: read config, create AWS Lambda session (using `LambdaStartersRegion` with fallback to `S3.Region`), create `LambdaStartersService`, create `semaphore.NewWeighted(config.LambdaStartersMaxConcurrent)`. Skip creation if `LambdaStartersARN` is empty (pass nil to App).

**Checkpoint**: Foundation ready — `StartersService` is wired, semaphore is initialized, mock is available. User story implementation can begin.

---

## Phase 3: User Story 1 — Requisição assíncrona de starters via WebSocket (Priority: P1) MVP

**Goal**: A registered client sends `get_pdp_starters` with product data and receives a `starters` response with generated questions, without blocking the WebSocket read loop.

**Independent Test**: Connect a WebSocket client, register, send `get_pdp_starters` with valid product data, verify `starters` event is received with `data.questions` array.

### Tests for User Story 1

> **Write these tests FIRST, ensure they FAIL before implementation**

- [x] T007 [P] [US1] Write unit tests for `LambdaStartersService.GetStarters` happy path in `pkg/starters/service_test.go`: mock the Lambda Invoke call, verify correct payload marshaling, verify response unmarshaling into `StartersOutput`
- [x] T008 [P] [US1] Write handler test for `GetPDPStarters` happy path in `pkg/websocket/client_test.go`: use mock `StartersService` returning valid questions, verify client receives `IncomingPayload{Type: "starters", Data: {"questions": [...]}}`. Use table-driven test pattern consistent with existing `ttParsePayload` tests.

### Implementation for User Story 1

- [x] T009 [US1] Add `"get_pdp_starters"` case to `ParsePayload` switch in `pkg/websocket/client.go` that calls `return c.GetPDPStarters(payload, app)` — consistent with existing handler pattern where synchronous errors are returned to the Read loop
- [x] T010 [US1] Implement `GetPDPStarters(payload OutgoingPayload, app *App) error` method on `Client` in `pkg/websocket/client.go` with the full handler flow: (1) check registration — if fail, return `ErrorNeedRegistration` (Read loop sends error payload), (2) check `app.StartersService` not nil (ARN configured) — if nil, debug log + return nil, (3) validate required fields `account` and `linkText` from `payload.Data` — if fail, return validation error, (4) `TryAcquire` semaphore — if false, return capacity error, (5) spawn goroutine that: defers `sem.Release(1)`, creates `context.WithTimeout(context.Background(), time.Duration(config.Get().LambdaStartersTimeoutSec)*time.Second)` + `defer cancel()`, calls `app.StartersService.GetStarters(ctx, input)`, on success sends `IncomingPayload{Type: "starters", Data: {"questions": output.Questions}}`, on error sends `IncomingPayload{Type: "error", Error: err.Error()}` with `isBenignConnectionError` check on send failure, (6) return nil (read loop continues)

**Checkpoint**: US1 complete — happy path works end-to-end. Client sends product data, receives starters asynchronously.

---

## Phase 4: User Story 2 — Tratamento de erros na invocação da Lambda (Priority: P2)

**Goal**: Lambda failures (timeout, throttling, invalid response) result in error payloads to the client without disrupting the connection or other clients.

**Independent Test**: Mock `StartersService` to return errors or invalid responses, verify the client receives appropriate error payloads and the WebSocket connection remains functional.

### Tests for User Story 2

- [x] T011 [P] [US2] Write unit test for `LambdaStartersService.GetStarters` error paths in `pkg/starters/service_test.go`: Lambda returns `FunctionError`, Lambda returns non-JSON response, Lambda returns JSON without `questions` field, Lambda returns empty `questions` array
- [x] T012 [P] [US2] Write handler test for Lambda invocation failure in `pkg/websocket/client_test.go`: mock `StartersService` returning error, verify client receives `IncomingPayload{Type: "error"}` with descriptive message
- [x] T013 [P] [US2] Write handler test for concurrency limit exceeded in `pkg/websocket/client_test.go`: create semaphore with weight 0, call `GetPDPStarters`, verify client receives error payload with capacity message
- [x] T014 [US2] Write handler test for client disconnect during goroutine in `pkg/websocket/client_test.go`: close the WebSocket connection before the mock service returns, verify goroutine completes without panic (use a channel-synchronized mock that blocks until signaled)
- [x] T014b [P] [US2] Write handler test for rapid sequential requests from same client in `pkg/websocket/client_test.go`: send two `get_pdp_starters` events in quick succession from the same registered client, verify both produce independent responses without interference or race conditions

### Implementation for User Story 2

> Implementation already covered by T010 (error handling is built into the goroutine body). These tests validate the error paths.

- [x] T015 [US2] Add structured log fields (`client_id`, `channel`, `lambda_arn`, `error`) to error paths in `GetPDPStarters` goroutine in `pkg/websocket/client.go` using `log.WithFields` consistent with existing handler patterns

**Checkpoint**: US2 complete — all error scenarios tested. Lambda failures produce error payloads, connections survive, no goroutine leaks.

---

## Phase 5: User Story 3 — Validação de pré-condições (Priority: P3)

**Goal**: Invalid requests (unregistered client, missing required fields, feature disabled) are rejected early without invoking the Lambda.

**Independent Test**: Send `get_pdp_starters` without registration, with missing fields, or with feature disabled, and verify appropriate errors or silent behavior.

### Tests for User Story 3

- [x] T016 [P] [US3] Write handler test for unregistered client in `pkg/websocket/client_test.go`: client with empty `ID`/`Callback` sends `get_pdp_starters`, verify `ErrorNeedRegistration` is returned
- [x] T017 [P] [US3] Write handler test for missing required fields in `pkg/websocket/client_test.go`: registered client sends `get_pdp_starters` with empty `account`, empty `linkText`, nil `data`, verify validation error returned without Lambda invocation (mock `StartersService` should NOT be called)
- [x] T018 [US3] Write handler test for ARN not configured (feature toggle) in `pkg/websocket/client_test.go`: `App.StartersService` is nil, registered client sends `get_pdp_starters`, verify no error returned, no payload sent to client, mock verifies no `GetStarters` call

### Implementation for User Story 3

> Implementation already covered by T010 (validation steps 1–3 in the handler). These tests validate those paths.

**Checkpoint**: US3 complete — validation prevents unnecessary Lambda invocations. Feature toggle works silently.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final verification and cleanup

- [x] T019 Update `pkg/websocket/testmain_test.go` to set `WWC_LAMBDA_STARTERS_ARN` and `WWC_LAMBDA_STARTERS_MAX_CONCURRENT` env vars for test suite if needed by any test
- [x] T020 Run full test suite (`go test ./...`) and verify all tests pass, no race conditions (`go test -race ./...`)
- [x] T021 Verify quickstart.md steps: set env vars, build, connect WebSocket, send `get_pdp_starters`, observe response or silent ignore

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 completion — BLOCKS all user stories
- **User Stories (Phase 3–5)**: All depend on Phase 2 completion
  - US1 (Phase 3) must complete before US2 (Phase 4) — US2 tests validate US1's error paths
  - US3 (Phase 5) can run in parallel with US2 after US1 is complete — tests independent validation paths
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: Can start after Phase 2 — MVP, delivers core value
- **US2 (P2)**: Depends on US1 implementation (T009, T010) — tests error paths of existing code
- **US3 (P3)**: Depends on US1 implementation (T009, T010) — tests validation paths. Can run in parallel with US2

### Within Each User Story

- Tests written FIRST, verified to FAIL
- Implementation makes tests PASS
- Story validated independently at checkpoint

### Parallel Opportunities

- T001 and T002 (Setup) are sequential (T002 depends on go.mod being clean)
- T003 and T004 are sequential (mock depends on interface)
- T005 and T006 are sequential (main.go depends on App struct)
- T007 and T008 can run in parallel (different files)
- T011, T012, T013 can run in parallel (different test cases, same files but independent)
- T016 and T017 can run in parallel (independent test cases)
- US2 and US3 can run in parallel after US1 completes

---

## Parallel Example: User Story 1

```bash
# Write tests in parallel (different files):
Task T007: "Unit tests for LambdaStartersService in pkg/starters/service_test.go"
Task T008: "Handler tests for GetPDPStarters happy path in pkg/websocket/client_test.go"

# Then implement sequentially:
Task T009: "Add get_pdp_starters case to ParsePayload"
Task T010: "Implement GetPDPStarters handler method"
```

## Parallel Example: After US1, US2 and US3 in parallel

```bash
# Developer A works on US2:
Task T011-T015: Error handling tests and logging

# Developer B works on US3 simultaneously:
Task T016-T018: Validation tests and feature toggle tests
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001–T002)
2. Complete Phase 2: Foundational (T003–T006)
3. Complete Phase 3: User Story 1 (T007–T010)
4. **STOP and VALIDATE**: Test US1 independently — send `get_pdp_starters`, receive `starters`
5. Deploy/demo if ready — feature is functional

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. Add US1 → Test → Deploy (MVP — core feature works)
3. Add US2 → Test → Deploy (error resilience validated)
4. Add US3 → Test → Deploy (validation hardened)
5. Polish → Final validation

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- The `GetPDPStarters` method is one cohesive piece of code; US2 and US3 primarily add test coverage for error/validation paths already built in US1
- Tests use gomock for `StartersService` and the existing `newTestClient` helper for WebSocket tests
- Synchronous validation errors (registration, missing fields, capacity) are returned to `ParsePayload` — the Read loop sends error payloads (consistent with existing handler pattern). Only after the goroutine is spawned does the handler return `nil`.
- The goroutine uses `context.WithTimeout` (configurable via `WWC_LAMBDA_STARTERS_TIMEOUT_SEC`, default 35s) to bound the Lambda SDK call and prevent indefinite blocking
