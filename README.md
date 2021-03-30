# weni-webchannel-socket

## Index

- [Features](#features)
- [Examples](#examples)
    - [Incoming (external to socket)](#incoming-external-to-socket)
    - [Outgoing (socket to external)](#outgoing-socket-to-external)
    - [Incoming (client to socket)](#incoming-client-to-socket)
    - [Outgoing (socket to client)](#outgoing-socket-to-client)

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

## Examples

### Incoming (external to socket)

- Text Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"text", //required
		"timestamp":"1616891274", //required
		"text":"Hello World!", //required
        "quick_replies": ["1","2","3"]
	}
}
```

- Image Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"image", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/image.png", //required
        "caption":"My caption",
        "quick_replies": ["1","2","3"]
	}
}
```

- Video Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"video", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/video.mp4", //required
        "caption":"My caption",
        "quick_replies": ["1","2","3"]
	}
}
```

- Audio Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"audio", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/audio.mp3", //required
        "caption":"My caption",
        "quick_replies": ["1","2","3"]
	}
}
```

- File Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"file", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/file.pdf", //required
        "caption":"My caption",
        "quick_replies": ["1","2","3"]
	}
}
```

- Location Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"location", //required
		"timestamp":"1616891274", //required
		"latitude":"-12.4364187", //required
        "longitude":"-49.5538636", //required
        "quick_replies": ["1","2","3"]
	}
}
```

### Outgoing (socket to external)

- Text Message

```json
{
	"type":"message", //required
	"from":"Caio", //required
	"message": {
		"type":"text", //required
		"timestamp":"1616891274", //required
		"text":"Hello World!", //required
	}
}
```

- Image Message

```json
{
	"type":"message", //required
	"from":"Caio", //required
	"message": {
		"type":"image", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/image.png", //required
        "caption":"My caption",
	}
}
```

- Video Message

```json
{
	"type":"message", //required
	"from":"Caio", //required
	"message": {
		"type":"video", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/video.mp4", //required
        "caption":"My caption",
	}
}
```

- Audio Message

```json
{
	"type":"message", //required
	"from":"Caio", //required
	"message": {
		"type":"audio", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/audio.mp3", //required
        "caption":"My caption",
	}
}
```

- File Message

```json
{
	"type":"message", //required
	"from":"Caio", //required
	"message": {
		"type":"file", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/file.pdf", //required
        "caption":"My caption",
	}
}
```

- Location Message

```json
{
	"type":"message", //required
	"from":"Caio", //required
	"message": {
		"type":"location", //required
		"timestamp":"1616891274", //required
		"latitude":"-12.4364187", //required
        "longitude":"-49.5538636", //required
	}
}
```

### Incoming (client to socket)

- Register (it is mandatory to be the first communication)

```json
{
   "type":"register", //required
   "from":"<uuid>",  //required
   "callback":"<url>",  //required
   "trigger": "<trigger>",
}
```

- Text Message

```json
{
	"type":"message", //required
	"message": {
		"type":"text", //required
		"text":"Hello World!", //required
	}
}
```

- Image Message

```json
{
	"type":"message", //required
	"message": {
		"type":"image", //required
		"media":"media_content", //required
        "caption":"My caption",
	}
}
```

- Video Message

```json
{
	"type":"message", //required
	"message": {
		"type":"video", //required
		"media":"media_content", //required
        "caption":"My caption",
	}
}
```

- Audio Message

```json
{
	"type":"message", //required
	"message": {
		"type":"audio", //required
		"media":"media_content", //required
        "caption":"My caption",
	}
}
```

- File Message

```json
{
	"type":"message", //required
	"message": {
		"type":"file", //required
		"media":"media_content", //required
        "caption":"My caption",
	}
}
```

- Location Message

```json
{
	"type":"message", //required
	"message": {
		"type":"location", //required
		"latitude":"-12.4364187", //required
        "longitude":"-49.5538636", //required
	}
}
```

### Outgoing (socket to client)

- Error

```json
{
	"type":"error", //required
	"error":"my_error" //required
}
```

- Text Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"text", //required
		"timestamp":"1616891274", //required
		"text":"Hello World!", //required
        "quick_replies": ["1","2","3"]
	}
}
```

- Image Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"image", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/image.png", //required
        "caption":"My caption",
        "quick_replies": ["1","2","3"]
	}
}
```

- Video Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"video", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/video.mp4", //required
        "caption":"My caption",
        "quick_replies": ["1","2","3"]
	}
}
```

- Audio Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"audio", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/audio.mp3", //required
        "caption":"My caption",
        "quick_replies": ["1","2","3"]
	}
}
```

- File Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"file", //required
		"timestamp":"1616891274", //required
		"media_url":"https://foo.bar/file.pdf", //required
        "caption":"My caption",
        "quick_replies": ["1","2","3"]
	}
}
```

- Location Message

```json
{
	"type":"message", //required
	"to":"Lucas", //required
	"from":"Caio", //required
	"message": {
		"type":"location", //required
		"timestamp":"1616891274", //required
		"latitude":"-12.4364187", //required
        "longitude":"-49.5538636", //required
        "quick_replies": ["1","2","3"]
	}
}
```