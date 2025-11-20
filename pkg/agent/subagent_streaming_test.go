package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// StreamingMockLLM is a mock LLM that supports streaming
type StreamingMockLLM struct {
	llmName         string
	responseContent string
	thinkingContent string
	streamDelay     time.Duration
}

func (m *StreamingMockLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	return m.responseContent, nil
}

func (m *StreamingMockLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	return m.responseContent, nil
}

func (m *StreamingMockLLM) GenerateDetailed(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return &interfaces.LLMResponse{
		Content: m.responseContent,
		Usage: &interfaces.TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
		Model: m.llmName,
	}, nil
}

func (m *StreamingMockLLM) GenerateWithToolsDetailed(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return m.GenerateDetailed(ctx, prompt, options...)
}

func (m *StreamingMockLLM) Name() string {
	return m.llmName
}

func (m *StreamingMockLLM) SupportsStreaming() bool {
	return true
}

func (m *StreamingMockLLM) GenerateStream(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	eventChan := make(chan interfaces.StreamEvent, 10)

	go func() {
		defer close(eventChan)

		// Send thinking event if configured
		if m.thinkingContent != "" {
			eventChan <- interfaces.StreamEvent{
				Type:      interfaces.StreamEventThinking,
				Content:   m.thinkingContent,
				Timestamp: time.Now(),
			}
			time.Sleep(m.streamDelay)
		}

		// Send message start
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStart,
			Timestamp: time.Now(),
		}

		// Send content in chunks
		words := strings.Split(m.responseContent, " ")
		for _, word := range words {
			select {
			case <-ctx.Done():
				return
			default:
				eventChan <- interfaces.StreamEvent{
					Type:      interfaces.StreamEventContentDelta,
					Content:   word + " ",
					Timestamp: time.Now(),
				}
				time.Sleep(m.streamDelay)
			}
		}

		// Send message stop
		eventChan <- interfaces.StreamEvent{
			Type:      interfaces.StreamEventMessageStop,
			Timestamp: time.Now(),
		}
	}()

	return eventChan, nil
}

func (m *StreamingMockLLM) GenerateWithToolsStream(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	return m.GenerateStream(ctx, prompt, options...)
}

func TestSubAgentStreaming(t *testing.T) {
	// Create streaming LLMs for sub-agents and main agent
	subLLM := &StreamingMockLLM{
		llmName:         "sub-llm",
		responseContent: "Sub-agent analysis complete",
		thinkingContent: "Analyzing the request...",
		streamDelay:     5 * time.Millisecond,
	}

	mainLLM := &StreamingMockLLM{
		llmName:         "main-llm",
		responseContent: "Main agent orchestration complete",
		thinkingContent: "Coordinating the workflow...",
		streamDelay:     5 * time.Millisecond,
	}

	// Create a sub-agent with streaming support
	subAgent, err := NewAgent(
		WithName("AnalyzerAgent"),
		WithDescription("Analyzes data with streaming"),
		WithLLM(subLLM),
		WithRequirePlanApproval(false), // Disable plan approval for testing
	)
	if err != nil {
		t.Fatalf("Failed to create sub-agent: %v", err)
	}

	// Create main agent with the sub-agent
	mainAgent, err := NewAgent(
		WithName("CoordinatorAgent"),
		WithDescription("Coordinates tasks"),
		WithLLM(mainLLM),
		WithAgents(subAgent),
		WithRequirePlanApproval(false), // Disable plan approval for testing
	)
	if err != nil {
		t.Fatalf("Failed to create main agent: %v", err)
	}

	// Test 1: Regular streaming from main agent (without sub-agent calls)
	t.Run("MainAgentStreaming", func(t *testing.T) {
		ctx := context.Background()
		eventChan, err := mainAgent.RunStream(ctx, "test prompt")
		if err != nil {
			t.Fatalf("Failed to start streaming: %v", err)
		}

		var receivedEvents []interfaces.AgentStreamEvent
		for event := range eventChan {
			receivedEvents = append(receivedEvents, event)
		}

		// Verify we received multiple events
		if len(receivedEvents) < 2 {
			t.Errorf("Expected multiple events, got %d", len(receivedEvents))
		}

		// Verify we have content events
		hasContent := false
		for _, event := range receivedEvents {
			if event.Type == interfaces.AgentEventContent {
				hasContent = true
				break
			}
		}
		if !hasContent {
			t.Error("Expected at least one content event")
		}

		// Verify completion event
		lastEvent := receivedEvents[len(receivedEvents)-1]
		if lastEvent.Type != interfaces.AgentEventComplete {
			t.Errorf("Expected last event to be complete, got %v", lastEvent.Type)
		}
	})

	// Test 2: Verify tools are available from sub-agents
	t.Run("SubAgentToolsAvailable", func(t *testing.T) {
		tools := mainAgent.GetTools()
		if len(tools) != 1 {
			t.Fatalf("Expected 1 tool from sub-agent, got %d", len(tools))
		}

		toolName := tools[0].Name()
		expectedName := "AnalyzerAgent_agent"
		if toolName != expectedName {
			t.Errorf("Expected tool name %s, got %s", expectedName, toolName)
		}
	})

	// Test 3: Direct sub-agent tool execution with streaming
	t.Run("SubAgentToolStreaming", func(t *testing.T) {
		tools := mainAgent.GetTools()
		if len(tools) == 0 {
			t.Fatal("No tools available")
		}

		// Create a context with stream forwarder
		var receivedEvents []interfaces.AgentStreamEvent
		forwarder := func(event interfaces.AgentStreamEvent) {
			receivedEvents = append(receivedEvents, event)
		}

		ctx := context.WithValue(context.Background(), interfaces.StreamForwarderKey, interfaces.StreamForwarder(forwarder))

		// Execute the sub-agent tool
		result, err := tools[0].Execute(ctx, `{"query": "analyze this data"}`)
		if err != nil {
			t.Fatalf("Failed to execute sub-agent tool: %v", err)
		}

		// Verify result
		if result == "" {
			t.Error("Expected non-empty result from sub-agent")
		}

		// Verify streaming events were forwarded
		if len(receivedEvents) == 0 {
			t.Error("Expected streaming events to be forwarded, got none")
		}

		// Verify we received various event types
		hasThinking := false
		hasContent := false
		hasComplete := false

		for _, event := range receivedEvents {
			switch event.Type {
			case interfaces.AgentEventThinking:
				hasThinking = true
			case interfaces.AgentEventContent:
				hasContent = true
			case interfaces.AgentEventComplete:
				hasComplete = true
			}
		}

		if !hasThinking {
			t.Error("Expected thinking event from sub-agent streaming")
		}
		if !hasContent {
			t.Error("Expected content event from sub-agent streaming")
		}
		if !hasComplete {
			t.Error("Expected complete event from sub-agent streaming")
		}
	})

	// Test 4: Verify streaming works without forwarder (fallback to blocking)
	t.Run("SubAgentWithoutStreamForwarder", func(t *testing.T) {
		tools := mainAgent.GetTools()
		if len(tools) == 0 {
			t.Fatal("No tools available")
		}

		ctx := context.Background() // No stream forwarder

		// Execute the sub-agent tool (should use blocking mode)
		result, err := tools[0].Execute(ctx, `{"query": "analyze this data"}`)
		if err != nil {
			t.Fatalf("Failed to execute sub-agent tool: %v", err)
		}

		// Verify result
		if result == "" {
			t.Error("Expected non-empty result from sub-agent")
		}
	})
}

func TestSubAgentStreamingWithMultipleLevels(t *testing.T) {
	// Create a chain of agents: Main -> Level1 -> Level2

	level2LLM := &StreamingMockLLM{
		llmName:         "level2-llm",
		responseContent: "Level 2 processing complete",
		thinkingContent: "Level 2 thinking...",
		streamDelay:     3 * time.Millisecond,
	}

	level1LLM := &StreamingMockLLM{
		llmName:         "level1-llm",
		responseContent: "Level 1 processing complete",
		thinkingContent: "Level 1 thinking...",
		streamDelay:     3 * time.Millisecond,
	}

	mainLLM := &StreamingMockLLM{
		llmName:         "main-llm",
		responseContent: "Main processing complete",
		thinkingContent: "Main thinking...",
		streamDelay:     3 * time.Millisecond,
	}

	// Create level 2 agent (deepest)
	level2Agent, err := NewAgent(
		WithName("Level2Agent"),
		WithDescription("Level 2 processing"),
		WithLLM(level2LLM),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("Failed to create level 2 agent: %v", err)
	}

	// Create level 1 agent with level 2 as sub-agent
	level1Agent, err := NewAgent(
		WithName("Level1Agent"),
		WithDescription("Level 1 processing"),
		WithLLM(level1LLM),
		WithAgents(level2Agent),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("Failed to create level 1 agent: %v", err)
	}

	// Create main agent with level 1 as sub-agent
	mainAgent, err := NewAgent(
		WithName("MainAgent"),
		WithDescription("Main processing"),
		WithLLM(mainLLM),
		WithAgents(level1Agent),
		WithRequirePlanApproval(false),
	)
	if err != nil {
		t.Fatalf("Failed to create main agent: %v", err)
	}

	// Test nested streaming
	t.Run("NestedStreaming", func(t *testing.T) {
		// Get the level1 tool from main agent
		tools := mainAgent.GetTools()
		if len(tools) == 0 {
			t.Fatal("No tools available on main agent")
		}

		// Create a stream forwarder
		var receivedEvents []interfaces.AgentStreamEvent
		forwarder := func(event interfaces.AgentStreamEvent) {
			receivedEvents = append(receivedEvents, event)
		}

		ctx := context.WithValue(context.Background(), interfaces.StreamForwarderKey, interfaces.StreamForwarder(forwarder))

		// Execute level 1 agent (which might call level 2)
		result, err := tools[0].Execute(ctx, `{"query": "process this"}`)
		if err != nil {
			t.Fatalf("Failed to execute nested agent: %v", err)
		}

		// Verify result
		if result == "" {
			t.Error("Expected non-empty result from nested execution")
		}

		// Verify we received events (at least from level 1)
		if len(receivedEvents) == 0 {
			t.Error("Expected events from nested streaming")
		}
	})
}

func TestStreamForwarderContextKey(t *testing.T) {
	// Test that the stream forwarder can be properly set and retrieved from context

	callCount := 0
	forwarder := func(event interfaces.AgentStreamEvent) {
		callCount++
	}

	ctx := context.WithValue(context.Background(), interfaces.StreamForwarderKey, interfaces.StreamForwarder(forwarder))

	// Retrieve the forwarder
	retrievedForwarder, ok := ctx.Value(interfaces.StreamForwarderKey).(interfaces.StreamForwarder)
	if !ok {
		t.Fatal("Failed to retrieve stream forwarder from context")
	}

	// Test the forwarder works
	retrievedForwarder(interfaces.AgentStreamEvent{
		Type:      interfaces.AgentEventContent,
		Content:   "test",
		Timestamp: time.Now(),
	})

	if callCount != 1 {
		t.Errorf("Expected forwarder to be called once, got %d", callCount)
	}
}
