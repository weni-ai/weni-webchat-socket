package message

import "go.mongodb.org/mongo-driver/bson/primitive"

type Message struct {
	ID          primitive.ObjectID `json:"id,omitempty"`
	ContactURN  string             `json:"contact_urn,omitempty"`
	Msg         string             `json:"msg,omitempty"`
	Direction   string             `json:"direction,omitempty"`
	Timestamp   string             `json:"timestamp,omitempty"`
	ChannelUUID string             `json:"channel_uuid,omitempty"`
}
