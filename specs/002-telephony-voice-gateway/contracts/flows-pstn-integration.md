# Contract: Courier/Flows PSTN Integration

**Branch**: `002-telephony-voice-gateway` | **Date**: 2026-07-22 (updated 2026-08-06)
**Status**: §1 **IMPLEMENTED** in Courier (`handlers/telephony`) and gateway (`pkg/telephony/courier`). §2 **IMPLEMENTED** (fixed receive URL). §3 `contact_urn` echo-back still **PROPOSED**.

> Filename kept for link stability; channel resolution lives on **Courier**, not Flows.

## 1. Channel resolution — `GET /c/tph/resolve?did=<did>` (Courier)

Courier owns the DID→channel mapping (same `GetChannelByAddress` lookup as `/c/tph/receive`).

**Request**:

```
GET /c/tph/resolve?did=<E.164>
Authorization: Bearer <token>   # optional; required when COURIER_TELEPHONY_RESOLVE_TOKEN is set
```

**Response (200)**:

```json
{
  "channel_uuid": "8adf206a-607b-4039-9cac-3de66d084f15",
  "project_uuid": "1a2b3c4d-....-....-....-............"
}
```

**Not found**: HTTP 400 with `"channel not found"` in body (gateway treats as empty result, same as 404).

**Gateway client** (`pkg/telephony/courier/client.go`):

```go
ResolveChannel(did string) (channelUUID, projectUUID string, err error)
```

**Env**:

| Service | Variable |
|---|---|
| Courier | `COURIER_TELEPHONY_RESOLVE_TOKEN` (optional auth) |
| Gateway | `WWC_COURIER_URL`, `WWC_TELEPHONY_COURIER_RESOLVE_TOKEN` |

**Why not Flows `pstn_channel`?** Flows internal endpoint requires JWT with `channel_uuid` or `project_uuid` before lookup — chicken-and-egg at registration time. Courier resolves by `address` without that constraint.

**Voice config after resolve** (still Flows, keyed by `channel_uuid`):

- `GetElevenLabsAPIKey(channelUUID)`
- `GetChannelProjectLanguage(channelUUID)`

## 2. Inbound transcript — `POST {WWC_COURIER_URL}/c/tph/receive`

Gateway posts committed STT transcripts to a **fixed** Courier receive URL (`WWC_COURIER_URL` + `/c/tph/receive`), not a per-channel callback URL from resolve.

Payload shape (Courier TPH handler):

```json
{
  "type": "message",
  "origin": "pstn",
  "did": "+15551234567",
  "caller_id": "+15559876543",
  "call_id": "<session_id>",
  "message": {
    "type": "text",
    "timestamp": "1753203600",
    "text": "I'd like to check my order status"
  }
}
```

- `caller_id` is the **raw caller identity** — Courier constructs the `tel:` URN (Product BD-010/FR-038).
- `did` identifies the PSTN channel (same lookup key as resolve/receive).

## 3. Contact URN echo-back (needed for gRPC delivery registration)

The gateway needs the `tel:` URN Courier constructed, so it can register the `CallSession` in `ClientManager`/`Router` under the *same* key Nexus will use in `contact_urn` when streaming deltas back.

**Proposed**: the `POST /c/tph/receive` response (200) includes:

```json
{
  "message": "Message Accepted",
  "data": [{ "type": "msg", "urn": "tel:+15559876543" }]
}
```

Until confirmed, the gateway falls back to locally constructing `tel:<raw caller id>` (documented risk in `plan.md` Complexity Tracking).

**Registration key**: `CallSession.RegistrationKey()` strips the scheme prefix before `AddConnectedClient` — must match `pkg/grpc/server.go` `normalizeContactURN`. See `contracts/grpc-telephony-delivery.md` §1.

## 4. Outbound agent responses — gRPC only (NOT Courier `SendMsg`)

Agent replies reach the gateway exclusively via the **existing gRPC pipeline**:

```
Nexus → grpc binary → Redis Streams → telephony Router → CallSession.handleGRPCPayload → TTS
```

The gateway does **not** expose `POST /send` and does **not** accept Courier `SendMsg` HTTP outbound. Courier TPH `SendMsg` (`base_url/send`) is out of scope for this gateway; streaming voice uses gRPC deltas, not complete-text Courier push.

See `contracts/grpc-telephony-delivery.md`.

## 5. Language & ElevenLabs (Flows)

No new endpoints — reuses existing `flows.IClient`:

- `GetChannelProjectLanguage(channelUUID)` — called after §1 resolve
- `GetElevenLabsAPIKey(channelUUID)` — called after §1 resolve
