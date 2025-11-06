package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
)

// MockSubAgentTool simulates a sub-agent tool
type MockSubAgentTool struct {
	name        string
	description string
}

func (m *MockSubAgentTool) Name() string {
	return m.name
}

func (m *MockSubAgentTool) Description() string {
	return m.description
}

func (m *MockSubAgentTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: "The query to send to the sub-agent",
			Required:    true,
		},
	}
}

func (m *MockSubAgentTool) Run(ctx context.Context, input string) (string, error) {
	return fmt.Sprintf("Response from %s: %s", m.name, input), nil
}

func (m *MockSubAgentTool) Execute(ctx context.Context, args string) (string, error) {
	return fmt.Sprintf("Response from %s: %s", m.name, args), nil
}

func main() {
	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create LLM client
	llm := openai.NewClient(apiKey)

	// Create conversation buffer memory
	bufferMemory := memory.NewConversationBuffer(
		memory.WithMaxSize(10),
	)

	// Create tools registry with mock sub-agent tools
	toolRegistry := tools.NewRegistry()

	// Add mock sub-agent tools
	researchAgent := &MockSubAgentTool{
		name:        "ResearchAgent_agent",
		description: "Research agent for gathering information",
	}
	analysisAgent := &MockSubAgentTool{
		name:        "AnalysisAgent_agent",
		description: "Analysis agent for data validation and pattern identification",
	}
	synthesisAgent := &MockSubAgentTool{
		name:        "SynthesisAgent_agent",
		description: "Synthesis agent for report generation with citations",
	}

	toolRegistry.Register(researchAgent)
	toolRegistry.Register(analysisAgent)
	toolRegistry.Register(synthesisAgent)

	// Get all tools from registry
	allTools := toolRegistry.List()

	// Create agent with tools
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithName("TestSubAgentsAgent"),
		agent.WithDescription("Test agent with sub-agent tools"),
		agent.WithSystemPrompt("You are a test agent with sub-agent tools available."),
		agent.WithMemory(bufferMemory),
		agent.WithTools(allTools...),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Create UI configuration
	uiConfig := &microservice.UIConfig{
		Enabled:     true,
		DefaultPath: "/",
		DevMode:     false,
		Theme:       "light",
		Features: microservice.UIFeatures{
			Chat:      true,
			Memory:    true,
			AgentInfo: true,
			Settings:  true,
		},
	}

	port := 8087

	// Create HTTP server with UI
	server := microservice.NewHTTPServerWithUI(myAgent, port, uiConfig)

	fmt.Printf("🚀 Starting sub-agents test server on http://localhost:%d\n", port)
	fmt.Printf("✅ %d tools available (including sub-agent tools)\n", len(allTools))
	fmt.Println("Sub-agent tools:")
	for _, tool := range allTools {
		fmt.Printf("  - %s: %s\n", tool.Name(), tool.Description())
	}
	fmt.Println("\n✅ Test the Sub-Agents tab to see detected sub-agents!")
	fmt.Println("✅ Check Tools tab to see all available tools!")
	fmt.Println("\nPress Ctrl+C to stop")

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
