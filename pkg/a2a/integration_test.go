package a2a

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/a2a"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// startTestServer creates and starts an A2A server on a random port, returning the base URL.
func startTestServer(t *testing.T, agent AgentAdapter, opts ...ServerOption) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	baseURL := fmt.Sprintf("http://%s", listener.Addr().String())

	card := NewCardBuilder(
		agent.GetName(),
		agent.GetDescription(),
		baseURL+"/",
		WithStreaming(true),
	).Build()

	allOpts := append([]ServerOption{WithServerLogger(logging.New())}, opts...)
	srv := NewServer(agent, card, allOpts...)

	go func() {
		_ = http.Serve(listener, srv.Handler())
	}()

	t.Cleanup(func() { listener.Close() })

	return baseURL
}

func TestIntegration_ClientSendMessage(t *testing.T) {
	agent := &mockAgent{
		name:        "integration-agent",
		description: "Agent for integration testing",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "Hello from A2A!", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	baseURL := startTestServer(t, agent)
	ctx := context.Background()

	client, err := NewClient(ctx, baseURL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Verify card was resolved
	card := client.Card()
	if card.Name != "integration-agent" {
		t.Errorf("expected card name 'integration-agent', got %s", card.Name)
	}

	// Send a message
	result, err := client.SendMessage(ctx, "test message")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	text := ExtractResultText(result)
	if text == "" {
		t.Error("expected non-empty response text")
	}
}

func TestIntegration_ClientSendMessageWithContextID(t *testing.T) {
	agent := &mockAgent{
		name:        "context-agent",
		description: "Agent for context_id testing",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "response 1", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	baseURL := startTestServer(t, agent)
	ctx := context.Background()

	client, err := NewClient(ctx, baseURL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	result, err := client.SendMessage(ctx, "hello", WithContextID("conversation-123"))
	if err != nil {
		t.Fatalf("SendMessage with context ID failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	text := ExtractResultText(result)
	if text == "" {
		t.Error("expected non-empty response text")
	}
}

func TestIntegration_ClientSendMessageWithTaskID(t *testing.T) {
	agent := &mockAgent{
		name:        "taskid-agent",
		description: "Agent for task_id testing",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "continued", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	baseURL := startTestServer(t, agent)
	ctx := context.Background()

	client, err := NewClient(ctx, baseURL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// First message to create a task
	result, err := client.SendMessage(ctx, "first message")
	if err != nil {
		t.Fatalf("first SendMessage failed: %v", err)
	}

	// Extract task ID from result
	task, ok := result.(*a2a.Task)
	if !ok {
		t.Skip("result is not a Task, skipping task ID continuation test")
	}

	// The first task completed (terminal state), so the A2A protocol correctly
	// rejects continuation. Verify the server enforces this constraint.
	_, err = client.SendMessage(ctx, "continue", WithTaskID(task.ID))
	if err == nil {
		t.Error("expected error when continuing a completed task")
	}
}

func TestIntegration_ClientFromCard(t *testing.T) {
	agent := &mockAgent{
		name:        "fromcard-agent",
		description: "Agent for NewClientFromCard testing",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "from card!", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	baseURL := startTestServer(t, agent)
	ctx := context.Background()

	// First resolve card manually via NewClient
	discoveryClient, err := NewClient(ctx, baseURL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	card := discoveryClient.Card()

	// Now create client from pre-resolved card
	client, err := NewClientFromCard(ctx, card, WithTimeout(10*time.Second))
	if err != nil {
		t.Fatalf("NewClientFromCard failed: %v", err)
	}

	if client.Card().Name != card.Name {
		t.Errorf("expected card name %q, got %q", card.Name, client.Card().Name)
	}

	result, err := client.SendMessage(ctx, "hello from card client")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	text := ExtractResultText(result)
	if text == "" {
		t.Error("expected non-empty response text")
	}
}

func TestIntegration_SendMessageStream(t *testing.T) {
	agent := &mockAgent{
		name:        "stream-agent",
		description: "Agent for streaming test",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "chunk1 ", Timestamp: time.Now()},
			{Type: interfaces.AgentEventContent, Content: "chunk2", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	baseURL := startTestServer(t, agent)
	ctx := context.Background()

	client, err := NewClient(ctx, baseURL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Use streaming API
	iter := client.SendMessageStream(ctx, "stream me")
	eventCount := 0
	var lastErr error
	iter(func(event a2a.Event, err error) bool {
		if err != nil {
			lastErr = err
			return false
		}
		eventCount++
		return true
	})

	if lastErr != nil {
		t.Fatalf("streaming error: %v", lastErr)
	}
	if eventCount == 0 {
		t.Error("expected at least one streaming event")
	}
}

func TestIntegration_SendMessageStreamWithContextID(t *testing.T) {
	agent := &mockAgent{
		name:        "stream-ctx-agent",
		description: "Agent for streaming with context",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "streamed", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	baseURL := startTestServer(t, agent)
	ctx := context.Background()

	client, err := NewClient(ctx, baseURL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	iter := client.SendMessageStream(ctx, "stream with context", WithContextID("ctx-stream-1"))
	eventCount := 0
	iter(func(event a2a.Event, err error) bool {
		if err != nil {
			return false
		}
		eventCount++
		return true
	})

	if eventCount == 0 {
		t.Error("expected at least one streaming event")
	}
}

func TestIntegration_RemoteAgentTool(t *testing.T) {
	agent := &mockAgent{
		name:        "tool-test-agent",
		description: "Agent used via tool interface",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "tool response", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	baseURL := startTestServer(t, agent)
	ctx := context.Background()

	client, err := NewClient(ctx, baseURL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	tool := NewRemoteAgentTool(client)

	// Verify tool metadata
	if tool.Name() == "" {
		t.Error("expected non-empty tool name")
	}
	if tool.Description() == "" {
		t.Error("expected non-empty tool description")
	}
	params := tool.Parameters()
	if _, ok := params["query"]; !ok {
		t.Error("expected 'query' parameter")
	}

	// Run via tool interface
	result, err := tool.Run(ctx, "hello via tool")
	if err != nil {
		t.Fatalf("tool.Run failed: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty tool result")
	}

	// Execute with JSON args
	result, err = tool.Execute(ctx, `{"query": "hello via execute"}`)
	if err != nil {
		t.Fatalf("tool.Execute failed: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty execute result")
	}
}

func TestIntegration_RemoteAgentToolWithName(t *testing.T) {
	agent := &mockAgent{
		name:        "named-tool-agent",
		description: "Agent for tool name override test",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "ok", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	baseURL := startTestServer(t, agent)
	ctx := context.Background()

	client, err := NewClient(ctx, baseURL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	tool := NewRemoteAgentTool(client, WithToolName("my_custom_tool"))
	if tool.Name() != "my_custom_tool" {
		t.Errorf("expected 'my_custom_tool', got %q", tool.Name())
	}
}

func TestIntegration_RemoteAgentTool_RunError(t *testing.T) {
	agent := &mockAgent{
		name:        "run-error-agent",
		description: "Agent for tool.Run error path",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "ok", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	baseURL := startTestServer(t, agent)

	client, err := NewClient(context.Background(), baseURL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	tool := NewRemoteAgentTool(client)

	// Use a cancelled context to trigger the error path in Run
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = tool.Run(canceledCtx, "should fail")
	if err == nil {
		t.Error("expected error from tool.Run with cancelled context")
	}
	if !strings.Contains(err.Error(), "a2a tool") {
		t.Errorf("expected wrapped error with 'a2a tool' prefix, got: %v", err)
	}
}

func TestIntegration_RemoteAgentTool_ExecuteInvalidJSON(t *testing.T) {
	agent := &mockAgent{
		name:        "json-test-agent",
		description: "test",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	baseURL := startTestServer(t, agent)
	ctx := context.Background()

	client, err := NewClient(ctx, baseURL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	tool := NewRemoteAgentTool(client)

	// Invalid JSON
	_, err = tool.Execute(ctx, "not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}

	// Empty query
	_, err = tool.Execute(ctx, `{"query": ""}`)
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestIntegration_ClientOptions(t *testing.T) {
	agent := &mockAgent{
		name:        "options-agent",
		description: "test",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	baseURL := startTestServer(t, agent)
	ctx := context.Background()

	client, err := NewClient(ctx, baseURL,
		WithClientLogger(logging.New()),
		WithTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("NewClient with options failed: %v", err)
	}
	if client.Card() == nil {
		t.Error("expected non-nil card")
	}
}

func TestIntegration_ServerOptions(t *testing.T) {
	agent := &mockAgent{
		name:        "srv-options-agent",
		description: "test",
	}

	card := NewCardBuilder("test", "test", "http://localhost").Build()

	srv := NewServer(agent, card,
		WithAddress("127.0.0.1:0"),
		WithBasePath("/custom"),
		WithShutdownTimeout(5*time.Second),
	)

	if srv.Addr() != "127.0.0.1:0" {
		t.Errorf("expected addr 127.0.0.1:0, got %s", srv.Addr())
	}
}

func TestIntegration_BearerToken(t *testing.T) {
	agent := &mockAgent{
		name:        "auth-agent",
		description: "Agent testing bearer token",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "authenticated!", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	// Auth middleware that rejects unauthenticated requests on the JSON-RPC endpoint
	// but allows the agent card endpoint through (so client can discover the card).
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allow agent card discovery without auth
			if r.URL.Path == "/.well-known/agent-card.json" {
				next.ServeHTTP(w, r)
				return
			}
			// Require bearer token on all other endpoints
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-secret-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	baseURL := startTestServer(t, agent, WithMiddleware(authMiddleware))
	ctx := context.Background()

	// Without token: should fail
	clientNoAuth, err := NewClient(ctx, baseURL)
	if err != nil {
		t.Fatalf("NewClient (no auth) failed: %v", err)
	}
	_, err = clientNoAuth.SendMessage(ctx, "should fail")
	if err == nil {
		t.Error("expected error when sending without bearer token")
	}

	// With token: should succeed
	clientAuth, err := NewClient(ctx, baseURL, WithBearerToken("test-secret-token"))
	if err != nil {
		t.Fatalf("NewClient (auth) failed: %v", err)
	}
	result, err := clientAuth.SendMessage(ctx, "hello with auth")
	if err != nil {
		t.Fatalf("SendMessage with token failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	text := ExtractResultText(result)
	if text != "authenticated!" {
		t.Errorf("expected 'authenticated!', got %q", text)
	}
}
