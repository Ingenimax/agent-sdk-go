package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"

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
// interfaces.ErrContentUnsupported, and the agent then removes the refused
// message from memory again, so later turns in the conversation do not replay
// it. That needs a memory that implements interfaces.MessageRemover, as the
// ones in pkg/memory do. Custom run functions and remote agents take text
// only, and return the same error for content with media.
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

// forgetRefusedInput takes the run's user message back out of memory when
// the run failed because the model refused one of its parts.
//
// beginRun stores the message before the provider sees it, because the
// providers build each request from memory. A provider that cannot send a part
// refuses the request with ErrContentUnsupported, but by then the message is
// stored, and every later turn in the conversation replays it: even a
// text-only follow-up then fails the same way. Removing the message leaves the
// conversation as it was before the run.
//
// Only a run whose input has media can have been refused for its own input. A
// text run refused for media in an earlier message keeps its message, since
// removing it would not unblock the conversation.
//
// A failed removal is logged rather than returned: the caller is already
// returning the refusal, which is the error that matters.
func (a *Agent) forgetRefusedInput(ctx context.Context, content *interfaces.Content, runErr error) {
	if a.memory == nil || !errors.Is(runErr, interfaces.ErrContentUnsupported) || !interfaces.HasMedia(content.Parts) {
		return
	}

	remover, ok := interfaces.AsMessageRemover(a.memory)
	if !ok {
		a.logger.Warn(ctx, "Memory cannot remove messages, so the refused input stays in the conversation and later turns will replay it", map[string]interface{}{
			"memory": fmt.Sprintf("%T", a.memory),
		})
		return
	}

	// The run may have ended with its context cancelled, by a caller that
	// stopped reading a stream, say. The cleanup still has to reach the store.
	ctx = context.WithoutCancel(ctx)

	stored := userMessage(content)
	removed, err := remover.RemoveLastMessage(ctx, func(m interfaces.Message) bool {
		return m.Role == stored.Role && m.Content == stored.Content && reflect.DeepEqual(m.Parts, stored.Parts)
	})
	switch {
	case err != nil:
		a.logger.Warn(ctx, "Failed to remove the refused input from memory", map[string]interface{}{
			"error": err.Error(),
		})
	case !removed:
		a.logger.Warn(ctx, "The refused input was not found in memory to remove", nil)
	}
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
