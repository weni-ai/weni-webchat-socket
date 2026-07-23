# Feature Specification: Telephony Voice Gateway (Asterisk ↔ ElevenLabs ↔ Flows/Nexus)

**Feature Branch**: `002-telephony-voice-gateway`
**Created**: 2026-07-22
**Status**: Draft
**Input**: User description: "Implement the `weni-webchat-socket` (gateway) side of telephony voice mode: bridge Asterisk (SIP/PABX) call audio to ElevenLabs real-time STT/TTS, forward committed transcripts to Flows/Courier, receive streamed agent responses over the existing gRPC pipeline, and speak them back into the call with reliable barge-in — reusing the pipeline already built for `001-full-voice-mode`."

## Governing Product Spec (source of truth — DO NOT redefine)

- **Product Spec**: [`specs/004-voice-mode-telephony/spec.md`](https://github.com/weni-ai/vtex-cx-engine-specs/blob/7838a70eed496aa45a85f4d86e81ca2f4fb2dbc0/specs/004-voice-mode-telephony/spec.md)
- **Pinned commit**: `7838a70eed496aa45a85f4d86e81ca2f4fb2dbc0` (branch `main`)
- **Repository**: `weni-ai/vtex-cx-engine-specs`

This Engineering Spec inherits — verbatim and without modification — the Product Spec's problem statement, Jobs-to-be-Done, scope (in/out), all nine User Journeys, all Binding Decisions (BD-001…BD-010), and all Success Criteria (SC-001…SC-011). Nothing in this document overrides, narrows, or re-derives the *what* or *why*. It exists solely to translate the parts of the Product Spec that fall on the gateway (`weni-webchat-socket`) into engineering-scenarios, requirements, and entities that are directly actionable in this repository. Where a requirement below implements a Product FR/NFR, the mapping is stated explicitly. Any requirement that cannot be traced to the Product Spec MUST be flagged and confirmed with product before planning — none were found during this decomposition.

**Out of repository scope** (owned by sibling engineering repos, per Product Spec Solution Architecture Vision and BD-010 — NOT redefined or re-planned here):

- Asterisk PABX dialplan/SIP trunk configuration and telephony provisioning — telephony/Asterisk deployment repo `[NOT YET DEFINED in Product Spec]`.
- Courier's new PSTN channel type, DID→channel/tenant configuration, and `tel:` URN construction (BD-010, FR-035–FR-038) — `engineering/courier`.
- Nexus agent reasoning/response generation — out of scope per Product Spec.
- Any change to `001-full-voice-mode` (web voice mode) behavior — BD-009.

## Clarifications

### Session 2026-07-22 (engineering decomposition)

- Q: The Product Spec marks the Asterisk↔gateway transport as `[NOT YET DEFINED: final production transport, pending engineering validation]` (WebSocket or AudioSocket). Which does this repo implement? → A: **Asterisk AudioSocket** (TCP, binary framing) — see `research.md` for the full rationale. This is an engineering-level decision explicitly delegated by the Product Spec and does not change any Product FR/NFR/SC.
- Q: How does the gateway learn the Courier channel's callback URL for a given DID, given Courier owns the DID→channel/tenant mapping (FR-037)? → A: A new Flows/Courier internal endpoint, analogous to existing `flows.IClient` internals calls (e.g. `GetElevenLabsAPIKey`, `GetChannelProjectLanguage`), resolves DID → `{channel_uuid, callback_url, project_uuid}`. The exact endpoint path/payload is a joint contract with the Courier team (tracked in `contracts/flows-pstn-integration.md` and Assumptions) — this repo only defines the client-side interface it needs.
- Q: How does a Nexus response delta reach the right phone call instead of a WebSocket client? → A: Reuse the existing `ClientManager` + `Router` (Redis Streams) abstractions unchanged. A `CallSession` registers itself as a `ConnectedClient` (exact key format refined below); the existing gRPC `StreamMessages`/`SendMessage` code path in `pkg/grpc/server.go` requires no changes — it already delivers by contact URN, agnostic of what "connected" means. See `research.md` §3.

### Session 2026-07-23 (engineering review correction)

- Q: `pkg/grpc/server.go` normalizes every inbound `contact_urn` by stripping its scheme prefix (`normalizeContactURN`, e.g. `"tel:+1555..."` → `"+1555..."`) before every `ClientManager.GetConnectedClient`/`Router.PublishToClient` call, and existing WebSocket clients register with that same bare form (`c.ID = payload.From`, no scheme prefix). What key MUST a `CallSession` use when calling `ClientManager.AddConnectedClient`? → A: The **bare, scheme-stripped** contact identifier — i.e., exactly what `normalizeContactURN` would produce from whatever `contact_urn` Nexus uses for this call (the same identifier space WebSocket clients already register under). `CallSession.ContactURN` MAY still store the full `tel:`-prefixed form for logging/traceability, but the literal string passed to `AddConnectedClient`/`RemoveConnectedClient` and used for `ClientManager`/`Router` lookups MUST be the stripped form. This requires zero changes to `pkg/grpc/server.go` (Design Decision 5 is preserved) and corrects an inconsistency found across `research.md` §5, `data-model.md`, and `contracts/grpc-telephony-delivery.md`, which previously described the registration key as the full `tel:`-prefixed URN.

## User Scenarios & Testing *(mandatory)*

Each story below is the gateway-scoped slice of a Product Journey (cited). Stories are ordered so each is independently implementable and testable against a mock Asterisk/ElevenLabs/Flows boundary, consistent with the repo's Test-First Quality Gate (Constitution IV).

### User Story 1 - Accept a Call and Establish the Media + STT Session (Priority: P1)

*(Implements Product Journey 1)*

An Asterisk dialplan opens an AudioSocket TCP connection to the gateway for an answered call, after first registering the call's context (DID, caller ID, origin tag) with the gateway over HTTP. The gateway resolves the Courier channel/tenant for that DID, opens a dedicated ElevenLabs STT session, and only then confirms readiness so Asterisk can play the greeting.

**Why this priority**: Nothing else in this spec can be exercised without an established call session and STT connection.

**Independent Test**: Start the gateway, simulate an Asterisk AudioSocket client (test harness) that first calls the session-registration HTTP endpoint with a DID/caller-ID/origin, then opens the AudioSocket TCP connection with the returned session UUID, and verify the gateway responds with a session-ready acknowledgment and an active `CallSession` is tracked.

**Acceptance Scenarios**:

1. **Given** a DID configured on a Courier PSTN channel, **When** the gateway receives a session-registration request with that DID + caller ID + origin tag, **Then** it resolves the channel/tenant via the Flows/Courier internal API, creates a `CallSession` in `Connecting` state, and returns a session UUID.
2. **Given** a registered session UUID, **When** the matching AudioSocket TCP connection arrives and sends the UUID frame, **Then** the gateway attaches the connection to the pending `CallSession` and opens a dedicated ElevenLabs STT WebSocket session using the tenant's ElevenLabs API key.
3. **Given** the STT session initializes successfully, **When** initialization completes, **Then** the gateway transitions the `CallSession` to `Listening` and signals readiness back over the AudioSocket connection (or HTTP registration response) so Asterisk plays the greeting.
4. **Given** the DID does not resolve to any Courier channel, **When** the session-registration request is received, **Then** the gateway rejects it with an error and does not open an AudioSocket/STT session.
5. **Given** the ElevenLabs STT session cannot be established (auth failure, unavailable), **When** this happens during setup, **Then** the `CallSession` transitions to `Error`, a spoken-fallback error code is returned over the session-ready signal, and the session is torn down (implements Product FR-005).
6. **Given** the Flows/Courier resolution call for the DID fails or times out, **When** this happens during setup, **Then** the same graceful error/teardown path as scenario 5 applies (implements Product FR-005).

---

### User Story 2 - Stream Caller Audio to STT and Produce a Committed Transcript (Priority: P1)

*(Implements Product Journey 2, FR-006–FR-011)*

While a `CallSession` is `Listening`, the gateway receives AudioSocket audio frames (PCM 16-bit LE mono, 8 kHz), resamples/encodes them to the format ElevenLabs Scribe v2 Realtime expects (PCM 16 kHz, Base64), and forwards them as `input_audio_chunk` messages. Partial transcripts are tracked internally; a server-VAD `committed_transcript` becomes exactly one caller message per turn.

**Why this priority**: This is the input half of the conversation loop; without it Story 3 has nothing to respond to.

**Independent Test**: Feed a sequence of AudioSocket audio frames into an active session against a mocked ElevenLabs STT connection that emits partial transcripts then one committed transcript, and verify exactly one message is forwarded downstream (Story 3's boundary) with no duplication.

**Acceptance Scenarios**:

1. **Given** an active STT session, **When** AudioSocket audio frames arrive, **Then** the gateway converts each frame to the STT input format and forwards it without blocking the AudioSocket read loop.
2. **Given** audio is streaming, **When** ElevenLabs returns `partial_transcript` events, **Then** the gateway tracks them internally but does not forward anything downstream yet.
3. **Given** the caller pauses for the configured VAD silence threshold, **When** ElevenLabs emits `committed_transcript`, **Then** the gateway treats it as exactly one turn and hands it to the message-forwarding path (Story 4) exactly once.
4. **Given** a `committed_transcript` is empty or whitespace-only, **When** it is received, **Then** the gateway discards it and the `CallSession` remains `Listening` (implements Product FR-009).
5. **Given** the STT WebSocket connection drops unexpectedly mid-call, **When** the drop is detected, **Then** the gateway automatically reopens a new STT session with the same tenant/language configuration and resumes without ending the call or losing the current turn boundary (implements Product FR-011).
6. **Given** a malformed or undecodable AudioSocket audio frame, **When** it is received, **Then** the gateway drops that frame, logs it, and continues the session without crashing (Edge Case — Data).

---

### User Story 3 - Forward Transcript to Flows/Courier and Receive Streamed Agent Deltas (Priority: P1)

*(Implements Product Journey 3 (input leg), FR-012, FR-013, FR-035, FR-038)*

The gateway POSTs the committed transcript to the Courier channel's callback URL (resolved in Story 1) carrying the raw caller identity and origin tag, exactly like existing channel message delivery. The existing gRPC `MessageStreamService` (already implemented for `001-full-voice-mode`/other channels) delivers the resulting Nexus response as `setup` → `delta`* → `completed` messages; this story only needs the `CallSession` to be reachable as a delivery target for its contact URN.

**Why this priority**: Bridges STT output to the existing, already-implemented agent pipeline. No new gRPC contract is introduced — this story is about making the `CallSession` a valid delivery target.

**Independent Test**: Register a `CallSession` for a `tel:` contact URN, send a committed transcript, and — using the gateway's existing gRPC test harness — call `StreamMessages` with `setup`/`delta`/`completed` for that same contact URN; verify the `CallSession`'s handler (not a WebSocket client) receives each event.

**Acceptance Scenarios**:

1. **Given** a `CallSession` with a resolved channel/callback URL, **When** a committed transcript is produced, **Then** the gateway POSTs it to the resolved callback URL with the raw caller identity and origin tag, following the same outbound message contract already used for other channels (implements Product FR-012).
2. **Given** a `CallSession` is active, **When** it starts, **Then** the gateway registers it in `ClientManager`/`Router` keyed by the **bare, scheme-stripped** contact identifier (the same identifier space WebSocket clients already register under, and the same form `pkg/grpc/server.go`'s `normalizeContactURN` produces from the contact URN), so existing gRPC delivery code requires no changes (implements Product FR-013 delivery path).
3. **Given** the existing gRPC server receives `setup`/`delta`/`completed` for a `tel:` contact URN, **When** it publishes via `Router.PublishToClient`, **Then** the `CallSession`'s local delivery handler (not a WebSocket `Send`) receives the JSON payload and hands it to the TTS batching pipeline (Story 4).
4. **Given** the Flows/Courier callback POST fails (network error, non-2xx), **When** this happens, **Then** the gateway logs the failure with retry/backoff appropriate to the operation and does not crash the `CallSession`; if the pipeline is unreachable at the time of the *first* turn of the call, the graceful spoken-unavailability/teardown path applies (implements Product FR-005 for mid-call: session continues in `Listening`, teardown only at hangup).
5. **Given** duplicate delivery of the same committed transcript is attempted (e.g. retry), **When** this happens, **Then** the existing de-duplication guarantee from Story 2 (exactly-once commit) prevents a duplicate outbound POST.

---

### User Story 4 - Batch and Synthesize the Agent Response as Speech (Priority: P1)

*(Implements Product Journey 3 (output leg), FR-015–FR-021)*

As delta events arrive for the active `CallSession`, the gateway accumulates text until a sentence boundary or a minimum-length threshold, then opens/reuses an ElevenLabs TTS WebSocket session (model `eleven_flash_v2_5`, output `pcm_8000`) to synthesize each batch, streaming the resulting audio back over the AudioSocket connection to Asterisk as soon as the first batch is ready.

**Why this priority**: This is the half of the conversation the caller actually experiences; without it there is no spoken response.

**Independent Test**: Feed a mocked 3-sentence delta stream (`setup`→3×`delta`→`completed`) into an active `CallSession` against a mocked ElevenLabs TTS client, and verify at most 3–4 TTS requests are issued, playback starts before the full text is buffered, and batches play with no gap between them.

**Acceptance Scenarios**:

1. **Given** delta text is arriving, **When** it accumulates to a sentence boundary or the configured minimum-threshold, **Then** the gateway issues one TTS request for that batch without waiting for the rest of the response (implements Product FR-016, FR-017).
2. **Given** the first batch's audio is ready, **When** it arrives from ElevenLabs, **Then** the gateway streams it to Asterisk over the AudioSocket connection in the sample rate/encoding Asterisk expects (8 kHz PCM), immediately (implements Product FR-015, FR-019).
3. **Given** multiple batches are queued, **When** they play sequentially, **Then** there is no perceptible gap between batches (implements Product FR-018, target <100 ms per Product SC-003).
4. **Given** the stream completes (`completed` message) with buffered text remaining, **When** no more deltas are expected, **Then** the remaining text is flushed as a final TTS request (implements Product FR-016, edge case in Product spec).
5. **Given** an ElevenLabs TTS request for one batch fails, **When** this happens, **Then** the gateway skips that batch, continues with the next, and returns the `CallSession` to `Listening` after the response ends without dropping the call (implements Product FR-021).
6. **Given** a batch's text is non-speakable (URL/code/emoji only), **When** detected, **Then** the gateway skips synthesis for that batch and continues (Product Edge Case — Data).
7. **Given** the configured voice ID and active language for the channel, **When** any TTS request is issued, **Then** it uses that voice ID and language (implements Product FR-020, FR-027).

---

### User Story 5 - Reliable Barge-In (Priority: P1)

*(Implements Product Journey 4, FR-022–FR-026, NFR-002)*

Because the STT WebSocket session already runs continuously regardless of `CallSession` state, any `partial_transcript` event received while the session is `Speaking` is treated as caller voice activity: playback is stopped, all buffered/in-flight TTS audio and pending deltas for the interrupted response are discarded, and the caller's speech is captured as a new turn (re-entering Story 2's flow).

**Why this priority**: Non-negotiable per Product BD-006; the prototype explicitly lacked this, and it is called out as the highest-risk gap to close.

**Independent Test**: Drive an active `Speaking` session with a mocked `partial_transcript` event mid-playback and verify playback stops within the target latency, buffered audio/pending deltas are discarded, and the next `committed_transcript` produces a new, non-appended message.

**Acceptance Scenarios**:

1. **Given** the `CallSession` is `Speaking`, **When** a `partial_transcript` event is received from the always-on STT session, **Then** the gateway stops streaming TTS audio to Asterisk and transitions the session out of `Speaking` (implements Product FR-022, FR-023, target <300 ms per NFR-002/SC-004).
2. **Given** a barge-in is triggered, **When** it occurs, **Then** any buffered TTS audio, any in-flight ElevenLabs TTS request, and any unflushed delta text for the interrupted response are discarded (implements Product FR-024).
3. **Given** a barge-in is triggered, **When** the caller continues speaking, **Then** the resulting `committed_transcript` is sent as a new, independent message — never appended to the interrupted response (implements Product FR-025).
4. **Given** line noise or a transient sound (not real speech) occurs while `Speaking`, **When** the STT session does not emit a `partial_transcript` for it, **Then** no barge-in is triggered (implements Product FR-026 — the decision to key barge-in off STT's own speech-recognition signal, rather than raw energy/VAD, is what provides this distinction; see `research.md` §4 for rationale).

---

### User Story 6 - Language Configuration via Channel (Priority: P2)

*(Implements Product Journey 6, FR-027–FR-029)*

The `CallSession`'s language is resolved once, at Story 1's channel-resolution step, from the platform's channel/project language configuration (reusing the existing `flows.IClient.GetChannelProjectLanguage` call), defaulting to English when unset. A mid-call language-change signal updates subsequent STT sessions and TTS requests.

**Why this priority**: Reuses an existing Flows client method; low implementation risk but required for correctness across tenants (P2 in the Product Spec).

**Independent Test**: Configure a mocked channel language response of `"pt"`, start a session, and verify the STT session-open request and every TTS request use `"pt"`; repeat with no language configured and verify `"en"`.

**Acceptance Scenarios**:

1. **Given** a channel is configured with language `"pt"`, **When** the `CallSession` starts, **Then** the STT session and all TTS requests for that session use `"pt"` (implements Product FR-027).
2. **Given** no language is configured for the channel, **When** the `CallSession` starts, **Then** the gateway defaults to `"en"` (implements Product FR-028).
3. **Given** a language-change signal is received mid-call (mechanism owned by Flows/platform, consumed here), **When** it arrives, **Then** subsequent STT sessions (post-reconnect) and TTS requests use the updated language (implements Product FR-029).
4. **Given** the configured language is unsupported by ElevenLabs STT/TTS, **When** a session is opened, **Then** the gateway falls back to English and proceeds rather than failing silently (Product Edge Case — i18n).

---

### User Story 7 - Continuous Multi-Turn Conversation and Concurrency (Priority: P2)

*(Implements Product Journeys 5 & 9, FR-030–FR-034, NFR-003, NFR-006)*

After a `Speaking` session finishes playing all TTS audio for a turn, it automatically returns to `Listening`. Each `CallSession` is fully isolated (its own STT/TTS connections, buffers, and state); the gateway enforces a configurable maximum concurrent-call capacity and queues excess callers with hold audio, admitting them in arrival order as slots free.

**Why this priority**: Required for production viability at scale but does not block a single-call demo (hence P2, matching the Product Spec).

**Independent Test**: Run N concurrent mocked `CallSession`s at the configured capacity limit, verify each is isolated (no cross-session data), then start one more and verify it is queued with hold audio and admitted once a slot frees.

**Acceptance Scenarios**:

1. **Given** TTS playback for a turn completes, **When** no more batches are queued, **Then** the `CallSession` automatically returns to `Listening` within the target turn-around time (implements Product FR-030).
2. **Given** a multi-turn call, **When** turns repeat 10+ times, **Then** no `CallSession` state, buffer, or connection is shared or leaked across turns (implements Product FR-031, NFR-006).
3. **Given** concurrent calls are active, **When** any one call's audio/transcript/agent context is inspected, **Then** it is fully isolated from every other active `CallSession` (implements Product FR-033, NFR-003).
4. **Given** the configured concurrent-call capacity is reached, **When** a new session-registration request arrives, **Then** the gateway places it in a queue, plays hold audio, and admits it in arrival order as a slot frees, rather than rejecting or silently dropping it (implements Product FR-034, SC-011).
5. **Given** a call ends (hangup, either side), **When** teardown completes, **Then** its concurrency slot is released and made available to the queue (implements Product FR-032, User Journey 9).

---

### User Story 8 - Efficient TTS Credit Usage (Priority: P2)

*(Implements Product Journey 7, FR-016, BD-005 — overlaps Story 4's batching mechanics; called out separately because it has its own measurable success criterion)*

**Why this priority**: Directly ties to Product SC-005 (cost control), which the prototype violated; worth its own explicit test even though the implementation is the same batching logic as Story 4.

**Independent Test**: Feed a realistic 3-sentence agent response through the batcher and count TTS requests issued.

**Acceptance Scenarios**:

1. **Given** a 3-sentence agent response streamed as multiple small deltas, **When** it completes, **Then** the gateway issues at most 3–4 TTS requests total, not one per delta (implements Product SC-005).
2. **Given** a barge-in occurs mid-response, **When** it occurs, **Then** all buffered/unflushed text for the interrupted response is discarded and no further TTS requests are issued for it (implements Product FR-016 edge case, consistent with Story 5).

---

### User Story 9 - Graceful Call Termination and Observability (Priority: P3)

*(Implements Product Journey 9, FR-032, NFR-005)*

Whichever side hangs up (caller via AudioSocket hangup frame, or a server-side forced termination), the gateway tears down the STT/TTS WebSocket sessions, stops any in-flight audio synthesis/playback immediately, releases the concurrency slot, and emits structured logs/metrics across setup, STT commit, agent round-trip, TTS, and teardown.

**Why this priority**: Correctness-critical for resource hygiene at scale, but doesn't block demonstrating the core conversational value (P3 in the Product Spec).

**Independent Test**: End an active session from a simulated hangup frame and from a forced server-side termination, and verify STT/TTS connections close, no goroutine/connection leak remains, and teardown metrics are emitted.

**Acceptance Scenarios**:

1. **Given** an active `CallSession`, **When** the AudioSocket hangup frame (`0x00`) is received, **Then** the gateway closes the STT/TTS WebSocket connections, stops any playback, and releases the concurrency slot (implements Product FR-032).
2. **Given** the agent is speaking, **When** the caller hangs up mid-playback, **Then** synthesis/playback stop immediately with no orphaned goroutines or streams.
3. **Given** a call ends for any reason, **When** teardown completes, **Then** structured logs/metrics are emitted for call setup, STT commit, agent round-trip, TTS, and teardown, sufficient to reconstruct the call's latency profile (implements Product NFR-005).
4. **Given** the gateway process itself needs to force-terminate a session (e.g. shutdown, capacity reclaim), **When** this happens, **Then** the same teardown path executes as a caller-initiated hangup.

---

### Edge Cases

*(Traced 1:1 to the Product Spec's Edge Cases section — no new edge cases are introduced; each is restated only where it implies a gateway-specific behavior.)*

- **Empty/silent audio** → no committed transcript forwarded; session stays `Listening` (Story 2, Scenario 4).
- **Malformed/undecodable AudioSocket frame** → frame dropped, session continues (Story 2, Scenario 6).
- **Non-speakable agent content** → TTS synthesis skipped for that batch (Story 4, Scenario 6).
- **Maximum-length agent response** → continues to stream/batch normally; barge-in remains available throughout (Stories 4–5).
- **Duplicate committed transcripts** → de-duplicated, exactly-once forwarding (Story 2, Scenario 3; Story 3, Scenario 5).
- **Setup delay** → session-ready signal only sent once STT + channel resolution succeed, bounding silence (Story 1).
- **Barge-in during speaking** → Story 5.
- **STT connection drop mid-call** → Story 2, Scenario 5.
- **Caller hangup while agent speaks** → Story 9, Scenario 2.
- **Concurrency / cross-call isolation** → Story 7, Scenario 3.
- **Idempotency of turn forwarding** → Story 2/3.
- **Unknown/withheld caller ID** → the gateway forwards whatever caller identity Asterisk provides (including empty/anonymous markers) to Flows/Courier unmodified; URN/anonymous-contact construction is Courier's responsibility (BD-010) and is out of this repo's scope.
- **Tenant isolation** → channel/tenant resolution in Story 1 scopes every subsequent operation (STT/TTS keys, callback URL, language) to that tenant; no cross-tenant state is ever shared on a `CallSession`.
- **STT unavailable at connect / agent pipeline unavailable at connect** → Story 1, Scenarios 5–6.
- **TTS error for a batch** → Story 4, Scenario 5.
- **Unexpected payload shapes from STT/Nexus** → unrecognized message types/fields are logged and ignored; the session continues (defensive parsing, consistent with Constitution Principle II).
- **High concurrent-call volume / queuing** → Story 7, Scenario 4.
- **Long calls (10+ turns)** → Story 7, Scenario 2.
- **ElevenLabs rate limiting/throttling** → treated the same as a TTS/STT failure (Story 2 Scenario 5 / Story 4 Scenario 5) with backoff before giving up on a batch or session.
- **Configured language unsupported by STT/TTS** → Story 6, Scenario 4.
- **Mid-call language change** → Story 6, Scenario 3.

## Requirements *(mandatory)*

### Functional Requirements

**Session establishment & channel resolution**

- **FR-001**: The gateway MUST expose an HTTP endpoint that accepts a call-session registration request containing DID, caller identity, and an origin tag, and returns a session UUID to be used by the subsequent AudioSocket connection. (Story 1; implements Product FR-035.)
- **FR-002**: On session registration, the gateway MUST resolve the Courier channel/tenant for the given DID via a Flows/Courier internal API call before accepting any audio for that session. (Story 1; implements Product FR-037.)
- **FR-003**: The gateway MUST run an AudioSocket TCP listener that accepts the audio connection for a previously registered session UUID and attaches it to that session's `CallSession` state. (Story 1.)
- **FR-004**: The gateway MUST open a dedicated ElevenLabs Scribe v2 Realtime WebSocket session per `CallSession`, authenticated with the resolved tenant's ElevenLabs API key, before signaling readiness. (Story 1; implements Product FR-002.)
- **FR-005**: The gateway MUST reject session registration and MUST NOT open an AudioSocket/STT session when the DID does not resolve to a Courier channel. (Story 1, Scenario 4.)
- **FR-006**: The gateway MUST transition a `CallSession` to `Error` and initiate teardown, signaling a spoken-fallback error code, when STT session establishment or channel resolution fails at setup. (Story 1, Scenarios 5–6; implements Product FR-005.)

**Speech-to-text**

- **FR-007**: The gateway MUST convert AudioSocket audio frames (PCM 16-bit LE mono, 8 kHz) to the encoding ElevenLabs Scribe v2 Realtime requires (PCM 16 kHz, Base64) before forwarding. (Story 2; implements Product FR-007.)
- **FR-008**: The gateway MUST forward audio to STT without blocking the AudioSocket connection's read loop. (Story 2.)
- **FR-009**: The gateway MUST treat exactly one ElevenLabs `committed_transcript` event as exactly one conversational turn and MUST NOT forward it more than once. (Story 2, Scenario 3; implements Product FR-010.)
- **FR-010**: The gateway MUST discard empty/whitespace-only committed transcripts without forwarding them downstream. (Story 2, Scenario 4; implements Product FR-009.)
- **FR-011**: The gateway MUST detect an unexpected STT WebSocket closure during an active call and automatically reopen a new STT session with the same tenant/language configuration, without ending the call. (Story 2, Scenario 5; implements Product FR-011.)
- **FR-012**: The gateway MUST drop and log malformed/undecodable AudioSocket audio frames without terminating the session. (Story 2, Scenario 6.)

**Agent pipeline integration**

- **FR-013**: The gateway MUST POST each committed transcript to the Courier channel callback URL resolved in FR-002, carrying the raw caller identity and origin tag, using the same outbound delivery mechanism already used for other channel types. (Story 3; implements Product FR-012, FR-038.)
- **FR-014**: The gateway MUST register each active `CallSession` in the existing `ClientManager`/`Router` as a connected client keyed by the **bare, scheme-stripped** contact identifier — never the full `tel:`-prefixed URN — matching the exact identifier space `pkg/grpc/server.go`'s `normalizeContactURN` and existing WebSocket clients (`c.ID = payload.From`) already use, so the existing gRPC `MessageStreamService` delivers Nexus response deltas to it without any change to the gRPC server code. (Story 3, Scenarios 2–3; implements Product FR-013 delivery path.)
- **FR-015**: The gateway MUST NOT crash or terminate a `CallSession` when the Flows/Courier callback POST fails; it MUST log the failure and retry per the operation's configured policy. (Story 3, Scenario 4.)

**Text-to-speech & batching**

- **FR-016**: The gateway MUST accumulate streamed delta text per `CallSession` until a sentence boundary or a configured minimum-length threshold before issuing an ElevenLabs TTS request. (Story 4, Scenario 1; implements Product FR-016.)
- **FR-017**: The gateway MUST begin streaming the first ready TTS batch's audio to Asterisk without waiting for the remainder of the agent response. (Story 4, Scenario 2; implements Product FR-017.)
- **FR-018**: The gateway MUST play sequential TTS batches with no perceptible gap between them (target <100 ms, per Product SC-003). (Story 4, Scenario 3; implements Product FR-018.)
- **FR-019**: The gateway MUST request ElevenLabs TTS output in the sample rate/encoding Asterisk consumes (PCM 8 kHz) to avoid resampling on the return path. (Story 4, Scenario 2; implements Product FR-019.)
- **FR-020**: The gateway MUST flush any remaining buffered delta text as a final TTS request when the response stream completes. (Story 4, Scenario 4.)
- **FR-021**: The gateway MUST skip a batch whose TTS request fails, continue with subsequent batches, and return the `CallSession` to `Listening` without dropping the call. (Story 4, Scenario 5; implements Product FR-021.)
- **FR-022**: The gateway MUST skip TTS synthesis for a batch identified as non-speakable content (e.g., only a URL, code, or emoji). (Story 4, Scenario 6.)
- **FR-023**: The gateway MUST use the channel's configured voice ID and resolved language for every TTS request in a session. (Story 4, Scenario 7; implements Product FR-020.)

**Barge-in**

- **FR-024**: The gateway MUST treat any `partial_transcript` event received from the always-on STT session while the `CallSession` is `Speaking` as caller voice activity that triggers barge-in. (Story 5, Scenario 1; implements Product FR-022, FR-026.)
- **FR-025**: The gateway MUST stop streaming TTS audio to Asterisk within 300 ms of a barge-in trigger. (Story 5, Scenario 1; implements Product NFR-002, SC-004.)
- **FR-026**: The gateway MUST discard all buffered/in-flight TTS audio and any unflushed delta text for the interrupted response on barge-in. (Story 5, Scenario 2; implements Product FR-024.)
- **FR-027**: The gateway MUST treat the caller's post-barge-in committed transcript as a new, independent turn, never appended to the interrupted response. (Story 5, Scenario 3; implements Product FR-025.)

**Language**

- **FR-028**: The gateway MUST resolve the `CallSession`'s language once at setup via the existing `flows.IClient.GetChannelProjectLanguage` call (or equivalent), defaulting to `"en"` when unset. (Story 6, Scenarios 1–2; implements Product FR-027, FR-028.)
- **FR-029**: The gateway MUST apply an updated language to subsequently opened STT sessions and to all subsequent TTS requests when a language-change signal is received. (Story 6, Scenario 3; implements Product FR-029.)
- **FR-030**: The gateway MUST fall back to `"en"` and proceed, rather than fail, when the resolved language is unsupported by ElevenLabs STT/TTS. (Story 6, Scenario 4.)

**Lifecycle & concurrency**

- **FR-031**: The gateway MUST automatically return a `CallSession` to `Listening` once all queued TTS batches for a turn finish playing. (Story 7, Scenario 1; implements Product FR-030.)
- **FR-032**: The gateway MUST isolate every `CallSession`'s audio buffers, STT/TTS connections, and turn state from every other concurrent `CallSession`. (Story 7, Scenario 3; implements Product FR-033, NFR-003.)
- **FR-033**: The gateway MUST enforce a configurable maximum concurrent-call capacity; when reached, MUST queue additional session-registration requests, play hold audio, and admit them in arrival order as slots free. (Story 7, Scenario 4; implements Product FR-034, SC-011.)
- **FR-034**: The gateway MUST release a `CallSession`'s concurrency slot immediately on teardown, regardless of the termination reason. (Story 7, Scenario 5; Story 9, Scenario 1.)
- **FR-035**: The gateway MUST close STT/TTS WebSocket sessions, stop any in-flight synthesis/playback, and free all resources when an AudioSocket hangup frame is received or a server-side forced termination is issued. (Story 9, Scenarios 1–2, 4; implements Product FR-032.)
- **FR-036**: The gateway MUST emit structured logs/metrics for call setup, STT commit, agent round-trip, TTS, and teardown for every `CallSession`. (Story 9, Scenario 3; implements Product NFR-005.)

### Key Entities *(gateway-scoped restatement of the Product Spec's Key Entities)*

- **CallSession**: In-process representation of one active call. Fields: session UUID, DID, origin tag, raw caller identity, resolved `channel_uuid`/`project_uuid`/callback URL, contact `tel:` URN (as received back from Courier once available — kept in full form for logging/traceability; the `ClientManager`/`Router` registration key derived from it is always the bare, scheme-stripped form, per FR-014), resolved language, current state (`Connecting`|`Listening`|`Processing`|`Speaking`|`Error`|`Ended`), the AudioSocket connection, the active STT WebSocket connection, the active TTS batching buffer, and per-session metrics counters. One `CallSession` exists per phone call for its full lifetime.
- **VoiceConfig**: Per-tenant/channel settings resolved at session setup — ElevenLabs API key, voice ID, language code, VAD silence threshold, TTS batching thresholds (min length / sentence-boundary rule), STT/TTS model IDs, concurrent-call capacity limit.
- **Turn**: One exchange within a `CallSession` — the committed caller transcript plus the resulting sequence of agent deltas — tracked only long enough to batch/synthesize TTS and to guarantee exactly-once forwarding; persistence of the turn as message history is Flows/Courier's responsibility (out of this repo's scope), reached via the callback POST (FR-013).
- **VoiceError**: A structured internal error (code, spoken-fallback message, recoverable flag) raised by STT/agent-pipeline/TTS/media failures, used to decide whether a `CallSession` degrades gracefully in place (recoverable) or tears down (non-recoverable). Mirrors the Product Spec's `Voice Error` entity.

## Success Criteria *(mandatory — inherited from Product Spec; restated with the gateway's observable surface)*

- **SC-001**: The gateway signals session-ready (enabling the greeting) within 3 seconds of the AudioSocket connection attaching to a registered session, in ≥95% of setups under nominal load (inherits Product SC-001).
- **SC-002**: Measured from the last STT `committed_transcript` byte to the first TTS audio byte written to the AudioSocket connection, gateway-internal latency is under 2 seconds at P95 (inherits Product SC-002; excludes Flows/Nexus round-trip time, which is out of this repo's control but is included in the end-to-end Product SC-002 budget).
- **SC-003**: Gap between sequential TTS batches written to the AudioSocket connection is under 100 ms (inherits Product SC-003).
- **SC-004**: From `partial_transcript` received while `Speaking` to the last TTS audio byte written before playback stops, elapsed time is under 300 ms in ≥95% of barge-in attempts (inherits Product SC-004).
- **SC-005**: A 3-sentence agent response results in at most 3–4 ElevenLabs TTS requests (inherits Product SC-005).
- **SC-006**: A single `CallSession` sustains 10+ turns with no measurable growth in per-turn latency or STT/TTS error rate (inherits Product SC-006).
- **SC-007**: Every committed transcript successfully triggers exactly one callback POST attempt sequence (including retries) to Flows/Courier (inherits Product SC-007; downstream persistence is Flows/Courier's SC).
- **SC-008**: 100% of `CallSession`s use the resolved channel language (or English default) for both STT and TTS (inherits Product SC-008).
- **SC-009**: Under N concurrent `CallSession`s (N = configured capacity), no session's logs, metrics, or in-memory state reference another session's identifiers (inherits Product SC-009).
- **SC-010**: 100% of STT-unavailable, agent-pipeline-unavailable, and TTS-failure conditions result in the defined graceful degrade/teardown path — never a silent stall (inherits Product SC-010).
- **SC-011**: When at capacity, 100% of additional session-registration requests are queued and admitted in arrival order — none silently dropped (inherits Product SC-011).

## Assumptions

- The Product Spec's deferred technical items are resolved for this repository as follows (all engineering-level, non-product decisions — see `research.md`):
  - Asterisk↔gateway transport: **AudioSocket** (TCP).
  - The Flows/Courier internal endpoint for DID→`{channel_uuid, callback_url, project_uuid}` resolution does not exist yet; its contract is proposed in `contracts/flows-pstn-integration.md` and MUST be confirmed with the Courier team before or during implementation of Story 1/3. This repo's tasks build against that proposed contract behind an interface, so the concrete endpoint can be swapped in without touching call-handling logic.
  - The Asterisk-side dialplan/AGI/ARI script that calls the session-registration HTTP endpoint before dialing `AudioSocket()` is assumed to exist or be built in the (currently `[NOT YET DEFINED]`) Asterisk/telephony deployment repository; this repo only defines and documents the HTTP contract it depends on (`contracts/audiosocket-session-protocol.md`).
- ElevenLabs credentials continue to be resolved per-tenant via the existing `flows.IClient.GetElevenLabsAPIKey(channelUUID)` — no new credential-storage mechanism is introduced (consistent with Product NFR-004 and repo Constitution III).
- Hold-audio content/format for the capacity queue (FR-033) is a fixed, pre-recorded asset shipped with the gateway; its content is not a product decision requiring further clarification (silence-avoidance is what the Product Spec requires, not specific wording).
- This spec covers only `weni-webchat-socket`. Corresponding engineering specs for `engineering/courier` (new PSTN channel type, URN construction, DID config) and the Asterisk/telephony deployment repository are expected to exist or be authored separately, each pinning this same Product Spec commit.

## Scope Boundaries

### In Scope

- HTTP session-registration endpoint + AudioSocket TCP listener (Story 1).
- Audio format conversion and forwarding to ElevenLabs STT; VAD-based turn commit; STT reconnect-on-drop (Story 2).
- Outbound callback POST of committed transcripts; registering `CallSession` as a `Router`/`ClientManager` delivery target for the existing gRPC pipeline (Story 3).
- Sentence-boundary TTS batching; streaming synthesized audio back over AudioSocket (Stories 4, 8).
- Barge-in detection (reusing the STT session's own partial-transcript signal) and cancellation of in-flight TTS/deltas (Story 5).
- Per-channel language resolution and propagation to STT/TTS (Story 6).
- Automatic turn-taking, per-call isolation, concurrency capacity + queueing with hold audio (Story 7).
- Graceful teardown and observability (Story 9).
- New Go packages, config (`WWC_*` env vars), and a new gateway process/entrypoint for AudioSocket handling, following existing project conventions (`pkg/flows`, `pkg/history`, `pkg/starters` patterns).

### Out of Scope

- Anything owned by Courier (new PSTN channel type, DID configuration UI/storage, `tel:` URN construction, contact get-or-create) — BD-010, FR-035–FR-038's Courier-side half.
- Anything owned by the Asterisk/telephony deployment repo (dialplan, SIP trunking, carrier contracts, the AGI/ARI script that calls this gateway's registration endpoint).
- Nexus agent logic, prompt design, or tool behavior.
- Any change to `001-full-voice-mode` browser-based voice mode code paths (BD-009) — this feature adds new packages/entrypoints; it does not modify the existing token-issuance flow used by the browser.
- Outbound/dial-out calls, IVR/DTMF, call recording, voicemail, human call transfer — all explicitly out of scope in the Product Spec.
- Acoustic echo cancellation (EchoGuard) — explicitly N/A per Product BD-007.
