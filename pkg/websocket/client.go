package websocket

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

// Client errors
var (
	// Register
	ErrorIDAlreadyExists = errors.New("unable to register: client from already exists")
	// Redirect
	ErrorNeedRegistration = errors.New("unable to redirect: id and url is blank")
)

// Client side data
type Client struct {
	ID       string
	Callback string
	Conn     *websocket.Conn
}

func (c *Client) Read(pool *Pool) {
	defer func() {
		c.Unregister(pool)
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

		err = c.ParsePayload(pool, socketPayload, toCallback)
		if err != nil {
			log.Error(err)
		}
	}
}

// ParsePayload to the respective event
func (c *Client) ParsePayload(pool *Pool, payload SocketPayload, to postForm) error {
	err := validateSocketPayload(payload)
	if err != nil {
		return err
	}

	switch payload.Type {
	case "register":
		return c.Register(pool, payload, to)
	case "message":
		return c.Redirect(payload, to)
	}

	return nil
}

// Register register an user
func (c *Client) Register(pool *Pool, payload SocketPayload, triggerTo postForm) error {
	err := validateSocketPayloadRegister(payload)
	if err != nil {
		return err
	}

	if _, found := pool.Clients[payload.From]; found {
		return ErrorIDAlreadyExists
	}

	c.ID = payload.From
	c.Callback = payload.Callback
	pool.Register(c)

	// if has a trigger to start a flow, redirect it
	if payload.Trigger != "" {
		rPayload := SocketPayload{
			Type: "message",
			Message: Message{
				Type: "text",
				Text: payload.Trigger,
			},
		}
		err := c.Redirect(rPayload, triggerTo)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) Unregister(pool *Pool) {
	pool.Unregister(c)
}

type postForm func(string, url.Values) error

func toCallback(url string, form url.Values) error {
	log.Trace("redirecting message to callback")
	req, err := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	log.Trace(res)
	return nil
}

// Redirect a message to the provided callback url
func (c *Client) Redirect(payload SocketPayload, to postForm) error {
	if c.ID == "" || c.Callback == "" {
		return ErrorNeedRegistration
	}

	err := validateSocketPayloadMessage(payload)
	if err != nil {
		return err
	}

	form := url.Values{}
	form.Set("from", c.ID)
	form.Set("text", payload.Message.Text)

	err = to(c.Callback, form)
	if err != nil {
		return err
	}

	return nil
}

// Send a message to the client
func (c *Client) Send(payload ExternalPayload) error {
	log.Trace("sending message to client")
	if err := c.Conn.WriteJSON(payload); err != nil {
		log.Error(err)
	}

	return nil
}
