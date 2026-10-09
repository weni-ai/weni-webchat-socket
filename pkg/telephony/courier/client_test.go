package courier

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveURL(t *testing.T) {
	got := ResolveURL("https://courier.example.com", "+15551234567")
	want := "https://courier.example.com/c/tph/resolve?did=%2B15551234567"
	if got != want {
		t.Fatalf("ResolveURL() = %q, want %q", got, want)
	}
}

func TestResolveChannelSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ResolvePath {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("did"); got != "+15551234567" {
			http.Error(w, "unexpected did", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"channel_uuid":"ch-1","project_uuid":"proj-1"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret", nil)
	channelUUID, projectUUID, err := client.ResolveChannel("+15551234567")
	if err != nil {
		t.Fatalf("ResolveChannel() error = %v", err)
	}
	if channelUUID != "ch-1" || projectUUID != "proj-1" {
		t.Fatalf("ResolveChannel() = (%q, %q), want (ch-1, proj-1)", channelUUID, projectUUID)
	}
}

func TestResolveChannelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Error","data":[{"type":"error","error":"channel not found"}]}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", nil)
	channelUUID, projectUUID, err := client.ResolveChannel("+15559999999")
	if err != nil {
		t.Fatalf("ResolveChannel() error = %v", err)
	}
	if channelUUID != "" || projectUUID != "" {
		t.Fatalf("ResolveChannel() = (%q, %q), want empty result", channelUUID, projectUUID)
	}
}

func TestResolveChannelMissingBaseURL(t *testing.T) {
	client := NewClient("", "", nil)
	_, _, err := client.ResolveChannel("+15551234567")
	if err == nil {
		t.Fatal("expected error for missing base URL")
	}
}
