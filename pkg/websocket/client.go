package websocket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adjust/rmq/v4"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/memcache"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/ilhasoft/wwcs/pkg/queue"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Client errors
var (
	// Register
	ErrorIDAlreadyExists = errors.New("unable to register: client from already exists")
	// Redirect
	ErrorNeedRegistration = errors.New("unable to redirect: id and url is blank")
)

var cacheChannelDomains = memcache.New[string, []string]()

// Client side data
type Client struct {
	ID                 string
	Callback           string
	Conn               *websocket.Conn
	RegistrationMoment time.Time
	Queue              queue.Queue
	QueueConnection    queue.Connection
	Origin             string
	Channel            string
	Host               string
	AuthToken          string
	Histories          history.Service
	mu                 sync.Mutex
}

func (c *Client) ChannelUUID() string {
	m := regexp.MustCompile(`[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}`)
	return m.FindString(c.Callback)
}

func (c *Client) Read(app *App) {
	defer func() {
		removed := c.Unregister(app.ClientPool)
		c.Conn.Close()
		if removed {
			app.ClientManager.RemoveConnectedClient(c.ID)
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
		return CloseClientSession(payload, app)
	case "get_history":
		return c.FetchHistory(payload)
	case "verify_contact_timeout":
		return c.VerifyContactTimeout(app)
	}

	return ErrorInvalidPayloadType
}

// VerifyContactTimeout verifies if the contact has an open ticket
// It returns an error if the request fails
// It returns nil if the contact has an open ticket
func (c *Client) VerifyContactTimeout(app *App) error {
	if c.ID == "" || c.Callback == "" {
		return errors.Wrap(ErrorNeedRegistration, "verify contact timeout")
	}
	contactURN := c.ID
	hasTicket, err := app.FlowsClient.ContactHasOpenTicket(contactURN)
	if err != nil {
		log.Error("error on verify contact timeout", err)
		return errors.Wrap(err, "verify contact timeout")
	}
	if hasTicket {
		return nil
	}
	return c.Send(IncomingPayload{
		Type: "allow_contact_timeout",
	})
}

func CloseClientSession(payload OutgoingPayload, app *App) error {
	clientID := payload.From
	clientConnected, err := app.ClientManager.GetConnectedClient(clientID)
	if err != nil {
		return err
	}
	if clientConnected != nil {
		if clientConnected.AuthToken != "" && clientConnected.AuthToken != payload.Token {
			return ErrorInvalidToken
		}

		warningPayload := IncomingPayload{Type: "warning", Warning: "Connection closed by request"}
		payloadMarshalled, err := json.Marshal(warningPayload)
		if err != nil {
			log.Error("error to marshal warning connection", err)
			return err
		}

		queueConnection := queue.OpenConnection("wwcs-service", app.RDB, nil)
		defer queueConnection.Close()
		cQueue := queueConnection.OpenQueue(clientID)
		defer cQueue.Close()
		err = cQueue.PublishEX(queue.KeysExpiration, string(payloadMarshalled))
		if err != nil {
			log.Error("error to publish incoming payload: ", err)
			return err
		}
	} else {
		log.Error(ErrorInvalidClient)
		return ErrorInvalidClient
	}
	return nil
}

func CheckAllowedDomain(app *App, channelUUID string, originDomain string) bool {
	var allowedDomains []string = nil
	var err error
	cachedDomains, notexpired := cacheChannelDomains.Get(channelUUID)
	if notexpired {
		allowedDomains = cachedDomains
	} else {
		allowedDomains, err = app.FlowsClient.GetChannelAllowedDomains(channelUUID)
		if err != nil {
			log.Error("Error on get allowed domains", err)
			return false
		}
		cacheTimeout := config.Get().MemCacheTimeout
		cacheChannelDomains.Set(channelUUID, allowedDomains, time.Minute*time.Duration(cacheTimeout))
	}
	if len(allowedDomains) > 0 {
		for _, domain := range allowedDomains {
			if originDomain == domain {
				return true
			}
		}
		return false
	}
	return true
}

func OriginToDomain(origin string) (string, error) {
	u, err := url.Parse(origin)
	if err != nil {
		fmt.Println("Error on parse URL to get domain:", err)
		return "", err
	}
	domain := strings.Split(u.Host, ":")[0]
	return domain, nil
}

// Register register an user
func (c *Client) Register(payload OutgoingPayload, triggerTo postJSON, app *App) error {
	clientHost, err := payload.Host()
	if err != nil {
		log.Println(err)
	}
	hostToCheck := false
	if strings.Contains(config.Get().FlowsURL, clientHost) { // if this client is not from flows allow connection
		hostToCheck = true
	}
	if hostToCheck && config.Get().RestrictDomains {
		domain, err := OriginToDomain(c.Origin)
		if err != nil {
			return err
		}
		allowed := CheckAllowedDomain(app, payload.ChannelUUID(), domain)
		if !allowed {
			payload := IncomingPayload{
				Type:    "forbidden",
				Warning: "domain not allowed, forbidden connection",
			}
			return c.Send(payload)
		}
	}
	start := time.Now()
	err = validateOutgoingPayloadRegister(payload)
	if err != nil {
		return err
	}
	clientID := payload.From

	// check if client is connected
	clientConnected, err := app.ClientManager.GetConnectedClient(clientID)
	if err != nil {
		return err
	}
	if clientConnected != nil {
		tokenPayload := IncomingPayload{
			Type:  "token",
			Token: clientConnected.AuthToken,
		}
		tokenPayloadMarshalled, err := json.Marshal(tokenPayload)
		if err != nil {
			return err
		}
		queueConnection := queue.OpenConnection("wwcs-service", app.RDB, nil)
		defer queueConnection.Close()
		cQueue := queueConnection.OpenQueue(clientID)
		defer cQueue.Close()
		err = cQueue.PublishEX(queue.KeysExpiration, string(tokenPayloadMarshalled))
		if err != nil {
			return err
		}
		return ErrorIDAlreadyExists
	}

	// setup client info
	err = c.setupClientInfo(payload)
	if err != nil {
		return err
	}

	ConnectedClient := ConnectedClient{
		ID:        clientID,
		AuthToken: c.AuthToken,
		Channel:   c.Channel,
	}
	err = app.ClientManager.AddConnectedClient(ConnectedClient)
	if err != nil {
		return err
	}

	c.setupClientQueue(app.RDB)

	c.Histories = app.Histories

	app.ClientPool.Register(c)

	if err := c.startQueueConsuming(); err != nil {
		log.Error(err)
	}

	// if has a trigger to start a flow, redirect it
	if err := c.processTrigger(payload, triggerTo, app); err != nil {
		return err
	}
	// setup metrics if configured
	c.setupMetrics(app, start)
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

		mustCloseConnection := false
		if incomingPayload.Type == "warning" && incomingPayload.Warning == "Connection closed by request" {
			mustCloseConnection = true
		}

		if err := c.Send(incomingPayload); err != nil {
			delivery.Push()
			log.Error(err)
			return
		}

		delivery.Ack()

		if mustCloseConnection {
			c.Conn.Close()
			return
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

func (c *Client) Unregister(pool *ClientPool) bool {
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

	historyMessages, err := c.Histories.Get(c.ID, channelUUID, &c.RegistrationMoment, limit, page)
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

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteJSON(historyPayload)
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

	_, err = to(c.Callback, presenter)
	if err != nil {
		log.Error(err)
		return err
	}
	if messageType == "text" || messageType == "image" || messageType == "video" || messageType == "audio" || messageType == "file" && app != nil {
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
			err := c.SaveHistory(DirectionOut, presenter.Message)
			if err != nil {
				log.Error(err)
				return err
			}
		}
	}

	return nil
}

// Send a message to the client
func (c *Client) Send(payload IncomingPayload) error {
	log.Trace("sending message to client")
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteJSON(payload)
}

func (c *Client) SaveHistory(direction Direction, msg Message) error {
	channelUUID := c.ChannelUUID()
	if channelUUID == "" {
		return errors.New("contact channelUUID is empty")
	}
	msgTime, err := strconv.ParseInt(msg.Timestamp, 10, 64)
	if err != nil {
		return errors.Wrap(err, "error on parse timestamp from str to int64")
	}
	hmsg := NewHistoryMessagePayload(direction, c.ID, channelUUID, msg, msgTime)
	return c.Histories.Save(hmsg)
}

func (c *Client) setupClientInfo(payload OutgoingPayload) error {
	c.ID = payload.From
	c.Callback = payload.Callback
	c.RegistrationMoment = time.Now()
	u, err := url.Parse(payload.Callback)
	if err != nil {
		return err
	}
	c.Channel = u.Path
	c.Host = u.Host
	c.AuthToken = payload.Token
	return nil
}

func (c *Client) processTrigger(payload OutgoingPayload, triggerTo postJSON, app *App) error {
	if payload.Trigger != "" {
		rPayload := OutgoingPayload{
			From:     c.ID,
			Callback: c.Callback,
			Type:     "message",
			Message: Message{
				Type: "text",
				Text: payload.Trigger,
			},
		}
		trPayload, err := formatOutgoingPayload(rPayload)
		if err != nil {
			return err
		}
		if _, err := triggerTo(c.Callback, trPayload); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) setupMetrics(app *App, start time.Time) {
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
}

func (c *Client) sendToken() error {
	tokenPayload := IncomingPayload{
		Type:  "token",
		Token: c.AuthToken,
	}
	return c.Send(tokenPayload)
}
