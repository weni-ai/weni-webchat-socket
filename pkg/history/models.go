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
	Type         string       `json:"type" bson:"type,omitempty"`
	Timestamp    string       `json:"timestamp" bson:"timestamp"`
	Text         string       `json:"text,omitempty" bson:"text"`
	Media        string       `json:"media,omitempty" bson:"media,omitempty"`
	MediaURL     string       `json:"media_url,omitempty" bson:"media_url,omitempty"`
	Caption      string       `json:"caption,omitempty" bson:"caption,omitempty"`
	Latitude     string       `json:"latitude,omitempty" bson:"latitude,omitempty"`
	Longitude    string       `json:"longitude,omitempty" bson:"longitude,omitempty"`
	QuickReplies []string     `json:"quick_replies,omitempty" bson:"quick_replies,omitempty"`
	ListMessage  ListMessage  `json:"list_message,omitempty" bson:"list_message,omitempty"`
	CTAMessage   *CTAMessage  `json:"cta_message,omitempty" bson:"cta_message,omitempty"`
	Interactive  *Interactive `json:"interactive,omitempty" bson:"interactive,omitempty"`
	Order        *Order       `json:"order,omitempty" bson:"order,omitempty"`
}

type ListMessage struct {
	ButtonText string      `json:"button_text"`
	ListItems  []ListItems `json:"list_items"`
}

type ListItems struct {
	UUID        string `json:"uuid"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type CTAMessage struct {
	URL         string `json:"url" bson:"url,omitempty"`
	DisplayText string `json:"display_text" bson:"display_text,omitempty"`
}

// Interactive message types (product_list, product, etc.)
type Interactive struct {
	Type   string            `json:"type" bson:"type,omitempty"`
	Action InteractiveAction `json:"action" bson:"action,omitempty"`
}

type InteractiveAction struct {
	Name     string               `json:"name,omitempty" bson:"name,omitempty"`
	Sections []InteractiveSection `json:"sections,omitempty" bson:"sections,omitempty"`
}

type InteractiveSection struct {
	Title        string        `json:"title,omitempty" bson:"title,omitempty"`
	ProductItems []ProductItem `json:"product_items,omitempty" bson:"product_items,omitempty"`
}

type ProductItem struct {
	ProductRetailerID string `json:"product_retailer_id,omitempty" bson:"product_retailer_id,omitempty"`
	Name              string `json:"name,omitempty" bson:"name,omitempty"`
	Price             string `json:"price,omitempty" bson:"price,omitempty"`
	Image             string `json:"image,omitempty" bson:"image,omitempty"`
	Description       string `json:"description,omitempty" bson:"description,omitempty"`
	SellerID          string `json:"seller_id,omitempty" bson:"seller_id,omitempty"`
	Quantity          int    `json:"quantity,omitempty" bson:"quantity,omitempty"`
}

// Order message (sent by client when placing an order)
type Order struct {
	Text         string        `json:"text,omitempty" bson:"text,omitempty"`
	ProductItems []ProductItem `json:"product_items,omitempty" bson:"product_items,omitempty"`
}

func NewMessagePayload(direction string, contactURN string, channelUUID string, message Message) *MessagePayload {
	return &MessagePayload{
		ContactURN: contactURN,
		Direction:  direction,
		Message:    message,
		Timestamp:  time.Now().Unix(),
	}
}
