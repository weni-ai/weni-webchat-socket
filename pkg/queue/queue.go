package queue

import (
	"errors"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/config"
	log "github.com/sirupsen/logrus"

	"github.com/adjust/rmq/v4"
)

var KeysExpiration time.Duration = 10 * time.Minute

func SetKeysExpiration(minutes int64) {
	KeysExpiration = time.Duration(minutes) * time.Minute
}

// Connection encapsulates the logic of queue connection
type Connection interface {
	Close() error
	OpenQueue(string) Queue
	NewCleaner() error
}

type connection struct {
	redisClient *redis.Client
	rmqConn     rmq.Connection
	openQueues  []Queue
}

// OpenConnection opens and returns a new connection
func OpenConnection(name string, redisClient *redis.Client, errorChan chan<- error) Connection {
	conn, err := rmq.OpenConnectionWithRedisClient(name, redisClient, errorChan)
	if err != nil {
		log.Error("unable to open Redis connection: ", err)
		panic(err)
	}
	return &connection{
		redisClient: redisClient,
		rmqConn:     conn,
	}
}

// Close closes the queue connection
func (c *connection) Close() error {
	<-c.rmqConn.StopAllConsuming()
	return c.rmqConn.StopHeartbeat()
}

// NewCleaner create a new cleaner that clean rmq unused resources
func (c *connection) NewCleaner() error {
	cleanBatchSize := config.Get().RedisQueue.CleanBatchSize
	cleaner := rmq.NewCleaner(c.rmqConn)

	if cleanBatchSize == 0 {
		return nil
	}

	go func() {
		for range time.Tick(time.Second * 5) {
			log.Debug("cleaning...")
			cleaned, err := cleaner.CleanInBatches(cleanBatchSize)
			if err != nil {
				log.Debugf("error cleaning: %v", err)
			}
			if cleaned > 0 {
				log.Infof("cleaned %d connections", cleaned)
			}
		}
	}()

	if cleaner == nil {
		return errors.New("cleaner could not be created")
	}
	return nil
}

// Queue encapsulates the logic of queue
type Queue interface {
	Publish(string) error
	PublishEX(time.Duration, string) error
	SetPushQueue(Queue)
	StartConsuming(int64, time.Duration) error
	AddConsumer(string, rmq.Consumer) (string, error)
	AddConsumerFunc(tag string, consumerFunc func(rmq.Delivery)) (string, error)
	Queue() rmq.Queue
	PrefetchLimit() int64
	PollDuration() time.Duration
	SetPrefetchLimit(int64)
	SetPollDuration(time.Duration)
	Close()
	Destroy()
}

type queue struct {
	rmqQueue      rmq.Queue
	prefetchLimit int64
	pollDuration  time.Duration
}

// OpenQueue open a queue and make it available to publish or consume
func (c *connection) OpenQueue(name string) Queue {
	q, err := c.rmqConn.OpenQueue(name)
	if err != nil {
		log.Error("Unable to open queue: ", err)
		panic(err)
	}
	q.SetPushQueue(q)

	queue := &queue{
		rmqQueue:      q,
		prefetchLimit: config.Get().RedisQueue.ConsumerPrefetchLimit,
		pollDuration:  time.Duration(config.Get().RedisQueue.ConsumerPollDuration) * time.Millisecond,
	}
	c.openQueues = append(c.openQueues, queue)
	return queue
}

// Close closes the queue
func (q *queue) Close() {
	q.rmqQueue.StopConsuming()
}

// Destroy destroy queue key on redis
func (q *queue) Destroy() {
	q.rmqQueue.Destroy()
}

// StartConsuming starts consuming into a channel of size prefetchLimit, the pollDuration
// is the duration the queue sleeps before checking for new deliveries
func (q *queue) StartConsuming(prefetchLimit int64, pollDuration time.Duration) error {
	return q.rmqQueue.StartConsuming(
		prefetchLimit, pollDuration,
	)
}

// AddConsumer adds a consumer to the queue and returns its internal name
func (q *queue) AddConsumer(tag string, consumer rmq.Consumer) (string, error) {
	return q.rmqQueue.AddConsumer(tag, consumer)
}

// SetPushQueue sets a push queue. if a push queue is set the delivery can be moved
// from original queue to push queue.
func (q *queue) SetPushQueue(queue Queue) {
	q.rmqQueue.SetPushQueue(queue.Queue())
}

// Queue returns the rmq queue
func (q *queue) Queue() rmq.Queue {
	return q.rmqQueue
}

// AddConsumerFunc adds a consumer wich is defined only by a function.
func (q *queue) AddConsumerFunc(tag string, consumerFunc func(rmq.Delivery)) (string, error) {
	return q.rmqQueue.AddConsumerFunc(tag, consumerFunc)
}

// Publish adds a delivery with the given payload to the queue
func (q *queue) Publish(payload string) error {
	return q.rmqQueue.Publish(payload)
}

// Publish deliveries with expiration time
func (q *queue) PublishEX(expiration time.Duration, payload string) error {
	return q.rmqQueue.PublishEX(expiration, payload)
}

// PrefetchLimit returns the queue's consume prefetchLimit
func (q *queue) PrefetchLimit() int64 {
	return q.prefetchLimit
}

// PollDuration returns the queue's pollDuration
func (q *queue) PollDuration() time.Duration {
	return q.pollDuration
}

// SetPrefetchLimit sets the queue's consume prefetchLimit
func (q *queue) SetPrefetchLimit(limit int64) {
	q.prefetchLimit = limit
}

// SetPollDuration sets the queue's pollDuration
func (q *queue) SetPollDuration(duration time.Duration) {
	q.pollDuration = duration
}
