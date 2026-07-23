# Contract (proposed): Flows/Courier PSTN Integration

**Branch**: `002-telephony-voice-gateway` | **Date**: 2026-07-22
**Status**: PROPOSED — to be confirmed with the Courier team; implemented behind `flows.IClient` so gateway work can proceed against a mock in the meantime (see `research.md` §3).

## 1. `GET /api/v2/internals/pstn_channel?did=<did>`

Mirrors the existing pattern of `GET /api/v2/internals/elevenlabs_api_key?channel=<uuid>` and `GET /api/v2/projects/project_language?channel_uuid=<uuid>` already in `pkg/flows/client.go`.

**Response (200)**:

```json
{
  "channel_uuid": "8adf206a-607b-4039-9cac-3de66d084f15",
  "project_uuid": "1a2b3c4d-....-....-....-............",
  "callback_url": "https://flows.weni.ai/c/pstn/8adf206a-.../receive"
}
```

**Response (404)**: `did` is not configured on any Courier PSTN channel instance — mirrors the existing `GetElevenLabsAPIKey` 404-as-empty-result pattern rather than a hard error, so the gateway can distinguish "not configured" (FR-005: reject registration) from a transient failure (retry).

**Client-side interface** (added to `pkg/flows/client.go`, `IClient`):

```go
ResolvePSTNChannel(did string) (channelUUID, projectUUID, callbackURL string, err error)
```

## 2. Outbound message delivery — `POST <callback_url>`

Reuses the existing outbound-message shape already sent by WebSocket clients via `ToCallback` (`pkg/websocket/client.go`), so Courier's receiver side needs no new payload parser beyond recognizing the new `origin` metadata:

```json
{
  "type": "message",
  "from": "+15559876543",
  "message": {
    "type": "text",
    "timestamp": "1753203600",
    "text": "I'd like to check my order status"
  },
  "origin": "pstn",
  "did": "+15551234567"
}
```

- `from` carries the **raw caller identity**, not a constructed URN — Courier owns URN construction (`tel:+15559876543`) per Product BD-010/FR-038, exactly as it already normalizes raw WhatsApp phone numbers into `whatsapp:` URNs today.
- `origin` and `did` are additive metadata Courier's new PSTN channel handler uses to select the PSTN contact/channel path instead of an existing one; they do not change the shape other channels already send.

## 3. Contact URN echo-back (needed for gRPC delivery registration)

The gateway needs to know the `tel:` URN Courier constructed, so it can register the `CallSession` in `ClientManager`/`Router` under the *same* key Nexus will use in `contact_urn` when streaming deltas back (`pkg/grpc/proto` `StreamMessage.contact_urn`).

**Proposed**: the `POST <callback_url>` response (200) includes:

```json
{ "contact_urn": "tel:+15559876543" }
```

If Courier's existing callback contract does not currently return a body, this is the one true new requirement on the Courier side introduced by this feature — everything else reuses existing patterns. Until confirmed, the gateway falls back to locally constructing `tel:<raw caller id>` as a working assumption for registration purposes (documented as a risk in `plan.md` Complexity Tracking), since it must match whatever Nexus is told to use as `contact_urn` — that alignment is itself part of the joint contract to confirm.

**Registration key derivation (important, independent of the above)**: whichever full `tel:`-prefixed value ends up in `CallSession.ContactURN` (echoed by Courier or locally constructed as a fallback), the gateway MUST NOT register that full string with `ClientManager.AddConnectedClient`. `pkg/grpc/server.go` strips the scheme prefix (`normalizeContactURN`) from the `contact_urn` Nexus sends before every `ClientManager`/`Router` lookup, and existing WebSocket clients already register bare (no scheme prefix). So the gateway must apply the same stripping rule (`CallSession.RegistrationKey()`, see `data-model.md`) before calling `AddConnectedClient`/`RemoveConnectedClient` — otherwise gRPC delta delivery to the call silently never arrives, regardless of which of the two `contact_urn` sourcing options above is used. See `contracts/grpc-telephony-delivery.md` §1 and `research.md` §5.

## 4. Language

No new endpoint — reuses the existing `GetChannelProjectLanguage(channelUUID)` already in `flows.IClient`, called with the `channel_uuid` resolved in §1.

## 5. ElevenLabs API key

No new endpoint — reuses the existing `GetElevenLabsAPIKey(channelUUID)` already in `flows.IClient`.
