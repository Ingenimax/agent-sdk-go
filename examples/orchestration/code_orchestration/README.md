# Code Orchestration Example

This example demonstrates how to implement code-based orchestration for specialized agents in the Agent SDK.

## Features

- Code-based orchestration to route queries to specialized agents
- Multiple specialized agents with different capabilities:
  - Research agent with web search capabilities
  - Math agent with calculator capabilities
  - Creative agent for content generation
  - Summary agent for information condensation
- Rule-based routing logic
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
orchestrator := orchestration.NewCodeOrchestrator(registry)
```

### Executing Queries

```go
response, err := orchestrator.Execute(ctx, query)
```

The orchestrator:
1. Analyzes the user query using predefined rules
2. Determines which specialized agent is best suited to handle it
3. Routes the query to the appropriate agent
4. Returns the agent's response

## How It Works

The code orchestrator uses rule-based logic to:

1. Check for specific keywords or patterns in the user's query
2. Match those patterns to the capabilities of available agents
3. Select the appropriate agent to handle the query
4. Route the query to that agent and return its response

For example:
- Queries containing math expressions or calculations are routed to the Math agent
- Queries asking for research or information are routed to the Research agent
- Queries requesting creative content are routed to the Creative agent
- Queries asking for summaries are routed to the Summary agent

## Comparison with LLM Orchestration

Unlike the LLM-based orchestrator, the code orchestrator:
- Uses explicit rules rather than LLM reasoning
- Is more predictable and deterministic
- May be less flexible for handling edge cases
- Is typically faster as it doesn't require an additional LLM call
- Can be more cost-effective as it doesn't use additional tokens

## Customization

You can customize this example by:
- Adding more specialized agents with different capabilities
- Modifying the routing rules in the orchestrator
- Adding more tools to the agents
- Implementing more sophisticated pattern matching logic

## Example Queries

- "Explain the impact of quantum computing on cryptography"
- "Design a sustainable urban garden for a small apartment"
- "Compare and contrast different machine learning algorithms for image recognition"
- "Research the latest advancements in renewable energy technologies"
- "Investigate the effects of artificial intelligence on job markets"
- "Explore the relationship between diet and cognitive performance"
- "Research the history and evolution of blockchain technology"
- "Analyze the environmental impact of electric vehicles compared to traditional cars"

## Workflow Structure

The default workflow follows this pattern:

1. **Research Task**: Gathers information about the query
2. **Math Task**: Performs calculations based on the research (depends on research)
3. **Creative Task**: Generates creative content (depends on research and math)
4. **Summary Task**: Produces the final response (depends on all previous tasks)

This workflow can be customized for different types of queries by modifying the `createWorkflow` function.

## Troubleshooting

### API Key Errors
If you see authentication errors, make sure your OpenAI API key is correctly set and has sufficient quota.

### Organization ID Error
If you see an error like:
```
Error: final task failed: agent execution failed: failed to add user message to memory: organization ID not found in context: no organization ID found in context
```

This is related to missing context values that the agent system needs to function properly.

Solutions:
1. Make sure you're using the latest version of the code which includes the necessary context values
2. If you're implementing your own agent system, ensure you're setting the organization ID in the context:
   ```go
   ctx = multitenancy.WithOrgID(ctx, "your-org-id")
   ```
3. You may also need to set conversation ID and user ID depending on your implementation

### Timeout Errors
If tasks take too long to complete, you may need to increase the timeout in the context creation:
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
```

### Missing Tool Errors
If the research agent can't use web search, make sure you've set the Google API key and Search Engine ID environment variables.
