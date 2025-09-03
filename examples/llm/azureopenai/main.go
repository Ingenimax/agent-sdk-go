package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/azureopenai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// isReasoningModel returns true if the model is a reasoning model (o1, o3, etc.)
func isReasoningModel(model string) bool {
	reasoningModels := []string{
		"o1-", "o1-mini", "o1-preview",
		"o3-", "o3-mini",
		"o4-", "o4-mini",
		"gpt-5", "gpt-5-mini", "gpt-5-nano",
	}

	for _, prefix := range reasoningModels {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

func main() {
	// Create a logger
	logger := logging.New()
	ctx := context.Background()

	// Get Azure OpenAI configuration from environment
	apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	baseURL := os.Getenv("AZURE_OPENAI_BASE_URL")
	region := os.Getenv("AZURE_OPENAI_REGION")
	resourceName := os.Getenv("AZURE_OPENAI_RESOURCE_NAME")
	deployment := os.Getenv("AZURE_OPENAI_DEPLOYMENT")
	apiVersion := os.Getenv("AZURE_OPENAI_API_VERSION")

	if apiKey == "" {
		logger.Error(ctx, "AZURE_OPENAI_API_KEY environment variable is required", nil)
		os.Exit(1)
	}

	// Check if we have either baseURL or region+resourceName
	if baseURL == "" && (region == "" || resourceName == "") {
		logger.Error(ctx, "Either AZURE_OPENAI_BASE_URL or both AZURE_OPENAI_REGION and AZURE_OPENAI_RESOURCE_NAME must be provided", nil)
		os.Exit(1)
	}

	if deployment == "" {
		logger.Error(ctx, "AZURE_OPENAI_DEPLOYMENT environment variable is required", nil)
		os.Exit(1)
	}

	// Create client with options
	clientOptions := []azureopenai.Option{
		azureopenai.WithLogger(logger),
	}

	// Add API version if provided
	if apiVersion != "" {
		clientOptions = append(clientOptions, azureopenai.WithAPIVersion(apiVersion))
	}

	// Add region and resource name if provided
	if region != "" {
		clientOptions = append(clientOptions, azureopenai.WithRegion(region))
	}
	if resourceName != "" {
		clientOptions = append(clientOptions, azureopenai.WithResourceName(resourceName))
	}

	var client *azureopenai.AzureOpenAIClient

	// Choose creation method based on available configuration
	if baseURL != "" {
		// Traditional approach with base URL
		client = azureopenai.NewClient(apiKey, baseURL, deployment, clientOptions...)
		logger.Info(ctx, "Using base URL approach", map[string]interface{}{"base_url": baseURL})
	} else {
		// Region-based approach (recommended)
		client = azureopenai.NewClientFromRegion(apiKey, region, resourceName, deployment, clientOptions...)
		logger.Info(ctx, "Using region-based approach", map[string]interface{}{
			"region":        region,
			"resource_name": resourceName,
		})
	}

	logger.Info(ctx, "Azure OpenAI client created", map[string]interface{}{
		"deployment":    client.GetDeployment(),
		"model":         client.GetModel(),
		"base_url":      client.GetBaseURL(),
		"region":        client.GetRegion(),
		"resource_name": client.GetResourceName(),
		"api_version":   apiVersion,
	})

	// Test 1: Simple text generation with system message
	logger.Info(ctx, "Testing simple text generation...", nil)
	resp, err := client.Generate(
		ctx,
		"Write a haiku about programming",
		azureopenai.WithSystemMessage("You are a creative assistant who specializes in writing haikus."),
		azureopenai.WithTemperature(0.7),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "Generated haiku", map[string]interface{}{"text": resp})

	// Test 2: Chat completion
	logger.Info(ctx, "Testing chat completion...", nil)
	messages := []llm.Message{
		{
			Role:    "system",
			Content: "You are a helpful programming assistant.",
		},
		{
			Role:    "user",
			Content: "What's the best way to handle errors in Go?",
		},
	}

	resp, err = client.Chat(ctx, messages, nil)
	if err != nil {
		logger.Error(ctx, "Failed to chat", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "Chat response", map[string]interface{}{"text": resp})

	// Test 3: Multi-turn conversation with Chat method
	logger.Info(ctx, "Testing multi-turn conversation...", nil)
	multiTurnMessages := []llm.Message{
		{
			Role:    "system",
			Content: "You are a senior Go programmer who provides concise code examples.",
		},
		{
			Role:    "user",
			Content: "Show me how to implement a simple HTTP server in Go.",
		},
	}

	resp, err = client.Chat(ctx, multiTurnMessages, &llm.GenerateParams{
		Temperature: 0.5,
	})
	if err != nil {
		logger.Error(ctx, "Failed multi-turn chat", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "Multi-turn response", map[string]interface{}{"text": resp})

	// Test 4: Generation with reasoning modes
	logger.Info(ctx, "Testing reasoning modes...", nil)

	reasoningModes := []string{"none", "minimal", "comprehensive"}
	for _, mode := range reasoningModes {
		logger.Info(ctx, "Testing reasoning mode", map[string]interface{}{"mode": mode})

		resp, err := client.Generate(
			ctx,
			"Explain how to calculate the factorial of 5",
			azureopenai.WithReasoning(mode),
			azureopenai.WithTemperature(0.3),
		)
		if err != nil {
			logger.Error(ctx, "Failed reasoning test", map[string]interface{}{
				"mode":  mode,
				"error": err.Error(),
			})
			continue
		}

		logger.Info(ctx, "Reasoning response", map[string]interface{}{
			"mode": mode,
			"text": resp,
		})
	}

	// Test 5: Generation with various parameters
	logger.Info(ctx, "Testing parameter variations...", nil)
	resp, err = client.Generate(
		ctx,
		"List 3 benefits of using Go for backend development",
		azureopenai.WithTemperature(0.2),
		azureopenai.WithTopP(0.9),
		azureopenai.WithFrequencyPenalty(0.1),
		azureopenai.WithPresencePenalty(0.1),
		azureopenai.WithStopSequences([]string{"4.", "Fourth"}),
	)
	if err != nil {
		logger.Error(ctx, "Failed parameter test", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "Parameter test response", map[string]interface{}{"text": resp})

	// Test 6: Structured output (JSON)
	logger.Info(ctx, "Testing structured output...", nil)
	jsonSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"language": map[string]interface{}{
				"type":        "string",
				"description": "The programming language name",
			},
			"benefits": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"description": "List of benefits",
			},
			"difficulty": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"beginner", "intermediate", "advanced"},
				"description": "Learning difficulty level",
			},
		},
		"required": []string{"language", "benefits", "difficulty"},
	}

	resp, err = client.Generate(
		ctx,
		"Describe the Go programming language",
		azureopenai.WithResponseFormat(interfaces.ResponseFormat{
			Name:   "language_info",
			Schema: jsonSchema,
		}),
		azureopenai.WithTemperature(0.3),
	)
	if err != nil {
		logger.Error(ctx, "Failed structured output test", map[string]interface{}{
			"error": err.Error(),
			"note":  "Structured output requires API version 2024-08-01-preview or later",
		})
		// Check if it's an API version issue
		if strings.Contains(err.Error(), "json_schema is enabled only for api versions") {
			logger.Info(ctx, "Suggestion: Update AZURE_OPENAI_API_VERSION to 2024-08-01-preview or later", nil)
		}
	} else {
		logger.Info(ctx, "Structured output response", map[string]interface{}{"json": resp})
	}

	// Test 7: Basic Streaming
	logger.Info(ctx, "Testing basic streaming...", nil)
	eventChan, err := client.GenerateStream(
		ctx,
		"Tell me a short story about a robot learning to paint",
		azureopenai.WithTemperature(0.8),
		azureopenai.WithSystemMessage("You are a creative storyteller."),
	)
	if err != nil {
		logger.Error(ctx, "Failed to start streaming", map[string]interface{}{"error": err.Error()})
	} else {
		logger.Info(ctx, "Streaming started, receiving events...", nil)
		var streamedContent string
		for event := range eventChan {
			switch event.Type {
			case interfaces.StreamEventMessageStart:
				fmt.Print("\n[STREAM START] ")
			case interfaces.StreamEventContentDelta:
				fmt.Print(event.Content)
				streamedContent += event.Content
			case interfaces.StreamEventContentComplete:
				fmt.Println("\n[STREAM COMPLETE]")
			case interfaces.StreamEventMessageStop:
				logger.Info(ctx, "Stream finished", map[string]interface{}{
					"total_length": len(streamedContent),
				})
			case interfaces.StreamEventError:
				logger.Error(ctx, "Stream error", map[string]interface{}{"error": event.Error})
			}
		}
	}

	// Test 8: Prompt-based reasoning with different modes
	logger.Info(ctx, "Testing prompt-based reasoning modes (system message enhancement)...", nil)
	logger.Info(ctx, "Note: This uses prompt engineering, not native model reasoning tokens", nil)

	reasoningTests := []struct {
		mode        string
		prompt      string
		description string
	}{
		{
			mode:        "none",
			prompt:      "What is 15 * 24?",
			description: "Simple calculation with no reasoning",
		},
		{
			mode:        "minimal",
			prompt:      "Explain why the sky appears blue",
			description: "Scientific explanation with brief reasoning",
		},
		{
			mode:        "comprehensive",
			prompt:      "How would you design a simple recommendation system for a bookstore?",
			description: "Complex problem requiring detailed step-by-step reasoning",
		},
	}

	for i, test := range reasoningTests {
		logger.Info(ctx, fmt.Sprintf("Reasoning test %d/%d", i+1, len(reasoningTests)), map[string]interface{}{
			"mode":        test.mode,
			"description": test.description,
		})

		resp, err := client.Generate(
			ctx,
			test.prompt,
			azureopenai.WithReasoning(test.mode),
			azureopenai.WithTemperature(0.3),
		)
		if err != nil {
			logger.Error(ctx, "Failed reasoning test", map[string]interface{}{
				"mode":  test.mode,
				"error": err.Error(),
			})
			continue
		}

		logger.Info(ctx, fmt.Sprintf("Reasoning (%s) response", test.mode), map[string]interface{}{
			"prompt":   test.prompt,
			"response": resp,
			"length":   len(resp),
		})

		// Add a small delay between tests
		time.Sleep(1 * time.Second)
	}

	// Test 9: Native reasoning model support (if using o1/reasoning models)
	logger.Info(ctx, "Testing native reasoning model support...", nil)
	if isReasoningModel(client.GetModel()) {
		logger.Info(ctx, "Detected reasoning model - using native reasoning tokens", map[string]interface{}{
			"model": client.GetModel(),
		})

		// For reasoning models, use EnableReasoning instead of Reasoning
		resp, err := client.Generate(
			ctx,
			"Solve this step by step: If a train travels 120 miles in 2 hours, and then 180 miles in the next 3 hours, what is the average speed for the entire journey?",
			interfaces.WithReasoning(true),   // This enables native reasoning tokens
			azureopenai.WithTemperature(1.0), // Reasoning models require temperature = 1.0
		)
		if err != nil {
			logger.Error(ctx, "Failed native reasoning test", map[string]interface{}{"error": err.Error()})
		} else {
			logger.Info(ctx, "Native reasoning response", map[string]interface{}{
				"response": resp,
				"length":   len(resp),
			})
		}
	} else {
		logger.Info(ctx, "Non-reasoning model detected - native reasoning not available", map[string]interface{}{
			"model": client.GetModel(),
			"note":  "Use o1-preview, o1-mini, or other reasoning models for native reasoning tokens",
		})
	}

	// Test 11: Streaming with Reasoning
	logger.Info(ctx, "Testing streaming with prompt-based reasoning...", nil)
	eventChan, err = client.GenerateStream(
		ctx,
		"Explain the process of photosynthesis step by step",
		azureopenai.WithReasoning("comprehensive"),
		azureopenai.WithTemperature(0.4),
		azureopenai.WithSystemMessage("You are a biology teacher explaining complex concepts clearly."),
	)
	if err != nil {
		logger.Error(ctx, "Failed to start reasoning stream", map[string]interface{}{"error": err.Error()})
	} else {
		logger.Info(ctx, "Reasoning stream started...", nil)
		fmt.Println("\n=== STREAMING WITH REASONING ===")
		var reasoningContent string
		for event := range eventChan {
			switch event.Type {
			case interfaces.StreamEventMessageStart:
				fmt.Print("[REASONING STREAM] ")
			case interfaces.StreamEventContentDelta:
				fmt.Print(event.Content)
				reasoningContent += event.Content
			case interfaces.StreamEventContentComplete:
				fmt.Println("\n[REASONING COMPLETE]")
			case interfaces.StreamEventMessageStop:
				logger.Info(ctx, "Reasoning stream finished", map[string]interface{}{
					"total_length": len(reasoningContent),
				})
			case interfaces.StreamEventError:
				logger.Error(ctx, "Reasoning stream error", map[string]interface{}{"error": event.Error})
			}
		}
	}

	// Test 12: Advanced Streaming with Custom Configuration
	logger.Info(ctx, "Testing advanced streaming with custom configuration...", nil)

	// Create a custom stream configuration
	streamConfig := interfaces.StreamConfig{
		BufferSize: 50, // Smaller buffer for more responsive streaming
	}

	eventChan, err = client.GenerateStream(
		ctx,
		"Write a haiku about technology and nature, then explain your creative process",
		azureopenai.WithTemperature(0.7),
		azureopenai.WithReasoning("minimal"),
		azureopenai.WithSystemMessage("You are a poet who loves both technology and nature."),
		interfaces.WithStreamConfig(streamConfig),
	)
	if err != nil {
		logger.Error(ctx, "Failed to start advanced stream", map[string]interface{}{"error": err.Error()})
	} else {
		logger.Info(ctx, "Advanced streaming started with custom config...", nil)
		fmt.Println("\n=== ADVANCED STREAMING ===")

		var advancedContent string
		eventCount := 0
		startTime := time.Now()

		for event := range eventChan {
			eventCount++
			switch event.Type {
			case interfaces.StreamEventMessageStart:
				fmt.Printf("[%s] Stream started\n", event.Timestamp.Format("15:04:05"))
			case interfaces.StreamEventContentDelta:
				fmt.Print(event.Content)
				advancedContent += event.Content
			case interfaces.StreamEventContentComplete:
				fmt.Printf("\n[%s] Content complete\n", event.Timestamp.Format("15:04:05"))
			case interfaces.StreamEventMessageStop:
				duration := time.Since(startTime)
				logger.Info(ctx, "Advanced stream finished", map[string]interface{}{
					"total_length":  len(advancedContent),
					"event_count":   eventCount,
					"duration_ms":   duration.Milliseconds(),
					"chars_per_sec": float64(len(advancedContent)) / duration.Seconds(),
				})
			case interfaces.StreamEventError:
				logger.Error(ctx, "Advanced stream error", map[string]interface{}{
					"error":       event.Error,
					"event_count": eventCount,
				})
			}
		}
	}

	logger.Info(ctx, "All Azure OpenAI tests completed successfully!", map[string]interface{}{
		"tests_completed": []string{
			"basic_generation",
			"chat_completion",
			"multi_turn_conversation",
			"prompt_based_reasoning_modes",
			"parameter_variations",
			"structured_output",
			"basic_streaming",
			"detailed_prompt_reasoning",
			"native_reasoning_model_support",
			"streaming_with_reasoning",
			"advanced_streaming",
		},
	})
}
