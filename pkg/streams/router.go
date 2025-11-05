package streams

import (
	"context"
	"errors"
	"fmt"
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
}

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

func (r *router) Start(ctx context.Context) {
	go r.heartbeatLoop(ctx)
	go r.consumeLoop(ctx)
	go r.autoClaimLoop(ctx)
	go r.janitorLoop(ctx)
}

func (r *router) Stop(context.Context) {
	atomic.StoreInt32(&r.stopFlag, 1)
}

func (r *router) PublishToClient(ctx context.Context, to string, payload []byte) error {
	podID, found, err := r.lookup(to)
	if err != nil {
		return err
	}
	if !found {
		// Client offline, nothing to do
		return nil
	}
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

func (r *router) processMessage(ctx context.Context, stream, group string, msg redis.XMessage) {
	clientID, _ := msg.Values["clientId"].(string)
	payloadStr, _ := msg.Values["payload"].(string)
	payload := []byte(payloadStr)
	ack := func() { _ = r.rdb.XAck(ctx, stream, group, msg.ID).Err() }

	if clientID == "" {
		ack()
		return
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
	if err != nil {
		log.WithError(err).Warn("streams: lookup error")
		// best-effort ack to avoid stuck pendings
		ack()
		return
	}
	if !found {
		ack()
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
	// Re-publish to target pod and ack original
	if err := r.PublishToClient(ctx, clientID, payload); err != nil {
		log.WithError(err).Warn("streams: re-publish failed")
	}
	log.WithFields(log.Fields{"from": r.podID, "to": podID, "client": clientID}).Debug("streams: re-routed to pod")
	ack()
}

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

// janitorLoop scans for streams owned by dead pods and drains them to the correct live pods.
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

	// Reclaim pendings first
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

	// Drain unseen entries (if any)
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

	// If stream is empty and no pendings, destroy group and delete stream
	if xp, err := r.rdb.XPending(ctx, stream, group).Result(); err == nil && xp != nil && xp.Count == 0 {
		if ln, err := r.rdb.XLen(ctx, stream).Result(); err == nil && ln == 0 {
			_ = r.rdb.XGroupDestroy(ctx, stream, group).Err()
			_ = r.rdb.Del(ctx, stream).Err()
		}
	}

	return nil
}

func (r *router) acquireLock(ctx context.Context, key string, lease time.Duration) (string, bool, error) {
	token := fmt.Sprintf("%s-%d", r.podID, time.Now().UnixNano())
	ok, err := r.rdb.SetNX(ctx, key, token, lease).Result()
	return token, ok, err
}

func (r *router) releaseLock(ctx context.Context, key, token string) error {
	// Lua: if value == token then DEL
	script := redis.NewScript(`if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`)
	return script.Run(ctx, r.rdb, []string{key}, token).Err()
}

func streamKeyForPod(podID string) string { return "ws:pod:" + podID }
func groupForPod(podID string) string     { return "wsgrp:" + podID }
func heartbeatKey(podID string) string    { return "ws:pod:hb:" + podID }

func isBusyGroupErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "BUSYGROUP")
}

func isNoGroupErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "NOGROUP")
}

func isUnblockedErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNBLOCKED")
}

func int64OrDefault(v int64, def int64) int64 {
	if v > 0 {
		return v
	}
	return def
}
