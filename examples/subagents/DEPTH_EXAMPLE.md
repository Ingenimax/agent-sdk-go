# Max Depth Example for Sub-Agents

This example demonstrates how the maximum depth validation works in the sub-agents feature.

## Understanding Max Depth

The max depth limit prevents two types of issues:
1. **Deep nesting** - Agents calling sub-agents in a very long chain
2. **Infinite recursion** - Agents calling each other in loops

## How Depth is Calculated

Depth is calculated as the number of edges (connections) between agents, not the number of agents:

```
MainAgent -> SubAgent1 -> SubAgent2 -> SubAgent3
         ^1            ^2            ^3

Depth = 3 (three connections/edges)
```

## Two Levels of Protection

### 1. Initialization Time Validation

When you create an agent with `NewAgent()`, the system validates the entire agent tree:

```go
// This validation happens automatically
if len(agent.subAgents) > 0 {
    // Check for circular dependencies
    if err := agent.validateSubAgents(); err != nil {
        return nil, err
    }

    // Validate agent tree depth (max 5 levels)
    if err := validateAgentTree(agent, 5); err != nil {
        return nil, err
    }
}
```

### 2. Runtime Validation

During execution, each sub-agent call increments a counter in the context:

```go
// In AgentTool.Run()
depth := at.getRecursionDepth(ctx)
if depth > 5 { // Maximum recursion depth
    return "", fmt.Errorf("maximum sub-agent recursion depth exceeded")
}
ctx = context.WithValue(ctx, "recursion_depth", depth+1)
```

## Running the Example

```bash
# Set your OpenAI API key
export OPENAI_API_KEY=your_api_key_here

# Run the depth example
go run depth_example.go
```

## Expected Output

```
=== Sub-Agents Max Depth Example ===

1. Creating a valid shallow hierarchy (depth = 2):
   Hierarchy: MainAgent -> Level1Agent -> Level2Agent (depth = 2)
   ✅ Success: Shallow hierarchy created

2. Creating a valid deep hierarchy (depth = 5, at limit):
   Hierarchy: Agent0 -> Agent1 -> Agent2 -> Agent3 -> Agent4 -> Agent5 (depth = 5)
   ✅ Success: Deep hierarchy created at maximum depth

3. Attempting to create invalid deep hierarchy (depth = 6, exceeds limit):
   ✅ Expected error caught: agent tree depth 6 exceeds maximum allowed depth 5

4. Demonstrating runtime recursion depth checking:
   Testing context recursion depth limits:
   - Depth 0: ✅ Within limit
   - Depth 1: ✅ Within limit
   - Depth 2: ✅ Within limit
   - Depth 3: ✅ Within limit
   - Depth 4: ✅ Within limit
   - Depth 5: ✅ Within limit
   - Depth 6: ❌ Exceeds limit (error: maximum recursion depth 5 exceeded)

5. Creating a complex branching hierarchy:
   ✅ Successfully created branching hierarchy:
      RootAgent
      ├── BusinessAgent
      │   ├── DataAgent
      │   └── AnalyticsAgent
      └── TechnicalAgent
          └── ReportAgent
   Max depth: 3, Total agents: 6
```

## Key Concepts Demonstrated

### Example 1: Shallow Hierarchy
- Shows a simple 3-level hierarchy
- Depth = 2 (well within limits)
- Demonstrates basic agent chaining

### Example 2: Deep Hierarchy at Limit
- Creates exactly 5 levels of nesting
- Shows the maximum allowed depth
- All agents created successfully

### Example 3: Too Deep Hierarchy
- Attempts to create 6 levels of nesting
- Fails during agent creation
- Error is caught and handled gracefully

### Example 4: Runtime Depth Checking
- Shows how context tracks recursion depth
- Demonstrates validation at each level
- Shows the exact point where depth is exceeded

### Example 5: Branching Hierarchy
- Multiple sub-agents at each level
- Depth is measured by longest path, not total agents
- Shows that width (branching) is unlimited

## Important Notes

### Depth vs Count
- **Depth** = longest path from root to leaf
- **Count** = total number of agents
- Only depth is limited, not count

### Branching is Allowed
```
MainAgent
├── SubAgent1
├── SubAgent2
├── SubAgent3
└── SubAgent4
```
This is depth = 1, even with 4 sub-agents

### Why Max Depth = 5?
- Prevents stack overflow
- Limits complexity
- Ensures reasonable response times
- Avoids excessive API calls

### Customizing Max Depth

Currently hardcoded to 5, but you can modify in:
1. `pkg/agent/agent.go` - Line where `validateAgentTree(agent, 5)` is called
2. `pkg/agent/context.go` - `MaxRecursionDepth` constant
3. `pkg/tools/agent_tool.go` - Recursion check in `Run()` method

## Use Cases for Different Depths

### Depth 1-2: Simple Delegation
- Main agent with specialized helpers
- Direct task delegation
- Quick responses

### Depth 3-4: Multi-tier Architecture
- Department -> Team -> Individual pattern
- Hierarchical organizations
- Complex workflows

### Depth 5: Maximum Complexity
- Enterprise-level orchestration
- Multi-stage processing pipelines
- Deep specialization chains

## Troubleshooting

### "agent tree depth X exceeds maximum allowed depth 5"
- Reduce nesting levels
- Combine some agents
- Use branching instead of deep nesting

### "maximum sub-agent recursion depth exceeded"
- Check for accidental loops
- Verify context is properly managed
- Ensure sub-agents don't call parents

### Performance Considerations
- Each level adds latency
- Deep hierarchies = more API calls
- Consider caching at each level
