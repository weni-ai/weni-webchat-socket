package websocket

import (
	"fmt"
	"strings"

	"github.com/ilhasoft/wwcs/pkg/helper/slicex"
)

var validMessageTypes = []string{"text"}

const errorPrefix = "invalid payload:"

// validate errors
var (
	ErrorInvalidPayloadType = fmt.Errorf("%s invalid payload.type", errorPrefix)
	// message
	ErrorBlankMessageType   = fmt.Errorf("%s blank message.type", errorPrefix)
	ErrorInvalidMessageType = fmt.Errorf("%s invalid message.type", errorPrefix)
	// register
)

func validateSocketPayload(payload SocketPayload) error {
	switch payload.Type {
	case "message":
		return validateSocketPayloadMessage(payload)
	case "register":
		return validateSocketPayloadRegister(payload)
	default:
		return ErrorInvalidPayloadType
	}
}

func validateSocketPayloadMessage(payload SocketPayload) error {
	message := payload.Message
	var logs []string
	// check if type is blank
	if message.Type == "" {
		return ErrorBlankMessageType
	}
	// check if message type is valid
	if !slicex.FoundString(validMessageTypes, message.Type) {
		return ErrorInvalidMessageType
	}

	// validate text type
	if message.Type == "text" {
		if message.Text == "" {
			logs = append(logs, "blank message.text")
		}
	}

	// append all logs to one error
	if logs != nil {
		return fmt.Errorf("%s %s", errorPrefix, strings.Join(logs, ", "))
	}

	return nil
}

func validateSocketPayloadRegister(payload SocketPayload) error {
	var logs []string
	if payload.From == "" {
		logs = append(logs, "blank from")
	}

	if payload.Callback == "" {
		logs = append(logs, "blank callback")
	}

	if logs != nil {
		return fmt.Errorf("%s %s", errorPrefix, strings.Join(logs, ", "))
	}

	return nil
}
