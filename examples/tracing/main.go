package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
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

	logger.Info(ctx, "Starting unified tracing example", nil)

	// Determine which tracer to use based on environment variables
	var tracer interfaces.Tracer
	var tracerName string

	if os.Getenv("LANGFUSE_SECRET_KEY") != "" && os.Getenv("LANGFUSE_PUBLIC_KEY") != "" {
		// Use Langfuse tracing
		langfuseTracer, err := tracing.NewLangfuseTracer(tracing.LangfuseConfig{
			Enabled:     true,
			SecretKey:   os.Getenv("LANGFUSE_SECRET_KEY"),
			PublicKey:   os.Getenv("LANGFUSE_PUBLIC_KEY"),
			Host:        getEnvOr("LANGFUSE_HOST", "https://cloud.langfuse.com"),
			Environment: getEnvOr("LANGFUSE_ENVIRONMENT", "development"),
		})
		if err != nil {
			logger.Error(ctx, "Failed to initialize Langfuse tracer", map[string]interface{}{"error": err.Error()})
			os.Exit(1)
		}
		defer func() {
			if err := langfuseTracer.Flush(); err != nil {
				logger.Error(ctx, "Failed to flush Langfuse tracer", map[string]interface{}{"error": err.Error()})
			}
		}()
		tracer = langfuseTracer.AsInterfaceTracer()
		tracerName = "Langfuse"
		logger.Info(ctx, "Langfuse tracer initialized", nil)
	} else if os.Getenv("OTEL_COLLECTOR_ENDPOINT") != "" {
		// Use OpenTelemetry tracing
		otelTracer, err := tracing.NewOTelTracer(tracing.OTelConfig{
			Enabled:           true,
			ServiceName:       getEnvOr("SERVICE_NAME", "agent-sdk-example"),
			CollectorEndpoint: os.Getenv("OTEL_COLLECTOR_ENDPOINT"),
		})
		if err != nil {
			logger.Error(ctx, "Failed to initialize OpenTelemetry tracer", map[string]interface{}{"error": err.Error()})
			os.Exit(1)
		}
		tracer = otelTracer
		tracerName = "OpenTelemetry"
		logger.Info(ctx, "OpenTelemetry tracer initialized", nil)
	} else {
		logger.Error(ctx, "No tracing configuration found", map[string]interface{}{
			"langfuse_required": []string{"LANGFUSE_SECRET_KEY", "LANGFUSE_PUBLIC_KEY"},
			"otel_required":     []string{"OTEL_COLLECTOR_ENDPOINT"},
		})
		os.Exit(1)
	}

	// Create base LLM client
	llm := openai.NewClient(os.Getenv("OPENAI_API_KEY"),
		openai.WithModel("gpt-4o-mini"),
		openai.WithLogger(logger),
	)

	// Use unified tracing middleware
	tracedLLM := tracing.NewTracedLLM(llm, tracer)
	logger.Info(ctx, fmt.Sprintf("LLM with %s tracing created", tracerName), nil)

	// Create memory with tracing
	tracedMemory := tracing.NewTracedMemory(memory.NewConversationBuffer(), tracer)
	logger.Info(ctx, fmt.Sprintf("Memory with %s tracing created", tracerName), nil)

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

	// Create agent with unified tracing
	agent, err := agent.NewAgent(
		agent.WithLLM(tracedLLM),
		agent.WithMemory(tracedMemory),
		agent.WithTools(calcTool, searchTool),
		agent.WithTracer(tracer), // This enables comprehensive agent tracing
		agent.WithSystemPrompt("You are a helpful AI assistant with access to a calculator and web search. Be precise and helpful."),
		agent.WithOrgID("example-org"),
		agent.WithRequirePlanApproval(false),
	)
	if err != nil {
		logger.Error(ctx, "Failed to create agent", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info(ctx, fmt.Sprintf("Agent created successfully with comprehensive %s tracing", tracerName), map[string]interface{}{
		"tracer_type":    tracerName,
		"llm_tracing":    tracerName,
		"memory_tracing": tracerName,
		"agent_tracing":  tracerName,
		"capabilities": []string{
			"LLM call tracing",
			"Memory operation tracing",
			"Agent execution tracing",
			"Tool usage tracing",
			"Error tracking",
			"Performance metrics",
		},
	})

	fmt.Printf("\n🚀 Agent with %s Tracing is ready!\n", tracerName)
	if tracerName == "Langfuse" {
		fmt.Println("📊 All interactions will be traced to Langfuse")
	} else {
		fmt.Println("📊 All interactions will be traced to OpenTelemetry collector")
	}
	fmt.Println("💡 Try queries like:")
	fmt.Println("   - 'Calculate 15 * 23 + 45'")
	fmt.Println("   - 'Search for latest news about AI'")
	fmt.Println("   - 'What is the weather in Tokyo?'")
	fmt.Println("   - 'exit' to quit")
	fmt.Println()

	// Handle user queries with comprehensive tracing
	conversationID := fmt.Sprintf("conv-%d", time.Now().UnixNano())

	ctx = memory.WithConversationID(ctx, conversationID)
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

	fmt.Printf("👋 Goodbye! Check your %s dashboard to see all the traced interactions.\n", tracerName)
	logger.Info(ctx, "Session ended", map[string]interface{}{
		"tracer_type":       tracerName,
		"total_traces_sent": fmt.Sprintf("check_%s_dashboard", strings.ToLower(tracerName)),
	})
}

// Helper function to get environment variable with default
func getEnvOr(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
