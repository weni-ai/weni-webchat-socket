# Examples


## Incoming (external to socket)

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

## Outgoing (socket to external)

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

## Incoming (client to socket)

- Register

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

## Outgoing (client to socket)