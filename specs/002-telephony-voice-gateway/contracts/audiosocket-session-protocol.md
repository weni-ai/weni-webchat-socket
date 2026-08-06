# Contract: Session Registration + AudioSocket Protocol (Asterisk ↔ Gateway)

**Branch**: `002-telephony-voice-gateway` | **Date**: 2026-07-22

This is the contract the (currently `[NOT YET DEFINED]`) Asterisk/telephony deployment repository's dialplan/AGI/ARI script MUST follow to use this gateway. It has two legs: an HTTP registration call, then a standard AudioSocket TCP connection.

## 1. HTTP: `POST /telephony/sessions`

**Request**:

```json
{
  "did": "+15551234567",
  "caller_id": "+15559876543",
  "origin": "pstn"
}
```

- `did` (string, required): the dialed number, from the SIP `To`/Request-URI, exactly as Courier has it configured on the PSTN channel.
- `caller_id` (string, optional): the SIP `From` number; omitted/empty for withheld numbers (handled as anonymous, per Product Edge Cases).
- `origin` (string, required): `"pstn"` for this delivery; the field exists so a future `"whatsapp_voice"` origin can reuse this same endpoint without a contract change (Product Spec Assumptions & Dependencies — forward compatibility, not implemented here).

**Response (success, 200)**:

```json
{
  "session_id": "b3f2e1d0-....-....-....-............",
  "audiosocket_addr": "gateway-telephony.internal:9095"
}
```

**Response (error)**:

| Status | Condition |
|---|---|
| 400 | Missing/invalid `did` or `origin` |
| 404 | `did` does not resolve to any Courier PSTN channel (FR-005) |
| 503 | Channel resolution succeeded but ElevenLabs/STT dependency is known-down (fail fast before Asterisk answers, where the caller hasn't heard a greeting yet — implements Product FR-005's "graceful, not silent" at the earliest possible point) |

**Timeout**: caller (Asterisk-side script) SHOULD apply a 3-second timeout consistent with Product SC-001's setup budget.

## 2. AudioSocket TCP connection

Standard Asterisk AudioSocket wire format (see `research.md` §1). The dialplan MUST invoke `AudioSocket(<audiosocket_addr>,<session_id>)` using the `session_id` returned in step 1 as the UUID argument.

| Frame type | Byte | Payload | Direction | Notes |
|---|---|---|---|---|
| Hangup | `0x00` | none | Asterisk → gateway | Triggers teardown (FR-035) |
| UUID | `0x01` | 16 bytes (the `session_id` from step 1, parsed as raw UUID bytes) | Asterisk → gateway | First frame; gateway attaches the connection to the pending `CallSession` |
| DTMF | `0x03` | 1 byte (ASCII digit) | Asterisk → gateway | Recognized-and-ignored — DTMF/IVR handling is out of scope (Product Spec Out of Scope); logged at debug level only, never treated as an error |
| Audio | `0x10` | N bytes, signed linear PCM 8 kHz mono, 20 ms frames (320 bytes) | both directions | Continuous while call is active |
| Error | `0xFF` | 1 byte, optional Asterisk error code (`0x01` caller hangup, `0x02` frame-forwarding error, `0x04` memory-allocation error) | Asterisk → gateway | Logged; gateway treats as non-fatal unless repeated |

**Correction (2026-07-23)**: an earlier version of this table mislabeled `0x03` as "Error"; per Asterisk's `ast_audiosocket_msg_kind` enum, `0x03` is DTMF and `0xFF` is the actual error type. See `research.md` §1 for the full rationale.

**Session-ready signal**: after the gateway successfully opens the ElevenLabs STT session (Story 1, Scenario 3), it MUST send at least one `0x10` audio frame (the greeting, synthesized via the normal TTS batching path using a fixed greeting prompt) within the SC-001 budget. There is no separate "ready" control frame in standard AudioSocket — readiness is observed by Asterisk as "the gateway started sending audio," which is why channel/STT resolution MUST complete before any audio is sent (FR-002, FR-004 ordering).

## 3. Capacity / queueing (FR-033)

When the gateway is at its configured concurrent-call capacity, `POST /telephony/sessions` still returns `200` with a `session_id`, but the dialplan is expected to proceed to `AudioSocket()` immediately; the gateway plays hold audio over that same AudioSocket connection until a slot frees, then proceeds with normal setup. This keeps the contract from needing a distinct "queued" HTTP status — from Asterisk's perspective the call is answered and connected either way, satisfying "no caller is dropped or met with a silent line" (Product SC-011) without new dialplan branching logic.
