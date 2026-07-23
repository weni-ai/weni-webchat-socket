# Contract: gRPC Delivery to Telephony Sessions (No Wire Changes)

**Branch**: `002-telephony-voice-gateway` | **Date**: 2026-07-22

This documents the (unchanged) existing `pkg/grpc/proto/message_stream.proto` contract and specifies exactly how a `CallSession` participates in it, per the decision in `research.md` §5. There is **no new proto**, **no new RPC**, and **no change to `pkg/grpc/server.go`**.

## Existing contract (reference only, already implemented)

`MessageStreamService.StreamMessages` / `SendMessage` — Nexus (via Flows) sends `StreamMessage{type: "setup"|"delta"|"completed"|"control", msg_id, content, channel_uuid, contact_urn, ...}`. The server resolves `contact_urn` via `ClientManager.GetConnectedClient` and publishes JSON via `Router.PublishToClient(ctx, contact_urn, payloadJSON)`.

## What this feature adds (application-level, not wire-level)

1. **Registration**: on reaching `Listening` for the first time (i.e., once `ContactURN` is known per `data-model.md`), a `CallSession` calls the same `ClientManager.AddConnectedClient(ConnectedClient{ID: registrationKey, Channel: channelUUID, PodID: podID})` a WebSocket client calls on `register`. **`registrationKey` MUST be the bare, scheme-stripped identifier (`CallSession.RegistrationKey()`, i.e. `ContactURN` with any `scheme:` prefix removed), never the full `tel:`-prefixed `ContactURN`.** This is not a stylistic choice: `pkg/grpc/server.go`'s `normalizeContactURN` strips the scheme prefix from the inbound `contact_urn` before *every* `ClientManager.GetConnectedClient`/`Router.PublishToClient` call (`"ext:217138695938@" -> "217138695938@"`), and existing WebSocket clients already register under that same bare form (`c.ID = payload.From` in `pkg/websocket/client.go` — never scheme-prefixed). Registering under `"tel:+1555..."` would make every lookup below miss silently.
2. **Local delivery table**: today, `Router`'s `DeliverFunc` writes directly to a WebSocket connection found in the pod's `ClientPool`. This feature adds a second local lookup: if the (already-normalized) `contactURN` has an active `CallSession` on this pod (checked first, since a given pod only ever hosts telephony sessions it accepted an AudioSocket connection for), the `Router`'s configured `DeliverFunc` for telephony dispatches to that `CallSession`'s handler instead of a `ClientPool` lookup. Both destinations key off the exact same bare-identifier string space (post-`normalizeContactURN`), so no protocol-level disambiguation is needed at the `Router`/Redis layer — only the pod-local `DeliverFunc` implementation needs an `if` branch (registered once at `telephony/main.go` startup, wired into the same `App`/`Router` instance the `grpc` and `api` processes share via Redis).
3. **Payload parsing**: the `CallSession`'s delivery handler parses the exact same JSON shapes already defined in `pkg/websocket` (`StreamStartPayload`, `StreamDeltaPayload{V, Seq}`, `StreamEndPayload{Type, ID, Content}`) — no new payload types.
   - `stream_start` → begin a new `Turn` (or confirm the existing one), reset the `TTSBatcher`.
   - `delta` (`StreamDeltaPayload.V`) → `TTSBatcher.Append(V)`; sequence numbers (`Seq`) are used only to detect and drop out-of-order/duplicate delivery, mirroring how a WebSocket client would.
   - `stream_end` (`StreamEndPayload.Content`) → `TTSBatcher.Flush(final: true)`; if the content is the final full text, it is **not** re-sent to Flows/Courier for history — persistence of the *inbound* transcript already happened via the callback POST (`flows-pstn-integration.md` §2), and persistence of the *outbound* agent message is Flows/Courier's existing responsibility once it receives the message through its normal pipeline (Nexus → Flows already knows the full conversation; this repo does not duplicate that write).
4. **Deregistration**: on teardown, `ClientManager.RemoveConnectedClient(registrationKey)` (same bare form as registration) — same call a disconnecting WebSocket client triggers today.

## Non-goals

- No change to `StreamMessage`/`StreamResponse`/`SetupRequest`/`SetupResponse` proto messages.
- No change to sequence-tracking logic in `pkg/grpc/server.go` (`unarySeqTracker`, `seqTracker`) — a `CallSession` is just another consumer of the same ordered delta stream.
- No new gRPC service or method.
