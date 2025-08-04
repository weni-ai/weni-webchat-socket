# weni-webchannel-socket

[![ci](https://github.com/Ilhasoft/weni-webchat-socket/actions/workflows/ci.yml/badge.svg)](https://github.com/Ilhasoft/weni-webchat-socket/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/Ilhasoft/weni-webchat-socket/branch/main/graph/badge.svg?token=1WQT40U2EQ)](https://codecov.io/gh/Ilhasoft/weni-webchat-socket)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/Ilhasoft/weni-webchat-socket)

## Index

- [Running](#running)
- [Features](#features)
- [Examples](#examples)
	- [Incoming (external to socket)](#incoming-external-to-socket)
	- [Outgoing (socket to external)](#outgoing-socket-to-external)
	- [Incoming (client to socket)](#incoming-client-to-socket)
	- [Outgoing (socket to client)](#outgoing-socket-to-client)

## Running

- Environment Variables

	|         Variable                               | Required | Default                                 |
	|------------------------------------------------|:--------:|:----------------------------------------|
	| WWC_PORT                                       |   false  |  8080                                   |
	| WWC_LOG_LEVEL                                  |   false  |  info                                   |
	| WWC_S3_ACCESS_KEY                              |   true   |  -                                      |
	| WWC_S3_SECRET_KEY                              |   true   |  -                                      |
	| WWC_S3_ENDPOINT                                |   true   |  -                                      |
	| WWC_S3_REGION                                  |   true   |  -                                      |
	| WWC_S3_BUCKET                                  |   true   |  -                                      |
	| WWC_S3_DISABLE_SSL                             |   false  |  false                                  |
	| WWC_S3_FORCE_PATH_STYLE                        |   false  |  false                                  |
	| WWC_REDIS_QUEUE_TAG                            |   false  |  wwcs-service                           |
	| WWC_REDIS_QUEUE_URL                            |   false  |  redis://localhost:6379/1               |
	| WWC_REDIS_QUEUE_CONSUMER_PREFETCH_LIMIT        |   false  |  1000                                   |
	| WWC_REDIS_QUEUE_CONSUMER_POLL_DURATION         |   false  |  100                                    |
	| WWC_REDIS_QUEUE_RETRY_PREFETCH_LIMIT           |   false  |  1000                                   |
	| WWC_REDIS_QUEUE_RETRY_POLL_DURATION            |   false  |  60000                                  |
	| WWC_APP_SENTRY_DSN                             |   false  |  -                                      |
	| WWC_SESSION_TYPE_TO_STORE                      |   false  |  remote                                 |
	| WWC_DB_URI                                     |   false  |  mongodb://admin:admin@localhost:27017  |
	| WWC_DB_NAME                                    |   false  |  weni-webchat                           |
	| WWC_DB_CONTEXT_TIMEOUT                         |   false  |  15                                     |

- To execute the project just run:
	```sh
	go run ./api
	```

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
- Metrics

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

- Typing Indicator

```json
{
  "type": "typing_start", //required
  "to": "Lucas", //required
  "from": "Caio", //required
  "channel_uuid": "9d1339fb-c21b-4ff5-a24f-faf421c2dcdc" //optional
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

- Get History

```json
{
	"type":"get_history",
	"params":{
		"limit":10,
		"page":1
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

- History

```json
{
	"type":"history",
	"history": [
		{
			"contact_urn":"Fulano",
			"channel_uuid":"8adf206a-607b-4039-9cac-3de66d084f15",
			"direction":"in||out",
			"timestamp":"1616891274",
			"message": {
				"type":"text||image||audio||file||location",
				"timestamp":"1616891274",
				...
				...
				...
			}
		},
	]
}
```
