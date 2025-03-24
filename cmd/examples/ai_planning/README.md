# AI Task Planning Example

This example demonstrates how to use AI-based planning for task management in the Agent SDK. It showcases different types of planners and how they can be used to generate task plans.

## What You'll Learn

- How to use the AI planning interfaces in the task system
- Different types of AI planners (real LLM-based and mock implementations)
- How to integrate custom AI planners with the core planner
- Using domain-specific system prompts to tailor AI planning
- How fallback behavior works when AI planning fails

## Setup

The example can use either a real LLM via the OpenAI API or a mock implementation:

### Using a Real LLM (Recommended)

Set your OpenAI API key as an environment variable:

```bash
export OPENAI_API_KEY=your_api_key_here
```

With a valid API key, the example will use the OpenAI API to generate real, dynamic plans tailored to each task.

### Using the Mock Implementation

If no API key is provided, the example will automatically fall back to using a mock implementation that returns predefined responses. You'll see a warning message at runtime.

## Running the Example

```bash
go run main.go
```

## Code Explanation

### AI Planning Interface

The example uses the `AIPlanner` interface, which is implemented by different planning strategies:

```go
// AIPlanner is an interface for AI-based planning services
type AIPlanner interface {
    GeneratePlan(ctx context.Context, task *core.Task) (string, error)
}
```

### LLM Integration

The example integrates with a real LLM via the interfaces.LLM interface:

```go
// Set up the LLM client
llmClient := setupLLMClient(logger)

// Create a planner that uses the LLM
simpleLLMPlanner := planner.NewSimpleLLMPlanner(llmClient, logger)
```

The `SimpleLLMPlanner` takes care of:

1. Formatting task data into an effective prompt
2. Setting the appropriate system message to guide the LLM's response
3. Handling the LLM API communication
4. Processing and validating the response

### Custom System Prompts

The example demonstrates how to create a domain-specific planner by using a custom system prompt:

```go
// Create a planner with a custom system prompt for software development
softwareDevSystemPrompt := `You are an expert software development planner...`

customPromptPlanner := planner.NewSimpleLLMPlannerWithSystemPrompt(
    llmClient, 
    logger, 
    softwareDevSystemPrompt
)
```

This approach allows you to:

1. Tailor the planner for specific domains (software development, marketing, research, etc.)
2. Set expectations for the format and content of the plans
3. Define the phases and structure you want the plans to follow
4. Emphasize domain-specific best practices and methodologies

### Planner Implementations

The example demonstrates several planner implementations:

1. **SimpleLLMPlanner**: Uses a real LLM to generate detailed, context-aware plans
2. **Custom System Prompt Planner**: Uses a domain-specific prompt for software development tasks
3. **MockAIPlanner**: A simple mock implementation for testing
4. **CorePlanner with SimpleLLMPlanner**: The main planner using LLM capabilities
5. **CorePlanner with MockAIPlanner**: Using a specific mock planner implementation

### Example Output

The program generates several different plans for a variety of tasks:

1. A data processing task plan with the real LLM planner
2. An API integration task plan with the mock planner
3. A mobile app feature task plan with the custom software development prompt
4. A bug fix task plan with the CorePlanner using the real LLM
5. A database optimization task plan with a CorePlanner using a mock AI planner
6. A fallback scenario when no valid task is provided

### Real-World Integration

In a production environment, you would typically:

1. Use a proper configuration system for LLM API keys and settings
2. Configure more sophisticated prompts tailored to your specific task domain
3. Include error handling, retry logic, and rate limiting
4. Add caching or optimization strategies for similar tasks

## Implementation Details

The `CorePlanner` is designed to use AI planning when available but gracefully fall back to simpler planning methods if:

- No AI planner is configured
- The AI planner fails to generate a plan
- The input is not a valid task

This ensures robustness while still leveraging AI capabilities when possible.

## Configuration Options

The LLM client used by the planner can be configured with various options:

```go
client := openai.NewClient(
    apiKey,
    openai.WithModel("gpt-4o"), // Use different models
    openai.WithTemperature(0.5), // Adjust temperature
    openai.WithLogger(logger), // Add logging
)
```

## Domain-Specific Planning

You can create specialized planners for different domains by crafting appropriate system prompts:

1. **Software Development**: Include phases for design, implementation, testing, and deployment
2. **Data Science**: Focus on data collection, preprocessing, analysis, and visualization
3. **Marketing**: Structure around research, strategy, implementation, and measurement
4. **Research**: Organize by literature review, hypothesis formation, experimentation, and analysis

Each domain-specific planner can produce plans that follow industry-standard methodologies and best practices.

## Next Steps

To enhance this example:

1. Try different LLM providers (Anthropic, Azure OpenAI, etc.)
2. Experiment with different system prompts and task structures
3. Add more sophisticated prompt engineering and context building
4. Implement caching of similar plans to improve performance
5. Add metrics and logging to track plan quality and success rates 