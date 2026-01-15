package history

import (
	"time"

	log "github.com/sirupsen/logrus"
)

// Service represents a message service.
type Service interface {
	Get(contactURN, channelUUID string, before *time.Time, limit, page int) ([]MessagePayload, error)
	Save(msg MessagePayload) error
}

type service struct {
	repo Repo
}

// NewService creates and return a new message service.
func NewService(repo Repo) Service {
	return &service{
		repo: repo,
	}
}

// Get retrieves messages from the given contact URN.
// It returns a slice of messages or nil and any error ocurred while getting messages
func (s *service) Get(contactURN, channelUUID string, before *time.Time, limit, page int) ([]MessagePayload, error) {
	messages, err := s.repo.Get(contactURN, channelUUID, before, limit, page)
	if err != nil {
		return nil, err
	}
	return messages, nil
}

// Save stores the given message. It returns any error occurred while saving.
func (s *service) Save(msg MessagePayload) error {
	log.WithFields(log.Fields{
		"contact_urn":  msg.ContactURN,
		"channel_uuid": msg.ChannelUUID,
		"direction":    msg.Direction,
		"timestamp":    msg.Timestamp,
		"message_type": msg.Message.Type,
		"text_preview": truncateText(msg.Message.Text, 50),
	}).Debug("HISTORY_SAVE_DEBUG: History service Save() called")

	err := s.repo.Save(msg)
	if err != nil {
		return err
	}
	return nil
}

// truncateText truncates text to maxLen characters, adding "..." if truncated
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}
