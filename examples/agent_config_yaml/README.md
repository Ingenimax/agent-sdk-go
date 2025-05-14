# YAML Configuration for Agents and Tasks

This example demonstrates how to use YAML configuration files to define agents and tasks in the Agent SDK.

## Configuration Files

### `agents.yaml`

This file defines the available agents with their roles, goals, and backstories. Each agent has a unique identifier (key).

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
```

Variables can be used in the configuration by wrapping them in curly braces, like `{topic}`. These will be replaced at runtime.

### `tasks.yaml`

This file defines the available tasks with their descriptions, expected outputs, and associated agents.

```yaml
research_task:
  description: >
    Conduct a thorough research about {topic}
    Make sure you find any interesting and relevant information.
  expected_output: >
    A list with 10 bullet points of the most relevant information about {topic}
  agent: researcher
  output_file: "{topic}_report.md"  # Optional
```

The `output_file` field is optional. If provided, the task result will be written to this file.

## Usage

You can run the example with the following command:

```bash
go run main.go --agent-config=agents.yaml --task-config=tasks.yaml --task=research_task --topic="Quantum Computing"
```

Options:
- `--agent-config`: Path to the agent configuration YAML file
- `--task-config`: Path to the task configuration YAML file
- `--task`: Name of the task to execute
- `--topic`: Topic for the agents to work on (optional, default: "Artificial Intelligence")
- `--openai-key`: OpenAI API key (optional, defaults to OPENAI_API_KEY environment variable)

## Loading YAML Configurations in Your Code

```go
// Load agent configurations
agentConfigs, err := agent.LoadAgentConfigsFromFile("agents.yaml")
if err != nil {
    log.Fatal(err)
}

// Load task configurations
taskConfigs, err := agent.LoadTaskConfigsFromFile("tasks.yaml")
if err != nil {
    log.Fatal(err)
}

// Create variables map for template substitution
variables := map[string]string{
    "topic": "Artificial Intelligence",
}

// Create agent for a specific task
agent, err := agent.CreateAgentForTask("research_task", agentConfigs, taskConfigs, variables, agent.WithLLM(llm))
if err != nil {
    log.Fatal(err)
}

// Execute the task
result, err := agent.ExecuteTaskFromConfig(context.Background(), "research_task", taskConfigs, variables)
if err != nil {
    log.Fatal(err)
}
```

You can also load all YAML files from a directory:

```go
// Load all agent configurations from a directory
agentConfigs, err := agent.LoadAgentConfigsFromDir("config")
if err != nil {
    log.Fatal(err)
}

// Load all task configurations from a directory
taskConfigs, err := agent.LoadTaskConfigsFromDir("config")
if err != nil {
    log.Fatal(err)
}
``` 