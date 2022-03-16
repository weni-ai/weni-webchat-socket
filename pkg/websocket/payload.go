package websocket

// IncomingPayload data (incoming messages)
type IncomingPayload struct {
	Type    string  `json:"type" validate:"required"`
	To      string  `json:"to" validate:"required"`
	From    string  `json:"from" validate:"required"`
	Error   string  `json:"error,omitempty"`
	Message Message `json:"message,omitempty"`
	Token   string  `json:"token,omitempty"`
	Warning  string  `json:"warning,omitempty"`
}

// OutgoingPayload data (outgoing messages)
type OutgoingPayload struct {
	Type     string  `json:"type" validate:"required"`
	From     string  `json:"from,omitempty"`
	Callback string  `json:"callback,omitempty"`
	Trigger  string  `json:"trigger,omitempty"`
	Message  Message `json:"message,omitempty"`
	Token    string  `json:"token,omitempty"`
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
