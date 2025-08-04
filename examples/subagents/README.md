# Sub-Agents Example

This example demonstrates how to use the new sub-agents feature in the Agent SDK. It shows how to create specialized agents and compose them into a main orchestrator agent that can delegate tasks.

## Overview

The example creates a main agent with three specialized sub-agents:
- **MathAgent**: Handles mathematical calculations and numerical analysis
- **ResearchAgent**: Performs research and information retrieval
- **CodeAgent**: Assists with programming and software development tasks

## Features

- **Automatic Tool Wrapping**: Sub-agents are automatically wrapped as tools that the main agent can invoke
- **Context Preservation**: Context and conversation history are maintained across sub-agent calls
- **Circular Dependency Detection**: The SDK automatically detects and prevents circular dependencies
- **Recursion Depth Limits**: Built-in protection against infinite recursion

## Setup

1. Set your OpenAI API key:
```bash
export OPENAI_API_KEY=your_api_key_here
```

2. (Optional) For web search functionality in the ResearchAgent:
```bash
export GOOGLE_API_KEY=your_google_api_key
export GOOGLE_SEARCH_ENGINE_ID=your_search_engine_id
```

## Running the Example

```bash
go run main.go
```

## Example Queries

Try these queries to see how the main agent delegates to sub-agents:

1. **Math queries** (delegated to MathAgent):
   - "What is 15% of 250?"
   - "Solve the equation: 2x + 5 = 17"
   - "Calculate the compound interest on $1000 at 5% for 3 years"

2. **Research queries** (delegated to ResearchAgent):
   - "What are the latest developments in quantum computing?"
   - "Tell me about the history of artificial intelligence"
   - "What are the health benefits of meditation?"

3. **Code queries** (delegated to CodeAgent):
   - "Write a Python function to find prime numbers"
   - "How do I implement a binary search in Go?"
   - "Debug this code: [paste your code]"

4. **General queries** (handled by MainAgent):
   - "Hello, how are you?"
   - "What can you help me with?"
   - "Explain your capabilities"

## How It Works

1. **Agent Creation**: Each specialized agent is created with its own LLM, memory, tools, and system prompt
2. **Sub-Agent Registration**: Sub-agents are registered with the main agent using `WithAgents()`
3. **Automatic Tool Creation**: The SDK automatically wraps each sub-agent as a tool
4. **Task Delegation**: The main agent's LLM decides when to delegate tasks based on the sub-agents' descriptions
5. **Result Integration**: Sub-agent responses are seamlessly integrated into the main conversation

## Key Implementation Details

### Creating a Sub-Agent
```go
mathAgent, err := agent.NewAgent(
    agent.WithName("MathAgent"),
    agent.WithDescription("Specialized in mathematical calculations"),
    agent.WithLLM(llm),
    agent.WithTools(calculator),
    agent.WithSystemPrompt("You are a math expert..."),
)
```

### Registering Sub-Agents
```go
mainAgent, err := agent.NewAgent(
    agent.WithName("MainAgent"),
    agent.WithLLM(llm),
    agent.WithAgents(mathAgent, researchAgent, codeAgent),
    agent.WithSystemPrompt("You can delegate to specialized agents..."),
)
```

### Important: WithDescription()
The `WithDescription()` option is crucial for sub-agents. It helps the main agent understand when to delegate tasks. Make descriptions clear and specific about the sub-agent's capabilities.

## Architecture Benefits

1. **Modularity**: Each agent is self-contained with its own expertise
2. **Reusability**: Sub-agents can be reused across different main agents
3. **Scalability**: Easy to add new specialized agents
4. **Maintainability**: Clear separation of concerns
5. **Testing**: Each agent can be tested independently

## Production Considerations

- **Timeouts**: Sub-agent calls have default 30-second timeouts (configurable)
- **Error Handling**: Failures in sub-agents are gracefully handled
- **Tracing**: Full observability support for debugging
- **Resource Management**: Connection pooling and efficient memory usage
- **Security**: Input validation and output sanitization

## Additional Examples

### Depth Validation Example

To see a demonstration of the recursion depth validation system, check out the depth validation example:

```bash
cd depth_validation
go run main.go
```

This example shows:
- How hierarchical agent structures are validated
- Maximum depth enforcement (5 levels by default)
- Runtime recursion depth checking
- Complex branching hierarchies

## Troubleshooting

1. **Circular Dependencies**: The SDK will error if agents reference each other circularly
2. **Max Depth Exceeded**: Default maximum recursion depth is 5 levels
3. **Timeout Issues**: Adjust timeouts if sub-agents need more processing time
4. **Memory Issues**: Each sub-agent maintains its own memory context