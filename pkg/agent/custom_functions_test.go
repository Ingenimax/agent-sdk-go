package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Mock LLM for testing
type mockLLM struct {
	generateFunc func(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error)
	name         string
}

func (m *mockLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, prompt, options...)
	}
	return "mock response", nil
}

func (m *mockLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	return m.Generate(ctx, prompt, options...)
}

func (m *mockLLM) Name() string {
	if m.name == "" {
		return "mock-llm"
	}
	return m.name
}

func (m *mockLLM) SupportsStreaming() bool {
	return false
}

// Mock tool for testing
type mockTool struct {
	name        string
	description string
	runFunc     func(ctx context.Context, input string) (string, error)
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) DisplayName() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) Internal() bool {
	return false
}

func (m *mockTool) Run(ctx context.Context, input string) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, input)
	}
	return fmt.Sprintf("tool %s executed with: %s", m.name, input), nil
}

func (m *mockTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"input": {
			Type:        "string",
			Description: "Input text",
			Required:    true,
		},
	}
}

func (m *mockTool) Execute(ctx context.Context, args string) (string, error) {
	return m.Run(ctx, args)
}

func TestWithCustomRunFunction(t *testing.T) {
	// Test that WithCustomRunFunction option sets the custom function
	customFunc := func(ctx context.Context, input string, agent *Agent) (string, error) {
		return "custom result", nil
	}

	option := WithCustomRunFunction(customFunc)
	agent := &Agent{}
	option(agent)

	if agent.customRunFunc == nil {
		t.Error("Expected customRunFunc to be set")
	}
}

func TestWithCustomRunStreamFunction(t *testing.T) {
	// Test that WithCustomRunStreamFunction option sets the custom stream function
	customStreamFunc := func(ctx context.Context, input string, agent *Agent) (<-chan interfaces.AgentStreamEvent, error) {
		ch := make(chan interfaces.AgentStreamEvent, 1)
		ch <- interfaces.AgentStreamEvent{Type: interfaces.AgentEventComplete}
		close(ch)
		return ch, nil
	}

	option := WithCustomRunStreamFunction(customStreamFunc)
	agent := &Agent{}
	option(agent)

	if agent.customRunStreamFunc == nil {
		t.Error("Expected customRunStreamFunc to be set")
	}
}

func TestAgentRunWithCustomFunction(t *testing.T) {
	// Create a custom function that transforms input
	customFunc := func(ctx context.Context, input string, agent *Agent) (string, error) {
		// Use agent components
		logger := agent.GetLogger()
		mem := agent.GetMemory()

		if logger != nil {
			logger.Info(ctx, "Custom function called", map[string]interface{}{
				"input": input,
			})
		}

		result := fmt.Sprintf("CUSTOM[%s]", strings.ToUpper(input))

		// Add to memory if available
		if mem != nil {
			if err := mem.AddMessage(ctx, interfaces.Message{
				Role:    "user",
				Content: input,
			}); err != nil {
				return "", fmt.Errorf("failed to add user message to memory: %w", err)
			}
			if err := mem.AddMessage(ctx, interfaces.Message{
				Role:    "assistant",
				Content: result,
			}); err != nil {
				return "", fmt.Errorf("failed to add assistant message to memory: %w", err)
			}
		}

		return result, nil
	}

	// Create agent with custom function
	llm := &mockLLM{}
	mem := memory.NewConversationBuffer()
	logger := logging.New()

	agent, err := NewAgent(
		WithLLM(llm),
		WithMemory(mem),
		WithLogger(logger),
		WithCustomRunFunction(customFunc),
		WithName("TestAgent"),
	)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	// Test the custom function
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "test-org")
	ctx = memory.WithConversationID(ctx, "test-conversation")
	result, err := agent.Run(ctx, "hello world")
	if err != nil {
		t.Fatalf("Failed to run agent: %v", err)
	}

	expected := "CUSTOM[HELLO WORLD]"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	// Verify memory was used
	messages, err := mem.GetMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("Expected 2 messages in memory, got %d", len(messages))
	}

	if messages[0].Role != "user" || messages[0].Content != "hello world" {
		t.Errorf("Unexpected user message: %+v", messages[0])
	}

	if messages[1].Role != "assistant" || messages[1].Content != expected {
		t.Errorf("Unexpected assistant message: %+v", messages[1])
	}
}

func TestAgentRunWithoutCustomFunction(t *testing.T) {
	// Test that agent falls back to default behavior when no custom function is set
	llm := &mockLLM{
		generateFunc: func(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
			return "default LLM response", nil
		},
	}

	agent, err := NewAgent(
		WithLLM(llm),
		WithMemory(memory.NewConversationBuffer()),
		WithName("TestAgent"),
		WithSystemPrompt("You are a test agent"),
	)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "test-org")
	ctx = memory.WithConversationID(ctx, "test-conversation")
	result, err := agent.Run(ctx, "test input")
	if err != nil {
		t.Fatalf("Failed to run agent: %v", err)
	}

	// Should get the default LLM response
	if result != "default LLM response" {
		t.Errorf("Expected 'default LLM response', got %s", result)
	}
}

func TestAgentRunStreamWithCustomFunction(t *testing.T) {
	// Create a custom streaming function
	customStreamFunc := func(ctx context.Context, input string, agent *Agent) (<-chan interfaces.AgentStreamEvent, error) {
		eventChan := make(chan interfaces.AgentStreamEvent, 10)

		go func() {
			defer close(eventChan)

			// Send thinking event
			eventChan <- interfaces.AgentStreamEvent{
				Type:         interfaces.AgentEventThinking,
				ThinkingStep: "Processing input...",
				Timestamp:    time.Now(),
			}

			// Process words
			words := strings.Fields(input)
			for i, word := range words {
				eventChan <- interfaces.AgentStreamEvent{
					Type:      interfaces.AgentEventContent,
					Content:   fmt.Sprintf("Word %d: %s\n", i+1, strings.ToUpper(word)),
					Timestamp: time.Now(),
				}
			}

			// Send completion
			eventChan <- interfaces.AgentStreamEvent{
				Type:      interfaces.AgentEventComplete,
				Timestamp: time.Now(),
				Metadata: map[string]interface{}{
					"words_processed": len(words),
				},
			}
		}()

		return eventChan, nil
	}

	// Create agent with custom streaming function
	llm := &mockLLM{}
	agent, err := NewAgent(
		WithLLM(llm),
		WithCustomRunStreamFunction(customStreamFunc),
		WithName("StreamTestAgent"),
	)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	// Test streaming
	ctx := context.Background()
	eventChan, err := agent.RunStream(ctx, "hello world")
	if err != nil {
		t.Fatalf("Failed to start streaming: %v", err)
	}

	var events []interfaces.AgentStreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	// Should have: thinking + 2 content events + complete = 4 events
	expectedEvents := 4
	if len(events) != expectedEvents {
		t.Errorf("Expected %d events, got %d", expectedEvents, len(events))
	}

	// Check event types
	if events[0].Type != interfaces.AgentEventThinking {
		t.Errorf("Expected first event to be thinking, got %s", events[0].Type)
	}

	if events[1].Type != interfaces.AgentEventContent {
		t.Errorf("Expected second event to be content, got %s", events[1].Type)
	}

	if events[len(events)-1].Type != interfaces.AgentEventComplete {
		t.Errorf("Expected last event to be complete, got %s", events[len(events)-1].Type)
	}

	// Check metadata on completion event
	completeEvent := events[len(events)-1]
	if completeEvent.Metadata == nil {
		t.Error("Expected metadata on complete event")
	} else if wordsProcessed, ok := completeEvent.Metadata["words_processed"]; !ok || wordsProcessed != 2 {
		t.Errorf("Expected words_processed to be 2, got %v", wordsProcessed)
	}
}

func TestCustomFunctionWithAgentComponents(t *testing.T) {
	// Test that custom function can access all agent components
	var capturedLLM interfaces.LLM
	var capturedMemory interfaces.Memory
	var capturedTools []interfaces.Tool
	var capturedLogger logging.Logger
	var capturedSystemPrompt string

	customFunc := func(ctx context.Context, input string, agent *Agent) (string, error) {
		// Capture all agent components
		capturedLLM = agent.GetLLM()
		capturedMemory = agent.GetMemory()
		capturedTools = agent.GetTools()
		capturedLogger = agent.GetLogger()
		capturedSystemPrompt = agent.GetSystemPrompt()

		// Use the LLM
		if capturedLLM != nil {
			llmResult, err := capturedLLM.Generate(ctx, "test")
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("LLM says: %s", llmResult), nil
		}

		return "no LLM available", nil
	}

	// Create agent with all components
	llm := &mockLLM{name: "test-llm"}
	mem := memory.NewConversationBuffer()
	logger := logging.New()
	tool := &mockTool{name: "test-tool", description: "A test tool"}

	agent, err := NewAgent(
		WithLLM(llm),
		WithMemory(mem),
		WithLogger(logger),
		WithTools(tool),
		WithSystemPrompt("Test system prompt"),
		WithCustomRunFunction(customFunc),
		WithName("ComponentTestAgent"),
	)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	// Run the agent
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "test-org")
	result, err := agent.Run(ctx, "test input")
	if err != nil {
		t.Fatalf("Failed to run agent: %v", err)
	}

	// Verify all components were accessible
	if capturedLLM == nil {
		t.Error("LLM was not accessible in custom function")
	} else if capturedLLM.Name() != "test-llm" {
		t.Errorf("Expected LLM name 'test-llm', got %s", capturedLLM.Name())
	}

	if capturedMemory == nil {
		t.Error("Memory was not accessible in custom function")
	}

	if capturedLogger == nil {
		t.Error("Logger was not accessible in custom function")
	}

	if len(capturedTools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(capturedTools))
	} else if capturedTools[0].Name() != "test-tool" {
		t.Errorf("Expected tool name 'test-tool', got %s", capturedTools[0].Name())
	}

	if capturedSystemPrompt != "Test system prompt" {
		t.Errorf("Expected system prompt 'Test system prompt', got %s", capturedSystemPrompt)
	}

	// Verify the result
	expected := "LLM says: mock response"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestCustomFunctionError(t *testing.T) {
	// Test that errors from custom functions are properly propagated
	customFunc := func(ctx context.Context, input string, agent *Agent) (string, error) {
		return "", fmt.Errorf("custom function error: %s", input)
	}

	llm := &mockLLM{}
	agent, err := NewAgent(
		WithLLM(llm),
		WithCustomRunFunction(customFunc),
		WithName("ErrorTestAgent"),
	)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "test-org")
	result, err := agent.Run(ctx, "error test")

	if err == nil {
		t.Error("Expected error from custom function")
	}

	if result != "" {
		t.Errorf("Expected empty result on error, got %s", result)
	}

	expectedError := "custom function error: error test"
	if err.Error() != expectedError {
		t.Errorf("Expected error %s, got %s", expectedError, err.Error())
	}
}

func TestBothCustomFunctions(t *testing.T) {
	// Test that an agent can have both custom run and stream functions
	customFunc := func(ctx context.Context, input string, agent *Agent) (string, error) {
		return fmt.Sprintf("CUSTOM: %s", input), nil
	}

	customStreamFunc := func(ctx context.Context, input string, agent *Agent) (<-chan interfaces.AgentStreamEvent, error) {
		eventChan := make(chan interfaces.AgentStreamEvent, 1)
		go func() {
			defer close(eventChan)
			eventChan <- interfaces.AgentStreamEvent{
				Type:      interfaces.AgentEventContent,
				Content:   fmt.Sprintf("STREAM: %s", input),
				Timestamp: time.Now(),
			}
			eventChan <- interfaces.AgentStreamEvent{
				Type:      interfaces.AgentEventComplete,
				Timestamp: time.Now(),
			}
		}()
		return eventChan, nil
	}

	llm := &mockLLM{}
	agent, err := NewAgent(
		WithLLM(llm),
		WithCustomRunFunction(customFunc),
		WithCustomRunStreamFunction(customStreamFunc),
		WithName("BothTestAgent"),
	)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "test-org")

	// Test regular run
	result, err := agent.Run(ctx, "test")
	if err != nil {
		t.Fatalf("Failed to run agent: %v", err)
	}
	if result != "CUSTOM: test" {
		t.Errorf("Expected 'CUSTOM: test', got %s", result)
	}

	// Test streaming
	eventChan, err := agent.RunStream(ctx, "stream test")
	if err != nil {
		t.Fatalf("Failed to start streaming: %v", err)
	}

	var events []interfaces.AgentStreamEvent
	for event := range eventChan {
		events = append(events, event)
	}

	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}

	if events[0].Content != "STREAM: stream test" {
		t.Errorf("Expected 'STREAM: stream test', got %s", events[0].Content)
	}
}
