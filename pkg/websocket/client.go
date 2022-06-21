package websocket

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/adjust/rmq/v4"
	uni "github.com/dchest/uniuri"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/history"
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
	AuthToken       string
	Histories       history.Service
	SessionType     SessionType
}

type SessionType string

func (c *Client) ChannelUUID() string {
	m := regexp.MustCompile(`[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}`)
	return m.FindString(c.Callback)
}

func (c *Client) Read(app *App) {
	defer func() {
		removed := c.Unregister(app.Pool)
		c.Conn.Close()
		if removed {
			if app.Metrics != nil {
				openConnectionsMetrics := metric.NewOpenConnection(
					c.Channel,
					c.Host,
					c.Origin,
				)
				app.Metrics.DecOpenConnections(openConnectionsMetrics)
			}
		}
	}()

	for {
		log.Trace("Reading messages")
		OutgoingPayload := OutgoingPayload{}
		err := c.Conn.ReadJSON(&OutgoingPayload)
		if err != nil {
			ignoredLowLevelCloseErrorCodes := []string{
				"1000",
				"1001",
				"1002",
				"1003",
				"1004",
				"1005",
				"1006",
				"1007",
				"1008",
				"1009",
				"1010",
				"1011",
				"1012",
				"1013",
				"1014",
				"1015",
				// Occur when this server close connection.
				// As this application has concurrent reader and writer and one of them closes the
				// connection, then it's typical that the other operation will return this error. The error is benign in this case. Ignore it.
				"use of closed network connection",
			}
			ignore := false
			for _, code := range ignoredLowLevelCloseErrorCodes {
				if strings.Contains(err.Error(), code) {
					ignore = true
				}
			}
			if !ignore {
				log.Error(err, c)
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
	case "close_session":
		return CloseSession(payload, app)
	case "get_history":
		return c.FetchHistory(payload)
	}

	return ErrorInvalidPayloadType
}

func CloseSession(payload OutgoingPayload, app *App) error {

	client := app.Pool.Clients[payload.From]
	if client != nil {
		if client.AuthToken == payload.Token {
			errorPayload := IncomingPayload{
				Type:    "warning",
				Warning: "Connection closed by request",
			}
			err := client.Send(errorPayload)
			if err != nil {
				log.Error(err)
			}
			client.Conn.Close()
			return nil
		} else {
			return ErrorInvalidToken
		}
	}
	return ErrorInvalidClient
}

// Register register an user
func (c *Client) Register(payload OutgoingPayload, triggerTo postJSON, app *App) error {
	start := time.Now()
	err := validateOutgoingPayloadRegister(payload)
	if err != nil {
		return err
	}

	if client, found := app.Pool.Clients[payload.From]; found {
		tokenPayload := IncomingPayload{
			Type:  "token",
			Token: client.AuthToken,
		}
		err = c.Send(tokenPayload)
		if err != nil {
			return err
		}
		return ErrorIDAlreadyExists
	}

	c.ID = payload.From
	c.Callback = payload.Callback
	c.AuthToken = uni.NewLen(32)
	c.setupClientQueue(app.RDB)

	u, err := url.Parse(payload.Callback)
	if err != nil {
		return err
	}
	c.Channel = u.Path
	c.Host = u.Host

	c.SessionType = payload.SessionType
	if payload.SessionType == SessionType(config.Get().SessionTypeToStore) {
		c.Histories = app.Histories
	}

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

	if app.Metrics != nil {
		duration := time.Since(start).Seconds()
		socketRegistrationMetrics := metric.NewSocketRegistration(
			c.Channel,
			c.Host,
			c.Origin,
			duration,
		)
		openConnectionsMetrics := metric.NewOpenConnection(
			c.Channel,
			c.Host,
			c.Origin,
		)
		app.Metrics.IncOpenConnections(openConnectionsMetrics)
		app.Metrics.SaveSocketRegistration(socketRegistrationMetrics)
	}

	// token sending
	tokenPayload := IncomingPayload{
		Type:  "token",
		Token: c.AuthToken,
	}
	err = c.Send(tokenPayload)
	if err != nil {
		return err
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
		config.Get().RedisQueue.ConsumerPrefetchLimit,
		time.Duration(config.Get().RedisQueue.ConsumerPollDuration)*time.Millisecond,
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

		if c.Histories != nil {
			err := c.SaveHistory(DirectionIncoming, incomingPayload.Message)
			if err != nil {
				log.Error(err)
			}
		}
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

func (c *Client) Unregister(pool *Pool) bool {
	c.CloseQueueConnections()
	return pool.Unregister(c) != nil
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

func (c *Client) FetchHistory(payload OutgoingPayload) error {
	if c.ID == "" {
		return ErrorNeedRegistration
	}

	if c.SessionType != SessionType(config.Get().SessionTypeToStore) {
		err := fmt.Errorf(
			"error on get history: only client with session type %s is allowed to fetch history",
			config.Get().SessionTypeToStore,
		)
		errorPayload := IncomingPayload{
			Type:  "error",
			Error: err.Error(),
		}
		c.Send(errorPayload)
		return err
	}

	limitParam := fmt.Sprint(payload.Params["limit"])
	pageParam := fmt.Sprint(payload.Params["page"])

	limit, err := strconv.Atoi(limitParam)
	if err != nil {
		err = fmt.Errorf("error on get history: could not parse limit param: %s", err.Error())
		errorPayload := IncomingPayload{
			Type:  "error",
			Error: err.Error(),
		}
		c.Send(errorPayload)
		return err
	}
	page, err := strconv.Atoi(pageParam)
	if err != nil {
		err = fmt.Errorf("error on get history: could not parse page param: %s", err.Error())
		errorPayload := IncomingPayload{
			Type:  "error",
			Error: err.Error(),
		}
		c.Send(errorPayload)
		return err
	}

	channelUUID := c.ChannelUUID()
	if channelUUID == "" {
		err := errors.New("channelUUID is not set, could not fetch history")
		errorPayload := IncomingPayload{
			Type:  "error",
			Error: fmt.Sprintf("error on get history, %s", err.Error()),
		}
		c.Send(errorPayload)
		return err
	}

	historyMessages, err := c.Histories.Get(c.ID, channelUUID, limit, page)
	if err != nil {
		errorPayload := IncomingPayload{
			Type:  "error",
			Error: fmt.Sprintf("error on get history, %s", err.Error()),
		}
		c.Send(errorPayload)
		return nil
	}

	historyPayload := HistoryPayload{
		Type:    "history",
		History: historyMessages,
	}

	c.Conn.WriteJSON(historyPayload)

	return nil
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
		return nil
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
			if err = app.OutgoingQueue.PublishEX(MSG_EXPIRATION, string(sjm)); err != nil {
				return err
			}
		}
	}
	if messageType == "text" && app != nil {
		if app.Metrics != nil {
			duration := time.Since(start).Seconds()
			clientMessageMetrics := metric.NewClientMessage(
				c.Channel,
				c.Host,
				c.Origin,
				fmt.Sprint(http.StatusOK),
				duration,
			)
			app.Metrics.SaveClientMessages(clientMessageMetrics)
		}

		if c.Histories != nil {
			err := c.SaveHistory(DirectionOutgoing, presenter.Message)
			if err != nil {
				log.Error(err)
			}
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

func (c *Client) SaveHistory(direction Direction, msg Message) error {
	channelUUID := c.ChannelUUID()
	if channelUUID == "" {
		return errors.New("contact channelUUID is empty")
	}
	hmsg := NewHistoryMessagePayload(direction, c.ID, channelUUID, msg)
	return c.Histories.Save(hmsg)
}
