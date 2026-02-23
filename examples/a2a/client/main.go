// Package main demonstrates calling a remote A2A agent from an agent-sdk-go client.
// It connects to any A2A-compliant server (Google ADK, LangChain, or agent-sdk-go).
//
// Prerequisites:
//
//	Start an A2A server first, e.g.: go run ./examples/a2a/server
//
// Run with:
//
//	go run ./examples/a2a/client
//
// Or specify a custom URL:
//
//	go run ./examples/a2a/client http://some-other-a2a-agent:9000
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/a2aproject/a2a-go/a2a"

	a2apkg "github.com/Ingenimax/agent-sdk-go/pkg/a2a"
)

func main() {
	agentURL := "http://localhost:9100"
	if len(os.Args) > 1 {
		agentURL = os.Args[1]
	}

	ctx := context.Background()

	// Discover and connect to the remote A2A agent.
	// Use WithBearerToken("token") for authenticated agents.
	client, err := a2apkg.NewClient(ctx, agentURL)
	if err != nil {
		log.Fatalf("Failed to connect to A2A agent at %s: %v", agentURL, err)
	}

	card := client.Card()
	fmt.Printf("Connected to: %s\n", card.Name)
	fmt.Printf("Description:  %s\n", card.Description)
	fmt.Printf("Skills:       %d\n", len(card.Skills))
	for _, skill := range card.Skills {
		fmt.Printf("  - %s: %s\n", skill.Name, skill.Description)
	}
	fmt.Println()

	// Send a synchronous message
	fmt.Println("--- Sending message ---")
	result, err := client.SendMessage(ctx, "Hello from agent-sdk-go!")
	if err != nil {
		log.Fatalf("SendMessage failed: %v", err)
	}

	switch r := result.(type) {
	case *a2a.Task:
		fmt.Printf("Task ID:   %s\n", r.ID)
		fmt.Printf("Status:    %s\n", r.Status.State)
		for _, artifact := range r.Artifacts {
			for _, p := range artifact.Parts {
				if tp, ok := p.(a2a.TextPart); ok {
					fmt.Printf("Response:  %s\n", tp.Text)
				}
			}
		}
	case *a2a.Message:
		for _, p := range r.Parts {
			if tp, ok := p.(a2a.TextPart); ok {
				fmt.Printf("Response:  %s\n", tp.Text)
			}
		}
	}

	fmt.Println()

	// Multi-turn conversation using context ID
	fmt.Println("--- Multi-turn conversation ---")
	_, err = client.SendMessage(ctx, "Remember my name is Alice",
		a2apkg.WithContextID("conversation-1"),
	)
	if err != nil {
		log.Fatalf("First turn failed: %v", err)
	}
	fmt.Println("Turn 1 sent")

	result2, err := client.SendMessage(ctx, "What is my name?",
		a2apkg.WithContextID("conversation-1"),
	)
	if err != nil {
		log.Fatalf("Second turn failed: %v", err)
	}
	fmt.Printf("Turn 2 response: %s\n", a2apkg.ExtractResultText(result2))

	fmt.Println()

	// Also demonstrate using the RemoteAgentTool (wraps A2A agent as an SDK tool)
	fmt.Println("--- Using as a tool ---")
	tool := a2apkg.NewRemoteAgentTool(client)
	fmt.Printf("Tool name:  %s\n", tool.Name())
	fmt.Printf("Tool desc:  %s\n", tool.Description())

	toolResult, err := tool.Run(ctx, "This message goes through the tool interface")
	if err != nil {
		log.Fatalf("Tool.Run failed: %v", err)
	}
	fmt.Printf("Tool result: %s\n", toolResult)
}
