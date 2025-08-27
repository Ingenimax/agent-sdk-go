package client

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/pb"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// contextKey is a custom type for context keys
type contextKey string

// JWTTokenKey is used for JWT token context propagation
const JWTTokenKey contextKey = "jwtToken"

// jwtContextKey matches starops-agent middleware exactly
type jwtContextKey struct{}

// Use exact same variable name and type as starops-agent middleware
var JWTTokenKeyStruct = jwtContextKey{}

// RemoteAgentClient handles communication with remote agents via gRPC
type RemoteAgentClient struct {
	url        string
	conn       *grpc.ClientConn
	client     pb.AgentServiceClient
	timeout    time.Duration
	retryCount int
	
	// Event handlers
	thinkingHandlers []func(string)
	contentHandlers  []func(string)
	toolCallHandlers []func(*interfaces.ToolCallEvent)
	toolResultHandlers []func(*interfaces.ToolCallEvent)
	errorHandlers    []func(error)
	completeHandlers []func()
	handlersMu       sync.RWMutex
	
}

// RemoteAgentConfig configures the remote agent client
type RemoteAgentConfig struct {
	URL        string
	Timeout    time.Duration
	RetryCount int
}

// NewRemoteAgentClient creates a new remote agent client
func NewRemoteAgentClient(config RemoteAgentConfig) *RemoteAgentClient {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute
	}
	if config.RetryCount == 0 {
		config.RetryCount = 3
	}

	return &RemoteAgentClient{
		url:        config.URL,
		timeout:    config.Timeout,
		retryCount: config.RetryCount,
	}
}

// Connect establishes a connection to the remote agent service
func (r *RemoteAgentClient) Connect() error {
	if r.conn != nil {
		return nil // Already connected
	}

	conn, err := grpc.NewClient(r.url,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", r.url, err)
	}

	r.conn = conn
	r.client = pb.NewAgentServiceClient(conn)

	// Test the connection with standard gRPC health check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	healthClient := grpc_health_v1.NewHealthClient(conn)
	_, err = healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: "", // Check the overall server health (empty string means overall server)
	})
	if err != nil {
		if closeErr := r.conn.Close(); closeErr != nil {
			// Log the close error but continue with the original error
			fmt.Printf("Warning: failed to close connection during cleanup: %v\n", closeErr)
		}
		r.conn = nil
		r.client = nil
		return fmt.Errorf("health check failed for %s: %w", r.url, err)
	}

	return nil
}

// Disconnect closes the connection to the remote agent service
func (r *RemoteAgentClient) Disconnect() error {
	if r.conn != nil {
		err := r.conn.Close()
		r.conn = nil
		r.client = nil
		return err
	}
	return nil
}

// Run executes the remote agent with the given input
func (r *RemoteAgentClient) Run(ctx context.Context, input string) (string, error) {
	if err := r.ensureConnected(); err != nil {
		return "", err
	}

	// Create request
	req := &pb.RunRequest{
		Input:   input,
		Context: make(map[string]string),
	}

	// Add org_id from context if available
	if orgID, _ := multitenancy.GetOrgID(ctx); orgID != "" {
		req.OrgId = orgID
	}

	// Add conversation_id from context if available
	if conversationID, ok := memory.GetConversationID(ctx); ok && conversationID != "" {
		req.ConversationId = conversationID
	}

	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// Execute with retry logic
	var lastErr error
	for attempt := 0; attempt < r.retryCount; attempt++ {
		resp, err := r.client.Run(ctx, req)
		if err != nil {
			lastErr = err
			// Exponential backoff
			if attempt < r.retryCount-1 {
				backoff := time.Duration(attempt+1) * time.Second
				time.Sleep(backoff)
			}
			continue
		}

		if resp.Error != "" {
			return "", fmt.Errorf("remote agent error: %s", resp.Error)
		}

		return resp.Output, nil
	}

	return "", fmt.Errorf("failed after %d attempts, last error: %w", r.retryCount, lastErr)
}

// RunWithAuth executes the remote agent with explicit auth token
func (r *RemoteAgentClient) RunWithAuth(ctx context.Context, input string, authToken string) (string, error) {
	if err := r.ensureConnected(); err != nil {
		return "", err
	}

	// Create request
	req := &pb.RunRequest{
		Input:   input,
		Context: make(map[string]string),
	}

	// Add org_id from context if available
	if orgID, _ := multitenancy.GetOrgID(ctx); orgID != "" {
		req.OrgId = orgID
	}

	// Add conversation_id from context if available
	if conversationID, ok := memory.GetConversationID(ctx); ok && conversationID != "" {
		req.ConversationId = conversationID
	}

	// Add explicit auth token to gRPC metadata
	if authToken != "" {
		md := metadata.Pairs("authorization", "Bearer "+authToken)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// Execute with retry logic
	var lastErr error
	for attempt := 0; attempt < r.retryCount; attempt++ {
		resp, err := r.client.Run(ctx, req)
		if err != nil {
			lastErr = err
			// Exponential backoff
			if attempt < r.retryCount-1 {
				backoff := time.Duration(attempt+1) * time.Second
				time.Sleep(backoff)
			}
			continue
		}

		if resp.Error != "" {
			return "", fmt.Errorf("remote agent error: %s", resp.Error)
		}

		return resp.Output, nil
	}

	return "", fmt.Errorf("failed after %d attempts, last error: %w", r.retryCount, lastErr)
}

// GetMetadata retrieves metadata from the remote agent
func (r *RemoteAgentClient) GetMetadata(ctx context.Context) (*pb.MetadataResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return r.client.GetMetadata(ctx, &pb.MetadataRequest{})
}

// GetCapabilities retrieves capabilities from the remote agent
func (r *RemoteAgentClient) GetCapabilities(ctx context.Context) (*pb.CapabilitiesResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return r.client.GetCapabilities(ctx, &pb.CapabilitiesRequest{})
}

// Health checks the health of the remote agent service
func (r *RemoteAgentClient) Health(ctx context.Context) (*pb.HealthResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.client.Health(ctx, &pb.HealthRequest{})
}

// Ready checks if the remote agent service is ready
func (r *RemoteAgentClient) Ready(ctx context.Context) (*pb.ReadinessResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.client.Ready(ctx, &pb.ReadinessRequest{})
}

// GenerateExecutionPlan generates an execution plan via the remote agent
func (r *RemoteAgentClient) GenerateExecutionPlan(ctx context.Context, input string) (*pb.PlanResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	req := &pb.PlanRequest{
		Input:   input,
		Context: make(map[string]string),
	}

	// Add org_id from context if available
	if orgID, _ := multitenancy.GetOrgID(ctx); orgID != "" {
		req.OrgId = orgID
	}

	// Add conversation_id from context if available
	if conversationID, ok := memory.GetConversationID(ctx); ok && conversationID != "" {
		req.ConversationId = conversationID
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	return r.client.GenerateExecutionPlan(ctx, req)
}

// ApproveExecutionPlan approves an execution plan via the remote agent
func (r *RemoteAgentClient) ApproveExecutionPlan(ctx context.Context, planID string, approved bool, modifications string) (*pb.ApprovalResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	req := &pb.ApprovalRequest{
		PlanId:        planID,
		Approved:      approved,
		Modifications: modifications,
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	return r.client.ApproveExecutionPlan(ctx, req)
}

// ensureConnected ensures that the client is connected to the remote service
func (r *RemoteAgentClient) ensureConnected() error {
	if r.conn == nil || r.client == nil {
		return r.Connect()
	}
	return nil
}

// IsConnected returns true if the client is connected
func (r *RemoteAgentClient) IsConnected() bool {
	return r.conn != nil && r.client != nil
}

// GetURL returns the URL of the remote agent
func (r *RemoteAgentClient) GetURL() string {
	return r.url
}

// SetTimeout sets the timeout for requests
func (r *RemoteAgentClient) SetTimeout(timeout time.Duration) {
	r.timeout = timeout
}

// SetRetryCount sets the number of retries for failed requests
func (r *RemoteAgentClient) SetRetryCount(count int) {
	r.retryCount = count
}

// RunStream executes the remote agent with streaming response
func (r *RemoteAgentClient) RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	// Create request
	req := &pb.RunRequest{
		Input:   input,
		Context: make(map[string]string),
	}

	// Add org_id from context if available
	if orgID, _ := multitenancy.GetOrgID(ctx); orgID != "" {
		req.OrgId = orgID
	}

	// Add conversation_id from context if available
	if conversationID, ok := memory.GetConversationID(ctx); ok && conversationID != "" {
		req.ConversationId = conversationID
	}

	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, r.timeout)

	// Execute streaming call
	stream, err := r.client.RunStream(ctx, req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start stream: %w", err)
	}

	// Create event channel
	eventChan := make(chan interfaces.AgentStreamEvent, 100)

	// Start goroutine to handle streaming response
	go func() {
		defer cancel()
		defer close(eventChan)

		for {
			resp, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					// Stream completed normally
					eventChan <- interfaces.AgentStreamEvent{
						Type:      interfaces.AgentEventComplete,
						Timestamp: time.Now(),
					}
					return
				}
				// Stream error
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventError,
					Error:     fmt.Errorf("stream error: %w", err),
					Timestamp: time.Now(),
				}
				return
			}

			if resp.Error != "" {
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventError,
					Error:     fmt.Errorf("remote agent error: %s", resp.Error),
					Timestamp: time.Now(),
				}
				return
			}

			// Convert gRPC response to AgentStreamEvent
			event := convertPbToStreamEvent(resp)
			eventChan <- event
		}
	}()

	return eventChan, nil
}

// RunStreamWithAuth executes the remote agent with streaming response and explicit auth token
func (r *RemoteAgentClient) RunStreamWithAuth(ctx context.Context, input string, authToken string) (<-chan interfaces.AgentStreamEvent, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	// Create request
	req := &pb.RunRequest{
		Input:   input,
		Context: make(map[string]string),
	}

	// Add org_id from context if available
	if orgID, _ := multitenancy.GetOrgID(ctx); orgID != "" {
		req.OrgId = orgID
	}

	// Add conversation_id from context if available
	if conversationID, ok := memory.GetConversationID(ctx); ok && conversationID != "" {
		req.ConversationId = conversationID
	}

	// Add explicit auth token to gRPC metadata
	if authToken != "" {
		md := metadata.Pairs("authorization", "Bearer "+authToken)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, r.timeout)

	// Execute streaming call
	stream, err := r.client.RunStream(ctx, req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start stream: %w", err)
	}

	// Create event channel
	eventChan := make(chan interfaces.AgentStreamEvent, 100)

	// Start goroutine to handle streaming response
	go func() {
		defer cancel()
		defer close(eventChan)

		for {
			resp, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					// Stream completed normally
					eventChan <- interfaces.AgentStreamEvent{
						Type:      interfaces.AgentEventComplete,
						Timestamp: time.Now(),
					}
					return
				}
				// Stream error
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventError,
					Error:     fmt.Errorf("stream error: %w", err),
					Timestamp: time.Now(),
				}
				return
			}

			if resp.Error != "" {
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventError,
					Error:     fmt.Errorf("remote agent error: %s", resp.Error),
					Timestamp: time.Now(),
				}
				return
			}

			// Convert gRPC response to AgentStreamEvent
			event := convertPbToStreamEvent(resp)
			eventChan <- event
		}
	}()

	return eventChan, nil
}

// OnThinking registers a handler for thinking events
func (r *RemoteAgentClient) OnThinking(handler func(string)) *RemoteAgentClient {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()
	r.thinkingHandlers = append(r.thinkingHandlers, handler)
	return r
}

// OnContent registers a handler for content events
func (r *RemoteAgentClient) OnContent(handler func(string)) *RemoteAgentClient {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()
	r.contentHandlers = append(r.contentHandlers, handler)
	return r
}

// OnToolCall registers a handler for tool call events
func (r *RemoteAgentClient) OnToolCall(handler func(*interfaces.ToolCallEvent)) *RemoteAgentClient {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()
	r.toolCallHandlers = append(r.toolCallHandlers, handler)
	return r
}

// OnToolResult registers a handler for tool result events
func (r *RemoteAgentClient) OnToolResult(handler func(*interfaces.ToolCallEvent)) *RemoteAgentClient {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()
	r.toolResultHandlers = append(r.toolResultHandlers, handler)
	return r
}

// OnError registers a handler for error events
func (r *RemoteAgentClient) OnError(handler func(error)) *RemoteAgentClient {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()
	r.errorHandlers = append(r.errorHandlers, handler)
	return r
}

// OnComplete registers a handler for completion events
func (r *RemoteAgentClient) OnComplete(handler func()) *RemoteAgentClient {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()
	r.completeHandlers = append(r.completeHandlers, handler)
	return r
}

// Stream executes the remote agent with registered event handlers
func (r *RemoteAgentClient) Stream(ctx context.Context, input string) error {
	events, err := r.RunStream(ctx, input)
	if err != nil {
		return err
	}

	for event := range events {
		// Execute handlers synchronously to preserve event ordering
		r.executeHandlers(event)
	}

	return nil
}

// StreamWithAuth executes the remote agent with registered event handlers and explicit auth token
func (r *RemoteAgentClient) StreamWithAuth(ctx context.Context, input string, authToken string) error {
	events, err := r.RunStreamWithAuth(ctx, input, authToken)
	if err != nil {
		return err
	}

	for event := range events {
		// Execute handlers synchronously to preserve event ordering
		r.executeHandlers(event)
	}

	return nil
}

// executeHandlers executes the appropriate handlers for an event
func (r *RemoteAgentClient) executeHandlers(event interfaces.AgentStreamEvent) {
	r.handlersMu.RLock()
	defer r.handlersMu.RUnlock()

	switch event.Type {
	case interfaces.AgentEventThinking:
		for _, handler := range r.thinkingHandlers {
			handler(event.ThinkingStep)
		}

	case interfaces.AgentEventContent:
		for _, handler := range r.contentHandlers {
			handler(event.Content)
		}

	case interfaces.AgentEventToolCall:
		for _, handler := range r.toolCallHandlers {
			handler(event.ToolCall)
		}

	case interfaces.AgentEventToolResult:
		for _, handler := range r.toolResultHandlers {
			handler(event.ToolCall)
		}

	case interfaces.AgentEventError:
		for _, handler := range r.errorHandlers {
			handler(event.Error)
		}

	case interfaces.AgentEventComplete:
		for _, handler := range r.completeHandlers {
			handler()
		}
	}
}

// convertPbToStreamEvent converts a protobuf RunStreamResponse to AgentStreamEvent
func convertPbToStreamEvent(resp *pb.RunStreamResponse) interfaces.AgentStreamEvent {
	event := interfaces.AgentStreamEvent{
		Content:   resp.Chunk,
		Timestamp: time.Unix(resp.Timestamp, 0),
		Metadata:  make(map[string]interface{}),
	}

	// Copy metadata
	for k, v := range resp.Metadata {
		event.Metadata[k] = v
	}

	// Convert event type
	switch resp.EventType {
	case pb.EventType_EVENT_TYPE_CONTENT:
		event.Type = interfaces.AgentEventContent
	case pb.EventType_EVENT_TYPE_THINKING:
		event.Type = interfaces.AgentEventThinking
		event.ThinkingStep = resp.Thinking
	case pb.EventType_EVENT_TYPE_TOOL_CALL:
		event.Type = interfaces.AgentEventToolCall
		if resp.ToolCall != nil {
			event.ToolCall = &interfaces.ToolCallEvent{
				ID:        resp.ToolCall.Id,
				Name:      resp.ToolCall.Name,
				Arguments: resp.ToolCall.Arguments,
				Result:    resp.ToolCall.Result,
				Status:    resp.ToolCall.Status,
			}
		}
	case pb.EventType_EVENT_TYPE_TOOL_RESULT:
		event.Type = interfaces.AgentEventToolResult
		if resp.ToolCall != nil {
			event.ToolCall = &interfaces.ToolCallEvent{
				ID:        resp.ToolCall.Id,
				Name:      resp.ToolCall.Name,
				Arguments: resp.ToolCall.Arguments,
				Result:    resp.ToolCall.Result,
				Status:    resp.ToolCall.Status,
			}
		}
	case pb.EventType_EVENT_TYPE_ERROR:
		event.Type = interfaces.AgentEventError
	case pb.EventType_EVENT_TYPE_COMPLETE:
		event.Type = interfaces.AgentEventComplete
	default:
		event.Type = interfaces.AgentEventContent
	}

	// Handle timestamp (use current time if not provided)
	if resp.Timestamp == 0 {
		event.Timestamp = time.Now()
	}

	return event
}
