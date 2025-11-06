package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/microservice"
)

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

	// Create a test agent with system prompt that has sub-agents (similar to your real agent)
	systemPrompt := `# Test Deep Research Lead Agent

You are a Test Deep Research Lead Agent that orchestrates research using specialized sub-agents.

## Available Sub-Agent Tools

### ResearchAgent_agent
- **Purpose**: Web search and information gathering, source validation, fact extraction with URL tracking
- **Usage**: ResearchAgent_agent tool with query parameter
- **When to use**: For gathering information from multiple sources on specific topics

### AnalysisAgent_agent
- **Purpose**: Data validation, pattern identification, insight extraction
- **Usage**: AnalysisAgent_agent tool with query parameter
- **When to use**: To analyze and validate information collected by research agents

### SynthesisAgent_agent
- **Purpose**: Report generation with in-text citations, reference management, executive summaries
- **Usage**: SynthesisAgent_agent tool with query parameter
- **When to use**: To create final comprehensive reports with proper citations and references

## Research Workflow
1. Use ResearchAgent_agent tool for gathering information
2. Use AnalysisAgent_agent tool to process research results
3. Use SynthesisAgent_agent tool to create final reports`

	// Create agent
	myAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithName("TestUIAgent"),
		agent.WithDescription("Test agent for UI demonstration with sub-agents in system prompt"),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithMemory(bufferMemory),
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

	// Use port 8086 to avoid conflicts
	port := 8086

	// Create HTTP server with UI
	server := microservice.NewHTTPServerWithUI(myAgent, port, uiConfig)

	fmt.Printf("🚀 Starting test UI server on http://localhost:%d\n", port)
	fmt.Println("This agent has sub-agents defined in the system prompt:")
	fmt.Println("  - ResearchAgent_agent")
	fmt.Println("  - AnalysisAgent_agent")
	fmt.Println("  - SynthesisAgent_agent")
	fmt.Println("\n✅ Test the Sub-Agents tab to see if they appear!")
	fmt.Println("✅ Check Tools tab to see if any tools are detected!")
	fmt.Println("\nPress Ctrl+C to stop")

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}