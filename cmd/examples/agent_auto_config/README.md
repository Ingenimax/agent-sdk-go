# Auto-Configuration Example

This example demonstrates how to use the auto-configuration feature to generate agent and task configurations from a system prompt using an LLM.

## How It Works

The auto-configuration feature uses an LLM to analyze a system prompt and generate:

1. An agent configuration with role, goal, and backstory
2. Task configurations with descriptions and expected outputs

This is particularly useful when you have a system prompt but want a more structured representation of the agent's capabilities and tasks.

## Usage

You can run the example with the following command:

```bash
go run main.go --system-prompt="You are a SQL expert who helps users design, optimize, and debug database queries." --agent-name="SQL Assistant" --save-configs
```

Options:
- `--system-prompt`: The system prompt to generate configurations from (required)
- `--agent-name`: Name for the agent (optional, default: "Auto-Configured Agent")
- `--openai-key`: OpenAI API key (optional, defaults to OPENAI_API_KEY environment variable)
- `--save-configs`: Save generated configurations to YAML files (optional)

## Example Output

```
=== Generated Agent Configuration ===
Role: SQL Database Expert
Goal: To help users design, optimize, and debug SQL queries for better database performance
Backstory: You've spent over a decade working with various database systems and have become an expert in SQL query optimization. Your experience spans from small applications to enterprise-level database architectures.

=== Generated Task Configurations ===

Task: auto_task_1
Description: Analyze a provided SQL query and suggest optimizations to improve performance.
Expected Output: A list of optimization suggestions with explanations and the optimized query.

Task: auto_task_2
Description: Help debug an SQL query that is returning unexpected results or errors.
Expected Output: A comprehensive analysis of the issue with the query, including the root cause and a corrected version.

Configurations saved to auto_agent_config.yaml and auto_task_config.yaml

Use these configurations in your application or save them to YAML files.
```

## Generated YAML Files

### auto_agent_config.yaml
```yaml
SQL Assistant:
  role: SQL Database Expert
  goal: To help users design, optimize, and debug SQL queries for better database performance
  backstory: You've spent over a decade working with various database systems and have become an expert in SQL query optimization. Your experience spans from small applications to enterprise-level database architectures.
```

### auto_task_config.yaml
```yaml
auto_task_1:
  description: Analyze a provided SQL query and suggest optimizations to improve performance.
  expected_output: A list of optimization suggestions with explanations and the optimized query.
  agent: SQL Assistant

auto_task_2:
  description: Help debug an SQL query that is returning unexpected results or errors.
  expected_output: A comprehensive analysis of the issue with the query, including the root cause and a corrected version.
  agent: SQL Assistant
```

## Using the Generated Configurations in Your Code

Once you have the generated configurations, you can use them in your application:

```go
// Load the generated agent configurations
agentConfigs, err := agent.LoadAgentConfigsFromFile("auto_agent_config.yaml")
if err != nil {
    log.Fatal(err)
}

// Load the generated task configurations
taskConfigs, err := agent.LoadTaskConfigsFromFile("auto_task_config.yaml")
if err != nil {
    log.Fatal(err)
}

// Create variables for template substitution
variables := make(map[string]string)

// Create an agent for a specific task
agent, err := agent.CreateAgentForTask("auto_task_1", agentConfigs, taskConfigs, variables, agent.WithLLM(llm))
if err != nil {
    log.Fatal(err)
}

// Execute the task
result, err := agent.ExecuteTaskFromConfig(context.Background(), "auto_task_1", taskConfigs, variables)
if err != nil {
    log.Fatal(err)
}
```

## Benefits of Auto-Configuration

- **Simplified Setup**: No need to manually write agent and task configurations
- **Consistent Structure**: Generated configurations follow a consistent format
- **Flexibility**: Works with any system prompt to generate appropriate tasks
- **Seamless Integration**: Generated configurations can be saved to files and used with the existing YAML configuration functionality 