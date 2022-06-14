package message

// Service represents a message service.
type Service interface {
	Get(contactURN string) ([]Message, error)
	Save(msg Message) error
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
func (s *service) Get(contactURN string) ([]Message, error) {
	return nil, nil
}

// Save stores the given message.
func (s *service) Save(msg Message) error {
	return nil
}
