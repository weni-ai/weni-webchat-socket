package websocket

import (
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
