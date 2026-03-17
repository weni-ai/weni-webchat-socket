# Quickstart: PDP Conversation Starters

**Branch**: `001-pdp-starters` | **Date**: 2026-03-08

## Prerequisites

- Go 1.24+
- Redis running (for client manager / streams)
- MongoDB running (for history)
- AWS credentials configured (IAM role or env vars with `lambda:InvokeFunction` permission)

## New Environment Variables

| Variable | Required | Default | Description |
| -------- | -------- | ------- | ----------- |
| `WWC_LAMBDA_STARTERS_ARN` | No | `""` | Lambda function ARN. Empty = feature disabled (silent ignore). |
| `WWC_LAMBDA_STARTERS_MAX_CONCURRENT` | No | `50` | Max concurrent Lambda invocations per pod. |
| `WWC_LAMBDA_STARTERS_REGION` | No | `""` | AWS region for Lambda. Falls back to `WWC_S3_REGION`. |
| `WWC_LAMBDA_STARTERS_TIMEOUT_SEC` | No | `35` | Timeout in seconds for Lambda SDK invocation. Should be slightly above the Lambda execution timeout (30s). |

## Setup

1. Set the Lambda ARN:
   ```bash
   export WWC_LAMBDA_STARTERS_ARN="arn:aws:lambda:us-east-1:123456789:function:webchat-starters"
   ```

2. Optionally configure concurrency limit:
   ```bash
   export WWC_LAMBDA_STARTERS_MAX_CONCURRENT=100
   ```

3. Optionally set Lambda region (if different from S3):
   ```bash
   export WWC_LAMBDA_STARTERS_REGION="us-east-1"
   ```

4. Add the new dependency:
   ```bash
   go get golang.org/x/sync
   ```

5. Run normally:
   ```bash
   go run api/main.go
   ```

## Testing

```bash
go test ./pkg/websocket/... -v -run TestGetPDPStarters
go test ./pkg/starters/... -v
```

## Verifying the Feature

1. Connect a WebSocket client to `ws://localhost:8080/ws`
2. Send a `register` event
3. Wait for `ready_for_message`
4. Send:
   ```json
   {
     "type": "get_pdp_starters",
     "from": "<your-client-id>",
     "data": {
       "account": "test-store",
       "linkText": "test-product"
     }
   }
   ```
5. Expect a `starters` event with `data.questions` array (or `error` if Lambda is not reachable)

## Feature Toggle

To disable the feature, simply leave `WWC_LAMBDA_STARTERS_ARN` empty or unset. The handler will silently ignore `get_pdp_starters` events with a debug-level log.

## Deployment Notes

- **New env vars**: `WWC_LAMBDA_STARTERS_ARN`, `WWC_LAMBDA_STARTERS_MAX_CONCURRENT`, `WWC_LAMBDA_STARTERS_REGION` must be added to Kubernetes deployment manifests / infrastructure config.
- **IAM permissions**: The pod's IAM role must have `lambda:InvokeFunction` on the configured Lambda ARN.
- **New dependency**: `golang.org/x/sync` (semaphore). No new Docker image changes needed beyond the Go build.
- **No breaking changes**: Existing message contracts are unchanged. New event type `get_pdp_starters` is additive.
