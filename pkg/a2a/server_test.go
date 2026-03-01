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

func TestServer_ResolvedAddr(t *testing.T) {
	agent := &mockAgent{
		name:        "addr-agent",
		description: "Addr test",
	}
	card := NewCardBuilder("Addr Agent", "test", "http://localhost/a2a").Build()
	srv := NewServer(agent, card, WithAddress("127.0.0.1:0"))

	// Before Start, returns configured address
	if srv.Addr() != "127.0.0.1:0" {
		t.Errorf("expected configured addr '127.0.0.1:0', got %s", srv.Addr())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Give server time to start and resolve addr
	time.Sleep(50 * time.Millisecond)

	// After Start, returns resolved address with real port
	resolvedAddr := srv.Addr()
	if resolvedAddr == "127.0.0.1:0" {
		t.Error("expected resolved addr to differ from configured addr")
	}
	if resolvedAddr == "" {
		t.Error("expected non-empty resolved addr")
	}

	cancel()
	<-errCh
}

func TestServer_Middleware(t *testing.T) {
	agent := &mockAgent{
		name:        "mw-agent",
		description: "Middleware test",
		streamEvents: []interfaces.AgentStreamEvent{
			{Type: interfaces.AgentEventContent, Content: "ok", Timestamp: time.Now()},
			{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
		},
	}

	card := NewCardBuilder("MW Agent", "test", "http://localhost/a2a").Build()

	headerSeen := false
	testMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Test-Auth") == "secret" {
				headerSeen = true
			}
			next.ServeHTTP(w, r)
		})
	}

	srv := NewServer(agent, card, WithMiddleware(testMiddleware))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	go func() {
		_ = http.Serve(listener, srv.Handler())
	}()

	addr := listener.Addr().String()

	// Request with custom header
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s/.well-known/agent-card.json", addr), nil)
	req.Header.Set("X-Test-Auth", "secret")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if !headerSeen {
		t.Error("middleware was not invoked")
	}
}

func TestServer_ShutdownTimeout(t *testing.T) {
	agent := &mockAgent{
		name:        "shutdown-agent",
		description: "Shutdown test",
	}
	card := NewCardBuilder("Shutdown Agent", "test", "http://localhost/a2a").Build()
	srv := NewServer(agent, card,
		WithAddress("127.0.0.1:0"),
		WithShutdownTimeout(1*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	start := time.Now()
	cancel()

	select {
	case err := <-errCh:
		elapsed := time.Since(start)
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("unexpected error: %v", err)
		}
		// Graceful shutdown should be fast when no active connections
		if elapsed > 2*time.Second {
			t.Errorf("shutdown took too long: %v", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestServer_MiddlewareOrdering(t *testing.T) {
	agent := &mockAgent{name: "order-agent", description: "test"}
	card := NewCardBuilder("Order Agent", "test", "http://localhost/a2a").Build()

	var order []string

	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw1")
			next.ServeHTTP(w, r)
		})
	}
	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw2")
			next.ServeHTTP(w, r)
		})
	}

	srv := NewServer(agent, card, WithMiddleware(mw1), WithMiddleware(mw2))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	go func() {
		_ = http.Serve(listener, srv.Handler())
	}()

	resp, err := http.Get(fmt.Sprintf("http://%s/.well-known/agent-card.json", listener.Addr().String()))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if len(order) != 2 {
		t.Fatalf("expected 2 middleware calls, got %d", len(order))
	}
	if order[0] != "mw1" || order[1] != "mw2" {
		t.Errorf("expected middleware order [mw1, mw2], got %v", order)
	}
}

func TestServer_NilAgent(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil agent")
		}
	}()
	card := NewCardBuilder("test", "test", "http://localhost").Build()
	NewServer(nil, card)
}

func TestServer_NilCard(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil agentCard")
		}
	}()
	agent := &mockAgent{name: "test", description: "test"}
	NewServer(agent, nil)
}
