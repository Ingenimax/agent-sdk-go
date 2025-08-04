package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// TestMockLLM is a mock implementation of the LLM interface for testing
type TestMockLLM struct {
	llmName string
}

func (m *TestMockLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	return "mock response", nil
}

func (m *TestMockLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	return "mock response with tools", nil
}

func (m *TestMockLLM) Name() string {
	return m.llmName
}

func TestWithAgents(t *testing.T) {
	// Create mock LLMs
	mainLLM := &TestMockLLM{llmName: "main"}
	subLLM1 := &TestMockLLM{llmName: "sub1"}
	subLLM2 := &TestMockLLM{llmName: "sub2"}

	// Create sub-agents
	subAgent1, err := NewAgent(
		WithName("MathAgent"),
		WithDescription("Handles mathematical calculations"),
		WithLLM(subLLM1),
	)
	if err != nil {
		t.Fatalf("Failed to create sub-agent 1: %v", err)
	}

	subAgent2, err := NewAgent(
		WithName("ResearchAgent"),
		WithDescription("Handles research and information retrieval"),
		WithLLM(subLLM2),
	)
	if err != nil {
		t.Fatalf("Failed to create sub-agent 2: %v", err)
	}

	// Create main agent with sub-agents
	mainAgent, err := NewAgent(
		WithName("MainAgent"),
		WithLLM(mainLLM),
		WithAgents(subAgent1, subAgent2),
	)
	if err != nil {
		t.Fatalf("Failed to create main agent: %v", err)
	}

	// Verify sub-agents were added
	if len(mainAgent.subAgents) != 2 {
		t.Errorf("Expected 2 sub-agents, got %d", len(mainAgent.subAgents))
	}

	// Verify sub-agents were wrapped as tools
	if len(mainAgent.tools) != 2 {
		t.Errorf("Expected 2 tools from sub-agents, got %d", len(mainAgent.tools))
	}

	// Verify tool names
	expectedToolNames := []string{"MathAgent_agent", "ResearchAgent_agent"}
	for i, tool := range mainAgent.tools {
		if tool.Name() != expectedToolNames[i] {
			t.Errorf("Expected tool name %s, got %s", expectedToolNames[i], tool.Name())
		}
	}
}

func TestCircularDependencyDetection(t *testing.T) {
	// Create mock LLMs
	llm1 := &TestMockLLM{llmName: "llm1"}
	llm2 := &TestMockLLM{llmName: "llm2"}
	llm3 := &TestMockLLM{llmName: "llm3"}

	// Create agent 1
	agent1, err := NewAgent(
		WithName("Agent1"),
		WithLLM(llm1),
	)
	if err != nil {
		t.Fatalf("Failed to create agent 1: %v", err)
	}

	// Create agent 2 with agent 1 as sub-agent
	agent2, err := NewAgent(
		WithName("Agent2"),
		WithLLM(llm2),
		WithAgents(agent1),
	)
	if err != nil {
		t.Fatalf("Failed to create agent 2: %v", err)
	}

	// Try to add agent 2 as a sub-agent to agent 1 (circular dependency)
	agent1.subAgents = append(agent1.subAgents, agent2)

	// Validate should detect circular dependency
	err = agent1.validateSubAgents()
	if err == nil {
		t.Error("Expected circular dependency error, got nil")
	}

	// Create agent 3 without circular dependency
	_, err = NewAgent(
		WithName("Agent3"),
		WithLLM(llm3),
		WithAgents(agent1),
	)
	if err == nil {
		t.Error("Expected error due to circular dependency in sub-agents")
	}
}

func TestSubAgentRetrieval(t *testing.T) {
	// Create mock LLMs
	mainLLM := &TestMockLLM{llmName: "main"}
	subLLM := &TestMockLLM{llmName: "sub"}

	// Create sub-agent
	subAgent, err := NewAgent(
		WithName("MathAgent"),
		WithLLM(subLLM),
	)
	if err != nil {
		t.Fatalf("Failed to create sub-agent: %v", err)
	}

	// Create main agent with sub-agent
	mainAgent, err := NewAgent(
		WithName("MainAgent"),
		WithLLM(mainLLM),
		WithAgents(subAgent),
	)
	if err != nil {
		t.Fatalf("Failed to create main agent: %v", err)
	}

	// Test HasSubAgent
	if !mainAgent.HasSubAgent("MathAgent") {
		t.Error("Expected to find MathAgent as sub-agent")
	}

	if mainAgent.HasSubAgent("NonExistentAgent") {
		t.Error("Should not find non-existent agent")
	}

	// Test GetSubAgent
	retrieved, found := mainAgent.GetSubAgent("MathAgent")
	if !found {
		t.Error("Expected to find MathAgent")
	}
	if retrieved.name != "MathAgent" {
		t.Errorf("Expected agent name MathAgent, got %s", retrieved.name)
	}

	// Test GetSubAgents
	subAgents := mainAgent.GetSubAgents()
	if len(subAgents) != 1 {
		t.Errorf("Expected 1 sub-agent, got %d", len(subAgents))
	}
}

func TestMaxDepthValidation(t *testing.T) {
	// Create a chain of agents that exceeds max depth
	var agents []*Agent
	
	for i := 0; i < 10; i++ {
		llm := &TestMockLLM{llmName: fmt.Sprintf("llm%d", i)}
		agent, err := NewAgent(
			WithName(fmt.Sprintf("Agent%d", i)),
			WithLLM(llm),
		)
		if err != nil {
			t.Fatalf("Failed to create agent %d: %v", i, err)
		}
		
		// Add previous agent as sub-agent
		if i > 0 {
			agent.subAgents = append(agent.subAgents, agents[i-1])
		}
		
		agents = append(agents, agent)
	}
	
	// Validate the deepest agent (should exceed max depth)
	err := validateAgentTree(agents[9], 5)
	if err == nil {
		t.Error("Expected max depth validation error")
	}
}

func TestContextManagement(t *testing.T) {
	ctx := context.Background()
	
	// Test initial recursion depth
	depth := GetRecursionDepth(ctx)
	if depth != 0 {
		t.Errorf("Expected initial depth 0, got %d", depth)
	}
	
	// Add sub-agent context
	ctx = WithSubAgentContext(ctx, "MainAgent", "SubAgent1")
	
	// Test updated depth
	depth = GetRecursionDepth(ctx)
	if depth != 1 {
		t.Errorf("Expected depth 1, got %d", depth)
	}
	
	// Test sub-agent name
	name := GetSubAgentName(ctx)
	if name != "SubAgent1" {
		t.Errorf("Expected sub-agent name SubAgent1, got %s", name)
	}
	
	// Test parent agent
	parent := GetParentAgent(ctx)
	if parent != "MainAgent" {
		t.Errorf("Expected parent agent MainAgent, got %s", parent)
	}
	
	// Test IsSubAgentCall
	if !IsSubAgentCall(ctx) {
		t.Error("Expected IsSubAgentCall to return true")
	}
	
	// Test recursion depth validation
	for i := 0; i < MaxRecursionDepth; i++ {
		ctx = WithSubAgentContext(ctx, fmt.Sprintf("Agent%d", i), fmt.Sprintf("Agent%d", i+1))
	}
	
	err := ValidateRecursionDepth(ctx)
	if err == nil {
		t.Error("Expected recursion depth validation error")
	}
}

func TestAgentToolWrapper(t *testing.T) {
	// This test would require the tools package to be available
	// For now, we'll create a basic test structure
	t.Skip("Skipping AgentTool wrapper test - requires tools package integration")
}