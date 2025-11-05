package websocket

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-playground/validator"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// SetupRoutes handle all routes
func SetupRoutes(app *App) {
	log.Trace("Setting up routes")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/ws", app.WSHandler)
	http.HandleFunc("/send", app.SendHandler)
	http.HandleFunc("/healthcheck", app.HealthCheckHandler)
	http.Handle("/metrics", promhttp.Handler())
}

func checkWebsocketProtocol(r *http.Request) bool {
	if r.Header.Get("Upgrade") != "websocket" || r.Header.Get("Connection") != "Upgrade" || r.Header.Get("Sec-Websocket-Version") != "13" || r.Method != "GET" {
		return false
	}
	return true
}

func (a *App) WSHandler(w http.ResponseWriter, r *http.Request) {
	log.Trace("Serving websocket")

	conn, err := Upgrade(w, r)
	if err != nil {
		if !checkWebsocketProtocol(r) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("websocket: the client is not using the websocket protocol"))
			return
		}
		log.Error(err, r)
		return
	}

	client := &Client{
		Conn:   conn,
		Origin: r.Header.Get("Origin"),
	}

	client.Read(a)
}

var validate = validator.New()

var (
	ErrorConnectionClosed = errors.New("unable to send: connection closed")
	ErrorInternalError    = errors.New("unable to send: internal error")
	ErrorBadRequest       = errors.New("unable to send: bad request")
	ErrorNotFound         = errors.New("unable to send: not found")
	ErrorAWSConnection    = errors.New("unable to connect to AWS")
)

// SendHandler is used to receive messages from external systems
func (a *App) SendHandler(w http.ResponseWriter, r *http.Request) {
	log.Tracef("Receiving message from %q", r.Host)
	payload := IncomingPayload{}
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(ErrorBadRequest.Error()))
		log.Error("error on decode request body: ", err)
		return
	}

	err = validate.Struct(payload)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(ErrorBadRequest.Error()))
		log.Error("error on validate payload: ", err)
		return
	}

	payloadMarshalled, err := json.Marshal(payload)
	if err != nil {
		log.Error("error to parse incoming payload: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(ErrorInternalError.Error()))
		return
	}

	if payload.Type != "typing_start" {
		msgTime := tryParseStr2Timestamp(payload.Message.Timestamp)
		hmsg := NewHistoryMessagePayload(DirectionIn, payload.To, payload.ChannelUUID, payload.Message, msgTime)
		err = a.Histories.Save(hmsg)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	connectedClient, _ := a.ClientManager.GetConnectedClient(payload.To)
	if connectedClient != nil {
		if a.Router != nil {
			if err := a.Router.PublishToClient(r.Context(), payload.To, payloadMarshalled); err != nil {
				log.Error("error to publish incoming payload: ", err)
			}
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

// HealthCheckHandler is used to provide a mechanism to check the service status
func (a *App) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	redisErr := CheckRedis(a)
	dbErr := CheckDB(a)

	status := HealthStatus{"ok", "ok"}
	hasError := false

	if redisErr != nil {
		status.Redis = redisErr.Error()
		hasError = true
	}
	if dbErr != nil {
		status.MongoDB = dbErr.Error()
		hasError = true
	}

	if hasError {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	err := json.NewEncoder(w).Encode(status)
	if err != nil {
		log.Error(err)
		http.Error(w, err.Error(), 500)
	}
}

type HealthStatus struct {
	Redis   string `json:"redis,omitempty"`
	MongoDB string `json:"mongo_db,omitempty"`
}

func handleError(w http.ResponseWriter, err error, msg string) {
	log.Error(msg, err)
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(ErrorInternalError.Error()))
}

func tryParseStr2Timestamp(t string) int64 {
	mt, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return time.Now().Unix()
	}
	return mt
}
