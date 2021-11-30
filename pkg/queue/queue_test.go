package queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConnectionAndOpenQueue(t *testing.T) {
	connection := NewQueueConnection("testconn", "localhost:6379", 2)
	assert.NotNil(t, connection)
	queue := OpenQueue("test-queue", connection)
	assert.NotNil(t, queue)
}
