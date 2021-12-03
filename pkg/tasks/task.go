package tasks

import (
	"encoding/json"
	"log"

	"github.com/ilhasoft/wwcs/pkg/websocket"
)

// Tasks encapsulates the tasks logic
type Tasks interface {
	SendMsgToExternalService(string) error
}

type tasks struct {
	app *websocket.App
}

// NewTasks returns a new tasks representation
func NewTasks(app *websocket.App) Tasks {
	return &tasks{
		app: app,
	}
}

// SendMsgToExternalService is a task that do a request to external service from job.URL to send job.Payload as body
func (t *tasks) SendMsgToExternalService(payload string) error {
	log.Print(payload)
	var sJob websocket.OutgoingJob
	if err := json.Unmarshal([]byte(payload), &sJob); err != nil {
		return err
	}
	_, err := websocket.ToCallback(sJob.URL, sJob.Payload)
	if err != nil {
		return err
	}
	return nil
}
