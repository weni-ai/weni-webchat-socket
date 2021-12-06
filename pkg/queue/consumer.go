package queue

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/adjust/rmq/v4"
)

// Consumer encapsulates the logic to consume deliveries from a queue
type Consumer interface {
	StartConsuming(int, func(string) error) error
}

type consumer struct {
	queue Queue
}

// NewConsumer Create a new queue consumer
func NewConsumer(queue Queue) Consumer {
	return &consumer{
		queue: queue,
	}
}

// StartConsuming start a queue Consumer
func (c *consumer) StartConsuming(workerCount int, task func(string) error) error {
	if err := c.queue.StartConsuming(
		c.queue.PrefetchLimit(),
		c.queue.PollDuration(),
	); err != nil {
		return err
	}
	for i := 0; i < workerCount; i++ {
		name := fmt.Sprintf("msgconsumer %d", i)
		cw := NewMsgConsumer(i)
		cw.Name = name
		cw.Task = task
		if _, err := c.queue.AddConsumer(name, cw); err != nil {
			return err
		}
	}
	return nil
}

// MsgConsumer represents a struct of a queue delivery consumer
type MsgConsumer struct {
	Name   string
	Before time.Time
	Task   func(string) error
}

// NewMsgConsumer creates a new MsgConsumer instance
func NewMsgConsumer(tag int) *MsgConsumer {
	return &MsgConsumer{
		Name:   fmt.Sprintf("consumer %d", tag),
		Before: time.Now(),
	}
}

// Consume implements Consume function that MsgConsumer run when consuming a delivery
func (consumer *MsgConsumer) Consume(delivery rmq.Delivery) {
	payload := delivery.Payload()
	if err := consumer.Task(payload); err != nil {
		delivery.Push()
		return
	}
	if err := delivery.Ack(); err != nil {
		log.Error(err)
	}
}
