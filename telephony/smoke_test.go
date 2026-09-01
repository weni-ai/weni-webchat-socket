package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
	"github.com/ilhasoft/wwcs/pkg/telephony/session"
	"github.com/ilhasoft/wwcs/pkg/telephony/stt"
	"github.com/ilhasoft/wwcs/pkg/telephony/tts"
	"github.com/ilhasoft/wwcs/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuickstartSmokeTest exercises the quickstart.md end-to-end flow using real
// HTTP registration and AudioSocket TCP servers with mocked external dependencies.
func TestQuickstartSmokeTest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := flows.NewMockIClient(ctrl)
	callbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"Message Accepted","data":[{"urn":"tel:+15559876543"}]}`))
	}))
	t.Cleanup(callbackSrv.Close)

	mockFlows.EXPECT().GetElevenLabsAPIKey("ch-1").Return("test-key", nil).AnyTimes()
	mockFlows.EXPECT().GetChannelProjectLanguage("ch-1").Return("en", nil).AnyTimes()

	sttEvents := make(chan stt.Event, 4)
	sttSession := &smokeSTTSession{events: sttEvents}
	sttFactory := func(_ context.Context, _ *session.VoiceConfig) (stt.STTSession, error) {
		return sttSession, nil
	}

	ttsAudio := make([]byte, audiosocket.ExpectedAudioFrameSize*2)
	ttsFactory := func(_ *session.VoiceConfig) tts.TTSStreamClient {
		return &smokeTTSClient{audio: ttsAudio}
	}

	baseMetrics, err := metric.NewPrometheusService()
	require.NoError(t, err)
	sessionMetrics, err := session.NewSessionMetrics(baseMetrics)
	require.NoError(t, err)

	sessionManager := session.NewSessionManager(mockFlows, smokeCourierClient{}, 10, "", sessionMetrics, nil)
	clientManager := &smokeClientManager{}
	deliveryCoordinator := session.NewDeliveryCoordinator(clientManager, sessionManager, "telephony-smoke", callbackSrv.URL)
	teardownCoordinator := &session.TeardownCoordinator{
		SessionManager:      sessionManager,
		DeliveryCoordinator: deliveryCoordinator,
		Metrics:             sessionMetrics,
	}
	sessionManager.SetTeardownCoordinator(teardownCoordinator)

	mediaRunner := session.NewMediaRunner(sttFactory, deliveryCoordinator.OnCommittedTranscript)
	mediaRunner.SetHangupHandler(func(cs *session.CallSession) {
		teardownCoordinator.Complete(cs, "caller_hangup")
	})
	setupRunner := session.NewSetupRunner(mockFlows, sttFactory, ttsFactory, sessionMetrics, mediaRunner, deliveryCoordinator, nil)
	sessionManager.SetSetupRunner(setupRunner)

	audioServer := audiosocket.NewServer("127.0.0.1:0", func(sessionID string, conn audiosocket.AudioSocketConn) {
		if err := sessionManager.Attach(sessionID, conn); err != nil {
			_ = conn.Close()
		}
	})
	require.NoError(t, audioServer.Start())
	defer func() { _ = audioServer.Stop() }()

	regHandler := &audiosocket.RegistrationHandler{
		Registrar:       sessionManager,
		AudioSocketAddr: audioServer.Addr(),
	}
	httpServer := httptest.NewServer(regHandler)
	defer httpServer.Close()

	regBody := bytes.NewBufferString(`{"did":"+15551234567","caller_id":"+15559876543","origin":"pstn"}`)
	regResp, err := http.Post(httpServer.URL+"/telephony/sessions", "application/json", regBody)
	require.NoError(t, err)
	defer regResp.Body.Close()
	require.Equal(t, http.StatusOK, regResp.StatusCode)

	var registration struct {
		SessionID       string `json:"session_id"`
		AudioSocketAddr string `json:"audiosocket_addr"`
	}
	require.NoError(t, json.NewDecoder(regResp.Body).Decode(&registration))
	require.NotEmpty(t, registration.SessionID)
	assert.Equal(t, audioServer.Addr(), registration.AudioSocketAddr)

	tcpConn, err := net.Dial("tcp", registration.AudioSocketAddr)
	require.NoError(t, err)
	require.NoError(t, writeUUIDFrame(tcpConn, registration.SessionID))

	receivedAudio := make(chan []byte, 8)
	go readAudioFrames(tcpConn, receivedAudio)

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-receivedAudio:
			goto greetingReceived
		case <-deadline:
			t.Fatal("timed out waiting for greeting TTS audio")
		case <-time.After(20 * time.Millisecond):
			cs, ok := sessionManager.Get(registration.SessionID)
			if ok && cs.CurrentState() == session.StateListening {
				goto greetingReceived
			}
		}
	}
greetingReceived:

	pcm := make([]byte, audiosocket.ExpectedAudioFrameSize)
	require.NoError(t, writeAudioFrame(tcpConn, pcm))

	deadline = time.After(3 * time.Second)
	for sttSession.sent == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for caller audio to reach STT")
		case <-time.After(10 * time.Millisecond):
		}
	}

	sttEvents <- stt.Event{
		Kind:                stt.EventCommittedTranscript,
		CommittedTranscript: stt.CommittedTranscript{Text: "book a flight"},
	}

	cs, ok := sessionManager.Get(registration.SessionID)
	require.True(t, ok)
	deadline = time.After(3 * time.Second)
	for cs.HandoffCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for committed transcript handoff")
		case <-time.After(10 * time.Millisecond):
			cs, ok = sessionManager.Get(registration.SessionID)
			require.True(t, ok)
		}
	}

	assert.True(t, sessionMetrics.HasObservedSetupDuration())
	assert.True(t, sessionMetrics.HasObservedSTTCommitLatency())

	require.NoError(t, writeHangupFrame(tcpConn))

	deadline = time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for teardown")
		case <-time.After(10 * time.Millisecond):
			_, stillRegistered := sessionManager.Get(registration.SessionID)
			if !stillRegistered {
				assert.Equal(t, 1.0, sessionMetrics.TeardownCount("caller_hangup"))
				_ = tcpConn.Close()
				return
			}
		}
	}
}

type smokeSTTSession struct {
	events chan stt.Event
	closed bool
	sent   int
}

func (s *smokeSTTSession) Send([]byte) error {
	s.sent++
	return nil
}
func (s *smokeSTTSession) Events() <-chan stt.Event {
	return s.events
}
func (s *smokeSTTSession) Close() error {
	s.closed = true
	return nil
}

type smokeTTSClient struct {
	audio []byte
}

func (c *smokeTTSClient) Synthesize(_ context.Context, _, _, _ string) (<-chan []byte, error) {
	ch := make(chan []byte, 1)
	ch <- append([]byte(nil), c.audio...)
	close(ch)
	return ch, nil
}

func writeUUIDFrame(conn net.Conn, sessionID string) error {
	uid, err := uuid.Parse(sessionID)
	if err != nil {
		return err
	}
	return writeFrame(conn, audiosocket.KindUUID, uid[:])
}

func writeAudioFrame(conn net.Conn, pcm []byte) error {
	return writeFrame(conn, audiosocket.KindAudio, pcm)
}

func writeHangupFrame(conn net.Conn) error {
	return writeFrame(conn, audiosocket.KindHangup, nil)
}

func writeFrame(conn net.Conn, kind byte, payload []byte) error {
	header := make([]byte, 3)
	header[0] = kind
	binary.BigEndian.PutUint16(header[1:3], uint16(len(payload)))
	if _, err := conn.Write(header); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := conn.Write(payload)
	return err
}

func readAudioFrames(conn net.Conn, out chan<- []byte) {
	raw := audiosocket.NewTCPConn(conn)
	for {
		frame, err := raw.ReadFrame()
		if err != nil {
			return
		}
		if frame.Kind == audiosocket.KindAudio && len(frame.Payload) > 0 {
			out <- append([]byte(nil), frame.Payload...)
		}
		if frame.Kind == audiosocket.KindHangup {
			return
		}
	}
}

type smokeClientManager struct{}

type smokeCourierClient struct{}

func (smokeCourierClient) ResolveChannel(string) (string, string, error) {
	return "ch-1", "proj-1", nil
}

func (m *smokeClientManager) GetConnectedClients() ([]string, error) { return nil, nil }
func (m *smokeClientManager) GetConnectedClient(string) (*websocket.ConnectedClient, error) {
	return nil, nil
}
func (m *smokeClientManager) AddConnectedClient(cc websocket.ConnectedClient) error { return nil }
func (m *smokeClientManager) RemoveConnectedClient(string) error                    { return nil }
func (m *smokeClientManager) RemoveConnectedClientIf(string, string) (bool, error)  { return false, nil }
func (m *smokeClientManager) UpdateClientTTL(string, int) (bool, error)             { return false, nil }
func (m *smokeClientManager) DefaultClientTTL() int                                 { return 60 }
