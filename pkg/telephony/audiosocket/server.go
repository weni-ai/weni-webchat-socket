package audiosocket

import (
	"fmt"
	"net"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// ConnectHandler is invoked when a new AudioSocket session is established.
type ConnectHandler func(sessionID string, conn AudioSocketConn)

// Server listens for Asterisk AudioSocket TCP connections.
type Server struct {
	addr      string
	onConnect ConnectHandler
	listener  net.Listener
}

// NewServer creates an AudioSocket TCP listener on addr.
func NewServer(addr string, onConnect ConnectHandler) *Server {
	return &Server{addr: addr, onConnect: onConnect}
}

// Start begins accepting AudioSocket connections.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("audiosocket: listen on %s: %w", s.addr, err)
	}
	s.listener = ln

	go s.acceptLoop()
	log.WithField("addr", s.addr).Info("audiosocket server listening")
	return nil
}

// Stop closes the TCP listener.
func (s *Server) Stop() error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.listener == nil {
				return
			}
			log.WithError(err).Warn("audiosocket: accept failed")
			continue
		}

		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(raw net.Conn) {
	conn := NewTCPConn(raw)

	frame, err := conn.ReadFrame()
	if err != nil {
		log.WithError(err).Warn("audiosocket: failed to read first frame")
		_ = conn.Close()
		return
	}

	if frame.Kind != KindUUID {
		log.WithField("kind", fmt.Sprintf("0x%02x", frame.Kind)).Warn("audiosocket: expected UUID frame as first message")
		_ = conn.Close()
		return
	}

	if len(frame.Payload) != 16 {
		log.WithField("length", len(frame.Payload)).Warn("audiosocket: UUID frame must be 16 bytes")
		_ = conn.Close()
		return
	}

	parsed, err := uuid.FromBytes(frame.Payload)
	if err != nil {
		log.WithError(err).Warn("audiosocket: invalid UUID payload")
		_ = conn.Close()
		return
	}

	sessionID := parsed.String()
	if s.onConnect != nil {
		s.onConnect(sessionID, conn)
	}
}
