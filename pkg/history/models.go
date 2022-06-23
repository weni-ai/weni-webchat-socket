package history

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type MessagePayload struct {
	ID          *primitive.ObjectID `bson:"_id,omitempty"`
	ContactURN  string              `json:"contact_urn,omitempty" bson:"contact_urn,omitempty"`
	Message     Message             `json:"message,omitempty" bson:"message,omitempty"`
	Direction   string              `json:"direction,omitempty" bson:"direction,omitempty"`
	Timestamp   int64               `json:"timestamp,omitempty" bson:"timestamp,omitempty"`
	ChannelUUID string              `json:"channel_uuid,omitempty" bson:"channel_uuid,omitempty"`
}

// Message data
type Message struct {
	Type         string   `json:"type" bson:"type,omitempty"`
	Timestamp    string   `json:"timestamp" bson:"timestamp"`
	Text         string   `json:"text,omitempty" bson:"text"`
	Media        string   `json:"media,omitempty" bson:"media,omitempty"`
	MediaURL     string   `json:"media_url,omitempty" bson:"media_url,omitempty"`
	Caption      string   `json:"caption,omitempty" bson:"caption,omitempty"`
	Latitude     string   `json:"latitude,omitempty" bson:"latitude,omitempty"`
	Longitude    string   `json:"longitude,omitempty" bson:"longitude,omitempty"`
	QuickReplies []string `json:"quick_replies,omitempty" bson:"quick_replies,omitempty"`
}

func NewMessagePayload(direction string, contactURN string, channelUUID string, message Message) *MessagePayload {
	return &MessagePayload{
		ContactURN: contactURN,
		Direction:  direction,
		Message:    message,
		Timestamp:  time.Now().UnixNano(),
	}
}
