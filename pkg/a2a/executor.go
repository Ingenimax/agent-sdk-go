package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

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
	agent   AgentAdapter
	logger  logging.Logger
	cancels sync.Map // map[a2a.TaskID]context.CancelFunc
}

func newAgentExecutor(agent AgentAdapter, logger logging.Logger) *agentExecutor {
	return &agentExecutor{
		agent:  agent,
		logger: logger,
	}
}

// Execute runs the agent with the incoming A2A message and writes events to the queue.
func (e *agentExecutor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.cancels.Store(reqCtx.TaskID, cancel)
	defer e.cancels.Delete(reqCtx.TaskID)

	input := extractTextFromMessage(reqCtx.Message, e.logger, ctx)

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

	var contentLen int
	var lastErr error
	var artifactID a2a.ArtifactID
	firstChunk := true

	for agentEvent := range eventChan {
		switch agentEvent.Type {
		case interfaces.AgentEventContent:
			contentLen += len(agentEvent.Content)

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

		case interfaces.AgentEventToolCall:
			toolName := ""
			if agentEvent.ToolCall != nil {
				toolName = agentEvent.ToolCall.Name
			}
			statusMsg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{
				Text: fmt.Sprintf("Executing tool: %s", toolName),
			})
			statusEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateWorking, statusMsg)
			if err := queue.Write(ctx, statusEvent); err != nil {
				return err
			}

		case interfaces.AgentEventToolResult:
			resultPreview := ""
			if agentEvent.ToolCall != nil {
				resultPreview = agentEvent.ToolCall.Result
				if len(resultPreview) > 100 {
					resultPreview = resultPreview[:100] + "..."
				}
			}
			statusMsg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{
				Text: fmt.Sprintf("Tool completed: %s", resultPreview),
			})
			statusEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateWorking, statusMsg)
			if err := queue.Write(ctx, statusEvent); err != nil {
				return err
			}

		case interfaces.AgentEventThinking:
			statusMsg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{
				Text: agentEvent.ThinkingStep,
			})
			statusEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateWorking, statusMsg)
			if err := queue.Write(ctx, statusEvent); err != nil {
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
		"response_length": contentLen,
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

	if cancelFn, ok := e.cancels.LoadAndDelete(reqCtx.TaskID); ok {
		cancelFn.(context.CancelFunc)()
	}

	cancelEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateCanceled, nil)
	cancelEvent.Final = true
	return queue.Write(ctx, cancelEvent)
}

// extractTextFromMessage extracts the concatenated text from all parts of an A2A message.
// Non-text parts (DataPart, FilePart) are converted to text representations with log warnings.
func extractTextFromMessage(msg *a2a.Message, logger logging.Logger, ctx context.Context) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, p := range msg.Parts {
		switch tp := p.(type) {
		case a2a.TextPart:
			parts = append(parts, tp.Text)
		case a2a.DataPart:
			logger.Warn(ctx, "A2A executor: non-text DataPart in message, converting to JSON", nil)
			data, err := json.Marshal(tp.Data)
			if err != nil {
				parts = append(parts, fmt.Sprintf("[data: marshal error: %v]", err))
			} else {
				parts = append(parts, string(data))
			}
		case a2a.FilePart:
			logger.Warn(ctx, "A2A executor: non-text FilePart in message, using placeholder", nil)
			parts = append(parts, formatFilePart(tp))
		default:
			logger.Warn(ctx, "A2A executor: unknown part type in message, skipping", map[string]interface{}{
				"type": fmt.Sprintf("%T", p),
			})
		}
	}
	return strings.Join(parts, "\n")
}

