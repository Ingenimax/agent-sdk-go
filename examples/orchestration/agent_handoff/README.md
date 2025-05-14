# Agent Handoff Example

This example demonstrates how to implement agent handoff functionality in the Agent SDK, allowing specialized agents to pass control to each other based on query requirements.

## Features

- Agent handoff mechanism for specialized task routing
- Multiple specialized agents with different capabilities:
  - General agent for everyday questions
  - Research agent with web search capabilities
  - Math agent with calculator capabilities
- LLM-based router for intelligent query routing
- Conversation memory for each agent
- Error handling and recovery

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
openaiClient := openai.NewClient(apiKey)
```

### Creating Specialized Agents

```go
// Create general agent
generalAgent, err := createGeneralAgent(openaiClient)
registry.Register("general", generalAgent)

// Create research agent
researchAgent, err := createResearchAgent(openaiClient)
registry.Register("research", researchAgent)

// Create math agent
mathAgent, err := createMathAgent(openaiClient)
registry.Register("math", mathAgent)
```

### Agent System Prompts with Handoff Instructions

```go
// General agent prompt
`You are a helpful general-purpose assistant. You can answer questions on a wide range of topics.
If you encounter a question that requires specialized knowledge in research or mathematics, you should hand off to a specialized agent.

To hand off to the research agent, respond with: [HANDOFF:research:needs specialized research]
To hand off to the math agent, respond with: [HANDOFF:math:needs mathematical calculation]

Otherwise, provide helpful and accurate responses to the user's questions.`
```

### Creating the Router and Orchestrator

```go
// Create router
router := orchestration.NewLLMRouter(openaiClient)

// Create orchestrator
orchestrator := orchestration.NewOrchestrator(registry, router)
```

### Handling Requests

```go
// Prepare context for routing
routingContext := map[string]interface{}{
    "agents": map[string]string{
        "general":  "General-purpose assistant for everyday questions and tasks",
        "research": "Specialized in research, fact-finding, and information retrieval",
        "math":     "Specialized in mathematical calculations and problem-solving",
    },
}

// Handle the request
result, err := orchestrator.HandleRequest(ctx, query, routingContext)
```

## How It Works

The agent handoff system works through these steps:

1. The user query is initially sent to the router, which determines the most appropriate agent
2. The selected agent processes the query
3. If the agent determines it can't handle the query effectively, it can "hand off" to another agent by returning a special response format: `[HANDOFF:agent_id:reason]`
4. The orchestrator detects this handoff signal and routes the query to the specified agent
5. The new agent processes the query and returns its response
6. The orchestrator returns the final response to the user

This approach allows for:
- Initial intelligent routing based on query content
- Dynamic re-routing when an agent discovers it's not the best fit
- Specialized handling of different query types
- Seamless user experience with appropriate agent selection

## Handoff Format

The handoff format is:
```
[HANDOFF:agent_id:reason]
```

Where:
- `agent_id` is the ID of the agent to hand off to
- `reason` is a brief explanation of why the handoff is occurring

## Customization

You can customize this example by:
- Adding more specialized agents with different capabilities
- Modifying the system prompts to change handoff behavior
- Adding more tools to the agents
- Implementing custom routing logic
- Adding a feedback loop where agents can provide context to the next agent
