package queue

import (
	"fmt"
	"time"

	"github.com/ilhasoft/wwcs/config"
	log "github.com/sirupsen/logrus"

	"github.com/adjust/rmq/v4"
)

// Consumer encapsulates the logic to consume deliveries from a queue
type Consumer interface {
	StartConsuming(func(string) error) error
}

type consumer struct {
	queue rmq.Queue
}

// NewConsumer Create a new queue consumer
func NewConsumer(queue rmq.Queue) Consumer {
	return &consumer{
		queue: queue,
	}
}

// StartConsuming start a queue Consumer
func (c *consumer) StartConsuming(task func(string) error) error {
	config := config.Get.RedisQueue
	if err := c.queue.StartConsuming(
		config.ConsumerPrefetchLimit,
		time.Duration(config.ConsumerPollDuration)*time.Millisecond,
	); err != nil {
		return err
	}
	for i := 0; i < config.ConsumerWorkers; i++ {
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
		log.Info(err)
		return
	}
	if err := delivery.Ack(); err != nil {
		log.Error(err)
	}
	log.Info(fmt.Sprintf("consumer %s, consumed: %s", consumer.Name, payload))
}
