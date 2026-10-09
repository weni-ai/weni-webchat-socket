# Phase 1 Data Model: Telephony Voice Gateway

**Branch**: `002-telephony-voice-gateway` | **Date**: 2026-07-22 | **Spec**: [spec.md](./spec.md)

All entities are in-memory/Go-native (no new persistent datastore — Constitution III/VI: no new Redis/MongoDB usage beyond what already exists). `CallSession` registration with `ClientManager` reuses the existing Redis-backed `ws:clients` hash (see `research.md` §5); nothing new is added to Redis/Mongo schemas.

## CallSession

Package: `pkg/telephony/session`

```go
type State string

const (
    StateConnecting State = "connecting"
    StateListening  State = "listening"
    StateProcessing State = "processing"
    StateSpeaking   State = "speaking"
    StateError      State = "error"
    StateEnded      State = "ended"
)

type CallSession struct {
    ID          string // session UUID, also the AudioSocket UUID
    DID         string
    CallerID    string // raw caller identity as received from Asterisk; may be empty (withheld)
    Origin      string // e.g. "pstn"; forward-compatible with a future "whatsapp_voice"

    ChannelUUID  string
    ProjectUUID  string
    ContactURN   string // "tel:<callerID>" once known; full form, kept for logs/traceability
    Language     string // resolved once at setup; "en" default

    State       State
    StateMu     sync.RWMutex

    Conn        AudioSocketConn      // abstraction over the AudioSocket TCP connection (testable)
    STT         STTSession           // abstraction over the ElevenLabs STT WebSocket (testable)
    TTSBatcher  *TTSBatcher          // Story 4/8 batching state, see below
    BargeIn     *BargeInController   // Story 5 state

    CreatedAt   time.Time
    Metrics     *SessionMetrics      // setup/STT/agent/TTS/teardown timing, Story 9
}
```

**Validation / invariants**:
- `RegistrationKey()` — derived, not stored: `stripScheme(ContactURN)`, i.e. `ContactURN` with everything up to and including the first `:` removed (mirrors `pkg/grpc/server.go`'s `normalizeContactURN`). This — **not** the full `ContactURN` — is the literal string passed to `ClientManager.AddConnectedClient`/`RemoveConnectedClient` and used as the `Router.PublishToClient` target, because `pkg/grpc/server.go` normalizes every inbound `contact_urn` the same way before every lookup, and existing WebSocket clients already register under the bare form (`c.ID = payload.From`). Registering under the full `tel:`-prefixed `ContactURN` would make every gRPC delivery lookup miss (see `research.md` §5).
- `ID` is unique across all pods (UUID v4); collisions are treated as a registration error (fail fast, no silent overwrite).
- State transitions are only ever driven by one internal state machine goroutine per `CallSession` (no external mutation of `State` without going through it) — prevents races between the AudioSocket read loop, the STT event loop, and the gRPC-delivered delta handler.
- `ContactURN` is empty until the first successful channel resolution response; no audio is accepted (FR-002) before it is set.
- A `CallSession` is removed from the in-process registry and from `ClientManager` (Redis) as the *last* step of teardown (FR-034), never before all sub-resources (STT, TTS, AudioSocket) are closed.

**Relationships**: one `CallSession` ↔ one AudioSocket TCP connection ↔ one active STT WebSocket connection (replaced, not duplicated, on reconnect per FR-011) ↔ zero-or-one in-flight TTS WebSocket connection at a time ↔ one `ClientManager`/`Router` registration keyed by `RegistrationKey()` (the bare, scheme-stripped form of `ContactURN` — see Validation/invariants above).

## VoiceConfig

Package: `pkg/telephony/session` (value object, not a separate store)

```go
type VoiceConfig struct {
    ElevenLabsAPIKey string // resolved via flows.IClient.GetElevenLabsAPIKey(channelUUID)
    VoiceID          string
    Language         string
    STTModelID       string // e.g. "scribe_v2_realtime"
    TTSModelID       string // e.g. "eleven_flash_v2_5"
    VADSilenceMs     int64  // default ~1500ms, aligned with 001-full-voice-mode
    TTSMinBatchChars int64  // minimum characters before flushing a non-sentence-boundary batch
    MaxConcurrency   int64  // global cap enforced by the session manager (FR-033)
}
```

Resolved once per `CallSession` at setup from a combination of: (a) config defaults (`config.Configuration`, new `Telephony` struct — see `contracts/` and `plan.md`), and (b) per-channel overrides fetched via `flows.IClient` (`GetElevenLabsAPIKey`, `GetChannelProjectLanguage`). Channel/tenant identity comes from `courier.IClient.ResolveChannel(did)` at registration.

## Turn

Package: `pkg/telephony/session` (transient, not persisted by this repo)

```go
type Turn struct {
    MsgID           string    // correlates STT commit -> outbound POST -> gRPC delta stream -> TTS batches
    CommittedText   string
    StartedAt       time.Time
    DeltaBuffer     strings.Builder // accumulates until sentence boundary / min-threshold (FR-016)
    BatchesIssued   int             // for SC-005 observability
    Interrupted     bool            // set true on barge-in; discarded rather than flushed
}
```

A `CallSession` holds at most one *active* `Turn` awaiting an agent response at a time (Product FR-031: continuous single-line conversation, no manual multiplexing). A new committed transcript while a `Turn` is still `Processing`/`Speaking` never happens under normal operation (the session is `Listening` only after the previous `Turn` fully resolves) **except** as the direct result of barge-in, which explicitly starts a *new* `Turn` and marks the old one `Interrupted` (Story 5, Scenario 3).

## VoiceError

Package: `pkg/telephony/session`

```go
type ErrorCode string

const (
    ErrSTTUnavailable      ErrorCode = "stt_unavailable"
    ErrChannelUnresolved   ErrorCode = "channel_unresolved"
    ErrAgentUnavailable    ErrorCode = "agent_unavailable"
    ErrTTSBatchFailed      ErrorCode = "tts_batch_failed"
    ErrMediaError          ErrorCode = "media_error"
)

type VoiceError struct {
    Code        ErrorCode
    Message     string // internal, for logs
    SpokenKey   string // key into localized spoken-fallback message table
    Recoverable bool   // true => degrade in place (e.g. skip a TTS batch); false => teardown (FR-006)
}
```

`Recoverable == false` errors drive the `CallSession` to `StateError` → teardown with a spoken-fallback signal (Story 1 Scenarios 5–6). `Recoverable == true` errors (e.g. a single TTS batch failure) are handled inline by the component that raised them (Story 4 Scenario 5) without a state transition.

## TTSBatcher (implementation detail supporting FR-016–FR-023, Story 4/8)

Package: `pkg/telephony/tts`

```go
type TTSBatcher struct {
    buffer      strings.Builder
    minChars    int64
    voiceID     string
    language    string
    client      TTSStreamClient // interface, ElevenLabs streaming TTS
    out         chan AudioFrame // consumed by the AudioSocket write loop
    turnID      string
    cancel      context.CancelFunc // cancels any in-flight TTS request; used by barge-in
}
```

Exposes `Append(delta string)`, `Flush(final bool)`, and `Discard()` (barge-in). Sentence-boundary detection is a simple punctuation-based heuristic (`.`, `!`, `?`, or the configured `TTSMinBatchChars` fallback) — no NLP dependency, consistent with Constitution's "dependencies MUST be minimal, justified."

## BargeInController (implementation detail supporting FR-024–FR-027, Story 5)

Package: `pkg/telephony/session`

```go
type BargeInController struct {
    armed  atomic.Bool // true only while CallSession.State == Speaking
    onTrigger func()   // wired to: stop TTS write loop, TTSBatcher.Discard(), transition state, start new Turn
}
```

Fed directly by the STT session's `partial_transcript` event stream (research.md §4) — no separate audio tap or VAD buffer is needed; `armed` simply gates whether a partial transcript is interpreted as a trigger or as normal in-progress recognition.
