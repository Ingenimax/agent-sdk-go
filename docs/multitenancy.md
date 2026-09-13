# Multitenancy

This document explains how to use the multitenancy features of the Agent SDK.

## Overview

Multitenancy allows you to use a single instance of the Agent SDK to serve multiple organizations or users, with isolated data and configurations for each tenant.

## Enabling Multitenancy

To enable multitenancy, set the `MULTITENANCY_ENABLED` environment variable to `true`:

```bash
export MULTITENANCY_ENABLED=true
```

You can also set a default organization ID with the `DEFAULT_ORG_ID` environment variable:

```bash
export DEFAULT_ORG_ID=my-default-org
```

## Using Multitenancy in Your Code

### Setting the Organization ID in Context

To specify which organization's resources to use, add the organization ID to the context:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create a context with an organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// Use this context when calling agent methods
response, err := agent.Run(ctx, "What is the capital of France?")
```

### Getting the Organization ID from Context

To retrieve the organization ID from a context:

```go
orgID := multitenancy.GetOrgID(ctx)
```

If no organization ID is set in the context, this will return the default organization ID.

## Multitenancy with Different Components

### LLM Providers

Each organization can have its own LLM configuration:

```go
// Create an LLM provider for a specific organization
llmProvider := openai.NewClient(
    cfg.LLM.OpenAI.APIKey,
    openai.WithOrgID("org-123"),
)
```

### Memory

Memory is automatically isolated by organization ID:

```go
// Create a memory system
mem := memory.NewConversationBuffer()

// When using the memory with a context that has an organization ID,
// the data will be isolated to that organization
agent.WithMemory(mem)
```

### Vector Stores

> **Correction:** earlier revisions of this page showed
> `weaviate.WithOrgID("org-123")` and described the store as isolated per
> organization. **No such option exists, and `pkg/vectorstore/weaviate` has no
> concept of an organization at all** — it never reads the org from the context.
> Do not rely on it for tenant isolation.

`weaviate.New` takes a config struct, not a URL:

```go
store := weaviate.New(&interfaces.VectorStoreConfig{
    Host:   "localhost:8080",
    Scheme: "http",
    APIKey: cfg.VectorStore.Weaviate.APIKey,
}, weaviate.WithEmbedder(embedder))
```

The config carries `Host`, `Scheme`, `APIKey`, `ClassPrefix` and
`DistanceMetric`; the available options are `WithClassPrefix`, `WithEmbedder`,
`WithDistanceMetric` and `WithLogger`.

To separate tenants, give each organization its own class prefix and select the
store for the org yourself:

```go
store := weaviate.New(&interfaces.VectorStoreConfig{
    Host:   "localhost:8080",
    Scheme: "http",
}, weaviate.WithClassPrefix("org_"+orgID+"_"))
```

That is a naming convention you enforce, not an isolation guarantee the SDK
provides: nothing stops another caller from constructing a store with a
different prefix and reading across tenants.

### Data Stores

Data stores can also be isolated by organization:

```go
// Create a data store with organization isolation
dataStore := supabase.New(
    cfg.DataStore.Supabase.URL,
    cfg.DataStore.Supabase.APIKey,
    supabase.WithOrgID("org-123"),
)
```

## Best Practices

1. **Always use contexts**: Pass the context with the organization ID to all methods that accept a context.

2. **Set default organization ID**: Always set a default organization ID to handle cases where no organization ID is provided.

3. **Validate organization IDs**: Implement validation to ensure that organization IDs are valid and that users have access to the requested organization.

4. **Audit access**: Log organization ID access for security auditing.

## Example: Complete Multitenancy Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create a new agent
    agent, err := agent.NewAgent(
        agent.WithLLM(openaiClient),
        agent.WithMemory(memory.NewConversationBuffer()),
        agent.WithSystemPrompt("You are a helpful AI assistant."),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Create a context with organization ID
    ctx := context.Background()
    ctx = multitenancy.WithOrgID(ctx, "org-123")

    // Run the agent with the organization context
    response, err := agent.Run(ctx, "What is the capital of France?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }

    fmt.Println(response)

    // Switch to a different organization
    ctx = multitenancy.WithOrgID(context.Background(), "org-456")

    // Run the agent with the new organization context
    response, err = agent.Run(ctx, "What is the capital of Germany?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }

    fmt.Println(response)
}
```
