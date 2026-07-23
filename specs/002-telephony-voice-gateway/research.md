# Phase 0 Research: Telephony Voice Gateway

**Branch**: `002-telephony-voice-gateway` | **Date**: 2026-07-22 | **Spec**: [spec.md](./spec.md)

This document resolves every item the Product Spec (`004-voice-mode-telephony`) explicitly marked `[NOT YET DEFINED]` or deferred to "engineering validation," plus the additional implementation-level unknowns discovered while grounding the spec in the current `weni-webchat-socket` codebase. Each decision is engineering-only: none changes a Product FR/NFR/SC/Binding Decision.

## 1. Asterisk ↔ gateway transport: AudioSocket vs. WebSocket

**Decision**: Implement **Asterisk AudioSocket** (TCP, Asterisk's built-in binary framing) as the production transport.

**Rationale**:
- AudioSocket is a **native Asterisk dialplan application** (`AudioSocket()`) available since Asterisk 13.14/16 with no third-party module required. A "WebSocket" alternative would require either a custom Asterisk module or ARI `externalMedia` (which itself speaks RTP/UDP to an external endpoint, not WebSocket) — meaningfully more infrastructure risk for the Asterisk/telephony deployment repo, which is explicitly out of this repo's scope and `[NOT YET DEFINED]` in the Product Spec.
- The documented prototype (`server.py`) already validates the AudioSocket framing end-to-end (Product Spec Assumptions & Dependencies), so choosing it minimizes new integration risk on the Asterisk side while this repo focuses on the parts genuinely new to Go: gateway-side STT/TTS orchestration, batching, and barge-in.
- The wire format is trivial to implement server-side in Go with the standard library (`net.Listener` + a small binary reader), consistent with Constitution Principle I (idiomatic Go, minimal dependencies) — no new heavyweight dependency needed.
- **Trade-off accepted**: the Product Spec notes "the target design favors WebSocket." AudioSocket is chosen anyway because it is Asterisk-native and already proven; if a future need for WebSocket transport emerges (e.g. to remove Asterisk from the path entirely for a browser-avoiding client), it can be added as an additional listener behind the same internal `CallSession` abstraction without touching STT/TTS/batching/barge-in logic — see §5.

**AudioSocket frame types used** (per Asterisk's official protocol definition — [docs.asterisk.org/.../AudioSocket](https://docs.asterisk.org/Configuration/Channel-Drivers/AudioSocket/) — and the validated prototype). **Correction (2026-07-23)**: an earlier version of this table mislabeled `0x03` as "Error"; the byte values below match Asterisk's own `ast_audiosocket_msg_kind` enum and the reference `CyCoreSystems/audiosocket` Go implementation:

| Byte | Meaning | Direction |
|---|---|---|
| `0x00` | Hangup / terminate (`0x00 0x00 0x00`, no payload) | Asterisk → gateway |
| `0x01` | Session UUID (16 bytes), sent once at connection start | Asterisk → gateway |
| `0x03` | DTMF digit (1-byte ASCII payload) | Asterisk → gateway |
| `0x10` | Audio frame (signed linear PCM, 8 kHz, 20 ms ≈ 320 bytes) | both directions |
| `0xFF` | Error (1-byte application-specific error code payload; Asterisk's own codes: `0x01` caller hung up, `0x02` frame-forwarding error, `0x04` memory-allocation error) | Asterisk → gateway |

This repo's scope excludes IVR/DTMF handling (Product Spec Out of Scope), so `0x03` frames are simply recognized-and-ignored (logged at debug level, no state change) rather than acted upon — but they MUST NOT be misclassified as errors. `0xFF` is the only frame type that should ever be logged/metriced as an Asterisk-side error (Story 9's observability requirement, NFR-005); anything else unrecognized still falls back to the generic malformed-frame path (Story 2, Scenario 6).

## 2. Conveying DID, caller ID, and origin tag (Product FR-035)

**Problem**: The standard AudioSocket protocol carries only a 16-byte UUID at connection start — no room for DID, caller ID, or an origin tag.

**Decision**: A **pre-call HTTP session-registration hop**. The Asterisk-side dialplan (via AGI or an ARI application — owned by the Asterisk/telephony deployment repo) calls a new gateway HTTP endpoint (`POST /telephony/sessions`, see `contracts/audiosocket-session-protocol.md`) with `{did, caller_id, origin}` *before* executing `AudioSocket()`. The gateway:
1. Resolves the Courier channel/tenant for the DID (§3).
2. Creates a `CallSession` in `Connecting` state, keyed by a generated session UUID.
3. Returns that UUID in the HTTP response.
4. The dialplan passes this UUID as the AudioSocket UUID argument, so when the AudioSocket TCP connection's `0x01` frame arrives, the gateway looks up the pending `CallSession` by UUID and attaches the connection.

**Rationale**: This mirrors "each hop confirms session initialization before audio is accepted" (Product Journey 1) precisely, keeps the AudioSocket wire format 100% standard (no custom Asterisk-side patching), and gives the gateway a clean synchronous point to fail fast (unknown DID, unreachable Flows) before any audio flows — directly implementing Product FR-005's "graceful, not silent" requirement. It also gives Courier resolution (§3) a natural place to happen without adding latency to the audio path itself.

**Alternative considered and rejected**: Encoding metadata into the UUID itself (e.g., a composite string). Rejected because AudioSocket's UUID field is a fixed 16 raw bytes (a real UUID), not a free-form string, and overloading it would break interoperability with any AudioSocket client library.

## 3. Resolving DID → Courier channel/tenant/callback URL

**Problem**: Per Product FR-037/BD-010, the DID→channel/tenant mapping is configuration on the Courier PSTN channel instance — Courier owns it, not the gateway. But the gateway needs the resolved `channel_uuid`, `project_uuid`, and the channel's inbound-message callback URL to do anything.

**Decision**: Add one new method to the existing `flows.IClient` interface, following the exact pattern of `GetElevenLabsAPIKey`/`GetChannelProjectLanguage` (both already call `{FlowsURL}/api/v2/internals/...`):

```go
// ResolvePSTNChannel resolves a dialed number (DID) to its Courier PSTN
// channel instance, returning enough context to route inbound messages.
ResolvePSTNChannel(did string) (channelUUID, projectUUID, callbackURL string, err error)
```

Proposed endpoint (to be confirmed jointly with the Courier team — see `contracts/flows-pstn-integration.md`): `GET /api/v2/internals/pstn_channel?did=<did>`.

**Rationale**: Every other cross-service lookup in this codebase already goes through `flows.IClient` as a thin, mockable adapter (Constitution Principle II: "External integrations... MUST prefer thin adapters behind interfaces"). This keeps the new capability consistent with existing conventions and, critically, lets Story 1/3 be implemented and tested against a mock **before** the real Courier endpoint exists — the interface is the contract boundary, not the HTTP call.

**Not decided here (explicitly deferred, tracked as a dependency in spec.md)**: the exact response shape and error semantics (e.g., 404 for unknown DID) of the real Courier endpoint. This is a joint contract, analogous to how `GetElevenLabsAPIKey` already special-cases a 404 "not found" response distinctly from a hard error — the proposed contract follows that same shape.

## 4. Barge-in detection mechanism (Product FR-022, FR-026, NFR-002)

**Problem**: The Product Spec requires barge-in to trigger on real caller speech, not line noise, within 300 ms — and this codebase has no existing VAD/energy-detection code to reuse.

**Decision**: Reuse the **ElevenLabs Scribe v2 Realtime session's own server-side VAD/partial-transcript signal** as the barge-in trigger, rather than building a separate local VAD. Because Story 2 already keeps the STT WebSocket session open continuously (audio is always being streamed to STT, regardless of `CallSession` state), the *first* `partial_transcript` event received while `Speaking` is treated as the barge-in trigger.

**Rationale**:
- ElevenLabs' STT already performs speech/non-speech discrimination server-side to produce partial transcripts — reusing it directly satisfies "distinguish caller speech from line noise" (FR-026) for free, with no new signal-processing code in Go.
- It avoids running two independent VAD implementations (one for STT turn-commit, one for barge-in) that could disagree.
- Latency: ElevenLabs Scribe v2 Realtime's partial transcripts arrive on the order of tens of milliseconds after speech onset, well inside the 300 ms budget (NFR-002); the gateway's own reaction (stop streaming TTS frames to the AudioSocket connection) is a local, synchronous operation with negligible added latency.

**Trade-off accepted**: this makes barge-in latency dependent on ElevenLabs' STT partial-transcript latency rather than a purely local signal. If P95 measurements during implementation show this is too slow, a fallback local energy-based VAD (e.g., simple RMS-over-threshold on inbound AudioSocket frames) can be added as an additional/earlier trigger without changing the barge-in *contract* (still "stop TTS, discard buffers, treat next commit as new turn") — flagged as a Complexity Tracking watch-item in `plan.md`, not a spec change.

## 5. Delivering Nexus response deltas to a `CallSession` instead of a WebSocket client

**Problem**: The existing gRPC `MessageStreamService` (`pkg/grpc/server.go`) already delivers `setup`/`delta`/`completed` messages by contact URN via `ClientManager.GetConnectedClient` + `Router.PublishToClient`, built for WebSocket clients. Telephony calls are not WebSocket clients.

**Decision**: No changes to `pkg/grpc/server.go`. Both `websocket.ClientManager` (`ConnectedClient{ID, AuthToken, Channel, PodID}`) and `streams.Router` (`PublishToClient(ctx, contactURN, payloadJSON)` + a pod-local `DeliverFunc(clientID string, raw []byte) error`) are already fully generic over "what does 'connected' mean" and "how is a payload delivered locally." A `CallSession`:
1. Registers itself via `ClientManager.AddConnectedClient(ConnectedClient{ID: registrationKey, Channel: channelUUID, PodID: podID})` exactly as a WebSocket client does today — **critically, `registrationKey` is the bare, scheme-stripped contact identifier, not the full `tel:`-prefixed `ContactURN`**. `pkg/grpc/server.go` calls `normalizeContactURN(req.ContactUrn)` (which strips everything up to and including the first `:`) before *every* `ClientManager.GetConnectedClient`/`Router.PublishToClient` call, and existing WebSocket clients register with `ID: c.ID` where `c.ID = payload.From` — already a bare identifier, never scheme-prefixed (`pkg/websocket/client.go`). If a `CallSession` registered under `"tel:+1555..."` instead, every lookup from `pkg/grpc/server.go` (searching for the stripped `"+1555..."`) would miss, silently breaking delta delivery for the entire call. So: `CallSession.ContactURN` keeps the full `tel:`-prefixed form (useful for logs and for matching whatever Courier echoes back), but `registrationKey = stripScheme(ContactURN)` — the same stripping rule as `normalizeContactURN` — is what's actually passed to `AddConnectedClient`/`RemoveConnectedClient` and used as the `Router.PublishToClient` target.
2. Supplies its own `DeliverFunc` implementation, wired into a **`telephony`-owned `streams.Router` instance** created in `telephony/main.go` — **not** a shared/extended instance of the `api` process's `Router`. Every binary in this codebase already constructs its own `Router` bound to its own `podID` (`grpc/main.go` builds a publish-only one with `isLocal` always `false`; `api/main.go` builds a full consumer one via `pkg/websocket.NewStreamsRouter`, closed over its own `ClientPool`). `telephony/main.go` follows the exact same established pattern: its own `podID`, and a `deliver` closure that looks up `SessionManager.GetByRegistrationKey` directly — it has no `ClientPool` and never needs one, and there is no "if branch" or fallthrough to check, because `Router.PublishToClient` already routes by the target `ConnectedClient.PodID` at the Redis Streams layer (see `pkg/streams/router.go`); a `CallSession` registers with `PodID` = the telephony pod's own ID, so its stream entries only ever land on — and are only ever consumed by — that telephony pod's own `Router`, never `api`'s. `pkg/streams/router.go` itself needs zero changes; only new wiring code in `telephony/main.go` (and the `deliver` closure in `pkg/telephony/session/delivery.go`) is needed. The delivery handler parses the same JSON stream payloads (`StreamStartPayload`/`StreamDeltaPayload`/`StreamEndPayload` — already defined in `pkg/websocket`) and feeds them into the TTS batching pipeline (Story 4) instead of writing to a WebSocket connection.
3. Deregisters via `ClientManager.RemoveConnectedClient(registrationKey)` on teardown.

**Rationale**: This is the single highest-leverage decision in this plan — it means Story 3 requires **zero changes** to the already-implemented, already-tested gRPC/Router/Redis-Streams cross-pod delivery pipeline (Constitution Principle I: prefer minimal, justified changes; existing `client.go` conventions). The only new code is a second implementation of the local-delivery side (`pkg/telephony` instead of `pkg/websocket`), registered against the same shared interfaces. Multi-pod correctness (a call's AudioSocket TCP connection may be on a different pod than the gRPC stream from Nexus) is inherited for free from the existing Redis Streams router.

## 6. Gateway-side ElevenLabs STT/TTS clients are new — not the existing `pkg/elevenlabs`

**Problem**: `pkg/elevenlabs/client.go` today only issues single-use tokens (`RequestSingleUseTokens`) so a **browser** can connect directly to ElevenLabs for `001-full-voice-mode`. Telephony has no browser; per Product BD-002, the gateway itself must be the STT/TTS client.

**Decision**: Add new, separate realtime client code (new files, not a rewrite of the existing token-issuance client, to avoid any risk to `001-full-voice-mode` per BD-009): a Scribe v2 Realtime WebSocket client and a streaming TTS WebSocket client, authenticated with the tenant's full ElevenLabs API key (already fetchable via `flows.IClient.GetElevenLabsAPIKey`, not a single-use token — the gateway is a trusted server-side actor here, not a browser).

**Rationale**: Keeps the browser-facing token-issuance code path (`RequestSingleUseTokens`) completely untouched, directly satisfying BD-009 ("does not modify `001-full-voice-mode`"). Reuses the already-vetted credential-resolution path (`GetElevenLabsAPIKey`) rather than introducing a new one, satisfying Constitution Principle III (secrets flow through existing, documented adapters).

## 7. New process entrypoint

**Decision**: Add a third binary, `telephony/main.go`, alongside the existing `api/main.go` (WebSocket + HTTP) and `grpc/main.go` (Nexus gRPC ingress). It hosts the AudioSocket TCP listener and the session-registration HTTP endpoint, and shares `pkg/flows`, `pkg/history`, `pkg/metric`, `config`, `pkg/websocket.ClientManager`/`streams.Router` with the other two binaries via the same Redis/Mongo connections.

**Rationale**: Follows the existing one-binary-per-ingress-protocol pattern already established by `api/` (WebSocket) and `grpc/` (gRPC from Nexus) rather than overloading either existing binary with an unrelated TCP protocol. Keeps deployment/scaling independent (Constitution Principle VI: release/infra alignment) — the telephony process can be scaled by concurrent-call capacity, independent of WebSocket connection count or gRPC throughput.
