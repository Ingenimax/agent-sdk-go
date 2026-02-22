package a2a

import (
	"context"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// AgentAdapter is the interface that the A2A executor needs from an agent.
// It mirrors the subset of agent.Agent methods required for A2A.
type AgentAdapter interface {
	Run(ctx context.Context, input string) (string, error)
	RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error)
	GetName() string
	GetDescription() string
}

// agentExecutor implements a2asrv.AgentExecutor by delegating to an AgentAdapter.
type agentExecutor struct {
	agent  AgentAdapter
	logger logging.Logger
}

func newAgentExecutor(agent AgentAdapter, logger logging.Logger) *agentExecutor {
	return &agentExecutor{
		agent:  agent,
		logger: logger,
	}
}

// Execute runs the agent with the incoming A2A message and writes events to the queue.
func (e *agentExecutor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	input := extractTextFromMessage(reqCtx.Message)

	e.logger.Debug(ctx, "A2A executor: starting agent execution", map[string]interface{}{
		"agent":      e.agent.GetName(),
		"task_id":    string(reqCtx.TaskID),
		"context_id": reqCtx.ContextID,
		"input":      input,
	})

	// Signal working state
	workingEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateWorking, nil)
	if err := queue.Write(ctx, workingEvent); err != nil {
		return err
	}

	// Stream the agent response
	eventChan, err := e.agent.RunStream(ctx, input)
	if err != nil {
		e.logger.Error(ctx, "A2A executor: agent stream failed", map[string]interface{}{
			"agent": e.agent.GetName(),
			"error": err.Error(),
		})
		failMsg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: err.Error()})
		failEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateFailed, failMsg)
		failEvent.Final = true
		return queue.Write(ctx, failEvent)
	}

	var contentBuilder strings.Builder
	var lastErr error
	var artifactID a2a.ArtifactID
	firstChunk := true

	for agentEvent := range eventChan {
		switch agentEvent.Type {
		case interfaces.AgentEventContent:
			contentBuilder.WriteString(agentEvent.Content)

			var artifact *a2a.TaskArtifactUpdateEvent
			if firstChunk {
				artifact = a2a.NewArtifactEvent(reqCtx, a2a.TextPart{Text: agentEvent.Content})
				artifactID = artifact.Artifact.ID
				firstChunk = false
			} else {
				artifact = a2a.NewArtifactUpdateEvent(reqCtx, artifactID, a2a.TextPart{Text: agentEvent.Content})
				artifact.Append = true
			}

			if err := queue.Write(ctx, artifact); err != nil {
				return err
			}

		case interfaces.AgentEventError:
			lastErr = agentEvent.Error

		case interfaces.AgentEventComplete:
			// handled after loop
		}
	}

	// Determine final state
	if lastErr != nil {
		e.logger.Error(ctx, "A2A executor: agent completed with error", map[string]interface{}{
			"agent": e.agent.GetName(),
			"error": lastErr.Error(),
		})
		failMsg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: lastErr.Error()})
		failEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateFailed, failMsg)
		failEvent.Final = true
		return queue.Write(ctx, failEvent)
	}

	// Mark the last chunk if we had content
	if artifactID != "" {
		lastChunk := a2a.NewArtifactUpdateEvent(reqCtx, artifactID, a2a.TextPart{Text: ""})
		lastChunk.LastChunk = true
		lastChunk.Append = true
		if err := queue.Write(ctx, lastChunk); err != nil {
			return err
		}
	}

	e.logger.Debug(ctx, "A2A executor: agent execution completed", map[string]interface{}{
		"agent":           e.agent.GetName(),
		"response_length": contentBuilder.Len(),
	})

	completeEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateCompleted, nil)
	completeEvent.Final = true
	return queue.Write(ctx, completeEvent)
}

// Cancel handles cancellation of an in-progress task.
func (e *agentExecutor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	e.logger.Info(ctx, "A2A executor: task cancellation requested", map[string]interface{}{
		"task_id": string(reqCtx.TaskID),
	})
	cancelEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateCanceled, nil)
	cancelEvent.Final = true
	return queue.Write(ctx, cancelEvent)
}

// extractTextFromMessage extracts the concatenated text from all TextParts of an A2A message.
func extractTextFromMessage(msg *a2a.Message) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, p := range msg.Parts {
		if tp, ok := p.(a2a.TextPart); ok {
			parts = append(parts, tp.Text)
		}
	}
	return strings.Join(parts, "\n")
}
