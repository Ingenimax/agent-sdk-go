// Package main demonstrates exposing an agent-sdk-go agent as an A2A server.
// This server is discoverable by any A2A-compliant client (Google ADK, LangChain, etc.)
//
// Run with:
//
//	go run ./examples/a2a/server
//
// Then test with:
//
//	curl http://localhost:9100/.well-known/agent-card.json
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/a2aproject/a2a-go/a2a"

	a2apkg "github.com/Ingenimax/agent-sdk-go/pkg/a2a"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// echoAgent is a simple agent that echoes back the user's input.
// Replace this with a real agent built with agent.NewAgent().
type echoAgent struct{}

func (e *echoAgent) GetName() string        { return "echo" }
func (e *echoAgent) GetDescription() string  { return "Echoes back your message" }

func (e *echoAgent) Run(_ context.Context, input string) (string, error) {
	return fmt.Sprintf("Echo: %s", input), nil
}

func (e *echoAgent) RunStream(_ context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
	ch := make(chan interfaces.AgentStreamEvent, 2)
	go func() {
		defer close(ch)
		ch <- interfaces.AgentStreamEvent{
			Type:    interfaces.AgentEventContent,
			Content: fmt.Sprintf("Echo: %s", input),
		}
		ch <- interfaces.AgentStreamEvent{
			Type: interfaces.AgentEventComplete,
		}
	}()
	return ch, nil
}

func main() {
	agent := &echoAgent{}

	// Build the agent card that describes this agent to A2A clients
	card := a2apkg.NewCardBuilder(
		"Echo Agent",
		"A simple echo agent that repeats your message back to you",
		"http://localhost:9100/",
		a2apkg.WithVersion("1.0.0"),
		a2apkg.WithProviderInfo("Ingenimax", "https://github.com/Ingenimax"),
		a2apkg.WithStreaming(true),
	).AddSkill(a2a.AgentSkill{
		ID:          "echo",
		Name:        "Echo",
		Description: "Echoes back the user's message",
		Tags:        []string{"echo", "demo"},
		Examples:    []string{"Hello, world!"},
	}).Build()

	// Create and start the A2A server
	srv := a2apkg.NewServer(agent, card,
		a2apkg.WithAddress(":9100"),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Printf("Starting A2A server on :9100")
	log.Printf("Agent card: http://localhost:9100/.well-known/agent-card.json")

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
