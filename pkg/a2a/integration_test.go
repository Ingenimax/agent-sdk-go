package a2a

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// startTestServer creates and starts an A2A server on a random port, returning the base URL.
func startTestServer(t *testing.T, agent AgentAdapter) string {
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

	srv := NewServer(agent, card, WithServerLogger(logging.New()))

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

	// Extract text from the result
	text := extractResultText(result)
	if text == "" {
		t.Error("expected non-empty response text")
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

	store := NewInMemoryTaskStore()
	srv := NewServer(agent, card,
		WithAddress("127.0.0.1:0"),
		WithBasePath("/custom"),
		WithTaskStore(store),
	)

	if srv.Addr() != "127.0.0.1:0" {
		t.Errorf("expected addr 127.0.0.1:0, got %s", srv.Addr())
	}
}
