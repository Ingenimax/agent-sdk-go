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

// Keep only the 10 most recent messages
mem := memory.NewConversationBuffer(memory.WithMaxSize(10))
```

The buffer trims from the front once it exceeds the size, so the oldest messages
are dropped first. The default is 100.

### Redis Memory

Stores messages in Redis for persistence:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
    "github.com/go-redis/redis/v8"
)

client := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
})

mem := memory.NewRedisMemory(client,
    memory.WithTTL(24*time.Hour),
    memory.WithKeyPrefix("agent:"),
)
```

Or from a config struct, which creates the client for you:

```go
mem, err := memory.NewRedisMemoryFromConfig(memory.RedisConfig{
    URL: "localhost:6379",
})
```

#### Summarization, and the archive

Redis memory can summarize old messages in place, keeping the conversation
bounded without growing the key forever:

```go
mem := memory.NewRedisMemory(client,
    memory.WithSummarization(llmClient, 50, 5),
    // 50 = summarize once the conversation exceeds 50 messages
    //  5 = how many summaries to keep
)
```

Summarization is lossy, and for Redis the raw messages exist nowhere else. So
the messages a summary replaces are **moved to an archive list** rather than
deleted, and can be read back:

```go
archived, err := mem.ArchivedMessages(ctx)
```

The archive lives beside the conversation, at the conversation key plus an
`:archive` suffix. Change the prefix with `memory.WithArchiveKeyPrefix("...")`,
or turn archiving off with `memory.WithoutArchive()` when storage cost matters
more than being able to recover what a summary dropped.

Two details worth knowing:

- The archive write and the trim happen in **one Lua script**, so a concurrent
  write cannot land in the window between them and be lost.
- The trim point is **pair-safe**. A cut can otherwise land between an assistant
  turn carrying `tool_calls` and the tool messages answering it, and both the
  OpenAI and Anthropic APIs reject a tool result whose call is absent from the
  transcript — which failed the *next* turn, far from the cause. The boundary is
  walked back over trailing tool messages and the assistant turn that requested
  them.

> **Changed:** summarized messages were previously trimmed away permanently. A
> bad summary took the only copy of the transcript with it, and nothing could
> audit what had been dropped. See
> [Upgrading](upgrading.md#redis-summarization-now-archives-what-it-replaces).

### Conversation Summary

Keeps a rolling buffer and, once it fills, replaces the buffered messages with
an LLM-generated summary. Useful for long conversations that would otherwise
outgrow the model's context window.

```go
mem := memory.NewConversationSummary(llmClient,
    memory.WithMaxBufferSize(20),  // summarize once 20 messages accumulate
    memory.WithSummaryLength(150), // target word count for the summary
)
```

How it behaves:

- When the buffer reaches `WithMaxBufferSize`, the buffered messages are
  summarized and cleared.
- The **previous summary is folded into the new one**, so the record compounds
  rather than being replaced. `Metadata["count"]` on the stored summary
  accumulates across every round.
- `GetMessages` returns the current summary first, followed by any messages
  buffered since.
- Assistant turns carrying only tool calls are rendered as
  `(called tools: name, name)` rather than as empty content, so the summarizer
  can see what the agent did.

> **Changed:** before the current development line, each new summary *replaced*
> the previous one and the summarizer only saw the current buffer — so every
> summarization after the first discarded all earlier history irrecoverably.
> Separately, the internal buffer was hardcoded to 100 messages, so any
> `WithMaxBufferSize` above 100 silently never fired. Both are fixed. See
> [Upgrading](upgrading.md#behaviour-changes).

Summarization runs inline on the call that crosses the threshold, so that
`AddMessage` blocks on an LLM call.

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
    // Get organization ID. multitenancy stores it under an unexported key, so
    // go through GetOrgID rather than reaching into the context yourself --
    // there is no exported key to read, and the similarly-named key used by
    // pkg/context is a different one that GetOrgID does not see.
    orgID := "default"
    if id, err := multitenancy.GetOrgID(ctx); err == nil {
        orgID = id
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
```

## Semantic recall (vector store)

`VectorStoreRetriever` embeds every message into a vector store and can surface
relevant earlier turns on later questions.

```go
mem := memory.NewVectorStoreRetriever(store,
    memory.WithAutoRecall(),      // search the store on every read
    memory.WithRecallLimit(5),    // how many excerpts to surface
)
```

> **Important:** without `WithAutoRecall` the store is **write-only**. Reads only
> searched it when the caller passed `interfaces.WithQuery`, and no LLM provider
> does — every one builds its request with a bare `GetMessages()`. So messages
> were embedded and stored, and nothing ever read them back. `WithAutoRecall` is
> opt-in because enabling it changes what the model sees, but without it you are
> paying embedding cost for nothing.

With auto-recall on, the most recent user message is used as the query — it is
what the agent is being asked to answer.

### Recalled content never reorders the transcript

Recalled excerpts arrive as a single leading system message; the real message
sequence is passed through untouched.

That is deliberate. A similarity search returns results that are neither
contiguous nor chronological, and splicing them into the message list would
separate `tool_call` from its `tool_result` — which both the OpenAI and
Anthropic APIs reject outright. Content already present in the recent window is
skipped, so the model is not told the same thing twice, and a failing vector
store degrades to the plain transcript rather than failing the turn.

## Capability discovery through decorators

`interfaces.Memory` is three methods — `AddMessage`, `GetMessages`, `Clear`.
Richer behaviour is optional and discovered by type assertion:

- `interfaces.ConversationMemory` adds `GetAllConversations`,
  `GetConversationMessages` and `GetMemoryStatistics`.
- `interfaces.AdminConversationMemory` adds the cross-organization variants.

A **decorator** that implements only the three core methods hides every optional
capability of whatever it wraps, and a bare type assertion fails silently:
callers get an empty result rather than an error. This is exactly what happened
when `tracing.NewTracedMemory` wrapped a `RedisMemory` — `Agent.GetAllConversations`
and its siblings started returning nothing.

### Asking about capabilities

Use the helpers rather than a bare assertion. They walk the decorator chain:

```go
if convMem, ok := interfaces.AsConversationMemory(mem); ok {
    conversations, err := convMem.GetAllConversations(ctx)
}

if adminMem, ok := interfaces.AsAdminConversationMemory(mem); ok {
    all, err := adminMem.GetAllConversationsAcrossOrgs()
}

inner := interfaces.UnwrapMemory(mem) // the innermost Memory
```

### Writing a Memory decorator

Implement `interfaces.MemoryUnwrapper` so the helpers can see past you:

```go
type auditedMemory struct {
    inner interfaces.Memory
}

// ... AddMessage, GetMessages, Clear ...

func (m *auditedMemory) Unwrap() interfaces.Memory { return m.inner }
```

Without `Unwrap`, your decorator silently disables conversation listing,
per-conversation retrieval and memory statistics for every agent that uses it.

## See also

- [Tracing](tracing.md) — the memory tracing decorator
- [Multi-tenancy](multitenancy.md) — how memory is scoped
- [Upgrading](upgrading.md) — memory behaviour changes
