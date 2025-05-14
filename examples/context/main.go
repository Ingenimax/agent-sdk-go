package main

import (
	"context"
	"os"
	"time"

	pkgcontext "github.com/Ingenimax/agent-sdk-go/pkg/context"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
)

func main() {
	// Create a logger
	logger := logging.New()

	// Create a new context
	ctx := pkgcontext.New()

	// Set organization and conversation IDs
	ctx = ctx.WithOrganizationID("example-org")
	ctx = ctx.WithConversationID("example-conversation")
	ctx = ctx.WithUserID("example-user")
	ctx = ctx.WithRequestID("example-request")

	// Add memory
	memoryBuffer := memory.NewConversationBuffer()
	ctx = ctx.WithMemory(memoryBuffer)

	// Add tools
	toolRegistry := tools.NewRegistry()
	searchTool := websearch.New(
		os.Getenv("GOOGLE_API_KEY"),
		os.Getenv("GOOGLE_SEARCH_ENGINE_ID"),
	)
	toolRegistry.Register(searchTool)
	ctx = ctx.WithTools(toolRegistry)

	// Add LLM
	openaiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	ctx = ctx.WithLLM(openaiClient)

	// Add environment variables
	ctx = ctx.WithEnvironment("temperature", 0.7)
	ctx = ctx.WithEnvironment("max_tokens", 1000)

	// Use the context
	logger.Info(ctx, "Context created with:", nil)
	orgID, _ := ctx.OrganizationID()
	logger.Info(ctx, "Organization ID", map[string]interface{}{"org_id": orgID})
	conversationID, _ := ctx.ConversationID()
	logger.Info(ctx, "Conversation ID", map[string]interface{}{"conversation_id": conversationID})
	userID, _ := ctx.UserID()
	logger.Info(ctx, "User ID", map[string]interface{}{"user_id": userID})
	requestID, _ := ctx.RequestID()
	logger.Info(ctx, "Request ID", map[string]interface{}{"request_id": requestID})

	// Create a context with timeout
	ctxWithTimeout, cancel := ctx.WithTimeout(5 * time.Second)
	defer cancel()

	// Simulate a long-running operation
	select {
	case <-time.After(1 * time.Second):
		logger.Info(ctx, "Operation completed successfully", nil)
	case <-ctxWithTimeout.Done():
		logger.Info(ctx, "Operation timed out", nil)
	}

	// Access components from context
	if mem, ok := ctx.Memory(); ok {
		logger.Info(ctx, "Memory found in context", nil)

		// Create a standard context with organization ID for memory operations
		orgID, _ := ctx.OrganizationID()
		convID, _ := ctx.ConversationID()
		stdCtx := context.Background()
		stdCtx = multitenancy.WithOrgID(stdCtx, orgID)
		// Use the exported function from memory package
		stdCtx = memory.WithConversationID(stdCtx, convID)

		// Use memory to add messages directly with interfaces.Message
		err := mem.AddMessage(stdCtx, interfaces.Message{
			Role:    "user",
			Content: "Hello, this is a test message from the user",
		})
		if err != nil {
			logger.Error(ctx, "Failed to add user message", map[string]interface{}{"error": err.Error()})
		}

		err = mem.AddMessage(stdCtx, interfaces.Message{
			Role:    "assistant",
			Content: "Hello! I'm the AI assistant responding to your test message",
		})
		if err != nil {
			logger.Error(ctx, "Failed to add assistant message", map[string]interface{}{"error": err.Error()})
		}

		// Retrieve and log the conversation history
		messages, err := mem.GetMessages(stdCtx)
		if err != nil {
			logger.Error(ctx, "Failed to get messages", map[string]interface{}{"error": err.Error()})
		} else {
			logger.Info(ctx, "Memory contains messages:", map[string]interface{}{
				"message_count": len(messages),
			})

			// Log each message in the conversation history
			for i, msg := range messages {
				logger.Info(ctx, "Message", map[string]interface{}{
					"index":   i,
					"role":    msg.Role,
					"content": msg.Content,
				})
			}
		}
	}

	if tools, ok := ctx.Tools(); ok {
		logger.Info(ctx, "Tools found in context:", nil)
		for _, tool := range tools.List() {
			logger.Info(ctx, "Tool", map[string]interface{}{
				"name":        tool.Name(),
				"description": tool.Description(),
			})
		}
	}

	if llm, ok := ctx.LLM(); ok {
		logger.Info(ctx, "LLM found in context", nil)

		// Create a prompt for the LLM
		prompt := "You are a helpful assistant. What is the capital of France?"

		// Use the Generate method with empty options slice instead of nil
		response, err := llm.Generate(ctx, prompt, []interfaces.GenerateOption{}...)

		if err != nil {
			logger.Error(ctx, "Failed to generate response from LLM", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			logger.Info(ctx, "LLM Response", map[string]interface{}{
				"content": response,
			})
		}
	}

	if temp, ok := ctx.Environment("temperature"); ok {
		logger.Info(ctx, "Temperature", map[string]interface{}{"value": temp})
	}
}
