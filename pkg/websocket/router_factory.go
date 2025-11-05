package websocket

import (
    "encoding/json"

    "github.com/go-redis/redis/v8"
    "github.com/ilhasoft/wwcs/pkg/streams"
)

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
            return nil
        }
        var incoming IncomingPayload
        if err := json.Unmarshal(raw, &incoming); err != nil {
            return err
        }
        if err := client.Send(incoming); err != nil {
            return err
        }
        // Update last-seen (no-op for TTL, used by cleanup)
        _, _ = clientM.UpdateClientTTL(clientID, clientM.DefaultClientTTL())
        return nil
    }

    return streams.NewRouter(rdb, podID, cfg, lookup, isLocal, deliver)
}


