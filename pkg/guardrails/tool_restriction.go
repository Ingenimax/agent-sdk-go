package guardrails

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ToolRestriction implements a guardrail that restricts which tools can be used
type ToolRestriction struct {
	allowedTools []string
	action       Action
	regex        *regexp.Regexp
}

// NewToolRestriction creates a new tool restriction guardrail
func NewToolRestriction(allowedTools []string, action Action) *ToolRestriction {
	// Create a regex to match tool invocations
	// This is a simplified example - in a real implementation, you would need
	// to parse the request more carefully to identify tool invocations
	pattern := `(?i)use\s+tool\s+([a-z0-9_]+)`
	regex := regexp.MustCompile(pattern)

	return &ToolRestriction{
		allowedTools: allowedTools,
		action:       action,
		regex:        regex,
	}
}

// Type returns the type of guardrail
func (t *ToolRestriction) Type() GuardrailType {
	return ToolRestrictionGuardrail
}

// CheckRequest checks if a request violates the guardrail
func (t *ToolRestriction) CheckRequest(ctx context.Context, request string) (bool, string, error) {
	matches := t.regex.FindAllStringSubmatch(request, -1)
	if len(matches) == 0 {
		return false, request, nil
	}

	triggered := false
	modified := request

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		toolName := strings.ToLower(match[1])
		allowed := false
		for _, allowedTool := range t.allowedTools {
			if strings.ToLower(allowedTool) == toolName {
				allowed = true
				break
			}
		}

		if !allowed {
			triggered = true
			modified = strings.ReplaceAll(
				modified,
				match[0],
				fmt.Sprintf("use tool [RESTRICTED TOOL: %s is not allowed]", toolName),
			)
		}
	}

	return triggered, modified, nil
}

// CheckResponse checks if a response violates the guardrail
func (t *ToolRestriction) CheckResponse(ctx context.Context, response string) (bool, string, error) {
	// Tool restrictions typically apply to requests, not responses
	return false, response, nil
}

// Action returns the action to take when the guardrail is triggered
func (t *ToolRestriction) Action() Action {
	return t.action
}
