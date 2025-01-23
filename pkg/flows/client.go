package flows

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type IClient interface {
	GetChannelAllowedDomains(string) ([]string, error)
}

type Client struct {
	BaseURL string `json:"base_url"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
	}
}

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
