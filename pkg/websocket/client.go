package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/memcache"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Client errors
var (
	// Register
	ErrorIDAlreadyExists = errors.New("unable to register: client from already exists")
	// Redirect
	ErrorNeedRegistration = errors.New("unable to redirect: id and url is blank")
	// Original handler is dead
	ErrorOriginalHandlerDead = errors.New("unable to register: original handler is dead, wait for unregister to register again")
)

var cacheChannelDomains = memcache.New[string, []string]()

// Client side data
type Client struct {
	ID                 string
	Callback           string
	Conn               *websocket.Conn
	RegistrationMoment time.Time
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
	start := time.Now()
	defer func() {
		log.Debugf("closing client %s", c.ID)
		removed := c.Unregister(app.ClientPool)
		c.Conn.Close()
		if removed {
			log.Debugf("removing connected client %s", c.ID)
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
		log.Debugf("reading messages for client %s", c.ID)
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
				// When intermediaries or clients send non-standard frames or alter WS frames
				// gorilla/websocket may return errors like below; treat them as benign.
				"unknown opcode",
				"unexpected reserved bits",
			}
			ignore := false
			for _, code := range ignoredLowLevelCloseErrorCodes {
				if strings.Contains(err.Error(), code) {
					ignore = true
				}
			}
			if !ignore {
				log.WithField("client_id", c.ID).WithField("reading_since", time.Since(start).Seconds()).WithError(err).Error("error reading payload")
			}
			return
		}

		log.Debugf("parsing payload for client %s, payload: %+v", c.ID, OutgoingPayload)
		err = c.ParsePayload(app, OutgoingPayload, ToCallback)
		if err != nil {
			log.WithField("client_id", c.ID).WithField("reading_since", time.Since(start).Seconds()).WithError(err).Error("error parsing payload")
			errorPayload := IncomingPayload{
				Type:  "error",
				Error: err.Error(),
			}
			err := c.Send(errorPayload)
			if err != nil {
				log.WithField("client_id", c.ID).WithField("reading_since", time.Since(start).Seconds()).WithError(err).Error("error sending error payload")
			}
		}

		// Refresh presence TTL on any received frame when ID is known
		if c.ID != "" {
			_, _ = app.ClientManager.UpdateClientTTL(c.ID, app.ClientManager.DefaultClientTTL())
		}
	}
}

// ParsePayload to the respective event
func (c *Client) ParsePayload(app *App, payload OutgoingPayload, to postJSON) error {
	switch payload.Type {
	case "register":
		log.Debugf("registering client %s", payload.From)
		return c.Register(payload, to, app)
	case "message":
		log.Debugf("redirecting message for client %s", payload.From)
		return c.Redirect(payload, to, app)
	case "ping":
		log.Debugf("redirecting ping for client %s", payload.From)
		return c.Redirect(payload, to, app)
	case "close_session":
		return c.CloseClientSession(payload, app)
	case "get_history":
		log.Debugf("fetching history for client %s", payload.From)
		return c.FetchHistory(payload)
	case "verify_contact_timeout":
		log.Debugf("verifying contact timeout for client %s", c.ID)
		return c.VerifyContactTimeout(app)
	case "get_project_language":
		return c.GetProjectLanguage(payload, app)
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

func (c *Client) GetProjectLanguage(payload OutgoingPayload, app *App) error {
	if c.ID == "" || c.Callback == "" {
		return errors.Wrap(ErrorNeedRegistration, "get project language")
	}
	channelUUID := c.ChannelUUID()
	language, err := app.FlowsClient.GetChannelProjectLanguage(channelUUID)
	if err != nil {
		log.Error("error on get project language", err)
		return errors.Wrap(err, "get project language")
	}

	data := map[string]any{
		"language": language,
	}
	return c.Send(IncomingPayload{Type: "project_language", Data: data})
}

func (c *Client) CloseClientSession(payload OutgoingPayload, app *App) error {
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
		if app.Router != nil {
			if err := app.Router.PublishToClient(context.Background(), clientID, payloadMarshalled); err != nil {
				log.Error("error to publish incoming payload: ", err)
				return err
			}
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
	log.Debugf("registering client %s", payload.From)
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
	log.Debugf("checking if client %s is connected", clientID)
	clientConnected, err := app.ClientManager.GetConnectedClient(clientID)
	if err != nil {
		return err
	}
	if clientConnected != nil {
		// If the recorded pod is dead, remove stale mapping and allow registration
		isAlive := false
		if clientConnected.PodID != "" {
			if exists, _ := app.RDB.Exists(context.Background(), "ws:pod:hb:"+clientConnected.PodID).Result(); exists == 1 {
				isAlive = true
			}
		}
		if !isAlive {
			_ = app.ClientManager.RemoveConnectedClient(clientID)
			return ErrorOriginalHandlerDead
		} else {
			tokenPayload := IncomingPayload{Type: "token", Token: clientConnected.AuthToken}
			tokenPayloadMarshalled, err := json.Marshal(tokenPayload)
			if err != nil {
				return err
			}
			if app.Router != nil {
				log.Debugf("publishing token to client %s", clientID)
				if err := app.Router.PublishToClient(context.Background(), clientID, tokenPayloadMarshalled); err != nil {
					return err
				}
			}
			return ErrorIDAlreadyExists
		}
	}

	// setup client info
	log.Debugf("setting up client info for client %s, payload: %+v", clientID, payload)
	err = c.setupClientInfo(payload)
	if err != nil {
		return err
	}

	ConnectedClient := ConnectedClient{
		ID:        clientID,
		AuthToken: c.AuthToken,
		Channel:   c.Channel,
		PodID:     app.PodID,
	}
	log.Debugf("adding connected client %s to client manager", clientID)
	err = app.ClientManager.AddConnectedClient(ConnectedClient)
	if err != nil {
		return err
	}

	c.Histories = app.Histories

	log.Debugf("registering client %s in pool", clientID)
	app.ClientPool.Register(c)

	// if has a trigger to start a flow, redirect it
	log.Debugf("processing trigger for client %s", clientID)
	if err := c.processTrigger(payload, triggerTo, app); err != nil {
		return err
	}
	// setup metrics if configured
	log.Debugf("setup metrics for client %s", clientID)
	c.setupMetrics(app, start)

	// when client is ready to receive messages, send this
	// message with ready_for_message type to the
	// client to frontend know that it is ready to send messages to channel
	log.Debugf("sending ready for message to client %s", clientID)
	c.Send(IncomingPayload{
		Type: "ready_for_message",
	})
	log.Debugf("client %s registered successfully", clientID)
	return nil
}

func (c *Client) Unregister(pool *ClientPool) bool {
	return pool.Unregister(c) != nil
}

type postJSON func(string, interface{}) ([]byte, error)

func ToCallback(url string, data interface{}) ([]byte, error) {
	log.Trace("redirecting message to callback")

	if payload, ok := data.(OutgoingPayload); ok {
		data = applyContextForCallback(payload)
	}

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

// applyContextForCallback applies context concatenation only for external callbacks
func applyContextForCallback(payload OutgoingPayload) OutgoingPayload {
	callbackPayload := payload

	if payload.Context != "" && payload.Message.Type == "text" {
		callbackPayload.Message = payload.Message
		callbackPayload.Message.Text = fmt.Sprintf("%s; Context: %s", payload.Message.Text, payload.Context)
	}

	return callbackPayload
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
