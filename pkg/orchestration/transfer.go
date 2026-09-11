package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// TransferTool lets a model hand a conversation to another agent by calling a
// tool, rather than by emitting a marker in its prose.
//
// The existing handoff mechanism parses free-form output:
//
//	regexp.MustCompile(`\[HANDOFF:([a-zA-Z0-9_-]+):([^\]]+)\]`)
//
// which requires the prompt to teach a bespoke syntax, breaks when the model
// paraphrases or wraps the marker in a code fence, and cannot constrain the
// target -- a hallucinated agent name is only discovered after the fact. A tool
// call is structured, and the agent name can be constrained to an enum so the
// model cannot invent one.
//
// Transfer is a request, not an execution. The tool records the decision and
// returns; the caller runs the target agent. That keeps the depth guard, the
// cycle check and usage accounting under the caller's control rather than
// buried in a tool.
type TransferTool struct {
	registry     *AgentRegistry
	descriptions map[string]string

	mu        sync.Mutex
	requested *TransferRequest
}

// TransferRequest is a model's decision to hand off.
type TransferRequest struct {
	// TargetAgentID is the agent to transfer to.
	TargetAgentID string

	// Reason is why, in the model's words. Worth keeping: it is the only
	// explanation a human debugging a routing decision will have.
	Reason string

	// Query is what to ask the target agent.
	Query string
}

// NewTransferTool creates a transfer tool over a registry.
//
// descriptions maps agent ID to a one-line summary of what that agent is for.
// They are what the model routes on, so an agent with no description is
// effectively unreachable.
func NewTransferTool(registry *AgentRegistry, descriptions map[string]string) *TransferTool {
	return &TransferTool{registry: registry, descriptions: descriptions}
}

// Name implements interfaces.Tool.
func (t *TransferTool) Name() string { return "transfer_to_agent" }

// Description implements interfaces.Tool.
func (t *TransferTool) Description() string {
	var b strings.Builder
	b.WriteString("Transfer the conversation to a more suitable agent. ")
	b.WriteString("Use this when another agent is better equipped to answer.\n\nAvailable agents:\n")

	for _, id := range t.agentIDs() {
		if desc := t.descriptions[id]; desc != "" {
			fmt.Fprintf(&b, "- %s: %s\n", id, desc)
		} else {
			fmt.Fprintf(&b, "- %s\n", id)
		}
	}
	return b.String()
}

// Parameters implements interfaces.Tool.
func (t *TransferTool) Parameters() map[string]interfaces.ParameterSpec {
	ids := t.agentIDs()
	enum := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		enum = append(enum, id)
	}

	spec := interfaces.ParameterSpec{
		Type:        "string",
		Description: "The agent to transfer to",
		Required:    true,
	}
	// Constraining the target is the point. A free-form string invites a
	// hallucinated agent name, which costs a whole turn to discover.
	if len(enum) > 0 {
		spec.Enum = enum
	}

	return map[string]interfaces.ParameterSpec{
		"agent": spec,
		"reason": {
			Type:        "string",
			Description: "Why this agent is better suited",
			Required:    false,
		},
		"query": {
			Type:        "string",
			Description: "What to ask the target agent. Defaults to the current request.",
			Required:    false,
		},
	}
}

// Run implements interfaces.Tool.
func (t *TransferTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute implements interfaces.Tool.
func (t *TransferTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		Agent  string `json:"agent"`
		Reason string `json:"reason"`
		Query  string `json:"query"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parsing transfer arguments: %w", err)
	}

	if params.Agent == "" {
		return "A target agent is required. " + t.Description(), nil
	}

	if _, ok := t.registry.Get(params.Agent); !ok {
		// Reported to the model rather than failing: listing the real options
		// makes the next turn recoverable.
		return fmt.Sprintf("There is no agent named %q. %s", params.Agent, t.Description()), nil
	}

	t.mu.Lock()
	t.requested = &TransferRequest{
		TargetAgentID: params.Agent,
		Reason:        params.Reason,
		Query:         params.Query,
	}
	t.mu.Unlock()

	return fmt.Sprintf("Transferring to %q.", params.Agent), nil
}

// Requested returns the transfer the model asked for, if any, and clears it.
//
// Clearing on read means a handle reused across turns cannot replay a stale
// decision from a previous turn.
func (t *TransferTool) Requested() (*TransferRequest, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	req := t.requested
	t.requested = nil
	return req, req != nil
}

func (t *TransferTool) agentIDs() []string {
	agents := t.registry.List()
	ids := make([]string, 0, len(agents))
	for id := range agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

var _ interfaces.Tool = (*TransferTool)(nil)

// RunWithTransfers runs an agent and follows any transfers it requests.
//
// maxTransfers bounds the chain. Without a bound two agents can transfer to each
// other indefinitely, each call costing a request -- the routing equivalent of
// an infinite loop, and expensive.
//
// Returns the final answer and the chain of agent IDs that produced it, so a
// caller can log or display how a request was routed.
func RunWithTransfers(
	ctx context.Context,
	registry *AgentRegistry,
	tool *TransferTool,
	startAgentID string,
	input string,
	maxTransfers int,
) (result string, chain []string, err error) {
	if maxTransfers < 0 {
		maxTransfers = 0
	}

	currentID := startAgentID
	currentInput := input
	visited := map[string]int{}

	for transfers := 0; ; transfers++ {
		current, ok := registry.Get(currentID)
		if !ok {
			return "", chain, fmt.Errorf("agent %q is not registered", currentID)
		}
		chain = append(chain, currentID)

		// A repeat visit is not automatically wrong -- an agent may legitimately
		// be returned to -- but an unbounded cycle is, so count it.
		visited[currentID]++
		if visited[currentID] > 2 {
			return "", chain, fmt.Errorf("transfer loop: agent %q was reached %d times",
				currentID, visited[currentID])
		}

		answer, runErr := current.Run(ctx, currentInput)
		if runErr != nil {
			return "", chain, runErr
		}

		req, requested := tool.Requested()
		if !requested {
			return answer, chain, nil
		}

		if transfers >= maxTransfers {
			// Return what the last agent said rather than an error: the user
			// asked a question, and a partial answer beats a failure.
			return answer, chain, fmt.Errorf(
				"transfer limit of %d reached; returning the last agent's answer", maxTransfers)
		}

		currentID = req.TargetAgentID
		if req.Query != "" {
			currentInput = req.Query
		}
	}
}
