package queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConsumer(t *testing.T) {
	qconn := NewConnection("testconn", "localhost:6379", 3)
	q := OpenQueue("test_consumer_queue", qconn)
	consumer := NewConsumer(q)
	taskOk := func(payload string) error {
		return nil
	}
	taskError := func(payload string) error {
		return nil
	}
	assert.NoError(t, consumer.StartConsuming(taskOk))
	assert.Error(t, consumer.StartConsuming(taskError))
}
