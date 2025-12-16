package history

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMessagePayload(t *testing.T) {
	msg := Message{
		Type:         "text",
		Timestamp:    fmt.Sprint(time.Now().Unix()),
		Text:         "hello",
		Media:        "",
		MediaURL:     "",
		Caption:      "",
		Latitude:     "",
		Longitude:    "",
		QuickReplies: []string{},
		ListMessage:  ListMessage{},
		CTAMessage:   nil,
	}

	newHistoryMsg := NewMessagePayload("outcoming", "text:123456", "bba8457f-69b2-4f67-bab5-72c463fa7701", msg)

	assert.NotNil(t, newHistoryMsg)
}

func TestNewMessagePayloadWithCTAMessage(t *testing.T) {
	now := fmt.Sprint(time.Now().Unix())
	cta := &CTAMessage{
		URL:         "https://weni.ai",
		DisplayText: "Go to Weni",
	}
	msg := Message{
		Type:         "text",
		Timestamp:    now,
		Text:         "hello",
		QuickReplies: []string{},
		ListMessage:  ListMessage{},
		CTAMessage:   cta,
	}

	newHistoryMsg := NewMessagePayload("outcoming", "text:123456", "bba8457f-69b2-4f67-bab5-72c463fa7701", msg)
	assert.NotNil(t, newHistoryMsg.Message.CTAMessage)
	assert.Equal(t, cta.URL, newHistoryMsg.Message.CTAMessage.URL)
	assert.Equal(t, cta.DisplayText, newHistoryMsg.Message.CTAMessage.DisplayText)
}

func TestCTAMessageOmitEmptyOnMarshal(t *testing.T) {
	msg := Message{
		Type:      "text",
		Timestamp: fmt.Sprint(time.Now().Unix()),
		Text:      "hello",
	}
	b, err := json.Marshal(msg)
	assert.NoError(t, err)
	jsonStr := string(b)
	assert.False(t, strings.Contains(jsonStr, "\"cta_message\""))
}
