package client

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/pb"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// mockStreamServer implements the streaming server for testing
type mockStreamServer struct {
	responses []*pb.RunStreamResponse
	index     int
}

func (m *mockStreamServer) Recv() (*pb.RunStreamResponse, error) {
	if m.index >= len(m.responses) {
		return nil, io.EOF
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func (m *mockStreamServer) Header() (metadata.MD, error) {
	return metadata.MD{}, nil
}

func (m *mockStreamServer) Trailer() metadata.MD {
	return metadata.MD{}
}

func (m *mockStreamServer) CloseSend() error {
	return nil
}

func (m *mockStreamServer) Context() context.Context {
	return context.Background()
}

func (m *mockStreamServer) SendMsg(m2 any) error {
	return nil
}

func (m *mockStreamServer) RecvMsg(m2 any) error {
	return nil
}

// mockAgentServiceClient implements a mock gRPC client for testing
type mockAgentServiceClient struct {
	pb.AgentServiceClient
	streamResponses []*pb.RunStreamResponse
	streamError     error
	authToken       string
}

func (m *mockAgentServiceClient) RunStream(ctx context.Context, req *pb.RunRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.RunStreamResponse], error) {
	if m.streamError != nil {
		return nil, m.streamError
	}

	// Check for auth token in metadata
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if auth := md.Get("authorization"); len(auth) > 0 {
			m.authToken = auth[0]
		}
	}

	return &mockStreamServer{
		responses: m.streamResponses,
		index:     0,
	}, nil
}

func TestRemoteAgentClient_RunStream(t *testing.T) {
	// Create mock responses
	mockResponses := []*pb.RunStreamResponse{
		{
			EventType: pb.EventType_EVENT_TYPE_CONTENT,
			Chunk:     "Hello",
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: pb.EventType_EVENT_TYPE_CONTENT,
			Chunk:     " World",
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: pb.EventType_EVENT_TYPE_COMPLETE,
			IsFinal:   true,
			Timestamp: time.Now().Unix(),
		},
	}

	// Create mock client
	mockClient := &mockAgentServiceClient{
		streamResponses: mockResponses,
	}

	// Create remote agent client
	client := &RemoteAgentClient{
		client:     mockClient,
		conn:       &grpc.ClientConn{}, // Mock connection
		timeout:    5 * time.Second,
		retryCount: 3,
	}

	// Test RunStream
	ctx := context.Background()
	eventChan, err := client.RunStream(ctx, "test input")
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}

	// Collect events
	var events []interfaces.AgentStreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Verify events - should have 2 content events + 1 complete from mock + 1 auto-complete from stream end
	if len(events) != 4 {
		t.Fatalf("Expected 4 events, got %d", len(events))
	}

	// Check first content event
	if events[0].Type != interfaces.AgentEventContent {
		t.Errorf("Expected first event type to be content, got %s", events[0].Type)
	}
	if events[0].Content != "Hello" {
		t.Errorf("Expected first event content to be 'Hello', got '%s'", events[0].Content)
	}

	// Check second content event
	if events[1].Type != interfaces.AgentEventContent {
		t.Errorf("Expected second event type to be content, got %s", events[1].Type)
	}
	if events[1].Content != " World" {
		t.Errorf("Expected second event content to be ' World', got '%s'", events[1].Content)
	}

	// Check complete events (both from mock and auto-generated)
	if events[2].Type != interfaces.AgentEventComplete {
		t.Errorf("Expected third event type to be complete, got %s", events[2].Type)
	}
	if events[3].Type != interfaces.AgentEventComplete {
		t.Errorf("Expected fourth event type to be complete, got %s", events[3].Type)
	}
}

func TestRemoteAgentClient_RunStreamWithAuth(t *testing.T) {
	// Create mock responses
	mockResponses := []*pb.RunStreamResponse{
		{
			EventType: pb.EventType_EVENT_TYPE_THINKING,
			Thinking:  "Let me think about this...",
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: pb.EventType_EVENT_TYPE_CONTENT,
			Chunk:     "Authenticated response",
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: pb.EventType_EVENT_TYPE_COMPLETE,
			IsFinal:   true,
			Timestamp: time.Now().Unix(),
		},
	}

	// Create mock client
	mockClient := &mockAgentServiceClient{
		streamResponses: mockResponses,
	}

	// Create remote agent client
	client := &RemoteAgentClient{
		client:     mockClient,
		conn:       &grpc.ClientConn{}, // Mock connection
		timeout:    5 * time.Second,
		retryCount: 3,
	}

	// Test RunStreamWithAuth
	ctx := context.Background()
	authToken := "test-token-123"
	eventChan, err := client.RunStreamWithAuth(ctx, "authenticated test input", authToken)
	if err != nil {
		t.Fatalf("RunStreamWithAuth failed: %v", err)
	}

	// Verify auth token was set
	expectedAuth := "Bearer " + authToken
	if mockClient.authToken != expectedAuth {
		t.Errorf("Expected auth token '%s', got '%s'", expectedAuth, mockClient.authToken)
	}

	// Collect events
	var events []interfaces.AgentStreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Verify events - should have thinking + content + complete from mock + auto-complete from stream end
	if len(events) != 4 {
		t.Fatalf("Expected 4 events, got %d", len(events))
	}

	// Check thinking event
	if events[0].Type != interfaces.AgentEventThinking {
		t.Errorf("Expected first event type to be thinking, got %s", events[0].Type)
	}
	if events[0].ThinkingStep != "Let me think about this..." {
		t.Errorf("Expected thinking step, got '%s'", events[0].ThinkingStep)
	}

	// Check content event
	if events[1].Type != interfaces.AgentEventContent {
		t.Errorf("Expected second event type to be content, got %s", events[1].Type)
	}
	if events[1].Content != "Authenticated response" {
		t.Errorf("Expected content 'Authenticated response', got '%s'", events[1].Content)
	}

	// Check complete events (both from mock and auto-generated)
	if events[2].Type != interfaces.AgentEventComplete {
		t.Errorf("Expected third event type to be complete, got %s", events[2].Type)
	}
	if events[3].Type != interfaces.AgentEventComplete {
		t.Errorf("Expected fourth event type to be complete, got %s", events[3].Type)
	}
}

func TestRemoteAgentClient_RunStreamWithAuth_ErrorHandling(t *testing.T) {
	// Test with stream error
	mockClient := &mockAgentServiceClient{
		streamError: fmt.Errorf("connection failed"),
	}

	client := &RemoteAgentClient{
		client:     mockClient,
		conn:       &grpc.ClientConn{},
		timeout:    5 * time.Second,
		retryCount: 3,
	}

	ctx := context.Background()
	_, err := client.RunStreamWithAuth(ctx, "test input", "token")
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Test with error response
	mockClient = &mockAgentServiceClient{
		streamResponses: []*pb.RunStreamResponse{
			{
				EventType: pb.EventType_EVENT_TYPE_ERROR,
				Error:     "Remote agent error occurred",
				Timestamp: time.Now().Unix(),
			},
		},
	}

	client.client = mockClient
	eventChan, err := client.RunStreamWithAuth(ctx, "test input", "token")
	if err != nil {
		t.Fatalf("RunStreamWithAuth failed: %v", err)
	}

	// Should receive error event
	var events []interfaces.AgentStreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 error event, got %d", len(events))
	}

	if events[0].Type != interfaces.AgentEventError {
		t.Errorf("Expected error event, got %s", events[0].Type)
	}
	if events[0].Error == nil {
		t.Error("Expected error to be set")
	}
}

func TestRemoteAgentClient_RunStreamWithAuth_ToolCallEvent(t *testing.T) {
	// Create mock responses with tool call
	mockResponses := []*pb.RunStreamResponse{
		{
			EventType: pb.EventType_EVENT_TYPE_TOOL_CALL,
			ToolCall: &pb.ToolCall{
				Id:        "tool-123",
				Name:      "calculator",
				Arguments: `{"operation": "add", "a": 2, "b": 3}`,
				Status:    "executing",
			},
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: pb.EventType_EVENT_TYPE_TOOL_RESULT,
			ToolCall: &pb.ToolCall{
				Id:     "tool-123",
				Name:   "calculator",
				Result: "5",
				Status: "completed",
			},
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: pb.EventType_EVENT_TYPE_COMPLETE,
			IsFinal:   true,
			Timestamp: time.Now().Unix(),
		},
	}

	// Create mock client
	mockClient := &mockAgentServiceClient{
		streamResponses: mockResponses,
	}

	// Create remote agent client
	client := &RemoteAgentClient{
		client:     mockClient,
		conn:       &grpc.ClientConn{},
		timeout:    5 * time.Second,
		retryCount: 3,
	}

	// Test RunStreamWithAuth
	ctx := context.Background()
	eventChan, err := client.RunStreamWithAuth(ctx, "test input", "token")
	if err != nil {
		t.Fatalf("RunStreamWithAuth failed: %v", err)
	}

	// Collect events
	var events []interfaces.AgentStreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Verify events - should have tool_call + tool_result + complete from mock + auto-complete from stream end
	if len(events) != 4 {
		t.Fatalf("Expected 4 events, got %d", len(events))
	}

	// Check tool call event
	if events[0].Type != interfaces.AgentEventToolCall {
		t.Errorf("Expected first event type to be tool_call, got %s", events[0].Type)
	}
	if events[0].ToolCall == nil {
		t.Fatal("Expected tool call to be set")
	}
	if events[0].ToolCall.ID != "tool-123" {
		t.Errorf("Expected tool call ID 'tool-123', got '%s'", events[0].ToolCall.ID)
	}
	if events[0].ToolCall.Name != "calculator" {
		t.Errorf("Expected tool call name 'calculator', got '%s'", events[0].ToolCall.Name)
	}

	// Check tool result event
	if events[1].Type != interfaces.AgentEventToolResult {
		t.Errorf("Expected second event type to be tool_result, got %s", events[1].Type)
	}
	if events[1].ToolCall == nil {
		t.Fatal("Expected tool call to be set")
	}
	if events[1].ToolCall.Result != "5" {
		t.Errorf("Expected tool result '5', got '%s'", events[1].ToolCall.Result)
	}

	// Check complete events (both from mock and auto-generated)
	if events[2].Type != interfaces.AgentEventComplete {
		t.Errorf("Expected third event type to be complete, got %s", events[2].Type)
	}
	if events[3].Type != interfaces.AgentEventComplete {
		t.Errorf("Expected fourth event type to be complete, got %s", events[3].Type)
	}
}

func TestConvertPbToStreamEvent(t *testing.T) {
	timestamp := time.Now().Unix()

	tests := []struct {
		name     string
		input    *pb.RunStreamResponse
		expected interfaces.AgentStreamEvent
	}{
		{
			name: "content event",
			input: &pb.RunStreamResponse{
				EventType: pb.EventType_EVENT_TYPE_CONTENT,
				Chunk:     "test content",
				Timestamp: timestamp,
				Metadata:  map[string]string{"key": "value"},
			},
			expected: interfaces.AgentStreamEvent{
				Type:      interfaces.AgentEventContent,
				Content:   "test content",
				Timestamp: time.Unix(timestamp, 0),
				Metadata:  map[string]any{"key": "value"},
			},
		},
		{
			name: "thinking event",
			input: &pb.RunStreamResponse{
				EventType: pb.EventType_EVENT_TYPE_THINKING,
				Thinking:  "thinking step",
				Timestamp: timestamp,
			},
			expected: interfaces.AgentStreamEvent{
				Type:         interfaces.AgentEventThinking,
				ThinkingStep: "thinking step",
				Timestamp:    time.Unix(timestamp, 0),
				Metadata:     map[string]any{},
			},
		},
		{
			name: "zero timestamp uses current time",
			input: &pb.RunStreamResponse{
				EventType: pb.EventType_EVENT_TYPE_CONTENT,
				Chunk:     "content",
				Timestamp: 0, // Zero timestamp
			},
			expected: interfaces.AgentStreamEvent{
				Type:     interfaces.AgentEventContent,
				Content:  "content",
				Metadata: map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertPbToStreamEvent(tt.input)

			if result.Type != tt.expected.Type {
				t.Errorf("Expected type %s, got %s", tt.expected.Type, result.Type)
			}
			if result.Content != tt.expected.Content {
				t.Errorf("Expected content '%s', got '%s'", tt.expected.Content, result.Content)
			}
			if result.ThinkingStep != tt.expected.ThinkingStep {
				t.Errorf("Expected thinking step '%s', got '%s'", tt.expected.ThinkingStep, result.ThinkingStep)
			}

			// Special handling for zero timestamp test
			if tt.input.Timestamp == 0 {
				if result.Timestamp.IsZero() {
					t.Error("Expected non-zero timestamp when input timestamp is 0")
				}
			} else {
				if !result.Timestamp.Equal(tt.expected.Timestamp) {
					t.Errorf("Expected timestamp %v, got %v", tt.expected.Timestamp, result.Timestamp)
				}
			}

			// Check metadata length
			if len(result.Metadata) != len(tt.expected.Metadata) {
				t.Errorf("Expected %d metadata items, got %d", len(tt.expected.Metadata), len(result.Metadata))
			}
		})
	}
}

func TestRemoteAgentClient_EventHandlers(t *testing.T) {
	// Create mock responses with different event types
	mockResponses := []*pb.RunStreamResponse{
		{
			EventType: pb.EventType_EVENT_TYPE_THINKING,
			Thinking:  "Let me analyze this problem...",
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: pb.EventType_EVENT_TYPE_CONTENT,
			Chunk:     "Here's my response",
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: pb.EventType_EVENT_TYPE_TOOL_CALL,
			ToolCall: &pb.ToolCall{
				Id:        "tool-456",
				Name:      "search",
				Arguments: `{"query": "test"}`,
				Status:    "executing",
			},
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: pb.EventType_EVENT_TYPE_COMPLETE,
			IsFinal:   true,
			Timestamp: time.Now().Unix(),
		},
	}

	// Create mock client
	mockClient := &mockAgentServiceClient{
		streamResponses: mockResponses,
	}

	// Create remote agent client
	client := &RemoteAgentClient{
		client:     mockClient,
		conn:       &grpc.ClientConn{},
		timeout:    5 * time.Second,
		retryCount: 3,
	}

	// Track handler calls with mutex for race safety
	var mu sync.Mutex
	var thinkingCalls []string
	var contentCalls []string
	var toolCallCalls []*interfaces.ToolCallEvent
	var completeCalls int

	// Set up event handlers
	client.
		OnThinking(func(thinking string) {
			mu.Lock()
			thinkingCalls = append(thinkingCalls, thinking)
			mu.Unlock()
		}).
		OnContent(func(content string) {
			mu.Lock()
			contentCalls = append(contentCalls, content)
			mu.Unlock()
		}).
		OnToolCall(func(toolCall *interfaces.ToolCallEvent) {
			mu.Lock()
			toolCallCalls = append(toolCallCalls, toolCall)
			mu.Unlock()
		}).
		OnComplete(func() {
			mu.Lock()
			completeCalls++
			mu.Unlock()
		})

	// Execute with handlers
	ctx := context.Background()
	err := client.Stream(ctx, "test input")
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	// Wait a bit for asynchronous handlers to execute
	time.Sleep(100 * time.Millisecond)

	// Verify handler calls (with mutex protection)
	mu.Lock()
	if len(thinkingCalls) != 1 {
		t.Errorf("Expected 1 thinking call, got %d", len(thinkingCalls))
	}
	if len(thinkingCalls) > 0 && thinkingCalls[0] != "Let me analyze this problem..." {
		t.Errorf("Expected thinking 'Let me analyze this problem...', got '%s'", thinkingCalls[0])
	}

	if len(contentCalls) != 1 {
		t.Errorf("Expected 1 content call, got %d", len(contentCalls))
	}
	if len(contentCalls) > 0 && contentCalls[0] != "Here's my response" {
		t.Errorf("Expected content 'Here's my response', got '%s'", contentCalls[0])
	}

	if len(toolCallCalls) != 1 {
		t.Errorf("Expected 1 tool call, got %d", len(toolCallCalls))
	}
	if len(toolCallCalls) > 0 && toolCallCalls[0].Name != "search" {
		t.Errorf("Expected tool call name 'search', got '%s'", toolCallCalls[0].Name)
	}

	// Should have 2 complete calls: 1 from mock response + 1 auto-generated
	if completeCalls != 2 {
		t.Errorf("Expected 2 complete calls, got %d", completeCalls)
	}
	mu.Unlock()
}

func TestRemoteAgentClient_StreamWithAuthHandlers(t *testing.T) {
	// Create mock responses
	mockResponses := []*pb.RunStreamResponse{
		{
			EventType: pb.EventType_EVENT_TYPE_CONTENT,
			Chunk:     "Authenticated response",
			Timestamp: time.Now().Unix(),
		},
		{
			EventType: pb.EventType_EVENT_TYPE_COMPLETE,
			IsFinal:   true,
			Timestamp: time.Now().Unix(),
		},
	}

	// Create mock client
	mockClient := &mockAgentServiceClient{
		streamResponses: mockResponses,
	}

	// Create remote agent client
	client := &RemoteAgentClient{
		client:     mockClient,
		conn:       &grpc.ClientConn{},
		timeout:    5 * time.Second,
		retryCount: 3,
	}

	// Track handler calls with mutex for race safety
	var mu sync.Mutex
	var contentReceived string
	var errorReceived error

	// Set up event handlers
	client.
		OnContent(func(content string) {
			mu.Lock()
			contentReceived = content
			mu.Unlock()
		}).
		OnError(func(err error) {
			mu.Lock()
			errorReceived = err
			mu.Unlock()
		})

	// Execute with handlers and auth
	ctx := context.Background()
	authToken := "test-auth-token"
	err := client.StreamWithAuth(ctx, "authenticated request", authToken)
	if err != nil {
		t.Fatalf("StreamWithAuth failed: %v", err)
	}

	// Wait a bit for asynchronous handlers to execute
	time.Sleep(100 * time.Millisecond)

	// Verify auth token was passed
	expectedAuth := "Bearer " + authToken
	if mockClient.authToken != expectedAuth {
		t.Errorf("Expected auth token '%s', got '%s'", expectedAuth, mockClient.authToken)
	}

	// Verify content handler was called (with mutex protection)
	mu.Lock()
	if contentReceived != "Authenticated response" {
		t.Errorf("Expected content 'Authenticated response', got '%s'", contentReceived)
	}

	// Verify no errors
	if errorReceived != nil {
		t.Errorf("Unexpected error: %v", errorReceived)
	}
	mu.Unlock()
}

func TestRemoteAgentClient_HandlerChaining(t *testing.T) {
	client := &RemoteAgentClient{}

	// Test that handler methods return the client for chaining
	result := client.
		OnThinking(func(string) {}).
		OnContent(func(string) {}).
		OnComplete(func() {})

	if result != client {
		t.Error("Handler methods should return the client for method chaining")
	}

	// Verify handlers were actually registered
	if len(client.thinkingHandlers) != 1 {
		t.Error("Thinking handler was not registered")
	}
	if len(client.contentHandlers) != 1 {
		t.Error("Content handler was not registered")
	}
	if len(client.completeHandlers) != 1 {
		t.Error("Complete handler was not registered")
	}
}

// mockPanicStreamServer simulates the panic scenario
type mockPanicStreamServer struct {
	panicOnRecv bool
	nilResponse bool
	callCount   int
}

func (m *mockPanicStreamServer) Recv() (*pb.RunStreamResponse, error) {
	m.callCount++
	
	if m.panicOnRecv {
		// Simulate the panic that occurs in the real scenario
		panic("runtime error: invalid memory address or nil pointer dereference")
	}
	
	if m.nilResponse {
		// Return nil response to test nil handling
		return nil, nil
	}
	
	// Return EOF after first call to end the stream
	if m.callCount > 1 {
		return nil, io.EOF
	}
	
	return &pb.RunStreamResponse{
		EventType: pb.EventType_EVENT_TYPE_CONTENT,
		Chunk:     "test content",
		Timestamp: time.Now().Unix(),
	}, nil
}

func (m *mockPanicStreamServer) Header() (metadata.MD, error) {
	return metadata.MD{}, nil
}

func (m *mockPanicStreamServer) Trailer() metadata.MD {
	return metadata.MD{}
}

func (m *mockPanicStreamServer) CloseSend() error {
	return nil
}

func (m *mockPanicStreamServer) Context() context.Context {
	return context.Background()
}

func (m *mockPanicStreamServer) SendMsg(m2 any) error {
	return nil
}

func (m *mockPanicStreamServer) RecvMsg(m2 any) error {
	return nil
}

// mockPanicAgentServiceClient implements a mock gRPC client that can simulate panics
type mockPanicAgentServiceClient struct {
	pb.AgentServiceClient
	panicOnRecv bool
	nilResponse bool
}

func (m *mockPanicAgentServiceClient) RunStream(ctx context.Context, req *pb.RunRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.RunStreamResponse], error) {
	return &mockPanicStreamServer{
		panicOnRecv: m.panicOnRecv,
		nilResponse: m.nilResponse,
	}, nil
}

// TestRemoteAgentClient_StreamPanicRecovery tests that the client recovers from panics during streaming
func TestRemoteAgentClient_StreamPanicRecovery(t *testing.T) {
	// Create mock client that will panic during Recv()
	mockClient := &mockPanicAgentServiceClient{
		panicOnRecv: true,
	}

	// Create remote agent client
	client := &RemoteAgentClient{
		client:     mockClient,
		conn:       &grpc.ClientConn{}, // Mock connection
		timeout:    5 * time.Second,
		retryCount: 3,
	}

	// Test RunStreamWithAuth with panic scenario
	ctx := context.Background()
	eventChan, err := client.RunStreamWithAuth(ctx, "test input", "token")
	if err != nil {
		t.Fatalf("RunStreamWithAuth failed: %v", err)
	}

	// Collect events - should receive panic recovery error
	var events []interfaces.AgentStreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Should receive exactly 1 error event from panic recovery
	if len(events) != 1 {
		t.Fatalf("Expected 1 error event from panic recovery, got %d events", len(events))
	}

	// Verify it's an error event
	if events[0].Type != interfaces.AgentEventError {
		t.Errorf("Expected error event type, got %s", events[0].Type)
	}

	// Verify the error message indicates panic recovery
	if events[0].Error == nil {
		t.Fatal("Expected error to be set")
	}
	
	errorMsg := events[0].Error.Error()
	if !contains(errorMsg, "stream panic recovered") {
		t.Errorf("Expected panic recovery error message, got: %s", errorMsg)
	}
}

// TestRemoteAgentClient_StreamNilResponseHandling tests handling of nil responses
func TestRemoteAgentClient_StreamNilResponseHandling(t *testing.T) {
	// Create mock client that returns nil responses
	mockClient := &mockPanicAgentServiceClient{
		nilResponse: true,
	}

	// Create remote agent client
	client := &RemoteAgentClient{
		client:     mockClient,
		conn:       &grpc.ClientConn{}, // Mock connection
		timeout:    5 * time.Second,
		retryCount: 3,
	}

	// Test RunStreamWithAuth with nil response scenario
	ctx := context.Background()
	eventChan, err := client.RunStreamWithAuth(ctx, "test input", "token")
	if err != nil {
		t.Fatalf("RunStreamWithAuth failed: %v", err)
	}

	// Collect events - should receive nil response error
	var events []interfaces.AgentStreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Should receive exactly 1 error event from nil response
	if len(events) != 1 {
		t.Fatalf("Expected 1 error event from nil response, got %d events", len(events))
	}

	// Verify it's an error event
	if events[0].Type != interfaces.AgentEventError {
		t.Errorf("Expected error event type, got %s", events[0].Type)
	}

	// Verify the error message indicates nil response
	if events[0].Error == nil {
		t.Fatal("Expected error to be set")
	}
	
	errorMsg := events[0].Error.Error()
	if !contains(errorMsg, "received nil response") {
		t.Errorf("Expected nil response error message, got: %s", errorMsg)
	}
}

// TestRemoteAgentClient_RunStreamPanicRecovery tests the RunStream method (without auth)
func TestRemoteAgentClient_RunStreamPanicRecovery(t *testing.T) {
	// Create mock client that will panic during Recv()
	mockClient := &mockPanicAgentServiceClient{
		panicOnRecv: true,
	}

	// Create remote agent client
	client := &RemoteAgentClient{
		client:     mockClient,
		conn:       &grpc.ClientConn{}, // Mock connection
		timeout:    5 * time.Second,
		retryCount: 3,
	}

	// Test RunStream with panic scenario
	ctx := context.Background()
	eventChan, err := client.RunStream(ctx, "test input")
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}

	// Collect events - should receive panic recovery error
	var events []interfaces.AgentStreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Should receive exactly 1 error event from panic recovery
	if len(events) != 1 {
		t.Fatalf("Expected 1 error event from panic recovery, got %d events", len(events))
	}

	// Verify it's an error event
	if events[0].Type != interfaces.AgentEventError {
		t.Errorf("Expected error event type, got %s", events[0].Type)
	}

	// Verify the error message indicates panic recovery
	if events[0].Error == nil {
		t.Fatal("Expected error to be set")
	}
	
	errorMsg := events[0].Error.Error()
	if !contains(errorMsg, "stream panic recovered") {
		t.Errorf("Expected panic recovery error message, got: %s", errorMsg)
	}
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(substr) <= len(s) && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()))
}