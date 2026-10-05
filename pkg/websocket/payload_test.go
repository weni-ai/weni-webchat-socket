package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ilhasoft/wwcs/pkg/history"
)

func TestAsOutgoingMessage_OrderWithoutText(t *testing.T) {
	payload := OutgoingPayload{
		Type: "message",
		From: "2345678",
		Message: Message{
			Type: "order",
			Order: &history.Order{
				ProductItems: []history.ProductItem{
					{
						ProductRetailerID: "product-001",
						Name:              "Smart TV 50\"",
						Price:             "2999.90",
						SalePrice:         "2599.90",
						Currency:          "BRL",
						SellerID:          "seller-001",
						Quantity:          2,
						ProductURL:        "https://loja.com/tv",
						Image:             "https://foo.bar/image.png",
						Description:       "Smart TV description",
						Extra:             map[string]any{"line_note": "gift wrap"},
					},
				},
			},
		},
	}

	outgoing, err := payload.AsOutgoingMessage()
	if err != nil {
		t.Fatalf("AsOutgoingMessage() error = %v", err)
	}

	if outgoing.Message.Order == nil {
		t.Fatal("expected order payload")
	}
	if outgoing.Message.Order.Text != "" {
		t.Fatalf("expected empty order text, got %q", outgoing.Message.Order.Text)
	}
	if len(outgoing.Message.Order.ProductItems) != 1 {
		t.Fatalf("expected one product item, got %d", len(outgoing.Message.Order.ProductItems))
	}

	item := outgoing.Message.Order.ProductItems[0]
	if item.Price != "2999.90" {
		t.Fatalf("expected price preserved, got %q", item.Price)
	}
	if item.ProductURL != "https://loja.com/tv" {
		t.Fatalf("expected product_url preserved, got %q", item.ProductURL)
	}
	if item.Extra["line_note"] != "gift wrap" {
		t.Fatalf("expected extra preserved, got %#v", item.Extra)
	}
	if item.Image != "" || item.Description != "" {
		t.Fatalf("expected image and description stripped, got image=%q description=%q", item.Image, item.Description)
	}

	body, err := json.Marshal(outgoing)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(body), `"text":"should be stripped"`) {
		t.Fatalf("serialized payload should not contain message text: %s", body)
	}
}

func TestFormatOutgoingPayload_OrderWithoutMessageText(t *testing.T) {
	payload := OutgoingPayload{
		Type: "message",
		From: "2345678",
		Message: Message{
			Type: "order",
			Order: &history.Order{
				ProductItems: []history.ProductItem{
					{
						ProductRetailerID: "product-001",
						Name:              "Smart TV 50\"",
						Price:             "2999.90",
						Currency:          "BRL",
						SellerID:          "seller-001",
						Quantity:          2,
					},
				},
			},
		},
	}

	presenter, err := formatOutgoingPayload(payload)
	if err != nil {
		t.Fatalf("formatOutgoingPayload() error = %v", err)
	}

	if presenter.Message.Text != "" {
		t.Fatalf("expected empty message text, got %q", presenter.Message.Text)
	}
	if presenter.Message.Order == nil || len(presenter.Message.Order.ProductItems) != 1 {
		t.Fatalf("expected order with one product item, got %#v", presenter.Message.Order)
	}
}

func TestToCallback_OrderPayload(t *testing.T) {
	payload := OutgoingPayload{
		Type: "message",
		From: "2345678",
		Message: Message{
			Type: "order",
			Order: &history.Order{
				ProductItems: []history.ProductItem{
					{
						ProductRetailerID: "product-001",
						Name:              "Smart TV 50\"",
						Price:             "2999.90",
						Currency:          "BRL",
						SellerID:          "seller-001",
						Quantity:          2,
					},
				},
			},
		},
	}

	presenter, err := formatOutgoingPayload(payload)
	if err != nil {
		t.Fatalf("formatOutgoingPayload() error = %v", err)
	}

	outgoing, err := presenter.AsOutgoingMessage()
	if err != nil {
		t.Fatalf("AsOutgoingMessage() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	body, err := ToCallback(server.URL, outgoing)
	if err != nil {
		t.Fatalf("ToCallback() error = %v", err)
	}

	var sent OutgoingPayload
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if sent.Type != "message" {
		t.Fatalf("expected type message, got %q", sent.Type)
	}
	if sent.From != "2345678" {
		t.Fatalf("expected from preserved, got %q", sent.From)
	}
	if sent.Message.Type != "order" {
		t.Fatalf("expected order message type, got %q", sent.Message.Type)
	}
	if sent.Message.Order == nil || len(sent.Message.Order.ProductItems) != 1 {
		t.Fatalf("expected one product item in callback payload, got %#v", sent.Message.Order)
	}
	if sent.Message.Order.ProductItems[0].Price != "2999.90" {
		t.Fatalf("expected webchat price field, got %q", sent.Message.Order.ProductItems[0].Price)
	}
}
