package history

import (
	"os"
	"testing"
	"time"

	"github.com/ilhasoft/wwcs/pkg/db"
	"github.com/stretchr/testify/assert"
)

var message1 = &MessagePayload{
	Message: Message{
		Type: "text",
		Text: "Hello",
	},
	ContactURN:  "test:123",
	ChannelUUID: "AbCdEf-123456-123456",
	Timestamp:   time.Now().Add(-2 * time.Second).Unix(),
	Direction:   "in",
}

var message2 = &MessagePayload{
	Message: Message{
		Type: "text",
		Text: "world!",
	},
	ContactURN:  "test:123",
	ChannelUUID: "AbCdEf-123456-123456",
	Timestamp:   time.Now().Add(-1 * time.Second).Unix(),
	Direction:   "in",
}

func TestRepo(t *testing.T) {
	os.Setenv("WWC_DB_URI", "mongodb://admin:admin@localhost:27017/")
	os.Setenv("WWC_DB_NAME", "weni-webchat")

	mdb := db.NewDB()
	err := db.Clear(mdb)
	assert.NoError(t, err)

	repo := NewRepo(mdb, time.Duration(15))

	// should get 0 records
	messages, err := repo.Get(message1.ContactURN, message1.ChannelUUID, nil, 10, 1)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(messages))

	// saving the first message
	err = repo.Save(*message1)
	assert.NoError(t, err)

	// saving the second message
	err = repo.Save(*message2)
	assert.NoError(t, err)

	// shold get the 2 messages saved before
	messages, err = repo.Get(message1.ContactURN, message1.ChannelUUID, nil, 10, 1)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(messages))

	// shold get the 1 message because pagination limit is 1
	messages, err = repo.Get(message1.ContactURN, message1.ChannelUUID, nil, 1, 1)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(messages))
	assert.Equal(t, message2.Message.Text, messages[0].Message.Text)

	// shoud get 0 messages because get messages before the pastMoment that is now - 1hour
	pastMoment := time.Now().Add(-1 * time.Hour)
	messages, err = repo.Get(message1.ContactURN, message1.ChannelUUID, &pastMoment, 10, 1)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(messages))
}
