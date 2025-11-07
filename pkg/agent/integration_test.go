package agent

import (
	"os"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

func TestSubAgentsIntegration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	llm := openai.NewClient(apiKey)

	// Create a math sub-agent
	mathAgent, err := NewAgent(
		WithName("MathAgent"),
		WithDescription("Expert in mathematical calculations and numerical analysis"),
		WithLLM(llm),
		WithMemory(memory.NewConversationBuffer()),
		WithTools(calculator.New()),
		WithSystemPrompt("You are a math expert. Solve mathematical problems accurately."),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("Failed to create math agent: %v", err)
	}

	// Create a general sub-agent
	generalAgent, err := NewAgent(
		WithName("GeneralAgent"),
		WithDescription("General knowledge assistant for everyday questions"),
		WithLLM(llm),
		WithMemory(memory.NewConversationBuffer()),
		WithSystemPrompt("You are a helpful general assistant."),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("Failed to create general agent: %v", err)
	}

	// Create main agent with sub-agents
	mainAgent, err := NewAgent(
		WithName("MainAgent"),
		WithLLM(llm),
		WithMemory(memory.NewConversationBuffer()),
		WithAgents(mathAgent, generalAgent),
		WithSystemPrompt(`You are a main orchestrator with access to specialized sub-agents.
		Use MathAgent_agent for mathematical calculations.
		Use GeneralAgent_agent for general questions.
		Delegate tasks appropriately based on the query.`),
		WithMaxIterations(2),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("Failed to create main agent: %v", err)
	}

	// Test that sub-agents were properly registered
	if len(mainAgent.GetSubAgents()) != 2 {
		t.Errorf("Expected 2 sub-agents, got %d", len(mainAgent.GetSubAgents()))
	}

	// Test sub-agent retrieval
	if !mainAgent.HasSubAgent("MathAgent") {
		t.Error("Expected to find MathAgent")
	}

	if !mainAgent.HasSubAgent("GeneralAgent") {
		t.Error("Expected to find GeneralAgent")
	}
}
