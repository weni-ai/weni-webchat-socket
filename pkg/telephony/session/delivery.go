package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/pkg/streams"
	"github.com/ilhasoft/wwcs/pkg/websocket"
	log "github.com/sirupsen/logrus"
)

const (
	transcriptPostMaxAttempts = 3
	transcriptPostInitialWait = 100 * time.Millisecond
)

// transcriptHTTPClient performs outbound transcript POSTs; replaced in tests.
var transcriptHTTPClient = http.DefaultClient

type transcriptCallbackResponse struct {
	ContactURN string `json:"contact_urn"`
}

type transcriptCallbackPayload struct {
	Type   string `json:"type"`
	From   string `json:"from"`
	Origin string `json:"origin"`
	DID    string `json:"did"`
	Message struct {
		Type      string `json:"type"`
		Text      string `json:"text"`
		Timestamp string `json:"timestamp"`
	} `json:"message"`
}

// PostTranscript forwards a committed transcript to Flows/Courier and returns the resolved contact URN.
func PostTranscript(callbackURL, callerID, origin, did, text string) (contactURN string, err error) {
	payload := transcriptCallbackPayload{
		Type:   "message",
		From:   callerID,
		Origin: origin,
		DID:    did,
	}
	payload.Message.Type = "text"
	payload.Message.Text = text
	payload.Message.Timestamp = strconv.FormatInt(time.Now().Unix(), 10)

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	var respBody []byte
	var statusCode int
	wait := transcriptPostInitialWait
	for attempt := 1; attempt <= transcriptPostMaxAttempts; attempt++ {
		statusCode, respBody, err = doTranscriptPOST(callbackURL, body)
		if err == nil && statusCode >= 200 && statusCode < 300 {
			break
		}
		if attempt == transcriptPostMaxAttempts {
			if err != nil {
				return "", fmt.Errorf("post transcript: %w", err)
			}
			return "", fmt.Errorf("post transcript: unexpected status %d", statusCode)
		}
		time.Sleep(wait)
		wait *= 2
	}

	if len(respBody) > 0 {
		var parsed transcriptCallbackResponse
		if json.Unmarshal(respBody, &parsed) == nil && parsed.ContactURN != "" {
			return parsed.ContactURN, nil
		}
	}

	return fallbackContactURN(callerID), nil
}

func doTranscriptPOST(callbackURL string, body []byte) (statusCode int, respBody []byte, err error) {
	req, err := http.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := transcriptHTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()

	respBody, err = io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, nil, err
	}
	return res.StatusCode, respBody, nil
}

func fallbackContactURN(callerID string) string {
	if strings.Contains(callerID, ":") {
		return callerID
	}
	return "tel:" + callerID
}

// RegisterDelivery registers the CallSession as a gRPC delivery target in ClientManager.
func RegisterDelivery(cs *CallSession, clientManager websocket.ClientManager, podID string) error {
	key := cs.RegistrationKey()
	if key == "" {
		return fmt.Errorf("session %s: contact URN not resolved", cs.ID)
	}

	cs.deliveryMu.Lock()
	defer cs.deliveryMu.Unlock()
	if cs.deliveryRegistered {
		return nil
	}

	err := clientManager.AddConnectedClient(websocket.ConnectedClient{
		ID:      key,
		Channel: cs.ChannelUUID,
		PodID:   podID,
	})
	if err != nil {
		return err
	}
	cs.deliveryRegistered = true
	return nil
}

// DeregisterDelivery removes the CallSession from ClientManager.
func DeregisterDelivery(cs *CallSession, clientManager websocket.ClientManager) error {
	key := cs.RegistrationKey()
	if key == "" {
		return nil
	}

	cs.deliveryMu.Lock()
	defer cs.deliveryMu.Unlock()
	if !cs.deliveryRegistered {
		return nil
	}

	if err := clientManager.RemoveConnectedClient(key); err != nil {
		return err
	}
	cs.deliveryRegistered = false
	return nil
}

// DeliveryCoordinator wires transcript posting and gRPC delivery registration.
type DeliveryCoordinator struct {
	clientManager  websocket.ClientManager
	sessionManager *SessionManager
	podID          string
}

// NewDeliveryCoordinator creates a coordinator for transcript and delivery lifecycle.
func NewDeliveryCoordinator(clientManager websocket.ClientManager, sessionManager *SessionManager, podID string) *DeliveryCoordinator {
	return &DeliveryCoordinator{
		clientManager:  clientManager,
		sessionManager: sessionManager,
		podID:          podID,
	}
}

// OnCommittedTranscript posts the transcript, resolves ContactURN, and registers delivery.
func (d *DeliveryCoordinator) OnCommittedTranscript(cs *CallSession, turn *Turn) {
	contactURN, err := PostTranscript(cs.CallbackURL, cs.CallerID, cs.Origin, cs.DID, turn.CommittedText)
	if err != nil {
		log.WithFields(log.Fields{
			"session_id":   cs.ID,
			"channel_uuid": cs.ChannelUUID,
		}).WithError(err).Error("telephony: failed to post transcript")
		return
	}

	cs.ContactURN = contactURN
	if d.sessionManager != nil {
		if err := d.sessionManager.SetContactURN(cs.ID, contactURN); err != nil {
			log.WithFields(log.Fields{
				"session_id":   cs.ID,
				"contact_urn":  contactURN,
				"channel_uuid": cs.ChannelUUID,
			}).WithError(err).Warn("telephony: failed to index contact URN")
		}
	}

	if err := RegisterDelivery(cs, d.clientManager, d.podID); err != nil {
		log.WithFields(log.Fields{
			"session_id":        cs.ID,
			"registration_key": cs.RegistrationKey(),
			"channel_uuid":      cs.ChannelUUID,
		}).WithError(err).Error("telephony: failed to register delivery")
		return
	}

	if err := cs.transition(StateProcessing); err != nil {
		log.WithFields(log.Fields{
			"session_id": cs.ID,
		}).WithError(err).Warn("telephony: failed to transition to processing")
	}
}

// TeardownDelivery deregisters the session from ClientManager.
func (d *DeliveryCoordinator) TeardownDelivery(cs *CallSession) {
	if err := DeregisterDelivery(cs, d.clientManager); err != nil {
		log.WithFields(log.Fields{
			"session_id":        cs.ID,
			"registration_key": cs.RegistrationKey(),
		}).WithError(err).Warn("telephony: failed to deregister delivery")
	}
}

// TelephonyDeliverFunc returns a streams deliver closure for telephony CallSessions.
func TelephonyDeliverFunc(sessionManager *SessionManager) streams.DeliverFunc {
	return func(clientID string, raw []byte) error {
		cs, ok := sessionManager.GetByRegistrationKey(clientID)
		if !ok || cs == nil {
			return fmt.Errorf("telephony session not found for client %s", clientID)
		}
		cs.handleGRPCPayload(raw)
		return nil
	}
}

// NewTelephonyStreamsRouter constructs a telephony-owned streams.Router.
func NewTelephonyStreamsRouter(
	rdb *redis.Client,
	cfg streams.StreamsConfig,
	podID string,
	clientManager websocket.ClientManager,
	sessionManager *SessionManager,
) streams.Router {
	lookup := func(clientID string) (string, bool, error) {
		cc, err := clientManager.GetConnectedClient(clientID)
		if err != nil {
			return "", false, err
		}
		if cc == nil || cc.PodID == "" {
			return "", false, nil
		}
		return cc.PodID, true, nil
	}

	isLocal := func(clientID string) bool {
		_, ok := sessionManager.GetByRegistrationKey(clientID)
		return ok
	}

	return streams.NewRouter(rdb, podID, cfg, lookup, isLocal, TelephonyDeliverFunc(sessionManager))
}
