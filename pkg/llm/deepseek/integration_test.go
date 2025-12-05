package deepseek

import (
	"context"
	"os"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// TestRealAPIIntegration tests the DeepSeek client with the real API
// This test is skipped by default and only runs when DEEPSEEK_API_KEY is set
func TestRealAPIIntegration(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: DEEPSEEK_API_KEY not set")
	}

	client := NewClient(apiKey, WithModel("deepseek-chat"))
	ctx := context.Background()

	t.Run("Simple Generation", func(t *testing.T) {
		response, err := client.Generate(ctx, "Say 'Hello, DeepSeek!' and nothing else.")
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}

		if response == "" {
			t.Fatal("Expected non-empty response")
		}

		t.Logf("Response: %s", response)
	})

	t.Run("Detailed Response", func(t *testing.T) {
		detailedResp, err := client.GenerateDetailed(
			ctx,
			"What is 2+2? Answer with just the number.",
			interfaces.WithTemperature(0.1),
		)
		if err != nil {
			t.Fatalf("GenerateDetailed failed: %v", err)
		}

		if detailedResp.Content == "" {
			t.Fatal("Expected non-empty content")
		}

		if detailedResp.Usage.TotalTokens == 0 {
			t.Fatal("Expected non-zero token usage")
		}

		t.Logf("Content: %s", detailedResp.Content)
		t.Logf("Model: %s", detailedResp.Model)
		t.Logf("Input Tokens: %d", detailedResp.Usage.InputTokens)
		t.Logf("Output Tokens: %d", detailedResp.Usage.OutputTokens)
		t.Logf("Total Tokens: %d", detailedResp.Usage.TotalTokens)
	})

	t.Run("With System Message", func(t *testing.T) {
		response, err := client.Generate(
			ctx,
			"What is your primary function?",
			interfaces.WithSystemMessage("You are a helpful assistant. Always start your responses with 'As an AI assistant,'"),
		)
		if err != nil {
			t.Fatalf("Generate with system message failed: %v", err)
		}

		if response == "" {
			t.Fatal("Expected non-empty response")
		}

		t.Logf("Response with system message: %s", response)
	})
}
