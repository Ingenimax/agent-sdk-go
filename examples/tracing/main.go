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

	logger.Info(ctx, "Starting OTEL-based Langfuse tracing example", nil)

	// Initialize the new OTEL-based Langfuse tracer directly
	otelLangfuseTracer, err := tracing.NewOTELLangfuseTracer(tracing.LangfuseConfig{
		Enabled:     true,
		SecretKey:   os.Getenv("LANGFUSE_SECRET_KEY"),
		PublicKey:   os.Getenv("LANGFUSE_PUBLIC_KEY"),
		Host:        getEnvOr("LANGFUSE_HOST", "https://cloud.langfuse.com"),
		Environment: getEnvOr("LANGFUSE_ENVIRONMENT", "development"),
	})
	if err != nil {
		logger.Error(ctx, "Failed to initialize OTEL Langfuse tracer", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer func() {
		if err := otelLangfuseTracer.Shutdown(); err != nil {
			logger.Error(ctx, "Failed to shutdown OTEL Langfuse tracer", map[string]interface{}{"error": err.Error()})
		}
	}()
	logger.Info(ctx, "OTEL-based Langfuse tracer initialized successfully", map[string]interface{}{
		"implementation": "OpenTelemetry",
		"protocol":       "OTLP HTTP",
		"benefits":       []string{"reliable", "production-ready", "standards-compliant"},
	})

	// This still works exactly the same but now uses OTEL internally!
	langfuseTracer, err := tracing.NewLangfuseTracer(tracing.LangfuseConfig{
		Enabled:     true,
		SecretKey:   os.Getenv("LANGFUSE_SECRET_KEY"),
		PublicKey:   os.Getenv("LANGFUSE_PUBLIC_KEY"),
		Host:        getEnvOr("LANGFUSE_HOST", "https://cloud.langfuse.com"),
		Environment: getEnvOr("LANGFUSE_ENVIRONMENT", "development"),
	})
	if err != nil {
		logger.Error(ctx, "Failed to initialize backward-compatible Langfuse tracer", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer func() {
		if err := langfuseTracer.Flush(); err != nil {
			logger.Error(ctx, "Failed to flush backward-compatible Langfuse tracer", map[string]interface{}{"error": err.Error()})
		}
	}()
	logger.Info(ctx, "Backward-compatible Langfuse tracer initialized (powered by OTEL internally)", nil)

	// Create base LLM client
	llm := openai.NewClient(os.Getenv("OPENAI_API_KEY"),
		openai.WithModel("gpt-4o-mini"),
		openai.WithLogger(logger),
	)

	// NEW APPROACH: Use OTEL-based LLM middleware directly
	llmWithOTELLangfuse := tracing.NewOTELLLMMiddleware(llm, otelLangfuseTracer)
	logger.Info(ctx, "LLM with OTEL-based Langfuse tracing created", nil)

	logger.Info(ctx, "Backward-compatible LLM middleware available (not used in this example)", nil)

	// Use the new OTEL-based approach for this example
	finalLLM := llmWithOTELLangfuse

	// Create Agent-compatible tracer using the new OTEL implementation
	agentTracer := tracing.NewOTELTracerAdapter(otelLangfuseTracer)
	logger.Info(ctx, "Agent tracer adapter created from OTEL Langfuse tracer", nil)

	// Create memory
	mem := memory.NewConversationBuffer()

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

	// Create agent with OTEL-based tracing
	agent, err := agent.NewAgent(
		agent.WithLLM(finalLLM),
		agent.WithMemory(mem),
		agent.WithTools(calcTool, searchTool),
		agent.WithTracer(agentTracer), // This enables comprehensive agent tracing
		agent.WithSystemPrompt("You are a helpful AI assistant with access to a calculator and web search. Be precise and helpful."),
		agent.WithOrgID("example-org"),
	)
	if err != nil {
		logger.Error(ctx, "Failed to create agent", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, "Agent created successfully with comprehensive OTEL-based tracing", map[string]interface{}{
		"llm_tracing":   "OTEL Langfuse",
		"agent_tracing": "OTEL Langfuse",
		"capabilities": []string{
			"LLM call tracing",
			"Agent execution tracing",
			"Tool usage tracing",
			"Error tracking",
			"Performance metrics",
		},
	})

	fmt.Println("\n🚀 Agent with OTEL-based Langfuse Tracing is ready!")
	fmt.Println("📊 All interactions will be traced to Langfuse using reliable OTEL infrastructure")
	fmt.Println("💡 Try queries like:")
	fmt.Println("   - 'Calculate 15 * 23 + 45'")
	fmt.Println("   - 'Search for latest news about AI'")
	fmt.Println("   - 'What is the weather in Tokyo?'")
	fmt.Println("   - 'exit' to quit")
	fmt.Println()

	// Handle user queries with comprehensive tracing
	conversationID := fmt.Sprintf("conv-%d", time.Now().UnixNano())
	logger.Info(ctx, "Starting interactive session", map[string]interface{}{"conversation_id": conversationID})

	for {
		// Get user input
		fmt.Print("You: ")
		reader := bufio.NewReader(os.Stdin)
		query, inputErr := reader.ReadString('\n')
		if inputErr != nil {
			logger.Error(ctx, "Error reading input", map[string]interface{}{"error": inputErr.Error()})
			continue
		}
		query = strings.TrimSpace(query)

		if query == "" {
			continue
		}
		if query == "exit" {
			logger.Info(ctx, "User requested exit", nil)
			break
		}

		// Process query with comprehensive tracing
		// This will automatically trace:
		// 1. Agent execution (via agentTracer)
		// 2. LLM calls (via llmWithOTELLangfuse)
		// 3. Tool usage (if any)
		// 4. Performance metrics
		// 5. Error handling
		logger.Info(ctx, "Processing query", map[string]interface{}{"query": query})
		startTime := time.Now()

		response, err := agent.Run(ctx, query)
		if err != nil {
			logger.Error(ctx, "Error executing query", map[string]interface{}{"error": err.Error()})
			fmt.Printf("Error: %v\n", err)
			continue
		}

		duration := time.Since(startTime)
		logger.Info(ctx, "Query processed successfully", map[string]interface{}{
			"duration_ms":     duration.Milliseconds(),
			"response_length": len(response),
		})

		fmt.Printf("Agent: %s\n\n", response)
	}

	// Flush all traces before exiting
	logger.Info(ctx, "Flushing traces before exit...", nil)
	if err := otelLangfuseTracer.Flush(); err != nil {
		logger.Error(ctx, "Failed to flush OTEL traces", map[string]interface{}{"error": err.Error()})
	}
	if err := langfuseTracer.Flush(); err != nil {
		logger.Error(ctx, "Failed to flush backward-compatible traces", map[string]interface{}{"error": err.Error()})
	}

	fmt.Println("👋 Goodbye! Check your Langfuse dashboard to see all the traced interactions.")
	logger.Info(ctx, "Session ended", map[string]interface{}{"total_traces_sent": "check_langfuse_dashboard"})
}

// Helper function to get environment variable with default
func getEnvOr(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
