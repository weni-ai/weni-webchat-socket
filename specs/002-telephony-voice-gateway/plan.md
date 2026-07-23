# Implementation Plan: Telephony Voice Gateway (Asterisk ↔ ElevenLabs ↔ Flows/Nexus)

**Branch**: `002-telephony-voice-gateway` | **Date**: 2026-07-22 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/002-telephony-voice-gateway/spec.md`
**Governing Product Spec**: `vtex-cx-engine-specs` `specs/004-voice-mode-telephony/spec.md` @ `7838a70eed496aa45a85f4d86e81ca2f4fb2dbc0`

## Summary

Add telephony voice support to `weni-webchat-socket` without touching the existing `001-full-voice-mode` browser code path. A new `telephony/main.go` process exposes an HTTP session-registration endpoint and an Asterisk AudioSocket TCP listener; each accepted call becomes a `CallSession` that (1) resolves its Courier channel/tenant via a new `flows.IClient.ResolvePSTNChannel` method, (2) opens a dedicated gateway-side ElevenLabs Scribe v2 Realtime STT WebSocket, (3) forwards committed transcripts to Flows/Courier via the existing outbound-callback HTTP mechanism, (4) registers itself in the existing `ClientManager`/`Router` so the **already-implemented** gRPC `MessageStreamService` delivers Nexus's streamed deltas to it with zero changes to `pkg/grpc/server.go`, (5) batches those deltas at sentence boundaries into ElevenLabs TTS requests streamed back over AudioSocket, and (6) treats any STT `partial_transcript` received while speaking as a barge-in trigger. All engineering-only decisions (transport choice, session-registration hop, barge-in mechanism, gRPC delivery reuse) are recorded in `research.md`; none alter the Product Spec's requirements or success criteria.

## Technical Context

**Language/Version**: Go 1.24 (module `github.com/ilhasoft/wwcs`), matching existing codebase
**Primary Dependencies**: `gorilla/websocket` (new ElevenLabs STT/TTS realtime clients — already a project dependency for the WebSocket server), Go stdlib `net` (AudioSocket TCP), Go stdlib `net/http` (registration endpoint), existing `go-redis/redis/v8`, `configor`, `logrus`; no new third-party dependency required for the core media path
**Storage**: N/A for new persistent state — `CallSession` state is in-memory per pod; reuses the existing Redis `ws:clients` hash (via `ClientManager`) and Redis Streams (via `Router`) with no schema changes
**Testing**: Go stdlib `testing`, `testify/assert`, `gomock` (existing patterns from `pkg/starters`, `pkg/history`), table-driven tests, a lightweight in-process AudioSocket test client
**Target Platform**: Linux containers (Kubernetes) — new Deployment/Service, independently scalable from `api`/`grpc`
**Project Type**: Long-running TCP + WebSocket-client server process (new third entrypoint alongside `api/`, `grpc/`)
**Performance Goals**: Barge-in stop <300 ms (NFR-002/SC-004); STT-commit-to-first-TTS-byte <2 s P95 (SC-002, gateway-internal portion); no perceptible (<100 ms) gap between TTS batches (SC-003); no per-turn degradation across 10+ turns (SC-006)
**Constraints**: Per-call isolation (no shared mutable state across `CallSession`s, NFR-003); configurable concurrency cap with queue+hold-audio at capacity (FR-033); ElevenLabs credentials never persisted beyond runtime (NFR-004, reuses existing `GetElevenLabsAPIKey` adapter); zero changes to `pkg/grpc/server.go` wire contract or to any `001-full-voice-mode` code path (BD-009)
**Scale/Scope**: New packages under `pkg/telephony/{session,audiosocket,stt,tts}`, one new `flows.IClient` method, one new `config.Configuration` sub-struct, one new binary (`telephony/main.go`); no changes to `pkg/grpc`, `pkg/websocket` core WebSocket handling, or `pkg/elevenlabs`'s existing token-issuance code

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

### Pre-design check

| Principle | Status | Notes |
|---|---|---|
| I. Clear, Idiomatic Go Packages | PASS | New, focused packages per responsibility (`session`, `audiosocket`, `stt`, `tts`), mirroring the existing `pkg/flows`/`pkg/history`/`pkg/starters` shape. No existing file grows past its current size — all new logic lives in new files. |
| II. WebSocket Contract & Configuration Discipline | PASS | This feature's "contract" is the AudioSocket protocol + the new HTTP registration endpoint, both documented in `contracts/` before code, matching the spirit of the principle even though the transport isn't literally the existing WebSocket server. All config via new `WWC_TELEPHONY_*` env vars through the existing `configor`-based `config` package. Boundary validation: the registration handler validates `did`/`origin` before creating any `CallSession`. |
| III. Secrets, Security & Least Privilege | PASS | ElevenLabs API key resolution reuses the existing `flows.IClient.GetElevenLabsAPIKey` adapter — no new secret storage or logging path. No secrets in AudioSocket frames or registration payloads. |
| IV. Test-First Quality Gates | PASS | Every new package is interface-first (`AudioSocketConn`, `STTSession`, `TTSStreamClient`, `flows.IClient.ResolvePSTNChannel`) enabling `gomock`-based unit tests without a real Asterisk/ElevenLabs/Courier dependency, consistent with existing `pkg/starters`/`pkg/history` test patterns. |
| V. Observability & Operational Resilience | PASS | NFR-005 is a first-class requirement (FR-036); metrics/log fields planned per Story 9 before implementation, not bolted on after. |
| VI. Release & Infrastructure Alignment | PASS | New binary/Docker stage/K8s Deployment is called out explicitly in `quickstart.md` and here as required follow-up coordination, not silently assumed. Additive-only change to `flows.IClient` (new method) and `config.Configuration` (new sub-struct) — no breaking change to existing contracts. |

### Post-design check (after Phase 0/1: research.md, data-model.md, contracts/)

| Principle | Status | Notes |
|---|---|---|
| I. Clear, Idiomatic Go Packages | PASS | Confirmed package boundaries in `data-model.md`: `pkg/telephony/session` (CallSession, VoiceConfig, Turn, VoiceError, BargeInController), `pkg/telephony/audiosocket` (TCP protocol + connection abstraction), `pkg/telephony/stt`, `pkg/telephony/tts` (TTSBatcher). Each has a single, GoDoc-documented responsibility. |
| II. WebSocket Contract & Configuration Discipline | PASS | `contracts/audiosocket-session-protocol.md` and `contracts/elevenlabs-realtime.md` fully specify message shapes and connection lifecycle before implementation. `contracts/grpc-telephony-delivery.md` confirms zero wire-level changes to the existing gRPC contract — the existing contract discipline is preserved, not weakened. |
| III. Secrets, Security & Least Privilege | PASS | Confirmed: no new secret ever touches a log line (STT/TTS client GoDoc will explicitly note this); the proposed Courier endpoint (`flows-pstn-integration.md`) carries no secrets, only routing metadata. |
| IV. Test-First Quality Gates | PASS | `tasks.md` places test tasks before implementation tasks in every user story phase, per the repo's Delivery Workflow requirement. |
| V. Observability & Operational Resilience | PASS | `SessionMetrics` (data-model.md) and structured log fields are designed alongside `CallSession`, not deferred. Retriable (STT reconnect, TTS batch failure) vs. permanent (channel unresolved, STT setup failure) error handling is explicit via `VoiceError.Recoverable` — directly satisfies "distinguish retriable... from permanent... errors." |
| VI. Release & Infrastructure Alignment | PASS | No breaking changes identified. New env vars documented in `quickstart.md`. Infra coordination flagged as an explicit dependency, not a silent gap (see Complexity Tracking). |

**Result**: No unresolved violations. No Complexity Tracking entries are required to justify a constitutional deviation — the entries below are risk/dependency call-outs, not violations.

## Project Structure

### Documentation (this feature)

```text
specs/002-telephony-voice-gateway/
├── spec.md                              # Engineering Spec (this feature)
├── plan.md                              # This file
├── research.md                          # Phase 0 — engineering decisions for every [NOT YET DEFINED] item
├── data-model.md                        # Phase 1 — CallSession, VoiceConfig, Turn, VoiceError, TTSBatcher, BargeInController
├── quickstart.md                        # Local run + manual smoke test
├── contracts/
│   ├── audiosocket-session-protocol.md  # HTTP registration + AudioSocket wire contract
│   ├── flows-pstn-integration.md        # Proposed Flows/Courier internal API + callback contract
│   ├── grpc-telephony-delivery.md       # How CallSession reuses the existing gRPC pipeline unchanged
│   └── elevenlabs-realtime.md           # Gateway-side STT/TTS WebSocket contracts
├── checklists/
│   └── requirements.md
└── tasks.md                             # Phase 2 output (via /speckit.tasks) — the requested backlog
```

### Source Code (repository root)

```text
config/
└── config.go                 # Add `Telephony` sub-struct to Configuration (WWC_TELEPHONY_* env vars)

pkg/
├── flows/
│   └── client.go              # Add ResolvePSTNChannel(did) to IClient + Client implementation
├── telephony/                 # NEW top-level package tree for this feature
│   ├── session/
│   │   ├── call_session.go    # CallSession struct + state machine
│   │   ├── call_session_test.go
│   │   ├── voice_config.go    # VoiceConfig resolution (flows calls + config defaults)
│   │   ├── voice_config_test.go
│   │   ├── turn.go            # Turn struct
│   │   ├── voice_error.go     # VoiceError + ErrorCode
│   │   ├── bargein.go         # BargeInController
│   │   ├── bargein_test.go
│   │   ├── manager.go         # SessionManager: registry, capacity cap, hold-audio queue (FR-033)
│   │   ├── manager_test.go
│   │   ├── delivery.go        # ClientManager/Router registration + DeliverFunc adapter (research.md §5)
│   │   ├── delivery_test.go
│   │   └── metrics.go         # SessionMetrics, wraps pkg/metric
│   ├── audiosocket/
│   │   ├── server.go          # TCP listener, frame parsing (0x01/0x10/0x00/0x03)
│   │   ├── server_test.go
│   │   ├── conn.go            # AudioSocketConn interface + real implementation
│   │   ├── registration.go    # POST /telephony/sessions HTTP handler
│   │   └── registration_test.go
│   ├── stt/
│   │   ├── client.go          # STTSession interface + ElevenLabs Scribe v2 Realtime implementation
│   │   ├── client_test.go
│   │   └── client_mock.go     # gomock-generated
│   └── tts/
│       ├── client.go          # TTSStreamClient interface + ElevenLabs streaming implementation
│       ├── client_test.go
│       ├── client_mock.go     # gomock-generated
│       ├── batcher.go         # TTSBatcher (sentence-boundary batching, FR-016/FR-020)
│       └── batcher_test.go
└── metric/
    └── interface.go            # Add telephony-specific counters/histograms (setup, STT commit, TTS batch, barge-in, teardown)

telephony/
└── main.go                    # NEW third entrypoint: wires config, Redis, Mongo, flows client,
                                #   ClientManager/Router (shared with api/grpc), SessionManager,
                                #   AudioSocket TCP listener, registration HTTP server

docker/
└── Dockerfile                  # Add a build stage/target for the telephony binary (flagged, coordinated with infra)
```

**Structure Decision**: A new `pkg/telephony/` tree keeps every new responsibility isolated from `pkg/websocket` (which stays untouched, protecting `001-full-voice-mode` per BD-009) while still depending on and reusing `pkg/flows`, `pkg/websocket.ClientManager`, `pkg/streams.Router`, `pkg/history`, and `pkg/metric` exactly as `pkg/grpc` and `pkg/starters` already do — this repo's established pattern for a focused package per external integration/responsibility, wired together at a binary's `main.go`.

## Design Decisions

*(Full rationale in `research.md`; summarized here for plan traceability.)*

1. **AudioSocket over WebSocket** for Asterisk↔gateway transport — native, already-validated, no new Asterisk-side module.
2. **HTTP session-registration hop precedes the AudioSocket TCP connection** — the only way to convey DID/caller-ID/origin, which the standard AudioSocket protocol cannot carry.
3. **`flows.IClient.ResolvePSTNChannel`** — new method, same pattern as existing internals calls; the real Courier endpoint is a joint contract tracked as a dependency, not blocking implementation (mockable interface).
4. **Barge-in reuses the STT session's own `partial_transcript` signal** — no new local VAD; the STT session runs continuously across all `CallSession` states specifically to make this possible.
5. **`CallSession` registers as a `ClientManager`/`Router` "connected client"** — zero changes to `pkg/grpc/server.go`; the existing Redis Streams-based cross-pod delivery is reused verbatim.
6. **New, separate ElevenLabs realtime STT/TTS clients** (`pkg/telephony/stt`, `pkg/telephony/tts`) rather than extending `pkg/elevenlabs` — protects the browser-facing token-issuance path used by `001-full-voice-mode` (BD-009) and matches the trust model difference (gateway holds the full API key here; the browser only ever gets single-use tokens).
7. **New `telephony/main.go` binary** — follows the existing one-entrypoint-per-ingress-protocol pattern (`api/`, `grpc/`), independently scalable by call concurrency.

## Complexity Tracking

> No Constitution violations were identified (see Constitution Check above). The items below are risk/dependency call-outs surfaced during planning, tracked here per this repo's Delivery Workflow ("complexity exceptions MUST be documented explicitly... with the simpler alternative rejected") even though none required rejecting a simpler alternative — they are forward-looking watch-items for the tasks/implementation phase.

| Aspect | Decision / Watch-item | Why flagged |
|---|---|---|
| Flows/Courier `ResolvePSTNChannel` endpoint does not exist yet | Build behind an interface + mock now; confirm the real contract with the Courier team in parallel (tracked in `contracts/flows-pstn-integration.md`) | This repo's tasks must not block on another team's endpoint; the interface boundary is the risk mitigation |
| Contact URN echo-back from the callback response is unconfirmed | Fall back to locally constructing `tel:<caller_id>` for `CallSession.ContactURN` until confirmed; regardless of source, the literal `ClientManager`/`Router` registration key is always `CallSession.RegistrationKey()` — the bare, scheme-stripped form, matching `pkg/grpc/server.go`'s `normalizeContactURN` (see `research.md` §5, `data-model.md`) | If Courier constructs a different normalized form (e.g. E.164 differences), gRPC delivery lookups would miss — must be reconciled before this feature can go to production, not before it can be planned/coded |
| Barge-in latency depends on ElevenLabs STT partial-transcript latency, not a purely local signal | Accepted for v1; add a local energy-based VAD fallback trigger only if P95 measurements during implementation exceed the 300 ms budget (NFR-002) | Avoids building a second detection mechanism speculatively; keeps the barge-in *contract* stable either way |
| New Kubernetes Deployment/Service + Docker build stage for `telephony/main.go` | Out of this repo's task list; flagged as required infrastructure-repo coordination before this feature can deploy | Constitution Principle VI requires this be surfaced explicitly rather than assumed |
