# Doc

## Frontend to backend

### Register payload

```json
{
   "type":"register",
   "from":"<uuid>",
   "callback":"<url>"
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
   "to":"<to>",
   "to_no_plus":"<to_no_plus>",
   "from":"<from>",
   "from_no_plus":"<from_no_plus>",
   "text":"<text>",
   "id":"<id>",
   "quick_replies":"<quick_replies>"
}
```

### External body

- Formatted
```json
{
   "id":{{id}},
   "text":{{text}},
   "to":{{to}},
   "to_no_plus":{{to_no_plus}},
   "from":{{from}},
   "from_no_plus":{{from_no_plus}},
   "channel":{{channel}}
}
```

- One line

```json
{"id":{{id}},"text":{{text}},"to":{{to}},"to_no_plus":{{to_no_plus}},"from":{{from}},"from_no_plus":{{from_no_plus}},"channel":{{channel}}}
```