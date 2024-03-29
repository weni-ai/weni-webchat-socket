package history

import "time"

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
	err := s.repo.Save(msg)
	if err != nil {
		return err
	}
	return nil
}
