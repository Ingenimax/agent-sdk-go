package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/a2a"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestServer_AgentCardEndpoint(t *testing.T) {
	agent := &mockAgent{
		name:        "test-agent",
		description: "A test agent for A2A",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "Hello!", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	card := NewCardBuilder("Test Agent", "A test agent", "http://localhost/a2a").Build()

	srv := NewServer(agent, card, WithServerLogger(logging.New()))

	// Start server on random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	go func() {
		_ = http.Serve(listener, srv.Handler())
	}()

	addr := listener.Addr().String()

	// Fetch agent card
	resp, err := http.Get(fmt.Sprintf("http://%s/.well-known/agent-card.json", addr))
	if err != nil {
		t.Fatalf("failed to fetch agent card: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var fetchedCard a2a.AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&fetchedCard); err != nil {
		t.Fatalf("failed to decode agent card: %v", err)
	}

	if fetchedCard.Name != "Test Agent" {
		t.Errorf("expected name 'Test Agent', got %s", fetchedCard.Name)
	}
	if fetchedCard.Description != "A test agent" {
		t.Errorf("expected description 'A test agent', got %s", fetchedCard.Description)
	}
}

func TestServer_Handler(t *testing.T) {
	agent := &mockAgent{
		name:        "handler-agent",
		description: "Handler test",
	}
	card := NewCardBuilder("Handler Agent", "test", "http://localhost/a2a").Build()
	srv := NewServer(agent, card)

	if srv.Handler() == nil {
		t.Fatal("Handler() returned nil")
	}
}

func TestServer_Start_ContextCancellation(t *testing.T) {
	agent := &mockAgent{
		name:        "start-agent",
		description: "Start test",
	}
	card := NewCardBuilder("Start Agent", "test", "http://localhost/a2a").Build()
	srv := NewServer(agent, card, WithAddress("127.0.0.1:0"))

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		// Server should exit cleanly or with closed error
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down within timeout")
	}
}
