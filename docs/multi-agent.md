# Multi-Agent Patterns

`pkg/orchestration` composes several agents into one workflow. Four named
patterns cover the common shapes; `Graph` handles the rest.

## Sequential

Each agent's result feeds the next.

```go
reg := orchestration.NewAgentRegistry()
reg.Register("research", researchAgent)
reg.Register("draft", draftAgent)
reg.Register("edit", editAgent)

result, err := orchestration.Sequential(reg, "research", "draft", "edit").
    Run(ctx, "write about tide pools")
```

The final agent's output is the workflow's result.

## Parallel

Every agent runs concurrently on the same input; a combiner merges the results.

```go
result, err := orchestration.Parallel(reg, "summarize",
    "legal-review", "security-review", "cost-review").
    Run(ctx, "assess this proposal")
```

The combiner receives each branch's output appended to its input, in the order
the branches were declared.

## Loop

One agent runs repeatedly, each iteration receiving the previous result.

```go
result, err := orchestration.Loop(reg, "refiner", 3).Run(ctx, "draft")
```

The iteration count is required. There is no model-driven escape hatch yet, and
an accidental unbounded loop against a paid API is not a failure mode worth
offering by default. A count below 1 is clamped to 1.

## Graph

For shapes the named patterns do not cover, declare the dependencies directly.

```go
result, err := orchestration.Graph(reg, "report",
    orchestration.GraphNode{ID: "fetch", AgentID: "fetcher"},
    orchestration.GraphNode{ID: "enrich", AgentID: "enricher", DependsOn: []string{"fetch"}},
    orchestration.GraphNode{ID: "verify", AgentID: "verifier", DependsOn: []string{"fetch"}},
    orchestration.GraphNode{ID: "report", AgentID: "writer", DependsOn: []string{"enrich", "verify"}},
).Run(ctx, "quarterly numbers")
```

Independent nodes run concurrently. `enrich` and `verify` above both depend only
on `fetch`, so they run in parallel.

## Scheduling guarantees

- A task starts as soon as **all** its dependencies have completed, not in
  declaration order.
- Independent tasks run **concurrently**.
- Each task runs **exactly once**.
- A task whose dependency **failed** is skipped rather than run with a missing
  input, and the failure propagates to the final result.
- **Cycles**, unknown dependency IDs, duplicate task IDs and an unknown final
  task are rejected before anything runs, with an error naming the problem.

> Earlier releases scheduled through a monitor goroutine that could deadlock
> when two tasks finished simultaneously, could launch the same task twice, and
> wrote shared result maps from many goroutines without a lock. See
> [Upgrading](upgrading.md).

## Inspecting or extending a pattern

`Pattern.Workflow()` exposes the underlying workflow, so a composed pattern can
be inspected or extended before it runs.

```go
p := orchestration.Sequential(reg, "research", "draft")
p.Workflow().AddTask("audit", "auditor", "check it", []string{"step-1"})
p.Workflow().SetFinalTask("audit")
result, err := p.Run(ctx, "topic")
```

## Related

- [Sub-agents](subagents.md) — an agent used as a tool by another agent
- [A2A](a2a.md) — remote agents over the A2A protocol
- [Execution plans](execution_plan.md) — plan-then-approve within a single agent
