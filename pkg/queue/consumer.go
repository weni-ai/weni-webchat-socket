package queue

import (
	"fmt"
	"log"
	"time"

	"github.com/adjust/rmq/v4"
)

const (
	prefetchLimit   = 1000
	pollDuration    = 100 * time.Millisecond
	numConsumers    = 5
	consumeDuration = time.Millisecond
	shouldLog       = true
)

func StartConsuming(q rmq.Queue, task func(string) error) error {
	if err := q.StartConsuming(1, pollDuration); err != nil {
		return err
	}
	for i := 0; i < numConsumers; i++ {
		name := fmt.Sprintf("msgconsumer %d", i)
		c := NewMsgConsumer(i)
		c.Name = name
		c.Task = task
		if _, err := q.AddConsumer(name, c); err != nil {
			return err
		}
	}
	return nil
}

type MsgConsumer struct {
	Name   string
	Before time.Time
	Task   func(string) error
}

func NewMsgConsumer(tag int) *MsgConsumer {
	return &MsgConsumer{
		Name:   fmt.Sprintf("consumer %d", tag),
		Before: time.Now(),
	}
}

func (consumer *MsgConsumer) Consume(delivery rmq.Delivery) {
	payload := delivery.Payload()
	if err := consumer.Task(payload); err != nil {
		log.Println(err)
		return
	}
	if err := delivery.Ack(); err != nil {
		log.Println(err)
	}
	log.Printf("consumer %s, consumed: %s", consumer.Name, payload)
}
