# Structured Output with YAML Configuration

This document explains how to use structured output (JSON responses) with YAML-based agent and task configurations in the Agent SDK.

## Overview

The SDK now supports defining structured output schemas directly in YAML configuration files. This allows you to:

- Define JSON response formats for agents and tasks
- Automatically apply structured output when creating agents from YAML
- Unmarshal responses directly into Go structs

## YAML Configuration

### Agent Configuration with Structured Output

You can define a `response_format` field in your agent configuration:

```yaml
researcher:
  role: >
    {topic} Senior Data Researcher
  goal: >
    Uncover cutting-edge developments in {topic}
  backstory: >
    You're a seasoned researcher with a knack for uncovering the latest
    developments in {topic}. Known for your ability to find the most relevant
    information and present it in a clear and concise manner.
  response_format:
    type: "json_object"
    schema_name: "ResearchResult"
    schema_definition:
      type: "object"
      properties:
        findings:
          type: "array"
          items:
            type: "object"
            properties:
              title:
                type: "string"
                description: "Title of the finding"
              description:
                type: "string"
                description: "Detailed description"
              source:
                type: "string"
                description: "Source of the information"
        summary:
          type: "string"
          description: "Executive summary of findings"
        metadata:
          type: "object"
          properties:
            total_findings:
              type: "integer"
            research_date:
              type: "string"
```

### Task Configuration with Structured Output

You can also define structured output at the task level:

```yaml
research_task:
  description: >
    Conduct a thorough research about {topic}
    Make sure you find any interesting and relevant information.
  expected_output: >
    A structured JSON response with findings, summary, and metadata
  agent: researcher
  output_file: "{topic}_report.json"
  response_format:
    type: "json_object"
    schema_name: "ResearchResult"
    schema_definition:
      # Same schema as above
```

## Response Format Configuration

The `response_format` field contains:

- **type**: The response format type (currently supports "json_object")
- **schema_name**: A name for the schema (used for identification)
- **schema_definition**: A JSON schema that defines the expected response structure

### Schema Definition

The `schema_definition` follows the JSON Schema specification:

```yaml
schema_definition:
  type: "object"
  properties:
    field_name:
      type: "string" | "integer" | "number" | "boolean" | "array" | "object"
      description: "Optional description of the field"
    array_field:
      type: "array"
      items:
        type: "object"
        properties:
          # Nested object properties
    object_field:
      type: "object"
      properties:
        # Nested properties
```

## Usage in Go Code

### 1. Define Your Go Struct

Create a Go struct that matches your YAML schema:

```go
type ResearchResult struct {
    Findings []struct {
        Title       string `json:"title"`
        Description string `json:"description"`
        Source      string `json:"source"`
    } `json:"findings"`
    Summary  string `json:"summary"`
    Metadata struct {
        TotalFindings int    `json:"total_findings"`
        ResearchDate  string `json:"research_date"`
    } `json:"metadata"`
}
```

### 2. Load and Use Configuration

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
    // Load configurations
    agentConfigs, err := agent.LoadAgentConfigsFromFile("agents.yaml")
    if err != nil {
        log.Fatal(err)
    }

    taskConfigs, err := agent.LoadTaskConfigsFromFile("tasks.yaml")
    if err != nil {
        log.Fatal(err)
    }

    // Create LLM client
    llm := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

    // Variables for template substitution
    variables := map[string]string{
        "topic": "Quantum Computing",
    }

    // Create agent for task
    agent, err := agent.CreateAgentForTask("research_task", agentConfigs, taskConfigs, variables, agent.WithLLM(llm))
    if err != nil {
        log.Fatal(err)
    }

    // Execute task
    result, err := agent.ExecuteTaskFromConfig(context.Background(), "research_task", taskConfigs, variables)
    if err != nil {
        log.Fatal(err)
    }

    // Unmarshal structured output
    var structured ResearchResult
    err = json.Unmarshal([]byte(result), &structured)
    if err != nil {
        log.Printf("Failed to unmarshal structured output: %v", err)
        return
    }

    // Use the structured data
    fmt.Printf("Found %d findings\n", len(structured.Findings))
    for _, finding := range structured.Findings {
        fmt.Printf("- %s: %s\n", finding.Title, finding.Description)
    }
}
```

## Priority Rules

When both agent and task have `response_format` defined:

1. **Task-level response format takes precedence** over agent-level
2. If only agent has `response_format`, it will be used
3. If neither has `response_format`, no structured output is applied

## Example: Complete Working Example

See `examples/agent_config_yaml/` for a complete working example that demonstrates:

- YAML configuration with structured output
- Agent creation from YAML
- Task execution with structured responses
- Unmarshaling into Go structs

### Running the Example

```bash
# Set your OpenAI API key
export OPENAI_API_KEY=your_api_key_here

# Run the example
go run examples/agent_config_yaml/main.go \
  --agent-config=examples/agent_config_yaml/agents.yaml \
  --task-config=examples/agent_config_yaml/tasks.yaml \
  --task=research_task \
  --topic="Quantum Computing"
```

## Best Practices

1. **Keep schemas in sync**: Ensure your Go structs match your YAML schemas
2. **Use descriptive field names**: Make your JSON fields self-documenting
3. **Add descriptions**: Include descriptions in your schema for better LLM understanding
4. **Test your schemas**: Verify that the LLM can generate valid responses matching your schema
5. **Handle unmarshaling errors**: Always check for JSON unmarshaling errors

## Schema Validation

The SDK validates that:
- The `type` field is valid ("json_object" is currently supported)
- The `schema_definition` is a valid JSON schema
- Required fields are properly defined

## Limitations

- Currently only supports "json_object" response format
- Schema definitions must be valid JSON Schema
- Complex nested schemas may require careful testing with your LLM provider
