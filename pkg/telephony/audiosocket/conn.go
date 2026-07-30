package audiosocket

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

const (
	KindHangup byte = 0x00
	KindUUID   byte = 0x01
	KindDTMF   byte = 0x03
	KindAudio  byte = 0x10
	KindError  byte = 0xFF
)

// Frame is a parsed AudioSocket wire frame.
type Frame struct {
	Kind    byte
	Payload []byte
}

// AudioSocketConn abstracts an AudioSocket TCP connection.
type AudioSocketConn interface {
	ReadFrame() (Frame, error)
	WriteAudio([]byte) error
	Close() error
}

type tcpConn struct {
	conn net.Conn
	mu   sync.Mutex
}

// NewTCPConn wraps a net.Conn as an AudioSocketConn.
func NewTCPConn(conn net.Conn) AudioSocketConn {
	return &tcpConn{conn: conn}
}

func (c *tcpConn) ReadFrame() (Frame, error) {
	header := make([]byte, 3)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return Frame{}, err
	}

	kind := header[0]
	length := binary.BigEndian.Uint16(header[1:3])

	var payload []byte
	if length > 0 {
		payload = make([]byte, length)
		if _, err := io.ReadFull(c.conn, payload); err != nil {
			return Frame{}, err
		}
	}

	switch kind {
	case KindHangup, KindUUID, KindDTMF, KindAudio, KindError:
		return Frame{Kind: kind, Payload: payload}, nil
	default:
		return Frame{}, fmt.Errorf("audiosocket: unrecognized frame kind 0x%02x", kind)
	}
}

func (c *tcpConn) WriteAudio(audio []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	header := make([]byte, 3)
	header[0] = KindAudio
	binary.BigEndian.PutUint16(header[1:3], uint16(len(audio)))

	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	if len(audio) == 0 {
		return nil
	}
	_, err := c.conn.Write(audio)
	return err
}

func (c *tcpConn) Close() error {
	return c.conn.Close()
}
