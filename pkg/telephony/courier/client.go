package courier

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	log "github.com/sirupsen/logrus"
)

const ResolvePath = "/c/tph/resolve"

// IClient resolves PSTN channels via the Courier HTTP API.
type IClient interface {
	ResolveChannel(did string) (channelUUID, projectUUID string, err error)
}

// Client is the HTTP client for Courier telephony endpoints.
type Client struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

// NewClient creates a Courier client for telephony channel resolution.
func NewClient(baseURL, authToken string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		authToken:  strings.TrimSpace(authToken),
		httpClient: httpClient,
	}
}

// ResolveURL builds the Courier PSTN channel resolve endpoint for a DID.
func ResolveURL(baseURL, did string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s%s?did=%s", base, ResolvePath, url.QueryEscape(did))
}

// ResolveChannel resolves a dialed number to its Courier PSTN channel instance.
func (c *Client) ResolveChannel(did string) (channelUUID, projectUUID string, err error) {
	reqURL := ResolveURL(c.baseURL, did)
	if reqURL == "" {
		return "", "", fmt.Errorf("courier base URL is required")
	}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", "", err
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.WithFields(log.Fields{
			"did": did,
			"url": reqURL,
		}).WithError(err).Error("courier API: HTTP request failed for ResolveChannel")
		return "", "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithFields(log.Fields{
			"did":         did,
			"status_code": resp.StatusCode,
		}).WithError(err).Error("courier API: failed to read response body for ResolveChannel")
		return "", "", err
	}

	if resp.StatusCode == http.StatusNotFound {
		return "", "", nil
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusBadRequest && strings.Contains(string(bodyBytes), "channel not found") {
			return "", "", nil
		}

		log.WithFields(log.Fields{
			"did":           did,
			"status_code":   resp.StatusCode,
			"response_body": string(bodyBytes),
		}).Error("courier API: non-200 response for ResolveChannel")
		return "", "", fmt.Errorf("failed to resolve PSTN channel, status code: %d", resp.StatusCode)
	}

	var response struct {
		ChannelUUID string `json:"channel_uuid"`
		ProjectUUID string `json:"project_uuid"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		log.WithFields(log.Fields{
			"did":           did,
			"response_body": string(bodyBytes),
		}).WithError(err).Error("courier API: failed to unmarshal resolve response")
		return "", "", err
	}

	return response.ChannelUUID, response.ProjectUUID, nil
}
