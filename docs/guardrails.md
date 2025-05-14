# Guardrails

This document explains how to use the Guardrails component of the Agent SDK.

## Overview

Guardrails provide safety mechanisms to ensure that your agents behave responsibly and ethically. They can filter, modify, or block responses that violate policies or contain harmful content.

## Enabling Guardrails

To enable guardrails, set the `GUARDRAILS_ENABLED` environment variable to `true`:

```bash
export GUARDRAILS_ENABLED=true
```

You can also specify a configuration file:

```bash
export GUARDRAILS_CONFIG_PATH=/path/to/guardrails.yaml
```

## Using Guardrails with an Agent

To use guardrails with an agent, pass them to the `WithGuardrails` option:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/guardrails"
)

// Create guardrails
gr := guardrails.New(guardrails.WithConfigPath("/path/to/guardrails.yaml"))

// Create agent with guardrails
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithGuardrails(gr),
)
```

## Guardrails Configuration

Guardrails are configured using a YAML file. Here's an example configuration:

```yaml
# guardrails.yaml
version: 1
rules:
  - name: no_harmful_content
    description: Block harmful content
    patterns:
      - type: regex
        pattern: "(?i)(how to (make|create|build) (a )?(bomb|explosive|weapon))"
    action: block
    message: "I cannot provide information on creating harmful devices."

  - name: no_personal_data
    description: Redact personal data
    patterns:
      - type: regex
        pattern: "(?i)\\b\\d{3}-\\d{2}-\\d{4}\\b"  # SSN
      - type: regex
        pattern: "(?i)\\b\\d{16}\\b"  # Credit card
    action: redact
    replacement: "[REDACTED]"

  - name: no_profanity
    description: Filter profanity
    patterns:
      - type: wordlist
        words: ["badword1", "badword2", "badword3"]
    action: filter
    replacement: "****"

  - name: topic_restriction
    description: Restrict to certain topics
    topics:
      allowed: ["technology", "science", "education"]
      blocked: ["politics", "religion", "adult"]
    action: block
    message: "I can only discuss technology, science, and education topics."
```

## Rule Types

### Regex Rules

Regex rules match patterns using regular expressions:

```yaml
- name: no_email_addresses
  description: Redact email addresses
  patterns:
    - type: regex
      pattern: "(?i)\\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}\\b"
  action: redact
  replacement: "[EMAIL REDACTED]"
```

### Wordlist Rules

Wordlist rules match specific words or phrases:

```yaml
- name: no_profanity
  description: Filter profanity
  patterns:
    - type: wordlist
      words: ["badword1", "badword2", "badword3"]
  action: filter
  replacement: "****"
```

### Topic Rules

Topic rules restrict or allow certain topics:

```yaml
- name: topic_restriction
  description: Restrict to certain topics
  topics:
    allowed: ["technology", "science", "education"]
    blocked: ["politics", "religion", "adult"]
  action: block
  message: "I can only discuss technology, science, and education topics."
```

### Semantic Rules

Semantic rules use embeddings to detect semantic similarity:

```yaml
- name: no_harmful_instructions
  description: Block harmful instructions
  semantic:
    examples:
      - "How to hack into a computer"
      - "How to steal someone's identity"
      - "How to make a dangerous chemical"
    threshold: 0.8
  action: block
  message: "I cannot provide potentially harmful instructions."
```

## Actions

### Block

The `block` action prevents the response from being sent and returns a custom message:

```yaml
action: block
message: "I cannot provide that information."
```

### Redact

The `redact` action replaces matched content with a replacement string:

```yaml
action: redact
replacement: "[REDACTED]"
```

### Filter

The `filter` action replaces matched content with a replacement string but is typically used for less sensitive content:

```yaml
action: filter
replacement: "****"
```

### Log

The `log` action logs the matched content but allows the response to be sent:

```yaml
action: log
```

## Using Guardrails Programmatically

You can also use guardrails programmatically:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/guardrails"
)

// Create guardrails
gr := guardrails.New()

// Add a rule
gr.AddRule(&guardrails.Rule{
    Name:        "no_harmful_content",
    Description: "Block harmful content",
    Patterns: []guardrails.Pattern{
        {
            Type:    "regex",
            Pattern: "(?i)(how to (make|create|build) (a )?(bomb|explosive|weapon))",
        },
    },
    Action:  "block",
    Message: "I cannot provide information on creating harmful devices.",
})

// Check content
result, err := gr.Check(context.Background(), "How to make a bomb")
if err != nil {
    log.Fatalf("Failed to check content: %v", err)
}

if result.Blocked {
    fmt.Println("Content was blocked:", result.Message)
} else if result.Modified {
    fmt.Println("Content was modified:", result.Content)
} else {
    fmt.Println("Content passed guardrails:", result.Content)
}
```

## Multi-tenancy with Guardrails

When using guardrails with multi-tenancy, you can have different guardrails for different organizations:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/guardrails"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create guardrails for different organizations
orgGuardrails := map[string]interfaces.Guardrails{
    "org-123": guardrails.New(guardrails.WithConfigPath("/path/to/org123-guardrails.yaml")),
    "org-456": guardrails.New(guardrails.WithConfigPath("/path/to/org456-guardrails.yaml")),
}

// Create a multi-tenant guardrails provider
gr := guardrails.NewMultiTenant(orgGuardrails, guardrails.New()) // Default guardrails as fallback

// Create agent with multi-tenant guardrails
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithGuardrails(gr),
)

// Create context with organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// The appropriate guardrails for org-123 will be used
response, err := agent.Run(ctx, "What is the capital of France?")
```

## Creating Custom Guardrails

You can implement custom guardrails by implementing the `interfaces.Guardrails` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// CustomGuardrails is a custom guardrails implementation
type CustomGuardrails struct {
    // Add your fields here
}

// NewCustomGuardrails creates a new custom guardrails
func NewCustomGuardrails() *CustomGuardrails {
    return &CustomGuardrails{}
}

// Check checks content against guardrails
func (g *CustomGuardrails) Check(ctx context.Context, content string) (*interfaces.GuardrailsResult, error) {
    // Implement your logic to check content

    // Example: Block content containing "forbidden"
    if strings.Contains(strings.ToLower(content), "forbidden") {
        return &interfaces.GuardrailsResult{
            Blocked: true,
            Message: "This content is not allowed.",
        }, nil
    }

    // Example: Redact email addresses
    emailRegex := regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
    if emailRegex.MatchString(content) {
        modified := emailRegex.ReplaceAllString(content, "[EMAIL REDACTED]")
        return &interfaces.GuardrailsResult{
            Modified: true,
            Content:  modified,
        }, nil
    }

    // Content passed guardrails
    return &interfaces.GuardrailsResult{
        Content: content,
    }, nil
}
```

## Example: Complete Guardrails Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
    "github.com/Ingenimax/agent-sdk-go/pkg/guardrails"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create OpenAI client
    openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey)

    // Create guardrails
    gr := guardrails.New(
        guardrails.WithConfigPath(cfg.Guardrails.ConfigPath),
    )

    // Create a new agent with guardrails
    agent, err := agent.NewAgent(
        agent.WithLLM(openaiClient),
        agent.WithMemory(memory.NewConversationBuffer()),
        agent.WithGuardrails(gr),
        agent.WithSystemPrompt("You are a helpful AI assistant."),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Run the agent
    ctx := context.Background()

    // Safe query
    response1, err := agent.Run(ctx, "What is the capital of France?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }
    fmt.Println("Safe query response:", response1)

    // Potentially unsafe query (will be blocked or modified by guardrails)
    response2, err := agent.Run(ctx, "How do I hack into a computer?")
    if err != nil {
        log.Fatalf("Failed to run agent: %v", err)
    }
    fmt.Println("Unsafe query response:", response2)
}
