# PMG Fork Patches

This fork of `github.com/Ingenimax/agent-sdk-go` carries local patches on top of
an upstream tag. Keep each patch as a discrete, well-described commit so rebasing
onto future upstream releases stays cheap.

## Goal

Make reasoning/thinking work uniformly across providers, including when tools are
attached. Reasoning is gated on the reasoning **config**, not a model-name
allowlist — the operator selects a reasoning-capable model (via SSM in
staging/prod), and the SDK does not second-guess the model name.

## Patches

### 1. Anthropic — adaptive thinking, including the tools path

**File:** `pkg/llm/anthropic/client.go` (`generateInternal` and `GenerateWithTools`;
`GenerateWithToolsDetailed` delegates to `GenerateWithTools`).

**Problem:**
- The tools path reserved a reasoning token budget but never set `req.Thinking`,
  so extended thinking was silently dropped whenever tools were attached.
- The non-tools path only enabled thinking when the model matched a hardcoded
  dated-ID allowlist (`SupportsThinking`), which was already stale for current
  models (e.g. `claude-sonnet-4-6` was absent).
- The emitted shape was legacy extended thinking (`{type:"enabled",
  budget_tokens}` + `temperature=1.0`), which **400s on Opus 4.7/4.8** — those
  models accept only adaptive thinking and reject `temperature`/`top_p`.

**Fix:** When `LLMConfig.EnableReasoning` is set, both paths now emit adaptive
thinking (`req.Thinking = &ReasoningSpec{Type:"adaptive"}`) and clear
`temperature`/`top_p`. **Claude 4.6+ only** — legacy extended thinking and
pre-4.6 models are not supported (operator selects a 4.6+ model). The
`SupportsThinking` model allowlist is no longer consulted for gating.

**Tests:** `pkg/llm/anthropic/thinking_tools_test.go`
- `TestGenerateWithToolsEnablesThinking` — asserts `thinking.type=="adaptive"`,
  no `budget_tokens`, and no `temperature`/`top_p` in the request body.
- `TestGenerateWithToolsNoThinkingWhenDisabled` — asserts no `thinking` block
  when reasoning is disabled.

### 2. Gemini — thinking config in the tools paths + no model allowlist

**File:** `pkg/llm/gemini/client.go` (`GenerateWithTools` initial call + post-tool
synthesis call) and `pkg/llm/gemini/streaming.go`.

**Problem:** Thinking config (`c.thinkingConfig`, set via `WithThinkingBudget`)
was applied only in the plain `Generate` path. The tool-calling request builds
and the post-tool synthesis call never set `config.ThinkingConfig`, so thinking
was dropped with tools. All sites were also gated on the `SupportsThinking`
model allowlist.

**Fix:** Apply `config.ThinkingConfig` in the tools paths too, gated only on
`c.thinkingConfig != nil` (no model allowlist).

### 3. OpenAI — no change

`GenerateWithTools` already sets `req.ReasoningEffort` from
`LLMConfig.Reasoning` for reasoning models (`client.go`), gated by the
forward-compatible `isReasoningModel` prefix check (`o1`/`o3`/`o4`/`gpt-5`). That
gate is protective — non-reasoning models reject `reasoning_effort` — so it is
left as-is.

## Consumer note (genworkflow)

Anthropic reasoning is adaptive-only here, so the resolver should set
`EnableReasoning: true` (not the legacy `Reasoning: "low"` string, which is a
no-op on Anthropic) and must use a Claude 4.6+ model. The depth knob on modern
models is `output_config.effort`, which this SDK does not yet model.
