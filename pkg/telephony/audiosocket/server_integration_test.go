package audiosocket

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerAcceptsUUIDConnection(t *testing.T) {
	sessionID := uuid.NewString()
	connected := make(chan string, 1)

	srv := NewServer("127.0.0.1:0", func(id string, conn AudioSocketConn) {
		connected <- id
		_ = conn.Close()
	})
	require.NoError(t, srv.Start())
	defer func() { _ = srv.Stop() }()
	require.NotEmpty(t, srv.Addr())

	conn, err := net.Dial("tcp", srv.Addr())
	require.NoError(t, err)
	defer conn.Close()

	uid, err := uuid.Parse(sessionID)
	require.NoError(t, err)
	require.NoError(t, writeTestFrame(conn, KindUUID, uid[:]))

	select {
	case got := <-connected:
		assert.Equal(t, sessionID, got)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for audiosocket connection")
	}
}

func TestServerRejectsNonUUIDFirstFrame(t *testing.T) {
	connected := make(chan struct{}, 1)

	srv := NewServer("127.0.0.1:0", func(_ string, _ AudioSocketConn) {
		connected <- struct{}{}
	})
	require.NoError(t, srv.Start())
	defer func() { _ = srv.Stop() }()

	conn, err := net.Dial("tcp", srv.Addr())
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, writeTestFrame(conn, KindAudio, make([]byte, ExpectedAudioFrameSize)))

	select {
	case <-connected:
		t.Fatal("expected non-UUID first frame to be rejected")
	case <-time.After(500 * time.Millisecond):
	}
}

func TestServerAddrEmptyBeforeStart(t *testing.T) {
	srv := NewServer("127.0.0.1:0", nil)
	assert.Empty(t, srv.Addr())
}

func writeTestFrame(conn net.Conn, kind byte, payload []byte) error {
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
