package tasks

import (
	"encoding/json"
	"log"

	"github.com/ilhasoft/wwcs/pkg/websocket"
)

type Tasks interface {
	SendMsgToContact(string) error
	SendMsgToCourier(string) error
}

type tasks struct {
	app *websocket.App
}

func NewTasks(app *websocket.App) Tasks {
	return &tasks{
		app: app,
	}
}

func (t *tasks) SendMsgToContact(payload string) error {
	log.Print(payload)
	var inJob websocket.IncomingPayload
	if err := json.Unmarshal([]byte(payload), &inJob); err != nil {
		return err
	}
	c, found := t.app.Pool.Clients[inJob.To]
	if !found {
		return websocket.ErrorNotFound
	}
	if err := c.Send(inJob); err != nil {
		return websocket.ErrorInternalError
	}
	return nil
}

func (t *tasks) SendMsgToCourier(payload string) error {
	log.Print(payload)
	var sJob websocket.OutgoingJob
	if err := json.Unmarshal([]byte(payload), &sJob); err != nil {
		return err
	}
	err := websocket.ToCallback(sJob.URL, sJob.Payload)
	if err != nil {
		return err
	}
	return nil
}
