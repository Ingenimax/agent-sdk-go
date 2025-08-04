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
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

func main() {
	// Create logger with debug level to see sub-agent tracing
	logger := logging.New()
	debugOption := logging.WithLevel("debug")
	debugOption(logger)

	// Create context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Check for API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		logger.Error(ctx, "OPENAI_API_KEY environment variable is not set", nil)
		os.Exit(1)
	}

	// Create LLM client
	llmClient := openai.NewClient(apiKey, openai.WithLogger(logger))

	// Create specialized sub-agents
	mathAgent, err := createMathAgent(llmClient, logger)
	if err != nil {
		logger.Error(ctx, "Failed to create math agent", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}

	researchAgent, err := createResearchAgent(llmClient, logger)
	if err != nil {
		logger.Error(ctx, "Failed to create research agent", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}

	codeAgent, err := createCodeAgent(llmClient, logger)
	if err != nil {
		logger.Error(ctx, "Failed to create code agent", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}

	// Create main agent with sub-agents
	mainAgent, err := createMainAgent(llmClient, logger, mathAgent, researchAgent, codeAgent)
	if err != nil {
		logger.Error(ctx, "Failed to create main agent", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}

	logger.Info(ctx, "Sub-agents example initialized successfully!", nil)
	logger.Info(ctx, "Main agent has access to the following sub-agents:", map[string]interface{}{
		"math_agent":     "Handles mathematical calculations and problems",
		"research_agent": "Performs research and information retrieval",
		"code_agent":     "Assists with code-related tasks",
	})

	// Interactive loop
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("\nEnter your query (or 'exit' to quit): ")
		query, _ := reader.ReadString('\n')
		query = strings.TrimSpace(query)

		if query == "exit" {
			break
		}

		if query == "" {
			continue
		}

		// Add required context for multi-tenancy and memory
		queryCtx := multitenancy.WithOrgID(ctx, "default-org")
		queryCtx = memory.WithConversationID(queryCtx, "default-conversation")

		// Process query
		logger.Info(ctx, "Processing query...", map[string]interface{}{"query": query})

		result, err := mainAgent.Run(queryCtx, query)
		if err != nil {
			logger.Error(ctx, "Error processing query", map[string]interface{}{"error": err.Error()})
			continue
		}

		fmt.Printf("\nResponse: %s\n", result)
	}

	logger.Info(ctx, "Goodbye!", nil)
}

func createMathAgent(llm interfaces.LLM, logger logging.Logger) (*agent.Agent, error) {
	// Create calculator tool
	calc := calculator.New()

	// Create memory
	mem := memory.NewConversationBuffer()

	// Create math agent
	return agent.NewAgent(
		agent.WithName("MathAgent"),
		agent.WithDescription("Specialized in mathematical calculations, solving equations, and numerical analysis"),
		agent.WithLLM(llm),
		agent.WithMemory(mem),
		agent.WithTools(calc),
		agent.WithOrgID("default-org"),
		agent.WithRequirePlanApproval(false), // No plan approval needed for tool calls
		agent.WithSystemPrompt(`You are a specialized mathematics agent. You excel at:
- Solving mathematical equations and problems
- Performing complex calculations
- Statistical analysis
- Numerical computations
- Mathematical reasoning and proofs

Use your calculator tool when needed for precise calculations.
Provide clear, step-by-step solutions when solving problems.`),
	)
}

func createResearchAgent(llm interfaces.LLM, logger logging.Logger) (*agent.Agent, error) {
	// Create web search tool (requires API keys)
	var searchTool interfaces.Tool
	googleAPIKey := os.Getenv("GOOGLE_API_KEY")
	searchEngineID := os.Getenv("GOOGLE_SEARCH_ENGINE_ID")

	if googleAPIKey != "" && searchEngineID != "" {
		searchTool = websearch.New(googleAPIKey, searchEngineID)
	}

	// Create memory
	mem := memory.NewConversationBuffer()

	// Create research agent
	agentOpts := []agent.Option{
		agent.WithName("ResearchAgent"),
		agent.WithDescription("Specialized in research, fact-finding, and information retrieval from various sources"),
		agent.WithLLM(llm),
		agent.WithMemory(mem),
		agent.WithOrgID("default-org"),
		agent.WithRequirePlanApproval(false), // No plan approval needed for tool calls
		agent.WithSystemPrompt(`You are a specialized research agent. You excel at:
- Finding and verifying information
- Conducting thorough research on topics
- Fact-checking and validation
- Summarizing complex information
- Providing accurate, up-to-date information

When available, use search tools to find current information.
Always cite your sources when possible.`),
	}

	// Add search tool if available
	if searchTool != nil {
		agentOpts = append(agentOpts, agent.WithTools(searchTool))
	}

	return agent.NewAgent(agentOpts...)
}

func createCodeAgent(llm interfaces.LLM, logger logging.Logger) (*agent.Agent, error) {
	// Create memory
	mem := memory.NewConversationBuffer()

	// Create code agent
	return agent.NewAgent(
		agent.WithName("CodeAgent"),
		agent.WithDescription("Specialized in programming, code analysis, debugging, and software development"),
		agent.WithLLM(llm),
		agent.WithMemory(mem),
		agent.WithOrgID("default-org"),
		agent.WithSystemPrompt(`You are a specialized code agent. You excel at:
- Writing clean, efficient code in various programming languages
- Debugging and fixing code issues
- Code review and optimization
- Explaining complex programming concepts
- Suggesting best practices and design patterns
- Creating documentation and tests

Provide well-commented, production-ready code.
Always consider error handling and edge cases.`),
	)
}

func createMainAgent(llm interfaces.LLM, logger logging.Logger, subAgents ...*agent.Agent) (*agent.Agent, error) {
	// Create memory
	mem := memory.NewConversationBuffer()

	// Build sub-agent descriptions for the system prompt
	var subAgentDescriptions []string
	for _, subAgent := range subAgents {
		subAgentDescriptions = append(subAgentDescriptions,
			fmt.Sprintf("- %s: %s", subAgent.GetName(), subAgent.GetDescription()))
	}

	systemPrompt := fmt.Sprintf(`You are a versatile AI assistant with access to specialized sub-agents for specific tasks.

Your available sub-agents are:
%s

When you encounter a task that would benefit from specialized expertise, delegate it to the appropriate sub-agent:
- Use MathAgent for mathematical calculations, equations, or numerical analysis
- Use ResearchAgent for finding information, fact-checking, or research tasks
- Use CodeAgent for programming, debugging, or software development tasks

You can handle general queries yourself, but leverage your sub-agents when their expertise would provide better results.
Always explain which agent you're using and why when delegating tasks.`,
		strings.Join(subAgentDescriptions, "\n"))

	// Create main agent with sub-agents
	return agent.NewAgent(
		agent.WithName("MainAgent"),
		agent.WithDescription("Main orchestrator agent that delegates to specialized sub-agents"),
		agent.WithLLM(llm),
		agent.WithMemory(mem),
		agent.WithOrgID("default-org"),
		agent.WithAgents(subAgents...),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithRequirePlanApproval(false), // No plan approval needed for sub-agent calls
		agent.WithMaxIterations(3),           // Allow up to 3 tool calls (sub-agent invocations)
	)
}
