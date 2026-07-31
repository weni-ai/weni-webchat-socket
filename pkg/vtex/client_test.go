package vtex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestClient(server *httptest.Server) *Client {
	return &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}
}

func singleCartItem(id, seller string, quantity int) []CartItemInput {
	return []CartItemInput{{ID: id, Seller: seller, Quantity: quantity}}
}

func TestAddOrUpdateCartItems_AddNewItem(t *testing.T) {
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
	err := client.AddOrUpdateCartItems(context.Background(), "teststore", "of123", singleCartItem("prod_1", "seller_a", 1))

	assert.NoError(t, err)
	assert.Len(t, addBody.OrderItems, 1)
	assert.Equal(t, "prod_1", addBody.OrderItems[0].ID)
	assert.Equal(t, "seller_a", addBody.OrderItems[0].Seller)
	assert.Equal(t, 1, addBody.OrderItems[0].Quantity)
}

func TestAddOrUpdateCartItems_UpdateExistingItem(t *testing.T) {
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
	err := client.AddOrUpdateCartItems(context.Background(), "teststore", "of123", singleCartItem("prod_1", "seller_a", 1))

	assert.NoError(t, err)
	assert.Len(t, updateBody.OrderItems, 1)
	assert.Equal(t, 1, updateBody.OrderItems[0].Index)
	assert.Equal(t, 3, updateBody.OrderItems[0].Quantity)
}

func TestAddOrUpdateCartItems_GetOrderFormError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.AddOrUpdateCartItems(context.Background(), "teststore", "of123", singleCartItem("prod_1", "seller_a", 1))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get order form failed with status 500")
}

func TestAddOrUpdateCartItems_AddItemError(t *testing.T) {
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
	err := client.AddOrUpdateCartItems(context.Background(), "teststore", "of123", singleCartItem("prod_1", "seller_a", 1))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cart operation failed with status 400")
}

func TestAddOrUpdateCartItems_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.AddOrUpdateCartItems(context.Background(), "teststore", "of123", singleCartItem("prod_1", "seller_a", 1))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse order form response")
}

func TestAddOrUpdateCartItems_InvalidAccount(t *testing.T) {
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
			err := c.AddOrUpdateCartItems(context.Background(), tt.account, "of123", singleCartItem("prod_1", "seller_a", 1))
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid account name")
		})
	}
}

func TestAddOrUpdateCartItems_InvalidOrderFormID(t *testing.T) {
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
			err := c.AddOrUpdateCartItems(context.Background(), "teststore", tt.orderFormID, singleCartItem("prod_1", "seller_a", 1))
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid order form ID")
		})
	}
}

func TestAddOrUpdateCartItems_AddNewItemWithCustomQuantity(t *testing.T) {
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
	err := client.AddOrUpdateCartItems(context.Background(), "teststore", "of123", singleCartItem("prod_1", "seller_a", 3))

	assert.NoError(t, err)
	assert.Len(t, addBody.OrderItems, 1)
	assert.Equal(t, 3, addBody.OrderItems[0].Quantity)
}

func TestAddOrUpdateCartItems_UpdateExistingItemWithCustomQuantity(t *testing.T) {
	var updateBody updateItemsRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(OrderForm{
				Items: []OrderFormItem{{ID: "prod_1", Quantity: 2}},
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
	err := client.AddOrUpdateCartItems(context.Background(), "teststore", "of123", singleCartItem("prod_1", "seller_a", 3))

	assert.NoError(t, err)
	assert.Len(t, updateBody.OrderItems, 1)
	assert.Equal(t, 0, updateBody.OrderItems[0].Index)
	assert.Equal(t, 5, updateBody.OrderItems[0].Quantity)
}

func TestAddOrUpdateCartItems_BatchMixedAddAndUpdate(t *testing.T) {
	var addBody addItemsRequest
	var updateBody updateItemsRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(OrderForm{
				Items: []OrderFormItem{
					{ID: "prod_1", Quantity: 2},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/checkout/pub/orderForm/of123/items/update":
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &updateBody)
			w.WriteHeader(http.StatusOK)
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
	err := client.AddOrUpdateCartItems(context.Background(), "teststore", "of123", []CartItemInput{
		{ID: "prod_1", Seller: "seller_a", Quantity: 2},
		{ID: "prod_2", Seller: "seller_b", Quantity: 1},
	})

	assert.NoError(t, err)
	assert.Len(t, updateBody.OrderItems, 1)
	assert.Equal(t, 4, updateBody.OrderItems[0].Quantity)
	assert.Len(t, addBody.OrderItems, 1)
	assert.Equal(t, "prod_2", addBody.OrderItems[0].ID)
	assert.Equal(t, 1, addBody.OrderItems[0].Quantity)
}

func TestAddOrUpdateCartItems_EmptyItems(t *testing.T) {
	client := &Client{httpClient: &http.Client{}}
	err := client.AddOrUpdateCartItems(context.Background(), "teststore", "of123", nil)
	assert.NoError(t, err)
}

func TestUpdateMarketingData_UTMSourceSuccess(t *testing.T) {
	var receivedBody MarketingData
	var receivedPath string
	var receivedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(OrderForm{})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/attachments/marketingData"):
			receivedPath = r.URL.Path
			receivedMethod = r.Method
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedBody)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.UpdateMarketingData(context.Background(), "teststore", "of123", "cx_shopping_assistant_cart", false)

	assert.NoError(t, err)
	assert.Equal(t, http.MethodPost, receivedMethod)
	assert.Equal(t, "/api/checkout/pub/orderForm/of123/attachments/marketingData", receivedPath)
	assert.NotNil(t, receivedBody.UTMSource)
	assert.Equal(t, "cx_shopping_assistant_cart", *receivedBody.UTMSource)
	assert.Equal(t, []string{}, receivedBody.MarketingTags)
}

func TestUpdateMarketingData_UTMSourcePreservesExistingMarketingData(t *testing.T) {
	campaign := "summer"
	existingSource := "google"
	var receivedBody MarketingData

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(OrderForm{
				MarketingData: &MarketingData{
					MarketingTags: []string{"cx_shopping_assistant_conv_starter"},
					UTMCampaign:   &campaign,
					UTMSource:     &existingSource,
				},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/attachments/marketingData"):
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedBody)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.UpdateMarketingData(context.Background(), "teststore", "of123", "cx_shopping_assistant", false)

	assert.NoError(t, err)
	assert.NotNil(t, receivedBody.UTMSource)
	assert.Equal(t, "cx_shopping_assistant", *receivedBody.UTMSource)
	assert.Equal(t, []string{"cx_shopping_assistant_conv_starter"}, receivedBody.MarketingTags)
	assert.Equal(t, &campaign, receivedBody.UTMCampaign)
}

func TestUpdateMarketingData_MarketingTags_MergesExisting(t *testing.T) {
	campaign := "summer"
	existingSource := "google"
	var receivedBody MarketingData

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(OrderForm{
				MarketingData: &MarketingData{
					MarketingTags: []string{"existing-tag"},
					UTMCampaign:   &campaign,
					UTMSource:     &existingSource,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/checkout/pub/orderForm/of123/attachments/marketingData":
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedBody)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.UpdateMarketingData(context.Background(), "teststore", "of123", "cx_shopping_assistant_cart", true)

	assert.NoError(t, err)
	assert.Equal(t, []string{"existing-tag", "cx_shopping_assistant_cart"}, receivedBody.MarketingTags)
	assert.Equal(t, &campaign, receivedBody.UTMCampaign)
	assert.Equal(t, &existingSource, receivedBody.UTMSource)
}

func TestUpdateMarketingData_MarketingTags_Deduplicates(t *testing.T) {
	var receivedBody MarketingData

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(OrderForm{
				MarketingData: &MarketingData{
					MarketingTags: []string{"cx_shopping_assistant_cart"},
				},
			})
		case r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedBody)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.UpdateMarketingData(context.Background(), "teststore", "of123", "cx_shopping_assistant_cart", true)

	assert.NoError(t, err)
	assert.Equal(t, []string{"cx_shopping_assistant_cart"}, receivedBody.MarketingTags)
}

func TestUpdateMarketingData_MarketingTags_EmptyMarketingData(t *testing.T) {
	var receivedBody MarketingData

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(OrderForm{})
		case r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedBody)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.UpdateMarketingData(context.Background(), "teststore", "of123", "cx_shopping_assistant", true)

	assert.NoError(t, err)
	assert.Equal(t, []string{"cx_shopping_assistant"}, receivedBody.MarketingTags)
}

func TestUpdateMarketingData_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(OrderForm{})
		default:
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"internal"}`))
		}
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.UpdateMarketingData(context.Background(), "teststore", "of123", "cx_shopping_assistant_cart", false)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cart operation failed with status 500")
}

func TestUpdateMarketingData_GetOrderFormError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.UpdateMarketingData(context.Background(), "teststore", "of123", "cx_shopping_assistant_cart", false)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get order form failed with status 500")
}

func TestUpdateMarketingData_InvalidInputs(t *testing.T) {
	c := &Client{httpClient: &http.Client{}}

	tests := []struct {
		name        string
		account     string
		orderFormID string
		errContains string
	}{
		{"invalid account", "evil.attacker.com", "of123", "invalid account name"},
		{"empty account", "", "of123", "invalid account name"},
		{"invalid order form", "teststore", "../../etc/passwd", "invalid order form ID"},
		{"empty order form", "teststore", "", "invalid order form ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.UpdateMarketingData(context.Background(), tt.account, tt.orderFormID, "cx_shopping_assistant_cart", false)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestMergeMarketingTag(t *testing.T) {
	assert.Equal(t, []string{"a"}, mergeMarketingTag([]string{}, "a"))
	assert.Equal(t, []string{"a", "b"}, mergeMarketingTag([]string{"a"}, "b"))
	assert.Equal(t, []string{"a"}, mergeMarketingTag([]string{"a"}, "a"))
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
