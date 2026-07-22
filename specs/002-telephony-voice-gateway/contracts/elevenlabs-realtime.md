# Contract: Gateway ↔ ElevenLabs Realtime STT/TTS

**Branch**: `002-telephony-voice-gateway` | **Date**: 2026-07-22

New gateway-side clients (see `research.md` §6) — separate from the existing token-issuance `pkg/elevenlabs/client.go` used by `001-full-voice-mode`.

## STT: Scribe v2 Realtime (WebSocket)

**Auth**: tenant's full ElevenLabs API key (via `flows.IClient.GetElevenLabsAPIKey`), sent as a connection header — never a single-use token (there is no browser to protect here; the gateway is the trusted server-side actor, per Product BD-002).

**Session parameters**: `commit_strategy=vad`, audio format `pcm_16000`, language from `VoiceConfig.Language`, VAD silence threshold from `VoiceConfig.VADSilenceMs`.

**Outbound (gateway → ElevenLabs)**: `input_audio_chunk` messages, Base64-encoded PCM 16 kHz mono, `commit=false` (server-side VAD commits, not the client).

**Inbound (ElevenLabs → gateway)**:
- `partial_transcript` — text so far in the current utterance. Consumed two ways: (a) internal turn-tracking (Story 2), (b) barge-in trigger when `CallSession.State == Speaking` (Story 5, `research.md` §4).
- `committed_transcript` — final text for the utterance; becomes exactly one `Turn` (FR-009).
- Any other/unrecognized message type is logged and ignored (defensive parsing, Constitution Principle II).

**Reconnect** (FR-011): on unexpected close, the STT client wrapper (`pkg/telephony/stt`, implementing an `STTSession` interface for testability) automatically redials with the same `VoiceConfig`, preserving the `CallSession`'s current `Turn` state (no data loss for audio already committed; audio in flight during the gap is dropped and implicitly re-captured once the caller keeps speaking — acceptable per Product Edge Cases, which require *no call termination*, not zero audio loss during a transient drop).

## TTS: streaming synthesis (WebSocket)

**Auth**: same tenant API key.

**Model**: `eleven_flash_v2_5`. **Output format**: `pcm_8000` (matches Asterisk directly — no resampling on the return path, per FR-019).

**Outbound (gateway → ElevenLabs)**: one request per batch produced by `TTSBatcher` (`data-model.md`), carrying the batch text, `VoiceConfig.VoiceID`, and `VoiceConfig.Language`.

**Inbound (ElevenLabs → gateway)**: streamed audio chunks for the batch; forwarded to the AudioSocket write loop as `0x10` frames as they arrive (FR-017) — the gateway does not wait for a batch's full audio before starting playback of that batch, though it does sequence batches so batch *N+1* never starts playback before batch *N* finishes (FR-018, no perceptible gap, SC-003).

**Cancellation** (barge-in, FR-026): the in-flight TTS WebSocket request for the interrupted batch is closed/cancelled via the batcher's `context.CancelFunc`; any audio chunks already received but not yet written to the AudioSocket connection are discarded, and the write loop stops immediately.

**Failure handling** (FR-021): a batch-level error (non-200/close-with-error from ElevenLabs) is caught by the `TTSBatcher`, logged, and skipped — the next batch (if any) proceeds normally; if it was the last/only batch, the `CallSession` returns to `Listening`.
