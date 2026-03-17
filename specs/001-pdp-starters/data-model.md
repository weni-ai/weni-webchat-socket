# Data Model: PDP Conversation Starters

**Branch**: `001-pdp-starters` | **Date**: 2026-03-08

## Entities

### StartersInput (new)

Represents the product data sent by the client, extracted from `OutgoingPayload.Data` and forwarded to the Lambda.

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| Account | string | yes | VTEX store account identifier |
| LinkText | string | yes | Product slug (canonical URL identifier) |
| ProductName | string | no | Product display name |
| Description | string | no | Product description text |
| Brand | string | no | Product brand name |
| Attributes | map[string]string | no | Product attributes (key-value pairs) |

**Validation rules**:
- `Account` must be non-empty
- `LinkText` must be non-empty
- All other fields are optional (Lambda handles missing data gracefully)

### StartersOutput (new)

Represents the Lambda response containing generated questions.

| Field | Type | Required | Description |
| ----- | ---- | -------- | ----------- |
| Questions | []string | yes | Array of generated conversation starter questions (up to 3) |

**Validation rules**:
- `Questions` must be a non-nil array with at least 1 element
- Empty array or missing field is treated as an error

### LambdaStartersConfig (new, in config package)

Configuration for the Lambda starters feature.

| Field | Type | Env Var | Default | Description |
| ----- | ---- | ------- | ------- | ----------- |
| LambdaStartersARN | string | WWC_LAMBDA_STARTERS_ARN | "" | Lambda function ARN; empty disables feature |
| LambdaStartersMaxConcurrent | int64 | WWC_LAMBDA_STARTERS_MAX_CONCURRENT | 50 | Max concurrent Lambda invocations per pod |
| LambdaStartersRegion | string | WWC_LAMBDA_STARTERS_REGION | "" | AWS region for Lambda; falls back to S3 region |
| LambdaStartersTimeoutSec | int64 | WWC_LAMBDA_STARTERS_TIMEOUT_SEC | 35 | Timeout in seconds for Lambda SDK invocation; bounds goroutine duration |

## Existing Entities (unchanged)

### OutgoingPayload

No structural changes. The `Data map[string]interface{}` field is already present and will transport the product data from the client.

### IncomingPayload

No structural changes. The `Data map[string]any` field is already present and will transport `{"questions": [...]}` in the response.

## Relationships

```
Client --sends--> OutgoingPayload{type: "get_pdp_starters", data: StartersInput}
    |
    v
ParsePayload --delegates--> GetPDPStarters handler
    |
    v (goroutine)
StartersService.GetStarters(StartersInput) --> StartersOutput
    |
    v
Client <--receives-- IncomingPayload{type: "starters", data: {"questions": StartersOutput.Questions}}
```
