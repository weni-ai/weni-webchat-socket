package queue

import (
	"github.com/adjust/rmq/v4"
)

// Producer encapsulates the logic to produce deliveries to a queue.
type Producer interface {
	Publish(string) error
}

type producer struct {
	queue rmq.Queue
}

// NewProducer create a new queue producer.
func NewProducer(queue rmq.Queue) Producer {
	return producer{queue: queue}
}

// Publish send a new payload delivery to queue
func (p producer) Publish(message string) error {
	err := p.queue.Publish(message)
	if err != nil {
		return err
	}
	return nil
}
