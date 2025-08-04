package flows

import (
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

	client := Client{BaseURL: server.URL}

	domains, err := client.GetChannelAllowedDomains("09bf3dee-973e-43d3-8b94-441406c4a565")

	assert.NoError(t, err)
	assert.Equal(t, 2, len(domains))
}

func TestGetChannelAllowedDomainsStatus404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL}

	_, err := client.GetChannelAllowedDomains("09bf3dee-973e-43d3-8b94-441406c4a565")

	assert.Equal(t, err.Error(), "failed to get channel allowed domains, status code: 404")
}

func TestGetChannelAllowedDomainsStatusWithNoDomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL}

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

	client := Client{BaseURL: server.URL}

	hasTicket, err := client.ContactHasOpenTicket("wwc:1234567890")

	assert.NoError(t, err)
	assert.True(t, hasTicket)
}

func TestContactHasOpenTicketFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("false"))
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL}

	hasTicket, err := client.ContactHasOpenTicket("wwc:1234567890")

	assert.NoError(t, err)
	assert.False(t, hasTicket)
}

func TestContactHasOpenTicketStatus404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL}

	_, err := client.ContactHasOpenTicket("wwc:1234567890")

	assert.Equal(t, err.Error(), "failed to get contact has open ticket, status code: 404")
}
