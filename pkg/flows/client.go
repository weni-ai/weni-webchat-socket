package flows

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ilhasoft/wwcs/pkg/jwt"
	log "github.com/sirupsen/logrus"
)

// IClient is the interface for the flows client API
type IClient interface {
	ContactHasOpenTicket(channelUUID string, contactURN string) (bool, error)
	GetChannelAllowedDomains(channelUUID string) ([]string, error)
	GetChannelProjectLanguage(channelUUID string) (string, error)
	UpdateContactFields(channelUUID string, contactURN string, contactFields map[string]interface{}) error
}

// Client is the client implementation for the flows API
type Client struct {
	BaseURL    string `json:"base_url"`
	httpClient *http.Client
}

// NewClient creates a new client for the flows API.
// If jwtSigner is provided, all requests will include JWT authentication.
func NewClient(baseURL string, jwtSigner *jwt.Signer) *Client {
	return &Client{
		BaseURL:    baseURL,
		httpClient: &http.Client{Transport: newJWTTransport(jwtSigner)},
	}
}

// doGet performs an authenticated GET request
func (c *Client) doGet(url string, channelUUID string) (*http.Response, error) {
	ctx := withChannelUUID(context.Background(), channelUUID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

// doJSON performs an authenticated request with a JSON body
func (c *Client) doJSON(method, url string, body interface{}, channelUUID string) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	ctx := withChannelUUID(context.Background(), channelUUID)
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(req)
}

// ContactHasOpenTicket checks if a contact has an open ticket.
// It returns true if the contact has an open ticket, false otherwise.
func (c *Client) ContactHasOpenTicket(channelUUID string, contactURN string) (bool, error) {
	url := fmt.Sprintf("%s/api/v2/internals/contact_has_open_ticket?contact_urn=ext:%s", c.BaseURL, contactURN)

	resp, err := c.doGet(url, channelUUID)
	if err != nil {
		log.WithFields(log.Fields{
			"channel_uuid": channelUUID,
			"contact_urn":  contactURN,
			"url":          url,
		}).WithError(err).Error("flows API: HTTP request failed for ContactHasOpenTicket")
		return false, err
	}
	defer resp.Body.Close()

	// Read body first to enable connection reuse
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithFields(log.Fields{
			"channel_uuid": channelUUID,
			"contact_urn":  contactURN,
			"status_code":  resp.StatusCode,
		}).WithError(err).Error("flows API: failed to read response body for ContactHasOpenTicket")
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{
			"channel_uuid":  channelUUID,
			"contact_urn":   contactURN,
			"status_code":   resp.StatusCode,
			"response_body": string(bodyBytes),
		}).Error("flows API: non-200 response for ContactHasOpenTicket")
		return false, fmt.Errorf("failed to get contact has open ticket, status code: %d", resp.StatusCode)
	}

	return strings.Contains(string(bodyBytes), "true"), nil
}

// GetChannelAllowedDomains gets the allowed domains for a channel.
// It returns a list of allowed domains.
func (c *Client) GetChannelAllowedDomains(channelUUID string) ([]string, error) {
	url := fmt.Sprintf("%s/api/v2/internals/channel_allowed_domains?channel=%s", c.BaseURL, channelUUID)

	resp, err := c.doGet(url, channelUUID)
	if err != nil {
		log.WithFields(log.Fields{
			"channel_uuid": channelUUID,
			"url":          url,
		}).WithError(err).Error("flows API: HTTP request failed for GetChannelAllowedDomains")
		return nil, err
	}
	defer resp.Body.Close()

	// Read body first to enable connection reuse
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithFields(log.Fields{
			"channel_uuid": channelUUID,
			"status_code":  resp.StatusCode,
		}).WithError(err).Error("flows API: failed to read response body for GetChannelAllowedDomains")
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{
			"channel_uuid":  channelUUID,
			"status_code":   resp.StatusCode,
			"response_body": string(bodyBytes),
		}).Error("flows API: non-200 response for GetChannelAllowedDomains")
		return nil, fmt.Errorf("failed to get channel allowed domains, status code: %d", resp.StatusCode)
	}

	var domains []string
	if err := json.Unmarshal(bodyBytes, &domains); err != nil {
		log.WithFields(log.Fields{
			"channel_uuid":  channelUUID,
			"response_body": string(bodyBytes),
		}).WithError(err).Error("flows API: failed to unmarshal allowed domains response")
		return nil, err
	}

	return domains, nil
}

// GetChannelProjectLanguage gets the project language for a channel.
func (c *Client) GetChannelProjectLanguage(channelUUID string) (string, error) {
	url := fmt.Sprintf("%s/api/v2/projects/project_language?channel_uuid=%s", c.BaseURL, channelUUID)

	resp, err := c.doGet(url, channelUUID)
	if err != nil {
		log.WithFields(log.Fields{
			"channel_uuid": channelUUID,
			"url":          url,
		}).WithError(err).Error("flows API: HTTP request failed for GetChannelProjectLanguage")
		return "", err
	}
	defer resp.Body.Close()

	// Read body first to enable connection reuse
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithFields(log.Fields{
			"channel_uuid": channelUUID,
			"status_code":  resp.StatusCode,
		}).WithError(err).Error("flows API: failed to read response body for GetChannelProjectLanguage")
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{
			"channel_uuid":  channelUUID,
			"status_code":   resp.StatusCode,
			"response_body": string(bodyBytes),
		}).Error("flows API: non-200 response for GetChannelProjectLanguage")
		return "", fmt.Errorf("failed to get channel project language, status code: %d", resp.StatusCode)
	}

	var response struct {
		Language string `json:"language"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		log.WithFields(log.Fields{
			"channel_uuid":  channelUUID,
			"response_body": string(bodyBytes),
		}).WithError(err).Error("flows API: failed to unmarshal project language response")
		return "", err
	}

	return response.Language, nil
}

// UpdateContactFields updates the contact fields for a contact.
func (c *Client) UpdateContactFields(channelUUID string, contactURN string, contactFields map[string]interface{}) error {
	url := fmt.Sprintf("%s/api/v2/internals/update_contacts_fields", c.BaseURL)

	body := map[string]interface{}{
		"channel_uuid":   channelUUID,
		"contact_urn":    fmt.Sprintf("ext:%s", contactURN),
		"contact_fields": contactFields,
	}

	resp, err := c.doJSON(http.MethodPatch, url, body, channelUUID)
	if err != nil {
		log.WithFields(log.Fields{
			"channel_uuid": channelUUID,
			"contact_urn":  contactURN,
			"url":          url,
		}).WithError(err).Error("flows API: HTTP request failed for UpdateContactFields")
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithFields(log.Fields{
			"channel_uuid": channelUUID,
			"contact_urn":  contactURN,
			"status_code":  resp.StatusCode,
		}).WithError(err).Error("flows API: failed to read response body for UpdateContactFields")
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{
			"channel_uuid":  channelUUID,
			"contact_urn":   contactURN,
			"status_code":   resp.StatusCode,
			"response_body": string(bodyBytes),
		}).Error("flows API: non-200 response for UpdateContactFields")
		return fmt.Errorf("failed to update contact fields, status code: %d", resp.StatusCode)
	}

	return nil
}
