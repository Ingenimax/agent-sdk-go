# Max Depth Visual Guide

## How Depth is Calculated

### Linear Chain (Depth = Number of Arrows)

```
✅ VALID: Depth = 2 (within limit of 5)
┌─────────┐    ┌─────────┐    ┌─────────┐
│  Main   │───▶│ Level1  │───▶│ Level2  │
│ Agent   │    │ Agent   │    │ Agent   │
└─────────┘    └─────────┘    └─────────┘
     1              2              3
   levels         levels         levels
```

```
✅ VALID: Depth = 5 (at maximum limit)
┌─────┐   ┌─────┐   ┌─────┐   ┌─────┐   ┌─────┐   ┌─────┐
│  A0 │──▶│  A1 │──▶│  A2 │──▶│  A3 │──▶│  A4 │──▶│  A5 │
└─────┘   └─────┘   └─────┘   └─────┘   └─────┘   └─────┘
    1         2         3         4         5         6
 levels    levels    levels    levels    levels    levels
```

```
❌ INVALID: Depth = 6 (exceeds limit of 5)
┌─────┐   ┌─────┐   ┌─────┐   ┌─────┐   ┌─────┐   ┌─────┐   ┌─────┐
│  A0 │──▶│  A1 │──▶│  A2 │──▶│  A3 │──▶│  A4 │──▶│  A5 │──▶│  A6 │
└─────┘   └─────┘   └─────┘   └─────┘   └─────┘   └─────┘   └─────┘
                                           ❌ FAILS HERE
```

### Branching Structure (Depth = Longest Path)

```
✅ VALID: Depth = 3 (branching doesn't count against depth)
                    ┌─────────┐
                    │  Main   │
                    │ Agent   │
                    └────┬────┘
                         │
         ┌───────────────┼───────────────┐
         ▼               ▼               ▼
    ┌─────────┐    ┌─────────┐    ┌─────────┐
    │ Business│    │Research │    │  Code   │
    │ Agent   │    │ Agent   │    │ Agent   │
    └────┬────┘    └─────────┘    └────┬────┘
         │                               │
    ┌────┼────┐                    ┌────┼────┐
    ▼    ▼    ▼                    ▼    ▼    ▼
┌─────┐ ┌─────┐ ┌─────┐        ┌─────┐ ┌─────┐ ┌─────┐
│Data │ │Calc │ │File │        │Debug│ │Test ││Build│
│Agent│ │Agent│ │Agent│        │Agent│ │Agent│ │Agent│
└─────┘ └─────┘ └─────┘        └─────┘ └─────┘ └─────┘

Depth = 3 (Main -> Business -> Data is longest path)
Width = 8 total agents (unlimited)
```

## Two Types of Validation

### 1. Initialization Time (Build Time)

```go
// When creating agents, the entire tree is validated
mainAgent, err := agent.NewAgent(
    agent.WithAgents(subAgent1, subAgent2), // ← Validation happens here
)
if err != nil {
    // "agent tree depth X exceeds maximum allowed depth 5"
}
```

**Algorithm:**
```
validateAgentTree(rootAgent, maxDepth=5)
├── checkCircularDependency(rootAgent) // Prevent A→B→A loops
└── calculateMaxDepth(rootAgent)       // Measure longest path
    ├── if currentDepth == 0: return 0
    ├── for each subAgent:
    │   └── depth = calculateMaxDepth(subAgent, currentDepth+1)
    └── return max(all depths)
```

### 2. Runtime (Execution Time)

```go
// During execution, context tracks call depth
func (at *AgentTool) Run(ctx context.Context, input string) (string, error) {
    depth := at.getRecursionDepth(ctx)    // Check current depth
    if depth > 5 {                        // Enforce limit
        return "", fmt.Errorf("maximum sub-agent recursion depth exceeded")
    }
    ctx = context.WithValue(ctx, "recursion_depth", depth+1) // Increment
    return at.agent.Run(ctx, input)       // Call sub-agent
}
```

**Call Stack Example:**
```
MainAgent.Run(ctx depth=0)
├── calls MathAgent_agent.Run(ctx depth=1)
    ├── calls CalculatorAgent_agent.Run(ctx depth=2)
        ├── calls ValidationAgent_agent.Run(ctx depth=3)
            ├── calls LoggerAgent_agent.Run(ctx depth=4)
                ├── calls MetricsAgent_agent.Run(ctx depth=5)
                    ├── calls ❌ BLOCKED: depth > 5
```

## Context Flow

```
Context Values During Execution:

┌─────────────────┐  ctx["recursion_depth"] = 0
│   MainAgent     │  ctx["sub_agent_name"] = ""
│                 │  ctx["parent_agent"] = ""
└─────────────────┘
         │ calls MathAgent_agent
         ▼
┌─────────────────┐  ctx["recursion_depth"] = 1
│   MathAgent     │  ctx["sub_agent_name"] = "MathAgent"
│                 │  ctx["parent_agent"] = "MainAgent"
└─────────────────┘
         │ calls CalculatorAgent_agent
         ▼
┌─────────────────┐  ctx["recursion_depth"] = 2
│CalculatorAgent  │  ctx["sub_agent_name"] = "CalculatorAgent"
│                 │  ctx["parent_agent"] = "MathAgent"
└─────────────────┘
```

## Real-World Examples

### ✅ Good: Shallow Specialization
```
CustomerService (Main)
├── TechnicalSupport (handles tech issues)
├── BillingSupport (handles billing)
└── GeneralInquiry (handles everything else)

Depth = 1, Fast responses, Clear delegation
```

### ✅ Good: Department Structure
```
Company (CEO)
├── Engineering (CTO)
│   ├── Frontend (handles UI)
│   ├── Backend (handles API)
│   └── DevOps (handles infrastructure)
└── Sales (handles customers)

Depth = 2, Mirrors real org structure
```

### ⚠️ Caution: Complex Pipeline
```
DataPipeline (Main)
└── DataIngestion
    └── DataValidation
        └── DataTransformation
            └── DataStorage
                └── DataNotification (depth = 5, at limit)

Depth = 5, Each step adds latency
```

### ❌ Bad: Over-engineered
```
User Request
└── RequestRouter
    └── TaskClassifier
        └── ComplexityAnalyzer
            └── ResourceAllocator
                └── ExecutionPlanner
                    └── ❌ Would exceed depth limit

Too many layers, diminishing returns
```

## Performance Impact

### Depth vs Response Time
```
Depth 1: ~1-2 seconds    (1 LLM call)
Depth 2: ~2-4 seconds    (2 LLM calls)
Depth 3: ~3-6 seconds    (3 LLM calls)
Depth 4: ~4-8 seconds    (4 LLM calls)
Depth 5: ~5-10 seconds   (5 LLM calls)
```

### Memory Usage
```
Each level adds:
- Context overhead
- Memory buffer
- Tool registration
- Validation state

Depth 5 ≈ 5x base memory usage
```

## Best Practices

### ✅ Do:
- Keep depth ≤ 3 for most use cases
- Use branching instead of deep nesting
- Measure performance at each depth
- Cache responses when possible

### ❌ Don't:
- Create unnecessary intermediate agents
- Exceed depth 5 (will fail)
- Create circular dependencies
- Ignore performance implications

## Debugging Depth Issues

### Check Current Depth
```go
depth := agent.GetRecursionDepth(ctx)
fmt.Printf("Current depth: %d\n", depth)
```

### Visualize Hierarchy
```go
func printAgentTree(a *agent.Agent, indent int) {
    fmt.Printf("%s%s\n", strings.Repeat("  ", indent), a.GetName())
    for _, sub := range a.GetSubAgents() {
        printAgentTree(sub, indent+1)
    }
}
```

### Error Messages
- `"agent tree depth X exceeds maximum allowed depth 5"` → Reduce nesting
- `"maximum sub-agent recursion depth exceeded"` → Check for loops
- `"circular dependency detected"` → Fix A→B→A patterns