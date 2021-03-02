package websocket

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/ilhasoft/wwcs/config"
	log "github.com/sirupsen/logrus"
)

// Client errors
var (
	// Register
	ErrorBlankFrom     = errors.New("unable to register: blank from")
	ErrorBlankCallback = errors.New("unable to register: blank callback")
	// Send
	ErrorNeedRegistration = errors.New("unable to send: id and url is blank")
)

// Client side data
type Client struct {
	ID       string
	Callback string
	Conn     *websocket.Conn
	Pool     *Pool
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
	Type     string  `json:"type"`
	From     string  `json:"from,omitempty"`
	Callback string  `json:"callback,omitempty"`
	Message  Message `json:"message,omitempty"`
}

// Message data
type Message struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	URL       string `json:"url,omitempty"`
	Caption   string `json:"caption,omitempty"`
	FileName  string `json:"filename,omitempty"`
	Latitude  string `json:"latitude,omitempty"`
	Longitude string `json:"longitude,omitempty"`
}

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
		log.Trace("Reading messages")
		socketPayload := SocketPayload{}
		err := c.Conn.ReadJSON(&socketPayload)
		if err != nil {
			if err.Error() != "websocket: close 1001 (going away)" {
				log.Error(err)
			}
			return
		}

		switch socketPayload.Type {
		case "register":
			err = c.Register(socketPayload)
		case "message":
			err = c.Redirect(socketPayload)
		}

		if err != nil {
			log.Error(err)
			return
		}
	}
}

// Register register an user
func (c *Client) Register(payload SocketPayload) error {
	log.Tracef("Registering client %s", payload.From)
	if payload.From == "" {
		return ErrorBlankFrom
	}

	if payload.Callback == "" {
		return ErrorBlankCallback
	}

	c.ID = payload.From
	c.Callback = payload.Callback
	c.Pool.Register <- c

	if config.Get.Websocket.SendWellcomeMessage {
		c.sendWellcomeMessage()
	}
	return nil
}

// Redirect message to the active redirects
func (c *Client) Redirect(payload SocketPayload) error {
	if c.ID == "" || c.Callback == "" {
		return ErrorNeedRegistration
	}

	config := config.Get.Websocket

	if config.RedirectToFrontend {
		c.redirectToFrontend(payload)
	}

	if config.RedirectToCallback {
		c.redirectToCallback(payload)
	}

	return nil
}

// redirectToCallback will send the message to the callback url provided on register
func (c *Client) redirectToCallback(payload SocketPayload) {
	log.Trace("Redirecting message to callback")
	form := url.Values{}
	form.Set("from", c.ID)
	form.Set("text", payload.Message.Text)

	req, _ := http.NewRequest("POST", c.Callback, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(err)
	}
	log.Trace(res)
}

// redirectToFrontend will resend the message to the frontend
func (c *Client) redirectToFrontend(payload SocketPayload) {
	log.Trace("Redirecting message to frontend")
	external := &ExternalPayload{
		Text: payload.Message.Text,
	}

	sender := Sender{
		Client:  c,
		Payload: external,
	}

	c.Pool.Sender <- sender
}

// redirectToFrontend will resend the message to the frontend
func (c *Client) sendWellcomeMessage() {
	log.Tracef("Sending wellcome message to %q", c.ID)
	external := &ExternalPayload{
		Text: config.Get.Websocket.WellcomeMessage,
	}

	sender := Sender{
		Client:  c,
		Payload: external,
	}

	c.Pool.Sender <- sender
}
