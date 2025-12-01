package websocket

import (
	"net/url"
	"strings"

	"github.com/ilhasoft/wwcs/pkg/history"
)

// IncomingPayload data (incoming messages)
type IncomingPayload struct {
	Type        string         `json:"type" validate:"required"`
	To          string         `json:"to" validate:"required"`
	From        string         `json:"from" validate:"required"`
	Error       string         `json:"error,omitempty"`
	Message     Message        `json:"message,omitempty"`
	Token       string         `json:"token,omitempty"`
	Warning     string         `json:"warning,omitempty"`
	ChannelUUID string         `json:"channel_uuid,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
}

// OutgoingPayload data (outgoing messages)
type OutgoingPayload struct {
	Type     string                 `json:"type,omitempty" validate:"required"`
	From     string                 `json:"from,omitempty"`
	Callback string                 `json:"callback,omitempty"`
	Trigger  string                 `json:"trigger,omitempty"`
	Message  Message                `json:"message,omitempty"`
	Token    string                 `json:"token,omitempty"`
	Params   map[string]interface{} `json:"params,omitempty"`
	Context  string                 `json:"context,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

func (p *OutgoingPayload) ChannelUUID() string {
	// "https://flows.stg.cloud.weni.ai/c/wwc/c4dc40fa-37e0-4147-a379-a3e8ffd23f80/receive"
	cbsplited := strings.Split(p.Callback, "/")
	if len(cbsplited) < 2 {
		return ""
	}
	return cbsplited[len(cbsplited)-2]
}

func (p *OutgoingPayload) Host() (string, error) {
	cbURL, err := url.Parse(p.Callback)
	if err != nil {
		return "", err
	}
	return cbURL.Host, nil
}

// HistoryPayload data (history messages)
type HistoryPayload struct {
	Type    string                   `json:"type,omitempty"`
	To      string                   `json:"to,omitempty"`
	History []history.MessagePayload `json:"history,omitempty"`
}

// Message data
type Message struct {
	Type         string              `json:"type"`
	Timestamp    string              `json:"timestamp"`
	Text         string              `json:"text,omitempty"`
	Media        string              `json:"media,omitempty"`
	MediaURL     string              `json:"media_url,omitempty"`
	Caption      string              `json:"caption,omitempty"`
	Latitude     string              `json:"latitude,omitempty"`
	Longitude    string              `json:"longitude,omitempty"`
	QuickReplies []string            `json:"quick_replies,omitempty"`
	ListMessage  history.ListMessage `json:"list_message,omitempty"`

	// Streaming support field (for delta messages from Nexus)
	MessageID string `json:"messageId,omitempty"` // Unique ID that groups delta chunks together
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

func NewHistoryMessagePayload(direction Direction, contactURN string, channelUUID string, message Message, timestamp int64) history.MessagePayload {
	return history.MessagePayload{
		ContactURN:  contactURN,
		Direction:   direction.String(),
		ChannelUUID: channelUUID,
		Timestamp:   timestamp,
		Message: history.Message{
			Type:         message.Type,
			Timestamp:    message.Timestamp,
			Text:         message.Text,
			Media:        message.Media,
			MediaURL:     message.MediaURL,
			Caption:      message.Caption,
			Latitude:     message.Latitude,
			Longitude:    message.Longitude,
			QuickReplies: message.QuickReplies,
			ListMessage:  message.ListMessage,
		},
	}
}
