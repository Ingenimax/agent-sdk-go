package agent

import (
	"context"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tracing"
)

// applyRunIdentity stamps the agent name and, when configured, the org ID onto
// the context. Both run paths need this before anything that reads either.
func (a *Agent) applyRunIdentity(ctx context.Context) context.Context {
	ctx = tracing.WithAgentName(ctx, a.name)
	if a.orgID != "" {
		ctx = multitenancy.WithOrgID(ctx, a.orgID)
	}
	return ctx
}

// beginRun performs the steps both the sync and streaming run paths share, in
// the order they have to happen: establish identity, apply input guardrails,
// then persist the resulting user message.
//
// The ordering matters and was previously wrong in both paths. Each wrote the
// RAW input to memory and only afterwards called guardrails.ProcessInput,
// assigning the result to a local variable. But the providers build their
// request from memory, not from that variable -- pkg/llm/openai/message_history.go
// appends the prompt argument only when memory is nil:
//
//	} else {
//	    // Only append current user message when memory is nil
//	    messages = append(messages, openai.UserMessage(prompt))
//	}
//
// So with memory configured -- the normal case -- the model was shown the
// unguarded input, and ProcessInput silently had no effect on the request. For
// a guardrail whose job is stripping secrets or blocking prompt injection, that
// is the whole feature failing quietly. Guardrails now run first, and the
// guarded text is what gets persisted and sent.
//
// Returns the updated context and the guarded input.
func (a *Agent) beginRun(ctx context.Context, input string) (context.Context, string, error) {
	ctx = a.applyRunIdentity(ctx)

	if a.guardrails != nil {
		guarded, err := a.guardrails.ProcessInput(ctx, input)
		if err != nil {
			return ctx, "", fmt.Errorf("guardrails error: %w", err)
		}
		input = guarded
	}

	if a.memory != nil {
		if err := a.memory.AddMessage(ctx, interfaces.Message{
			Role:    interfaces.MessageRoleUser,
			Content: input,
		}); err != nil {
			return ctx, "", fmt.Errorf("failed to add user message to memory: %w", err)
		}
	}

	return ctx, input, nil
}

// assembleTools returns the tool set for a run: the agent's own tools plus any
// MCP and lazy-MCP tools, deduplicated.
//
// initializeMCPTools already populated a.tools at construction, so re-collecting
// here can append duplicates; the merged slice always goes through
// deduplicateTools to defend against that and against MCP servers re-listing
// tools they already exposed at startup.
//
// Both run paths built this list separately and had already drifted apart --
// the streaming path never added lazy MCP tools, so an agent configured with
// them saw a different tool set depending on whether it was streamed.
func (a *Agent) assembleTools(ctx context.Context) []interfaces.Tool {
	allTools := a.tools

	if len(a.mcpServers) > 0 {
		mcpTools, err := a.collectMCPTools(ctx)
		if err != nil {
			// MCP tools are optional; log and continue.
			a.logger.Warn(ctx, fmt.Sprintf("Failed to collect MCP tools: %v", err), nil)
		} else if len(mcpTools) > 0 {
			allTools = deduplicateTools(append(allTools, mcpTools...))
		}
	}

	if len(a.lazyMCPConfigs) > 0 {
		allTools = deduplicateTools(append(allTools, a.createLazyMCPTools()...))
	}

	return allTools
}
