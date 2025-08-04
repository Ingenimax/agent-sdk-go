package server

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/pb"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

// AgentServer implements the gRPC AgentService
type AgentServer struct {
	pb.UnimplementedAgentServiceServer
	agent    *agent.Agent
	server   *grpc.Server
	listener net.Listener
}

// NewAgentServer creates a new AgentServer wrapping the provided agent
func NewAgentServer(agent *agent.Agent) *AgentServer {
	return &AgentServer{
		agent: agent,
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

// RunStream executes the agent with streaming response (placeholder for future implementation)
func (s *AgentServer) RunStream(req *pb.RunRequest, stream pb.AgentService_RunStreamServer) error {
	// For now, just return the regular response as a single chunk
	response, err := s.Run(stream.Context(), req)
	if err != nil {
		return err
	}

	// Send the response as a single chunk
	chunk := &pb.RunStreamResponse{
		Chunk:   response.Output,
		IsFinal: true,
	}

	if response.Error != "" {
		chunk.Error = response.Error
	}

	return stream.Send(chunk)
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
		Tools:                   toolNames,
		SubAgents:              subAgentNames,
		SupportsExecutionPlans: true, // Most agents support execution plans
		SupportsMemory:         true, // Most agents support memory
		SupportsStreaming:      false, // Not implemented yet
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

	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Printf("Agent server starting on port %d...\n", port)
	return s.server.Serve(listener)
}

// Stop stops the gRPC server
func (s *AgentServer) Stop() {
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

