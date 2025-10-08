package flows

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// IClient is the interface for the flows client API
type IClient interface {
	ContactHasOpenTicket(string) (bool, error)
	GetChannelAllowedDomains(string) ([]string, error)
	GetChannelProjectLanguage(string) (string, error)
}

// Client is the client implementation for the flows API
type Client struct {
	BaseURL string `json:"base_url"`
}

// NewClient creates a new client for the flows API
// It returns a pointer to the client
// It returns an error if the request fails
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
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

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to get contact has open ticket, status code: %d", resp.StatusCode)
	}

	var bodyBytes []byte
	bodyBytes, err = io.ReadAll(resp.Body)
	if err != nil {
		return false, err
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get channel allowed domains, status code: %d", resp.StatusCode)
	}

	var domains []string
	err = json.NewDecoder(resp.Body).Decode(&domains)
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

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get channel project language, status code: %d", resp.StatusCode)
	}

	var response struct {
		Language string `json:"language"`
	}
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return "", err
	}

	return response.Language, nil
}
