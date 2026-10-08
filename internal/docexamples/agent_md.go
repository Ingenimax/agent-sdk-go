// Package docexamples compiles the code examples that appear in docs/.
//
// It exists because docs/agent.md once documented agent.NewToolExecutor,
// agent.WithToolExecutor, agent.NewMessageProcessor and
// agent.WithMessageProcessor, none of which ever existed (#333, #334). Prose
// examples drift silently; a package that `go build ./...` compiles does not.
//
// Nothing imports this package. When an example here stops compiling, update
// both the example and the corresponding snippet in docs/.
package docexamples

import (
	"context"
	"log"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/hooks"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

func isOutOfHours() bool { return false }

func hooksExample(openaiClient interfaces.LLM, searchTool, calculatorTool interfaces.Tool) {
	reg := hooks.NewRegistry(
		hooks.Logging(log.Printf),
		hooks.AllowList("websearch", "calculator"),
		hooks.Redact("[REDACTED]", hooks.CommonSecretPatterns()...),
		hooks.TruncateResults(8000),
	)

	reg.Register(hooks.Plugin{
		Name: "custom-tool-shortcut",
		BeforeTool: func(ctx context.Context, call hooks.ToolCall) (hooks.Outcome, error) {
			if call.Tool == "send_email" && isOutOfHours() {
				return hooks.Outcome{
					Decision: hooks.Deny,
					Message:  "Email is disabled outside business hours. Summarise instead.",
				}, nil
			}
			return hooks.Outcome{}, nil
		},
	})

	_, _ = agent.NewAgent(
		agent.WithLLM(openaiClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(searchTool, calculatorTool),
		agent.WithToolDecorator(reg.Decorate("my-agent")),
	)
}

type myWrapper struct{ inner interfaces.Tool }

func (w myWrapper) Name() string        { return w.inner.Name() }
func (w myWrapper) Description() string { return w.inner.Description() }
func (w myWrapper) Parameters() map[string]interfaces.ParameterSpec {
	return w.inner.Parameters()
}
func (w myWrapper) Run(ctx context.Context, input string) (string, error) {
	return w.inner.Run(ctx, input)
}
func (w myWrapper) Execute(ctx context.Context, args string) (string, error) {
	return w.inner.Execute(ctx, args)
}
func (w myWrapper) Unwrap() interfaces.Tool { return w.inner }

func decoratorExample() agent.Option {
	return agent.WithToolDecorator(func(toolSet []interfaces.Tool) []interfaces.Tool {
		wrapped := make([]interfaces.Tool, len(toolSet))
		for i, t := range toolSet {
			wrapped[i] = myWrapper{inner: t}
		}
		return wrapped
	})
}

func unwrapExample(t interfaces.Tool) interfaces.Tool { return agent.UnwrapTool(t) }

type taggingMemory struct{ inner interfaces.Memory }

func (m taggingMemory) AddMessage(ctx context.Context, msg interfaces.Message) error {
	if msg.Role == interfaces.MessageRoleUser {
		if msg.Metadata == nil {
			msg.Metadata = map[string]interface{}{}
		}
		msg.Metadata["processed_at"] = time.Now()
	}
	return m.inner.AddMessage(ctx, msg)
}

func (m taggingMemory) GetMessages(ctx context.Context, opts ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return m.inner.GetMessages(ctx, opts...)
}

func (m taggingMemory) Clear(ctx context.Context) error { return m.inner.Clear(ctx) }

func memoryExample(openaiClient interfaces.LLM) {
	_, _ = agent.NewAgent(
		agent.WithLLM(openaiClient),
		agent.WithMemory(taggingMemory{inner: memory.NewConversationBuffer()}),
	)
}

var _ = []any{hooksExample, decoratorExample, unwrapExample, memoryExample, agent.ForwardOptionalToolInterfaces}
