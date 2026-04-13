package history

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMessagePayload(t *testing.T) {
	msg := Message{
		Type:         "text",
		Timestamp:    fmt.Sprint(time.Now().Unix()),
		Text:         "hello",
		Media:        "",
		MediaURL:     "",
		Caption:      "",
		Latitude:     "",
		Longitude:    "",
		QuickReplies: []string{},
		ListMessage:  ListMessage{},
		CTAMessage:   nil,
	}

	newHistoryMsg := NewMessagePayload("outcoming", "text:123456", "bba8457f-69b2-4f67-bab5-72c463fa7701", msg)

	assert.NotNil(t, newHistoryMsg)
}

func TestNewMessagePayloadWithCTAMessage(t *testing.T) {
	now := fmt.Sprint(time.Now().Unix())
	cta := &CTAMessage{
		URL:         "https://weni.ai",
		DisplayText: "Go to Weni",
	}
	msg := Message{
		Type:         "text",
		Timestamp:    now,
		Text:         "hello",
		QuickReplies: []string{},
		ListMessage:  ListMessage{},
		CTAMessage:   cta,
	}

	newHistoryMsg := NewMessagePayload("outcoming", "text:123456", "bba8457f-69b2-4f67-bab5-72c463fa7701", msg)
	assert.NotNil(t, newHistoryMsg.Message.CTAMessage)
	assert.Equal(t, cta.URL, newHistoryMsg.Message.CTAMessage.URL)
	assert.Equal(t, cta.DisplayText, newHistoryMsg.Message.CTAMessage.DisplayText)
}

func TestCTAMessageOmitEmptyOnMarshal(t *testing.T) {
	msg := Message{
		Type:      "text",
		Timestamp: fmt.Sprint(time.Now().Unix()),
		Text:      "hello",
	}
	b, err := json.Marshal(msg)
	assert.NoError(t, err)
	jsonStr := string(b)
	assert.False(t, strings.Contains(jsonStr, "\"cta_message\""))
}

func TestNewMessagePayloadWithInteractive(t *testing.T) {
	now := fmt.Sprint(time.Now().Unix())
	interactive := &Interactive{
		Type: "product_list",
		Action: InteractiveAction{
			Name: "View Product",
			Sections: []InteractiveSection{
				{
					Title: "Product Name",
					ProductItems: []ProductItem{
						{
							ProductRetailerID: "product-123",
							Name:              "Smart TV 50\"",
							Price:             "2999.90",
							Image:             "https://example.com/tv.jpg",
							Description:       "Smart TV 4K 50 inches",
							SellerID:          "seller-001",
							ProductURL:        "https://example.com/product/tv",
							Extra: map[string]any{
								"catalog_id": "cat-9",
								"position":   1,
							},
						},
					},
				},
			},
		},
	}
	msg := Message{
		Type:        "interactive",
		Timestamp:   now,
		Text:        "Check out this product!",
		Interactive: interactive,
	}

	newHistoryMsg := NewMessagePayload("incoming", "text:123456", "bba8457f-69b2-4f67-bab5-72c463fa7701", msg)
	assert.NotNil(t, newHistoryMsg.Message.Interactive)
	assert.Equal(t, "product_list", newHistoryMsg.Message.Interactive.Type)
	assert.Equal(t, "View Product", newHistoryMsg.Message.Interactive.Action.Name)
	assert.Len(t, newHistoryMsg.Message.Interactive.Action.Sections, 1)
	assert.Len(t, newHistoryMsg.Message.Interactive.Action.Sections[0].ProductItems, 1)
	assert.Equal(t, "product-123", newHistoryMsg.Message.Interactive.Action.Sections[0].ProductItems[0].ProductRetailerID)
	assert.Equal(t, "https://example.com/product/tv", newHistoryMsg.Message.Interactive.Action.Sections[0].ProductItems[0].ProductURL)
	assert.Equal(t, "cat-9", newHistoryMsg.Message.Interactive.Action.Sections[0].ProductItems[0].Extra["catalog_id"])
	assert.Equal(t, 1, newHistoryMsg.Message.Interactive.Action.Sections[0].ProductItems[0].Extra["position"])
}

func TestNewMessagePayloadWithOrder(t *testing.T) {
	now := fmt.Sprint(time.Now().Unix())
	order := &Order{
		Text: "Order placed",
		ProductItems: []ProductItem{
			{
				ProductRetailerID: "product-001",
				Name:              "Smart TV 50\"",
				Price:             "2999.90",
				Currency:          "BRL",
				SellerID:          "seller-001",
				Quantity:          2,
				Extra: map[string]any{
					"line_note": "gift wrap",
				},
			},
			{
				ProductRetailerID: "product-002",
				Name:              "Smartphone",
				Price:             "1999.90",
				Currency:          "BRL",
				SellerID:          "seller-002",
				Quantity:          1,
			},
		},
	}
	msg := Message{
		Type:      "order",
		Timestamp: now,
		Order:     order,
	}

	newHistoryMsg := NewMessagePayload("outcoming", "text:123456", "bba8457f-69b2-4f67-bab5-72c463fa7701", msg)
	assert.NotNil(t, newHistoryMsg.Message.Order)
	assert.Equal(t, "Order placed", newHistoryMsg.Message.Order.Text)
	assert.Len(t, newHistoryMsg.Message.Order.ProductItems, 2)
	assert.Equal(t, "product-001", newHistoryMsg.Message.Order.ProductItems[0].ProductRetailerID)
	assert.Equal(t, "2999.90", newHistoryMsg.Message.Order.ProductItems[0].Price)
	assert.Equal(t, 2, newHistoryMsg.Message.Order.ProductItems[0].Quantity)
	assert.Equal(t, "gift wrap", newHistoryMsg.Message.Order.ProductItems[0].Extra["line_note"])
	assert.Nil(t, newHistoryMsg.Message.Order.ProductItems[1].Extra)
}

func TestInteractiveMessageOmitEmptyOnMarshal(t *testing.T) {
	msg := Message{
		Type:      "text",
		Timestamp: fmt.Sprint(time.Now().Unix()),
		Text:      "hello",
	}
	b, err := json.Marshal(msg)
	assert.NoError(t, err)
	jsonStr := string(b)
	assert.False(t, strings.Contains(jsonStr, "\"interactive\""))
	assert.False(t, strings.Contains(jsonStr, "\"order\""))
}

func TestInteractiveMessageMarshalUnmarshal(t *testing.T) {
	externalPayload := `{
		"type":"message",
		"to":"371298371241",
		"from":"250788383383",
		"message":{
			"type":"interactive",
			"timestamp":"1616700878",
			"text":"Check out this product!",
			"interactive":{
				"type":"product_list",
				"action":{
					"sections":[
						{
							"title":"Product Name",
							"product_items":[
								{
									"product_retailer_id":"product-123",
									"name":"Smart TV 50\"",
									"price":"2999.90",
									"image":"https://example.com/tv.jpg",
									"description":"Smart TV 4K 50 inches",
									"seller_id":"seller-001",
									"product_url":"https://example.com/product/tv",
									"extra":{
										"ref":"abc",
										"score":0.95
									}
								}
							]
						}
					],
					"name":"View Product"
				}
			}
		},
		"channel_uuid":"8eb23e93-5ecb-45ba-b726-3b064e0c568c"
	}`

	type IncomingPayload struct {
		Type        string  `json:"type"`
		To          string  `json:"to"`
		From        string  `json:"from"`
		Message     Message `json:"message"`
		ChannelUUID string  `json:"channel_uuid"`
	}

	var payload IncomingPayload
	err := json.Unmarshal([]byte(externalPayload), &payload)
	assert.NoError(t, err)
	assert.Equal(t, "interactive", payload.Message.Type)
	assert.NotNil(t, payload.Message.Interactive)
	assert.Equal(t, "product_list", payload.Message.Interactive.Type)
	assert.Equal(t, "View Product", payload.Message.Interactive.Action.Name)
	assert.Len(t, payload.Message.Interactive.Action.Sections, 1)
	assert.Equal(t, "Product Name", payload.Message.Interactive.Action.Sections[0].Title)
	assert.Len(t, payload.Message.Interactive.Action.Sections[0].ProductItems, 1)
	assert.Equal(t, "product-123", payload.Message.Interactive.Action.Sections[0].ProductItems[0].ProductRetailerID)
	assert.Equal(t, "Smart TV 50\"", payload.Message.Interactive.Action.Sections[0].ProductItems[0].Name)
	assert.Equal(t, "https://example.com/product/tv", payload.Message.Interactive.Action.Sections[0].ProductItems[0].ProductURL)
	assert.NotNil(t, payload.Message.Interactive.Action.Sections[0].ProductItems[0].Extra)
	assert.Equal(t, "abc", payload.Message.Interactive.Action.Sections[0].ProductItems[0].Extra["ref"])
	assert.InDelta(t, 0.95, payload.Message.Interactive.Action.Sections[0].ProductItems[0].Extra["score"], 0.0001)
}

func TestOrderMessageMarshal(t *testing.T) {
	order := &Order{
		Text: "Order placed",
		ProductItems: []ProductItem{
			{
				ProductRetailerID: "product-001",
				Name:              "Smart TV 50\"",
				Price:             "2999.90",
				SalePrice:         "2599.90",
				Currency:          "BRL",
				SellerID:          "seller-001",
				Quantity:          2,
				Extra: map[string]any{
					"warranty_months": 12,
				},
			},
		},
	}
	msg := Message{
		Type:      "order",
		Timestamp: "1616700878",
		Order:     order,
	}

	b, err := json.Marshal(msg)
	assert.NoError(t, err)
	jsonStr := string(b)

	// Verify expected fields are present
	assert.True(t, strings.Contains(jsonStr, "\"order\""))
	assert.True(t, strings.Contains(jsonStr, "\"product_retailer_id\":\"product-001\""))
	assert.True(t, strings.Contains(jsonStr, "\"price\":\"2999.90\""))
	assert.True(t, strings.Contains(jsonStr, "\"quantity\":2"))
	assert.True(t, strings.Contains(jsonStr, "\"extra\""))
	assert.True(t, strings.Contains(jsonStr, "\"warranty_months\":12"))
}

func TestProductItemExtraJSONRoundTrip(t *testing.T) {
	item := ProductItem{
		ProductRetailerID: "p1",
		Name:              "Item",
		Extra: map[string]any{
			"k_str":   "v",
			"k_num":   42,
			"k_float": 1.5,
		},
	}
	b, err := json.Marshal(item)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(b), "\"extra\""))

	var decoded ProductItem
	err = json.Unmarshal(b, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, "p1", decoded.ProductRetailerID)
	assert.NotNil(t, decoded.Extra)
	assert.Equal(t, "v", decoded.Extra["k_str"])
	assert.Equal(t, float64(42), decoded.Extra["k_num"])
	assert.Equal(t, 1.5, decoded.Extra["k_float"])
}

func TestProductItemExtraOmitEmptyOnMarshal(t *testing.T) {
	item := ProductItem{
		ProductRetailerID: "p1",
		Name:              "No extra map",
	}
	b, err := json.Marshal(item)
	assert.NoError(t, err)
	assert.False(t, strings.Contains(string(b), "\"extra\""))
}

func TestInteractiveMessageWithHeaderAndFooter(t *testing.T) {
	externalPayload := `{
		"type":"message",
		"to":"169808223086@",
		"from":"Weni Web Chat - #519",
		"message":{
			"type":"interactive",
			"timestamp":"1770302350",
			"text":"Oie",
			"interactive":{
				"type":"product_list",
				"header":{
					"type":"text",
					"text":"Coleção Workshirt"
				},
				"footer":{
					"text":"Todas com proteção UV e detalhes exclusivos :white_check_mark:"
				},
				"action":{
					"sections":[
						{
							"title":"Workshirt Camisa Titan Coyote",
							"product_items":[
								{
									"product_retailer_id":"5371#1",
									"name":"Blusa 2",
									"price":"10.00",
									"currency":"BRL",
									"image":"https://example.com/img-mock.jpg",
									"description":"aaaa",
									"seller_id":"1"
								},
								{
									"product_retailer_id":"10#bravtexgrocerystore02",
									"name":"Blusa 1",
									"price":"10.90",
									"currency":"BRL",
									"image":"https://example.com/img-mock.jpg",
									"description":"bbbbb",
									"seller_id":"1"
								}
							]
						}
					]
				}
			}
		},
		"channel_uuid":"b41808a0-71fd-4e43-9bc7-223b9a4c30a2"
	}`

	type IncomingPayload struct {
		Type        string  `json:"type"`
		To          string  `json:"to"`
		From        string  `json:"from"`
		Message     Message `json:"message"`
		ChannelUUID string  `json:"channel_uuid"`
	}

	var payload IncomingPayload
	err := json.Unmarshal([]byte(externalPayload), &payload)
	assert.NoError(t, err)
	assert.Equal(t, "interactive", payload.Message.Type)
	assert.NotNil(t, payload.Message.Interactive)
	assert.Equal(t, "product_list", payload.Message.Interactive.Type)

	// Verify header
	assert.NotNil(t, payload.Message.Interactive.Header)
	assert.Equal(t, "text", payload.Message.Interactive.Header.Type)
	assert.Equal(t, "Coleção Workshirt", payload.Message.Interactive.Header.Text)

	// Verify footer
	assert.NotNil(t, payload.Message.Interactive.Footer)
	assert.Equal(t, "Todas com proteção UV e detalhes exclusivos :white_check_mark:", payload.Message.Interactive.Footer.Text)

	// Verify action and sections
	assert.Len(t, payload.Message.Interactive.Action.Sections, 1)
	assert.Equal(t, "Workshirt Camisa Titan Coyote", payload.Message.Interactive.Action.Sections[0].Title)
	assert.Len(t, payload.Message.Interactive.Action.Sections[0].ProductItems, 2)
	assert.Equal(t, "5371#1", payload.Message.Interactive.Action.Sections[0].ProductItems[0].ProductRetailerID)
	assert.Equal(t, "Blusa 2", payload.Message.Interactive.Action.Sections[0].ProductItems[0].Name)
	assert.Equal(t, "10.00", payload.Message.Interactive.Action.Sections[0].ProductItems[0].Price)
	assert.Equal(t, "BRL", payload.Message.Interactive.Action.Sections[0].ProductItems[0].Currency)
	assert.Equal(t, "10#bravtexgrocerystore02", payload.Message.Interactive.Action.Sections[0].ProductItems[1].ProductRetailerID)
	assert.Equal(t, "Blusa 1", payload.Message.Interactive.Action.Sections[0].ProductItems[1].Name)
	assert.Equal(t, "10.90", payload.Message.Interactive.Action.Sections[0].ProductItems[1].Price)
}
