package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/vertex"
)

// ExampleTool implements a simple calculator tool
type ExampleTool struct{}

func (t *ExampleTool) Name() string {
	return "calculator"
}

func (t *ExampleTool) Description() string {
	return "Performs basic arithmetic operations"
}

func (t *ExampleTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"operation": {
			Type:        "string",
			Description: "The operation to perform: add, subtract, multiply, divide",
			Required:    true,
			Enum:        []interface{}{"add", "subtract", "multiply", "divide"},
		},
		"a": {
			Type:        "number",
			Description: "First number",
			Required:    true,
		},
		"b": {
			Type:        "number",
			Description: "Second number",
			Required:    true,
		},
	}
}

func (t *ExampleTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *ExampleTool) Execute(ctx context.Context, args string) (string, error) {
	// Simple implementation - in a real tool you'd parse the JSON args properly
	log.Printf("Calculator tool called with args: %s", args)

	// For this example, just return a mock result
	return "The calculation result is 42", nil
}

func main() {
	ctx := context.Background()

	// Get project ID from environment
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT environment variable is required")
	}

	// Optional: Set credentials file path if not using Application Default Credentials
	credentialsFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")

	// Create client options
	options := []vertex.ClientOption{
		vertex.WithModel(vertex.ModelGemini15Pro),
		vertex.WithLocation("us-central1"),
		vertex.WithMaxRetries(3),
	}

	if credentialsFile != "" {
		options = append(options, vertex.WithCredentialsFile(credentialsFile))
	}

	// Create Vertex AI client
	client, err := vertex.NewClient(ctx, projectID, options...)
	if err != nil {
		log.Fatalf("Failed to create Vertex AI client: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("Failed to close Vertex AI client: %v", err)
		}
	}()

	fmt.Printf("Created Vertex AI client: %s\n", client.Name())

	// Example 1: Basic text generation
	fmt.Println("\n=== Example 1: Basic Text Generation ===")

	response, err := client.Generate(ctx, "Write a haiku about artificial intelligence")
	if err != nil {
		log.Fatalf("Failed to generate text: %v", err)
	}
	fmt.Printf("Generated haiku:\n%s\n", response)

	// Sleep to avoid rate limits
	time.Sleep(30 * time.Second)

	// Example 2: Generation with system message
	fmt.Println("\n=== Example 2: Generation with System Message ===")

	response, err = client.Generate(ctx,
		"Explain quantum computing",
		func(options *interfaces.GenerateOptions) {
			options.SystemMessage = "You are a physics professor explaining complex topics to undergraduate students."
		},
	)
	if err != nil {
		log.Fatalf("Failed to generate with system message: %v", err)
	}
	fmt.Printf("Explanation:\n%s\n", response)

	// Sleep to avoid rate limits
	time.Sleep(30 * time.Second)

	// Example 3: Different reasoning modes
	fmt.Println("\n=== Example 3: Reasoning Modes ===")

	prompt := "How do neural networks learn?"

	// No reasoning
	fmt.Println("--- No Reasoning ---")
	response, err = client.Generate(ctx, prompt,
		func(options *interfaces.GenerateOptions) {
			if options.LLMConfig == nil {
				options.LLMConfig = &interfaces.LLMConfig{}
			}
			options.LLMConfig.Reasoning = "none"
		},
	)
	if err != nil {
		log.Printf("Failed to generate with no reasoning: %v", err)
	} else {
		fmt.Printf("%s\n", response)
	}

	// Sleep between reasoning examples
	time.Sleep(30 * time.Second)

	// Minimal reasoning
	fmt.Println("\n--- Minimal Reasoning ---")
	response, err = client.Generate(ctx, prompt,
		func(options *interfaces.GenerateOptions) {
			if options.LLMConfig == nil {
				options.LLMConfig = &interfaces.LLMConfig{}
			}
			options.LLMConfig.Reasoning = "minimal"
		},
	)
	if err != nil {
		log.Printf("Failed to generate with minimal reasoning: %v", err)
	} else {
		fmt.Printf("%s\n", response)
	}

	// Sleep to avoid rate limits
	time.Sleep(30 * time.Second)

	// Example 4: Generation with tools
	fmt.Println("\n=== Example 4: Generation with Tools ===")

	tools := []interfaces.Tool{
		&ExampleTool{},
	}

	response, err = client.GenerateWithTools(ctx,
		"I need to calculate 15 + 27. Can you help me?",
		tools,
		func(options *interfaces.GenerateOptions) {
			options.SystemMessage = "You are a helpful assistant that can perform calculations."
			if options.LLMConfig == nil {
				options.LLMConfig = &interfaces.LLMConfig{}
			}
			options.LLMConfig.Temperature = 0.3
		},
	)
	if err != nil {
		log.Printf("Failed to generate with tools: %v", err)
	} else {
		fmt.Printf("Response with tool usage:\n%s\n", response)
	}

	// Sleep to avoid rate limits
	time.Sleep(30 * time.Second)

	// Example 5: Different models
	fmt.Println("\n=== Example 5: Different Models ===")

	// Test with Gemini 1.5 Flash (faster, cheaper)
	flashClient, err := vertex.NewClient(ctx, projectID,
		vertex.WithModel(vertex.ModelGemini15Flash),
		vertex.WithLocation("us-central1"),
	)
	if err != nil {
		log.Printf("Failed to create Flash client: %v", err)
	} else {
		defer func() {
			if err := flashClient.Close(); err != nil {
				log.Printf("Failed to close Flash client: %v", err)
			}
		}()

		response, err = flashClient.Generate(ctx, "Write a short joke about programming")
		if err != nil {
			log.Printf("Failed to generate with Flash model: %v", err)
		} else {
			fmt.Printf("Gemini 1.5 Flash response:\n%s\n", response)
		}
	}

	// Sleep to avoid rate limits
	time.Sleep(30 * time.Second)

	// Example 6: Temperature and parameter control
	fmt.Println("\n=== Example 6: Parameter Control ===")

	response, err = client.Generate(ctx,
		"Write a creative story opening about a robot",
		func(options *interfaces.GenerateOptions) {
			if options.LLMConfig == nil {
				options.LLMConfig = &interfaces.LLMConfig{}
			}
			options.LLMConfig.Temperature = 0.9                // High creativity
			options.LLMConfig.TopP = 0.95                      // Diverse vocabulary
			options.LLMConfig.StopSequences = []string{"\n\n"} // Stop at double newline
		},
	)
	if err != nil {
		log.Printf("Failed to generate with parameters: %v", err)
	} else {
		fmt.Printf("Creative story opening:\n%s\n", response)
	}

	fmt.Println("\n=== All examples completed! ===")
}
