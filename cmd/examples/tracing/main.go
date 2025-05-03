package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
	"github.com/Ingenimax/agent-sdk-go/pkg/tracing"
)

func main() {
	// Create a logger
	logger := logging.New()

	// Create context with organization ID
	ctx := multitenancy.WithOrgID(context.Background(), "example-org")

	logger.Info(ctx, "Starting tracing example", nil)

	// Initialize Langfuse tracer
	langfuseTracer, err := tracing.NewLangfuseTracer(tracing.LangfuseConfig{
		Enabled:     true,
		SecretKey:   os.Getenv("LANGFUSE_SECRET_KEY"),
		PublicKey:   os.Getenv("LANGFUSE_PUBLIC_KEY"),
		Environment: "development",
	})
	if err != nil {
		logger.Error(ctx, "Failed to initialize Langfuse tracer", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer langfuseTracer.Flush()
	logger.Info(ctx, "Langfuse tracer initialized", nil)

	// Initialize OpenTelemetry tracer
	otelTracer, err := tracing.NewOTelTracer(tracing.OTelConfig{
		Enabled:           true,
		ServiceName:       "agent-sdk-example",
		CollectorEndpoint: "localhost:4317",
	})
	if err != nil {
		logger.Error(ctx, "Failed to initialize OpenTelemetry tracer", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "OpenTelemetry tracer initialized", nil)

	// Create LLM client with tracing
	llm := openai.NewClient(os.Getenv("OPENAI_API_KEY"),
		openai.WithModel("gpt-4o-mini"),
		openai.WithLogger(logger),
	)
	llmWithLangfuse := tracing.NewLLMMiddleware(llm, langfuseTracer)
	llmWithOTel := tracing.NewLLMOTelMiddleware(llmWithLangfuse, otelTracer)
	logger.Info(ctx, "LLM client with tracing created", nil)

	// Create memory with tracing
	mem := memory.NewConversationBuffer()
	memWithOTel := tracing.NewMemoryOTelMiddleware(mem, otelTracer)
	logger.Info(ctx, "Memory with tracing created", nil)

	// Create tools
	toolRegistry := tools.NewRegistry()
	calcTool := calculator.New()
	toolRegistry.Register(calcTool)
	searchTool := websearch.New(
		os.Getenv("GOOGLE_API_KEY"),
		os.Getenv("GOOGLE_SEARCH_ENGINE_ID"),
	)
	toolRegistry.Register(searchTool)
	logger.Info(ctx, "Tools registered", map[string]interface{}{"tools": []string{calcTool.Name(), searchTool.Name()}})

	// Create agent
	agent, err := agent.NewAgent(
		agent.WithLLM(llmWithOTel),
		agent.WithMemory(memWithOTel),
		agent.WithTools(calcTool, searchTool),
		agent.WithSystemPrompt("You are a helpful AI assistant."),
	)
	if err != nil {
		logger.Error(ctx, "Failed to create agent", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "Agent created successfully", nil)

	// Start a span for the entire conversation
	conversationID := fmt.Sprintf("conv-%d", time.Now().UnixNano())
	ctx, span := otelTracer.StartSpan(ctx, "conversation", map[string]string{
		"conversation_id": conversationID,
	})
	ctx = memory.WithConversationID(ctx, conversationID)
	defer otelTracer.EndSpan(span, nil)
	logger.Info(ctx, "Started conversation span", map[string]interface{}{"conversation_id": conversationID})

	// Handle user queries
	for {
		// Get user input
		fmt.Print("\nEnter your query (or 'exit' to quit): ")
		var query string
		reader := bufio.NewReader(os.Stdin)
		query, inputErr := reader.ReadString('\n')
		if inputErr != nil {
			logger.Error(ctx, "Error reading input", map[string]interface{}{"error": inputErr.Error()})
			continue
		}
		query = strings.TrimSpace(query)

		if query == "exit" {
			logger.Info(ctx, "User requested exit", nil)
			break
		}

		// Start a span for this query
		queryCtx, querySpan := otelTracer.StartSpan(ctx, "query", map[string]string{
			"query": query,
		})
		logger.Info(queryCtx, "Processing query", map[string]interface{}{"query": query})
		startTime := time.Now()

		response, err := agent.Run(queryCtx, query)
		if err != nil {
			logger.Error(queryCtx, "Error executing query", map[string]interface{}{"error": err.Error()})
			otelTracer.EndSpan(querySpan, err)
			continue
		}

		// Print the result
		duration := time.Since(startTime).Seconds()
		logger.Info(queryCtx, "Query processed successfully", map[string]interface{}{"duration_seconds": duration})
		logger.Info(queryCtx, "Response", map[string]interface{}{"response": response})

		// End the query span
		otelTracer.EndSpan(querySpan, nil)
	}
}
