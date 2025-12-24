package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
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
}

// NewServer creates a new gRPC server instance
func NewServer(app MessageStreamApp) *Server {
	return &Server{
		app: app,
	}
}

// normalizeContactURN removes the URN scheme prefix (e.g., "ext:", "tel:", "whatsapp:")
// to match the format used by WebSocket clients for registration.
// Example: "ext:217138695938@" -> "217138695938@"
// TODO: This is a temporary fix. The Nexus should send contact_urn without the prefix.
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
		response, err := s.processStreamMessage(ctx, req)
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

	response, err := s.processStreamMessage(ctx, req)
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
		lastResponse, err = s.processStreamMessage(ctx, req)
		if err != nil {
			log.WithError(err).WithField("msg_id", req.MsgId).Error("gRPC: Error processing streamed message")
		}
	}
}

// processStreamMessage processes a single StreamMessage and forwards to WebSocket clients
func (s *Server) processStreamMessage(ctx context.Context, req *proto.StreamMessage) (*proto.StreamResponse, error) {
	// Handle different message types
	switch req.Type {
	case "setup":
		// Setup messages are handled by Setup RPC, but can also arrive in stream
		log.WithField("msg_id", req.MsgId).Debug("gRPC: Setup message in stream")
		return &proto.StreamResponse{
			Status:  "success",
			MsgId:   req.MsgId,
			Message: "setup acknowledged",
		}, nil

	case "delta":
		// Delta message - accumulate content and forward to WebSocket
		return s.handleDeltaMessage(ctx, req)

	case "completed":
		// Completed message - forward final message and save to history
		return s.handleCompletedMessage(ctx, req)

	case "control":
		// Control messages (typing indicators, etc.)
		return s.handleControlMessage(ctx, req)

	default:
		log.WithFields(log.Fields{
			"msg_id": req.MsgId,
			"type":   req.Type,
		}).Warn("gRPC: Unknown message type, treating as delta")
		return s.handleDeltaMessage(ctx, req)
	}
}

// handleDeltaMessage processes delta (chunk) messages
func (s *Server) handleDeltaMessage(ctx context.Context, req *proto.StreamMessage) (*proto.StreamResponse, error) {
	// Normalize contact URN to match WebSocket client registration format
	// TODO: Remove this once Nexus sends contact_urn without the "ext:" prefix
	normalizedURN := normalizeContactURN(req.ContactUrn)

	// Create WebSocket payload for delta (type will be "delta")
	// Frontend is responsible for accumulating the chunks
	payload := websocket.IncomingPayload{
		Type:        "delta", // Frontend will know it's a chunk
		To:          normalizedURN,
		From:        "system",
		ChannelUUID: req.ChannelUuid,
		Message: websocket.Message{
			Type:      "text",
			Text:      req.Content, // Send just the delta chunk
			Timestamp: req.Timestamp,
			MessageID: req.MsgId, // ID to group chunks together
		},
	}

	// Forward to WebSocket clients via Router
	if err := s.publishToWebSocket(ctx, payload); err != nil {
		return nil, fmt.Errorf("failed to publish delta to websocket: %w", err)
	}

	log.WithFields(log.Fields{
		"msg_id":       req.MsgId,
		"chunk_size":   len(req.Content),
		"contact_urn":  req.ContactUrn,
		"channel_uuid": req.ChannelUuid,
		"message_id":   req.MsgId,
	}).Debug("gRPC: Delta message forwarded to WebSocket")

	return &proto.StreamResponse{
		Status:   "success",
		MsgId:    req.MsgId,
		Message:  "delta received and forwarded",
		IsFinal:  false,
		Sequence: 0, // Could track sequence number if needed
	}, nil
}

// handleCompletedMessage processes completed messages (final message)
func (s *Server) handleCompletedMessage(ctx context.Context, req *proto.StreamMessage) (*proto.StreamResponse, error) {
	// Normalize contact URN to match WebSocket client registration format
	// TODO: Remove this once Nexus sends contact_urn without the "ext:" prefix
	normalizedURN := normalizeContactURN(req.ContactUrn)

	// Create WebSocket payload for completed message
	// Frontend already accumulated the deltas, just send the final chunk
	payload := websocket.IncomingPayload{
		Type:        "completed", // Frontend will know message is complete
		To:          normalizedURN,
		From:        "system",
		ChannelUUID: req.ChannelUuid,
		Message: websocket.Message{
			Type:      "text",
			Text:      req.Content, // Just the final chunk (or full message if no deltas)
			Timestamp: req.Timestamp,
			MessageID: req.MsgId, // ID to group chunks together
		},
	}

	// Forward to WebSocket clients
	if err := s.publishToWebSocket(ctx, payload); err != nil {
		return nil, fmt.Errorf("failed to publish completed message to websocket: %w", err)
	}

	// Save to history (only complete messages are saved)
	timestamp := time.Now().Unix()
	if req.Timestamp != "" {
		// Try to parse timestamp from request
		if ts, err := time.Parse(time.RFC3339, req.Timestamp); err == nil {
			timestamp = ts.Unix()
		}
	}

	historyPayload := websocket.NewHistoryMessagePayload(
		websocket.DirectionIn,
		req.ContactUrn,
		req.ChannelUuid,
		payload.Message,
		timestamp,
	)

	if err := s.app.Histories().Save(historyPayload); err != nil {
		log.WithError(err).WithField("msg_id", req.MsgId).Error("gRPC: Failed to save message to history")
		// Don't fail the request, just log the error
	}

	log.WithFields(log.Fields{
		"msg_id":       req.MsgId,
		"chunk_size":   len(req.Content),
		"contact_urn":  req.ContactUrn,
		"channel_uuid": req.ChannelUuid,
		"message_id":   req.MsgId,
	}).Debug("gRPC: Completed message forwarded and saved to history")

	return &proto.StreamResponse{
		Status:   "success",
		MsgId:    req.MsgId,
		Message:  "message completed and saved",
		IsFinal:  true,
		Sequence: 0,
	}, nil
}

// handleControlMessage processes control messages (typing indicators, etc.)
func (s *Server) handleControlMessage(ctx context.Context, req *proto.StreamMessage) (*proto.StreamResponse, error) {
	// Normalize contact URN to match WebSocket client registration format
	normalizedURN := normalizeContactURN(req.ContactUrn)

	// Control messages are not saved to history
	payload := websocket.IncomingPayload{
		Type:        req.Content, // e.g., "typing_start", "typing_stop"
		To:          normalizedURN,
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
