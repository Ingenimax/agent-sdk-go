# LLM Orchestration Example

This example demonstrates how to implement LLM-based orchestration for specialized agents in the Agent SDK.

## Features

- LLM-based orchestration to create and execute multi-step plans
- Multiple specialized agents with different capabilities:
  - Research agent with web search capabilities
  - Math agent with calculator capabilities
  - Creative agent for content generation
  - Summary agent for information condensation
- Intelligent dependency handling between steps
- Fallback mechanisms for failed steps
- Character count tracking for research and summary outputs
- Conversation memory for each agent
- Timeout handling for long-running operations

## Usage

### Prerequisites

- Set the `OPENAI_API_KEY` environment variable with your OpenAI API key
- For web search functionality, set `GOOGLE_API_KEY` and `GOOGLE_SEARCH_ENGINE_ID`

```bash
export OPENAI_API_KEY=your_openai_api_key
export GOOGLE_API_KEY=your_google_api_key
export GOOGLE_SEARCH_ENGINE_ID=your_search_engine_id
```

### Running the Example

```bash
go run main.go
```

## Code Explanation

### Creating the OpenAI Client

```go
openaiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"),
    openai.WithModel("gpt-4o-mini"),
)
```

### Creating the Agent Registry

```go
registry := orchestration.NewAgentRegistry()
```

### Creating Specialized Agents

```go
// Create specialized agents
createAndRegisterAgents(registry, openaiClient)
```

The example creates and registers several specialized agents:
- Research agent with web search capabilities
- Math agent with calculator capabilities
- Creative agent for content generation
- Summary agent for information condensation

### Creating the Orchestrator

```go
orchestrator := orchestration.NewLLMOrchestrator(registry, openaiClient, mem)
```

### Executing Queries

```go
response, err := orchestrator.Execute(ctx, query)
```

The orchestrator:
1. Creates a plan using the LLM to break down the query into steps
2. Executes each step in the correct order based on dependencies
3. Handles any failures or deadlocks gracefully
4. Generates a final response using the results from all steps

## How It Works

The LLM orchestrator uses a sophisticated planning and execution system:

1. **Plan Creation**:
   - The LLM analyzes the query and creates a plan with multiple steps
   - Each step specifies which agent to use, what input to provide, and its dependencies
   - Dependencies are specified using step indices (e.g., "step_0", "step_1")

2. **Plan Execution**:
   - Steps are executed in the correct order based on their dependencies
   - Results from previous steps can be referenced in later steps using {{step_X}} syntax
   - The system tracks character counts for research and summary outputs
   - Deadlock detection prevents infinite loops

3. **Error Handling**:
   - Failed steps are handled gracefully
   - The system can proceed with partial results if some steps fail
   - Fallback mechanisms ensure a response is generated even if the preferred agent is unavailable

4. **Final Response**:
   - The final agent generates a comprehensive response using all available results
   - If some steps were not completed, the final agent works with the available information

## Customization

You can customize this example by:
- Adding more specialized agents with different capabilities
- Modifying the system prompts of existing agents
- Adding more tools to the agents
- Implementing custom dependency resolution strategies
- Adding more sophisticated error handling and recovery mechanisms 