// Package sdk adapts agent-sdk-go agents and tool decorators to pkg/eval.
package sdk

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	coreeval "github.com/Ingenimax/agent-sdk-go/pkg/eval"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Decorator records every invocation of the supplied tool set at one layer.
func Decorator(recorder *coreeval.Recorder, agentPath string, layer coreeval.ToolLayer) agent.ToolDecorator {
	if agentPath == "" {
		agentPath = coreeval.RootAgentPath
	}
	return func(toolSet []interfaces.Tool) []interfaces.Tool {
		if recorder == nil || len(toolSet) == 0 {
			return toolSet
		}
		wrapped := make([]interfaces.Tool, len(toolSet))
		for i, tool := range toolSet {
			wrapped[i] = &recordingTool{
				inner:     tool,
				recorder:  recorder,
				agentPath: agentPath,
				layer:     layer,
			}
		}
		return wrapped
	}
}

// Options returns agent options in the required order: execution recorder,
// policy decorators, then attempt recorder. Later decorators are outermost in
// the agent tool pipeline.
func Options(recorder *coreeval.Recorder, agentPath string, policies ...agent.ToolDecorator) []agent.Option {
	if recorder != nil {
		recorder.EnablePairedLayers()
	}
	options := []agent.Option{
		agent.WithToolDecorator(Decorator(recorder, agentPath, coreeval.ToolLayerExecution)),
	}
	for _, policy := range policies {
		if policy != nil {
			options = append(options, agent.WithToolDecorator(policy))
		}
	}
	options = append(options,
		agent.WithToolDecorator(Decorator(recorder, agentPath, coreeval.ToolLayerAttempt)),
	)
	return options
}

// NewTarget wraps an already constructed, instrumented agent for Runner.
func NewTarget(subject *agent.Agent, close func(context.Context) error) coreeval.Target {
	return coreeval.Target{
		Subject: subject,
		Close:   close,
		Capabilities: coreeval.Capabilities{
			ToolCapture: coreeval.CoverageComplete,
			Usage:       coreeval.CoverageComplete,
			Output:      coreeval.CoverageComplete,
		},
	}
}

type recordingTool struct {
	inner     interfaces.Tool
	recorder  *coreeval.Recorder
	agentPath string
	layer     coreeval.ToolLayer
}

func (t *recordingTool) Name() string { return t.inner.Name() }

func (t *recordingTool) Description() string { return t.inner.Description() }

func (t *recordingTool) Parameters() map[string]interfaces.ParameterSpec {
	return t.inner.Parameters()
}

func (t *recordingTool) Run(ctx context.Context, input string) (result string, err error) {
	return t.invoke(ctx, "Run", input, t.inner.Run)
}

func (t *recordingTool) Execute(ctx context.Context, arguments string) (result string, err error) {
	return t.invoke(ctx, "Execute", arguments, t.inner.Execute)
}

func (t *recordingTool) invoke(
	ctx context.Context,
	method string,
	arguments string,
	call func(context.Context, string) (string, error),
) (result string, err error) {
	spanContext, handle := t.recorder.Begin(ctx, coreeval.SpanStart{
		AgentPath: t.agentPath,
		Layer:     t.layer,
		Tool:      t.inner.Name(),
		Method:    method,
		Arguments: arguments,
	})
	defer func() {
		if recovered := recover(); recovered != nil {
			t.recorder.End(handle, coreeval.SpanEnd{PanicValue: recovered})
			panic(recovered)
		}
		t.recorder.End(handle, coreeval.SpanEnd{Result: result, Err: err})
	}()
	return call(spanContext, arguments)
}

// Unwrap preserves the tool pipeline's concrete-identity escape hatch.
func (t *recordingTool) Unwrap() interfaces.Tool { return t.inner }

// DisplayName forwards the optional display name.
func (t *recordingTool) DisplayName() string {
	if named, ok := t.inner.(interfaces.ToolWithDisplayName); ok {
		return named.DisplayName()
	}
	return t.inner.Name()
}

// Internal forwards the optional internal marker.
func (t *recordingTool) Internal() bool {
	if internal, ok := t.inner.(interfaces.InternalTool); ok {
		return internal.Internal()
	}
	return false
}

var (
	_ interfaces.Tool                = (*recordingTool)(nil)
	_ interfaces.ToolWithDisplayName = (*recordingTool)(nil)
	_ interfaces.InternalTool        = (*recordingTool)(nil)
	_ agent.ToolUnwrapper            = (*recordingTool)(nil)
)
