// Package testutil provides shared, scriptable test doubles for the SDK's own
// tests.
//
// Before this package there were a dozen separate hand-rolled mock LLMs across
// pkg/agent, pkg/memory, pkg/llm and pkg/microservice, each implementing the
// same six interfaces.LLM methods slightly differently and none of them safe
// for concurrent use. Every new test either grew a thirteenth or borrowed one
// from an unrelated file.
//
// All doubles here are safe to call from multiple goroutines, so a test that
// reports a race is reporting a race in the code under test.
//
// It lives under internal/ deliberately: these are conveniences for this
// module's tests, not API this module supports for downstream users.
package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// FakeLLM is a scriptable interfaces.LLM and interfaces.StreamingLLM.
//
// The zero value is usable: it answers every prompt with DefaultResponse. Queue
// specific answers with Responses, or take full control with GenerateFunc.
type FakeLLM struct {
	// ProviderName is returned by Name. Defaults to "fake".
	ProviderName string

	// ModelName is reported on LLMResponse.Model. Defaults to "fake-model".
	ModelName string

	// DefaultResponse is returned once Responses is exhausted.
	DefaultResponse string

	// Responses is consumed in order, one per Generate* call.
	Responses []string

	// Err, when set, is returned by every Generate* call.
	Err error

	// Usage, when set, is reported on every detailed response.
	Usage *interfaces.TokenUsage

	// GenerateFunc, when set, overrides all of the above.
	GenerateFunc func(ctx context.Context, prompt string, tools []interfaces.Tool) (string, error)

	// InvokeTools, when true, calls Execute on every supplied tool before
	// answering. Useful for exercising tool plumbing without a real model.
	InvokeTools bool

	// Streaming reports whether SupportsStreaming returns true. Defaults false.
	Streaming bool

	mu      sync.Mutex
	calls   int
	prompts []string
	tools   [][]string
}

// Name implements interfaces.LLM.
func (f *FakeLLM) Name() string {
	if f.ProviderName != "" {
		return f.ProviderName
	}
	return "fake"
}

// SupportsStreaming implements interfaces.LLM.
func (f *FakeLLM) SupportsStreaming() bool { return f.Streaming }

func (f *FakeLLM) model() string {
	if f.ModelName != "" {
		return f.ModelName
	}
	return "fake-model"
}

// next records the call and returns the scripted answer.
func (f *FakeLLM) next(ctx context.Context, prompt string, toolSet []interfaces.Tool) (string, error) {
	f.mu.Lock()
	f.calls++
	f.prompts = append(f.prompts, prompt)
	names := make([]string, 0, len(toolSet))
	for _, t := range toolSet {
		names = append(names, t.Name())
	}
	f.tools = append(f.tools, names)

	var scripted string
	var haveScripted bool
	if len(f.Responses) > 0 {
		scripted, haveScripted = f.Responses[0], true
		f.Responses = f.Responses[1:]
	}
	genFunc := f.GenerateFunc
	invoke := f.InvokeTools
	err := f.Err
	fallback := f.DefaultResponse
	f.mu.Unlock()

	if err != nil {
		return "", err
	}

	if invoke {
		for _, t := range toolSet {
			if _, execErr := t.Execute(ctx, "{}"); execErr != nil {
				return "", fmt.Errorf("fake llm: tool %s failed: %w", t.Name(), execErr)
			}
		}
	}

	if genFunc != nil {
		return genFunc(ctx, prompt, toolSet)
	}
	if haveScripted {
		return scripted, nil
	}
	return fallback, nil
}

// Generate implements interfaces.LLM.
func (f *FakeLLM) Generate(ctx context.Context, prompt string, _ ...interfaces.GenerateOption) (string, error) {
	return f.next(ctx, prompt, nil)
}

// GenerateWithTools implements interfaces.LLM.
func (f *FakeLLM) GenerateWithTools(ctx context.Context, prompt string, toolSet []interfaces.Tool, _ ...interfaces.GenerateOption) (string, error) {
	return f.next(ctx, prompt, toolSet)
}

// GenerateDetailed implements interfaces.LLM.
func (f *FakeLLM) GenerateDetailed(ctx context.Context, prompt string, _ ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	content, err := f.next(ctx, prompt, nil)
	if err != nil {
		return nil, err
	}
	return f.response(content), nil
}

// GenerateWithToolsDetailed implements interfaces.LLM.
func (f *FakeLLM) GenerateWithToolsDetailed(ctx context.Context, prompt string, toolSet []interfaces.Tool, _ ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	content, err := f.next(ctx, prompt, toolSet)
	if err != nil {
		return nil, err
	}
	return f.response(content), nil
}

func (f *FakeLLM) response(content string) *interfaces.LLMResponse {
	f.mu.Lock()
	usage := f.Usage
	f.mu.Unlock()

	return &interfaces.LLMResponse{
		Content:    content,
		Model:      f.model(),
		StopReason: "stop",
		Usage:      usage,
	}
}

// GenerateStream implements interfaces.StreamingLLM, emitting the scripted
// answer as one content event followed by completion.
func (f *FakeLLM) GenerateStream(ctx context.Context, prompt string, _ ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	return f.stream(ctx, prompt, nil)
}

// GenerateWithToolsStream implements interfaces.StreamingLLM.
func (f *FakeLLM) GenerateWithToolsStream(ctx context.Context, prompt string, toolSet []interfaces.Tool, _ ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	return f.stream(ctx, prompt, toolSet)
}

func (f *FakeLLM) stream(ctx context.Context, prompt string, toolSet []interfaces.Tool) (<-chan interfaces.StreamEvent, error) {
	content, err := f.next(ctx, prompt, toolSet)
	if err != nil {
		return nil, err
	}

	ch := make(chan interfaces.StreamEvent, 2)
	ch <- interfaces.StreamEvent{Type: interfaces.StreamEventContentDelta, Content: content}
	ch <- interfaces.StreamEvent{Type: interfaces.StreamEventContentComplete, Content: content}
	close(ch)
	return ch, nil
}

// Calls returns how many Generate* calls have been made.
func (f *FakeLLM) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// Prompts returns every prompt seen, in order.
func (f *FakeLLM) Prompts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

// ToolNamesAt returns the tool names supplied to the i'th call.
func (f *FakeLLM) ToolNamesAt(i int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i < 0 || i >= len(f.tools) {
		return nil
	}
	return append([]string(nil), f.tools[i]...)
}

// FakeTool is a scriptable interfaces.Tool that records its invocations.
type FakeTool struct {
	// ToolName is returned by Name. Defaults to "fake_tool".
	ToolName string

	// Desc is returned by Description.
	Desc string

	// Result is returned by Run and Execute when ExecuteFunc is nil.
	Result string

	// Err, when set, is returned by Run and Execute.
	Err error

	// Params is returned by Parameters. Defaults to an empty map.
	Params map[string]interfaces.ParameterSpec

	// ExecuteFunc, when set, overrides Result and Err.
	ExecuteFunc func(ctx context.Context, args string) (string, error)

	// Display, when non-empty, is returned by DisplayName.
	Display string

	// IsInternal is returned by Internal.
	IsInternal bool

	mu   sync.Mutex
	args []string
}

// Name implements interfaces.Tool.
func (f *FakeTool) Name() string {
	if f.ToolName != "" {
		return f.ToolName
	}
	return "fake_tool"
}

// Description implements interfaces.Tool.
func (f *FakeTool) Description() string {
	if f.Desc != "" {
		return f.Desc
	}
	return "a fake tool"
}

// Parameters implements interfaces.Tool.
func (f *FakeTool) Parameters() map[string]interfaces.ParameterSpec {
	if f.Params != nil {
		return f.Params
	}
	return map[string]interfaces.ParameterSpec{}
}

// Run implements interfaces.Tool.
func (f *FakeTool) Run(ctx context.Context, input string) (string, error) {
	return f.Execute(ctx, input)
}

// Execute implements interfaces.Tool.
func (f *FakeTool) Execute(ctx context.Context, args string) (string, error) {
	f.mu.Lock()
	f.args = append(f.args, args)
	execFunc := f.ExecuteFunc
	result := f.Result
	err := f.Err
	f.mu.Unlock()

	if execFunc != nil {
		return execFunc(ctx, args)
	}
	if err != nil {
		return "", err
	}
	return result, nil
}

// DisplayName implements interfaces.ToolWithDisplayName.
func (f *FakeTool) DisplayName() string {
	if f.Display != "" {
		return f.Display
	}
	return f.Name()
}

// Internal implements interfaces.InternalTool.
func (f *FakeTool) Internal() bool { return f.IsInternal }

// Calls returns the arguments of every invocation, in order.
func (f *FakeTool) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.args...)
}

// CallCount returns how many times the tool was invoked.
func (f *FakeTool) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.args)
}

// FakeMemory is an in-process interfaces.Memory keyed by a single scope, for
// tests that do not care about org or conversation scoping.
//
// Set Conversational to also satisfy interfaces.ConversationMemory.
type FakeMemory struct {
	// Conversational makes the fake report conversation-level capability.
	Conversational bool

	mu       sync.Mutex
	messages []interfaces.Message
}

// AddMessage implements interfaces.Memory.
func (f *FakeMemory) AddMessage(_ context.Context, message interfaces.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, message)
	return nil
}

// GetMessages implements interfaces.Memory.
func (f *FakeMemory) GetMessages(_ context.Context, _ ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]interfaces.Message(nil), f.messages...), nil
}

// Clear implements interfaces.Memory.
func (f *FakeMemory) Clear(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = nil
	return nil
}

// FakeConversationMemory is a FakeMemory that also implements
// interfaces.ConversationMemory, for testing capability discovery through
// decorators.
type FakeConversationMemory struct {
	FakeMemory

	// Conversations is returned by GetAllConversations.
	Conversations []string
}

// GetAllConversations implements interfaces.ConversationMemory.
func (f *FakeConversationMemory) GetAllConversations(context.Context) ([]string, error) {
	return append([]string(nil), f.Conversations...), nil
}

// GetConversationMessages implements interfaces.ConversationMemory.
func (f *FakeConversationMemory) GetConversationMessages(ctx context.Context, _ string) ([]interfaces.Message, error) {
	return f.GetMessages(ctx)
}

// GetMemoryStatistics implements interfaces.ConversationMemory.
func (f *FakeConversationMemory) GetMemoryStatistics(ctx context.Context) (int, int, error) {
	messages, err := f.GetMessages(ctx)
	if err != nil {
		return 0, 0, err
	}
	return len(f.Conversations), len(messages), nil
}

// FakeSpan is an inert interfaces.Span that records what it was told.
type FakeSpan struct {
	mu         sync.Mutex
	attributes map[string]interface{}
	events     []string
	errs       []error
	ended      bool
}

// End implements interfaces.Span.
func (s *FakeSpan) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
}

// AddEvent implements interfaces.Span.
func (s *FakeSpan) AddEvent(name string, _ map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, name)
}

// SetAttribute implements interfaces.Span.
func (s *FakeSpan) SetAttribute(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attributes == nil {
		s.attributes = map[string]interface{}{}
	}
	s.attributes[key] = value
}

// RecordError implements interfaces.Span.
func (s *FakeSpan) RecordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errs = append(s.errs, err)
}

// Attribute returns a recorded attribute.
func (s *FakeSpan) Attribute(key string) (interface{}, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.attributes[key]
	return v, ok
}

// Errors returns every error recorded on the span.
func (s *FakeSpan) Errors() []error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]error(nil), s.errs...)
}

// Ended reports whether End was called.
func (s *FakeSpan) Ended() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ended
}

// FakeTracer is an inert interfaces.Tracer that hands out FakeSpans.
type FakeTracer struct {
	mu    sync.Mutex
	spans []*FakeSpan
	names []string
}

// StartSpan implements interfaces.Tracer.
func (t *FakeTracer) StartSpan(ctx context.Context, name string) (context.Context, interfaces.Span) {
	return ctx, t.record(name)
}

// StartTraceSession implements interfaces.Tracer.
func (t *FakeTracer) StartTraceSession(ctx context.Context, name string) (context.Context, interfaces.Span) {
	return ctx, t.record(name)
}

func (t *FakeTracer) record(name string) *FakeSpan {
	span := &FakeSpan{}
	t.mu.Lock()
	t.spans = append(t.spans, span)
	t.names = append(t.names, name)
	t.mu.Unlock()
	return span
}

// SpanNames returns the names of every span started, in order.
func (t *FakeTracer) SpanNames() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.names...)
}

// Spans returns every span handed out, in order.
func (t *FakeTracer) Spans() []*FakeSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*FakeSpan(nil), t.spans...)
}

// Compile-time conformance assertions. Nothing else in this repo catches
// interface drift, so a signature change upstream shows up here first.
var (
	_ interfaces.LLM                 = (*FakeLLM)(nil)
	_ interfaces.StreamingLLM        = (*FakeLLM)(nil)
	_ interfaces.Tool                = (*FakeTool)(nil)
	_ interfaces.ToolWithDisplayName = (*FakeTool)(nil)
	_ interfaces.InternalTool        = (*FakeTool)(nil)
	_ interfaces.Memory              = (*FakeMemory)(nil)
	_ interfaces.Memory              = (*FakeConversationMemory)(nil)
	_ interfaces.ConversationMemory  = (*FakeConversationMemory)(nil)
	_ interfaces.Span                = (*FakeSpan)(nil)
	_ interfaces.Tracer              = (*FakeTracer)(nil)
)

// NewFakeLLM returns a FakeLLM that answers every prompt with "ok". It exists
// so migrated call sites read as intent rather than struct literals.
func NewFakeLLM() *FakeLLM {
	return &FakeLLM{DefaultResponse: "ok"}
}
