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
	cleanerStop chan struct{}
	cleanerDone chan struct{}
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
		cleanerStop: make(chan struct{}),
		cleanerDone: make(chan struct{}),
	}
}

// Close closes the queue connection
func (c *connection) Close() error {
	if c.cleanerStop != nil {
		close(c.cleanerStop)
		<-c.cleanerDone
	}

	<-c.rmqConn.StopAllConsuming()
	return c.rmqConn.StopHeartbeat()
}

// NewCleaner create a new cleaner that clean rmq unused resources
// The cleaner runs in a supervised goroutine that automatically restarts on failure
func (c *connection) NewCleaner() error {
	cleanBatchSize := config.Get().RedisQueue.CleanBatchSize

	if cleanBatchSize == 0 {
		close(c.cleanerDone)
		return nil
	}

	cleaner := rmq.NewCleaner(c.rmqConn)
	if cleaner == nil {
		close(c.cleanerDone)
		return errors.New("cleaner could not be created")
	}

	go c.superviseCleaner(cleaner, cleanBatchSize)

	return nil
}

// superviseCleaner manages the cleaner goroutine with auto-restart and exponential backoff
func (c *connection) superviseCleaner(cleaner *rmq.Cleaner, cleanBatchSize int64) {
	defer close(c.cleanerDone)

	const (
		maxRetries         = 5
		initialBackoff     = time.Second
		maxBackoff         = time.Minute
		backoffMultiplier  = 2
		resetFailuresAfter = 10 * time.Minute
	)

	consecutiveFailures := 0
	backoff := initialBackoff

	for {
		select {
		case <-c.cleanerStop:
			log.Info("cleaner supervisor received stop signal, shutting down")
			return
		default:
			workerDone := make(chan bool, 1)
			workerStartTime := time.Now()
			go c.runCleaner(cleaner, cleanBatchSize, workerDone)

			select {
			case success := <-workerDone:
				if success {
					return
				}

				// Check if worker ran successfully for enough time to reset failures
				uptime := time.Since(workerStartTime)
				if uptime >= resetFailuresAfter {
					if consecutiveFailures > 0 {
						log.Infof("cleaner ran successfully for %v, resetting failure counter from %d to 0", uptime, consecutiveFailures)
						consecutiveFailures = 0
					}
				}

				consecutiveFailures++
				log.Errorf("cleaner worker stopped unexpectedly (failure #%d)", consecutiveFailures)

				if consecutiveFailures >= maxRetries {
					log.Errorf("cleaner reached maximum consecutive failures (%d), giving up", maxRetries)
					return
				}

				backoff = time.Duration(float64(initialBackoff) * float64(consecutiveFailures*backoffMultiplier))
				if backoff > maxBackoff {
					backoff = maxBackoff
				}

				log.Infof("restarting cleaner in %v...", backoff)

				select {
				case <-time.After(backoff):
					log.Info("restarting cleaner worker")
				case <-c.cleanerStop:
					log.Info("stop signal received during backoff, shutting down")
					return
				}

			case <-c.cleanerStop:
				log.Info("stop signal received while worker running, shutting down")
				return
			}
		}
	}
}

// runCleaner executes the actual cleaning work in a protected goroutine
func (c *connection) runCleaner(cleaner *rmq.Cleaner, cleanBatchSize int64, done chan<- bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("cleaner worker panic recovered: %v", r)
			done <- false // Signal failure
		}
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Info("cleaner worker started")

	for {
		select {
		case <-c.cleanerStop:
			log.Info("cleaner worker received stop signal")
			done <- true
			return

		case <-ticker.C:
			log.Debug("cleaning...")
			cleaned, err := cleaner.CleanInBatches(cleanBatchSize, true, true)
			if err != nil {
				log.Debugf("error cleaning: %v", err)
			}
			if cleaned > 0 {
				log.Infof("cleaned %d connections", cleaned)
			}
		}
	}
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
