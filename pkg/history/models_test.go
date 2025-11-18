package history

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMessagePayload(t *testing.T) {
	msg := Message{
		"text",
		fmt.Sprint(time.Now().Unix()),
		"hello",
		"",
		"",
		"",
		"",
		"",
		[]string{},
		ListMessage{},
	}

	newHistoryMsg := NewMessagePayload("outcoming", "text:123456", "bba8457f-69b2-4f67-bab5-72c463fa7701", msg)

	assert.NotNil(t, newHistoryMsg)
}
