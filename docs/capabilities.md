# Agent Capabilities

Seven subsystems, each usable on its own. They compose, but nothing here
requires adopting the rest.

| Package | What it gives you |
| --- | --- |
| [`pkg/session`](#sessions) | Conversation identity, scoped state, persistence |
| [`pkg/run`](#background-runs) | Run IDs, a registry, cancel-by-ID |
| [`pkg/hooks`](#hooks-and-plugins) | Lifecycle callbacks, bundled as plugins |
| [`pkg/skill`](#skills) | SKILL.md bundles with progressive disclosure |
| [`pkg/consolidation`](#memory-consolidation) | Idle distillation of memory into facts |
| [`pkg/llm/cachepolicy`](#prompt-caching) | Honest per-provider caching capability |
| [`pkg/orchestration`](#llm-driven-transfer) | Model-chosen handoff between agents |

---

## Sessions

```go
store := session.NewInMemoryStore()
mgr := session.NewManager(store, session.WithAppName("support"))

s, err := mgr.Create(ctx, "org1", "user42")

// Bind before handing the context to an agent.
ctx = mgr.Bind(ctx, s)
answer, err := myAgent.Run(ctx, "how do I rotate a key?")
```

`Bind` is the important call. Memory derives its scope from an org ID and a
conversation ID and **fails if either is missing**, which is why a background or
scheduled run — with no inbound request to inherit from — could not write to
memory at all. `Bind` supplies both.

`Session.ID` *is* the conversation ID. There is no second ID space and no
mapping table, so a session and its transcript cannot drift apart. A session
does not own the transcript; `pkg/memory` already stores it under the same key.

### Scoped state

| Prefix | Scope |
| --- | --- |
| *(none)* | This session only |
| `user:` | Every session belonging to that user |
| `app:` | Every session in the application |
| `temp:` | Never persisted — dropped on save |

```go
s.Set("draft", "in progress")              // private
s.Set(session.UserPrefix+"tz", "Europe/Oslo") // follows the user
s.Set(session.TempPrefix+"scratch", "...")    // never stored
mgr.Save(ctx, s)
```

Prefixes resolve at the storage layer into separate rows, so shared values are
genuinely shared rather than copied — and deleting one session does not take its
user's state with it.

### Persistence

```go
store, err := session.NewSQLStore(db, session.Postgres) // or session.SQLite
err = store.Migrate(ctx)
```

`NewSQLStore` takes a live `*sql.DB` rather than a DSN, so a service that
already has a pool does not open a second one.

### Fork

```go
branch, err := mgr.Fork(ctx, "org1", s.ID)
```

Carries state, not the transcript, and records the parent under `forked_from`.

---

## Background runs

```go
mgr := run.NewManager()

h := mgr.Start(ctx, myAgent, "long job",
    run.WithDetached(),
    run.WithTimeout(10*time.Minute),
)

fmt.Println(h.ID())
answer, err := h.Wait(ctx)
```

`WithDetached` is opt-in. Without it, a run started inside an HTTP handler dies
when the client disconnects — rarely what "background" is meant to mean. With
it, only `Cancel` or a timeout stops the run, so pair it with a timeout.

```go
for _, info := range mgr.Active() {
    log.Printf("%s running %s for %s", info.ID, info.AgentName, info.Duration())
}
err := mgr.Cancel(runID)
```

`run.FromContext` returns the run **ID**, not the handle. A handle is mutable
and would be reachable from arbitrary tool code — including MCP tools backed by
remote processes — which could then mark a run succeeded or forge its result.

`pkg/task` is a different concern: task lifecycle with human approval, not a
single invocation in flight.

---

## Hooks and plugins

A hook intervenes at one point; a plugin is a named bundle of hooks. They attach
to the tool set rather than to any provider's loop, so they reach every provider.

```go
reg := hooks.NewRegistry(
    hooks.Logging(log.Printf),
    hooks.AllowList("search", "read_file"),
    hooks.Redact("[REDACTED]", hooks.CommonSecretPatterns()...),
    hooks.TruncateResults(8000),
)

agent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithTools(myTools...),
    agent.WithToolDecorator(reg.Decorate("my-agent")),
)
```

### Writing one

```go
reg.Register(hooks.Plugin{
    Name: "business-hours",
    BeforeTool: func(ctx context.Context, call hooks.ToolCall) (hooks.Outcome, error) {
        if call.Tool == "send_email" && isOutOfHours() {
            return hooks.Outcome{
                Decision: hooks.Deny,
                Message:  "Email is disabled outside business hours. Summarise instead.",
            }, nil
        }
        return hooks.Outcome{}, nil // the zero value allows
    },
})
```

Four behaviours worth knowing:

- **The zero `Outcome` allows.** A hook that returns early must not block every
  tool call.
- **A denial is a result, not an error.** Providers turn a tool error into a
  tool-result message and continue, so an error would just look like a broken
  tool. A readable denial lets the model adapt.
- **A panic in `BeforeTool` fails the call; in `AfterTool` it does not.** A
  pre-hook gates execution; a post-hook is observational, and a crash in metrics
  must not discard a result already produced.
- **The first denial short-circuits.** Later hooks do not run against a call
  that will not happen.

---

## Skills

```
skills/
  incident-triage/
    SKILL.md
    runbook.md
```

```markdown
---
name: incident-triage
description: Triage a production incident and identify the failing component
---

1. Check the error rate dashboard.
2. Correlate with recent deploys.
```

```go
lib, err := skill.LoadDir("./skills")
agent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithTools(skill.Tools(lib)...),
)
```

Only names and descriptions sit in context; instructions load when the model
calls `load_skill`. Twenty skills cost twenty one-line descriptions rather than
twenty documents, which is what makes a large library affordable.

**Skills are never executed.** Bundled files are read, not run. A format that
silently executes code from a cloned folder is a supply-chain problem. Resource
reads are confined to the skill's directory; traversal and symlink escape are
both refused.

---

## Memory consolidation

```go
store := consolidation.NewInMemoryStore()
tracker := consolidation.NewActivityTracker(memory.NewConversationBuffer())
c := consolidation.New(llm, store)

agent, err := agent.NewAgent(agent.WithLLM(llm), agent.WithMemory(tracker))

sched := consolidation.NewScheduler(tracker, c,
    consolidation.WithIdleFor(15*time.Minute),
    consolidation.WithPlanCallback(func(scope consolidation.IdleScope, p consolidation.Plan) {
        log.Print(p.Diff())
    }),
)
sched.Start(ctx)
defer sched.Stop()
```

It **proposes**; it does not rewrite. `Plan` produces a reviewable set of facts
and writes nothing. Apply happens only via `Apply` or `WithAutoApply`, which is
off by default — a model rewriting memory with no diff and no undo should be a
deliberate choice. Facts go to a separate store; the transcript is never touched.

Consolidation triggers on **both** quiet time and accumulated writes. Idle time
alone would consolidate a conversation that barely started; write count alone
would consolidate one mid-exchange.

### Multi-replica

An in-process ticker fires once per replica. Drive it externally instead:

```go
sched := consolidation.NewScheduler(tracker, c, consolidation.WithIdleFor(0))
sched.RunOnce(ctx) // from a CronJob
```

---

## Prompt caching

Providers differ, and pretending otherwise costs money silently.

```go
cap := cachepolicy.For("openai")
fmt.Println(cap.Control)       // automatic
fmt.Println(cap.Honors())      // false -- CacheConfig has no effect
fmt.Println(cap.ReportsUsage)  // true  -- cache reads are reported

if err := cachepolicy.CheckConfig("gemini", true); err != nil {
    log.Warn(err) // caching was configured but will do nothing
}
```

| Provider | Control | Reports usage |
| --- | --- | --- |
| anthropic | explicit | yes |
| openai, azureopenai | automatic | yes |
| gemini | none | no |
| bedrock, deepseek, ollama, vllm, vertex | none | no |

Control and reporting are tracked separately because OpenAI has no control
surface yet reports cache reads — a single "supports caching" boolean would
misrepresent it. OpenAI and Azure now populate
`TokenUsage.CacheReadInputTokens`, which is the only evidence a caller has that
caching happened.

---

## LLM-driven transfer

```go
reg := orchestration.NewAgentRegistry()
reg.Register("triage", triageAgent)
reg.Register("billing", billingAgent)

tool := orchestration.NewTransferTool(reg, map[string]string{
    "triage":  "Route a request to the right specialist",
    "billing": "Answer invoicing and payment questions",
})

answer, chain, err := orchestration.RunWithTransfers(
    ctx, reg, tool, "triage", "why was I charged twice?", 3)
// chain: [triage billing]
```

The target is constrained to an enum of registered agents, so a hallucinated
name is unrepresentable rather than a wasted turn. This replaces parsing
`[HANDOFF:agent:reason]` out of the model's prose, which required teaching a
bespoke syntax and broke whenever the model paraphrased it.

Transfer chains are bounded twice — by `maxTransfers` and by a repeat-visit
counter — because two agents can otherwise bounce a request between them, each
pass costing a request.

---

## See also

- [Multi-agent patterns](multi-agent.md) — sequential, parallel, loop, graph
- [Tool pipeline](tool-pipeline.md) — the seam hooks attach to
- [Memory](memory.md) · [Sub-agents](subagents.md) · [Upgrading](upgrading.md)
