package elevenlabs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

// IClient abstracts ElevenLabs token operations for testability.
type IClient interface {
	RequestSingleUseTokens() (sttToken string, ttsToken string, err error)
}

// Client fetches single-use tokens from the ElevenLabs API.
type Client struct {
	apiKey     string
	apiURL     string
	httpClient *http.Client
}

func NewClient(apiKey, apiURL string) *Client {
	return &Client{
		apiKey:     apiKey,
		apiURL:     strings.TrimRight(apiURL, "/"),
		httpClient: &http.Client{},
	}
}

type tokenResponse struct {
	Token string `json:"token"`
}

func (c *Client) fetchToken(scope string) (string, error) {
	url := fmt.Sprintf("%s/v1/single-use-token/%s", c.apiURL, scope)

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return "", fmt.Errorf("build request for %s: %w", scope, err)
	}
	req.Header.Set("xi-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request %s token: %w", scope, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read %s response: %w", scope, err)
	}

	if resp.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{
			"scope":       scope,
			"status_code": resp.StatusCode,
			"body":        string(body),
		}).Error("ElevenLabs token request failed")
		return "", fmt.Errorf("%s token request failed with status %d", scope, resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parse %s token response: %w", scope, err)
	}
	if tokenResp.Token == "" {
		return "", fmt.Errorf("%s token response has empty token", scope)
	}

	return tokenResp.Token, nil
}

// RequestSingleUseTokens fetches STT and TTS single-use tokens in parallel.
func (c *Client) RequestSingleUseTokens() (string, string, error) {
	var (
		sttToken, ttsToken string
		sttErr, ttsErr     error
		wg                 sync.WaitGroup
	)

	wg.Add(2)

	go func() {
		defer wg.Done()
		sttToken, sttErr = c.fetchToken("realtime_scribe")
	}()

	go func() {
		defer wg.Done()
		ttsToken, ttsErr = c.fetchToken("tts_websocket")
	}()

	wg.Wait()

	if sttErr != nil {
		return "", "", sttErr
	}
	if ttsErr != nil {
		return "", "", ttsErr
	}

	return sttToken, ttsToken, nil
}
