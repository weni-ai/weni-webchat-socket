package queue

import (
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/adjust/rmq/v4"
)

// Create a new redis conection
func NewQueueConnection(tag string, address string, db int) rmq.Connection {
	connection, err := rmq.OpenConnection(tag, "tcp", address, db, nil)
	if err != nil {
		log.Error("unable to open Redis connection: ", err)
		panic(err)
	}
	go func() {
		cleaner := rmq.NewCleaner(connection)
		for range time.Tick(time.Second * 5) {
			_, err := cleaner.Clean()
			if err != nil {
				log.Error(err)
			}
		}
	}()
	log.Info("Redis connection OK")
	return connection
}

// OpenQueue open a queue and make it available to publish or consume
func OpenQueue(name string, connection rmq.Connection) rmq.Queue {
	queue, err := connection.OpenQueue(name)
	if err != nil {
		log.Error("Unable to open queue: ", err)
		panic(err)
	}
	return queue
}
