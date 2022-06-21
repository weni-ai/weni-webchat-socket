package history

import (
	"errors"
	"testing"
	"time"

	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

const contactURN = "test:12345"
const channelUUID = "949e0cf8-3dc8-4103-859a-a8432aa17b9c"

func TestGet(t *testing.T) {
	t.Run("should get messages", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := NewMockRepo(ctrl)

		msgs := []MessagePayload{{}, {}}

		service := NewService(mockRepo)
		mockRepo.EXPECT().Get(contactURN, channelUUID, 10, 0).Return(msgs, nil)

		messages, err := service.Get(contactURN, channelUUID, 10, 0)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(messages))
	})

	t.Run("should return error when retrieving messages fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := NewMockRepo(ctrl)

		service := NewService(mockRepo)
		mockRepo.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("dummy error"))

		_, err := service.Get(contactURN, channelUUID, 10, 0)
		assert.Error(t, err)
	})
}

func TestSave(t *testing.T) {
	t.Run("should save a valid message", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := NewMockRepo(ctrl)
		msg := MessagePayload{
			ContactURN:  contactURN,
			ChannelUUID: channelUUID,
			Message:     Message{Type: "text", Text: "Hello"},
			Direction:   "incoming",
			Timestamp:   time.Now().UnixNano(),
		}

		service := NewService(mockRepo)
		mockRepo.EXPECT().Save(msg).Return(nil)

		err := service.Save(msg)
		assert.NoError(t, err)
	})

	t.Run("should return error when saving message fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRepo := NewMockRepo(ctrl)
		msg := MessagePayload{}

		service := NewService(mockRepo)
		mockRepo.EXPECT().Save(gomock.Any()).Return(errors.New("dummy error"))

		err := service.Save(msg)
		assert.Error(t, err)
	})
}
