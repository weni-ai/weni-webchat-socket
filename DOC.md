# Doc

## Frontend to backend

### Register payload

```json
{
   "type":"register",
   "from":"<uuid>",
   "callback":"<url>",
   "trigger": "<trigger>",
}
```

### Send payload

```json
{
   "type":"message",
   "message":{
      "text":"<text_message>"
   }
}
```

## Backend to Frontend

### Receive payload

```json
{
   "type":"message",
   "to":"<to>",
   "from":"<from>",
   "message":{
      "id":"<id>",
      "type": "text",
      "text": "<text>",
      "quick_replies":"<quick_replies>",
   }
}
```

### External body

- Formatted
```json
{
   "type":"message",
   "to":{{to}},
   "from":{{channel}},
   "message":{
      "id":{{id}},
      "type":"text",
      "text":{{text}},
      "quick_replies":{{quick_replies}}
   }
}
```

- One line
```json
{"type":"message","to":{{to}},"from":{{channel}},"message":{"id":{{id}},"type":"text","text":{{text}},"quick_replies":{{quick_replies}}}}
```