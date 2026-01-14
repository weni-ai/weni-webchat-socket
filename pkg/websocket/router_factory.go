package websocket

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/pkg/streams"
	log "github.com/sirupsen/logrus"
)

// ErrClientNotLocal is returned by deliver when the client is not found in the
// local pool. This signals the router to re-check presence and potentially
// re-route the message to another pod instead of dropping it.
var ErrClientNotLocal = errors.New("client not found in local pool")

// ErrClientDisconnected is returned when the client was found but the
// connection is no longer valid (e.g., browser closed, network dropped).
// This signals the router to re-check presence for potential re-routing.
var ErrClientDisconnected = errors.New("client disconnected during send")

// NewStreamsRouter wires a streams.Router with lookup/isLocal/deliver closures
// based on the websocket client manager and pool.
func NewStreamsRouter(
	rdb *redis.Client,
	cfg streams.StreamsConfig,
	podID string,
	pool *ClientPool,
	clientM ClientManager,
) streams.Router {
	lookup := func(clientID string) (string, bool, error) {
		cc, err := clientM.GetConnectedClient(clientID)
		if err != nil {
			return "", false, err
		}
		if cc == nil || cc.PodID == "" {
			return "", false, nil
		}
		return cc.PodID, true, nil
	}

	isLocal := func(clientID string) bool {
		_, ok := pool.Find(clientID)
		return ok
	}

	deliver := func(clientID string, raw []byte) error {
		client, ok := pool.Find(clientID)
		if !ok || client == nil {
			// Return error so router can re-check presence and re-route if needed.
			return ErrClientNotLocal
		}

		// Try to detect and handle streaming payloads first
		if streamPayload, ok := tryUnmarshalStreamPayload(raw); ok {
			if err := client.SendStreamPayload(streamPayload); err != nil {
				if isBenignConnectionError(err) {
					log.WithFields(log.Fields{
						"client_id": clientID,
					}).Debug("streams router: client disconnected during stream send, will re-check presence")
					return ErrClientDisconnected
				}
				log.WithFields(log.Fields{
					"client_id":    clientID,
					"payload_size": len(raw),
				}).WithError(err).Error("streams router: failed to send stream payload to websocket client")
				return err
			}
			_, _ = clientM.UpdateClientTTL(clientID, clientM.DefaultClientTTL())
			return nil
		}

		// Regular payload - unmarshal and send as before
		var incoming IncomingPayload
		if err := json.Unmarshal(raw, &incoming); err != nil {
			log.WithFields(log.Fields{
				"client_id":    clientID,
				"payload_size": len(raw),
			}).WithError(err).Error("streams router: failed to unmarshal incoming payload for delivery")
			return err
		}
		if err := client.Send(incoming); err != nil {
			if isBenignConnectionError(err) {
				// Return error so router can re-check presence and re-route if the
				// client reconnected to another pod.
				log.WithFields(log.Fields{
					"client_id":    clientID,
					"payload_type": incoming.Type,
				}).Debug("streams router: client disconnected during send, will re-check presence")
				return ErrClientDisconnected
			}
			log.WithFields(log.Fields{
				"client_id":    clientID,
				"payload_type": incoming.Type,
				"to":           incoming.To,
				"channel_uuid": incoming.ChannelUUID,
			}).WithError(err).Error("streams router: failed to send message to websocket client")
			return err
		}
		// Update last-seen (no-op for TTL, used by cleanup)
		_, _ = clientM.UpdateClientTTL(clientID, clientM.DefaultClientTTL())
		return nil
	}

	return streams.NewRouter(rdb, podID, cfg, lookup, isLocal, deliver)
}

// tryUnmarshalStreamPayload attempts to unmarshal raw JSON into a streaming payload.
// Returns the typed payload and true if successful, nil and false otherwise.
func tryUnmarshalStreamPayload(raw []byte) (StreamPayload, bool) {
	// Quick check for streaming payload indicators
	if bytes.Contains(raw, []byte(`"stream_start"`)) {
		var p StreamStartPayload
		if json.Unmarshal(raw, &p) == nil && p.Type == "stream_start" {
			return p, true
		}
	}
	if bytes.Contains(raw, []byte(`"stream_end"`)) {
		var p StreamEndPayload
		if json.Unmarshal(raw, &p) == nil && p.Type == "stream_end" {
			return p, true
		}
	}
	// Check for delta payload by checking JSON structure at the beginning.
	// Delta payloads are exactly {"v":"..."} with no type field at the root level.
	// We check for `{"v":` at the start to avoid matching content that happens to contain "v":
	// Empty deltas ({"v":""}) are valid and should be accepted.
	if bytes.HasPrefix(raw, []byte(`{"v":`)) {
		var p StreamDeltaPayload
		if json.Unmarshal(raw, &p) == nil {
			return p, true
		}
	}
	return nil, false
}
