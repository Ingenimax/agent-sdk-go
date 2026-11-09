package tracing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// fullContext returns a context with org, conversation and agent name set.
func fullContext() context.Context {
	ctx := multitenancy.WithOrgID(context.Background(), "org-1")
	ctx = memory.WithConversationID(ctx, "conv-1")
	return WithAgentName(ctx, "agent-1")
}

func TestOTELLangfuseTracer_StartSpan(t *testing.T) {
	tracer, exporter := newTestLangfuseTracer(t, false)

	_, span := tracer.StartSpan(WithTraceName(fullContext(), "my-trace"), "op")
	span.AddEvent("evt", map[string]any{"n": 1})
	span.SetAttribute("k", "v")
	span.End()

	got := spanByName(t, exporter, "op")
	assertAttrs(t, attrMap(got.Attributes), map[string]string{
		"langfuse.trace.name":                      "my-trace",
		"langfuse.environment":                     "test-env",
		"langfuse.user.id":                         "org-1",
		"langfuse.session.id":                      "conv-1",
		"langfuse.observation.metadata.agent_name": "agent-1",
		"langfuse.trace.metadata.agent_name":       "agent-1",
		"service.name":                             "agent-1",
		"component.name":                           "agent-1",
		"k":                                        "v",
	})
	if len(got.Events) != 1 || got.Events[0].Name != "evt" {
		t.Errorf("events = %+v", got.Events)
	}
}

func TestOTELLangfuseTracer_StartSpanMinimalContext(t *testing.T) {
	tracer, exporter := newTestLangfuseTracer(t, false)
	_, span := tracer.StartSpan(context.Background(), "op")
	span.End()

	attrs := attrMap(spanByName(t, exporter, "op").Attributes)
	if attrs["langfuse.trace.name"] != "op" {
		t.Errorf("trace name = %q, want span name", attrs["langfuse.trace.name"])
	}
	for _, k := range []string{"langfuse.user.id", "langfuse.session.id", "service.name"} {
		if _, ok := attrs[k]; ok {
			t.Errorf("unexpected attribute %q", k)
		}
	}
}

func TestOTELLangfuseTracer_Disabled(t *testing.T) {
	tracer := &OTELLangfuseTracer{}
	ctx := context.Background()

	if gotCtx, span := tracer.StartSpan(ctx, "op"); gotCtx != ctx || span == nil {
		t.Error("disabled StartSpan should return the input context and a span")
	}
	if gotCtx, span := tracer.StartTraceSession(ctx, "s"); gotCtx != ctx || span == nil {
		t.Error("disabled StartTraceSession should return the input context and a span")
	}
	now := time.Now()
	if id, err := tracer.TraceGeneration(ctx, "m", "p", "r", now, now, nil); id != "" || err != nil {
		t.Errorf("TraceGeneration = %q, %v", id, err)
	}
	if id, err := tracer.TraceSpan(ctx, "s", now, now, nil, ""); id != "" || err != nil {
		t.Errorf("TraceSpan = %q, %v", id, err)
	}
	if id, err := tracer.TraceEvent(ctx, "e", nil, nil, "info", nil, ""); id != "" || err != nil {
		t.Errorf("TraceEvent = %q, %v", id, err)
	}
	if err := tracer.Flush(); err != nil {
		t.Errorf("Flush = %v", err)
	}
	if err := tracer.Shutdown(); err != nil {
		t.Errorf("Shutdown = %v", err)
	}
}

func TestTraceGeneration_NoToolCalls(t *testing.T) {
	tracer, exporter := newTestLangfuseTracer(t, false)
	start := time.Unix(1000, 0)
	end := start.Add(2 * time.Second)

	id, err := tracer.TraceGeneration(fullContext(), "gpt-x", "12345678", "abcd", start, end, map[string]any{"extra": 7})
	if err != nil {
		t.Fatalf("TraceGeneration: %v", err)
	}

	got := spanByName(t, exporter, "agent-1")
	if id != got.SpanContext.SpanID().String() {
		t.Errorf("returned id %q does not match span %q", id, got.SpanContext.SpanID())
	}
	if !got.StartTime.Equal(start) || !got.EndTime.Equal(end) {
		t.Errorf("span times = %v..%v, want %v..%v", got.StartTime, got.EndTime, start, end)
	}
	assertAttrs(t, attrMap(got.Attributes), map[string]string{
		"gen_ai.request.model":                "gpt-x",
		"langfuse.trace.input":                "12345678",
		"langfuse.trace.output":               "abcd",
		"langfuse.observation.type":           "generation",
		"gen_ai.usage.prompt_tokens":          "2",
		"gen_ai.usage.completion_tokens":      "1",
		"gen_ai.usage.total_tokens":           "3",
		"langfuse.user.id":                    "org-1",
		"langfuse.session.id":                 "conv-1",
		"gen_ai.prompt.0.content":             "12345678",
		"gen_ai.completion.0.content":         "abcd",
		"langfuse.observation.metadata.extra": "7",
	})
	if len(exporter.GetSpans()) != 1 {
		t.Errorf("exported %d spans, want 1", len(exporter.GetSpans()))
	}
}

func TestTraceGeneration_WithToolCalls(t *testing.T) {
	start := time.Unix(2000, 0)
	toolCalls := []ToolCall{
		{Name: "timed", ID: "id-1", Arguments: "{}", Result: "ok", StartTime: start, Duration: 3 * time.Second},
		{Name: "stamped", Arguments: "a", Result: "b", Timestamp: start.Format(time.RFC3339)},
		{Name: "failed", Error: "kaboom", Timestamp: "not-a-time"},
		{Name: "bare"},
	}

	tests := []struct {
		name       string
		response   string
		wantOutput func(t *testing.T, out string)
	}{
		{
			name:     "json response gets tool_calls field",
			response: `{"answer":"42"}`,
			wantOutput: func(t *testing.T, out string) {
				var obj map[string]any
				if err := json.Unmarshal([]byte(out), &obj); err != nil {
					t.Fatalf("output is not JSON: %v", err)
				}
				if obj["answer"] != "42" {
					t.Errorf("answer = %v", obj["answer"])
				}
				if calls, ok := obj["tool_calls"].([]any); !ok || len(calls) != len(toolCalls) {
					t.Errorf("tool_calls = %v", obj["tool_calls"])
				}
			},
		},
		{
			name:     "text response gets a tool calls block",
			response: "plain text",
			wantOutput: func(t *testing.T, out string) {
				if !strings.HasPrefix(out, "plain text\n\n**Tool Calls:**\n```json\n") || !strings.Contains(out, `"name": "timed"`) {
					t.Errorf("output = %q", out)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer, exporter := newTestLangfuseTracer(t, false)
			ctx := WithToolCallsCollection(multitenancy.WithOrgID(context.Background(), "org-1"))
			for _, tc := range toolCalls {
				AddToolCallToContext(ctx, tc)
			}

			if _, err := tracer.TraceGeneration(ctx, "m", "p", tt.response, start, start.Add(time.Second), nil); err != nil {
				t.Fatalf("TraceGeneration: %v", err)
			}

			gen := spanByName(t, exporter, "llm.generation")
			attrs := attrMap(gen.Attributes)
			tt.wantOutput(t, attrs["langfuse.observation.output"])
			assertAttrs(t, attrs, map[string]string{
				"langfuse.observation.metadata.tool_calls_count":   "4",
				"langfuse.observation.metadata.tool_call_0.name":   "timed",
				"langfuse.observation.metadata.tool_call_0.result": "ok",
				"langfuse.observation.metadata.tool_call_2.error":  "kaboom",
			})
			if _, ok := attrs["langfuse.observation.metadata.tool_call_0.error"]; ok {
				t.Error("error attribute set for a successful tool call")
			}

			timed := spanByName(t, exporter, "timed")
			if !timed.StartTime.Equal(start) || timed.EndTime.Sub(timed.StartTime) != 3*time.Second {
				t.Errorf("timed span = %v..%v", timed.StartTime, timed.EndTime)
			}
			assertAttrs(t, attrMap(timed.Attributes), map[string]string{
				"tool.call_id":               "id-1",
				"tool.duration_ms":           "3000",
				"langfuse.user.id":           "org-1",
				"langfuse.observation.level": "info",
			})
			if timed.Parent.SpanID() != gen.SpanContext.SpanID() {
				t.Error("tool span is not a child of the generation span")
			}

			stamped := spanByName(t, exporter, "stamped")
			if !stamped.StartTime.Equal(start) || stamped.EndTime.Sub(stamped.StartTime) != 500*time.Millisecond {
				t.Errorf("stamped span = %v..%v", stamped.StartTime, stamped.EndTime)
			}

			failed := spanByName(t, exporter, "failed")
			assertAttrs(t, attrMap(failed.Attributes), map[string]string{
				"tool.error":                  "kaboom",
				"langfuse.observation.level":  "error",
				"langfuse.observation.output": "Error: kaboom",
			})
			if len(failed.Events) != 1 || failed.Events[0].Name != "exception" {
				t.Errorf("failed span events = %+v", failed.Events)
			}

			spanByName(t, exporter, "bare")
			if n := len(exporter.GetSpans()); n != 5 {
				t.Errorf("exported %d spans, want 5", n)
			}
		})
	}
}

func TestCreateToolCallSpans_NothingToDo(t *testing.T) {
	tracer, exporter := newTestLangfuseTracer(t, false)
	tracer.createToolCallSpansAsTraceItems(context.Background(), nil)
	(&OTELLangfuseTracer{}).createToolCallSpansAsTraceItems(context.Background(), []ToolCall{{Name: "x"}})
	if n := len(exporter.GetSpans()); n != 0 {
		t.Errorf("exported %d spans, want 0", n)
	}
}

func TestGetToolCallsFromContext_NoCollector(t *testing.T) {
	ctx := context.Background()
	AddToolCallToContext(ctx, ToolCall{Name: "dropped"})
	if calls := GetToolCallsFromContext(ctx); calls != nil {
		t.Errorf("calls = %v, want nil", calls)
	}
}

func TestTraceSpan(t *testing.T) {
	tracer, exporter := newTestLangfuseTracer(t, false)
	start := time.Unix(3000, 0)
	end := start.Add(time.Minute)

	id, err := tracer.TraceSpan(fullContext(), "work", start, end, map[string]any{"step": 2}, "parent")
	if err != nil {
		t.Fatalf("TraceSpan: %v", err)
	}
	got := spanByName(t, exporter, "work")
	if id != got.SpanContext.SpanID().String() {
		t.Errorf("id mismatch")
	}
	if !got.StartTime.Equal(start) || !got.EndTime.Equal(end) {
		t.Errorf("times = %v..%v", got.StartTime, got.EndTime)
	}
	assertAttrs(t, attrMap(got.Attributes), map[string]string{
		"langfuse.trace.name":                      "work",
		"langfuse.user.id":                         "org-1",
		"langfuse.observation.metadata.agent_name": "agent-1",
		"step": "2",
	})
}

func TestTraceEvent(t *testing.T) {
	tests := []struct {
		name       string
		input      any
		output     any
		wantEvents []string
	}{
		{name: "input and output", input: "in", output: 5, wantEvents: []string{"input", "output"}},
		{name: "no payload", wantEvents: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer, exporter := newTestLangfuseTracer(t, false)
			if _, err := tracer.TraceEvent(fullContext(), "evt", tt.input, tt.output, "warning", map[string]any{"m": true}, ""); err != nil {
				t.Fatalf("TraceEvent: %v", err)
			}
			got := spanByName(t, exporter, "evt")
			attrs := attrMap(got.Attributes)
			assertAttrs(t, attrs, map[string]string{
				"langfuse.observation.level":               "warning",
				"langfuse.observation.metadata.agent_name": "agent-1",
				"m": "true",
			})
			var names []string
			for _, e := range got.Events {
				names = append(names, e.Name)
			}
			if strings.Join(names, ",") != strings.Join(tt.wantEvents, ",") {
				t.Errorf("events = %v, want %v", names, tt.wantEvents)
			}
			if tt.input != nil {
				assertAttrs(t, attrs, map[string]string{"langfuse.trace.input": "in", "langfuse.trace.output": "5"})
			} else if _, ok := attrs["langfuse.trace.input"]; ok {
				t.Error("input attribute set without input")
			}
		})
	}
}

func TestOTELLangfuseTracer_StartTraceSession(t *testing.T) {
	tracer, exporter := newTestLangfuseTracer(t, false)

	ctx, span := tracer.StartTraceSession(fullContext(), "req-7")
	span.RecordError(errors.New("x"))
	span.End()

	if name, _ := GetTraceName(ctx); name != "req-7" {
		t.Errorf("trace name in context = %q", name)
	}
	if id, _ := GetRequestID(ctx); id != "req-7" {
		t.Errorf("request id in context = %q", id)
	}
	if !trace.SpanFromContext(ctx).SpanContext().IsValid() {
		t.Error("context carries no span")
	}
	got := spanByName(t, exporter, "request-session")
	assertAttrs(t, attrMap(got.Attributes), map[string]string{
		"langfuse.trace.name":                      "req-7",
		"langfuse.observation.type":                "span",
		"langfuse.user.id":                         "org-1",
		"langfuse.session.id":                      "conv-1",
		"langfuse.observation.metadata.agent_name": "agent-1",
	})
	if got.Status.Code != codes.Unset {
		t.Errorf("status = %v", got.Status)
	}
}

func TestOTELLangfuseTracer_FlushAndShutdown(t *testing.T) {
	tracer, _ := newTestLangfuseTracer(t, false)
	if err := tracer.Flush(); err != nil {
		t.Errorf("Flush: %v", err)
	}
	if err := tracer.Shutdown(); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

// otlpServer is a local stand-in for the Langfuse OTLP endpoint.
type otlpServer struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
	auths []string
}

func newOTLPServer(t *testing.T) *otlpServer {
	t.Helper()
	s := &otlpServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.paths = append(s.paths, r.URL.Path)
		s.auths = append(s.auths, r.Header.Get("Authorization"))
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *otlpServer) requests() ([]string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...), append([]string(nil), s.auths...)
}

func TestNewOTELLangfuseTracer_ExportsToHost(t *testing.T) {
	srv := newOTLPServer(t)

	tracer, err := NewOTELLangfuseTracer(LangfuseConfig{
		Enabled: true, PublicKey: "pk", SecretKey: "sk", Host: srv.URL, Environment: "dev", IncludeContent: true,
	})
	if err != nil {
		t.Fatalf("NewOTELLangfuseTracer: %v", err)
	}
	t.Cleanup(func() { _ = tracer.Shutdown() })
	if !tracer.ShouldIncludeContent() {
		t.Error("IncludeContent not carried over")
	}

	_, span := tracer.StartSpan(context.Background(), "exported")
	span.End()
	if err := tracer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	paths, auths := srv.requests()
	if len(paths) == 0 {
		t.Fatal("no spans were exported")
	}
	if paths[0] != "/api/public/otel/v1/traces" {
		t.Errorf("path = %q", paths[0])
	}
	// base64("pk:sk")
	if auths[0] != "Basic cGs6c2s=" {
		t.Errorf("authorization = %q", auths[0])
	}
}

func TestNewOTELLangfuseTracer_Config(t *testing.T) {
	tests := []struct {
		name     string
		cfg      LangfuseConfig
		wantErr  string
		wantOn   bool
		wantHost string
	}{
		{name: "disabled", cfg: LangfuseConfig{}, wantOn: false},
		{name: "missing keys", cfg: LangfuseConfig{Enabled: true, PublicKey: "pk"}, wantErr: "secret key and public key are required"},
		{name: "default host", cfg: LangfuseConfig{Enabled: true, PublicKey: "pk", SecretKey: "sk"}, wantOn: true, wantHost: "https://cloud.langfuse.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer, err := NewOTELLangfuseTracer(tt.cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewOTELLangfuseTracer: %v", err)
			}
			t.Cleanup(func() { _ = tracer.Shutdown() })
			if tracer.enabled != tt.wantOn {
				t.Errorf("enabled = %v, want %v", tracer.enabled, tt.wantOn)
			}
			if tracer.config.Host != tt.wantHost {
				t.Errorf("host = %q, want %q", tracer.config.Host, tt.wantHost)
			}
		})
	}
}

// disableLangfuseEnv makes the global config report Langfuse as disabled.
func disableLangfuseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LANGFUSE_ENABLED", "false")
	config.Reload()
	t.Cleanup(func() { config.Reload() })
}

func TestNewOTELLangfuseTracer_GlobalConfig(t *testing.T) {
	disableLangfuseEnv(t)
	tracer, err := NewOTELLangfuseTracer()
	if err != nil {
		t.Fatalf("NewOTELLangfuseTracer: %v", err)
	}
	if tracer.enabled {
		t.Error("tracer enabled although LANGFUSE_ENABLED=false")
	}
}

func TestNewOTELLangfuseTracerAsInterface(t *testing.T) {
	if _, err := NewOTELLangfuseTracerAsInterface(LangfuseConfig{Enabled: true}); err == nil {
		t.Error("expected an error for missing keys")
	}

	tracer, err := NewOTELLangfuseTracerAsInterface(LangfuseConfig{})
	if err != nil {
		t.Fatalf("NewOTELLangfuseTracerAsInterface: %v", err)
	}
	adapter, ok := tracer.(*OTELTracerAdapter)
	if !ok {
		t.Fatalf("got %T, want *OTELTracerAdapter", tracer)
	}
	ctx := context.Background()
	if gotCtx, _ := adapter.StartTraceSession(ctx, "s"); gotCtx != ctx {
		t.Error("disabled adapter changed the context")
	}
}

func TestOTELTracerAdapter_StartTraceSession(t *testing.T) {
	inner, exporter := newTestLangfuseTracer(t, false)
	ctx, span := NewOTELTracerAdapter(inner).StartTraceSession(context.Background(), "adapted")
	span.End()
	if name, _ := GetTraceName(ctx); name != "adapted" {
		t.Errorf("trace name = %q", name)
	}
	spanByName(t, exporter, "request-session")
}

func TestLangfuseTracer_Disabled(t *testing.T) {
	disableLangfuseEnv(t)
	for name, build := range map[string]func() (*LangfuseTracer, error){
		"explicit config": func() (*LangfuseTracer, error) { return NewLangfuseTracer(LangfuseConfig{}) },
		"global config":   func() (*LangfuseTracer, error) { return NewLangfuseTracer() },
	} {
		t.Run(name, func(t *testing.T) {
			tracer, err := build()
			if err != nil {
				t.Fatalf("NewLangfuseTracer: %v", err)
			}
			ctx := context.Background()
			now := time.Now()
			if id, err := tracer.TraceGeneration(ctx, "m", "p", "r", now, now, nil); id != "" || err != nil {
				t.Errorf("TraceGeneration = %q, %v", id, err)
			}
			if id, err := tracer.TraceSpan(ctx, "s", now, now, nil, ""); id != "" || err != nil {
				t.Errorf("TraceSpan = %q, %v", id, err)
			}
			if id, err := tracer.TraceEvent(ctx, "e", nil, nil, "", nil, ""); id != "" || err != nil {
				t.Errorf("TraceEvent = %q, %v", id, err)
			}
			if err := tracer.Flush(); err != nil {
				t.Errorf("Flush = %v", err)
			}
			if err := tracer.Shutdown(); err != nil {
				t.Errorf("Shutdown = %v", err)
			}
			if tracer.AsInterfaceTracer() != nil {
				t.Error("disabled tracer returned a non-nil interface tracer")
			}
		})
	}
}

func TestLangfuseTracer_MissingKeys(t *testing.T) {
	_, err := NewLangfuseTracer(LangfuseConfig{Enabled: true})
	if err == nil || !strings.Contains(err.Error(), "failed to create OTEL Langfuse tracer") {
		t.Fatalf("err = %v", err)
	}
}

func TestLangfuseTracer_Enabled(t *testing.T) {
	srv := newOTLPServer(t)
	tracer, err := NewLangfuseTracer(LangfuseConfig{Enabled: true, PublicKey: "pk", SecretKey: "sk", Host: srv.URL})
	if err != nil {
		t.Fatalf("NewLangfuseTracer: %v", err)
	}
	t.Cleanup(func() { _ = tracer.Shutdown() })

	ctx := context.Background()
	now := time.Now()
	for name, call := range map[string]func() (string, error){
		"generation": func() (string, error) { return tracer.TraceGeneration(ctx, "m", "p", "r", now, now, nil) },
		"span":       func() (string, error) { return tracer.TraceSpan(ctx, "s", now, now, nil, "") },
		"event":      func() (string, error) { return tracer.TraceEvent(ctx, "e", "in", "out", "info", nil, "") },
	} {
		id, err := call()
		if err != nil || len(id) != 16 {
			t.Errorf("%s = %q, %v; want a 16 char span id", name, id, err)
		}
	}

	if _, ok := tracer.AsInterfaceTracer().(*OTELTracerAdapter); !ok {
		t.Error("AsInterfaceTracer did not return an adapter")
	}
	if err := tracer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if paths, _ := srv.requests(); len(paths) == 0 {
		t.Error("no spans were exported")
	}
}

func TestLLMMiddlewareConstructors(t *testing.T) {
	inner, _ := newTestLangfuseTracer(t, false)

	byOTEL, ok := NewOTELLLMMiddleware(&fakeLLM{}, inner).(*TracedLLM)
	if !ok || byOTEL.tracer != inner {
		t.Errorf("NewOTELLLMMiddleware = %+v", byOTEL)
	}

	wrapper := &LangfuseTracer{otelTracer: inner, enabled: true}
	byLangfuse, ok := NewLLMMiddleware(&fakeLLM{}, wrapper).(*TracedLLM)
	if !ok {
		t.Fatal("NewLLMMiddleware did not return *TracedLLM")
	}
	if adapter, ok := byLangfuse.tracer.(*OTELTracerAdapter); !ok || adapter.otelTracer != inner {
		t.Errorf("NewLLMMiddleware tracer = %T", byLangfuse.tracer)
	}
}
