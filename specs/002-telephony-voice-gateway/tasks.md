# Tasks: Telephony Voice Gateway (Asterisk ↔ ElevenLabs ↔ Flows/Nexus)

**Input**: Design documents from `/specs/002-telephony-voice-gateway/`
**Prerequisites**: [plan.md](./plan.md) (required), [spec.md](./spec.md) (required), [research.md](./research.md), [data-model.md](./data-model.md), `contracts/`

**Tests**: Included per Constitution Principle IV (Test-First Quality Gates) — every implementation task in every user story phase is preceded by its test task(s).

**Organization**: Tasks are grouped by user story (US1–US9, matching `spec.md`) so each can be implemented, tested, and demoed independently, in Product-Spec priority order (P1 stories first).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on other unfinished tasks in this phase)
- **[Story]**: Maps the task to its user story for traceability back to `spec.md`
- File paths are exact, per `plan.md` Project Structure

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: New config surface and package skeleton — no behavior yet.

- [X] T001 Add `Telephony` sub-struct to `Configuration` in `config/config.go` with env-tagged fields: `HTTPPort` (`WWC_TELEPHONY_HTTP_PORT`, default `8081`), `AudioSocketPort` (`WWC_TELEPHONY_AUDIOSOCKET_PORT`, default `9095`), `MaxConcurrentCalls` (`WWC_TELEPHONY_MAX_CONCURRENT_CALLS`, default `100`), `VADSilenceMs` (`WWC_TELEPHONY_VAD_SILENCE_MS`, default `1500`), `TTSMinBatchChars` (`WWC_TELEPHONY_TTS_MIN_BATCH_CHARS`, default `40`), `STTModelID` (`WWC_TELEPHONY_STT_MODEL_ID`, default `scribe_v2_realtime`), `TTSModelID` (`WWC_TELEPHONY_TTS_MODEL_ID`, default `eleven_flash_v2_5`), `HoldAudioPath` (`WWC_TELEPHONY_HOLD_AUDIO_PATH`), `VoiceID` (`WWC_TELEPHONY_VOICE_ID`)
- [X] T002 [P] Create package skeletons with `doc.go` GoDoc package comments: `pkg/telephony/session/`, `pkg/telephony/audiosocket/`, `pkg/telephony/stt/`, `pkg/telephony/tts/`
- [X] T003 [P] Create `telephony/main.go` skeleton: flag/env bootstrap identical in shape to `api/main.go` (load config, connect Redis, connect Mongo, construct `flows.Client`) with a `TODO` marker where `SessionManager` wiring lands in Phase 2

**Checkpoint**: Config loads; new packages compile as empty shells; no runtime behavior yet.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core abstractions every user story depends on. **No user story work may begin until this phase is complete.**

- [X] T004 [P] Define `session.State` enum and `session.CallSession` struct in `pkg/telephony/session/call_session.go` per `data-model.md` (fields: ID, DID, CallerID, Origin, ChannelUUID, ProjectUUID, CallbackURL, ContactURN, Language, State + mutex, CreatedAt), with a single internal goroutine-driven state-transition method (`transition(to State) error`) that rejects invalid transitions (e.g. `Ended` → anything). Also add `func (cs *CallSession) RegistrationKey() string` returning `ContactURN` with any `scheme:` prefix stripped (mirrors `pkg/grpc/server.go`'s `normalizeContactURN`; empty string if `ContactURN` is not yet resolved) — this, not `ContactURN`, is the only key ever passed to `ClientManager`/`Router` (T042/T043) or to the `SessionManager` registration-key index (T014). Unit test: `RegistrationKey()` on `"tel:+15559876543"` returns `"+15559876543"`
- [X] T005 [P] Define `session.VoiceConfig` struct and a `ResolveVoiceConfig(flowsClient flows.IClient, channelUUID string) (*VoiceConfig, error)` function in `pkg/telephony/session/voice_config.go` that calls `GetElevenLabsAPIKey`, `GetChannelProjectLanguage` (default `"en"` on empty), and reads defaults from `config.Get().Telephony`
- [X] T006 [P] Define `session.Turn` struct in `pkg/telephony/session/turn.go` per `data-model.md` (MsgID, CommittedText, StartedAt, DeltaBuffer, BatchesIssued, Interrupted)
- [X] T007 [P] Define `session.VoiceError`, `session.ErrorCode` constants, and `session.Recoverable(err)` helper in `pkg/telephony/session/voice_error.go` per `data-model.md`
- [X] T008 Add `ResolvePSTNChannel(did string) (channelUUID, projectUUID, callbackURL string, err error)` to the `flows.IClient` interface and `flows.Client` implementation in `pkg/flows/client.go`, calling `GET {BaseURL}/api/v2/internals/pstn_channel?did=<did>` per `contracts/flows-pstn-integration.md` §1, treating 404 as `(\"\", \"\", \"\", nil)` (not-configured, not an error) exactly like the existing `GetElevenLabsAPIKey` 404 handling
- [X] T009 [P] Generate/hand-write `flows.IClient` mock updates (existing mock location/pattern) to include `ResolvePSTNChannel`
- [X] T010 Define `audiosocket.AudioSocketConn` interface (`ReadFrame() (Frame, error)`, `WriteAudio([]byte) error`, `Close() error`) and the AudioSocket frame types/constants — `KindHangup = 0x00`, `KindUUID = 0x01`, `KindDTMF = 0x03`, `KindAudio = 0x10`, `KindError = 0xFF` (per `research.md` §1 / Asterisk's `ast_audiosocket_msg_kind` enum — note `0x03` is DTMF, not error) — in `pkg/telephony/audiosocket/conn.go`, plus the real TCP-backed implementation. `ReadFrame` MUST surface `KindDTMF` frames to the caller as a recognized-but-ignored kind (never coerced into the malformed-frame or error path), and only `KindError` frames feed the error-observability path (T016)
- [X] T011 [P] Implement the AudioSocket TCP listener in `pkg/telephony/audiosocket/server.go`: accepts connections, reads the first frame expecting `0x01` (16-byte UUID), and exposes a `Server` with an injectable `OnConnect(sessionID string, conn AudioSocketConn)` callback (wired to `SessionManager` in T014) — no STT/TTS logic here, purely protocol framing
- [X] T012 [P] Define `stt.STTSession` interface (`Send(audio []byte) error`, `Events() <-chan stt.Event`, `Close() error`) in `pkg/telephony/stt/client.go`, where `stt.Event` is a small tagged union for `PartialTranscript{Text string}` / `CommittedTranscript{Text string}` / `Closed{Err error}`
- [X] T013 [P] Define `tts.TTSStreamClient` interface (`Synthesize(ctx context.Context, text, voiceID, language string) (<-chan []byte, error)`) in `pkg/telephony/tts/client.go`
- [X] T014 Implement `session.SessionManager` in `pkg/telephony/session/manager.go`: in-process registry (`map[string]*CallSession` keyed by session UUID, `sync.RWMutex`-guarded), `Register(did, callerID, origin string) (sessionID string, err error)` (calls `ResolvePSTNChannel`, creates a `Connecting` `CallSession`, enforces `MaxConcurrentCalls` by returning a queued/hold-audio marker rather than an error — see T015), `Attach(sessionID string, conn audiosocket.AudioSocketConn) error`, `Get(sessionID string) (*CallSession, bool)`, `Remove(sessionID string)`. Also maintain a second index, `map[string]*CallSession` keyed by `CallSession.RegistrationKey()` (the bare, scheme-stripped contact identifier — see T004/`data-model.md`; populated once `ContactURN` is resolved, not at `Register` time), with a `GetByRegistrationKey(key string) (*CallSession, bool)` accessor — this is a distinct index from the by-session-UUID map above and is what T043's `DeliverFunc` branch looks up
- [X] T015 [US-cross-cutting, foundational for US7] Implement capacity queueing in `session.SessionManager`: when `MaxConcurrentCalls` is reached, `Register` still returns a session ID (per `contracts/audiosocket-session-protocol.md` §3) but marks the `CallSession` `Queued` (add to `State` enum in T004) until a slot frees via `Remove`; queued sessions play `HoldAudioPath` in a loop once their AudioSocket connection attaches (`Attach` checks `Queued` and starts hold-audio playback instead of proceeding to STT setup)
- [X] T016 [P] Implement `session.SessionMetrics` in `pkg/telephony/session/metrics.go` wrapping `pkg/metric.Service` with named counters/histograms: `telephony_call_setup_duration_seconds`, `telephony_stt_commit_latency_seconds`, `telephony_agent_roundtrip_seconds`, `telephony_tts_batch_duration_seconds`, `telephony_bargein_latency_seconds`, `telephony_call_teardown_total{reason}`, `telephony_active_calls`, `telephony_queued_calls`
- [X] T017 Wire `SessionManager`, the AudioSocket `Server`, and the registration HTTP handler stub into `telephony/main.go`, replacing the T003 TODO

**Checkpoint**: A session can be registered (against a mocked/real `ResolvePSTNChannel`), an AudioSocket connection can attach to it, capacity queueing works, and metrics scaffolding exists. No STT/TTS/Flows-forwarding/TTS-batching/barge-in behavior yet — those are the user stories below.

---

## Phase 3: User Story 1 — Accept a Call and Establish the Media + STT Session (Priority: P1) 🎯 MVP

**Goal**: A DID resolves to a channel, an AudioSocket connection attaches, a dedicated ElevenLabs STT session opens, and the caller hears a greeting — or the call ends gracefully with a spoken fallback if any step fails.

**Independent Test**: Per `spec.md` User Story 1 — register a session for a known DID, attach a simulated AudioSocket connection, and verify an STT session opens and a greeting audio frame is written back; repeat for an unknown DID and an STT-unavailable scenario and verify graceful rejection/teardown.

### Tests for User Story 1

- [X] T018 [P] [US1] Unit tests for `session.SessionManager.Register`/`Attach` happy path and unknown-DID rejection in `pkg/telephony/session/manager_test.go`, using the `flows.IClient` mock from T009
- [X] T019 [P] [US1] Unit tests for the registration HTTP handler (`POST /telephony/sessions`) — valid request, missing `did`, missing `origin`, unknown DID (404), dependency-down (503) — in `pkg/telephony/audiosocket/registration_test.go`
- [X] T020 [P] [US1] Unit tests for `stt.Client` session-open success/auth-failure/unavailable paths (mocked WebSocket dialer) in `pkg/telephony/stt/client_test.go`
- [X] T021 [P] [US1] Integration-style test in `pkg/telephony/session/call_session_test.go`: full setup sequence (register → attach → STT opens → greeting sent → state reaches `Listening`) against mocked `flows.IClient`, `stt.STTSession`, and `tts.TTSStreamClient`
- [X] T022 [P] [US1] Test STT-setup-failure and channel-resolution-failure paths drive `CallSession` to `Error` + teardown + spoken-fallback signal, in `pkg/telephony/session/call_session_test.go`

### Implementation for User Story 1

- [X] T023 [US1] Implement `pkg/telephony/audiosocket/registration.go`: `POST /telephony/sessions` handler — validate `did`/`origin`, call `SessionManager.Register`, map its result to `200`/`404`/`503` per `contracts/audiosocket-session-protocol.md` §1
- [X] T024 [US1] Implement the real ElevenLabs Scribe v2 Realtime client in `pkg/telephony/stt/client.go` (`gorilla/websocket` dial with API-key header, session-open params from `VoiceConfig`, `Events()` channel fed by a read-loop goroutine)
- [X] T025 [US1] Implement `CallSession.setup()` orchestration in `pkg/telephony/session/call_session.go`: on `Attach`, call `ResolveVoiceConfig` (T005) if not already resolved at `Register` time, open the STT session (T024) via the injected `stt.STTSession` factory, transition `Connecting` → `Listening` on success or `Error` (with `VoiceError`) on failure, per `spec.md` US1 Scenarios 1–6
- [X] T026 [US1] Implement a minimal single-shot greeting path in `pkg/telephony/tts/client.go` (`Synthesize` used directly, not through `TTSBatcher` which lands in Phase 6) using a fixed, localized greeting text resolved via `WWC_TELEPHONY_GREETING_TEXT_KEY` and `VoiceConfig.Language`, writing resulting audio frames to the `AudioSocketConn`
- [X] T027 [US1] Implement graceful error teardown in `CallSession`: on `Error`, synthesize and play a localized spoken-fallback message (same single-shot path as T026) before closing the AudioSocket connection and releasing the `SessionManager` slot (implements Product FR-005)
- [X] T028 [US1] Wire the registration HTTP server and AudioSocket `Server.OnConnect` → `SessionManager.Attach` → `CallSession.setup()` end-to-end in `telephony/main.go`

**Checkpoint**: A call can be answered end-to-end through STT session establishment and a spoken greeting, with graceful failure paths — demoable without any agent/TTS-batching/barge-in logic.

---

## Phase 4: User Story 2 — Stream Caller Audio to STT and Produce a Committed Transcript (Priority: P1)

**Goal**: Continuous caller audio becomes exactly-once committed transcripts; STT drops are recovered without ending the call.

**Independent Test**: Per `spec.md` User Story 2 — feed AudioSocket audio frames into an active session against a mocked STT connection emitting partials then one commit; verify exactly one downstream hand-off, no duplication, and recovery from a simulated STT close.

### Tests for User Story 2

- [X] T029 [P] [US2] Unit tests for 8 kHz→16 kHz PCM conversion and Base64 `input_audio_chunk` framing in `pkg/telephony/stt/client_test.go`
- [X] T030 [P] [US2] Unit tests for exactly-once committed-transcript hand-off (including a duplicate-delivery simulation) in `pkg/telephony/session/call_session_test.go`
- [X] T031 [P] [US2] Unit test: empty/whitespace-only committed transcript is discarded without a downstream call, in `pkg/telephony/session/call_session_test.go`
- [X] T032 [P] [US2] Unit test: malformed/short AudioSocket audio frame is dropped and logged without terminating the session, in `pkg/telephony/audiosocket/server_test.go`
- [X] T033 [US2] Unit test: STT WebSocket unexpected close triggers automatic reconnect with the same `VoiceConfig` and the call continues, in `pkg/telephony/stt/client_test.go` and `pkg/telephony/session/call_session_test.go`

### Implementation for User Story 2

- [X] T034 [US2] Implement PCM 8 kHz → 16 kHz upsampling + Base64 encoding in `pkg/telephony/stt/audio.go`, called from the AudioSocket read loop before forwarding to `STTSession.Send`
- [X] T035 [US2] Implement the AudioSocket read loop in `pkg/telephony/audiosocket/conn.go`/`server.go`: on each `0x10` audio frame, validate length (320 bytes ± tolerance), drop+log malformed frames, forward valid ones to `CallSession` non-blockingly (buffered channel, per FR-008)
- [X] T036 [US2] Implement `CallSession.handleSTTEvent()` in `pkg/telephony/session/call_session.go`: on `PartialTranscript`, update in-memory tracking only (and check `BargeInController.armed`, wired fully in Phase 7); on `CommittedTranscript`, discard if empty/whitespace, else finalize the current `Turn` and hand off exactly once (idempotency guard keyed by an internal turn sequence, not `MsgID` yet — `MsgID` is assigned in Phase 5)
- [X] T037 [US2] Implement STT reconnect-on-drop in `pkg/telephony/stt/client.go`: on `Closed` event with a non-graceful error, redial with the session's stored `VoiceConfig`, replace the `CallSession`'s active `STTSession` reference under its state-transition lock, and resume forwarding buffered audio (implements FR-011)

**Checkpoint**: Sustained caller speech reliably produces exactly-once committed transcripts, resilient to a transient STT drop.

---

## Phase 5: User Story 3 — Forward Transcript to Flows/Courier and Receive Streamed Agent Deltas (Priority: P1)

**Goal**: Committed transcripts leave the gateway toward Flows/Courier, and the `CallSession` becomes a valid delivery target for the existing, unmodified gRPC pipeline.

**Independent Test**: Per `spec.md` User Story 3 — register a `CallSession` for a `tel:` contact URN, send a committed transcript, and drive the existing gRPC `StreamMessages` test harness with `setup`/`delta`/`completed` for that URN; verify the `CallSession`'s delivery handler (not a WebSocket client) receives each event.

### Tests for User Story 3

- [X] T038 [P] [US3] Unit tests for the outbound callback POST (success, non-2xx, network error/retry) in `pkg/telephony/session/delivery_test.go`
- [X] T039 [P] [US3] Unit tests for `CallSession` registration/deregistration in `ClientManager` (`AddConnectedClient`/`RemoveConnectedClient`) at the right lifecycle points, in `pkg/telephony/session/delivery_test.go`
- [X] T040 [US3] Integration test reusing the existing gRPC test harness (see `pkg/grpc/server_test.go` patterns): register a `CallSession`-backed `DeliverFunc` for a `tel:` URN, call `StreamMessages` with `setup`→`delta`→`completed`, and assert the `CallSession`'s handler (not `ClientPool`) receives `stream_start`/`delta`/`stream_end` payloads, in `pkg/telephony/session/delivery_test.go`

### Implementation for User Story 3

- [X] T041 [US3] Implement `session.PostTranscript(callbackURL, callerID, origin, did, text string) error` in `pkg/telephony/session/delivery.go`, POSTing the payload shape from `contracts/flows-pstn-integration.md` §2, with retry/backoff consistent with other outbound HTTP calls in the codebase; parse the response for `contact_urn` per §3 (fallback to locally constructed `tel:<callerID>` if absent, per `plan.md` Complexity Tracking)
- [X] T042 [US3] Implement `session.RegisterDelivery(cs *CallSession, clientManager websocket.ClientManager, podID string) error` and `session.DeregisterDelivery(cs *CallSession, clientManager websocket.ClientManager) error` in `pkg/telephony/session/delivery.go`, calling `AddConnectedClient`/`RemoveConnectedClient` with `CallSession.RegistrationKey()` (the bare, scheme-stripped form of `ContactURN` — **not** `ContactURN` itself) as the key, per `contracts/grpc-telephony-delivery.md` §1. Add a unit test asserting `RegistrationKey()` strips the scheme prefix exactly as `pkg/grpc/server.go`'s `normalizeContactURN` does, so a mismatch here fails a test rather than silently breaking delivery
- [X] T043 [US3] Implement a **dedicated, telephony-owned `streams.Router` instance** in `telephony/main.go` (own `podID`, e.g. `telephony-<hostname>`, following the exact pattern `grpc/main.go` and `pkg/websocket.NewStreamsRouter` already establish — each binary constructs its own `Router`, none are shared) whose `deliver DeliverFunc` closure (defined in `pkg/telephony/session/delivery.go`) looks up the incoming `clientID` (always the bare, scheme-stripped identifier — `pkg/grpc/server.go` normalizes it before publishing, see `research.md` §5) against `SessionManager.GetByRegistrationKey` (T014) and, if found, delivers to that `CallSession.handleGRPCPayload(raw []byte)`. **No `if` branch and no fallthrough to a WebSocket `ClientPool` lookup is needed or correct here** — `pkg/streams/router.go`'s `PublishToClient` already resolves the target pod from `ConnectedClient.PodID` before ever appending to a stream, so a `CallSession` (registered with `PodID` = the telephony pod's own id) only ever receives stream entries on the telephony pod's own `Router`, never `api`'s. This task touches only `telephony/main.go` and `pkg/telephony/session/delivery.go` — **zero changes to `pkg/streams/router.go` or `pkg/websocket/router_factory.go`**, which would be both unnecessary and a violation of this plan's "no `pkg/websocket` changes" invariant (BD-009)
- [X] T044 [US3] Implement `CallSession.handleGRPCPayload(raw []byte)` in `pkg/telephony/session/call_session.go`: unmarshal into the existing `websocket.StreamStartPayload`/`StreamDeltaPayload`/`StreamEndPayload` shapes (no new types), and dispatch `stream_start`→new `Turn`, `delta`→(stub call into `TTSBatcher.Append`, fully wired in Phase 6), `stream_end`→(stub call into `TTSBatcher.Flush`, fully wired in Phase 6)
- [X] T045 [US3] Wire `PostTranscript` into `CallSession.handleSTTEvent()`'s committed-transcript path (T036), and wire `RegisterDelivery`/`DeregisterDelivery` into the session lifecycle (`setup()` from T025 and teardown, fully implemented in Phase 11) in `pkg/telephony/session/call_session.go`

**Checkpoint**: A committed transcript reaches Flows/Courier, and Nexus's response deltas (via the existing, untouched gRPC service) are observably routed to the right `CallSession` instead of any WebSocket client.

---

## Phase 6: User Story 4 — Batch and Synthesize the Agent Response as Speech (Priority: P1)

**Goal**: Delta text is batched at sentence boundaries and streamed to Asterisk as TTS audio, starting playback before the full response is ready, with no perceptible gap between batches.

**Independent Test**: Per `spec.md` User Story 4 — feed a mocked 3-sentence delta stream into an active `CallSession` against a mocked TTS client; verify ≤3–4 TTS requests, early playback start, and gapless sequencing.

### Tests for User Story 4

- [ ] T046 [P] [US4] Unit tests for sentence-boundary and min-threshold batching logic (various punctuation/length combinations) in `pkg/telephony/tts/batcher_test.go`
- [ ] T047 [P] [US4] Unit test: final flush of remaining buffered text on `stream_end` with no trailing sentence boundary, in `pkg/telephony/tts/batcher_test.go`
- [ ] T048 [P] [US4] Unit test: non-speakable batch content (URL/code/emoji-only) is skipped without a TTS call, in `pkg/telephony/tts/batcher_test.go`
- [ ] T049 [P] [US4] Unit test: a batch's TTS request failing is skipped, next batch proceeds, session returns to `Listening` if it was last, in `pkg/telephony/tts/batcher_test.go`
- [ ] T050 [US4] Integration test: 3-sentence delta stream end-to-end (delta→batch→mocked TTS→AudioSocket write) produces ≤3–4 TTS requests and gapless sequential playback (assert write ordering + no gap flag), in `pkg/telephony/session/call_session_test.go`, validating Product SC-003 and SC-005

### Implementation for User Story 4

- [ ] T051 [US4] Implement `tts.TTSBatcher` in `pkg/telephony/tts/batcher.go` per `data-model.md`: `Append(delta string)` (sentence-boundary/min-threshold detection), `Flush(final bool)`, non-speakable-content heuristic (regex/heuristic for URL-only, code-fence-only, emoji-only strings)
- [ ] T052 [US4] Implement the real ElevenLabs streaming TTS client in `pkg/telephony/tts/client.go` (`gorilla/websocket` dial, model/voice/language params, PCM 8 kHz output, streamed audio chunks on a channel) per `contracts/elevenlabs-realtime.md`
- [ ] T053 [US4] Implement sequential, gapless playback in `CallSession`: a single writer goroutine drains `TTSBatcher`'s output channel and writes `0x10` AudioSocket frames, only advancing to batch N+1 once batch N's audio is fully written (implements FR-018, SC-003), in `pkg/telephony/session/call_session.go`
- [ ] T054 [US4] Wire `CallSession.handleGRPCPayload`'s `delta`/`stream_end` stubs (T044) to the real `TTSBatcher.Append`/`Flush`, and transition `CallSession.State` to `Speaking` on the first batch write and back to `Listening` when the writer goroutine drains empty after `Flush(final: true)` (implements FR-030, feeding Phase 9's US7)
- [ ] T055 [US4] Implement per-batch TTS failure handling: catch/log a failed `Synthesize` call in the writer goroutine, skip to the next queued batch, in `pkg/telephony/tts/client.go` / `pkg/telephony/session/call_session.go`

**Checkpoint**: A full committed-transcript → Nexus-delta → spoken-response loop works end-to-end against mocked externals, satisfying the credit-efficiency and gapless-playback success criteria.

---

## Phase 7: User Story 5 — Reliable Barge-In (Priority: P1)

**Goal**: Any real caller speech during playback stops the agent within 300 ms, discards in-flight audio/deltas, and starts a genuinely new turn.

**Independent Test**: Per `spec.md` User Story 5 — drive an active `Speaking` session with a mocked `partial_transcript` mid-playback; verify sub-300ms stop, discarded buffers, and a non-appended next turn.

### Tests for User Story 5

- [ ] T056 [P] [US5] Unit tests for `BargeInController.armed` gating (only triggers while `Speaking`, ignored while `Listening`/`Processing`) in `pkg/telephony/session/bargein_test.go`
- [ ] T057 [P] [US5] Unit test: barge-in trigger stops the AudioSocket writer goroutine and cancels the in-flight `TTSBatcher`/TTS request within the test's simulated 300 ms budget, in `pkg/telephony/session/call_session_test.go`
- [ ] T058 [P] [US5] Unit test: post-barge-in committed transcript starts a new `Turn` with `Interrupted: false`, and the prior `Turn.Interrupted == true`, in `pkg/telephony/session/call_session_test.go`
- [ ] T059 [US5] Latency measurement test asserting the barge-in path (partial_transcript event → last audio frame written) stays under 300 ms using the `SessionMetrics` histogram from T016, in `pkg/telephony/session/call_session_test.go`

### Implementation for User Story 5

- [ ] T060 [US5] Implement `session.BargeInController` in `pkg/telephony/session/bargein.go` per `data-model.md`: `atomic.Bool` armed flag toggled on `Speaking`/other-state transitions, `Trigger()` invoking an injected `onTrigger` callback
- [ ] T061 [US5] Wire `BargeInController.armed` to `CallSession`'s state-transition method (T004/T025/T054): armed=true only while `State == Speaking`
- [ ] T062 [US5] Wire `CallSession.handleSTTEvent()`'s `PartialTranscript` branch (stubbed in T036) to call `BargeInController.Trigger()` when armed
- [ ] T063 [US5] Implement the `onTrigger` callback in `CallSession`: stop the AudioSocket writer goroutine immediately, call `TTSBatcher.Discard()` (cancels in-flight TTS `context.CancelFunc`, drops queued audio), mark the current `Turn.Interrupted = true`, transition `State` to `Listening`, and record the `telephony_bargein_latency_seconds` metric
- [ ] T064 [US5] Add `Discard()` to `tts.TTSBatcher` in `pkg/telephony/tts/batcher.go`: cancels the active synthesis context, drains/drops the output channel, clears the buffer

**Checkpoint**: Barge-in is reliable, fast, and does not corrupt turn state — the Product Spec's explicitly-called-out prototype gap is closed.

---

## Phase 8: User Story 6 — Language Configuration via Channel (Priority: P2)

**Goal**: STT/TTS language is resolved once from the channel/project config (default English) and updated on a mid-call signal.

**Independent Test**: Per `spec.md` User Story 6 — mock the channel language response as `"pt"`, verify STT session-open and every TTS request use `"pt"`; repeat with no config and verify `"en"`.

### Tests for User Story 6

- [ ] T065 [P] [US6] Unit tests for `ResolveVoiceConfig` language resolution (configured, empty→`"en"` default) in `pkg/telephony/session/voice_config_test.go`
- [ ] T066 [P] [US6] Unit test: STT session-open call carries the resolved language in `pkg/telephony/stt/client_test.go`
- [ ] T067 [P] [US6] Unit test: every `TTSBatcher`/TTS request in a session carries the resolved language in `pkg/telephony/tts/batcher_test.go`
- [ ] T068 [US6] Unit test: unsupported language falls back to `"en"` and the session proceeds rather than failing, in `pkg/telephony/session/voice_config_test.go` (mock STT/TTS rejecting the original language code)
- [ ] T069 [US6] Unit test: a mid-call language-change signal updates the language used by the *next* STT session (post-reconnect) and subsequent TTS requests, in `pkg/telephony/session/call_session_test.go`

### Implementation for User Story 6

- [ ] T070 [US6] Confirm/finalize `ResolveVoiceConfig` (T005) English-default behavior and thread `VoiceConfig.Language` through `stt.Client` session-open params (T024) and `tts.Client.Synthesize` calls (T052) — most wiring already exists from Phases 3–6; this task is the explicit language-correctness pass plus the fallback-on-unsupported-language guard
- [ ] T071 [US6] Implement a `CallSession.UpdateLanguage(lang string) ` method in `pkg/telephony/session/call_session.go` that updates `VoiceConfig.Language` and is applied on the next STT reconnect (T037) and all subsequent `TTSBatcher` instantiations; document (in code comment) that the mid-call trigger mechanism itself is owned by Flows/platform and out of this repo's scope — this method is the consumption point only

**Checkpoint**: Multi-tenant language correctness is verified in isolation from the P1 conversational-loop stories.

---

## Phase 9: User Story 7 — Continuous Multi-Turn Conversation and Concurrency (Priority: P2)

**Goal**: Automatic turn-taking, full per-call isolation, and capacity-aware queueing with hold audio.

**Independent Test**: Per `spec.md` User Story 7 — run N concurrent mocked sessions at capacity, verify isolation, then start one more and verify queueing + hold audio + eventual admission.

### Tests for User Story 7

- [ ] T072 [P] [US7] Unit test: after `Flush(final: true)` drains, `CallSession.State` returns to `Listening` automatically within the target turn-around time, in `pkg/telephony/session/call_session_test.go` (extends T054's coverage with explicit timing assertion)
- [ ] T073 [P] [US7] Concurrency isolation test: N `CallSession`s driven with distinct STT/TTS mocks and distinct transcripts in parallel goroutines; assert no session's `Turn`/`VoiceConfig`/metrics leak into another's, in `pkg/telephony/session/manager_test.go`
- [ ] T074 [P] [US7] Race-detector test (`go test -race`) specifically targeting `SessionManager.Register`/`Attach`/`Remove` under concurrent load, in `pkg/telephony/session/manager_test.go`
- [ ] T075 [US7] Capacity queueing test: fill `SessionManager` to `MaxConcurrentCalls`, register one more, verify it is marked `Queued`, hold audio starts on attach, and it transitions to `Connecting`→normal setup once a slot is `Remove`d, in arrival order with a second queued caller, in `pkg/telephony/session/manager_test.go`
- [ ] T076 [US7] 10+ turn longevity test: drive one `CallSession` through 12 sequential mocked turns and assert no growth in per-turn processing time and no STT/TTS error-rate increase (simple linear-regression-free threshold check), in `pkg/telephony/session/call_session_test.go`

### Implementation for User Story 7

- [ ] T077 [US7] Confirm `CallSession`'s per-instance-only state (no package-level mutable state) across `session/`, `stt/`, `tts/` — audit pass, fix any accidental shared state found by T073/T074 (e.g., ensure `TTSBatcher` and `STTSession` instances are never reused across `CallSession`s)
- [ ] T078 [US7] Implement FIFO admission from the queue in `SessionManager.Remove` (T014/T015): on slot release, scan queued sessions by arrival timestamp and promote the earliest to normal setup (call `CallSession.setup()` per T025), in `pkg/telephony/session/manager.go`
- [ ] T079 [US7] Implement hold-audio looping playback in `CallSession` for `Queued` state (reads `WWC_TELEPHONY_HOLD_AUDIO_PATH`, loops fixed-size PCM frames over the AudioSocket connection until promoted), in `pkg/telephony/session/call_session.go`

**Checkpoint**: The gateway behaves correctly under concurrency and at capacity — production-readiness properties beyond the single-call MVP.

---

## Phase 10: User Story 8 — Efficient TTS Credit Usage (Priority: P2)

**Goal**: Explicit, standalone verification of the Product Spec's cost-efficiency success criterion (SC-005), since the mechanism itself is already built in Phase 6/7.

**Independent Test**: Per `spec.md` User Story 8 — feed a realistic 3-sentence response through the batcher and count TTS requests; verify barge-in mid-response issues zero further requests for the interrupted response.

### Tests for User Story 8

- [ ] T080 [P] [US8] Parameterized test across several realistic multi-sentence agent responses (short/long sentences, punctuation edge cases, mixed languages) asserting the TTS-request count stays within the "roughly one per sentence" budget, in `pkg/telephony/tts/batcher_test.go`
- [ ] T081 [US8] Test: triggering barge-in (via T060–T064) mid-response results in zero additional TTS requests for the discarded response, verified via a call-counting mock `tts.TTSStreamClient`, in `pkg/telephony/session/call_session_test.go`

### Implementation for User Story 8

> No new implementation — Phase 6 (`TTSBatcher`) and Phase 7 (`Discard()`) already provide the mechanism. This phase is test-only, per the repo's Delivery Workflow allowing a story to be "implementation already covered by [prior tasks]" (see `001-pdp-starters/tasks.md` for the established precedent in this repo).

**Checkpoint**: SC-005 (credit efficiency) is explicitly, independently verified and regression-protected.

---

## Phase 11: User Story 9 — Graceful Call Termination and Observability (Priority: P3)

**Goal**: Clean teardown on any termination path, with structured telemetry across the whole call lifecycle.

**Independent Test**: Per `spec.md` User Story 9 — end a session via a simulated hangup frame and via forced server-side termination; verify STT/TTS connections close, no leaks remain, and teardown metrics are emitted.

### Tests for User Story 9

- [ ] T082 [P] [US9] Unit test: `0x00` hangup frame triggers full teardown (STT closed, TTS in-flight cancelled, AudioSocket closed, slot released) in `pkg/telephony/session/call_session_test.go`
- [ ] T083 [P] [US9] Unit test: hangup while `Speaking` stops playback immediately with no orphaned goroutine (use `goleak`-style detection or explicit goroutine-count assertions) in `pkg/telephony/session/call_session_test.go`
- [ ] T084 [P] [US9] Unit test: forced server-side termination (`SessionManager.Remove` called directly, e.g. for shutdown) executes the identical teardown path as caller-initiated hangup, in `pkg/telephony/session/manager_test.go`
- [ ] T085 [US9] Test: teardown emits all required `SessionMetrics` (setup duration already covered by US1; assert STT-commit, agent-roundtrip, TTS-batch, barge-in [if applicable], and teardown-total metrics are present for a full sample call), in `pkg/telephony/session/call_session_test.go`

### Implementation for User Story 9

- [ ] T086 [US9] Implement `CallSession.Teardown(reason string)` in `pkg/telephony/session/call_session.go`: idempotent, closes `STTSession`, cancels any in-flight `TTSBatcher` synthesis, closes the `AudioSocketConn`, calls `DeregisterDelivery` (T042), calls `SessionManager.Remove` (releasing the concurrency slot and triggering T078's queue promotion), and records `telephony_call_teardown_total{reason}`
- [ ] T087 [US9] Wire the AudioSocket `0x00` hangup frame (parsed in T035) to call `CallSession.Teardown("caller_hangup")`
- [ ] T088 [US9] Wire a graceful-shutdown hook in `telephony/main.go` (SIGTERM handling) to call `Teardown("server_shutdown")` on every active `CallSession` before process exit
- [ ] T089 [US9] Add structured log fields (`session_id`, `channel_uuid`, `project_uuid`, `contact_urn`, `state`) to every log statement across `pkg/telephony/session/*.go` that doesn't already have them (audit pass), consistent with existing `pkg/grpc`/`pkg/websocket` logging conventions

**Checkpoint**: All nine user stories are complete and independently verified — the full Product Journey 1–9 slice owned by this repo is implemented.

---

## Phase 12: Polish & Cross-Cutting Concerns

**Purpose**: Final verification, documentation, and explicit hand-off of what remains outside this repo.

- [ ] T090 [P] Run `go vet ./... ` and `golangci-lint run` across all new `pkg/telephony/...` and `telephony/` code; fix findings
- [ ] T091 Run the full test suite (`go test ./...`) and the race detector (`go test -race ./...`) across the whole repo (not just new packages) to confirm no regression to `001-full-voice-mode`/other existing features
- [ ] T092 [P] Update the root `README.md` environment-variables table with the new `WWC_TELEPHONY_*` entries (mirrors the existing table format)
- [ ] T093 Execute `quickstart.md`'s manual smoke test end-to-end against a local Redis/Mongo and mocked externals; fix any gaps found
- [ ] T094 [P] Verify coverage on all new packages meets the repo's 80% line/branch guidance (Constitution IV) via `go test -cover ./pkg/telephony/...`
- [ ] T095 Open a tracked follow-up (issue/ticket, not a code task in this repo) for: (a) the Courier team to confirm `contracts/flows-pstn-integration.md`, (b) the infrastructure repo to add the `telephony` Docker build stage + K8s Deployment/Service, (c) the Asterisk/telephony deployment repo to implement the dialplan/AGI/ARI script against `contracts/audiosocket-session-protocol.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 — **BLOCKS all user stories**
- **User Stories (Phases 3–11)**: All depend on Phase 2 completion
  - **US1–US5 (Phases 3–7) are sequential in practice**, not just priority: US2 needs an established session from US1; US3 needs a committed transcript from US2; US4 needs delta delivery wired by US3; US5 needs the STT event stream from US2 and the TTS batching/playback loop from US4. Treat P1 stories as one connected MVP arc, built and demoed in this order, even though `spec.md` frames each as independently testable *against mocks*.
  - **US6, US7, US8 (Phases 8–10, P2)** can proceed in parallel with each other once the P1 arc (Phases 3–7) is complete — they layer onto the same `CallSession` without depending on each other.
  - **US9 (Phase 11, P3)** depends on `CallSession`/`SessionManager` existing (Phase 2) and is most meaningfully tested once US1–US5 exist, but its teardown implementation itself has no hard dependency on US6–US8.
- **Polish (Phase 12)**: Depends on all desired user stories being complete

### Within Each User Story

- Tests are written first and MUST fail before implementation (Constitution Principle IV)
- Interfaces/mocks (Phase 2) before real implementations (per story)
- Session/state changes before I/O wiring
- Story validated at its checkpoint before moving to the next

### Parallel Opportunities

- All `[P]`-marked Setup and Foundational tasks (T002, T004–T007, T009, T011–T013, T016) touch different files and can be split across engineers once their own small dependencies (e.g. T004 before T025) are respected
- Within each story's "Tests" subsection, all `[P]` tasks are parallelizable (different test files or independent test cases)
- **US6, US7, US8 can be staffed to three different engineers in parallel** once Phase 7 (US5) is checkpointed — they are the P2 layer described above
- T090/T092/T094 (Polish) are parallelizable; T091/T093/T095 are not (whole-repo test run, manual E2E pass, and follow-up filing are each single-threaded activities)

---

## Implementation Strategy

### MVP First (P1 arc only)

1. Phase 1: Setup
2. Phase 2: Foundational (blocking)
3. Phase 3 → 4 → 5 → 6 → 7 (US1 → US2 → US3 → US4 → US5), in order
4. **STOP and VALIDATE**: a single simulated call can be placed, transcribed, answered by a mocked agent, spoken back, and reliably interrupted — this is the demoable MVP matching the Product Spec's own P1 journeys
5. Deploy/demo behind the infra follow-up (T095) once available

### Incremental Delivery After MVP

1. Add US6 (language) → test → merge
2. Add US7 (concurrency/capacity) → test → merge
3. Add US8 (credit-efficiency regression tests) → test → merge
4. Add US9 (teardown/observability hardening) → test → merge
5. Phase 12 polish → final validation → hand off the three follow-ups in T095

### Parallel Team Strategy (once Phase 7 checkpoint is reached)

- Engineer A: US6 (Phase 8, language)
- Engineer B: US7 (Phase 9, concurrency/capacity)
- Engineer C: US8 (Phase 10, credit-efficiency tests) + starts US9 (Phase 11) test-writing early

---

## Notes

- Every task cites the exact file path per `plan.md`'s Project Structure — no task touches `pkg/websocket`'s core WebSocket handling, `pkg/grpc/server.go`, or `pkg/elevenlabs`'s existing token-issuance code, by design (protects `001-full-voice-mode`, BD-009).
- `[Story]` labels map 1:1 to `spec.md`'s User Story numbering, which itself maps to the governing Product Spec's Journey numbering — traceability runs Product Journey → Engineering User Story → Task.
- T041's `contact_urn` fallback and T008's `ResolvePSTNChannel` are the two tasks most likely to need rework once the real Courier contract (`contracts/flows-pstn-integration.md`) is confirmed — both are isolated behind interfaces specifically so that rework stays local.
- This backlog implements only `weni-webchat-socket`'s slice. It does not include Courier's new PSTN channel type, the Asterisk dialplan/AGI/ARI script, or any Nexus change — those require their own Engineering Specs in their own repositories, each pinning the same Product Spec commit (`7838a70eed496aa45a85f4d86e81ca2f4fb2dbc0`).
