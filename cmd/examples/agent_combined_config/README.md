# Combined Configuration Example

This example demonstrates how to use both the auto-configuration and YAML configuration approaches in a single application. It shows the complete workflow of:

1. Creating an agent using auto-configuration from a system prompt
2. Saving the generated configurations to YAML files
3. Loading those YAML configurations to create a new agent
4. Running both agents on the same task

## Features

### Auto-Configuration

The example creates an agent with auto-configuration by:
- Defining a system prompt that describes a travel advisor agent
- Using `agent.NewAgentWithAutoConfig()` to create the agent
- Retrieving and displaying the generated agent and task configurations
- Saving these configurations to YAML files

### YAML Configuration

The example then loads the saved YAML configurations:
- Using `agent.LoadAgentConfigsFromFile()` to load agent configurations
- Using `agent.LoadTaskConfigsFromFile()` to load task configurations
- Using `agent.NewAgentFromConfig()` to create an agent from these configurations

### Task Execution

Finally, the example executes the same task with both agents:
- The auto-configured agent
- The YAML-configured agent

This demonstrates that both configuration approaches yield functionally equivalent agents.

## Prerequisites

- Go 1.20 or higher
- OpenAI API key set as the `OPENAI_API_KEY` environment variable

## Running the Example

To run the example:

```bash
cd cmd/examples/combined_config_example
go run main.go
```

## What to Expect

When you run the example, you'll see:

1. Auto-configuration phase:
   - The system prompt used
   - The generated agent configuration (role, goal)
   - The number of tasks generated
   - Confirmation that configurations were saved to YAML

2. YAML configuration phase:
   - The loaded agent configurations
   - The number of tasks loaded for the agent
   - Confirmation that the agent was created from YAML configuration

3. Task execution phase:
   - Results from the auto-configured agent
   - Results from the YAML-configured agent

The results should be similar (though not necessarily identical) as both agents are configured with the same capabilities.

## Code Structure

The example is structured into three main functions:

- `createAutoConfiguredAgent()`: Creates an agent using auto-configuration
- `loadYamlAgent()`: Loads agent and task configurations from YAML files
- `executeExampleTask()`: Executes a travel itinerary task with an agent

This structure demonstrates how to separate the different aspects of agent configuration and execution in your own applications. 