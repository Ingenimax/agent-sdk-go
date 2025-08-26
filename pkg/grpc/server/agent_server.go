package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/pb"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

// JWTTokenKey is used for JWT token context propagation - must match starops-tools exactly
const JWTTokenKey contextKey = "jwtToken"

// AgentServer implements the gRPC AgentService
type AgentServer struct {
	pb.UnimplementedAgentServiceServer
	agent        *agent.Agent
	server       *grpc.Server
	listener     net.Listener
	healthServer *health.Server
}

// NewAgentServer creates a new AgentServer wrapping the provided agent
func NewAgentServer(agent *agent.Agent) *AgentServer {
	return &AgentServer{
		agent:        agent,
		healthServer: health.NewServer(),
	}
}

// Run executes the agent with the given input
func (s *AgentServer) Run(ctx context.Context, req *pb.RunRequest) (*pb.RunResponse, error) {
	if req.Input == "" {
		return nil, status.Error(codes.InvalidArgument, "input cannot be empty")
	}

	// Add org_id to context if provided
	if req.OrgId != "" {
		ctx = multitenancy.WithOrgID(ctx, req.OrgId)
	}

	// Add conversation_id to context if provided
	if req.ConversationId != "" {
		ctx = memory.WithConversationID(ctx, req.ConversationId)
	}

	// Extract JWT token from gRPC metadata and add to context
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if auths := md.Get("authorization"); len(auths) > 0 {
			auth := auths[0]
			if strings.HasPrefix(auth, "Bearer ") {
				jwtToken := strings.TrimPrefix(auth, "Bearer ")
				ctx = context.WithValue(ctx, JWTTokenKey, jwtToken)
			}
		}
	}

	// Add context metadata using typed keys
	for key, value := range req.Context {
		ctx = context.WithValue(ctx, contextKey(key), value)
	}

	// Execute the agent
	result, err := s.agent.Run(ctx, req.Input)
	if err != nil {
		return &pb.RunResponse{
			Output: "",
			Error:  err.Error(),
		}, nil
	}

	return &pb.RunResponse{
		Output: result,
		Error:  "",
		Metadata: map[string]string{
			"agent_name": s.agent.GetName(),
		},
	}, nil
}

// RunStream executes the agent with streaming response
func (s *AgentServer) RunStream(req *pb.RunRequest, stream pb.AgentService_RunStreamServer) error {
	ctx := stream.Context()

	if req.Input == "" {
		return status.Error(codes.InvalidArgument, "input cannot be empty")
	}

	// Add org_id to context if provided
	if req.OrgId != "" {
		ctx = multitenancy.WithOrgID(ctx, req.OrgId)
	}

	// Add conversation_id to context if provided
	if req.ConversationId != "" {
		ctx = memory.WithConversationID(ctx, req.ConversationId)
	}

	// Extract JWT token from gRPC metadata and add to context
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if auths := md.Get("authorization"); len(auths) > 0 {
			auth := auths[0]
			if strings.HasPrefix(auth, "Bearer ") {
				jwtToken := strings.TrimPrefix(auth, "Bearer ")
				ctx = context.WithValue(ctx, JWTTokenKey, jwtToken)
			}
		}
	}

	// Add context metadata using typed keys
	for key, value := range req.Context {
		ctx = context.WithValue(ctx, contextKey(key), value)
	}

	// Check if agent supports streaming
	streamingAgent, ok := interface{}(s.agent).(interfaces.StreamingAgent)
	if !ok {
		// Fall back to non-streaming execution
		response, err := s.Run(ctx, req)
		if err != nil {
			return err
		}

		// Send as single chunk
		chunk := &pb.RunStreamResponse{
			Chunk:     response.Output,
			IsFinal:   true,
			EventType: pb.EventType_EVENT_TYPE_CONTENT,
			Timestamp: time.Now().UnixMilli(),
		}

		if response.Error != "" {
			chunk.Error = response.Error
			chunk.EventType = pb.EventType_EVENT_TYPE_ERROR
		}

		return stream.Send(chunk)
	}

	// Get streaming events from agent
	eventChan, err := streamingAgent.RunStream(ctx, req.Input)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to start agent streaming: %v", err)
	}

	// Stream events to client
	for event := range eventChan {
		response := &pb.RunStreamResponse{
			Chunk:     event.Content,
			EventType: s.convertEventType(event.Type),
			IsFinal:   false,
			Timestamp: event.Timestamp.UnixMilli(),
		}

		// Add metadata if present
		if event.Metadata != nil {
			response.Metadata = make(map[string]string)
			for k, v := range event.Metadata {
				if str, ok := v.(string); ok {
					response.Metadata[k] = str
				} else {
					response.Metadata[k] = fmt.Sprintf("%v", v)
				}
			}
		}

		// Add tool call info if present
		if event.ToolCall != nil {
			response.ToolCall = &pb.ToolCall{
				Id:        event.ToolCall.ID,
				Name:      event.ToolCall.Name,
				Arguments: event.ToolCall.Arguments,
				Result:    event.ToolCall.Result,
				Status:    event.ToolCall.Status,
			}
		}

		// Add thinking if present
		if event.ThinkingStep != "" {
			response.Thinking = event.ThinkingStep
		}

		// Handle errors - only set error event type for non-tool errors
		// Tool errors should remain as tool result events with error status
		if event.Error != nil && event.Type != interfaces.AgentEventToolResult {
			response.Error = event.Error.Error()
			response.EventType = pb.EventType_EVENT_TYPE_ERROR
		}

		// Handle completion
		if event.Type == interfaces.AgentEventComplete {
			response.IsFinal = true
			response.EventType = pb.EventType_EVENT_TYPE_COMPLETE
		}

		// Send the event
		if err := stream.Send(response); err != nil {
			return status.Errorf(codes.Internal, "failed to send stream response: %v", err)
		}
	}

	// Send final completion if we haven't already
	finalResponse := &pb.RunStreamResponse{
		IsFinal:   true,
		EventType: pb.EventType_EVENT_TYPE_COMPLETE,
		Timestamp: time.Now().UnixMilli(),
	}

	return stream.Send(finalResponse)
}

// convertEventType converts agent event types to protobuf event types
func (s *AgentServer) convertEventType(eventType interfaces.AgentEventType) pb.EventType {
	switch eventType {
	case interfaces.AgentEventContent:
		return pb.EventType_EVENT_TYPE_CONTENT
	case interfaces.AgentEventThinking:
		return pb.EventType_EVENT_TYPE_THINKING
	case interfaces.AgentEventToolCall:
		return pb.EventType_EVENT_TYPE_TOOL_CALL
	case interfaces.AgentEventToolResult:
		return pb.EventType_EVENT_TYPE_TOOL_RESULT
	case interfaces.AgentEventError:
		return pb.EventType_EVENT_TYPE_ERROR
	case interfaces.AgentEventComplete:
		return pb.EventType_EVENT_TYPE_COMPLETE
	default:
		return pb.EventType_EVENT_TYPE_CONTENT
	}
}

// GetMetadata returns agent metadata
func (s *AgentServer) GetMetadata(ctx context.Context, req *pb.MetadataRequest) (*pb.MetadataResponse, error) {
	return &pb.MetadataResponse{
		Name:         s.agent.GetName(),
		Description:  s.agent.GetDescription(),
		SystemPrompt: "", // Don't expose system prompt for security
		Capabilities: []string{
			"run",
			"metadata",
			"health",
		},
		Properties: map[string]string{
			"type":    "agent",
			"version": "1.0.0",
		},
	}, nil
}

// GetCapabilities returns agent capabilities
func (s *AgentServer) GetCapabilities(ctx context.Context, req *pb.CapabilitiesRequest) (*pb.CapabilitiesResponse, error) {
	// Get tool names (simplified for now)
	var toolNames []string
	// Note: This would require exposing tool information from the agent
	// For now, we'll just return basic capabilities

	var subAgentNames []string
	// Note: This would require exposing subagent information from the agent
	// For now, we'll just return basic capabilities

	return &pb.CapabilitiesResponse{
		Tools:                  toolNames,
		SubAgents:              subAgentNames,
		SupportsExecutionPlans: true, // Most agents support execution plans
		SupportsMemory:         true, // Most agents support memory
		SupportsStreaming:      true, // Now implemented!
	}, nil
}

// Health returns the health status of the agent service
func (s *AgentServer) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	// Simple health check - if we can respond, we're healthy
	return &pb.HealthResponse{
		Status:  pb.HealthResponse_SERVING,
		Message: "Agent service is healthy",
	}, nil
}

// Ready returns the readiness status of the agent service
func (s *AgentServer) Ready(ctx context.Context, req *pb.ReadinessRequest) (*pb.ReadinessResponse, error) {
	// Check if agent is properly initialized
	if s.agent == nil {
		return &pb.ReadinessResponse{
			Ready:   false,
			Message: "Agent is not initialized",
		}, nil
	}

	if s.agent.GetName() == "" {
		return &pb.ReadinessResponse{
			Ready:   false,
			Message: "Agent name is not set",
		}, nil
	}

	return &pb.ReadinessResponse{
		Ready:   true,
		Message: "Agent service is ready",
	}, nil
}

// GenerateExecutionPlan generates an execution plan (if the agent supports it)
func (s *AgentServer) GenerateExecutionPlan(ctx context.Context, req *pb.PlanRequest) (*pb.PlanResponse, error) {
	// Add org_id to context if provided
	if req.OrgId != "" {
		ctx = multitenancy.WithOrgID(ctx, req.OrgId)
	}

	// Add conversation_id to context if provided
	if req.ConversationId != "" {
		ctx = memory.WithConversationID(ctx, req.ConversationId)
	}

	// Extract JWT token from gRPC metadata and add to context
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if auths := md.Get("authorization"); len(auths) > 0 {
			auth := auths[0]
			if strings.HasPrefix(auth, "Bearer ") {
				jwtToken := strings.TrimPrefix(auth, "Bearer ")
				ctx = context.WithValue(ctx, JWTTokenKey, jwtToken)
			}
		}
	}

	// Add context metadata using typed keys
	for key, value := range req.Context {
		ctx = context.WithValue(ctx, contextKey(key), value)
	}

	// Try to generate an execution plan
	plan, err := s.agent.GenerateExecutionPlan(ctx, req.Input)
	if err != nil {
		return &pb.PlanResponse{
			Error: fmt.Sprintf("Failed to generate execution plan: %v", err),
		}, nil
	}

	// Convert plan to protobuf format
	var steps []*pb.PlanStep
	for i, step := range plan.Steps {
		// Convert parameters from map[string]interface{} to map[string]string
		paramMap := make(map[string]string)
		for k, v := range step.Parameters {
			paramMap[k] = fmt.Sprintf("%v", v)
		}

		steps = append(steps, &pb.PlanStep{
			Id:          fmt.Sprintf("step_%d", i+1), // Generate ID since ExecutionStep doesn't have one
			Description: step.Description,
			ToolName:    step.ToolName,
			Parameters:  paramMap,
		})
	}

	return &pb.PlanResponse{
		PlanId:        plan.TaskID,
		FormattedPlan: formatExecutionPlan(plan),
		Steps:         steps,
	}, nil
}

// ApproveExecutionPlan approves an execution plan
func (s *AgentServer) ApproveExecutionPlan(ctx context.Context, req *pb.ApprovalRequest) (*pb.ApprovalResponse, error) {
	// Get the plan by ID
	plan, exists := s.agent.GetTaskByID(req.PlanId)
	if !exists {
		return &pb.ApprovalResponse{
			Error: fmt.Sprintf("Plan with ID %s not found", req.PlanId),
		}, nil
	}

	var result string
	var err error

	if req.Approved {
		if req.Modifications != "" {
			// Modify the plan first
			modifiedPlan, modErr := s.agent.ModifyExecutionPlan(ctx, plan, req.Modifications)
			if modErr != nil {
				return &pb.ApprovalResponse{
					Error: fmt.Sprintf("Failed to modify plan: %v", modErr),
				}, nil
			}
			plan = modifiedPlan
		}

		// Approve and execute the plan
		result, err = s.agent.ApproveExecutionPlan(ctx, plan)
		if err != nil {
			return &pb.ApprovalResponse{
				Error: fmt.Sprintf("Failed to execute approved plan: %v", err),
			}, nil
		}
	} else {
		result = "Plan rejected by user"
	}

	return &pb.ApprovalResponse{
		Result: result,
	}, nil
}

// Start starts the gRPC server on the specified port
func (s *AgentServer) Start(port int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	return s.StartWithListener(listener)
}

// StartWithListener starts the gRPC server with an existing listener
func (s *AgentServer) StartWithListener(listener net.Listener) error {
	s.listener = listener
	s.server = grpc.NewServer()

	// Register the agent service
	pb.RegisterAgentServiceServer(s.server, s)

	// Register the standard gRPC health service
	grpc_health_v1.RegisterHealthServer(s.server, s.healthServer)

	// Set the health status to SERVING for the overall service and agent service
	s.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	s.healthServer.SetServingStatus("AgentService", grpc_health_v1.HealthCheckResponse_SERVING)

	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Printf("Agent server starting on port %d...\n", port)
	return s.server.Serve(listener)
}

// Stop stops the gRPC server
func (s *AgentServer) Stop() {
	if s.healthServer != nil {
		// Set health status to NOT_SERVING before stopping
		s.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		s.healthServer.SetServingStatus("AgentService", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	}

	if s.server != nil {
		s.server.GracefulStop()
	}
}

// GetPort returns the port the server is listening on
func (s *AgentServer) GetPort() int {
	if s.listener != nil {
		return s.listener.Addr().(*net.TCPAddr).Port
	}
	return 0
}

// formatExecutionPlan formats an execution plan for display
func formatExecutionPlan(plan interface{}) string {
	// This is a placeholder implementation
	// In reality, you would use the actual execution plan formatting logic
	return fmt.Sprintf("Execution Plan: %+v", plan)
}
