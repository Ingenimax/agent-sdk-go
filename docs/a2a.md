# A2A (Agent-to-Agent) Protocol Support

This guide explains how to use the A2A protocol with agent-sdk-go to enable cross-framework agent interoperability. Your agents can communicate with any A2A-compliant framework, including Google ADK, LangChain, CrewAI, and others.

## Table of Contents

1. [What is A2A?](#what-is-a2a)
2. [Quick Start](#quick-start)
3. [Server: Exposing Agents via A2A](#server-exposing-agents-via-a2a)
4. [Client: Calling Remote A2A Agents](#client-calling-remote-a2a-agents)
5. [Remote Agent as Tool](#remote-agent-as-tool)
6. [Agent Card Builder](#agent-card-builder)
7. [Multi-Turn Conversations](#multi-turn-conversations)
8. [Streaming](#streaming)
9. [Authentication](#authentication)
10. [Architecture](#architecture)
11. [API Reference](#api-reference)

## What is A2A?

[A2A (Agent-to-Agent)](https://github.com/a2aproject/a2a-spec) is an open protocol that defines how AI agents discover, communicate, and collaborate with each other regardless of framework. It provides:

- **Agent discovery** via well-known agent cards (`/.well-known/agent-card.json`)
- **Synchronous and streaming** message exchange over JSON-RPC
- **Task lifecycle** management (working, completed, failed, canceled)
- **Multi-turn conversations** via context IDs

The `pkg/a2a` package provides both server (expose your agents) and client (call remote agents) implementations.

## Quick Start

### Expose an agent as an A2A server

```go
package main

import (
    "context"
    "log"
    "os/signal"

    "github.com/a2aproject/a2a-go/a2a"

    a2apkg "github.com/Ingenimax/agent-sdk-go/pkg/a2a"
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

func main() {
    // Any agent-sdk-go agent implements AgentAdapter automatically.
    llm := anthropic.NewClient(os.Getenv("ANTHROPIC_API_KEY"))
    myAgent, _ := agent.NewAgent(
        agent.WithName("Research Assistant"),
        agent.WithDescription("Helps with research tasks"),
        agent.WithLLM(llm),
    )

    card := a2apkg.NewCardBuilder(
        myAgent.GetName(),
        myAgent.GetDescription(),
        "http://localhost:9100/",
        a2apkg.WithStreaming(true),
    ).AddSkill(a2a.AgentSkill{
        ID:          "research",
        Name:        "Research",
        Description: "Searches and synthesizes information",
    }).Build()

    srv := a2apkg.NewServer(myAgent, card,
        a2apkg.WithAddress(":9100"),
    )

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    log.Fatal(srv.Start(ctx))
}
```

### Call a remote A2A agent

```go
ctx := context.Background()

client, err := a2apkg.NewClient(ctx, "http://localhost:9100")
if err != nil {
    log.Fatal(err)
}

fmt.Println("Connected to:", client.Card().Name)

result, err := client.SendMessage(ctx, "What is the A2A protocol?")
if err != nil {
    log.Fatal(err)
}

fmt.Println(a2apkg.ExtractResultText(result))
```

## Server: Exposing Agents via A2A

The server wraps any `AgentAdapter` implementation and exposes it as an A2A-compliant HTTP service with two endpoints:

| Endpoint | Purpose |
|----------|---------|
| `/.well-known/agent-card.json` | Agent discovery (capabilities, skills, metadata) |
| `/` (configurable) | JSON-RPC message exchange |

### AgentAdapter interface

Any type implementing these four methods can be served via A2A:

```go
type AgentAdapter interface {
    Run(ctx context.Context, input string) (string, error)
    RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error)
    GetName() string
    GetDescription() string
}
```

Agents created with `agent.NewAgent()` implement this interface automatically.

### Creating a server

```go
srv := a2apkg.NewServer(agent, card,
    a2apkg.WithAddress(":9100"),
    a2apkg.WithBasePath("/"),
    a2apkg.WithShutdownTimeout(30 * time.Second),
    a2apkg.WithServerLogger(logger),
    a2apkg.WithMiddleware(authMiddleware),
    a2apkg.WithMiddleware(corsMiddleware),
)
```

### Server options

| Option | Default | Description |
|--------|---------|-------------|
| `WithAddress(addr)` | `":0"` | TCP listen address. `:0` picks a random port. |
| `WithBasePath(path)` | `"/"` | JSON-RPC endpoint path. |
| `WithShutdownTimeout(d)` | `30s` | Graceful shutdown deadline. |
| `WithServerLogger(l)` | default | Structured logger for server events. |
| `WithMiddleware(mw)` | none | HTTP middleware (auth, CORS, logging, rate limiting). Applied in order. |

### Starting the server

`Start` blocks until the context is canceled, then performs graceful shutdown:

```go
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
defer cancel()

if err := srv.Start(ctx); err != nil {
    log.Fatal(err)
}
```

### Mounting on an existing HTTP server

Use `Handler()` to get the `http.Handler` and mount it yourself:

```go
mux := http.NewServeMux()
mux.Handle("/a2a/", http.StripPrefix("/a2a", srv.Handler()))
mux.Handle("/api/", apiHandler)

http.ListenAndServe(":8080", mux)
```

### Resolved address

After `Start`, `Addr()` returns the actual listen address (useful when binding to `:0`):

```go
go srv.Start(ctx)
time.Sleep(100 * time.Millisecond)
fmt.Println("Listening on:", srv.Addr()) // e.g. "127.0.0.1:54321"
```

## Client: Calling Remote A2A Agents

The client discovers remote agents via their agent card and communicates using JSON-RPC.

### Auto-discovery

`NewClient` fetches the agent card from `/.well-known/agent-card.json` automatically:

```go
client, err := a2apkg.NewClient(ctx, "http://remote-agent:9100")
if err != nil {
    log.Fatal(err)
}

card := client.Card()
fmt.Printf("Agent: %s (%s)\n", card.Name, card.Description)
fmt.Printf("Skills: %d, Streaming: %v\n", len(card.Skills), card.Capabilities.Streaming)
```

### Pre-resolved card

If you already have the agent card (e.g., from a registry), skip discovery:

```go
client, err := a2apkg.NewClientFromCard(ctx, card,
    a2apkg.WithTimeout(10 * time.Second),
)
```

### Client options

| Option | Default | Description |
|--------|---------|-------------|
| `WithClientLogger(l)` | default | Structured logger. |
| `WithTimeout(d)` | `5m` | HTTP request timeout. |
| `WithBearerToken(t)` | none | Static bearer token for authenticated agents. |

### Sending messages

```go
// Synchronous -- blocks until the agent responds
result, err := client.SendMessage(ctx, "Hello!")

// Extract text from the result (handles Task and Message types)
text := a2apkg.ExtractResultText(result)
```

### Task management

```go
// Retrieve a task by ID
task, err := client.GetTask(ctx, taskID)

// Cancel a running task
task, err := client.CancelTask(ctx, taskID)
```

## Remote Agent as Tool

`RemoteAgentTool` wraps an A2A client as an `interfaces.Tool`, allowing remote A2A agents to be used as tools by local agents. This is how you compose multi-agent systems across frameworks.

```go
// Connect to a remote specialist agent
client, _ := a2apkg.NewClient(ctx, "http://code-review-agent:9100")

// Wrap it as a tool
reviewTool := a2apkg.NewRemoteAgentTool(client)

// Use it in a local agent
localAgent, _ := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithTools(reviewTool),
    agent.WithSystemPrompt("You are a senior developer. Use the code review tool when needed."),
)

// The local agent can now delegate to the remote A2A agent
response, _ := localAgent.Run(ctx, "Review this pull request: ...")
```

### Custom tool name

When registering multiple remote agents as tools, use `WithToolName` to prevent name collisions:

```go
reviewTool := a2apkg.NewRemoteAgentTool(reviewClient, a2apkg.WithToolName("code_reviewer"))
testTool := a2apkg.NewRemoteAgentTool(testClient, a2apkg.WithToolName("test_runner"))

localAgent, _ := agent.NewAgent(
    agent.WithTools(reviewTool, testTool),
    // ...
)
```

### Direct tool usage

You can also call the tool directly without an agent:

```go
tool := a2apkg.NewRemoteAgentTool(client)

// Simple string input
result, err := tool.Run(ctx, "Analyze this code for bugs")

// JSON args (compatible with LLM tool calling)
result, err = tool.Execute(ctx, `{"query": "Analyze this code for bugs"}`)
```

## Agent Card Builder

The `CardBuilder` constructs the A2A agent card that describes your agent's capabilities to clients.

```go
card := a2apkg.NewCardBuilder(
    "My Agent",                          // name
    "Does useful things",                // description
    "http://localhost:9100/",            // base URL
    a2apkg.WithVersion("2.0.0"),
    a2apkg.WithProviderInfo("Acme Corp", "https://acme.com"),
    a2apkg.WithDocumentationURL("https://acme.com/docs/my-agent"),
    a2apkg.WithStreaming(true),
    a2apkg.WithInputModes("text/plain", "application/json"),
    a2apkg.WithOutputModes("text/plain"),
).AddSkill(a2a.AgentSkill{
    ID:          "analyze",
    Name:        "Code Analysis",
    Description: "Analyzes code for bugs and security issues",
    Tags:        []string{"code", "security"},
    Examples:    []string{"Check this Go file for race conditions"},
}).Build()
```

### Card options

| Option | Default | Description |
|--------|---------|-------------|
| `WithVersion(v)` | `"1.0.0"` | Agent version string. |
| `WithProviderInfo(org, url)` | none | Organization name and URL. |
| `WithDocumentationURL(url)` | none | Link to agent documentation. |
| `WithStreaming(bool)` | `true` | Whether the agent supports streaming responses. |
| `WithInputModes(modes...)` | `["text/plain"]` | Accepted input MIME types. |
| `WithOutputModes(modes...)` | `["text/plain"]` | Produced output MIME types. |

### Auto-generating skills from tools

If your agent has tools registered, you can convert them to skills automatically:

```go
card := a2apkg.NewCardBuilder("My Agent", "desc", "http://localhost:9100/").
    SetTools(myAgent.Tools()).
    Build()
```

Each tool becomes a skill with its name, description, and a `"tool"` tag.

## Multi-Turn Conversations

Use `WithContextID` to group messages into a conversation thread. The A2A server tracks context across messages sharing the same context ID.

```go
contextID := "session-abc-123"

// Turn 1
_, err := client.SendMessage(ctx, "My name is Alice",
    a2apkg.WithContextID(contextID),
)

// Turn 2 -- same conversation
result, err := client.SendMessage(ctx, "What is my name?",
    a2apkg.WithContextID(contextID),
)
// Response will reference the earlier context
```

### Task continuation

Use `WithTaskID` to continue an existing in-progress task:

```go
// Start a long-running task
result, _ := client.SendMessage(ctx, "Begin analysis")

task := result.(*a2a.Task)
fmt.Println("Task started:", task.ID, "State:", task.Status.State)

// Later, continue the same task (only valid while task is not in a terminal state)
result2, err := client.SendMessage(ctx, "Add more detail",
    a2apkg.WithTaskID(string(task.ID)),
)
```

Note: A completed, failed, or canceled task cannot be continued. The server will reject the request.

## Streaming

### Server-side streaming

The server automatically streams agent events to clients. The executor maps agent-sdk-go stream events to A2A protocol events:

| Agent Event | A2A Event | Description |
|-------------|-----------|-------------|
| `AgentEventContent` | `TaskArtifactUpdateEvent` | Text content chunks |
| `AgentEventToolCall` | `TaskStatusUpdateEvent` (working) | Tool invocation notification |
| `AgentEventToolResult` | `TaskStatusUpdateEvent` (working) | Tool result notification |
| `AgentEventThinking` | `TaskStatusUpdateEvent` (working) | Reasoning/thinking steps |
| `AgentEventError` | `TaskStatusUpdateEvent` (failed) | Error with message |
| `AgentEventComplete` | `TaskStatusUpdateEvent` (completed) | Task completion |

### Client-side streaming

Use `SendMessageStream` for real-time event consumption:

```go
iter := client.SendMessageStream(ctx, "Write a long analysis")

iter(func(event a2a.Event, err error) bool {
    if err != nil {
        log.Printf("Stream error: %v", err)
        return false // stop iteration
    }

    switch e := event.(type) {
    case *a2a.TaskStatusUpdateEvent:
        fmt.Printf("[%s] %s\n", e.Status.State, extractStatusText(e))
    case *a2a.TaskArtifactUpdateEvent:
        for _, p := range e.Artifact.Parts {
            if tp, ok := p.(a2a.TextPart); ok {
                fmt.Print(tp.Text) // print content chunk
            }
        }
    }

    return true // continue iteration
})
```

Streaming also supports `SendOption`:

```go
iter := client.SendMessageStream(ctx, "Continue our discussion",
    a2apkg.WithContextID("conversation-1"),
)
```

## Authentication

### Server-side: middleware

Use `WithMiddleware` to add authentication. The middleware wraps the HTTP handler, so you have full control over request validation:

```go
func bearerAuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Allow agent card discovery without auth
        if r.URL.Path == "/.well-known/agent-card.json" {
            next.ServeHTTP(w, r)
            return
        }

        token := r.Header.Get("Authorization")
        if token != "Bearer "+os.Getenv("A2A_SECRET") {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }

        next.ServeHTTP(w, r)
    })
}

srv := a2apkg.NewServer(agent, card,
    a2apkg.WithMiddleware(bearerAuthMiddleware),
)
```

Multiple middleware can be stacked (applied in order):

```go
srv := a2apkg.NewServer(agent, card,
    a2apkg.WithMiddleware(corsMiddleware),
    a2apkg.WithMiddleware(rateLimitMiddleware),
    a2apkg.WithMiddleware(authMiddleware),
)
```

### Client-side: bearer token

```go
client, err := a2apkg.NewClient(ctx, "http://secure-agent:9100",
    a2apkg.WithBearerToken(os.Getenv("A2A_TOKEN")),
)
```

The token is sent as `Authorization: Bearer <token>` on every request (except agent card discovery, which uses a separate HTTP call).

## Architecture

### Package structure

```
pkg/a2a/
    doc.go           -- package documentation
    server.go        -- A2A HTTP server with middleware, graceful shutdown
    executor.go      -- AgentAdapter -> A2A event bridge with cancel propagation
    client.go        -- A2A client with card discovery, auth, multi-turn
    tool.go          -- RemoteAgentTool (wraps A2A agent as interfaces.Tool)
    agent_card.go    -- CardBuilder for generating agent cards
    options.go       -- functional options for all components
```

### How it works

```
                           A2A Protocol (JSON-RPC over HTTP)
                          +---------------------------------+
                          |                                 |
  agent-sdk-go Agent      |   A2A Server    A2A Client     |   Remote A2A Agent
  +-----------------+     |   +--------+    +--------+     |   (any framework)
  | Run()           |<--->|-->|Executor|    |        |---->|-->
  | RunStream()     |     |   |        |    | Send   |     |
  | GetName()       |     |   |  A2A   |    | Message|     |
  | GetDescription()|     |   | Events |    |        |     |
  +-----------------+     |   +--------+    +--------+     |
                          |       |              |         |
                          |   Agent Card    Card Discovery |
                          |   /.well-known/ /.well-known/  |
                          +---------------------------------+
```

### Event flow (server)

1. Client sends JSON-RPC `message/send` request
2. Server extracts text from message parts
3. Executor calls `agent.RunStream()` to get event channel
4. Each `AgentStreamEvent` is mapped to an A2A event and written to the response queue
5. Content events produce `TaskArtifactUpdateEvent` (with append for subsequent chunks)
6. Tool/thinking events produce `TaskStatusUpdateEvent` with working state
7. On completion, a final `TaskStatusUpdateEvent` with completed state is sent

### Cancel propagation

When a client sends `tasks/cancel`, the server:
1. Looks up the `context.CancelFunc` for the task ID
2. Calls cancel to stop the running agent goroutine
3. Writes a canceled status event to the client

This ensures long-running agents are properly terminated on cancellation.

## API Reference

### Server

| Function/Method | Description |
|----------------|-------------|
| `NewServer(agent, card, ...ServerOption) *Server` | Create a new A2A server |
| `(*Server).Start(ctx) error` | Start serving (blocks until context canceled) |
| `(*Server).Handler() http.Handler` | Get the HTTP handler for custom mounting |
| `(*Server).Addr() string` | Get resolved listen address |

### Client

| Function/Method | Description |
|----------------|-------------|
| `NewClient(ctx, url, ...ClientOption) (*Client, error)` | Connect with auto-discovery |
| `NewClientFromCard(ctx, card, ...ClientOption) (*Client, error)` | Connect with pre-resolved card |
| `(*Client).Card() *a2a.AgentCard` | Get the resolved agent card |
| `(*Client).SendMessage(ctx, text, ...SendOption) (Result, error)` | Send synchronous message |
| `(*Client).SendMessageStream(ctx, text, ...SendOption) iter` | Send streaming message |
| `(*Client).GetTask(ctx, taskID) (*Task, error)` | Retrieve task by ID |
| `(*Client).CancelTask(ctx, taskID) (*Task, error)` | Cancel a running task |

### RemoteAgentTool

| Function/Method | Description |
|----------------|-------------|
| `NewRemoteAgentTool(client, ...RemoteAgentToolOption) *RemoteAgentTool` | Create tool from client |
| `(*RemoteAgentTool).Name() string` | Tool name (auto-generated or overridden) |
| `(*RemoteAgentTool).Description() string` | Tool description from agent card |
| `(*RemoteAgentTool).Parameters() map[string]ParameterSpec` | Tool parameters (`query`) |
| `(*RemoteAgentTool).Run(ctx, input) (string, error)` | Send message, return text |
| `(*RemoteAgentTool).Execute(ctx, jsonArgs) (string, error)` | Send from JSON args |

### CardBuilder

| Function/Method | Description |
|----------------|-------------|
| `NewCardBuilder(name, desc, url, ...CardOption) *CardBuilder` | Create a card builder |
| `(*CardBuilder).AddSkill(skill) *CardBuilder` | Add an explicit skill |
| `(*CardBuilder).SetTools(tools) *CardBuilder` | Convert tools to skills |
| `(*CardBuilder).Build() *a2a.AgentCard` | Build the agent card |

### Utilities

| Function | Description |
|----------|-------------|
| `ExtractResultText(result) string` | Extract text from `SendMessageResult` (Task or Message) |

### All Options

**Server options:** `WithAddress`, `WithBasePath`, `WithServerLogger`, `WithShutdownTimeout`, `WithMiddleware`

**Client options:** `WithClientLogger`, `WithTimeout`, `WithBearerToken`

**Send options:** `WithContextID`, `WithTaskID`

**Card options:** `WithVersion`, `WithProviderInfo`, `WithDocumentationURL`, `WithStreaming`, `WithInputModes`, `WithOutputModes`

**Tool options:** `WithToolName`
