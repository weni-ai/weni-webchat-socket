package vtex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	log "github.com/sirupsen/logrus"
)

var safeSlugRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

// IClient abstracts VTEX cart operations for testability.
type IClient interface {
	AddOrUpdateCartItem(ctx context.Context, vtexAccount, orderFormID, itemID, seller string) error
}

// OrderFormItem represents a single item in the VTEX order form.
type OrderFormItem struct {
	ID       string `json:"id"`
	Quantity int    `json:"quantity"`
}

// OrderForm represents the VTEX order form response.
type OrderForm struct {
	Items []OrderFormItem `json:"items"`
}

type addOrderItem struct {
	Quantity int    `json:"quantity"`
	Seller   string `json:"seller"`
	ID       string `json:"id"`
}

type addItemsRequest struct {
	OrderItems []addOrderItem `json:"orderItems"`
}

type updateOrderItem struct {
	Quantity int `json:"quantity"`
	Index    int `json:"index"`
}

type updateItemsRequest struct {
	OrderItems []updateOrderItem `json:"orderItems"`
}

// Client communicates with the VTEX Checkout API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

const defaultTimeout = 30 * time.Second

// NewClient creates a new VTEX client.
func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: defaultTimeout}}
}

func (c *Client) orderFormURL(vtexAccount, orderFormID string) string {
	if c.baseURL != "" {
		return fmt.Sprintf("%s/api/checkout/pub/orderForm/%s", c.baseURL, orderFormID)
	}
	return fmt.Sprintf(
		"https://%s.vtexcommercestable.com.br/api/checkout/pub/orderForm/%s",
		vtexAccount, orderFormID,
	)
}

func (c *Client) getOrderForm(ctx context.Context, vtexAccount, orderFormID string) (*OrderForm, error) {
	reqURL := c.orderFormURL(vtexAccount, orderFormID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("vtex: create get request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vtex: get order form: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vtex: read order form response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{
			"vtex_account":  vtexAccount,
			"order_form_id": orderFormID,
			"status_code":   resp.StatusCode,
			"body":          string(body),
		}).Error("VTEX get order form failed")
		return nil, fmt.Errorf("vtex: get order form failed with status %d", resp.StatusCode)
	}

	var orderForm OrderForm
	if err := json.Unmarshal(body, &orderForm); err != nil {
		return nil, fmt.Errorf("vtex: parse order form response: %w", err)
	}

	return &orderForm, nil
}

func (c *Client) postJSON(ctx context.Context, reqURL string, payload interface{}) error {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("vtex: marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("vtex: create post request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vtex: post request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.WithFields(log.Fields{
			"url":         reqURL,
			"status_code": resp.StatusCode,
			"body":        string(body),
		}).Error("VTEX cart operation failed")
		return fmt.Errorf("vtex: cart operation failed with status %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) addItem(ctx context.Context, vtexAccount, orderFormID, itemID, seller string) error {
	reqURL := c.orderFormURL(vtexAccount, orderFormID) + "/items"
	body := addItemsRequest{
		OrderItems: []addOrderItem{
			{Quantity: 1, Seller: seller, ID: itemID},
		},
	}
	return c.postJSON(ctx, reqURL, body)
}

func (c *Client) updateItem(ctx context.Context, vtexAccount, orderFormID string, index, newQuantity int) error {
	reqURL := c.orderFormURL(vtexAccount, orderFormID) + "/items/update"
	body := updateItemsRequest{
		OrderItems: []updateOrderItem{
			{Quantity: newQuantity, Index: index},
		},
	}
	return c.postJSON(ctx, reqURL, body)
}

// AddOrUpdateCartItem fetches the current cart, then adds the item if it is
// not present or increments its quantity if it already exists.
func (c *Client) AddOrUpdateCartItem(ctx context.Context, vtexAccount, orderFormID, itemID, seller string) error {
	if !safeSlugRe.MatchString(vtexAccount) {
		return fmt.Errorf("vtex: invalid account name %q", vtexAccount)
	}
	if !safeSlugRe.MatchString(orderFormID) {
		return fmt.Errorf("vtex: invalid order form ID %q", orderFormID)
	}

	orderForm, err := c.getOrderForm(ctx, vtexAccount, orderFormID)
	if err != nil {
		return err
	}

	for i, item := range orderForm.Items {
		if item.ID == itemID {
			return c.updateItem(ctx, vtexAccount, orderFormID, i, item.Quantity+1)
		}
	}

	return c.addItem(ctx, vtexAccount, orderFormID, itemID, seller)
}
