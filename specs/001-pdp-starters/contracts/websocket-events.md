# WebSocket Event Contracts: PDP Conversation Starters

**Branch**: `001-pdp-starters` | **Date**: 2026-03-08

## Event: `get_pdp_starters` (client → server)

**Direction**: Client sends to server  
**Payload type**: `OutgoingPayload`

```json
{
  "type": "get_pdp_starters",
  "from": "<client-uuid>",
  "data": {
    "account": "brandless",
    "linkText": "ipad-10th-gen",
    "productName": "iPad 10th Gen",
    "description": "Versatile tablet with large Retina display and support for Apple Pencil.",
    "brand": "Apple",
    "attributes": {
      "Storage": "64GB, 256GB",
      "Color": "Blue, Silver, Pink"
    }
  }
}
```

**Required fields in `data`**:
- `account` (string, non-empty)
- `linkText` (string, non-empty)

**Optional fields in `data`**:
- `productName` (string)
- `description` (string)
- `brand` (string)
- `attributes` (object, string keys → string values)

**Pre-conditions**:
- Client must be registered (have sent `register` and received `ready_for_message`)
- Lambda ARN must be configured (otherwise event is silently ignored)

---

## Event: `starters` (server → client)

**Direction**: Server sends to client  
**Payload type**: `IncomingPayload`

```json
{
  "type": "starters",
  "to": "<client-uuid>",
  "from": "system",
  "data": {
    "questions": [
      "Qual a diferença entre as versões de 64GB e 256GB?",
      "O iPad 10th Gen é compatível com Apple Pencil de qual geração?",
      "Quais cores estão disponíveis para pronta entrega?"
    ]
  }
}
```

**Fields in `data`**:
- `questions` (array of strings, 1–3 elements)

---

## Event: `error` (server → client, on failure)

**Direction**: Server sends to client  
**Payload type**: `IncomingPayload`

```json
{
  "type": "error",
  "error": "failed to generate conversation starters: <reason>"
}
```

**Error conditions**:
- Client not registered → `ErrorNeedRegistration`
- Missing required product data (`account` or `linkText`) → validation error
- Lambda invocation failure (timeout, throttling, network error) → wrapped error
- Lambda response missing `questions` or invalid format → parse error
- Concurrency limit reached → capacity error

---

## Lambda Invocation Contract

**Invocation type**: `RequestResponse` (synchronous, via AWS SDK v1)

### Input payload (socket → Lambda)

```json
{
  "account": "brandless",
  "linkText": "ipad-10th-gen",
  "productName": "iPad 10th Gen",
  "description": "Versatile tablet with large Retina display and support for Apple Pencil.",
  "brand": "Apple",
  "attributes": {
    "Storage": "64GB, 256GB",
    "Color": "Blue, Silver, Pink"
  }
}
```

### Output payload (Lambda → socket)

```json
{
  "questions": [
    "Qual a diferença entre as versões de 64GB e 256GB?",
    "O iPad 10th Gen é compatível com Apple Pencil de qual geração?",
    "Quais cores estão disponíveis para pronta entrega?"
  ]
}
```

### Error responses

- Lambda function error (`FunctionError` non-nil in SDK response) → treated as invocation failure
- HTTP status != 200 in Lambda response → treated as invocation failure
- JSON decode failure → treated as invalid response
- Empty or missing `questions` array → treated as invalid response
