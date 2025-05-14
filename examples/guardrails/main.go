package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/guardrails"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

func main() {
	// Create a logger
	logger := logging.New()
	ctx := context.Background()

	// Create guardrails
	contentFilter := guardrails.NewContentFilter(
		[]string{"hate", "violence", "profanity", "sexual"},
		guardrails.RedactAction,
	)

	tokenLimit := guardrails.NewTokenLimit(
		100,
		nil, // Use simple token counter
		guardrails.RedactAction,
		"end",
	)

	piiFilter := guardrails.NewPiiFilter(
		guardrails.RedactAction,
	)

	toolRestriction := guardrails.NewToolRestriction(
		[]string{"web_search", "calculator"},
		guardrails.BlockAction,
	)

	rateLimit := guardrails.NewRateLimit(
		10, // 10 requests per minute
		guardrails.BlockAction,
	)

	// Create a guardrails pipeline
	pipeline := guardrails.NewPipeline(
		[]guardrails.Guardrail{
			contentFilter,
			tokenLimit,
			piiFilter,
			toolRestriction,
			rateLimit,
		},
		logger,
	)

	// Create an LLM with guardrails
	openaiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"),
		openai.WithModel("gpt-4o-mini"),
		openai.WithLogger(logger),
	)
	llmWithGuardrails := guardrails.NewLLMMiddleware(openaiClient, pipeline)

	// Create a tool with guardrails
	tool := websearch.New(
		os.Getenv("GOOGLE_API_KEY"),
		os.Getenv("GOOGLE_SEARCH_ENGINE_ID"),
	)
	toolWithGuardrails := guardrails.NewToolMiddleware(tool, pipeline)

	// Test LLM with guardrails
	logger.Info(ctx, "=== Testing LLM with Guardrails ===", nil)
	testLLM(ctx, llmWithGuardrails, logger)

	// Test tool with guardrails
	logger.Info(ctx, "\n=== Testing Tool with Guardrails ===", nil)
	testTool(ctx, toolWithGuardrails, logger)
}

func testLLM(ctx context.Context, llm *guardrails.LLMMiddleware, logger logging.Logger) {
	// Test content filter
	prompt := "Tell me about violence and hate speech"
	logger.Info(ctx, fmt.Sprintf("Prompt: %s", prompt), nil)
	response, err := llm.Generate(ctx, prompt, map[string]interface{}{})
	if err != nil {
		logger.Error(ctx, "Error", map[string]interface{}{"error": err.Error()})
	} else {
		logger.Info(ctx, fmt.Sprintf("Response: %s", response), nil)
	}

	// Test PII filter
	prompt = "My email is john.doe@example.com and my phone number is 123-456-7890"
	logger.Info(ctx, fmt.Sprintf("\nPrompt: %s", prompt), nil)
	response, err = llm.Generate(ctx, prompt, map[string]interface{}{})
	if err != nil {
		logger.Error(ctx, "Error", map[string]interface{}{"error": err.Error()})
	} else {
		logger.Info(ctx, fmt.Sprintf("Response: %s", response), nil)
	}

	// Test token limit
	prompt = "Tell me a very long story about a programmer who is trying to implement guardrails for an AI system. The programmer is working day and night to make sure the AI system is safe and secure. The programmer is also trying to make sure the AI system is useful and helpful. The programmer is also trying to make sure the AI system is ethical and fair."
	logger.Info(ctx, fmt.Sprintf("\nPrompt: %s", prompt), nil)
	response, err = llm.Generate(ctx, prompt, map[string]interface{}{})
	if err != nil {
		logger.Error(ctx, "Error", map[string]interface{}{"error": err.Error()})
	} else {
		logger.Info(ctx, fmt.Sprintf("Response: %s", response), nil)
	}
}

func testTool(ctx context.Context, tool *guardrails.ToolMiddleware, logger logging.Logger) {
	// Test content filter
	input := `{"query": "Tell me about violence and hate speech"}`
	logger.Info(ctx, fmt.Sprintf("Input: %s", input), nil)
	output, err := tool.Run(ctx, input)
	if err != nil {
		logger.Error(ctx, "Error", map[string]interface{}{"error": err.Error()})
	} else {
		logger.Info(ctx, fmt.Sprintf("Output: %s", output), nil)
	}

	// Test tool restriction
	input = `Use tool aws_s3 to list all buckets`
	logger.Info(ctx, fmt.Sprintf("\nInput: %s", input), nil)
	output, err = tool.Run(ctx, input)
	if err != nil {
		logger.Error(ctx, "Error", map[string]interface{}{"error": err.Error()})
	} else {
		logger.Info(ctx, fmt.Sprintf("Output: %s", output), nil)
	}

	// Test rate limit (run multiple requests)
	logger.Info(ctx, "Testing rate limit...", nil)
	for i := 0; i < 15; i++ {
		input = fmt.Sprintf(`{"query": "Test query %d"}`, i)
		_, err := tool.Run(ctx, input)
		if err != nil {
			logger.Info(ctx, fmt.Sprintf("Request %d: Error", i), map[string]interface{}{"error": err.Error()})
		} else {
			logger.Info(ctx, fmt.Sprintf("Request %d: Success", i), nil)
		}
	}
}
