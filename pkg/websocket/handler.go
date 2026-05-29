package websocket

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-playground/validator"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// Connection attempt metric label values used when the HTTP Origin header
// cannot be resolved to a real origin string. They are intentionally chosen
// to sort near each other in dashboards while staying visually distinct.
const (
	// originLabelNone is used when no Origin header was sent at all. This is
	// typical of non-browser clients (curl, SDKs, bots) since browsers are
	// required by RFC 6455 to send Origin on WebSocket upgrades.
	originLabelNone = "<none>"
	// originLabelOpaque is used when the browser sent the literal "null"
	// string per the HTML spec for opaque origins (file://, data:, sandboxed
	// iframes, some cross-origin redirects).
	originLabelOpaque = "<opaque>"
)

// SetupRoutes handle all routes
func SetupRoutes(app *App) {
	log.Debugf("setting up routes")

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

// resolveMetricOrigin returns a Prometheus label value derived from the HTTP
// Origin header, with two fallbacks so the `origin` label is rarely empty:
//
//  1. When Origin is missing or the opaque "null" sentinel, try to recover a
//     real origin from the Referer header by keeping only scheme+host (any
//     path, query, or fragment is dropped to avoid leaking PII into labels).
//  2. When no real origin can be recovered, return a human-readable sentinel
//     so non-browser traffic ("<none>") is visually distinct from spec-defined
//     opaque-origin traffic ("<opaque>") in dashboards.
//
// The returned value is only used as a metric label; it does not affect the
// raw Origin stored on the Client or the domain-allowlist check.
func resolveMetricOrigin(origin, referer string) string {
	if origin != "" && origin != "null" {
		return origin
	}

	if fallback := originFromReferer(referer); fallback != "" {
		return fallback
	}

	if origin == "null" {
		return originLabelOpaque
	}
	return originLabelNone
}

// originFromReferer parses the Referer header and returns "scheme://host" if
// present, or "" if the header is missing, malformed, or lacks a usable host.
// Path, query, and fragment are intentionally dropped.
func originFromReferer(referer string) string {
	if referer == "" || referer == "null" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func (a *App) WSHandler(w http.ResponseWriter, r *http.Request) {
	log.Debugf("serving websocket")

	origin := r.Header.Get("Origin")
	// Resolve a label value separately from the raw Origin: the metric gets
	// the normalized value (with Referer fallback), while Client.Origin and
	// log fields keep the raw header so OriginToDomain / allowlist checks
	// and debugging are unaffected.
	metricOrigin := resolveMetricOrigin(origin, r.Header.Get("Referer"))

	log.Debugf("upgrading websocket")
	conn, err := Upgrade(w, r)
	if err != nil {
		// Try to upgrade first; if it fails, just log and return to avoid
		// superfluous WriteHeader when upgrader already wrote a response.
		if !checkWebsocketProtocol(r) {
			if a.Metrics != nil {
				a.Metrics.IncConnectionAttempts(metric.NewConnectionAttempt(metricOrigin, metric.ConnectionAttemptStatusProtocolInvalid))
			}
			log.WithField("origin", origin).WithField("connection", r.Header.Get("Connection")).WithField("upgrade", r.Header.Get("Upgrade")).WithError(err).Debug("invalid websocket protocol headers")
			return
		}
		if a.Metrics != nil {
			a.Metrics.IncConnectionAttempts(metric.NewConnectionAttempt(metricOrigin, metric.ConnectionAttemptStatusUpgradeFailed))
		}
		log.WithFields(log.Fields{
			"origin":      origin,
			"remote_addr": r.RemoteAddr,
			"user_agent":  r.Header.Get("User-Agent"),
			"request_uri": r.RequestURI,
		}).WithError(err).Error("failed to upgrade websocket connection")
		return
	}

	if a.Metrics != nil {
		a.Metrics.IncConnectionAttempts(metric.NewConnectionAttempt(metricOrigin, metric.ConnectionAttemptStatusUpgraded))
	}

	client := &Client{
		Conn:   conn,
		Origin: origin,
	}

	log.Debugf("websocket upgraded successfully, reading messages")
	client.Read(a)
	log.Debugf("messages read successfully")
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
	log.Debugf("receiving message")
	payload := IncomingPayload{}
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(ErrorBadRequest.Error()))
		log.WithFields(log.Fields{
			"remote_addr":  r.RemoteAddr,
			"content_type": r.Header.Get("Content-Type"),
			"request_uri":  r.RequestURI,
		}).WithError(err).Error("failed to decode incoming request body")
		return
	}

	err = validate.Struct(payload)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(ErrorBadRequest.Error()))
		log.WithFields(log.Fields{
			"payload_type": payload.Type,
			"to":           payload.To,
			"channel_uuid": payload.ChannelUUID,
			"remote_addr":  r.RemoteAddr,
		}).WithError(err).Error("incoming payload validation failed")
		return
	}

	payloadMarshalled, err := json.Marshal(payload)
	if err != nil {
		log.WithFields(log.Fields{
			"payload_type": payload.Type,
			"to":           payload.To,
			"channel_uuid": payload.ChannelUUID,
			"message_type": payload.Message.Type,
		}).WithError(err).Error("failed to marshal incoming payload")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(ErrorInternalError.Error()))
		return
	}

	if payload.Type != "typing_start" {
		msgTime := tryParseStr2Timestamp(payload.Message.Timestamp)
		hmsg := NewHistoryMessagePayload(DirectionIn, payload.To, payload.ChannelUUID, payload.Message, msgTime)

		log.WithFields(log.Fields{
			"to":           payload.To,
			"type":         payload.Type,
			"channel_uuid": payload.ChannelUUID,
			"message_type": payload.Message.Type,
			"source":       "http_incoming",
			"timestamp":    msgTime,
		}).Debug("HISTORY_SAVE_DEBUG: About to save from HTTP handler")

		err = a.Histories.Save(hmsg)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	connectedClient, err := a.ClientManager.GetConnectedClient(payload.To)
	if err != nil {
		log.WithError(err).WithField("to", payload.To).Error("error fetching connected client")
		w.WriteHeader(http.StatusInternalServerError)
		errBody, _ := json.Marshal(map[string]any{"error": ErrorInternalError.Error(), "to": payload.To})
		w.Write(errBody)
		return
	}

	if a.Router == nil {
		log.WithField("client", connectedClient).Error("message not published: router is not defined")
		w.WriteHeader(http.StatusInternalServerError)
		errBody, _ := json.Marshal(map[string]any{"error": "router is not defined", "client": connectedClient})
		w.Write(errBody)
		return
	}

	if err := a.Router.PublishToClient(r.Context(), payload.To, payloadMarshalled); err != nil {
		log.WithField("client", connectedClient).WithError(err).Error("error publishing incoming payload")
		w.WriteHeader(http.StatusInternalServerError)
		errBody, _ := json.Marshal(map[string]any{"error": err.Error(), "client": connectedClient})
		w.Write(errBody)
		return
	}

	response, _ := json.Marshal(map[string]any{"message": "message published", "client": connectedClient})
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(response))
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
		log.WithFields(log.Fields{
			"redis_status":   status.Redis,
			"mongodb_status": status.MongoDB,
		}).WithError(err).Error("failed to encode health check response")
		http.Error(w, err.Error(), 500)
	}
}

type HealthStatus struct {
	Redis   string `json:"redis,omitempty"`
	MongoDB string `json:"mongo_db,omitempty"`
}

func handleError(w http.ResponseWriter, err error, msg string) {
	log.WithError(err).Error(msg)
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
