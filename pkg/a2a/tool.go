package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// RemoteAgentTool wraps an A2A client as an interfaces.Tool so that a remote
// A2A agent can be used as a tool by agent-sdk-go agents.
type RemoteAgentTool struct {
	client *Client
	logger logging.Logger
}

// NewRemoteAgentTool creates a tool from an A2A client.
func NewRemoteAgentTool(client *Client) *RemoteAgentTool {
	return &RemoteAgentTool{
		client: client,
		logger: client.logger,
	}
}

var _ interfaces.Tool = (*RemoteAgentTool)(nil)

func (t *RemoteAgentTool) Name() string {
	card := t.client.Card()
	return sanitizeToolName(card.Name)
}

func (t *RemoteAgentTool) Description() string {
	return t.client.Card().Description
}

func (t *RemoteAgentTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: fmt.Sprintf("The message to send to the remote %s A2A agent", t.client.Card().Name),
			Required:    true,
		},
	}
}

func (t *RemoteAgentTool) Run(ctx context.Context, input string) (string, error) {
	result, err := t.client.SendMessage(ctx, input)
	if err != nil {
		return "", fmt.Errorf("a2a tool %s: %w", t.Name(), err)
	}
	return extractResultText(result), nil
}

func (t *RemoteAgentTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("a2a tool: failed to parse arguments: %w", err)
	}
	if params.Query == "" {
		return "", fmt.Errorf("a2a tool: query parameter is required")
	}
	return t.Run(ctx, params.Query)
}

// extractResultText pulls text content from an A2A SendMessageResult.
func extractResultText(result a2a.SendMessageResult) string {
	switch r := result.(type) {
	case *a2a.Task:
		if len(r.Artifacts) > 0 {
			var parts []string
			for _, artifact := range r.Artifacts {
				for _, p := range artifact.Parts {
					if tp, ok := p.(a2a.TextPart); ok {
						parts = append(parts, tp.Text)
					}
				}
			}
			return strings.Join(parts, "\n")
		}
		if r.Status.Message != nil {
			return extractTextFromMessage(r.Status.Message)
		}
		return ""
	case *a2a.Message:
		return extractTextFromMessage(r)
	default:
		return fmt.Sprintf("%v", result)
	}
}

func sanitizeToolName(name string) string {
	result := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return '_'
	}, name)
	return strings.ToLower(result)
}
