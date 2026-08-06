package audiosocket

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRegistrar struct {
	sessionID string
	err       error
}

func (s stubRegistrar) Register(did, callerID, origin string) (string, error) {
	return s.sessionID, s.err
}

func TestRegistrationHandlerSuccess(t *testing.T) {
	handler := &RegistrationHandler{
		Registrar:       stubRegistrar{sessionID: "sess-123"},
		AudioSocketAddr: "localhost:9095",
	}

	body := bytes.NewBufferString(`{"did":"+15551234567","caller_id":"+15559876543","origin":"pstn"}`)
	req := httptest.NewRequest(http.MethodPost, "/telephony/sessions", body)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp registrationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "sess-123", resp.SessionID)
	assert.Equal(t, "localhost:9095", resp.AudioSocketAddr)
}

func TestRegistrationHandlerMissingDID(t *testing.T) {
	handler := &RegistrationHandler{Registrar: stubRegistrar{}}

	body := bytes.NewBufferString(`{"origin":"pstn"}`)
	req := httptest.NewRequest(http.MethodPost, "/telephony/sessions", body)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRegistrationHandlerMissingOrigin(t *testing.T) {
	handler := &RegistrationHandler{Registrar: stubRegistrar{}}

	body := bytes.NewBufferString(`{"did":"+15551234567"}`)
	req := httptest.NewRequest(http.MethodPost, "/telephony/sessions", body)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRegistrationHandlerUnknownDID(t *testing.T) {
	handler := &RegistrationHandler{
		Registrar: stubRegistrar{err: ErrChannelNotFound},
	}

	body := bytes.NewBufferString(`{"did":"+15551234567","origin":"pstn"}`)
	req := httptest.NewRequest(http.MethodPost, "/telephony/sessions", body)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRegistrationHandlerSTTDependencyDown(t *testing.T) {
	handler := &RegistrationHandler{
		Registrar: stubRegistrar{err: ErrSTTDependencyDown},
	}

	body := bytes.NewBufferString(`{"did":"+15551234567","origin":"pstn"}`)
	req := httptest.NewRequest(http.MethodPost, "/telephony/sessions", body)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestRegistrationHandlerDependencyDown(t *testing.T) {
	handler := &RegistrationHandler{
		Registrar: stubRegistrar{err: errors.New("flows unavailable")},
	}

	body := bytes.NewBufferString(`{"did":"+15551234567","origin":"pstn"}`)
	req := httptest.NewRequest(http.MethodPost, "/telephony/sessions", body)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRegistrationHandlerMethodNotAllowed(t *testing.T) {
	handler := &RegistrationHandler{Registrar: stubRegistrar{}}
	req := httptest.NewRequest(http.MethodGet, "/telephony/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
