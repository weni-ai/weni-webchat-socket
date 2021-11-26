package queue

import (
	"github.com/adjust/rmq/v4"
)

type Producer interface {
	Publish(string) error
}

type producer struct {
	queue rmq.Queue
}

func NewProducer(queue rmq.Queue) Producer {
	return producer{queue: queue}
}

func (p producer) Publish(message string) error {
	err := p.queue.Publish(message)
	if err != nil {
		return err
	}
	return nil
}
