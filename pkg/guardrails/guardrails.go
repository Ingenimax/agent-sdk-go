package guardrails

import (
	"context"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// GuardrailType represents the type of guardrail
type GuardrailType string

const (
	// ContentFilterGuardrail filters content for inappropriate material
	ContentFilterGuardrail GuardrailType = "content_filter"

	// TokenLimitGuardrail limits the number of tokens in a request or response
	TokenLimitGuardrail GuardrailType = "token_limit"

	// PiiFilterGuardrail filters personally identifiable information
	PiiFilterGuardrail GuardrailType = "pii_filter"

	// ToolRestrictionGuardrail restricts which tools can be used
	ToolRestrictionGuardrail GuardrailType = "tool_restriction"

	// RateLimitGuardrail limits the rate of requests
	RateLimitGuardrail GuardrailType = "rate_limit"
)

// Action represents the action to take when a guardrail is triggered
type Action string

const (
	// BlockAction blocks the request or response
	BlockAction Action = "block"

	// RedactAction redacts the sensitive content
	RedactAction Action = "redact"

	// WarnAction allows the content but logs a warning
	WarnAction Action = "warn"
)

// Guardrail represents a guardrail that can be applied to requests and responses
type Guardrail interface {
	// Type returns the type of guardrail
	Type() GuardrailType

	// CheckRequest checks if a request violates the guardrail
	CheckRequest(ctx context.Context, request string) (bool, string, error)

	// CheckResponse checks if a response violates the guardrail
	CheckResponse(ctx context.Context, response string) (bool, string, error)

	// Action returns the action to take when the guardrail is triggered
	Action() Action
}

// Pipeline represents a pipeline of guardrails
type Pipeline struct {
	guardrails []Guardrail
	logger     logging.Logger
}

// NewPipeline creates a new guardrails pipeline
func NewPipeline(guardrails []Guardrail, logger logging.Logger) *Pipeline {
	return &Pipeline{
		guardrails: guardrails,
		logger:     logger,
	}
}

// ProcessRequest processes a request through the guardrails pipeline
func (p *Pipeline) ProcessRequest(ctx context.Context, request string) (string, error) {
	processedRequest := request

	for _, guardrail := range p.guardrails {
		triggered, modified, err := guardrail.CheckRequest(ctx, processedRequest)
		if err != nil {
			p.logger.Error(ctx, "Guardrail check failed", map[string]interface{}{
				"guardrail_type": guardrail.Type(),
				"error":          err.Error(),
			})
			return "", fmt.Errorf("guardrail check failed: %w", err)
		}

		if triggered {
			p.logger.Info(ctx, "Guardrail triggered", map[string]interface{}{
				"guardrail_type": guardrail.Type(),
				"action":         guardrail.Action(),
			})

			switch guardrail.Action() {
			case BlockAction:
				return "", fmt.Errorf("request blocked by %s guardrail", guardrail.Type())
			case RedactAction:
				processedRequest = modified
			case WarnAction:
				// Continue with original request but log warning
				p.logger.Warn(ctx, "Guardrail warning", map[string]interface{}{
					"guardrail_type": guardrail.Type(),
					"original":       processedRequest,
					"modified":       modified,
				})
			}
		}
	}

	return processedRequest, nil
}

// ProcessResponse processes a response through the guardrails pipeline
func (p *Pipeline) ProcessResponse(ctx context.Context, response string) (string, error) {
	processedResponse := response

	for _, guardrail := range p.guardrails {
		triggered, modified, err := guardrail.CheckResponse(ctx, processedResponse)
		if err != nil {
			p.logger.Error(ctx, "Guardrail check failed", map[string]interface{}{
				"guardrail_type": guardrail.Type(),
				"error":          err.Error(),
			})
			return "", fmt.Errorf("guardrail check failed: %w", err)
		}

		if triggered {
			p.logger.Info(ctx, "Guardrail triggered", map[string]interface{}{
				"guardrail_type": guardrail.Type(),
				"action":         guardrail.Action(),
			})

			switch guardrail.Action() {
			case BlockAction:
				return "", fmt.Errorf("response blocked by %s guardrail", guardrail.Type())
			case RedactAction:
				processedResponse = modified
			case WarnAction:
				// Continue with original response but log warning
				p.logger.Warn(ctx, "Guardrail warning", map[string]interface{}{
					"guardrail_type": guardrail.Type(),
					"original":       processedResponse,
					"modified":       modified,
				})
			}
		}
	}

	return processedResponse, nil
}

// ProcessInput implements interfaces.Guardrails, so a Pipeline can be handed
// straight to agent.WithGuardrails.
//
// Without these two methods the whole package was unreachable from an agent:
// WithGuardrails takes an interfaces.Guardrails (ProcessInput / ProcessOutput)
// while Pipeline exposed only ProcessRequest / ProcessResponse, and no type in
// the module implemented the interface. Every guardrail here could be built and
// none could be attached to anything.
func (p *Pipeline) ProcessInput(ctx context.Context, input string) (string, error) {
	return p.ProcessRequest(ctx, input)
}

// ProcessOutput implements interfaces.Guardrails. See ProcessInput.
func (p *Pipeline) ProcessOutput(ctx context.Context, output string) (string, error) {
	return p.ProcessResponse(ctx, output)
}

// AddGuardrail adds a guardrail to the pipeline
func (p *Pipeline) AddGuardrail(guardrail Guardrail) {
	p.guardrails = append(p.guardrails, guardrail)
}
