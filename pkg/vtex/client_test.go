package vtex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestClient(server *httptest.Server) *Client {
	return &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}
}

func TestAddOrUpdateCartItem_AddNewItem(t *testing.T) {
	var addBody addItemsRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(OrderForm{Items: []OrderFormItem{}})

		case r.Method == http.MethodPost && r.URL.Path == "/api/checkout/pub/orderForm/of123/items":
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &addBody)
			w.WriteHeader(http.StatusOK)

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.AddOrUpdateCartItem(context.Background(), "teststore", "of123", "prod_1", "seller_a")

	assert.NoError(t, err)
	assert.Len(t, addBody.OrderItems, 1)
	assert.Equal(t, "prod_1", addBody.OrderItems[0].ID)
	assert.Equal(t, "seller_a", addBody.OrderItems[0].Seller)
	assert.Equal(t, 1, addBody.OrderItems[0].Quantity)
}

func TestAddOrUpdateCartItem_UpdateExistingItem(t *testing.T) {
	var updateBody updateItemsRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(OrderForm{
				Items: []OrderFormItem{
					{ID: "other_prod", Quantity: 1},
					{ID: "prod_1", Quantity: 2},
				},
			})

		case r.Method == http.MethodPost && r.URL.Path == "/api/checkout/pub/orderForm/of123/items/update":
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &updateBody)
			w.WriteHeader(http.StatusOK)

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.AddOrUpdateCartItem(context.Background(), "teststore", "of123", "prod_1", "seller_a")

	assert.NoError(t, err)
	assert.Len(t, updateBody.OrderItems, 1)
	assert.Equal(t, 1, updateBody.OrderItems[0].Index)
	assert.Equal(t, 3, updateBody.OrderItems[0].Quantity)
}

func TestAddOrUpdateCartItem_GetOrderFormError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.AddOrUpdateCartItem(context.Background(), "teststore", "of123", "prod_1", "seller_a")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get order form failed with status 500")
}

func TestAddOrUpdateCartItem_AddItemError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(OrderForm{Items: []OrderFormItem{}})

		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"bad request"}`))

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.AddOrUpdateCartItem(context.Background(), "teststore", "of123", "prod_1", "seller_a")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cart operation failed with status 400")
}

func TestAddOrUpdateCartItem_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.AddOrUpdateCartItem(context.Background(), "teststore", "of123", "prod_1", "seller_a")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse order form response")
}

func TestAddOrUpdateCartItem_InvalidAccount(t *testing.T) {
	c := &Client{httpClient: &http.Client{}}

	tests := []struct {
		name    string
		account string
	}{
		{"fragment injection", "attacker.com/path#"},
		{"slash injection", "attacker.com/evil"},
		{"dot injection", "evil.attacker.com"},
		{"space", "has space"},
		{"empty after trim", ""},
		{"starts with hyphen", "-invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.AddOrUpdateCartItem(context.Background(), tt.account, "of123", "prod_1", "seller_a")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid account name")
		})
	}
}

func TestAddOrUpdateCartItem_InvalidOrderFormID(t *testing.T) {
	c := &Client{httpClient: &http.Client{}}

	tests := []struct {
		name        string
		orderFormID string
	}{
		{"path traversal", "../../etc/passwd"},
		{"fragment injection", "form#evil"},
		{"query injection", "form?evil=1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.AddOrUpdateCartItem(context.Background(), "teststore", tt.orderFormID, "prod_1", "seller_a")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid order form ID")
		})
	}
}

func TestOrderFormURL_WithBaseURL(t *testing.T) {
	c := &Client{baseURL: "http://localhost:8080"}
	url := c.orderFormURL("teststore", "of123")
	assert.Equal(t, "http://localhost:8080/api/checkout/pub/orderForm/of123", url)
}

func TestOrderFormURL_WithoutBaseURL(t *testing.T) {
	c := &Client{}
	url := c.orderFormURL("teststore", "of123")
	assert.Equal(t, "https://teststore.vtexcommercestable.com.br/api/checkout/pub/orderForm/of123", url)
}
