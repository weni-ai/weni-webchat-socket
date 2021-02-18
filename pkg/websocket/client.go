package websocket

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

// Client side data
type Client struct {
	ID   string
	Conn *websocket.Conn
	Pool *Pool
}

// ExternalPayload  data
type ExternalPayload struct {
	To           string `json:"to,omitempty"`
	ToNoPlus     string `json:"to_no_plus,omitempty"`
	From         string `json:"from,omitempty"`
	FromNoPlus   string `json:"from_no_plus,omitempty"`
	Text         string `json:"text,omitempty"`
	ID           string `json:"id,omitempty"`
	QuickReplies string `json:"quick_replies,omitempty"`
}

// SocketPayload data
type SocketPayload struct {
	From        string `json:"from,omitempty"`
	Text        string `json:"text,omitempty"`
	HostAPI     string `json:"hostApi,omitempty"`
	ChannelUUID string `json:"channelUUID,omitempty"`
}

// // Payload data
// type Payload struct {
// 	To      string  `json:"to"`
// 	From    string  `json:"from"`
// 	Type    int     `json:"type"`
// 	Message Message `json:"message"`
// }

// // Message data
// type Message struct {
// 	Type      string `json:"type"`
// 	Text      string `json:"text,omitempty"`
// 	URL       string `json:"url,omitempty"`
// 	Caption   string `json:"caption,omitempty"`
// 	FileName  string `json:"filename,omitempty"`
// 	Latitude  string `json:"latitude,omitempty"`
// 	Longitude string `json:"longitude,omitempty"`
// }

// Sender message data
type Sender struct {
	Payload *ExternalPayload
	Client  *Client
}

func (c *Client) Read() {
	defer func() {
		c.Pool.Unregister <- c
		c.Conn.Close()
	}()

	for {
		log.Trace("Reading messages...")
		socketPayload := SocketPayload{}
		err := c.Conn.ReadJSON(&socketPayload)
		if err != nil {
			log.Error(err)
			return
		}
		socketPayload.From = c.ID

		payload := &ExternalPayload{
			To:           c.ID,
			ToNoPlus:     c.ID,
			From:         c.ID,
			FromNoPlus:   c.ID,
			Text:         socketPayload.Text,
			ID:           c.ID,
			QuickReplies: "",
		}

		sender := Sender{
			Client:  c,
			Payload: payload,
		}

		c.Pool.Sender <- sender

		log.Trace("Redirecting message...")
		redirectMessage(socketPayload)
	}
}

func redirectMessage(payload SocketPayload) {
	form := url.Values{}
	form.Set("from", payload.From)
	form.Set("text", payload.Text)

	url := fmt.Sprintf("%s/c/ex/%s/receive", payload.HostAPI, payload.ChannelUUID)
	req, _ := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(err)
	}
	log.Debug(res)
}
