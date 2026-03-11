package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
)

func main() {
	// Load local .env if present (for examples convenience).
	// Note: shared.CreateLLM() doesn't read this cache; this example uses agent.GetEnvValue().
	_ = agent.LoadEnvFile(".env")

	fmt.Println("UI Multimodal Server Example (HTTP + Embedded UI)")
	fmt.Println("=================================================")
	fmt.Println()

	apiKey := agent.GetEnvValue("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required (or define it in .env in this folder)")
	}

	opts := make([]openai.Option, 0, 2)
	if model := agent.GetEnvValue("OPENAI_MODEL"); model != "" {
		opts = append(opts, openai.WithModel(model))
	}
	if baseURL := agent.GetEnvValue("OPENAI_BASE_URL"); baseURL != "" {
		opts = append(opts, openai.WithBaseURL(baseURL))
	}
	llm := openai.NewClient(apiKey, opts...)

	a, err := agent.NewAgent(
		agent.WithName("UIMultimodalAgent"),
		agent.WithDescription("Embedded UI server; supports multimodal content_parts (image_url via data URL)"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt(`You are a helpful assistant.

- If the user provides images (multimodal content_parts), describe them clearly.
- If the user provides a prompt and images, use both.`),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	port := 8085
	if v := agent.GetEnvValue("UI_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			port = p
		}
	}

	ui := microservice.NewHTTPServerWithUI(a, port, &microservice.UIConfig{
		Enabled:     true,
		DefaultPath: "/",
		DevMode:     false,
		Theme:       "light",
		Features: microservice.UIFeatures{
			Chat:      true,
			Memory:    true,
			AgentInfo: true,
			Settings:  true,
			Traces:    false,
		},
	})

	errCh := make(chan error, 1)
	go func() { errCh <- ui.Start() }()

	fmt.Printf("UI:  http://localhost:%d/\n", port)
	fmt.Printf("API: http://localhost:%d/api/v1\n", port)
	fmt.Println()
	fmt.Println("Use case:")
	fmt.Println("- Run the client example in another terminal:")
	fmt.Printf("  go run ./examples/microservices/ui_multimodal_server/client --base-url http://localhost:%d --image ./test.jpg --prompt \"描述这张图片\"\n", port)
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatalf("UI server exited: %v", err)
		}
	case <-sigCh:
	}

	fmt.Println("\nShutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := ui.Stop(ctx); err != nil {
		log.Printf("Warning: failed to stop UI server: %v", err)
	}
	fmt.Println("Stopped.")
}
