# Tool Pipeline

The tool pipeline lets you interpose on every tool call an agent makes — to
audit it, deny it, rewrite its arguments, bound its output, or time it — without
touching any LLM provider.

## Why this exists

The tool-calling loop does not live in `pkg/agent`. It lives inside each
provider: `pkg/llm/openai`, `pkg/llm/anthropic`, `pkg/llm/gemini` and five
others each run their own iteration and dispatch. There is no single place in
the agent where "a tool is about to be called" happens.

What *is* single is the `[]interfaces.Tool` slice handed to the provider. Wrap
the tools themselves and you reach every provider's loop without editing any of
them. That makes this slice the one interposition seam in the SDK.

## Basic usage

A `ToolDecorator` transforms the tool set an agent is about to expose:

```go
type ToolDecorator func([]interfaces.Tool) []interfaces.Tool
```

Register one with `WithToolDecorator`:

```go
agent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithTools(searchTool, deployTool),
    agent.WithToolDecorator(auditing),
)
```

## Writing a decorator

A decorator wraps each tool and returns a slice of the same length in the same
order:

```go
type auditedTool struct {
    inner interfaces.Tool
    log   *slog.Logger
}

func (t *auditedTool) Name() string        { return t.inner.Name() }
func (t *auditedTool) Description() string { return t.inner.Description() }

func (t *auditedTool) Parameters() map[string]interfaces.ParameterSpec {
    return t.inner.Parameters()
}

func (t *auditedTool) Run(ctx context.Context, input string) (string, error) {
    return t.inner.Run(ctx, input)
}

func (t *auditedTool) Execute(ctx context.Context, args string) (string, error) {
    start := time.Now()
    result, err := t.inner.Execute(ctx, args)
    t.log.Info("tool call",
        "tool", t.inner.Name(),
        "args", args,
        "duration", time.Since(start),
        "err", err,
    )
    return result, err
}

// Forward the optional interfaces. See below -- skipping this silently changes
// how the tool is presented.
func (t *auditedTool) DisplayName() string {
    name, _ := agent.ForwardOptionalToolInterfaces(t.inner)
    return name
}

func (t *auditedTool) Internal() bool {
    _, internal := agent.ForwardOptionalToolInterfaces(t.inner)
    return internal
}

// Let callers recover the tool underneath.
func (t *auditedTool) Unwrap() interfaces.Tool { return t.inner }

func auditing(toolSet []interfaces.Tool) []interfaces.Tool {
    out := make([]interfaces.Tool, len(toolSet))
    for i, tl := range toolSet {
        out[i] = &auditedTool{inner: tl, log: slog.Default()}
    }
    return out
}
```

### Denying a call

A decorator can refuse to run the underlying tool. Return a string the model can
read and act on rather than an error, unless you want the run to fail:

```go
func (t *policyTool) Execute(ctx context.Context, args string) (string, error) {
    if !t.allowed(t.inner.Name(), args) {
        return "This tool call was denied by policy. Do not retry it.", nil
    }
    return t.inner.Execute(ctx, args)
}
```

Returning an error here does not abort the run: the providers convert a tool
error into a tool-result message and continue the loop.

## Ordering

Decorators are applied in registration order, and the usage tracker is always
innermost:

```
outermost -> your last decorator
             your first decorator
innermost -> usage tracker
             the actual tool
```

The tracker sits innermost deliberately: a decorator that denies a call must act
before the tracker sees it, or usage accounting reports work that never ran.

Register observation-only decorators first and denying decorators last, so a
denial short-circuits everything outside it.

## Where decorators apply

The pipeline is applied at all three places an agent composes tools:

| Call site | Path |
| --- | --- |
| `agent.go` | Synchronous `Run` / `RunDetailed` |
| `streaming.go` | `RunStream` |
| `executionplan.NewExecutor` | The execution-plan path |

The third matters most and is easy to miss. `executionplan.NewExecutor` captures
the tool slice at construction and calls `tool.Execute` directly, bypassing the
provider loop entirely — and `requirePlanApproval` defaults to `true`, so it is
the SDK's default path. A decorator that reached only the first two call sites
would silently not apply to most runs.

## Unwrapping

Wrapping erases concrete type identity: after decoration, an assertion such as
`tool.(*tools.AgentTool)` fails. Implement `Unwrap() interfaces.Tool` on your
decorator and callers can recover the original:

```go
original := agent.UnwrapTool(decorated) // walks the whole chain
```

## Forwarding optional interfaces

`interfaces.Tool` is four methods, but a tool may additionally implement
`interfaces.ToolWithDisplayName` and `interfaces.InternalTool`, both discovered
by type assertion. A decorator that does not forward them silently changes how
the tool is presented in UIs and traces.

`agent.ForwardOptionalToolInterfaces` returns both values so each decorator does
not hand-roll the logic — and so a newly added optional interface is handled in
one place:

```go
displayName, internal := agent.ForwardOptionalToolInterfaces(inner)
```

## See also

- [Tools](tools.md) — writing the tools themselves
- [Sub-agents](subagents.md) — agents exposed as tools
- [Token usage tracking](token-usage-tracking.md) — what the innermost decorator records
