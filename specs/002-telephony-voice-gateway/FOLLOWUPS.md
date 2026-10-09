# External Follow-ups: Telephony Voice Gateway

Tracked coordination items outside `weni-webchat-socket` (T095). Open equivalent tickets in the owning repositories before production deployment.

## 1. Courier — contact URN echo-back

**Owner**: Courier team  
**Reference**: [`contracts/flows-pstn-integration.md`](./contracts/flows-pstn-integration.md) §3

**Done (2026-08-06)**:

- `GET /c/tph/resolve?did=<did>` — DID → `{channel_uuid, project_uuid}` (implemented in Courier; consumed by `pkg/telephony/courier`)
- Inbound receive at `POST /c/tph/receive` — gateway posts committed transcripts

**Still open**:

- Confirm and implement `contact_urn` echo-back in the `POST /c/tph/receive` response so gRPC delivery registration keys match Courier's normalized URN

**Risk if delayed**: Gateway falls back to locally constructed `tel:<caller_id>`; gRPC delivery lookups may miss if Courier normalizes differently.

## 2. Infrastructure — telephony binary deployment

**Owner**: Infrastructure repository  
**Reference**: [`quickstart.md`](./quickstart.md), [`plan.md`](./plan.md) Complexity Tracking

Add:

- Docker build stage/target for `telephony/main.go`
- Kubernetes Deployment + Service (HTTP registration + AudioSocket TCP), independently scalable from `api`/`grpc`
- `WWC_TELEPHONY_*`, `WWC_COURIER_URL`, `WWC_TELEPHONY_COURIER_RESOLVE_TOKEN` in the deployment manifest

## 3. Asterisk / telephony deployment — dialplan integration

**Owner**: Asterisk/telephony deployment repository  
**Reference**: [`contracts/audiosocket-session-protocol.md`](./contracts/audiosocket-session-protocol.md)

Implement the call flow:

1. `POST /telephony/sessions` with `did`, `caller_id`, `origin`
2. AudioSocket TCP connect to returned `audiosocket_addr` with UUID frame (`0x01`)
3. Bidirectional PCM 8 kHz audio streaming and hangup handling (`0x00`)

---

**Status**: Documented 2026-08-05 as part of Phase 12 (Polish) — replace this file with links to real issue/ticket IDs once filed in each team's tracker.
