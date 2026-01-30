// Package streams implements a Redis Streams based router used by the
// websocket proxy to fan-in/out messages across many pods. Each pod
// consumes from its own stream, re-routes messages when a client moves,
// and drains/deletes streams for dead pods to ensure eventual delivery.
package streams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"strings"

	"github.com/go-redis/redis/v8"
	log "github.com/sirupsen/logrus"
)

// DeliverFunc delivers a raw JSON payload to a local websocket client by id.
// It should parse the payload as needed and call the client's Send method.
type DeliverFunc func(clientID string, raw []byte) error

// IsLocalFunc returns true if the given client id is connected to this process (pod).
type IsLocalFunc func(clientID string) bool

// LookupClientFunc returns the pod id where the client is currently connected.
// found=false means the client is offline.
type LookupClientFunc func(clientID string) (podID string, found bool, err error)

// StreamsConfig holds Redis Streams operational parameters.
type StreamsConfig struct {
	StreamsMaxLenApprox int64 // MAXLEN ~ N on XADD (0 disables trimming)
	StreamsReadCount    int64 // COUNT for XREADGROUP/XAUTOCLAIM
	StreamsBlockMs      int64 // BLOCK milliseconds for XREADGROUP
	StreamsClaimIdleMs  int64 // Min idle time in ms before a pending is eligible to be reclaimed
	HeartbeatTTLSeconds int64 // TTL seconds for pod heartbeat key
	JanitorIntervalMs   int64 // Interval to scan and drain dead pod streams
	JanitorLeaseMs      int64 // Lease time for the distributed lock when draining
	StreamsRetentionMs  int64 // Optional time-based trim; 0 disables
	StreamsMaxPendingMs int64 // Max age in ms before pending messages are ACKed and dropped; 0 disables
	DeadPodRetentionMs  int64 // Retention time for dead pod stream cleanup; messages older than this are trimmed
}

// Router exposes publish and consume behaviors for per-pod streams.
type Router interface {
	Start(ctx context.Context)
	Stop(ctx context.Context)
	PublishToClient(ctx context.Context, to string, payload []byte) error
}

type router struct {
	rdb     *redis.Client
	podID   string
	cfg     StreamsConfig
	deliver DeliverFunc
	isLocal IsLocalFunc
	lookup  LookupClientFunc

	stopFlag int32
	// observability counters
	lookupErrCount int64
	rerouteCount   int64
	lastXLen       int64
}

// NewRouter constructs a new Router bound to the given pod id and Redis
// client. The lookup function resolves client -> pod, isLocal checks
// whether a client is attached to the current pod, and deliver writes to
// the in-memory websocket connection.
func NewRouter(rdb *redis.Client, podID string, cfg StreamsConfig, lookup LookupClientFunc, isLocal IsLocalFunc, deliver DeliverFunc) Router {
	return &router{
		rdb:     rdb,
		podID:   podID,
		cfg:     cfg,
		deliver: deliver,
		isLocal: isLocal,
		lookup:  lookup,
	}
}

// Start launches the router background loops: heartbeat, consumer, auto
// claim for idle pendings, dead-pod janitor, and presence cleanup.
func (r *router) Start(ctx context.Context) {
	go r.heartbeatLoop(ctx)
	go r.consumeLoop(ctx)
	go r.autoClaimLoop(ctx)
	go r.janitorLoop(ctx)
	go r.presenceCleanupLoop(ctx)
	go r.trimLoop(ctx)
	go r.observabilityLoop(ctx)
}

// Stop requests all background loops to stop on their next iteration.
func (r *router) Stop(context.Context) {
	atomic.StoreInt32(&r.stopFlag, 1)
}

// PublishToClient routes a message to the client by resolving its current
// pod and appending an entry to that pod's stream. If the client is offline
// the call is a no-op.
func (r *router) PublishToClient(ctx context.Context, to string, payload []byte) error {
	log.Debugf("publishing message to client %s", to)
	podID, found, err := r.lookup(to)
	if err != nil {
		log.Debugf("lookup client %s failed, error: %v", to, err)
		return err
	}
	if !found {
		// Client offline, nothing to do
		log.Debugf("client %s is offline, nothing to do", to)
		return nil
	}
	log.Debugf("client %s is found, pod: %s", to, podID)
	stream := streamKeyForPod(podID)
	args := &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{
			"clientId": to,
			"payload":  string(payload),
		},
	}
	if r.cfg.StreamsMaxLenApprox > 0 {
		args.Approx = true
		args.MaxLen = r.cfg.StreamsMaxLenApprox
	}
	log.Debugf("publishing message to client %s, args: %+v", to, args)
	return r.rdb.XAdd(ctx, args).Err()
}

// consumeLoop blocks on XREADGROUP for this pod's stream and delivers each
// entry to a local client or re-publishes to another pod when needed.
func (r *router) consumeLoop(ctx context.Context) {
	stream := streamKeyForPod(r.podID)
	group := groupForPod(r.podID)
	consumer := fmt.Sprintf("%s-%d", r.podID, time.Now().UnixNano())

	// Ensure the group exists (idempotent)
	if err := r.rdb.XGroupCreateMkStream(ctx, stream, group, "0-0").Err(); err != nil {
		if !isBusyGroupErr(err) {
			log.WithError(err).Error("streams: failed to create consumer group")
		}
	}

	block := time.Duration(r.cfg.StreamsBlockMs) * time.Millisecond
	if block <= 0 {
		block = 5 * time.Second
	}

	for atomic.LoadInt32(&r.stopFlag) == 0 {
		// Periodically check stop flag by using a finite block time
		res, err := r.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    int64OrDefault(r.cfg.StreamsReadCount, 100),
			Block:    block,
			NoAck:    false,
		}).Result()

		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			if err == redis.Nil {
				// timeout
				continue
			}
			// Ensure group/stream exists for both NOGROUP and UNBLOCKED cases
			if isUnblockedErr(err) || isNoGroupErr(err) {
				if cgErr := r.rdb.XGroupCreateMkStream(ctx, stream, group, "0-0").Err(); cgErr != nil && !isBusyGroupErr(cgErr) {
					log.WithError(cgErr).Warn("streams: failed to ensure group after XREADGROUP error")
				}
				continue
			}
			log.WithError(err).Warn("streams: XREADGROUP error")
			continue
		}

		for _, strm := range res {
			for _, msg := range strm.Messages {
				r.processMessage(ctx, stream, group, msg)
			}
		}
	}
}

// messageAgeMs extracts the timestamp from a Redis Stream message ID (format: timestamp-sequence)
// and returns the age in milliseconds. Returns 0 if the ID cannot be parsed.
func messageAgeMs(msgID string) int64 {
	parts := strings.Split(msgID, "-")
	if len(parts) < 1 {
		return 0
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0
	}
	return time.Now().UnixMilli() - ts
}

// retryDecision represents the outcome of shouldRetryMessage.
type retryDecision int

const (
	retryKeepPending retryDecision = iota // Leave message pending for retry
	retryDropMessage                      // ACK and drop the message
)

// shouldRetryMessage determines whether a message should be retried or dropped
// based on its age and the reason for retry (lookup error vs client offline).
// Returns retryKeepPending to leave the message for XAUTOCLAIM retry, or
// retryDropMessage to ACK and discard it.
func (r *router) shouldRetryMessage(msgID string, isLookupError bool) (retryDecision, int64) {
	ageMs := messageAgeMs(msgID)
	maxMs := r.cfg.StreamsMaxPendingMs
	if maxMs <= 0 {
		maxMs = 60000 // default 60s
	}

	// Lookup errors get full retry window (client might be online)
	// Client offline gets half window (confirmed disconnected)
	windowMs := maxMs
	if !isLookupError {
		windowMs = maxMs / 2
	}

	if ageMs > windowMs {
		return retryDropMessage, ageMs
	}
	return retryKeepPending, ageMs
}

// processMessage handles a single stream message: it delivers to a local
// connection when available, otherwise re-routes to the authoritative pod.
// It always ACKs the original message when finished.
func (r *router) processMessage(ctx context.Context, stream, group string, msg redis.XMessage) {
	clientID, _ := msg.Values["clientId"].(string)
	payloadStr, _ := msg.Values["payload"].(string)
	payload := []byte(payloadStr)
	ack := func() { _ = r.rdb.XAck(ctx, stream, group, msg.ID).Err() }

	if clientID == "" {
		ack()
		return
	}

	// Check if message is too old and should be dropped to prevent unbounded pending growth
	if r.cfg.StreamsMaxPendingMs > 0 {
		ageMs := messageAgeMs(msg.ID)
		if ageMs > r.cfg.StreamsMaxPendingMs {
			log.WithFields(log.Fields{
				"stream": stream,
				"client": clientID,
				"msg_id": msg.ID,
				"age_ms": ageMs,
				"max_ms": r.cfg.StreamsMaxPendingMs,
			}).Warn("streams: dropping message that exceeded max pending age")
			ack()
			return
		}
	}

	// If local, try deliver immediately
	if r.isLocal(clientID) {
		if err := r.deliver(clientID, payload); err != nil {
			// On failure, re-check presence and re-route if moved
			if podID, found, _ := r.lookup(clientID); found && podID != r.podID {
				_ = r.PublishToClient(ctx, clientID, payload)
			}
			ack()
			return
		}
		log.WithFields(log.Fields{"stream": stream, "client": clientID}).Trace("streams: delivered locally")
		ack()
		return
	}

	// Not local, find current pod and re-route or drop if offline
	podID, found, err := r.lookup(clientID)
	// Handle lookup errors and client-not-found with retry logic
	if err != nil || !found {
		isLookupErr := err != nil
		decision, ageMs := r.shouldRetryMessage(msg.ID, isLookupErr)

		logFields := log.Fields{
			"stream": stream,
			"client": clientID,
			"msg_id": msg.ID,
			"age_ms": ageMs,
		}

		if decision == retryDropMessage {
			if isLookupErr {
				log.WithError(err).WithFields(logFields).Warn("streams: lookup error, message too old - dropping")
			} else {
				log.WithFields(logFields).Warn("streams: client offline, message too old - dropping")
			}
			ack()
			return
		}

		// Keep pending for retry via XAUTOCLAIM
		if isLookupErr {
			log.WithError(err).WithFields(logFields).Warn("streams: lookup error - leaving pending for retry")
			atomic.AddInt64(&r.lookupErrCount, 1)
		} else {
			log.WithFields(logFields).Debug("streams: client offline - leaving pending for reconnect")
		}
		return
	}
	if podID == r.podID {
		// Rare race: local presence not yet visible; try deliver once
		if err := r.deliver(clientID, payload); err != nil {
			ack()
			return
		}
		log.WithFields(log.Fields{"stream": stream, "client": clientID}).Trace("streams: delivered on race-local")
		ack()
		return
	}
	// Avoid re-publishing back to the same (source) stream when that pod is alive.
	// If the source pod is dead (no heartbeat), re-publish to the current pod's
	// stream to guarantee forward progress.
	if srcPod := podIDFromStream(stream); srcPod != "" && srcPod == podID {
		if srcPod != r.podID {
			if exists, _ := r.rdb.Exists(ctx, heartbeatKey(srcPod)).Result(); exists == 0 {
				_ = r.publishToPod(ctx, r.podID, clientID, payload)
			}
		}
		ack()
		return
	}
	// Re-publish to target pod and ack original
	if err := r.PublishToClient(ctx, clientID, payload); err != nil {
		log.WithError(err).Warn("streams: re-publish failed")
	}
	atomic.AddInt64(&r.rerouteCount, 1)
	log.WithFields(log.Fields{"from": r.podID, "to": podID, "client": clientID}).Debug("streams: re-routed to pod")
	ack()
}

// autoClaimLoop periodically reclaims idle pending messages for this pod's
// consumer group using XAUTOCLAIM, ensuring at-least-once delivery on restarts.
func (r *router) autoClaimLoop(ctx context.Context) {
	stream := streamKeyForPod(r.podID)
	group := groupForPod(r.podID)
	consumer := fmt.Sprintf("%s-claimer-%d", r.podID, time.Now().UnixNano())
	minIdle := time.Duration(r.cfg.StreamsClaimIdleMs) * time.Millisecond
	if minIdle <= 0 {
		minIdle = 60 * time.Second
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	start := "0-0"
	for atomic.LoadInt32(&r.stopFlag) == 0 {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			msgs, nextStart, err := r.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
				Stream:   stream,
				Group:    group,
				Consumer: consumer,
				MinIdle:  minIdle,
				Start:    start,
				Count:    int64OrDefault(r.cfg.StreamsReadCount, 100),
			}).Result()
			if err != nil {
				if err != redis.Nil {
					if isNoGroupErr(err) {
						// Create group and retry next tick
						if cgErr := r.rdb.XGroupCreateMkStream(ctx, stream, group, "0-0").Err(); cgErr != nil && !isBusyGroupErr(cgErr) {
							log.WithError(cgErr).Trace("streams: failed to create group after NOGROUP in XAUTOCLAIM")
						}
					} else {
						log.WithError(err).Trace("streams: XAUTOCLAIM error")
					}
				}
				continue
			}
			for _, msg := range msgs {
				r.processMessage(ctx, stream, group, msg)
			}
			start = nextStart
		}
	}
}

// heartbeatLoop refreshes the per-pod heartbeat key so other pods can detect
// liveness and safely drain streams when a pod dies.
func (r *router) heartbeatLoop(ctx context.Context) {
	ttl := time.Duration(r.cfg.HeartbeatTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 10 * time.Second
	}
	interval := ttl / 2
	key := heartbeatKey(r.podID)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for atomic.LoadInt32(&r.stopFlag) == 0 {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.rdb.Set(ctx, key, "1", ttl).Err(); err != nil {
				log.WithError(err).Warn("streams: heartbeat set failed")
			}
		}
	}
}

// janitorLoop scans for streams owned by pods without a heartbeat and drains
// them by re-publishing messages to their latest target pods.
func (r *router) janitorLoop(ctx context.Context) {
	interval := time.Duration(r.cfg.JanitorIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for atomic.LoadInt32(&r.stopFlag) == 0 {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// find candidate stream keys
			var cursor uint64
			for {
				keys, cur, err := r.rdb.Scan(ctx, cursor, "ws:pod:*", 1000).Result()
				if err != nil {
					log.WithError(err).Trace("streams: janitor scan error")
					break
				}
				cursor = cur
				for _, key := range keys {
					// skip heartbeat keys
					if strings.HasPrefix(key, "ws:pod:hb:") {
						continue
					}
					podID := strings.TrimPrefix(key, "ws:pod:")
					if podID == "" || podID == r.podID {
						continue
					}
					// if heartbeat missing, consider dead
					hbKey := heartbeatKey(podID)
					exists, err := r.rdb.Exists(ctx, hbKey).Result()
					if err != nil || exists == 1 {
						continue
					}
					lockKey := "ws:janitor:drain:" + podID
					lease := time.Duration(r.cfg.JanitorLeaseMs) * time.Millisecond
					if lease <= 0 {
						lease = 30 * time.Second
					}
					token, ok, err := r.acquireLock(ctx, lockKey, lease)
					if err != nil || !ok {
						continue
					}
					// best-effort drain and then release lock
					_ = r.drainDeadPod(ctx, podID)
					_ = r.releaseLock(ctx, lockKey, token)
				}
				if cursor == 0 {
					break
				}
			}
		}
	}
}

// drainDeadPod reclaims pending and unseen entries from a dead pod's stream,
// re-publishes them to the correct pod, ACKs originals, and deletes the stream.
// For dead pods, we aggressively trim and force-ACK to ensure cleanup.
func (r *router) drainDeadPod(ctx context.Context, deadPod string) error {
	stream := streamKeyForPod(deadPod)
	group := groupForPod(deadPod)
	consumer := fmt.Sprintf("%s-janitor-%d", r.podID, time.Now().UnixNano())

	// Ensure group exists to read with a group
	if err := r.rdb.XGroupCreateMkStream(ctx, stream, group, "0-0").Err(); err != nil {
		if !isBusyGroupErr(err) {
			return err
		}
	}

	// Step 1: Apply time-based trimming to dead pod stream.
	// Use configured dead pod retention or default to 1 hour.
	retentionMs := r.cfg.DeadPodRetentionMs
	if retentionMs <= 0 {
		retentionMs = 3600000 // 1 hour default for dead pod cleanup
	}
	cutMs := time.Now().Add(-time.Duration(retentionMs)*time.Millisecond).UnixNano() / 1e6
	minID := fmt.Sprintf("%d-0", cutMs)
	if err := r.rdb.Do(ctx, "XTRIM", stream, "MINID", "~", minID).Err(); err != nil && err != redis.Nil {
		log.WithError(err).WithField("stream", stream).Trace("streams: janitor XTRIM error")
	}

	// Step 2: Reclaim and process pending messages (try to reroute to current pods)
	minIdle := time.Duration(r.cfg.StreamsClaimIdleMs) * time.Millisecond
	if minIdle <= 0 {
		minIdle = 60 * time.Second
	}
	start := "0-0"
	for {
		msgs, nextStart, err := r.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   stream,
			Group:    group,
			Consumer: consumer,
			MinIdle:  minIdle,
			Start:    start,
			Count:    int64OrDefault(r.cfg.StreamsReadCount, 100),
		}).Result()
		if err != nil && err != redis.Nil {
			log.WithError(err).Trace("streams: janitor XAUTOCLAIM error")
			break
		}
		if len(msgs) == 0 {
			break
		}
		for _, m := range msgs {
			r.processMessage(ctx, stream, group, m)
		}
		start = nextStart
	}

	// Step 3: Drain unseen entries (if any)
	for i := 0; i < 10; i++ { // bounded loops to avoid long locks
		res, err := r.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    int64OrDefault(r.cfg.StreamsReadCount, 100),
			Block:    100 * time.Millisecond,
		}).Result()
		if err == redis.Nil {
			break
		}
		if err != nil {
			log.WithError(err).Trace("streams: janitor XREADGROUP error")
			break
		}
		empty := true
		for _, s := range res {
			for _, m := range s.Messages {
				empty = false
				r.processMessage(ctx, stream, group, m)
			}
		}
		if empty {
			break
		}
	}

	// Step 4: Force-ACK all remaining pending messages.
	// The pod is dead, so these messages cannot be delivered through this stream.
	// Any that could be rerouted were handled in processMessage above.
	r.forceAckAllPending(ctx, stream, group)

	// Step 5: Check stream state and delete if empty
	xlen, _ := r.rdb.XLen(ctx, stream).Result()
	pendCount := int64(0)
	if xp, err := r.rdb.XPending(ctx, stream, group).Result(); err == nil && xp != nil {
		pendCount = xp.Count
	}

	if xlen == 0 && pendCount == 0 {
		// Stream is empty - safe to delete
		_ = r.rdb.XGroupDestroy(ctx, stream, group).Err()
		if err := r.rdb.Del(ctx, stream).Err(); err == nil {
			log.WithField("dead_pod", deadPod).Info("streams: deleted empty dead pod stream")
		}
	} else {
		// Stream still has messages - will be cleaned on next iteration
		log.WithFields(log.Fields{
			"dead_pod":   deadPod,
			"xlen":       xlen,
			"pend_count": pendCount,
		}).Debug("streams: dead pod stream still has messages after drain attempt")
	}

	return nil
}

// forceAckAllPending acknowledges all pending messages in the given stream/group.
// Used for dead pod cleanup where messages can no longer be delivered.
func (r *router) forceAckAllPending(ctx context.Context, stream, group string) {
	// Get all pending message IDs and ACK them in batches
	for {
		// Use XPENDING with range to get message IDs
		pending, err := r.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: stream,
			Group:  group,
			Start:  "-",
			End:    "+",
			Count:  500,
		}).Result()
		if err != nil {
			if err != redis.Nil {
				log.WithError(err).Trace("streams: forceAckAllPending XPENDING error")
			}
			break
		}
		if len(pending) == 0 {
			break
		}

		// Collect message IDs to ACK
		ids := make([]string, 0, len(pending))
		for _, p := range pending {
			ids = append(ids, p.ID)
		}

		// ACK all at once
		if err := r.rdb.XAck(ctx, stream, group, ids...).Err(); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"stream": stream,
				"count":  len(ids),
			}).Trace("streams: forceAckAllPending XACK error")
			break
		}

		log.WithFields(log.Fields{
			"stream": stream,
			"count":  len(ids),
		}).Debug("streams: force-ACKed pending messages from dead pod stream")

		// If we got fewer than requested, we're done
		if len(pending) < 500 {
			break
		}
	}
}

// acquireLock obtains a best-effort distributed lock with a lease TTL using
// SET NX PX. The returned token must be supplied to releaseLock.
func (r *router) acquireLock(ctx context.Context, key string, lease time.Duration) (string, bool, error) {
	token := fmt.Sprintf("%s-%d", r.podID, time.Now().UnixNano())
	ok, err := r.rdb.SetNX(ctx, key, token, lease).Result()
	return token, ok, err
}

// releaseLock releases a previously acquired lock by comparing the token in a
// Lua script and deleting the key only if it still matches.
func (r *router) releaseLock(ctx context.Context, key, token string) error {
	// Lua: if value == token then DEL
	script := redis.NewScript(`if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`)
	return script.Run(ctx, r.rdb, []string{key}, token).Err()
}

// streamKeyForPod returns the Redis Streams key used as the inbox for a pod.
func streamKeyForPod(podID string) string { return "ws:pod:" + podID }

// groupForPod returns the consumer group name associated with a pod's stream.
func groupForPod(podID string) string { return "wsgrp:" + podID }

// heartbeatKey returns the Redis key used to store a pod's liveness heartbeat.
func heartbeatKey(podID string) string { return "ws:pod:hb:" + podID }

// publishToPod appends a message directly to the given pod's stream,
// bypassing the lookup step. Used to move messages off dead streams.
func (r *router) publishToPod(ctx context.Context, podID, to string, payload []byte) error {
	stream := streamKeyForPod(podID)
	args := &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{
			"clientId": to,
			"payload":  string(payload),
		},
	}
	if r.cfg.StreamsMaxLenApprox > 0 {
		args.Approx = true
		args.MaxLen = r.cfg.StreamsMaxLenApprox
	}
	return r.rdb.XAdd(ctx, args).Err()
}

// isBusyGroupErr reports whether the error is a BUSYGROUP creation error.
func isBusyGroupErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "BUSYGROUP")
}

// isNoGroupErr reports whether the error is a NOGROUP error from Redis Streams.
func isNoGroupErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "NOGROUP")
}

// podIDFromStream extracts the pod id suffix from a stream key.
func podIDFromStream(stream string) string {
	const p = "ws:pod:"
	if strings.HasPrefix(stream, p) {
		return stream[len(p):]
	}
	return ""
}

// presenceCleanupLoop prunes stale entries from ws:clients by verifying that
// the mapped pod has no heartbeat and that the client's last-seen timestamp
// is older than a conservative threshold. A Lua script ensures atomicity.
func (r *router) presenceCleanupLoop(ctx context.Context) {
	interval := time.Duration(r.cfg.JanitorIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	type minimalClient struct {
		PodID string `json:"pod_id"`
	}

	threshold := time.Duration(r.cfg.HeartbeatTTLSeconds) * time.Second * 6 // ~6x TTL by default
	if threshold <= 0 {
		threshold = 2 * time.Minute
	}

	for atomic.LoadInt32(&r.stopFlag) == 0 {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cursor := uint64(0)
			now := time.Now()
			cleanupScript := redis.NewScript(`
			local clientsKey = KEYS[1]
			local hbKey = KEYS[2]
			local lastSeenKey = KEYS[3]
			local clientId = ARGV[1]
			local expectedRaw = ARGV[2]
			local cutoff = tonumber(ARGV[3])
			local current = redis.call('HGET', clientsKey, clientId)
			if (not current) or current ~= expectedRaw then return 0 end
			if redis.call('EXISTS', hbKey) == 1 then return 0 end
			local last = redis.call('ZSCORE', lastSeenKey, clientId)
			if (not last) then return 0 end
			if tonumber(last) < cutoff then
				redis.call('ZREM', lastSeenKey, clientId)
				return redis.call('HDEL', clientsKey, clientId)
			end
			return 0`)
			for {
				res, cur, err := r.rdb.HScan(ctx, "ws:clients", cursor, "*", 500).Result()
				if err != nil {
					log.WithError(err).Trace("streams: presence HSCAN error")
					break
				}
				cursor = cur
				// res is [field1, value1, field2, value2, ...]
				for i := 0; i+1 < len(res); i += 2 {
					clientID := res[i]
					raw := res[i+1]
					var mc minimalClient
					if err := json.Unmarshal([]byte(raw), &mc); err != nil || mc.PodID == "" {
						continue
					}
					// If pod heartbeat missing, attempt atomic cleanup
					if exists, _ := r.rdb.Exists(ctx, heartbeatKey(mc.PodID)).Result(); exists == 0 {
						cutoff := float64(now.Add(-threshold).Unix())
						_ = cleanupScript.Run(ctx, r.rdb, []string{
							"ws:clients",
							heartbeatKey(mc.PodID),
							"ws:clients:lastseen",
						}, clientID, raw, cutoff).Err()
					}
				}
				if cursor == 0 {
					break
				}
			}
		}
	}
}

// isUnblockedErr reports whether Redis unblocked a blocked read due to key
// deletion for the stream being read.
func isUnblockedErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNBLOCKED")
}

// int64OrDefault returns v when positive, otherwise def.
func int64OrDefault(v int64, def int64) int64 {
	if v > 0 {
		return v
	}
	return def
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// compareStreamIDs compares Redis Stream IDs "ms-seq". Returns -1 if a<b, 0 if equal, 1 if a>b.
func compareStreamIDs(a, b string) int {
	as := strings.SplitN(a, "-", 2)
	bs := strings.SplitN(b, "-", 2)
	if len(as) < 2 || len(bs) < 2 {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}
	if as[0] != bs[0] {
		if as[0] < bs[0] {
			return -1
		}
		return 1
	}
	if as[1] != bs[1] {
		if as[1] < bs[1] {
			return -1
		}
		return 1
	}
	return 0
}

// trimLoop performs optional time-based trimming using XTRIM MINID ~,
// while preserving any pending entries by keeping at least the oldest PEL ID.
func (r *router) trimLoop(ctx context.Context) {
	if r.cfg.StreamsRetentionMs <= 0 {
		return
	}
	stream := streamKeyForPod(r.podID)
	group := groupForPod(r.podID)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for atomic.LoadInt32(&r.stopFlag) == 0 {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// compute retention cutoff id
			cutMs := time.Now().Add(-time.Duration(r.cfg.StreamsRetentionMs)*time.Millisecond).UnixNano() / 1e6
			retentionMinID := fmt.Sprintf("%d-0", cutMs)
			minIDToKeep := retentionMinID
			// keep at least the oldest pending id
			if xp, err := r.rdb.XPending(ctx, stream, group).Result(); err == nil && xp != nil && xp.Count > 0 && xp.Lower != "" {
				if compareStreamIDs(xp.Lower, minIDToKeep) > 0 {
					minIDToKeep = xp.Lower
				}
			}
			// XTRIM MINID ~ minIDToKeep
			if err := r.rdb.Do(ctx, "XTRIM", stream, "MINID", "~", minIDToKeep).Err(); err != nil && err != redis.Nil {
				log.WithError(err).Trace("streams: XTRIM MINID error")
			} else {
				log.WithFields(log.Fields{
					"pod":          r.podID,
					"min_id_kept":  minIDToKeep,
					"retention_ms": r.cfg.StreamsRetentionMs,
				}).Info("STREAM_TRIM")
			}
		}
	}
}

// observabilityLoop periodically logs key stream/presence metrics and emits simple alerts.
func (r *router) observabilityLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	stream := streamKeyForPod(r.podID)
	group := groupForPod(r.podID)
	for atomic.LoadInt32(&r.stopFlag) == 0 {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nowMs := time.Now().UnixNano() / 1e6
			xlen, _ := r.rdb.XLen(ctx, stream).Result()
			// stream memory (best-effort; requires Redis memory command)
			memUsage, _ := r.rdb.Do(ctx, "MEMORY", "USAGE", stream).Int64()
			var pendCount int64
			var oldestPendingAgeMs int64
			if xp, err := r.rdb.XPending(ctx, stream, group).Result(); err == nil && xp != nil {
				pendCount = xp.Count
				if xp.Count > 0 && xp.Lower != "" {
					if parts := strings.SplitN(xp.Lower, "-", 2); len(parts) > 0 {
						if ms, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
							oldestPendingAgeMs = nowMs - ms
						}
					}
				}
			}
			clientsCount, _ := r.rdb.HLen(ctx, "ws:clients").Result()
			lastSeenCount, _ := r.rdb.ZCard(ctx, "ws:clients:lastseen").Result()
			lookupErrs := atomic.SwapInt64(&r.lookupErrCount, 0)
			reroutes := atomic.SwapInt64(&r.rerouteCount, 0)
			xlenDelta := xlen - atomic.LoadInt64(&r.lastXLen)
			atomic.StoreInt64(&r.lastXLen, xlen)
			log.WithFields(log.Fields{
				"pod":               r.podID,
				"stream":            stream,
				"xlen":              xlen,
				"xlen_delta":        xlenDelta,
				"stream_mem_bytes":  memUsage,
				"pend_count":        pendCount,
				"pend_oldest_ms":    oldestPendingAgeMs,
				"clients_hash_len":  clientsCount,
				"lastseen_zset_len": lastSeenCount,
				"cfg_maxlen":        r.cfg.StreamsMaxLenApprox,
				"cfg_claim_idle_ms": r.cfg.StreamsClaimIdleMs,
				"cfg_retention_ms":  r.cfg.StreamsRetentionMs,
				"lookup_errs_30s":   lookupErrs,
				"reroutes_30s":      reroutes,
			}).Info("STREAM_STATUS")
			if r.cfg.StreamsMaxLenApprox > 0 {
				backlogPct := float64(xlen) / float64(r.cfg.StreamsMaxLenApprox) * 100
				if backlogPct >= 80 {
					// ALERT_BACKLOG_HIGH:
					// Meaning: Stream backlog (XLEN) >= 80% of configured MaxLen.
					// Likely cause: Consumers not keeping up or retention too lax.
					// Tweak: Increase StreamsReadCount, lower StreamsBlockMs, enable StreamsRetentionMs, or scale pods.
					log.WithFields(log.Fields{
						"pod":           r.podID,
						"xlen":          xlen,
						"maxlen":        r.cfg.StreamsMaxLenApprox,
						"backlog_pct":   int(backlogPct),
						"suggest_read":  r.cfg.StreamsReadCount * 2,
						"suggest_block": int64(max(1000, int(r.cfg.StreamsBlockMs/2))),
						"suggest_retms": max64(60000, r.cfg.StreamsRetentionMs),
					}).Warn("ALERT_BACKLOG_HIGH: increase StreamsReadCount, lower StreamsBlockMs, enable StreamsRetentionMs, or scale pods")
				}
			}
			if pendCount > 0 && oldestPendingAgeMs > (r.cfg.StreamsClaimIdleMs*3) {
				// ALERT_PENDING_STUCK:
				// Meaning: PEL contains messages older than 3x claim idle; retries too slow or consumer errors.
				// Tweak: Lower StreamsClaimIdleMs, increase StreamsReadCount; check consumer send failures.
				log.WithFields(log.Fields{
					"pod":                r.podID,
					"pend_count":         pendCount,
					"pend_oldest_ms":     oldestPendingAgeMs,
					"cfg_claim_idle_ms":  r.cfg.StreamsClaimIdleMs,
					"suggest_claim_idle": max64(5000, r.cfg.StreamsClaimIdleMs/2),
					"suggest_read_count": r.cfg.StreamsReadCount + 50,
				}).Warn("ALERT_PENDING_STUCK: lower StreamsClaimIdleMs and/or increase StreamsReadCount; check consumer errors")
			}
			if lookupErrs > 100 {
				// ALERT_LOOKUP_RETRIES_HIGH:
				// Meaning: High presence/lookup errors; messages left pending for auto-claim retry.
				// Tweak: Investigate Redis latency/presence; lower StreamsClaimIdleMs to retry faster.
				log.WithFields(log.Fields{
					"pod":          r.podID,
					"lookup_errs":  lookupErrs,
					"suggest_idle": max64(5000, r.cfg.StreamsClaimIdleMs/2),
				}).Warn("ALERT_LOOKUP_RETRIES_HIGH: investigate Redis latency/presence; lower StreamsClaimIdleMs")
			}
			if reroutes > 1000 {
				// ALERT_REROUTE_HIGH:
				// Meaning: Many re-routes between pods; presence may be unstable.
				// Tweak: Increase Heartbeat TTL; ensure event-driven TTL refresh on activity is working.
				log.WithFields(log.Fields{
					"pod":          r.podID,
					"reroutes_30s": reroutes,
					"suggest_ttl":  r.cfg.HeartbeatTTLSeconds * 2,
				}).Warn("ALERT_REROUTE_HIGH: presence may be flapping; check heartbeat TTL and event-driven TTL refresh")
			}
			if r.cfg.StreamsRetentionMs == 0 && r.cfg.StreamsMaxLenApprox > 0 && xlen > int64(float64(r.cfg.StreamsMaxLenApprox)*0.5) {
				// ALERT_RETENTION_DISABLED:
				// Meaning: Time-based trimming disabled while backlog grows; memory may keep increasing.
				// Tweak: Enable StreamsRetentionMs (e.g., 60000) to bound memory alongside MAXLEN ~.
				log.WithFields(log.Fields{
					"pod":    r.podID,
					"xlen":   xlen,
					"maxlen": r.cfg.StreamsMaxLenApprox,
				}).Info("ALERT_RETENTION_DISABLED: consider enabling StreamsRetentionMs (e.g., 60000) to bound memory")
			}
		}
	}
}
