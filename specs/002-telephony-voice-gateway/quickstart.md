# Quickstart: Telephony Voice Gateway

**Branch**: `002-telephony-voice-gateway` | **Spec**: [spec.md](./spec.md)

## Run locally

```sh
# existing services (unchanged)
go run ./api    # WebSocket + HTTP (web voice mode, other channels)
go run ./grpc   # gRPC ingress from Nexus

# new for this feature
go run ./telephony
```

## New environment variables (proposed — finalized in plan.md Project Structure)

| Variable | Default | Purpose |
|---|---|---|
| `WWC_TELEPHONY_HTTP_PORT` | `8081` | Session-registration HTTP endpoint (`POST /telephony/sessions`) |
| `WWC_TELEPHONY_AUDIOSOCKET_PORT` | `9095` | AudioSocket TCP listener |
| `WWC_TELEPHONY_MAX_CONCURRENT_CALLS` | `100` | Capacity cap (FR-033) |
| `WWC_TELEPHONY_VAD_SILENCE_MS` | `1500` | Default STT VAD silence threshold, aligned with `001-full-voice-mode` |
| `WWC_TELEPHONY_TTS_MIN_BATCH_CHARS` | `40` | Minimum batch size when no sentence boundary is hit |
| `WWC_TELEPHONY_STT_MODEL_ID` | `scribe_v2_realtime` | ElevenLabs STT model |
| `WWC_TELEPHONY_TTS_MODEL_ID` | `eleven_flash_v2_5` | ElevenLabs TTS model |
| `WWC_TELEPHONY_HOLD_AUDIO_PATH` | `assets/hold_music_8k.raw` | Pre-recorded hold audio for capacity queueing |
| `WWC_COURIER_URL` | (see `config.go`) | Base URL for Courier TPH resolve + receive |
| `WWC_TELEPHONY_COURIER_RESOLVE_TOKEN` | (none) | Optional Bearer token for `GET /c/tph/resolve` |
| `WWC_TELEPHONY_GREETING_TEXT_KEY` | `voice.greeting` | Localized greeting prompt key (content per VTEX Content Guide) |

## Manual end-to-end smoke test (no real Asterisk needed)

1. Start `go run ./telephony` against a local Redis/Mongo (same `docker-compose` used for `api`/`grpc` today).
2. Use a lightweight in-process AudioSocket test client (assembled from the interfaces/mocks already defined in Phase 2 — `AudioSocketConn` (T010), the `flows.IClient` mock (T009), `stt.STTSession`/`tts.TTSStreamClient` mocks (T012–T013); no dedicated task creates a separate `testclient` package, this is exercised end-to-end by `tasks.md` T093) to:
   a. `POST http://localhost:8081/telephony/sessions` with a DID mocked to resolve via a stubbed `courier.IClient` (`GET /c/tph/resolve`).
   b. Open a raw TCP connection to `localhost:9095`, send the `0x01` UUID frame with the returned `session_id`.
   c. Stream a short WAV (converted to 8 kHz PCM frames) simulating caller speech.
   d. Observe (mocked ElevenLabs STT) a `committed_transcript`, then (mocked Courier receive + mocked gRPC delta stream) TTS audio frames written back.
3. Verify via `pkg/metric` test hooks that setup/STT/TTS/teardown metrics were recorded (NFR-005).

## Deployment note (Constitution Principle VI)

This feature ships as a new container entrypoint (`telephony/main.go` → its own Docker image target/stage) and a new Kubernetes Deployment + Service, independently scalable from `api`/`grpc`. Coordinate with the infrastructure repository before merge — flagged in `plan.md` Complexity Tracking as required follow-up, not part of this repo's own task list.
