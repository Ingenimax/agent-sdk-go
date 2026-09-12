# Guardrails

This document explains how to use the Guardrails component of the Agent SDK.

## Overview

Guardrails provide safety mechanisms to ensure that your agents behave responsibly and ethically. They can filter, modify, or block responses that violate policies or contain harmful content.

## Building a pipeline

Guardrails are wired up in code. Construct the ones you want and put them in a
`Pipeline`, which is what an agent accepts:

```go
gr := guardrails.NewPipeline([]guardrails.Guardrail{
    guardrails.NewPiiFilter(guardrails.RedactAction),
    guardrails.NewContentFilter([]string{"badword"}, guardrails.BlockAction),
    guardrails.NewTokenLimit(4000, nil, guardrails.RedactAction, "end"),
}, logging.New())

a, err := agent.NewAgent(
    agent.WithLLM(llmClient),
    agent.WithMemory(memory.NewConversationBuffer()),
    agent.WithGuardrails(gr),
)
```

> **Changed:** `Pipeline` previously exposed only `ProcessRequest` /
> `ProcessResponse`, while `agent.WithGuardrails` requires an
> `interfaces.Guardrails` (`ProcessInput` / `ProcessOutput`) — and no type in the
> module implemented that interface. **Every guardrail in this package could be
> constructed and none could be attached to an agent.** `Pipeline` now implements
> the interface directly.

### Configuration is not read from the environment

`GUARDRAILS_ENABLED` and `GUARDRAILS_CONFIG_PATH` are parsed into
`config.Guardrails` and **nothing reads them**. There is no YAML rule format, no
`guardrails.New`, and no config-file loader. Setting those variables has no
effect; build the pipeline in code.

## When guardrails run

Input guardrails run **before** the user message is written to memory, and the
guarded text is what gets persisted and sent to the model. An input your
guardrail rejects is not written to memory at all.

That ordering is load-bearing. The LLM providers build their request from
memory, not from the value returned by `ProcessInput` — for example
`pkg/llm/openai/message_history.go` appends the prompt argument only when memory
is `nil`:

```go
} else {
    // Only append current user message when memory is nil
    messages = append(messages, openai.UserMessage(prompt))
}
```

> **Changed:** both run paths previously wrote the raw input to memory and only
> then called `ProcessInput`, assigning the result to a local variable the
> providers never read. With memory configured — the normal case — **the model
> was shown the unguarded input and input guardrails had no effect at all.**
> After upgrading you may see rejections fire for the first time. See
> [Upgrading](upgrading.md#input-guardrails-now-actually-apply).

### Output guardrails

`ProcessOutput` runs on every terminal path that produces a complete response,
via `finishRun`, which guards and then persists — so the transcript holds what
the guardrail approved and the next turn replays that rather than raw model
output. A response the guardrail rejects is not persisted.

| Path | Guarded |
| --- | --- |
| `runWithoutExecutionPlanWithToolsTracked` | yes |
| `runWithExecutionPlan` (the default with tools) | yes |
| `generateRoleResponse` | yes |
| plan create / modify replies | yes |
| custom run function | output only — see below |
| remote agent | output only — see below |
| `RunStream` — role response | yes |
| `RunStream` — streamed content | **persisted text only — see below** |

> Earlier releases had `ProcessOutput` at a single call site, reached by one of
> five paths — and not the default one, since `requirePlanApproval` defaults to
> `true`. On a default configuration, output guardrails did not run.

#### Limitation: streamed deltas are not retroactively guarded

On `RunStream`, content reaches the consumer incrementally. By the time the full
response exists, the caller has already seen the raw deltas. Guardrails are
applied to the accumulated text **before it is written to memory**, so the
transcript is clean and the next turn is not poisoned — but the bytes already
streamed are not recalled.

If you need output filtering to reach the consumer, use the non-streaming path.
Guarding a stream as it is produced requires a policy for partial text (buffer
to a boundary? redact retroactively? abort mid-stream?) that has not been
decided.

#### Limitation: custom run functions

`agent.WithCustomRunFunction` and `agent.WithCustomRunStreamFunction` replace the
whole local run path. Their **output is guarded**, but they receive **no input
guardrails, no memory write and no tracing** — that is inherent to replacing the
run path. Call `ProcessInput` yourself inside a custom function if you need it.

## Using Guardrails with an Agent

`agent.WithGuardrails` takes any `interfaces.Guardrails`:

```go
type Guardrails interface {
    ProcessInput(ctx context.Context, input string) (string, error)
    ProcessOutput(ctx context.Context, output string) (string, error)
}
```

`*guardrails.Pipeline` satisfies it. Returning an error from either method aborts
the run, and the returned string replaces the content.

## Actions

Every guardrail is constructed with an `Action`, which decides what the pipeline
does when that guardrail triggers:

| Action | Effect |
| --- | --- |
| `BlockAction` | The run fails with `blocked by <type> guardrail`. Content is not sent or returned. |
| `RedactAction` | The guardrail's modified text replaces the content and the run continues. |
| `WarnAction` | The **original** content continues unchanged; the modification is logged only. |

An action the pipeline does not recognise falls through the switch and behaves
like `WarnAction`: the content passes unmodified.

Guardrails run in the order given, each seeing the previous one's output, so
redactions compose.

## Built-in guardrails

### PiiFilter

```go
guardrails.NewPiiFilter(guardrails.RedactAction)
```

Redacts email addresses, phone numbers, SSNs, credit card numbers and IP
addresses, replacing each match with `[REDACTED <kind>]`. Pattern-based, so it
catches formatted values and misses unformatted or unusual ones — treat it as
defence in depth, not a guarantee.

### ContentFilter

```go
guardrails.NewContentFilter([]string{"badword", "c++"}, guardrails.RedactAction)
```

Case-insensitive whole-word matching against a fixed list, replacing matches
with `****`.

> **Changed:** blocked words are now escaped and anchored per word. Previously
> they were interpolated into the pattern raw, so a word containing regex
> metacharacters (`c++`, `a.b`) **panicked at construction**, and an **empty word
> list matched at every word boundary — redacting the whole text**.

### TokenLimit

```go
guardrails.NewTokenLimit(4000, nil, guardrails.RedactAction, "end")
```

Truncates text over `maxTokens`. `truncateMode` is `"end"` (default), `"start"`
or `"middle"`. A `nil` counter uses `SimpleTokenCounter`, which counts
whitespace-separated fields — an approximation, not a real tokenizer. Supply
your own `TokenCounter` if the limit needs to match a provider's accounting.

### RateLimit

```go
guardrails.NewRateLimit(60, guardrails.BlockAction)
```

Caps requests per minute, counted per organization via
`multitenancy.GetOrgID` (requests with no org ID share a `"default"` bucket).

**Use `BlockAction` with this one.** Its modified-text slot carries the
"rate limit exceeded" diagnostic rather than a redacted prompt, so under
`RedactAction` the user's prompt is replaced by that message and sent to the
model. The counter map also retains one entry per organization for the process
lifetime.

### ToolRestriction

```go
guardrails.NewToolRestriction([]string{"search"}, guardrails.BlockAction)
```

**This does not restrict tool execution.** It scans request *text* for the
literal phrase `use tool <name>` and flags names outside the allow-list. A model
calling a tool through the provider's tool-calling API never passes through it.
To actually control tool access, use the hooks package
([capabilities.md](capabilities.md)) — `hooks.AllowList` gates tools at
invocation.

## Middleware

`LLMMiddleware` and `ToolMiddleware` wrap a single LLM or tool with a pipeline,
for guarding one component rather than a whole agent:

```go
guarded := guardrails.NewToolMiddleware(myTool, gr)
```

## Writing a custom guardrail

Implement `Guardrail` and add it to a pipeline:

```go
type MyGuardrail struct{}

func (g *MyGuardrail) Type() guardrails.GuardrailType { return "my_guardrail" }
func (g *MyGuardrail) Action() guardrails.Action      { return guardrails.RedactAction }

// CheckRequest returns (triggered, modifiedText, error). modifiedText is used
// only when Action is RedactAction.
func (g *MyGuardrail) CheckRequest(ctx context.Context, request string) (bool, string, error) {
    if strings.Contains(request, "forbidden") {
        return true, strings.ReplaceAll(request, "forbidden", "[REDACTED]"), nil
    }
    return false, request, nil
}

func (g *MyGuardrail) CheckResponse(ctx context.Context, response string) (bool, string, error) {
    return false, response, nil
}
```

Returning a non-nil error from either method aborts the run — reserve it for
genuine failures, not policy violations.

Alternatively, implement `interfaces.Guardrails` yourself and pass it to
`agent.WithGuardrails` directly; you do not have to use this package.

## Multi-tenancy

There is no multi-tenant guardrails wrapper in the SDK. `RateLimit` is the only
built-in that is org-aware. To vary policy per tenant, implement
`interfaces.Guardrails` and select a pipeline inside `ProcessInput` /
`ProcessOutput`:

```go
type PerOrg struct {
    byOrg    map[string]*guardrails.Pipeline
    fallback *guardrails.Pipeline
}

func (p *PerOrg) pipeline(ctx context.Context) *guardrails.Pipeline {
    orgID, err := multitenancy.GetOrgID(ctx)
    if err != nil {
        return p.fallback
    }
    if gr, ok := p.byOrg[orgID]; ok {
        return gr
    }
    return p.fallback
}

func (p *PerOrg) ProcessInput(ctx context.Context, input string) (string, error) {
    return p.pipeline(ctx).ProcessInput(ctx, input)
}

func (p *PerOrg) ProcessOutput(ctx context.Context, output string) (string, error) {
    return p.pipeline(ctx).ProcessOutput(ctx, output)
}
```
