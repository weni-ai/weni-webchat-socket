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
		// Setup messages signal the start of a streaming response
		// Send stream_start to the client
		contactURN := normalizeContactURN(req.ContactUrn)
		startPayload := websocket.StreamStartPayload{
			Type: "stream_start",
			ID:   req.MsgId,
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
	// Normalize contact URN to remove scheme prefix (e.g., "ext:")
	contactURN := normalizeContactURN(req.ContactUrn)

	// Create simplified delta payload containing only the content
	deltaPayload := websocket.StreamDeltaPayload{
		V: req.Content,
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
	// Normalize contact URN to remove scheme prefix (e.g., "ext:")
	contactURN := normalizeContactURN(req.ContactUrn)

	// Send simplified stream_end payload to client
	endPayload := websocket.StreamEndPayload{
		Type: "stream_end",
		ID:   req.MsgId,
	}

	// For completed messages, we still save to history even if client is offline
	if _, err := s.publishStreamPayload(ctx, contactURN, endPayload); err != nil {
		return nil, fmt.Errorf("failed to publish stream end: %w", err)
	}

	// Save to history (only complete messages are saved)
	timestamp := time.Now().Unix()
	if req.Timestamp != "" {
		// Try to parse timestamp from request
		if ts, err := time.Parse(time.RFC3339, req.Timestamp); err == nil {
			timestamp = ts.Unix()
		}
	}

	// Create message for history (internal, not sent to client)
	historyMessage := websocket.Message{
		Type:      "text",
		Text:      req.Content,
		Timestamp: req.Timestamp,
		MessageID: req.MsgId,
	}

	historyPayload := websocket.NewHistoryMessagePayload(
		websocket.DirectionIn,
		contactURN,
		req.ChannelUuid,
		historyMessage,
		timestamp,
	)

	if err := s.app.Histories().Save(historyPayload); err != nil {
		log.WithError(err).WithField("msg_id", req.MsgId).Error("gRPC: Failed to save message to history")
		// Don't fail the request, just log the error
	}

	log.WithFields(log.Fields{
		"msg_id":       req.MsgId,
		"contact_urn":  contactURN,
		"channel_uuid": req.ChannelUuid,
	}).Debug("gRPC: Completed message - stream_end sent and saved to history")

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
