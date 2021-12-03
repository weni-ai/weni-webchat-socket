package tasks

import (
	"encoding/json"
	"log"

	"github.com/ilhasoft/wwcs/pkg/websocket"
)

type Tasks interface {
	SendMsgToExternalService(string) error
}

type tasks struct {
	app *websocket.App
}

func NewTasks(app *websocket.App) Tasks {
	return &tasks{
		app: app,
	}
}

func (t *tasks) SendMsgToExternalService(payload string) error {
	log.Print(payload)
	var sJob websocket.OutgoingJob
	if err := json.Unmarshal([]byte(payload), &sJob); err != nil {
		return err
	}
	bodyPayload, err := websocket.ToCallback(sJob.URL, sJob.Payload)
	if err != nil {
		if bodyPayload != nil {
			t.app.OutgoingQueue.Publish(string(bodyPayload))
			return nil
		} else {
			return err
		}
	}
	return nil
}
