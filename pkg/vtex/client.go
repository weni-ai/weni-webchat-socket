package vtex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

var safeSlugRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

// CartItemInput represents an item to add or update in the VTEX cart.
type CartItemInput struct {
	ID       string
	Seller   string
	Quantity int
}

// CartItemResult represents an item quantity after a cart update.
type CartItemResult struct {
	ID       string
	Quantity int
}

// IClient abstracts VTEX cart operations for testability.
type IClient interface {
	AddOrUpdateCartItems(ctx context.Context, vtexAccount, orderFormID string, items []CartItemInput) ([]CartItemResult, error)
	UpdateMarketingData(ctx context.Context, vtexAccount, orderFormID, utmSource string, useMarketingTags bool) error
}

// OrderFormItem represents a single item in the VTEX order form.
type OrderFormItem struct {
	ID        string  `json:"id"`
	ProductID string  `json:"productId"`
	RefID     *string `json:"refId"`
	Quantity  int     `json:"quantity"`
}

// MarketingData represents the marketing data attachment on a VTEX order form.
type MarketingData struct {
	Coupon        *string  `json:"coupon,omitempty"`
	MarketingTags []string `json:"marketingTags"`
	UTMCampaign   *string  `json:"utmCampaign,omitempty"`
	UTMMedium     *string  `json:"utmMedium,omitempty"`
	UTMSource     *string  `json:"utmSource,omitempty"`
	UTMiCampaign  *string  `json:"utmiCampaign,omitempty"`
	UTMiPart      *string  `json:"utmiPart,omitempty"`
	UTMiPage      *string  `json:"utmipage,omitempty"`
}

// OrderForm represents the VTEX order form response.
type OrderForm struct {
	Items         []OrderFormItem `json:"items"`
	MarketingData *MarketingData  `json:"marketingData"`
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

func (c *Client) addItems(ctx context.Context, vtexAccount, orderFormID string, items []addOrderItem) error {
	if len(items) == 0 {
		return nil
	}
	reqURL := c.orderFormURL(vtexAccount, orderFormID) + "/items"
	return c.postJSON(ctx, reqURL, addItemsRequest{OrderItems: items})
}

func (c *Client) updateItems(ctx context.Context, vtexAccount, orderFormID string, items []updateOrderItem) error {
	if len(items) == 0 {
		return nil
	}
	reqURL := c.orderFormURL(vtexAccount, orderFormID) + "/items/update"
	return c.postJSON(ctx, reqURL, updateItemsRequest{OrderItems: items})
}

func normalizeCartItemID(id string) string {
	if idx := strings.Index(id, "#"); idx > 0 {
		return id[:idx]
	}
	return id
}

func orderFormItemMatchesInput(orderItem OrderFormItem, inputID string) bool {
	normalizedInputID := normalizeCartItemID(inputID)
	candidates := []string{orderItem.ID, orderItem.ProductID}
	if orderItem.RefID != nil {
		candidates = append(candidates, *orderItem.RefID)
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if candidate == inputID || normalizeCartItemID(candidate) == normalizedInputID {
			return true
		}
	}
	return false
}

func findOrderFormItem(orderItems []OrderFormItem, inputID string) (int, bool) {
	for i, item := range orderItems {
		if orderFormItemMatchesInput(item, inputID) {
			return i, true
		}
	}
	return 0, false
}

func mergeMarketingTag(existing []string, tag string) []string {
	for _, existingTag := range existing {
		if existingTag == tag {
			return existing
		}
	}
	return append(existing, tag)
}

// AddOrUpdateCartItems fetches the current cart, then adds new items or
// increments quantities for items that already exist. Updates are applied
// before adds so indices from the initial GET remain valid.
func (c *Client) AddOrUpdateCartItems(ctx context.Context, vtexAccount, orderFormID string, items []CartItemInput) ([]CartItemResult, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if !safeSlugRe.MatchString(vtexAccount) {
		return nil, fmt.Errorf("vtex: invalid account name %q", vtexAccount)
	}
	if !safeSlugRe.MatchString(orderFormID) {
		return nil, fmt.Errorf("vtex: invalid order form ID %q", orderFormID)
	}

	orderForm, err := c.getOrderForm(ctx, vtexAccount, orderFormID)
	if err != nil {
		return nil, err
	}

	var toUpdate []updateOrderItem
	var toAdd []addOrderItem
	results := make([]CartItemResult, 0, len(items))

	for _, input := range items {
		if index, exists := findOrderFormItem(orderForm.Items, input.ID); exists {
			cartItem := orderForm.Items[index]
			newQuantity := cartItem.Quantity + input.Quantity
			toUpdate = append(toUpdate, updateOrderItem{
				Index:    index,
				Quantity: newQuantity,
			})
			results = append(results, CartItemResult{
				ID:       input.ID,
				Quantity: newQuantity,
			})
			continue
		}

		toAdd = append(toAdd, addOrderItem{
			ID:       input.ID,
			Seller:   input.Seller,
			Quantity: input.Quantity,
		})
		results = append(results, CartItemResult{
			ID:       input.ID,
			Quantity: input.Quantity,
		})
	}

	if err := c.updateItems(ctx, vtexAccount, orderFormID, toUpdate); err != nil {
		return nil, err
	}
	if err := c.addItems(ctx, vtexAccount, orderFormID, toAdd); err != nil {
		return nil, err
	}
	return results, nil
}

// UpdateMarketingData fetches the current order form marketing data and posts
// the full attachment back to VTEX. When useMarketingTags is false, it sets
// utmSource while preserving existing fields. When true, it merges the UTM
// value into marketingTags without removing other data.
func (c *Client) UpdateMarketingData(ctx context.Context, vtexAccount, orderFormID, utmSource string, useMarketingTags bool) error {
	if !safeSlugRe.MatchString(vtexAccount) {
		return fmt.Errorf("vtex: invalid account name %q", vtexAccount)
	}
	if !safeSlugRe.MatchString(orderFormID) {
		return fmt.Errorf("vtex: invalid order form ID %q", orderFormID)
	}

	reqURL := c.orderFormURL(vtexAccount, orderFormID) + "/attachments/marketingData"

	orderForm, err := c.getOrderForm(ctx, vtexAccount, orderFormID)
	if err != nil {
		return err
	}

	marketingData := orderForm.MarketingData
	if marketingData == nil {
		marketingData = &MarketingData{}
	}
	if marketingData.MarketingTags == nil {
		marketingData.MarketingTags = []string{}
	}

	if useMarketingTags {
		marketingData.MarketingTags = mergeMarketingTag(marketingData.MarketingTags, utmSource)
	} else {
		marketingData.UTMSource = &utmSource
	}

	return c.postJSON(ctx, reqURL, marketingData)
}
