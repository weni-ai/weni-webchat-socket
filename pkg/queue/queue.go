package queue

import (
	"log"
	"time"

	"github.com/adjust/rmq/v4"
)

func NewQueueConnection(tag string) rmq.Connection {
	//TODO get params from config
	connection, err := rmq.OpenConnection(tag, "tcp", "localhost:6379", 1, nil)
	if err != nil {
		panic(err)
	}
	go func() {
		cleaner := rmq.NewCleaner(connection)
		for range time.Tick(time.Second * 5) {
			cleaned, err := cleaner.Clean()
			if err != nil {
				//TODO log with default logger
				log.Print(err)
			}
			if cleaned > 0 {
				//TODO log total cleaned tasks
				log.Print(err)
			}
		}
	}()
	//TODO log with default logger
	log.Print("Redis connection OK")
	return connection
}

func OpenQueue(name string, connection rmq.Connection) rmq.Queue {
	queue, err := connection.OpenQueue(name)
	if err != nil {
		panic(err)
	}
	return queue
}
