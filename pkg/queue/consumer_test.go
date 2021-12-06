package queue

import (
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
)

func TestConsumer(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	qconn := OpenConnection("testconn", rdb, nil)
	q := qconn.OpenQueue("test_consumer_queue")
	consumer := NewConsumer(q)
	taskOk := func(payload string) error {
		return nil
	}
	taskError := func(payload string) error {
		return nil
	}
	assert.NoError(t, consumer.StartConsuming(1, taskOk))
	assert.Error(t, consumer.StartConsuming(1, taskError))
}
