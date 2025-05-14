package main

import (
	"bufio"
	"context"
	"os"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/orchestration"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

// Define custom context key types to avoid using string literals
type userIDKey struct{}

func main() {
	// Create a logger with debug level
	baseLogger := logging.New()
	debugOption := logging.WithLevel("debug")
	debugOption(baseLogger)
	logger := baseLogger

	// Create context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Check for required API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		logger.Error(ctx, "OPENAI_API_KEY environment variable is not set", nil)
		logger.Info(ctx, "Please set it with: export OPENAI_API_KEY=your_openai_api_key", nil)
		os.Exit(1)
	}

	// Create LLM client
	openaiClient := openai.NewClient(apiKey, openai.WithLogger(logger))

	// Test the API key with a simple query
	logger.Info(ctx, "Testing OpenAI API key...", nil)
	_, err := openaiClient.Generate(ctx, "Hello")
	if err != nil {
		logger.Error(ctx, "Failed to validate OpenAI API key", map[string]interface{}{"error": err.Error()})
		logger.Info(ctx, "Please check that your API key is valid and has sufficient quota.", nil)
		os.Exit(1)
	}
	logger.Info(ctx, "API key is valid!", nil)

	// Create agent registry
	registry := orchestration.NewAgentRegistry()

	// Create general agent
	generalAgent, err := createGeneralAgent(openaiClient)
	if err != nil {
		logger.Error(ctx, "Failed to create general agent", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	registry.Register("general", generalAgent)

	// Create research agent
	researchAgent, err := createResearchAgent(openaiClient)
	if err != nil {
		logger.Error(ctx, "Failed to create research agent", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	registry.Register("research", researchAgent)

	// Create math agent
	mathAgent, err := createMathAgent(openaiClient)
	if err != nil {
		logger.Error(ctx, "Failed to create math agent", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	registry.Register("math", mathAgent)

	// Create router
	router := orchestration.NewLLMRouter(openaiClient)
	router.WithLogger(logger)

	// Create orchestrator
	orchestrator := orchestration.NewOrchestrator(registry, router)
	orchestrator.WithLogger(logger)

	// Add required IDs to context
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "default-conversation")
	ctx = context.WithValue(ctx, userIDKey{}, "default-user")

	// Handle user queries
	for {
		// Get user input
		logger.Info(ctx, "Enter your query (or 'exit' to quit):", nil)
		var query string
		// Use bufio.NewReader to read the entire line including spaces
		reader := bufio.NewReader(os.Stdin)
		query, _ = reader.ReadString('\n')
		query = strings.TrimSpace(query)

		if query == "exit" {
			break
		}

		// Prepare context for routing
		routingContext := map[string]interface{}{
			"agents": map[string]string{
				"general":  "General-purpose assistant for everyday questions and tasks",
				"research": "Specialized in research, fact-finding, and information retrieval",
				"math":     "Specialized in mathematical calculations and problem-solving",
			},
		}

		// Handle the request
		logger.Info(ctx, "Processing your request...", nil)
		result, err := orchestrator.HandleRequest(ctx, query, routingContext)
		if err != nil {
			logger.Error(ctx, "Error processing request", map[string]interface{}{"error": err.Error()})

			// Check for common error types and provide helpful messages
			errStr := err.Error()
			if strings.Contains(errStr, "401 Unauthorized") {
				logger.Info(ctx, "API key error detected. Please check that:", nil)
				logger.Info(ctx, "1. Your OpenAI API key is correctly set in the environment", nil)
				logger.Info(ctx, "2. The API key is valid and not expired", nil)
				logger.Info(ctx, "3. Your account has sufficient credits", nil)
				logger.Info(ctx, "You can verify your API key with: echo $OPENAI_API_KEY", nil)
			} else if strings.Contains(errStr, "context deadline exceeded") || strings.Contains(errStr, "timeout") {
				logger.Info(ctx, "The request timed out. This could be due to:", nil)
				logger.Info(ctx, "1. OpenAI API service being slow or unavailable", nil)
				logger.Info(ctx, "2. A complex query requiring too much processing time", nil)
				logger.Info(ctx, "3. Network connectivity issues", nil)
			}

			continue
		}

		// Print the result
		logger.Info(ctx, "Agent response", map[string]interface{}{
			"agent_id": result.AgentID,
			"response": result.Response,
		})
	}
}

func createGeneralAgent(llm interfaces.LLM) (*agent.Agent, error) {
	// Create memory
	mem := memory.NewConversationBuffer()

	// Create agent
	agent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(mem),
		agent.WithSystemPrompt(`You are a helpful general-purpose assistant. You can answer questions on a wide range of topics.
If you encounter a question that requires specialized knowledge in research or mathematics, you should hand off to a specialized agent.

Hand off to the research agent for questions that:
- Require up-to-date information or facts
- Need detailed information about specific topics
- Would benefit from web searches or data retrieval
- Ask about current events, statistics, or factual information

Hand off to the math agent for questions that:
- Involve calculations of any kind
- Require solving equations or mathematical problems
- Deal with financial calculations, interest rates, or statistics
- Need precise numerical answers

To hand off to the research agent, respond with: [HANDOFF:research:needs specialized research]
To hand off to the math agent, respond with: [HANDOFF:math:needs mathematical calculation]

Otherwise, provide helpful and accurate responses to the user's questions.`),
	)
	if err != nil {
		return nil, err
	}

	return agent, nil
}

func createResearchAgent(llm interfaces.LLM) (*agent.Agent, error) {
	// Create memory
	mem := memory.NewConversationBuffer()

	// Create tools
	toolRegistry := tools.NewRegistry()
	searchTool := websearch.New(
		os.Getenv("GOOGLE_API_KEY"),
		os.Getenv("GOOGLE_SEARCH_ENGINE_ID"),
	)
	toolRegistry.Register(searchTool)

	// Create agent
	agent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(mem),
		agent.WithTools(toolRegistry.List()...),
		agent.WithSystemPrompt(`You are a specialized research agent. You excel at finding information and answering factual questions.
You have access to search tools to help you find information.

If you encounter a question that requires mathematical calculation, you should hand off to the math agent.
This includes questions about:
- Calculating interest, compound interest, or financial calculations
- Solving equations or algebraic problems
- Statistical analysis or probability calculations
- Geometric calculations or any other mathematical operations

To hand off to the math agent, respond with: [HANDOFF:math:needs mathematical calculation]

Otherwise, use your tools to research and provide accurate information to the user.`),
	)
	if err != nil {
		return nil, err
	}

	return agent, nil
}

func createMathAgent(llm interfaces.LLM) (*agent.Agent, error) {
	// Create memory
	mem := memory.NewConversationBuffer()

	// Create tools
	toolRegistry := tools.NewRegistry()
	calcTool := calculator.New()
	toolRegistry.Register(calcTool)

	// Create agent
	agent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(mem),
		agent.WithTools(toolRegistry.List()...),
		agent.WithSystemPrompt(`You are a specialized math agent. You excel at solving mathematical problems and performing calculations.
You have access to a calculator tool to help you solve complex problems.

If you encounter a question that requires research or factual information, you should hand off to the research agent.
To hand off to the research agent, respond with: [HANDOFF:research:needs specialized research]

Otherwise, use your mathematical expertise and tools to solve problems for the user.`),
	)
	if err != nil {
		return nil, err
	}

	return agent, nil
}
