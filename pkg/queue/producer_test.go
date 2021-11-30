package queue

import (
	"encoding/json"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
)

func TestProducer(t *testing.T) {
	qconn := NewQueueConnection("testconn", "localhost:6379", 2)
	// qconn := rmq.NewTestConnection()
	qp := OpenQueue("test_producer_queue", qconn)
	pdcr := NewProducer(qp)

	texts := []string{"hello", "how", "are", "you"}

	for _, txt := range texts {
		go func(tt *testing.T, tx string, p Producer) {
			opm, err := json.Marshal(tx)
			if err != nil {
				log.Fatal(err)
			}
			err = p.Publish(string(opm))
			assert.Nil(tt, err)
		}(t, txt, pdcr)
	}

	time.Sleep(2 * time.Second)
}
