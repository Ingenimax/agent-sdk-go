package microservice

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/pb"
	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/server"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// AgentMicroservice represents a microservice wrapping an agent
type AgentMicroservice struct {
	agent      *agent.Agent
	server     *server.AgentServer
	port       int
	running    bool
	serving    bool // New field to track if gRPC server is actually serving
	mu         sync.RWMutex
	cancelFunc context.CancelFunc
	servingCh  chan struct{} // Channel to signal when server starts serving
	
	// Event handlers
	thinkingHandlers []func(string)
	contentHandlers  []func(string)
	toolCallHandlers []func(*interfaces.ToolCallEvent)
	toolResultHandlers []func(*interfaces.ToolCallEvent)
	errorHandlers    []func(error)
	completeHandlers []func()
	handlersMu       sync.RWMutex
}

// Config holds configuration for creating an agent microservice
type Config struct {
	Port    int           // Port to run the service on (0 for auto-assign)
	Timeout time.Duration // Request timeout
}

// CreateMicroservice creates a new agent microservice
func CreateMicroservice(agent *agent.Agent, config Config) (*AgentMicroservice, error) {
	if agent == nil {
		return nil, fmt.Errorf("agent cannot be nil")
	}

	if agent.IsRemote() {
		return nil, fmt.Errorf("cannot create microservice from remote agent")
	}

	if config.Port < 0 || config.Port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", config.Port)
	}

	server := server.NewAgentServer(agent)

	return &AgentMicroservice{
		agent:     agent,
		server:    server,
		port:      config.Port,
		servingCh: make(chan struct{}),
	}, nil
}

// Start starts the microservice
func (m *AgentMicroservice) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("microservice is already running")
	}

	// Create a listener first to get the actual port
	addr := fmt.Sprintf(":%d", m.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", m.port, err)
	}
	
	// Update port if it was auto-assigned (port 0)
	if m.port == 0 {
		m.port = listener.Addr().(*net.TCPAddr).Port
	}

	// Create a context for the server
	_, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	// Mark as running now that we have successfully bound to the port
	m.running = true
	
	// Start the server in a goroutine
	go func() {
		defer func() {
			m.mu.Lock()
			m.running = false
			m.serving = false
			m.mu.Unlock()
		}()
		
		// Signal that we're about to start serving
		m.mu.Lock()
		m.serving = true
		close(m.servingCh) // Signal that server is starting to serve
		m.mu.Unlock()
		
		err := m.server.StartWithListener(listener)
		if err != nil {
			fmt.Printf("Agent server error: %v\n", err)
		}
	}()

	fmt.Printf("Agent microservice '%s' started on port %d\n", m.agent.GetName(), m.port)
	return nil
}

// Stop stops the microservice
func (m *AgentMicroservice) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil // Already stopped
	}

	// Stop the gRPC server
	m.server.Stop()

	// Cancel the context
	if m.cancelFunc != nil {
		m.cancelFunc()
	}

	m.running = false
	fmt.Printf("Agent microservice '%s' stopped\n", m.agent.GetName())
	return nil
}

// IsRunning returns true if the microservice is currently running
func (m *AgentMicroservice) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// GetPort returns the port the microservice is running on
func (m *AgentMicroservice) GetPort() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.port
}

// GetURL returns the URL of the microservice
func (m *AgentMicroservice) GetURL() string {
	return fmt.Sprintf("localhost:%d", m.GetPort())
}

// GetAgent returns the underlying agent
func (m *AgentMicroservice) GetAgent() *agent.Agent {
	return m.agent
}

// WaitForReady waits for the microservice to be ready to serve requests
func (m *AgentMicroservice) WaitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	
	// First, wait for the service to be marked as running
	for time.Now().Before(deadline) {
		if m.IsRunning() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	
	// If still not running after timeout, return error
	if !m.IsRunning() {
		return fmt.Errorf("microservice failed to start within %v", timeout)
	}
	
	// Wait for the server to start serving with timeout
	servingTimeout := time.Until(deadline)
	select {
	case <-m.servingCh:
		fmt.Printf("Debug: Server started serving on port %d\n", m.port)
		// Server has started serving, give it a moment to initialize
		time.Sleep(100 * time.Millisecond)
	case <-time.After(servingTimeout):
		return fmt.Errorf("microservice failed to start serving within %v", timeout)
	}
	
	// Now test gRPC health endpoint
	for time.Now().Before(deadline) {
		if err := m.testGRPCHealth(); err == nil {
			return nil // gRPC health check passed
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("microservice not ready after %v", timeout)
}

// RunStream executes the agent with streaming response
func (m *AgentMicroservice) RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
	if !m.IsRunning() {
		return nil, fmt.Errorf("microservice is not running")
	}

	// Create gRPC client
	// Note: Using grpc.Dial instead of grpc.NewClient because we need immediate connection
	// establishment for streaming. grpc.NewClient doesn't guarantee connection readiness.
	conn, err := grpc.Dial(
		fmt.Sprintf("localhost:%d", m.port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to microservice: %w", err)
	}

	client := pb.NewAgentServiceClient(conn)
	stream, err := client.RunStream(ctx, &pb.RunRequest{
		Input: input,
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to start stream: %w", err)
	}

	// Create output channel
	outputCh := make(chan interfaces.AgentStreamEvent, 100)

	// Start goroutine to process stream
	go func() {
		defer func() {
			conn.Close()
			close(outputCh)
		}()

		for {
			response, err := stream.Recv()
			if err != nil {
				if err.Error() == "EOF" {
					return
				}
				// Send error event
				select {
				case outputCh <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventError,
					Error:     err,
					Timestamp: time.Now(),
				}:
				case <-ctx.Done():
				}
				return
			}

			// Convert gRPC response to AgentStreamEvent
			event := convertGRPCResponseToAgentEvent(response)
			
			// Send event to channel
			select {
			case outputCh <- event:
			case <-ctx.Done():
				return
			}

			// Check if final
			if response.IsFinal || response.EventType == pb.EventType_EVENT_TYPE_COMPLETE {
				return
			}
		}
	}()

	return outputCh, nil
}

// OnThinking registers a handler for thinking events
func (m *AgentMicroservice) OnThinking(handler func(string)) *AgentMicroservice {
	m.handlersMu.Lock()
	defer m.handlersMu.Unlock()
	m.thinkingHandlers = append(m.thinkingHandlers, handler)
	return m
}

// OnContent registers a handler for content events
func (m *AgentMicroservice) OnContent(handler func(string)) *AgentMicroservice {
	m.handlersMu.Lock()
	defer m.handlersMu.Unlock()
	m.contentHandlers = append(m.contentHandlers, handler)
	return m
}

// OnToolCall registers a handler for tool call events
func (m *AgentMicroservice) OnToolCall(handler func(*interfaces.ToolCallEvent)) *AgentMicroservice {
	m.handlersMu.Lock()
	defer m.handlersMu.Unlock()
	m.toolCallHandlers = append(m.toolCallHandlers, handler)
	return m
}

// OnToolResult registers a handler for tool result events
func (m *AgentMicroservice) OnToolResult(handler func(*interfaces.ToolCallEvent)) *AgentMicroservice {
	m.handlersMu.Lock()
	defer m.handlersMu.Unlock()
	m.toolResultHandlers = append(m.toolResultHandlers, handler)
	return m
}

// OnError registers a handler for error events
func (m *AgentMicroservice) OnError(handler func(error)) *AgentMicroservice {
	m.handlersMu.Lock()
	defer m.handlersMu.Unlock()
	m.errorHandlers = append(m.errorHandlers, handler)
	return m
}

// OnComplete registers a handler for completion events
func (m *AgentMicroservice) OnComplete(handler func()) *AgentMicroservice {
	m.handlersMu.Lock()
	defer m.handlersMu.Unlock()
	m.completeHandlers = append(m.completeHandlers, handler)
	return m
}

// Stream executes the agent with registered event handlers
func (m *AgentMicroservice) Stream(ctx context.Context, input string) error {
	events, err := m.RunStream(ctx, input)
	if err != nil {
		return err
	}

	for event := range events {
		m.executeHandlers(event)
	}

	return nil
}

// executeHandlers executes the appropriate handlers for an event
func (m *AgentMicroservice) executeHandlers(event interfaces.AgentStreamEvent) {
	m.handlersMu.RLock()
	defer m.handlersMu.RUnlock()

	switch event.Type {
	case interfaces.AgentEventThinking:
		for _, handler := range m.thinkingHandlers {
			handler(event.ThinkingStep)
		}

	case interfaces.AgentEventContent:
		for _, handler := range m.contentHandlers {
			handler(event.Content)
		}

	case interfaces.AgentEventToolCall:
		for _, handler := range m.toolCallHandlers {
			handler(event.ToolCall)
		}

	case interfaces.AgentEventToolResult:
		for _, handler := range m.toolResultHandlers {
			handler(event.ToolCall)
		}

	case interfaces.AgentEventError:
		for _, handler := range m.errorHandlers {
			handler(event.Error)
		}

	case interfaces.AgentEventComplete:
		for _, handler := range m.completeHandlers {
			handler()
		}
	}
}

// convertGRPCResponseToAgentEvent converts gRPC stream response to AgentStreamEvent
func convertGRPCResponseToAgentEvent(response *pb.RunStreamResponse) interfaces.AgentStreamEvent {
	event := interfaces.AgentStreamEvent{
		Timestamp: time.Now(),
	}

	// Set timestamp from response if available
	if response.Timestamp > 0 {
		event.Timestamp = time.UnixMilli(response.Timestamp)
	}

	// Convert metadata
	if response.Metadata != nil {
		event.Metadata = make(map[string]interface{})
		for k, v := range response.Metadata {
			event.Metadata[k] = v
		}
	}

	// Convert based on event type
	switch response.EventType {
	case pb.EventType_EVENT_TYPE_THINKING:
		event.Type = interfaces.AgentEventThinking
		event.ThinkingStep = response.Thinking

	case pb.EventType_EVENT_TYPE_CONTENT:
		event.Type = interfaces.AgentEventContent
		event.Content = response.Chunk

	case pb.EventType_EVENT_TYPE_TOOL_CALL, pb.EventType_EVENT_TYPE_TOOL_RESULT:
		if response.EventType == pb.EventType_EVENT_TYPE_TOOL_CALL {
			event.Type = interfaces.AgentEventToolCall
		} else {
			event.Type = interfaces.AgentEventToolResult
		}
		
		if response.ToolCall != nil {
			event.ToolCall = &interfaces.ToolCallEvent{
				ID:        response.ToolCall.Id,
				Name:      response.ToolCall.Name,
				Arguments: response.ToolCall.Arguments,
				Result:    response.ToolCall.Result,
				Status:    response.ToolCall.Status,
			}
		}

	case pb.EventType_EVENT_TYPE_ERROR:
		event.Type = interfaces.AgentEventError
		if response.Error != "" {
			event.Error = fmt.Errorf("%s", response.Error)
		}

	case pb.EventType_EVENT_TYPE_COMPLETE:
		event.Type = interfaces.AgentEventComplete

	default:
		// Default to content for unknown event types
		event.Type = interfaces.AgentEventContent
		event.Content = response.Chunk
	}

	return event
}

// testGRPCHealth tests the gRPC health endpoint
func (m *AgentMicroservice) testGRPCHealth() error {
	// Create a gRPC connection with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	// Note: Using grpc.Dial instead of grpc.NewClient because we need immediate connection
	// establishment for health checks. grpc.NewClient doesn't guarantee connection readiness.
	conn, err := grpc.Dial(
		fmt.Sprintf("localhost:%d", m.port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // Wait for connection to be established
	)
	if err != nil {
		fmt.Printf("Debug: Failed to create gRPC connection to localhost:%d: %v\n", m.port, err)
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			// Log the close error but don't fail the whole operation
			fmt.Printf("Warning: failed to close gRPC connection: %v\n", closeErr)
		}
	}()
	
	// Test the standard gRPC health service
	healthClient := grpc_health_v1.NewHealthClient(conn)
	resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: "", // Check overall server health
	})
	
	if err != nil {
		fmt.Printf("Debug: Health check failed for localhost:%d: %v\n", m.port, err)
		return err
	}
	
	fmt.Printf("Debug: Health check succeeded for localhost:%d, status: %v\n", m.port, resp.Status)
	return nil
}

// MicroserviceManager manages multiple agent microservices
type MicroserviceManager struct {
	services map[string]*AgentMicroservice
	mu       sync.RWMutex
}

// NewMicroserviceManager creates a new microservice manager
func NewMicroserviceManager() *MicroserviceManager {
	return &MicroserviceManager{
		services: make(map[string]*AgentMicroservice),
	}
}

// RegisterService registers a microservice with the manager
func (mm *MicroserviceManager) RegisterService(name string, service *AgentMicroservice) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, exists := mm.services[name]; exists {
		return fmt.Errorf("service with name %s already exists", name)
	}

	mm.services[name] = service
	return nil
}

// StartService starts a service by name
func (mm *MicroserviceManager) StartService(name string) error {
	mm.mu.RLock()
	service, exists := mm.services[name]
	mm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service %s not found", name)
	}

	return service.Start()
}

// StopService stops a service by name
func (mm *MicroserviceManager) StopService(name string) error {
	mm.mu.RLock()
	service, exists := mm.services[name]
	mm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service %s not found", name)
	}

	return service.Stop()
}

// StartAll starts all registered services
func (mm *MicroserviceManager) StartAll() error {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	for name, service := range mm.services {
		if err := service.Start(); err != nil {
			return fmt.Errorf("failed to start service %s: %w", name, err)
		}
	}

	return nil
}

// StopAll stops all running services
func (mm *MicroserviceManager) StopAll() error {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	var lastErr error
	for name, service := range mm.services {
		if err := service.Stop(); err != nil {
			lastErr = fmt.Errorf("failed to stop service %s: %w", name, err)
		}
	}

	return lastErr
}

// GetService returns a service by name
func (mm *MicroserviceManager) GetService(name string) (*AgentMicroservice, bool) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	service, exists := mm.services[name]
	return service, exists
}

// ListServices returns all registered service names
func (mm *MicroserviceManager) ListServices() []string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	names := make([]string, 0, len(mm.services))
	for name := range mm.services {
		names = append(names, name)
	}

	return names
}