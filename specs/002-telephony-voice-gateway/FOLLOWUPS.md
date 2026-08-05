# External Follow-ups: Telephony Voice Gateway

Tracked coordination items outside `weni-webchat-socket` (T095). Open equivalent tickets in the owning repositories before production deployment.

## 1. Courier / Flows — PSTN channel contract

**Owner**: Courier team  
**Reference**: [`contracts/flows-pstn-integration.md`](./contracts/flows-pstn-integration.md)

Confirm and implement:

- `GET /api/v2/internals/pstn_channel?did=<did>` (DID → channel/project/callback URL)
- Callback POST payload shape and `contact_urn` echo-back semantics after transcript delivery

**Risk if delayed**: `ResolvePSTNChannel` and `PostTranscript` remain behind mocks; production routing and gRPC delivery keys may mismatch until the contract is finalized.

## 2. Infrastructure — telephony binary deployment

**Owner**: Infrastructure repository  
**Reference**: [`quickstart.md`](./quickstart.md), [`plan.md`](./plan.md) Complexity Tracking

Add:

- Docker build stage/target for `telephony/main.go`
- Kubernetes Deployment + Service (HTTP registration + AudioSocket TCP), independently scalable from `api`/`grpc`
- `WWC_TELEPHONY_*` environment variables in the deployment manifest

## 3. Asterisk / telephony deployment — dialplan integration

**Owner**: Asterisk/telephony deployment repository  
**Reference**: [`contracts/audiosocket-session-protocol.md`](./contracts/audiosocket-session-protocol.md)

Implement the call flow:

1. `POST /telephony/sessions` with `did`, `caller_id`, `origin`
2. AudioSocket TCP connect to returned `audiosocket_addr` with UUID frame (`0x01`)
3. Bidirectional PCM 8 kHz audio streaming and hangup handling (`0x00`)

---

**Status**: Documented 2026-08-05 as part of Phase 12 (Polish) — replace this file with links to real issue/ticket IDs once filed in each team's tracker.
