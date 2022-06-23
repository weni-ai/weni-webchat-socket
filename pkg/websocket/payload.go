package websocket

import (
	"time"

	"github.com/ilhasoft/wwcs/pkg/history"
)

// IncomingPayload data (incoming messages)
type IncomingPayload struct {
	Type    string  `json:"type" validate:"required"`
	To      string  `json:"to" validate:"required"`
	From    string  `json:"from" validate:"required"`
	Error   string  `json:"error,omitempty"`
	Message Message `json:"message,omitempty"`
	Token   string  `json:"token,omitempty"`
	Warning string  `json:"warning,omitempty"`
}

// OutgoingPayload data (outgoing messages)
type OutgoingPayload struct {
	Type        string                 `json:"type,omitempty" validate:"required"`
	From        string                 `json:"from,omitempty"`
	Callback    string                 `json:"callback,omitempty"`
	Trigger     string                 `json:"trigger,omitempty"`
	Message     Message                `json:"message,omitempty"`
	Token       string                 `json:"token,omitempty"`
	SessionType SessionType            `json:"session_type"` // if "local" must save messages in history
	Params      map[string]interface{} `json:"params,omitempty"`
}

// HistoryPayload data (history messages)
type HistoryPayload struct {
	Type    string                   `json:"type,omitempty"`
	To      string                   `json:"to,omitempty"`
	History []history.MessagePayload `json:"history,omitempty"`
}

// Message data
type Message struct {
	Type         string   `json:"type"`
	Timestamp    string   `json:"timestamp"`
	Text         string   `json:"text,omitempty"`
	Media        string   `json:"media,omitempty"`
	MediaURL     string   `json:"media_url,omitempty"`
	Caption      string   `json:"caption,omitempty"`
	Latitude     string   `json:"latitude,omitempty"`
	Longitude    string   `json:"longitude,omitempty"`
	QuickReplies []string `json:"quick_replies,omitempty"`
}

type OutgoingJob struct {
	URL     string
	Payload OutgoingPayload
}

type IncomingJob struct {
	Payload IncomingPayload
}

type Direction string

func (d Direction) String() string {
	return string(d)
}

const (
	DirectionIn  Direction = "in"
	DirectionOut Direction = "out"
)

func NewHistoryMessagePayload(direction Direction, contactURN string, channelUUID string, message Message) history.MessagePayload {
	return history.MessagePayload{
		ContactURN:  contactURN,
		Direction:   direction.String(),
		ChannelUUID: channelUUID,
		Timestamp:   time.Now().UnixNano(),
		Message: history.Message{
			message.Type, message.Timestamp, message.Text, message.Media, message.MediaURL, message.Caption, message.Latitude, message.Longitude, message.QuickReplies,
		},
	}
}
