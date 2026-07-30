package audiosocket

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedConn struct {
	frames []Frame
	idx    int
	mu     sync.Mutex
}

func (c *scriptedConn) ReadFrame() (Frame, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.idx >= len(c.frames) {
		return Frame{}, io.EOF
	}
	frame := c.frames[c.idx]
	c.idx++
	return frame, nil
}

func (c *scriptedConn) WriteAudio([]byte) error { return nil }
func (c *scriptedConn) Close() error          { return nil }

func TestValidAudioFrameLength(t *testing.T) {
	assert.True(t, ValidAudioFrameLength(320))
	assert.True(t, ValidAudioFrameLength(304))
	assert.True(t, ValidAudioFrameLength(336))
	assert.False(t, ValidAudioFrameLength(100))
	assert.False(t, ValidAudioFrameLength(400))
}

func TestReadLoopDropsMalformedAudioFrame(t *testing.T) {
	valid := make([]byte, ExpectedAudioFrameSize)
	var received [][]byte

	conn := &scriptedConn{
		frames: []Frame{
			{Kind: KindAudio, Payload: []byte{1, 2, 3}},
			{Kind: KindAudio, Payload: valid},
		},
	}

	RunReadLoop(conn, ReadLoopConfig{
		OnAudio: func(pcm []byte) {
			received = append(received, pcm)
		},
	})

	require.Len(t, received, 1)
	assert.Equal(t, valid, received[0])
}

func TestReadLoopForwardsValidAudioNonBlocking(t *testing.T) {
	valid := make([]byte, ExpectedAudioFrameSize)
	received := make(chan []byte, 1)

	conn := &scriptedConn{
		frames: []Frame{
			{Kind: KindAudio, Payload: valid},
		},
	}

	done := make(chan struct{})
	go func() {
		RunReadLoop(conn, ReadLoopConfig{
			OnAudio: func(pcm []byte) {
				select {
				case received <- pcm:
				default:
					t.Error("OnAudio blocked")
				}
			},
		})
		close(done)
	}()

	select {
	case pcm := <-received:
		assert.Equal(t, valid, pcm)
	case <-done:
		t.Fatal("read loop finished without delivering audio")
	}
}

func TestReadLoopStopsOnHangup(t *testing.T) {
	hangupCalled := false
	conn := &scriptedConn{
		frames: []Frame{
			{Kind: KindHangup},
		},
	}

	RunReadLoop(conn, ReadLoopConfig{
		OnHangup: func() {
			hangupCalled = true
		},
	})

	assert.True(t, hangupCalled)
}

func TestWriteAudioFrameWireFormat(t *testing.T) {
	var buf bytes.Buffer
	raw := &bufferConn{buf: &buf}
	conn := NewTCPConn(raw)

	audio := make([]byte, 320)
	require.NoError(t, conn.WriteAudio(audio))

	require.Len(t, buf.Bytes(), 3+320)
	assert.Equal(t, byte(KindAudio), buf.Bytes()[0])
	length := binary.BigEndian.Uint16(buf.Bytes()[1:3])
	assert.Equal(t, uint16(320), length)
}

type bufferConn struct {
	buf *bytes.Buffer
}

func (b *bufferConn) Read([]byte) (int, error)  { return 0, io.EOF }
func (b *bufferConn) Write(p []byte) (int, error) { return b.buf.Write(p) }
func (b *bufferConn) Close() error              { return nil }
func (b *bufferConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (b *bufferConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (b *bufferConn) SetDeadline(time.Time) error      { return nil }
func (b *bufferConn) SetReadDeadline(time.Time) error  { return nil }
func (b *bufferConn) SetWriteDeadline(time.Time) error { return nil }
