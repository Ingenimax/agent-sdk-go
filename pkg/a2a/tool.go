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
	client   *Client
	logger   logging.Logger
	nameOver string
}

// RemoteAgentToolOption configures a RemoteAgentTool.
type RemoteAgentToolOption func(*RemoteAgentTool)

// WithToolName overrides the auto-generated tool name.
// Use this to prevent name collisions when registering multiple remote agents.
func WithToolName(name string) RemoteAgentToolOption {
	return func(t *RemoteAgentTool) {
		t.nameOver = name
	}
}

// NewRemoteAgentTool creates a tool from an A2A client.
func NewRemoteAgentTool(client *Client, opts ...RemoteAgentToolOption) *RemoteAgentTool {
	t := &RemoteAgentTool{
		client: client,
		logger: client.logger,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

var _ interfaces.Tool = (*RemoteAgentTool)(nil)

func (t *RemoteAgentTool) Name() string {
	if t.nameOver != "" {
		return t.nameOver
	}
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
	return extractResultText(result, t.logger, ctx), nil
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

// ExtractResultText pulls text content from an A2A SendMessageResult.
// Non-text parts are converted with warnings logged via the provided logger.
func ExtractResultText(result a2a.SendMessageResult) string {
	return extractResultText(result, logging.New(), context.Background())
}

// extractResultText is the internal version that accepts a logger for non-text part warnings.
func extractResultText(result a2a.SendMessageResult, logger logging.Logger, ctx context.Context) string {
	switch r := result.(type) {
	case *a2a.Task:
		if len(r.Artifacts) > 0 {
			var parts []string
			for _, artifact := range r.Artifacts {
				for _, p := range artifact.Parts {
					if text := partToText(p, logger, ctx); text != "" {
						parts = append(parts, text)
					}
				}
			}
			return strings.Join(parts, "\n")
		}
		if r.Status.Message != nil {
			return messagePartsToText(r.Status.Message, logger, ctx)
		}
		return ""
	case *a2a.Message:
		return messagePartsToText(r, logger, ctx)
	default:
		return fmt.Sprintf("%v", result)
	}
}

// messagePartsToText extracts text from all parts of a message.
func messagePartsToText(msg *a2a.Message, logger logging.Logger, ctx context.Context) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, p := range msg.Parts {
		parts = append(parts, partToText(p, logger, ctx))
	}
	return strings.Join(parts, "\n")
}

// partToText converts any A2A Part to a text representation.
func partToText(p a2a.Part, logger logging.Logger, ctx context.Context) string {
	switch tp := p.(type) {
	case a2a.TextPart:
		return tp.Text
	case a2a.DataPart:
		logger.Warn(ctx, "A2A tool: non-text DataPart in result, converting to JSON", nil)
		data, err := json.Marshal(tp.Data)
		if err != nil {
			return fmt.Sprintf("[data: marshal error: %v]", err)
		}
		return string(data)
	case a2a.FilePart:
		logger.Warn(ctx, "A2A tool: non-text FilePart in result, using placeholder", nil)
		return formatFilePart(tp)
	default:
		logger.Warn(ctx, "A2A tool: unknown part type in result", map[string]any{
			"type": fmt.Sprintf("%T", p),
		})
		return fmt.Sprintf("%v", p)
	}
}

// formatFilePart produces a text representation of a FilePart.
func formatFilePart(fp a2a.FilePart) string {
	switch fc := fp.File.(type) {
	case a2a.FileURI:
		name := fc.Name
		if name == "" {
			name = fc.URI
		}
		return fmt.Sprintf("[file: %s]", name)
	case a2a.FileBytes:
		name := fc.Name
		if name == "" {
			name = "unnamed"
		}
		return fmt.Sprintf("[file: %s (base64: %d chars)]", name, len(fc.Bytes))
	default:
		return "[file: unknown]"
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
