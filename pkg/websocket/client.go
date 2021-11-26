package websocket

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

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

func (c *Client) Read(app *App) {
	defer func() {
		c.Unregister(app.Pool)
		c.Conn.Close()
	}()

	for {
		log.Trace("Reading messages")
		OutgoingPayload := OutgoingPayload{}
		err := c.Conn.ReadJSON(&OutgoingPayload)
		if err != nil {
			if err.Error() != "websocket: close 1001 (going away)" {
				log.Error(err)
			}
			return
		}

		err = c.ParsePayload(app, OutgoingPayload, ToCallback)
		if err != nil {
			errorPayload := IncomingPayload{
				Type:  "error",
				Error: err.Error(),
			}
			err := c.Send(errorPayload)
			if err != nil {
				log.Error(err)
			}
		}
	}
}

// ParsePayload to the respective event
func (c *Client) ParsePayload(app *App, payload OutgoingPayload, to postJSON) error {
	switch payload.Type {
	case "register":
		return c.Register(app.Pool, payload, to)
	case "message":
		return c.Redirect(payload, to, app)
	case "ping":
		return c.Redirect(payload, to, app)
	}

	return ErrorInvalidPayloadType
}

// Register register an user
func (c *Client) Register(pool *Pool, payload OutgoingPayload, triggerTo postJSON) error {
	err := validateOutgoingPayloadRegister(payload)
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
		rPayload := OutgoingPayload{
			Type: "message",
			Message: Message{
				Type: "text",
				Text: payload.Trigger,
			},
		}
		err := c.Redirect(rPayload, triggerTo, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) Unregister(pool *Pool) {
	pool.Unregister(c)
}

type postJSON func(string, interface{}) error

func ToCallback(url string, data interface{}) error {
	log.Trace("redirecting message to callback")
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
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
func (c *Client) Redirect(payload OutgoingPayload, to postJSON, app *App) error {
	if c.ID == "" || c.Callback == "" {
		return ErrorNeedRegistration
	}

	payload.From = c.ID
	payload.Callback = c.Callback
	presenter, err := formatOutgoingPayload(payload)
	if err != nil {
		return err
	}

	// if the message have an attachment send the url back to client
	messageType := presenter.Message.Type
	if messageType != "text" && messageType != "location" && messageType != "pong" {
		clientPayload := IncomingPayload{
			Type:    "ack",
			To:      presenter.From,
			From:    "socket",
			Message: presenter.Message,
		}
		err = c.Send(clientPayload)
		if err != nil {
			return err
		}
	} else if messageType == "pong" {
		pongPayload := IncomingPayload{
			Type: "pong",
		}
		err = c.Send(pongPayload)
		if err != nil {
			return err
		}
	}

	// err = to(c.Callback, presenter)
	if app.QOProducer != nil {
		sJob := OutgoingJob{
			URL:     c.Callback,
			Payload: presenter,
		}
		sjm, err := json.Marshal(sJob)
		if err != nil {
			return err
		}
		err = app.QOProducer.Publish(string(sjm))
		if err != nil {
			return err
		}
	}

	return nil
}

// Send a message to the client
func (c *Client) Send(payload IncomingPayload) error {
	log.Trace("sending message to client")
	if err := c.Conn.WriteJSON(payload); err != nil {
		return err
	}

	return nil
}
