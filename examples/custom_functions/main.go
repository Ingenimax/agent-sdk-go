package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// customDataProcessor is a simple custom run function that processes data
func customDataProcessor(ctx context.Context, input string, agent *agent.Agent) (string, error) {
	logger := agent.GetLogger()
	logger.Info(ctx, "Running custom data processing", map[string]interface{}{
		"input_length": len(input),
	})

	// You can still use agent's memory
	mem := agent.GetMemory()
	if mem != nil {
		// Store the input in memory
		mem.AddMessage(ctx, interfaces.Message{
			Role:    "user",
			Content: input,
		})
	}

	// Custom processing logic - convert to uppercase and add timestamp
	processed := fmt.Sprintf("PROCESSED[%s]: %s", time.Now().Format("15:04:05"), strings.ToUpper(input))

	// Store the result in memory
	if mem != nil {
		mem.AddMessage(ctx, interfaces.Message{
			Role:    "assistant",
			Content: processed,
		})
	}

	logger.Info(ctx, "Custom processing completed", map[string]interface{}{
		"output_length": len(processed),
	})

	return processed, nil
}

// aiEnhancedProcessor is a more complex custom function that uses the LLM
func aiEnhancedProcessor(ctx context.Context, input string, agent *agent.Agent) (string, error) {
	logger := agent.GetLogger()
	llm := agent.GetLLM()
	mem := agent.GetMemory()

	logger.Info(ctx, "Running AI-enhanced processing", map[string]interface{}{
		"input": input,
	})

	// Use the agent's LLM for preprocessing
	preprocessPrompt := fmt.Sprintf("Please analyze and summarize this text in one sentence: %s", input)
	summary, err := llm.Generate(ctx, preprocessPrompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	// Use the agent's tools if available
	tools := agent.GetTools()
	var toolResults []string
	for _, tool := range tools {
		if tool.Name() == "calculator" {
			// Example: if we have a calculator tool, use it
			result, err := tool.Run(ctx, "2+2")
			if err == nil {
				toolResults = append(toolResults, fmt.Sprintf("Calculator result: %s", result))
			}
		}
	}

	// Combine everything
	var finalResult strings.Builder
	finalResult.WriteString("AI Analysis:\n")
	finalResult.WriteString(fmt.Sprintf("Summary: %s\n", summary))

	if len(toolResults) > 0 {
		finalResult.WriteString("Tool Results:\n")
		for _, result := range toolResults {
			finalResult.WriteString(fmt.Sprintf("- %s\n", result))
		}
	}

	finalResult.WriteString(fmt.Sprintf("Original input length: %d characters", len(input)))

	result := finalResult.String()

	// Store in memory if available
	if mem != nil {
		mem.AddMessage(ctx, interfaces.Message{
			Role:    "user",
			Content: input,
		})
		mem.AddMessage(ctx, interfaces.Message{
			Role:    "assistant",
			Content: result,
		})
	}

	return result, nil
}

// customStreamProcessor is a custom streaming function
func customStreamProcessor(ctx context.Context, input string, agent *agent.Agent) (<-chan interfaces.AgentStreamEvent, error) {
	logger := agent.GetLogger()
	logger.Info(ctx, "Starting custom stream processing", map[string]interface{}{
		"input": input,
	})

	eventChan := make(chan interfaces.AgentStreamEvent, 10)

	go func() {
		defer close(eventChan)

		// Send thinking event
		eventChan <- interfaces.AgentStreamEvent{
			Type:         interfaces.AgentEventThinking,
			ThinkingStep: "Analyzing input for custom processing...",
			Timestamp:    time.Now(),
		}

		// Process input word by word with streaming
		words := strings.Fields(input)
		for i, word := range words {
			// Simulate processing time
			time.Sleep(200 * time.Millisecond)

			// Send content event for each word
			eventChan <- interfaces.AgentStreamEvent{
				Type:      interfaces.AgentEventContent,
				Content:   fmt.Sprintf("Processing word %d/%d: '%s' -> '%s'\n", i+1, len(words), word, strings.ToUpper(word)),
				Timestamp: time.Now(),
			}
		}

		// Send final summary
		eventChan <- interfaces.AgentStreamEvent{
			Type:      interfaces.AgentEventContent,
			Content:   fmt.Sprintf("\nCustom processing completed! Processed %d words.", len(words)),
			Timestamp: time.Now(),
		}

		// Send completion event
		eventChan <- interfaces.AgentStreamEvent{
			Type:      interfaces.AgentEventComplete,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"words_processed": len(words),
				"processor_type":  "custom_stream",
			},
		}
	}()

	return eventChan, nil
}

func main() {
	// Get OpenAI API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OpenAI API key not provided. Set OPENAI_API_KEY environment variable.")
	}

	// Create logger
	logger := logging.New()

	// Create LLM client
	llm := openai.NewClient(apiKey, openai.WithLogger(logger))

	// Create memory
	mem := memory.NewConversationBuffer()

	// Example 1: Simple custom function
	fmt.Println("=== Example 1: Simple Custom Function ===")
	simpleAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(mem),
		agent.WithLogger(logger),
		agent.WithCustomRunFunction(customDataProcessor),
		agent.WithName("SimpleCustomAgent"),
		agent.WithDescription("An agent with a simple custom run function"),
	)
	if err != nil {
		log.Fatalf("Failed to create simple agent: %v", err)
	}

	ctx := context.Background()
	result1, err := simpleAgent.Run(ctx, "hello world this is a test")
	if err != nil {
		log.Fatalf("Failed to run simple agent: %v", err)
	}
	fmt.Printf("Result: %s\n\n", result1)

	// Example 2: AI-enhanced custom function
	fmt.Println("=== Example 2: AI-Enhanced Custom Function ===")
	aiAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()), // Fresh memory
		agent.WithLogger(logger),
		agent.WithCustomRunFunction(aiEnhancedProcessor),
		agent.WithName("AIEnhancedAgent"),
		agent.WithDescription("An agent that uses LLM in custom function"),
	)
	if err != nil {
		log.Fatalf("Failed to create AI agent: %v", err)
	}

	result2, err := aiAgent.Run(ctx, "The weather today is sunny and warm. Perfect for outdoor activities like hiking and picnics.")
	if err != nil {
		log.Fatalf("Failed to run AI agent: %v", err)
	}
	fmt.Printf("Result: %s\n\n", result2)

	// Example 3: Custom streaming function
	fmt.Println("=== Example 3: Custom Streaming Function ===")
	streamAgent, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()), // Fresh memory
		agent.WithLogger(logger),
		agent.WithCustomRunStreamFunction(customStreamProcessor),
		agent.WithName("CustomStreamAgent"),
		agent.WithDescription("An agent with custom streaming function"),
	)
	if err != nil {
		log.Fatalf("Failed to create stream agent: %v", err)
	}

	// Test streaming
	eventChan, err := streamAgent.RunStream(ctx, "artificial intelligence machine learning")
	if err != nil {
		log.Fatalf("Failed to start streaming: %v", err)
	}

	fmt.Println("Streaming results:")
	for event := range eventChan {
		switch event.Type {
		case interfaces.AgentEventThinking:
			fmt.Printf("[THINKING] %s\n", event.ThinkingStep)
		case interfaces.AgentEventContent:
			fmt.Printf("[CONTENT] %s", event.Content)
		case interfaces.AgentEventComplete:
			fmt.Printf("[COMPLETE] Processing finished\n")
			if event.Metadata != nil {
				fmt.Printf("Metadata: %+v\n", event.Metadata)
			}
		case interfaces.AgentEventError:
			fmt.Printf("[ERROR] %v\n", event.Error)
		}
	}

	fmt.Println("\n=== All Examples Completed ===")
}