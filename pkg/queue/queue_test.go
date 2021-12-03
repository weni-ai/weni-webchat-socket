package queue

import (
	"testing"
	"time"

	"github.com/adjust/rmq/v4"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
)

func TestConnection(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	qconn := OpenConnection("testconn", rdb, nil)
	assert.NotNil(t, qconn)
	queue1 := qconn.OpenQueue("test-queue")
	assert.NotNil(t, queue1)

	err := qconn.NewCleaner()
	assert.NoError(t, err)

	err = queue1.Publish("test-payload")
	assert.NoError(t, err)

	err = queue1.StartConsuming(1000, time.Millisecond*100)
	assert.NoError(t, err)

	consumer := NewMsgConsumer(0)
	_, err = queue1.AddConsumer("consumer-test", consumer)
	assert.NoError(t, err)

	queue2 := qconn.OpenQueue("test-queue2")
	queue2.StartConsuming(1000, time.Millisecond*100)

	consumerFunc := func(delivery rmq.Delivery) {
		delivery.Ack()
	}
	_, err = queue2.AddConsumerFunc("consumer-test2", consumerFunc)
	assert.NoError(t, err)

	var prefetchLimit int64 = 100
	queue2.SetPrefetchLimit(prefetchLimit)
	assert.Equal(t, prefetchLimit, queue2.PrefetchLimit())

	pollDuration := 100 * time.Millisecond
	queue2.SetPollDuration(pollDuration)
	assert.Equal(t, pollDuration, queue2.PollDuration())

	err = qconn.Close()
	assert.NoError(t, err)
}
