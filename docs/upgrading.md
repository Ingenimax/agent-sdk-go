# Upgrading

This page covers behaviour changes, removals and new APIs in the current
unreleased development line, on top of `v0.2.66`.

Read the [Behaviour changes](#behaviour-changes) section even if you use none of
the removed APIs. Several fixes change what the SDK *does* without changing any
signature, and one of them means a feature you may believe is protecting you was
not running at all.

---

## Security fix: remote configuration loading removed

**If you loaded agent configuration over HTTP, this affects you directly.**

`pkg/agentconfig` used to fetch agent YAML from a configuration service and
unmarshal it straight into an `agent.AgentConfig`. That struct carries an `MCP`
section naming a local executable, its arguments and its environment, and those
values reach `pkg/mcp` as:

```go
cmd := exec.CommandContext(ctx, commandPath, config.Args...)
cmd.Env = append(os.Environ(), config.Env...)
```

The only validation before that call was that the binary path is absolute,
exists, and is not a directory. `/bin/sh` satisfies all three. A compromised or
hostile configuration service therefore had arbitrary local code execution
inside the agent process at `NewAgent` time, with every API key in the process
environment inherited by the child process.

The transport has been removed rather than hardened.

### What was removed

| Removed | Replacement |
| --- | --- |
| `agentconfig.LoadAgentFromRemote` | `agentconfig.LoadAgentFromLocal` |
| `agentconfig.WithRemoteOnly` | none needed; loading is always local |
| `agentconfig.WithRemotePriorityMerge` | `agentconfig.MergeAgentConfig` directly |
| `agentconfig.ConfigurationClient`, `NewClient` | none |
| `agentconfig.LoadFromEnvironment` | none |
| `agentconfig.LoadAndMergeWithViper` | none |
| `agentsdk.NewConfigClient` | none |
| `agentsdk.LoadDeploymentConfig` | none |
| `AGENT_DEPLOYMENT_ID` requirement | no longer read |
| `examples/deployment_config` | removed |

`LoadAgentConfig`, `LoadAgentFromLocal`, `PreviewAgentConfig`,
`LoadAgentWithOptions` and `LoadAgentWithVariables` all still exist and now load
from local YAML only. Local configuration, **including the `mcp:` section**, is
unchanged: the transport was the problem, not the schema.

These remain as deprecated aliases so existing code keeps compiling:
`WithLocalFallback` (use `WithLocalPath`), `WithLocalOnly` (now a no-op),
`LoadAgentAuto` (identical to `LoadAgentFromLocal`),
`MergeStrategyRemotePriority` and `MergeStrategyLocalPriority`.

### If you depended on remote configuration

Fetch the YAML yourself, from a source you trust, and write it to disk before
constructing the agent:

```go
// Your own retrieval, with your own authentication and integrity checks.
data, err := fetchConfigFromYourService(ctx, agentName, environment)
if err != nil {
    return err
}
path := filepath.Join(configDir, agentName+".yaml")
if err := os.WriteFile(path, data, 0o600); err != nil {
    return err
}

agent, err := agentconfig.LoadAgentFromLocal(ctx, agentName, environment)
```

If you do this, **validate the `mcp:` section before writing the file**. Any
config source that can name a command is a source that can run it.

`go mod tidy` also drops `viper` and seven transitive dependencies, which only
the removed loader needed.

---

## Security fix: prompt template path containment

`prompts`' template loader checked containment with `strings.HasPrefix` on
cleaned paths, which is a string test rather than a path test. Two ways out of
the base directory passed it:

- a sibling directory whose name merely starts with the base name — base
  `prompts` admitted `prompts-evil/steal.tmpl`;
- a symlink pointing outside, because only the candidate path was resolved and
  the base never was.

Containment is now decided by resolving symlinks on both sides and asking
`filepath.Rel` whether the result is genuinely inside.

If you relied on either behaviour to load templates from outside the base
directory, pass that directory as the base instead.

## Behaviour changes

These change what the SDK does without changing any signature you call.

### Input guardrails now actually apply

**This is the most important item on this page.**

Both run paths used to write the raw user input to memory and only afterwards
call `guardrails.ProcessInput`, assigning the result to a local variable. The
LLM providers build their request from memory, not from that variable — for
example `pkg/llm/openai/message_history.go` appends the prompt argument only
when memory is `nil`:

```go
} else {
    // Only append current user message when memory is nil
    messages = append(messages, openai.UserMessage(prompt))
}
```

So with memory configured — the normal case — **the model was shown the
unguarded input and `ProcessInput` had no effect on the request at all.** If you
rely on input guardrails to strip secrets or block prompt injection, they were
not doing so.

Guardrails now run before the memory write, so the guarded text is what gets
persisted and sent. An input your guardrail rejects is no longer written to
memory at all.

You may see previously-hidden guardrail rejections start firing after
upgrading. That is the feature working.

### Output guardrails now run on every complete-response path

`ProcessOutput` previously had a single call site, reached by one of five
terminal paths — and not the default one, since `requirePlanApproval` defaults
to `true`. On a default configuration, output guardrails did not run at all.

Every path that produces a complete response now goes through `finishRun`, which
guards and then persists, so the transcript holds guarded text. Custom run
functions and remote agents have their output guarded at the run funnel.

Two limitations remain and are documented in
[Guardrails](guardrails.md#output-guardrails): streamed deltas have already
reached the consumer before the full response exists, so only the persisted text
is guarded; and a custom run function still bypasses input guardrails, the
memory write and tracing.

### Conversation summaries no longer destroy history

`ConversationSummary` stored each new summary with a plain map assignment while
the summarizer only ever saw the current buffer — and the raw messages had
already been cleared from the only store. Every summarization after the first
therefore discarded all earlier history, irrecoverably.

The prior summary is now folded into the next summarization prompt, so the
record compounds. `Metadata["count"]` accumulates across rounds instead of
describing only the most recent window.

### Summarization fires above a threshold of 100

`NewConversationSummary` built its internal buffer with a hardcoded maximum of
100 messages, while the trigger compared against your configured
`WithMaxBufferSize`. Any value above 100 was unreachable, so summarization was
silently disabled with no error. The buffer is now sized to your threshold.

If you set `WithMaxBufferSize` above 100, summarization will begin running for
the first time.

### Redis summarization thresholds were transposed

`memory/factory.go` called `WithSummarization(llm, messageThreshold,
summaryCount)` with its two integers swapped. With the defaults, summarization
fired every 3 messages instead of every 10 and retained 10 summaries instead
of 3. Your `max_summaries` and `summary_after_messages` YAML keys now mean what
they say.

### `ExecutionSummary.ToolCalls` counts calls, not distinct tools

`addToolCall` returned from inside its deduplication loop before incrementing,
so `ToolCalls` was always exactly `len(UsedTools)`. Forty calls to one tool
reported `1`. It now counts invocations; `UsedTools` remains the distinct set.

Dashboards reading this field will show higher, correct numbers.

### Cancelling a run now stops its sub-agents

`AgentTool.Execute` called `context.WithoutCancel` when the parent deadline was
earlier than its own timeout, intending to extend the sub-agent's budget. That
also severed cancellation propagation. With the default 30 minute sub-agent
timeout, cancelling a parent run left the sub-agent running for up to half an
hour, still writing into shared memory.

Sub-agents are now always children of the caller's context. Their own timeout
still bounds them, but **the parent deadline wins when it is shorter**. If you
relied on sub-agents outliving their parent, that no longer happens.

### `Agent` is safe for concurrent `Run`

`runLocalWithTracking` assigned `a.planGenerator` on every run with no lock, and
`requirePlanApproval` defaults to `true`, so this was the default configuration.
Two concurrent `Run` calls raced on an interface field. Because sub-agents are
shared `*Agent` pointers, a parent fanning out to two sub-agent tool calls hit
it without the caller writing any concurrent code.

The generator is now built per run. `a.planGenerator` is written once at
construction and read-only thereafter.

### Tracing no longer hides memory capabilities

`Agent.GetAllConversations`, `GetConversationMessages` and
`GetMemoryStatistics` used a bare type assertion for
`interfaces.ConversationMemory`. `tracing.TracedMemory` implements only the three
core `Memory` methods, so wrapping a `RedisMemory` in tracing made the assertion
miss and all three returned empty results with no error.

See [Memory capability discovery](#memory-capability-discovery) for the new
helpers.

### One sub-agent recursion counter

`pkg/agent` declared its own context keys with the same string values as the
unexported ones in `pkg/tools`. Go context keys compare by type *and* value, so
these were two independent counters that could not observe each other, and only
the `pkg/tools` one guarded anything. `pkg/agent` now delegates; its public API
is unchanged.

### Streaming and non-streaming see the same tools

The streaming path built its own tool list and never added lazy MCP tools, so an
agent configured with them saw a different tool set depending on whether it was
streamed. Both paths now share one assembly step.

---

### Redis summarization now archives what it replaces

`RedisMemory` with `WithSummarization` used to trim the summarized messages
away permanently. Summarization is lossy and, for Redis, the raw text existed
nowhere else — so a poor summary destroyed the only copy of the transcript and
nothing could audit what it had dropped.

Those messages are now moved to an archive list first, atomically with the trim,
and read back with `ArchivedMessages(ctx)`. The archive key is the conversation
key plus `:archive`.

**This costs storage you were not previously using.** If that is not the
trade-off you want, opt out:

```go
memory.NewRedisMemory(client,
    memory.WithSummarization(llmClient, 50, 5),
    memory.WithoutArchive(),
)
```

The trim point is also pair-safe now: it no longer cuts between an assistant
turn carrying `tool_calls` and the tool messages answering it, which used to
make the *next* turn fail with an unpaired-tool-result error from the provider.

### A configured `runtime.timeout` now actually applies

`runtime.timeout` in YAML was parsed and stored on the agent, and then read
nowhere. An agent configured with a timeout ran without one — and when the
caller passed `context.Background()`, a wedged provider call hung forever.

It is now applied to the run on both the synchronous and streaming paths.

**A long-running agent that previously ran to completion may now be cut off.**
If you set `runtime.timeout` years ago as documentation rather than as a limit,
check the value is one you actually want:

```yaml
runtime:
  timeout: 5m   # this is now enforced
```

A deadline already on the caller's context is left alone when it is tighter —
whichever bound is stricter wins, and an agent-level default never extends a
deadline the caller deliberately set.

### `WithAgents` no longer leaves tools for detached sub-agents

`WithAgents` replaced the sub-agent list but only *appended* the generated
`{name}_agent` tools. Calling it twice left the first set's tools in place, so
the model could still call sub-agents that were no longer attached. Stale
entries are now removed before the new set is added.

## Removals

### `pkg/workflow`

Deleted. It was 84 lines of type declarations with no execution logic and no
importers anywhere, and its `AddStep` corrupted the list it built.

Real composition already ships in [`pkg/orchestration`](../pkg/orchestration):
DAG execution with dependencies and concurrency, LLM-planned decomposition, and
handoff routing.

### Anthropic extended cache TTL

`NewCacheControlWithTTL("1h")` emitted `{"type":"ephemeral","ttl":"1h"}`, but the
client never sends an `anthropic-beta` header. Extended cache TTL requires that
header, so the field was inert: **callers who configured hourly caching were
billed at the five-minute rate while believing otherwise.**

Removed: `anthropic.WithCacheTTL`, `interfaces.CacheConfig.CacheTTL`,
`anthropic.CacheControl.TTL`, `anthropic.NewCacheControlWithTTL`. The `cache_ttl`
YAML key is no longer read; existing configs carrying it still load and the key
is ignored.

The other caching controls — `CacheSystemMessage`, `CacheTools`,
`CacheConversation` — are unaffected and continue to work at the five-minute
default.

---

## Deprecations

### `interfaces.TaskExecutor`

Deprecated, scheduled for removal. No type in this module satisfies it. Compiling
the conformance assertion against the only executor shipped fails:

```
*TaskExecutor does not implement interfaces.TaskExecutor (missing method CancelTask)
```

It also lacks `GetTaskStatus`, `ExecuteWorkflow` and `ExecuteWorkflowAsync`, and
its `ExecuteStep`/`ExecuteTask` take concrete `*core.Task` and `*core.Step`
rather than the `interface{}` the contract declares.

`task/service.NewInMemoryTaskService` used to take this interface as its third
parameter, which made it impossible to construct with anything at all. It now
takes `service.TaskRunner`, the single method it actually calls:

```go
type TaskRunner interface {
    ExecuteTask(ctx context.Context, t *task.Task) error
}
```

That takes a few lines to implement. Note that the shipped
`*task/executor.TaskExecutor` still does not satisfy it, for a separate reason:
it operates on `*task/core.Task` while the service operates on `*task.Task`, and
those are two distinct structs rather than an alias. `pkg/task` and
`pkg/task/core` are parallel type hierarchies for the same concept; reconciling
them is left undone rather than papered over with a conversion shim.

The `ExecuteWorkflow` methods are documented as initiating "a temporal workflow".
Temporal is not a dependency of this module and never has been.

Depend on the concrete `*task/executor.TaskExecutor`, or declare an interface in
your own package containing only the methods you use.
`agentsdk.NewTaskExecutor()` is unaffected.

---

## New APIs

### Tool decorator pipeline

Interpose on every tool call without editing any provider. See
[Tool pipeline](tool-pipeline.md).

```go
agent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithTools(myTool),
    agent.WithToolDecorator(auditDecorator),
)
```

### Memory capability discovery

`interfaces.Memory` is three methods; richer behaviour is discovered by type
assertion, which a decorator silently defeats. Decorators should now implement
`interfaces.MemoryUnwrapper`, and callers should use the helpers instead of bare
assertions:

```go
// Instead of: mem.(interfaces.ConversationMemory)
if convMem, ok := interfaces.AsConversationMemory(mem); ok {
    conversations, err := convMem.GetAllConversations(ctx)
}

if adminMem, ok := interfaces.AsAdminConversationMemory(mem); ok {
    // ...
}

inner := interfaces.UnwrapMemory(mem) // innermost Memory
```

`tracing.TracedMemory` implements `Unwrap`. If you have written your own `Memory`
decorator, implement it too — otherwise your decorator hides every optional
capability of whatever it wraps.

### Sub-agent context helpers

`pkg/tools` now exports the sub-agent context helpers it already owned:
`WithSubAgentContext`, `GetRecursionDepth`, `GetSubAgentName`, `GetParentAgent`,
`GetInvocationID`, `IsSubAgentCall`, `ValidateRecursionDepth` and
`MaxRecursionDepth`. The identically-named functions in `pkg/agent` delegate to
them and are unchanged for callers.

### Guardrails can now be attached to an agent

`agent.WithGuardrails` takes an `interfaces.Guardrails`
(`ProcessInput`/`ProcessOutput`). `guardrails.Pipeline` exposed only
`ProcessRequest`/`ProcessResponse`, and no type in the module implemented the
interface — so every guardrail in `pkg/guardrails` could be constructed and none
could be attached to anything.

`Pipeline` now implements it:

```go
gr := guardrails.NewPipeline([]guardrails.Guardrail{
    guardrails.NewPiiFilter(guardrails.RedactAction),
}, logging.New())

a, err := agent.NewAgent(
    agent.WithLLM(llmClient),
    agent.WithGuardrails(gr),
)
```

Two `ContentFilter` defects were fixed at the same time, both from
interpolating the blocked words into the pattern unescaped: a word containing
regex metacharacters (`c++`) **panicked at construction**, and an **empty word
list matched at every word boundary**, replacing the whole text with asterisks.
Words are now escaped, and a filter with no words matches nothing.

`docs/guardrails.md` previously documented an API that does not exist
(`guardrails.New`, `WithConfigPath`, `AddRule`, `Check`, `NewMultiTenant`, a YAML
rule format). `GUARDRAILS_ENABLED` and `GUARDRAILS_CONFIG_PATH` are parsed into
`config.Guardrails` and read by nothing; setting them has no effect. Build the
pipeline in code.

### `agentconfig.WithLocalPath`

Replaces `WithLocalFallback`, which is now a deprecated alias. "Fallback" no
longer describes anything, since there is no remote source to fall back from.

---

## Testing

`internal/testutil` provides shared, concurrency-safe test doubles for this
module's own tests: `FakeLLM`, `FakeTool`, `FakeMemory`,
`FakeConversationMemory`, `FakeTracer` and `FakeSpan`. See
[Development](development.md#test-doubles).

It is under `internal/` deliberately and is not importable by downstream
modules.
