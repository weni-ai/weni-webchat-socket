package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ilhasoft/wwcs/pkg/grpc/proto"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/streams"
	"github.com/ilhasoft/wwcs/pkg/websocket"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MessageStreamApp defines the interface needed from the websocket.App
type MessageStreamApp interface {
	Router() streams.Router
	Histories() history.Service
	ClientManager() websocket.ClientManager
}

// messageStreamApp is an adapter to convert websocket.App to MessageStreamApp interface
type messageStreamApp struct {
	app *websocket.App
}

// NewMessageStreamApp wraps a websocket.App to implement MessageStreamApp interface
func NewMessageStreamApp(app *websocket.App) MessageStreamApp {
	return &messageStreamApp{app: app}
}

func (m *messageStreamApp) Router() streams.Router {
	return m.app.Router
}

func (m *messageStreamApp) Histories() history.Service {
	return m.app.Histories
}

func (m *messageStreamApp) ClientManager() websocket.ClientManager {
	return m.app.ClientManager
}

// Server implements the gRPC MessageStreamService
type Server struct {
	proto.UnimplementedMessageStreamServiceServer
	app MessageStreamApp

	// unarySeqTracker maintains sequence numbers for unary SendMessage calls.
	// Key: msgId, Value: next sequence number (1-indexed).
	// Protected by unarySeqMu for concurrent access.
	unarySeqTracker map[string]int64
	unarySeqMu      sync.Mutex
}

// NewServer creates a new gRPC server instance
func NewServer(app MessageStreamApp) *Server {
	return &Server{
		app:             app,
		unarySeqTracker: make(map[string]int64),
	}
}

// normalizeContactURN removes the URN scheme prefix (e.g., "ext:", "tel:", "whatsapp:")
// to match the format used by WebSocket clients for registration.
// Example: "ext:217138695938@" -> "217138695938@"
func normalizeContactURN(contactURN string) string {
	if idx := strings.Index(contactURN, ":"); idx != -1 {
		normalized := contactURN[idx+1:]
		log.WithFields(log.Fields{
			"original":   contactURN,
			"normalized": normalized,
		}).Debug("gRPC: Normalized contact URN (removed scheme prefix)")
		return normalized
	}
	return contactURN
}

// Setup handles initial setup from external service (Nexus)
func (s *Server) Setup(ctx context.Context, req *proto.SetupRequest) (*proto.SetupResponse, error) {
	log.WithFields(log.Fields{
		"msg_id":       req.MsgId,
		"channel_uuid": req.ChannelUuid,
		"contact_urn":  req.ContactUrn,
		"project_uuid": req.ProjectUuid,
	}).Info("gRPC: Setup request received")

	// Validate client is connected
	connectedClient, err := s.app.ClientManager().GetConnectedClient(req.ContactUrn)
	if err != nil {
		log.WithError(err).Error("gRPC: Error checking connected client")
		return &proto.SetupResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("error checking connected client: %v", err),
		}, nil
	}

	if connectedClient == nil {
		log.WithField("contact_urn", req.ContactUrn).Warn("gRPC: Client not connected")
		return &proto.SetupResponse{
			Success:      false,
			ErrorMessage: "client is not connected",
		}, nil
	}

	sessionID := fmt.Sprintf("session_%s_%d", req.MsgId, time.Now().Unix())

	log.WithFields(log.Fields{
		"session_id": sessionID,
		"msg_id":     req.MsgId,
	}).Info("gRPC: Setup completed successfully")

	return &proto.SetupResponse{
		Success:   true,
		SessionId: sessionID,
		Message:   "streaming session initialized",
	}, nil
}

// StreamMessages handles bidirectional streaming of message chunks (deltas)
func (s *Server) StreamMessages(stream proto.MessageStreamService_StreamMessagesServer) error {
	ctx := stream.Context()
	streamID := fmt.Sprintf("stream_%d", time.Now().UnixNano())

	log.WithField("stream_id", streamID).Info("gRPC: New bidirectional stream established")

	// Track sequence numbers per message ID within this stream
	// Key: msgId, Value: next sequence number (1-indexed)
	seqTracker := make(map[string]int64)

	for {
		// Receive message from external service (Nexus)
		req, err := stream.Recv()
		if err == io.EOF {
			log.WithField("stream_id", streamID).Info("gRPC: Stream closed by client")
			return nil
		}
		if err != nil {
			log.WithError(err).Error("gRPC: Error receiving stream message")
			return status.Errorf(codes.Internal, "error receiving message: %v", err)
		}

		log.WithFields(log.Fields{
			"stream_id":    streamID,
			"msg_id":       req.MsgId,
			"type":         req.Type,
			"contact_urn":  req.ContactUrn,
			"channel_uuid": req.ChannelUuid,
			"content_size": len(req.Content),
		}).Debug("gRPC: Received message")

		// Process the message based on type
		response, err := s.processStreamMessageWithSeq(ctx, req, seqTracker)
		if err != nil {
			log.WithError(err).WithField("msg_id", req.MsgId).Error("gRPC: Error processing message")
			response = &proto.StreamResponse{
				Status:       "error",
				MsgId:        req.MsgId,
				ErrorCode:    "PROCESSING_ERROR",
				ErrorMessage: err.Error(),
			}
		}

		// Send response back to external service
		if err := stream.Send(response); err != nil {
			log.WithError(err).Error("gRPC: Error sending response")
			return status.Errorf(codes.Internal, "error sending response: %v", err)
		}
	}
}

// SendMessage handles single complete message (unary RPC)
func (s *Server) SendMessage(ctx context.Context, req *proto.StreamMessage) (*proto.StreamResponse, error) {
	log.WithFields(log.Fields{
		"msg_id":       req.MsgId,
		"type":         req.Type,
		"contact_urn":  req.ContactUrn,
		"channel_uuid": req.ChannelUuid,
		"content_size": len(req.Content),
	}).Debug("gRPC: Single message received")

	// For unary calls, use the server-level sequence tracker to maintain
	// sequence numbers across multiple SendMessage calls for the same msgId.
	// This allows streaming deltas via unary calls with correct sequencing.
	response, err := s.processStreamMessageWithUnarySeq(ctx, req)
	if err != nil {
		log.WithError(err).Error("gRPC: Error processing single message")
		return &proto.StreamResponse{
			Status:       "error",
			MsgId:        req.MsgId,
			ErrorCode:    "PROCESSING_ERROR",
			ErrorMessage: err.Error(),
		}, nil
	}

	return response, nil
}

// StreamToServer handles client streaming (external service streams, server responds once)
func (s *Server) StreamToServer(stream proto.MessageStreamService_StreamToServerServer) error {
	ctx := stream.Context()
	streamID := fmt.Sprintf("stream_to_server_%d", time.Now().UnixNano())

	log.WithField("stream_id", streamID).Info("gRPC: New client-stream established")

	// Track sequence numbers per message ID within this stream
	seqTracker := make(map[string]int64)

	var lastMsgID string
	var lastResponse *proto.StreamResponse

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			// Stream finished, send final response
			if lastResponse != nil {
				return stream.SendAndClose(lastResponse)
			}
			return stream.SendAndClose(&proto.StreamResponse{
				Status:  "success",
				MsgId:   lastMsgID,
				Message: "stream completed",
			})
		}
		if err != nil {
			log.WithError(err).Error("gRPC: Error receiving from client stream")
			return status.Errorf(codes.Internal, "error receiving: %v", err)
		}

		lastMsgID = req.MsgId
		lastResponse, err = s.processStreamMessageWithSeq(ctx, req, seqTracker)
		if err != nil {
			log.WithError(err).WithField("msg_id", req.MsgId).Error("gRPC: Error processing streamed message")
		}
	}
}

// processStreamMessage processes a single StreamMessage without sequence tracking.
// Used for backward compatibility with code that doesn't need sequence numbers.
func (s *Server) processStreamMessage(ctx context.Context, req *proto.StreamMessage) (*proto.StreamResponse, error) {
	return s.processStreamMessageWithSeq(ctx, req, nil)
}

// processStreamMessageWithUnarySeq processes a StreamMessage using the server-level
// sequence tracker for unary calls. This allows multiple SendMessage calls to maintain
// proper sequence ordering for the same msgId.
func (s *Server) processStreamMessageWithUnarySeq(ctx context.Context, req *proto.StreamMessage) (*proto.StreamResponse, error) {
	// Handle setup and completed messages which modify the sequence tracker
	switch req.Type {
	case "setup":
		// Reset sequence counter for this message ID
		s.unarySeqMu.Lock()
		s.unarySeqTracker[req.MsgId] = 0
		s.unarySeqMu.Unlock()
		return s.processStreamMessageWithSeq(ctx, req, nil)

	case "completed":
		// Clean up sequence tracker for this msgId
		s.unarySeqMu.Lock()
		delete(s.unarySeqTracker, req.MsgId)
		s.unarySeqMu.Unlock()
		return s.processStreamMessageWithSeq(ctx, req, nil)

	case "delta":
		// Get and increment sequence number atomically
		s.unarySeqMu.Lock()
		s.unarySeqTracker[req.MsgId]++
		seq := s.unarySeqTracker[req.MsgId]
		s.unarySeqMu.Unlock()
		return s.handleDeltaMessageWithSeq(ctx, req, seq)

	default:
		// For unknown types treated as delta
		s.unarySeqMu.Lock()
		s.unarySeqTracker[req.MsgId]++
		seq := s.unarySeqTracker[req.MsgId]
		s.unarySeqMu.Unlock()
		return s.handleDeltaMessageWithSeq(ctx, req, seq)
	}
}

// processStreamMessageWithSeq processes a single StreamMessage and forwards to WebSocket clients.
// seqTracker maintains sequence numbers per msgId within the stream (can be nil for non-delta messages).
func (s *Server) processStreamMessageWithSeq(ctx context.Context, req *proto.StreamMessage, seqTracker map[string]int64) (*proto.StreamResponse, error) {
	// Handle different message types
	switch req.Type {
	case "setup":
		// Setup messages signal the start of a streaming response
		// Send stream_start to the client and reset sequence counter for this msgId
		contactURN := normalizeContactURN(req.ContactUrn)
		startPayload := websocket.StreamStartPayload{
			Type: "stream_start",
			ID:   req.MsgId,
		}
		// Reset sequence counter for this message ID
		if seqTracker != nil {
			seqTracker[req.MsgId] = 0
		}
		published, err := s.publishStreamPayload(ctx, contactURN, startPayload)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"msg_id":      req.MsgId,
				"contact_urn": contactURN,
			}).Error("gRPC: Failed to publish stream_start")
			return &proto.StreamResponse{
				Status:       "error",
				MsgId:        req.MsgId,
				ErrorCode:    "PUBLISH_ERROR",
				ErrorMessage: err.Error(),
			}, nil
		}
		if !published {
			// Client is not connected - fail setup to prevent confusing state
			log.WithFields(log.Fields{
				"msg_id":      req.MsgId,
				"contact_urn": contactURN,
			}).Debug("gRPC: Setup failed - client not connected")
			return &proto.StreamResponse{
				Status:       "error",
				MsgId:        req.MsgId,
				ErrorCode:    "CLIENT_OFFLINE",
				ErrorMessage: "client is not connected",
			}, nil
		}
		log.WithFields(log.Fields{
			"msg_id":      req.MsgId,
			"contact_urn": contactURN,
		}).Debug("gRPC: Setup message - stream_start sent to client")
		return &proto.StreamResponse{
			Status:  "success",
			MsgId:   req.MsgId,
			Message: "setup acknowledged, stream_start sent",
		}, nil

	case "delta":
		// Delta message - accumulate content and forward to WebSocket
		return s.handleDeltaMessage(ctx, req, seqTracker)

	case "completed":
		// Completed message - forward final message and save to history
		// Clean up sequence tracker for this msgId
		if seqTracker != nil {
			delete(seqTracker, req.MsgId)
		}
		return s.handleCompletedMessage(ctx, req)

	case "control":
		// Control messages (typing indicators, etc.)
		return s.handleControlMessage(ctx, req)

	default:
		log.WithFields(log.Fields{
			"msg_id": req.MsgId,
			"type":   req.Type,
		}).Warn("gRPC: Unknown message type, treating as delta")
		return s.handleDeltaMessage(ctx, req, seqTracker)
	}
}

// handleDeltaMessage processes delta (chunk) messages
// seqTracker maintains sequence numbers per msgId (can be nil, in which case seq defaults to 1)
func (s *Server) handleDeltaMessage(ctx context.Context, req *proto.StreamMessage, seqTracker map[string]int64) (*proto.StreamResponse, error) {
	// Normalize contact URN to remove scheme prefix (e.g., "ext:")
	contactURN := normalizeContactURN(req.ContactUrn)

	// Get and increment sequence number for this message ID
	var seq int64 = 1
	if seqTracker != nil {
		seqTracker[req.MsgId]++
		seq = seqTracker[req.MsgId]
	}

	return s.sendDeltaWithSeq(ctx, req, contactURN, seq)
}

// handleDeltaMessageWithSeq processes delta messages with a pre-computed sequence number.
// Used by unary calls where sequence is tracked at the server level.
func (s *Server) handleDeltaMessageWithSeq(ctx context.Context, req *proto.StreamMessage, seq int64) (*proto.StreamResponse, error) {
	contactURN := normalizeContactURN(req.ContactUrn)
	return s.sendDeltaWithSeq(ctx, req, contactURN, seq)
}

// sendDeltaWithSeq sends a delta payload with the given sequence number.
// This is the common implementation used by both handleDeltaMessage and handleDeltaMessageWithSeq.
func (s *Server) sendDeltaWithSeq(ctx context.Context, req *proto.StreamMessage, contactURN string, seq int64) (*proto.StreamResponse, error) {
	// Create delta payload with sequence number for client-side ordering
	deltaPayload := websocket.StreamDeltaPayload{
		V:   req.Content,
		Seq: seq,
	}

	// Forward to WebSocket clients via Router
	// For deltas, we don't fail if client is offline - they just miss this chunk
	if _, err := s.publishStreamPayload(ctx, contactURN, deltaPayload); err != nil {
		return nil, fmt.Errorf("failed to publish delta: %w", err)
	}

	log.WithFields(log.Fields{
		"msg_id":      req.MsgId,
		"chunk_size":  len(req.Content),
		"contact_urn": contactURN,
		"seq":         seq,
	}).Debug("gRPC: Delta message forwarded to WebSocket")

	return &proto.StreamResponse{
		Status:   "success",
		MsgId:    req.MsgId,
		Message:  "delta received and forwarded",
		IsFinal:  false,
		Sequence: int32(seq),
	}, nil
}

// handleCompletedMessage processes completed messages (final message)
func (s *Server) handleCompletedMessage(ctx context.Context, req *proto.StreamMessage) (*proto.StreamResponse, error) {
	// Normalize contact URN to remove scheme prefix (e.g., "ext:")
	contactURN := normalizeContactURN(req.ContactUrn)

	// Send simplified stream_end payload to client
	endPayload := websocket.StreamEndPayload{
		Type: "stream_end",
		ID:   req.MsgId,
	}

	// Publish stream_end to client
	if _, err := s.publishStreamPayload(ctx, contactURN, endPayload); err != nil {
		return nil, fmt.Errorf("failed to publish stream end: %w", err)
	}

	// Save the completed streamed message to history
	if err := s.saveStreamedMessageToHistory(req, contactURN); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"msg_id":       req.MsgId,
			"contact_urn":  contactURN,
			"channel_uuid": req.ChannelUuid,
		}).Error("gRPC: Failed to save streamed message to history")
	}

	log.WithFields(log.Fields{
		"msg_id":       req.MsgId,
		"contact_urn":  contactURN,
		"channel_uuid": req.ChannelUuid,
	}).Debug("gRPC: Completed message - stream_end sent to client and saved to history")

	return &proto.StreamResponse{
		Status:   "success",
		MsgId:    req.MsgId,
		Message:  "message completed, stream_end sent",
		IsFinal:  true,
		Sequence: 0,
	}, nil
}

// saveStreamedMessageToHistory saves a completed streamed message to MongoDB history
func (s *Server) saveStreamedMessageToHistory(req *proto.StreamMessage, contactURN string) error {
	histories := s.app.Histories()
	if histories == nil {
		return fmt.Errorf("history service is not available")
	}

	// Parse timestamp from request, default to current time if not provided
	var timestamp int64
	if req.Timestamp != "" {
		var err error
		timestamp, err = parseTimestamp(req.Timestamp)
		if err != nil {
			log.WithError(err).WithField("timestamp", req.Timestamp).Warn("gRPC: Failed to parse timestamp, using current time")
			timestamp = time.Now().Unix()
		}
	} else {
		timestamp = time.Now().Unix()
	}

	// Create history message payload
	// Direction is "in" because this is an incoming message to the user (from the server/AI)
	msgPayload := history.MessagePayload{
		ContactURN:  contactURN,
		ChannelUUID: req.ChannelUuid,
		Direction:   "in",
		Timestamp:   timestamp,
		Message: history.Message{
			Type:      "text",
			Text:      req.Content,
			Timestamp: req.Timestamp,
		},
	}

	log.WithFields(log.Fields{
		"msg_id":       req.MsgId,
		"contact_urn":  contactURN,
		"channel_uuid": req.ChannelUuid,
		"content_size": len(req.Content),
		"timestamp":    timestamp,
		"source":       "grpc_stream_completed",
	}).Debug("gRPC: Saving streamed message to history")

	return histories.Save(msgPayload)
}

// parseTimestamp attempts to parse a timestamp string to Unix timestamp
func parseTimestamp(ts string) (int64, error) {
	// Try parsing as Unix timestamp (seconds)
	if timestamp, err := strconv.ParseInt(ts, 10, 64); err == nil {
		return timestamp, nil
	}

	// Try parsing as RFC3339
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Unix(), nil
	}

	// Try parsing as RFC3339Nano
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.Unix(), nil
	}

	return 0, fmt.Errorf("unable to parse timestamp: %s", ts)
}

// handleControlMessage processes control messages (typing indicators, etc.)
func (s *Server) handleControlMessage(ctx context.Context, req *proto.StreamMessage) (*proto.StreamResponse, error) {
	// Normalize contact URN to remove scheme prefix (e.g., "ext:")
	contactURN := normalizeContactURN(req.ContactUrn)

	// Control messages are not saved to history
	payload := websocket.IncomingPayload{
		Type:        req.Content, // e.g., "typing_start", "typing_stop"
		To:          contactURN,
		From:        "system",
		ChannelUUID: req.ChannelUuid,
	}

	if err := s.publishToWebSocket(ctx, payload); err != nil {
		return nil, fmt.Errorf("failed to publish control message: %w", err)
	}

	log.WithFields(log.Fields{
		"msg_id":      req.MsgId,
		"control":     req.Content,
		"contact_urn": req.ContactUrn,
	}).Debug("gRPC: Control message forwarded")

	return &proto.StreamResponse{
		Status:  "success",
		MsgId:   req.MsgId,
		Message: "control message forwarded",
	}, nil
}

// publishToWebSocket publishes a message to WebSocket clients via Router
func (s *Server) publishToWebSocket(ctx context.Context, payload websocket.IncomingPayload) error {
	router := s.app.Router()
	if router == nil {
		return fmt.Errorf("router is not available")
	}

	// Check if client is connected
	clientManager := s.app.ClientManager()
	if clientManager == nil {
		return fmt.Errorf("client manager is not available")
	}
	connectedClient, err := clientManager.GetConnectedClient(payload.To)
	if err != nil {
		return fmt.Errorf("error checking connected client: %w", err)
	}

	if connectedClient == nil {
		log.WithFields(log.Fields{
			"to":           payload.To,
			"type":         payload.Type,
			"channel_uuid": payload.ChannelUUID,
			"message_id":   payload.Message.MessageID,
		}).Debug("gRPC: Client not connected, message not published")
		return nil // Not an error, client just offline
	}

	// Marshal payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling payload: %w", err)
	}

	// Publish via Router (handles multi-pod routing)
	if err := router.PublishToClient(ctx, payload.To, payloadJSON); err != nil {
		return fmt.Errorf("error publishing to router: %w", err)
	}

	// Successful publish to Router/Redis
	log.WithFields(log.Fields{
		"to":           payload.To,
		"type":         payload.Type,
		"channel_uuid": payload.ChannelUUID,
		"message_id":   payload.Message.MessageID,
		"payload_size": len(payloadJSON),
	}).Debug("gRPC: Published message to Router")

	return nil
}

// publishStreamPayload publishes a streaming payload to WebSocket clients via Router.
// Accepts any type implementing StreamPayload interface for type safety.
// Returns (published, error) where published indicates if the message was actually sent.
// When client is offline, returns (false, nil) - not an error, but message wasn't delivered.
func (s *Server) publishStreamPayload(ctx context.Context, contactURN string, payload websocket.StreamPayload) (bool, error) {
	router := s.app.Router()
	if router == nil {
		return false, fmt.Errorf("router is not available")
	}

	// Check if client is connected
	clientManager := s.app.ClientManager()
	if clientManager == nil {
		return false, fmt.Errorf("client manager is not available")
	}
	connectedClient, err := clientManager.GetConnectedClient(contactURN)
	if err != nil {
		return false, fmt.Errorf("error checking connected client: %w", err)
	}

	if connectedClient == nil {
		log.WithFields(log.Fields{
			"contact_urn": contactURN,
		}).Debug("gRPC: Client not connected, stream payload not published")
		return false, nil // Not an error, client just offline
	}

	// Marshal payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("error marshaling stream payload: %w", err)
	}

	// Publish via Router (handles multi-pod routing)
	if err := router.PublishToClient(ctx, contactURN, payloadJSON); err != nil {
		return false, fmt.Errorf("error publishing stream payload to router: %w", err)
	}

	log.WithFields(log.Fields{
		"contact_urn":  contactURN,
		"payload_size": len(payloadJSON),
	}).Debug("gRPC: Published stream payload to Router")

	return true, nil
}
