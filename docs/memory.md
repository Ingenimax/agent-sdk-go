# Memory

This document explains how to use the Memory component of the Agent SDK.

## Overview

Memory allows an agent to remember previous interactions and maintain context across multiple turns of conversation. The Agent SDK provides several memory implementations to suit different needs.

## Memory Types

### Conversation Buffer

The simplest memory type that stores all messages in a buffer:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/memory"

// Create a conversation buffer memory
mem := memory.NewConversationBuffer()
```

### Conversation Buffer Window

Stores only the most recent N messages:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/memory"

// Create a conversation buffer window memory with a window size of 10 messages
mem := memory.NewConversationBufferWindow(10)
```

### Redis Memory

Stores messages in Redis for persistence:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/memory/redis"

// Create a Redis memory
mem := redis.New(
    "localhost:6379", // Redis URL
    "",               // Redis password (empty for no password)
    0,                // Redis database number
)
```

## Using Memory with an Agent

To use memory with an agent, pass it to the `WithMemory` option:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// Create memory
mem := memory.NewConversationBuffer()

// Create agent with memory
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(mem),
)
```

## Working with Messages

### Adding Messages

You can add messages to memory directly:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// Create memory
mem := memory.NewConversationBuffer()

// Create context
ctx := context.Background()

// Add a user message
err := mem.AddMessage(ctx, interfaces.Message{
    Role:    "user",
    Content: "Hello, how are you?",
})
if err != nil {
    log.Fatalf("Failed to add message: %v", err)
}

// Add an assistant message
err = mem.AddMessage(ctx, interfaces.Message{
    Role:    "assistant",
    Content: "I'm doing well, thank you! How can I help you today?",
})
if err != nil {
    log.Fatalf("Failed to add message: %v", err)
}
```

### Retrieving Messages

You can retrieve messages from memory:

```go
// Get all messages
messages, err := mem.GetMessages(ctx)
if err != nil {
    log.Fatalf("Failed to get messages: %v", err)
}

// Get only user messages
userMessages, err := mem.GetMessages(ctx, interfaces.WithRoles("user"))
if err != nil {
    log.Fatalf("Failed to get user messages: %v", err)
}

// Get the last 5 messages
recentMessages, err := mem.GetMessages(ctx, interfaces.WithLimit(5))
if err != nil {
    log.Fatalf("Failed to get recent messages: %v", err)
}
```

### Clearing Memory

You can clear all messages from memory:

```go
err := mem.Clear(ctx)
if err != nil {
    log.Fatalf("Failed to clear memory: %v", err)
}
```

## Multi-tenancy with Memory

When using memory with multi-tenancy, you need to include the organization ID in the context:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create context with organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// Add a message for this organization
err := mem.AddMessage(ctx, interfaces.Message{
    Role:    "user",
    Content: "Hello from org-123",
})

// Switch to a different organization
ctx = multitenancy.WithOrgID(context.Background(), "org-456")

// Add a message for the other organization
err = mem.AddMessage(ctx, interfaces.Message{
    Role:    "user",
    Content: "Hello from org-456",
})

// Messages are isolated by organization
```

## Conversation IDs

You can use conversation IDs to manage multiple conversations:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

// Create context with conversation ID
ctx := context.Background()
ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-123")

// Add a message to this conversation
err := mem.AddMessage(ctx, interfaces.Message{
    Role:    "user",
    Content: "Hello from conversation 123",
})

// Switch to a different conversation
ctx = context.WithValue(context.Background(), memory.ConversationIDKey, "conversation-456")

// Add a message to the other conversation
err = mem.AddMessage(ctx, interfaces.Message{
    Role:    "user",
    Content: "Hello from conversation 456",
})

// Messages are isolated by conversation ID
```

## Creating Custom Memory Implementations

You can create custom memory implementations by implementing the `interfaces.Memory` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// CustomMemory is a custom memory implementation
type CustomMemory struct {
    messages map[string][]interfaces.Message
}

// NewCustomMemory creates a new custom memory
func NewCustomMemory() *CustomMemory {
    return &CustomMemory{
        messages: make(map[string][]interfaces.Message),
    }
}

// AddMessage adds a message to memory
func (m *CustomMemory) AddMessage(ctx context.Context, message interfaces.Message) error {
    // Get conversation ID from context
    convID := getConversationID(ctx)

    // Add message to the conversation
    m.messages[convID] = append(m.messages[convID], message)

    return nil
}

// GetMessages retrieves messages from memory
func (m *CustomMemory) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
    // Get conversation ID from context
    convID := getConversationID(ctx)

    // Apply options
    opts := &interfaces.GetMessagesOptions{}
    for _, option := range options {
        option(opts)
    }

    // Get messages for the conversation
    messages := m.messages[convID]

    // Apply limit if specified
    if opts.Limit > 0 && opts.Limit < len(messages) {
        start := len(messages) - opts.Limit
        messages = messages[start:]
    }

    // Filter by role if specified
    if len(opts.Roles) > 0 {
        filtered := make([]interfaces.Message, 0)
        for _, msg := range messages {
            for _, role := range opts.Roles {
                if msg.Role == role {
                    filtered = append(filtered, msg)
                    break
                }
            }
        }
        messages = filtered
    }

    return messages, nil
}

// Clear clears the memory
func (m *CustomMemory) Clear(ctx context.Context) error {
    // Get conversation ID from context
    convID := getConversationID(ctx)

    // Clear messages for the conversation
    delete(m.messages, convID)

    return nil
}

// Helper function to get conversation ID from context
func getConversationID(ctx context.Context) string {
    // Get organization ID
    orgID := "default"
    if id := ctx.Value(multitenancy.OrgIDKey); id != nil {
        if s, ok := id.(string); ok {
            orgID = s
        }
    }

    // Get conversation ID
    convID := "default"
    if id := ctx.Value(memory.ConversationIDKey); id != nil {
        if s, ok := id.(string); ok {
            convID = s
        }
    }

    // Combine org ID and conversation ID
    return orgID + ":" + convID
}
```

## Example: Complete Memory Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory/redis"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create memory
    var mem interfaces.Memory
    if cfg.Memory.Redis.URL != "" {
        // Use Redis memory if configured
        mem = redis.New(
            cfg.Memory.Redis.URL,
            cfg.Memory.Redis.Password,
            cfg.Memory.Redis.DB,
        )
    } else {
        // Fall back to in-memory buffer
        mem = memory.NewConversationBuffer()
    }

    // Create a new agent with memory
    agent, err := agent.NewAgent(
        agent.WithLLM(openaiClient),
        agent.WithMemory(mem),
        agent.WithSystemPrompt("You are a helpful AI assistant."),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Create context with organization ID and conversation ID
    ctx := context.Background()
    ctx = multitenancy.WithOrgID(ctx, "org-123")
    ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-123")

    // Run the agent with the first query
    response1, err := agent.Run(ctx, "Hello, who are you?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }
    fmt.Println("Response 1:", response1)

    // Run the agent with a follow-up query (memory will be used)
    response2, err := agent.Run(ctx, "What did I just ask you?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }
    fmt.Println("Response 2:", response2)
}
