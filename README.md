# weni-webchannel-socket

## Index

- [Features](#features)
- [Endpoints](#endpoints)
    - [POST /send](#post-send)
        - [Text](#text)
        - [Image](#image)
        - [Video](#video)
        - [Document](#document)
        - [Location](#location)
        - [Quick Reply](#quick-reply)

## Features

- Send and receive messages
    - text
    - image
    - video
    - audio
    - document
    - location
    - quick replies
    - form
- Send notifications
    - browser
    - email
    - sound alerts
- Easy configuration
    - Extremely customizable
    - Easy connection
    - Fast integration
- Accessibility
    - Multi language
    - Read accessibility
    - Text to speech
- Save history
- Send initial form to register
- Call back methods

## Endpoints

### POST /send

#### Text

```json
POST /send
{
    "to": "foo",
    "from": "bar",
    "message": {
        "type": "text",
        "text": "Hello socket!"
    }
}
```

#### Image

```json
POST /send
{
    "to": "foo",
    "from": "bar",
    "message": {
        "type": "image",
        "url": "https://www.foo.bar",
        "caption": "My img caption!"
    }
}
```

#### Video

```json
POST /send
{
    "to": "foo",
    "from": "bar",
    "message": {
        "type": "video",
        "url": "https://www.foo.bar",
        "caption": "My img caption!"
    }
}
```

#### Document

```json
POST /send
{
    "to": "foo",
    "from": "bar",
    "message": {
        "type": "document",
        "url": "https://www.foo.bar",
        "filename": "foo.txt"
    }
}
```

#### Location

```json
POST /send
{
    "to": "foo",
    "from": "bar",
    "message": {
        "type": "document",
        "latitude": "0.00000",
        "longitude": "1.00000"
    }
}
```

#### Quick Reply

```json
POST /send
{
    "to": "foo",
    "from": "bar",
    "message": {
        "type": "text",
        "text": "Hello socket!",
        "quickReplies": [
            {"text": "value1"}
            {"text": "value2"}
            {"text": "value3"}
            {"text": "value4"}
        ]
    }
}
```
