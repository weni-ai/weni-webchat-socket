package flows

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetChannelAllowedDomains(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[\"domain1.com\", \"domain2.com\"]"))
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)

	domains, err := client.GetChannelAllowedDomains("09bf3dee-973e-43d3-8b94-441406c4a565")

	assert.NoError(t, err)
	assert.Equal(t, 2, len(domains))
}

func TestGetChannelAllowedDomainsStatus404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)

	_, err := client.GetChannelAllowedDomains("09bf3dee-973e-43d3-8b94-441406c4a565")

	assert.Equal(t, err.Error(), "failed to get channel allowed domains, status code: 404")
}

func TestGetChannelAllowedDomainsStatusWithNoDomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)

	domains, err := client.GetChannelAllowedDomains("09bf3dee-973e-43d3-8b94-441406c4a565")

	assert.NoError(t, err)
	assert.Equal(t, 0, len(domains))
}

func TestContactHasOpenTicket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("true"))
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)

	hasTicket, err := client.ContactHasOpenTicket("test-channel-uuid", "wwc:1234567890")

	assert.NoError(t, err)
	assert.True(t, hasTicket)
}

func TestContactHasOpenTicketFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("false"))
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)

	hasTicket, err := client.ContactHasOpenTicket("test-channel-uuid", "wwc:1234567890")

	assert.NoError(t, err)
	assert.False(t, hasTicket)
}

func TestContactHasOpenTicketStatus404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)

	_, err := client.ContactHasOpenTicket("test-channel-uuid", "wwc:1234567890")

	assert.Equal(t, err.Error(), "failed to get contact has open ticket, status code: 404")
}

func TestUpdateContactFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "/api/v2/internals/update_contacts_fields", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)

	err := client.UpdateContactFields(
		"09bf3dee-973e-43d3-8b94-441406c4a565",
		"wwc:1234567890",
		map[string]interface{}{"field_key": "field_value"},
	)

	assert.NoError(t, err)
}

func TestUpdateContactFieldsStatus400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)

	err := client.UpdateContactFields(
		"09bf3dee-973e-43d3-8b94-441406c4a565",
		"wwc:1234567890",
		map[string]interface{}{"field_key": "field_value"},
	)

	assert.Error(t, err)
	assert.Equal(t, "failed to update contact fields, status code: 400", err.Error())
}

func TestUpdateContactFieldsStatus500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)

	err := client.UpdateContactFields(
		"09bf3dee-973e-43d3-8b94-441406c4a565",
		"wwc:1234567890",
		map[string]interface{}{"field_key": "field_value"},
	)

	assert.Error(t, err)
	assert.Equal(t, "failed to update contact fields, status code: 500", err.Error())
}

func TestUpdateContactFieldsRequestBody(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&receivedBody)
		assert.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)

	err := client.UpdateContactFields(
		"channel-uuid-123",
		"contact-id-456",
		map[string]interface{}{"User ID": "12345"},
	)

	assert.NoError(t, err)
	assert.Equal(t, "channel-uuid-123", receivedBody["channel_uuid"])
	assert.Equal(t, "ext:contact-id-456", receivedBody["contact_urn"])
	contactFields := receivedBody["contact_fields"].(map[string]interface{})
	assert.Equal(t, "12345", contactFields["User ID"])
}
