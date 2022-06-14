package message

import "go.mongodb.org/mongo-driver/bson/primitive"

type Message struct {
	ID          primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	ContactURN  string             `json:"contact_urn,omitempty" bson:"contact_urn,omitempty"`
	Msg         string             `json:"msg,omitempty" bson:"msg,omitempty"`
	Direction   string             `json:"direction,omitempty" bson:"direction,omitempty"`
	Timestamp   int64              `json:"timestamp,omitempty" bson:"timestamp,omitempty"`
	ChannelUUID string             `json:"channel_uuid,omitempty" bson:"channel_uuid,omitempty"`
}
