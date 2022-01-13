package websocket

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/adjust/rmq/v4"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/ilhasoft/wwcs/pkg/queue"
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
	ID              string
	Callback        string
	Conn            *websocket.Conn
	Queue           queue.Queue
	QueueConnection queue.Connection
	Origin          string
	Channel         string
	Host            string
}

func (c *Client) Read(app *App) {
	openConnectionsMetrics := metric.NewOpenConnection(
		c.Channel,
		c.Host,
		c.Origin,
	)
	if app.Metrics != nil {
		app.Metrics.IncOpenConnections(openConnectionsMetrics)
	}

	defer func() {
		c.Unregister(app.Pool)
		c.Conn.Close()
		openConnectionsMetrics := metric.NewOpenConnection(
			c.Channel,
			c.Host,
			c.Origin,
		)
		if app.Metrics != nil {
			app.Metrics.DecOpenConnections(openConnectionsMetrics)
		}
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
		return c.Register(payload, to, app)
	case "message":
		return c.Redirect(payload, to, app)
	case "ping":
		return c.Redirect(payload, to, app)
	}

	return ErrorInvalidPayloadType
}

var apiPath = "/c/wwc/"

// Register register an user
func (c *Client) Register(payload OutgoingPayload, triggerTo postJSON, app *App) error {
	start := time.Now()
	err := validateOutgoingPayloadRegister(payload)
	if err != nil {
		return err
	}

	if _, found := app.Pool.Clients[payload.From]; found {
		return ErrorIDAlreadyExists
	}

	c.ID = payload.From
	c.Callback = payload.Callback
	c.setupClientQueue(app.RDB)

	u, err := url.Parse(payload.Callback)
	if err != nil {
		return err
	}
	c.Channel = u.Path
	c.Host = u.Host

	app.Pool.Register(c)

	readyDeliveriesCount, err := c.Queue.Queue().ReadyCount()
	if err != nil {
		log.Error(err)
	}

	if readyDeliveriesCount > 0 {
		if err := c.startQueueConsuming(); err != nil {
			log.Error(err)
		}
		return nil
	}

	// if has a trigger to start a flow, redirect it
	if payload.Trigger != "" {
		rPayload := OutgoingPayload{
			Type: "message",
			Message: Message{
				Type: "text",
				Text: payload.Trigger,
			},
		}
		err := c.Redirect(rPayload, triggerTo, app)
		if err != nil {
			return err
		}
	}

	duration := time.Since(start).Seconds()
	socketRegistrationMetrics := metric.NewSocketRegistration(
		c.Channel,
		c.Host,
		c.Origin,
		duration,
	)
	if app.Metrics != nil {
		app.Metrics.SaveSocketRegistration(socketRegistrationMetrics)
	}

	return nil
}

func (c *Client) setupClientQueue(rdb *redis.Client) {
	rmqConnection := queue.OpenConnection(c.ID, rdb, nil)
	c.QueueConnection = rmqConnection
	c.Queue = c.QueueConnection.OpenQueue(c.ID)
}

func (c *Client) startQueueConsuming() error {
	if err := c.Queue.StartConsuming(
		config.Get.RedisQueue.ConsumerPrefetchLimit,
		time.Duration(config.Get.RedisQueue.ConsumerPollDuration)*time.Millisecond,
	); err != nil {
		return err
	}
	c.Queue.AddConsumerFunc(c.ID, func(delivery rmq.Delivery) {
		var incomingPayload IncomingPayload
		if err := json.Unmarshal([]byte(delivery.Payload()), &incomingPayload); err != nil {
			delivery.Reject()
			log.Error(err)
			return
		}
		if err := c.Send(incomingPayload); err != nil {
			delivery.Push()
			log.Error(err)
			return
		}
		delivery.Ack()
	})
	return nil
}

func (c *Client) CloseQueueConnections() {
	if c.Queue != nil {
		c.Queue.Close()
		c.Queue.Destroy()
		c.QueueConnection.Close()
	}
}

func (c *Client) Unregister(pool *Pool) {
	c.CloseQueueConnections()
	pool.Unregister(c)
}

type postJSON func(string, interface{}) ([]byte, error)

func ToCallback(url string, data interface{}) ([]byte, error) {
	log.Trace("redirecting message to callback")
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return body, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return body, err
	}
	log.Trace(res)
	return body, nil
}

// Redirect a message to the provided callback url
func (c *Client) Redirect(payload OutgoingPayload, to postJSON, app *App) error {
	start := time.Now()
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

	body, err := to(c.Callback, presenter)
	if err != nil {
		if body == nil {
			return err
		}
		if app.OutgoingQueue != nil {
			sJob := OutgoingJob{
				URL:     c.Callback,
				Payload: presenter,
			}
			sjm, err := json.Marshal(sJob)
			if err != nil {
				return err
			}
			if err = app.OutgoingQueue.Publish(string(sjm)); err != nil {
				return err
			}
		}
	}
	if messageType == "text" && app != nil {
		duration := time.Since(start).Seconds()
		clientMessageMetrics := metric.NewClientMessage(
			c.Channel,
			c.Host,
			c.Origin,
			fmt.Sprint(http.StatusOK),
			duration,
		)
		if app.Metrics != nil {
			app.Metrics.SaveClientMessages(clientMessageMetrics)
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
