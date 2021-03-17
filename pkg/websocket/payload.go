package websocket

// ExternalPayload  data
type ExternalPayload struct {
	Type    string `json:"type"           validate:"required"`
	To      string `json:"to"             validate:"required"`
	From    string `json:"from,omitempty" validate:"required"`
	Message Message
}

// SocketPayload data
type SocketPayload struct {
	Type     string  `json:"type" validate:"required"`
	From     string  `json:"from,omitempty"`
	Callback string  `json:"callback,omitempty"`
	Trigger  string  `json:"trigger,omitempty"`
	Message  Message `json:"message,omitempty"`
}

// Message data
type Message struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Text         string   `json:"text,omitempty"`
	URL          string   `json:"url,omitempty"`
	Caption      string   `json:"caption,omitempty"`
	FileName     string   `json:"filename,omitempty"`
	Latitude     string   `json:"latitude,omitempty"`
	Longitude    string   `json:"longitude,omitempty"`
	QuickReplies []string `json:"quick_replies,omitempty"`
}
