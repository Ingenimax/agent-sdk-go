package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// RunWithContent runs the agent with a multipart input, such as text and an
// image, the way Google ADK's runner.Run takes a genai.Content:
//
//	answer, err := a.RunWithContent(ctx, interfaces.NewContentFromParts([]*interfaces.Part{
//		interfaces.NewPartFromBytes(png, "image/png"),
//		interfaces.NewPartFromText("What is in this picture?"),
//	}, interfaces.MessageRoleUser))
//
// The role must be user or empty. Run(ctx, input) is RunWithContent with one
// text part.
//
// With memory set, the agent stores the content with its parts, so later
// turns in the conversation still show the model its media. A provider that
// cannot send a part returns an error that wraps
// interfaces.ErrContentUnsupported. Custom run functions and remote agents
// take text only, and return the same error for content with media.
func (a *Agent) RunWithContent(ctx context.Context, content *interfaces.Content) (string, error) {
	response, err := a.runInternal(ctx, content, false)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// RunWithContentDetailed is RunWithContent with token usage and execution
// details.
func (a *Agent) RunWithContentDetailed(ctx context.Context, content *interfaces.Content) (*interfaces.AgentResponse, error) {
	return a.runInternal(ctx, content, true)
}

// textContent returns the content for a plain text run input.
func textContent(input string) *interfaces.Content {
	return interfaces.NewContentFromText(input, interfaces.MessageRoleUser)
}

// checkRunInput reports whether content is a valid run input: from the user,
// or with an empty role, and with valid parts.
func checkRunInput(content *interfaces.Content) error {
	if content == nil {
		return errors.New("run input content is nil")
	}
	if content.Role != "" && content.Role != interfaces.MessageRoleUser {
		return fmt.Errorf("run input must be from the user, got role %q", content.Role)
	}
	if err := interfaces.ValidateParts(content.Parts); err != nil {
		return fmt.Errorf("invalid run input: %w", err)
	}
	return nil
}

// textOnly returns the text of the content for a path that takes text only,
// or an error when the content has media that path would drop.
func textOnly(content *interfaces.Content, path string) (string, error) {
	if err := checkRunInput(content); err != nil {
		return "", err
	}
	if interfaces.HasMedia(content.Parts) {
		return "", fmt.Errorf("%s takes text only: %w", path, interfaces.ErrContentUnsupported)
	}
	return content.Text(), nil
}

// guardContent runs the input guardrails on each text part and returns a
// copy of the content with the guarded text. Media parts are kept as given.
func (a *Agent) guardContent(ctx context.Context, content *interfaces.Content) (*interfaces.Content, error) {
	guarded := &interfaces.Content{Role: interfaces.MessageRoleUser, Parts: make([]*interfaces.Part, 0, len(content.Parts))}
	for _, p := range content.Parts {
		if !p.IsText() || a.guardrails == nil {
			guarded.Parts = append(guarded.Parts, p)
			continue
		}
		text, err := a.guardrails.ProcessInput(ctx, p.Text)
		if err != nil {
			return nil, fmt.Errorf("guardrails error: %w", err)
		}
		guarded.Parts = append(guarded.Parts, interfaces.NewPartFromText(text))
	}
	return guarded, nil
}

// userMessage returns the memory message for a run input. Content is the
// text; Parts is set only when the input has media, so a text-only run stores
// the same message it always has.
func userMessage(content *interfaces.Content) interfaces.Message {
	msg := interfaces.Message{Role: interfaces.MessageRoleUser, Content: content.Text()}
	if interfaces.HasMedia(content.Parts) {
		msg.Parts = content.Parts
	}
	return msg
}

// partOptions returns the generate option that sends the input as parts when
// it has media. Providers read it only when no memory is set; with memory the
// parts travel on the stored user message instead.
func partOptions(content *interfaces.Content) []interfaces.GenerateOption {
	if !interfaces.HasMedia(content.Parts) {
		return nil
	}
	return []interfaces.GenerateOption{interfaces.WithParts(content.Parts...)}
}

// mediaParts returns the parts of the content that are not text.
func mediaParts(content *interfaces.Content) []*interfaces.Part {
	var media []*interfaces.Part
	for _, p := range content.Parts {
		if !p.IsText() {
			media = append(media, p)
		}
	}
	return media
}
