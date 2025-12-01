package flows

import (
	"bytes"
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
	ContactHasOpenTicket(string) (bool, error)
	GetChannelAllowedDomains(string) ([]string, error)
	GetChannelProjectLanguage(string) (string, error)
	UpdateContactFields(channelUUID string, contactURN string, contactFields map[string]interface{}) error
}

// Client is the client implementation for the flows API
type Client struct {
	BaseURL   string `json:"base_url"`
	JWTSigner *jwt.Signer
}

// NewClient creates a new client for the flows API
// It returns a pointer to the client
// It returns an error if the request fails
func NewClient(baseURL string, jwtSigner *jwt.Signer) *Client {
	return &Client{
		BaseURL:   baseURL,
		JWTSigner: jwtSigner,
	}
}

// ContactHasOpenTicket checks if a contact has an open ticket
// It returns true if the contact has an open ticket, false otherwise
// It returns an error if the request fails
func (c *Client) ContactHasOpenTicket(contactURN string) (bool, error) {
	url := fmt.Sprintf("%s/api/v2/internals/contact_has_open_ticket?contact_urn=ext:%s", c.BaseURL, contactURN)
	resp, err := http.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to get contact has open ticket, status code: %d", resp.StatusCode)
	}

	if strings.Contains(string(bodyBytes), "true") {
		return true, nil
	}

	return false, nil
}

// GetChannelAllowedDomains gets the allowed domains for a channel
// It returns a list of allowed domains
// It returns an error if the request fails
func (c *Client) GetChannelAllowedDomains(channelUUID string) ([]string, error) {
	url := fmt.Sprintf("%s/api/v2/internals/channel_allowed_domains?channel=%s", c.BaseURL, channelUUID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read body first to enable connection reuse
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get channel allowed domains, status code: %d", resp.StatusCode)
	}

	var domains []string
	err = json.Unmarshal(bodyBytes, &domains)
	if err != nil {
		return nil, err
	}

	return domains, nil
}

func (c *Client) GetChannelProjectLanguage(channelUUID string) (string, error) {
	url := fmt.Sprintf("%s/api/v2/projects/project_language?channel_uuid=%s", c.BaseURL, channelUUID)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read body first to enable connection reuse
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get channel project language, status code: %d", resp.StatusCode)
	}

	var response struct {
		Language string `json:"language"`
	}
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return "", err
	}

	return response.Language, nil
}

// UpdateContactFields updates the contact fields for a contact
// It returns an error if the request fails
func (c *Client) UpdateContactFields(channelUUID string, contactURN string, contactFields map[string]interface{}) error {
	url := fmt.Sprintf("%s/api/v2/internals/update_contacts_fields", c.BaseURL)

	body := map[string]interface{}{
		"channel_uuid":   channelUUID,
		"contact_urn":    fmt.Sprintf("ext:%s", contactURN),
		"contact_fields": contactFields,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if err := c.addJWTAuthHeader(req, channelUUID); err != nil {
		return fmt.Errorf("failed to add JWT auth header: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Debugf("failed to read response body, error: %s", err)
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Debugf("failed to update contact fields, status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("failed to update contact fields, status code: %d", resp.StatusCode)
	}

	return nil
}

// addJWTAuthHeader adds JWT authorization header to the request if a signer is configured
func (c *Client) addJWTAuthHeader(req *http.Request, channelUUID string) error {
	if c.JWTSigner == nil {
		return nil
	}

	token, err := c.JWTSigner.GenerateToken(map[string]interface{}{
		"channel_uuid": channelUUID,
	})
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}
