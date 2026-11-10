package microservice

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/pb"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// newCustomAgent builds a local agent whose run and stream paths are replaced
// by the given functions, so tests control every event.
func newCustomAgent(t *testing.T, run agent.CustomRunFunction, stream agent.CustomRunStreamFunction, opts ...agent.Option) *agent.Agent {
	t.Helper()
	base := []agent.Option{
		agent.WithLLM(&MockLLM{response: "llm"}),
		agent.WithName("custom-agent"),
		agent.WithDescription("an agent for tests"),
	}
	if run != nil {
		base = append(base, agent.WithCustomRunFunction(run))
	}
	if stream != nil {
		base = append(base, agent.WithCustomRunStreamFunction(stream))
	}
	a, err := agent.NewAgent(append(base, opts...)...)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a
}

// replayStream returns a stream function that emits events in order.
func replayStream(events ...interfaces.AgentStreamEvent) agent.CustomRunStreamFunction {
	return func(context.Context, string, *agent.Agent) (<-chan interfaces.AgentStreamEvent, error) {
		ch := make(chan interfaces.AgentStreamEvent, len(events))
		for _, e := range events {
			ch <- e
		}
		close(ch)
		return ch, nil
	}
}

func TestCreateMicroservice_Validation(t *testing.T) {
	local := newCustomAgent(t, nil, nil)
	tests := []struct {
		name    string
		agent   *agent.Agent
		port    int
		wantErr string
	}{
		{name: "nil agent", agent: nil, wantErr: "agent cannot be nil"},
		{name: "negative port", agent: local, port: -1, wantErr: "invalid port: -1"},
		{name: "port too high", agent: local, port: 70000, wantErr: "invalid port: 70000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateMicroservice(tt.agent, Config{Port: tt.port})
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestCreateMicroservice_RejectsRemoteAgent(t *testing.T) {
	remote := newRemoteAgent(t, startGRPCAgent(t, newCustomAgent(t, nil, nil)))
	if _, err := CreateMicroservice(remote, Config{}); err == nil || !strings.Contains(err.Error(), "remote agent") {
		t.Fatalf("err = %v", err)
	}
}

func TestAgentMicroservice_Lifecycle(t *testing.T) {
	a := newCustomAgent(t, nil, replayStream(
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventThinking, ThinkingStep: "hmm", Timestamp: time.Now()},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventContent, Content: "hello", Timestamp: time.Now()},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventToolCall, ToolCall: &interfaces.ToolCallEvent{ID: "1", Name: "search"}, Timestamp: time.Now()},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventToolResult, ToolCall: &interfaces.ToolCallEvent{ID: "1", Result: "found"}, Timestamp: time.Now()},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventError, Error: errors.New("minor"), Timestamp: time.Now()},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventComplete, Timestamp: time.Now()},
	))
	ms, err := CreateMicroservice(a, Config{})
	if err != nil {
		t.Fatalf("CreateMicroservice: %v", err)
	}
	if ms.GetAgent() != a {
		t.Error("GetAgent returned a different agent")
	}
	if _, err := ms.RunStream(context.Background(), "hi"); err == nil {
		t.Error("RunStream before Start should fail")
	}
	if err := ms.Stop(); err != nil {
		t.Errorf("Stop before Start = %v", err)
	}

	if err := ms.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = ms.Stop() })

	if err := ms.Start(); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Errorf("second Start = %v", err)
	}
	if ms.GetPort() == 0 {
		t.Fatal("auto-assigned port was not recorded")
	}
	if !strings.HasSuffix(ms.GetURL(), ":"+strconv.Itoa(ms.GetPort())) {
		t.Errorf("GetURL = %q", ms.GetURL())
	}
	if err := ms.WaitForReady(5 * time.Second); err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}
	if !ms.IsRunning() {
		t.Fatal("IsRunning = false after Start")
	}

	var mu sync.Mutex
	var got []string
	record := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, s)
	}
	ms.OnThinking(func(s string) { record("thinking:" + s) }).
		OnContent(func(s string) { record("content:" + s) }).
		OnToolCall(func(tc *interfaces.ToolCallEvent) { record("call:" + tc.Name) }).
		OnToolResult(func(tc *interfaces.ToolCallEvent) { record("result:" + tc.Result) }).
		OnError(func(err error) { record("error:" + err.Error()) }).
		OnComplete(func() { record("complete") })

	if err := ms.Stream(context.Background(), "go"); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	want := []string{"thinking:hmm", "content:hello", "call:search", "result:found", "error:minor", "complete"}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(got, want) {
		t.Errorf("handlers saw %v, want %v", got, want)
	}

	if err := ms.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if ms.IsRunning() {
		t.Error("IsRunning = true after Stop")
	}
}

func TestAgentMicroservice_StreamStartError(t *testing.T) {
	ms, err := CreateMicroservice(newCustomAgent(t, nil, nil), Config{})
	if err != nil {
		t.Fatalf("CreateMicroservice: %v", err)
	}
	if err := ms.Stream(context.Background(), "x"); err == nil {
		t.Error("Stream on a stopped service should fail")
	}
}

func TestAgentMicroservice_RunStreamServerError(t *testing.T) {
	a := newCustomAgent(t, nil, func(context.Context, string, *agent.Agent) (<-chan interfaces.AgentStreamEvent, error) {
		return nil, errors.New("cannot start")
	})
	ms, err := CreateMicroservice(a, Config{})
	if err != nil {
		t.Fatalf("CreateMicroservice: %v", err)
	}
	if err := ms.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = ms.Stop() })
	if err := ms.WaitForReady(5 * time.Second); err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}

	events, err := ms.RunStream(context.Background(), "x")
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	var last interfaces.AgentStreamEvent
	for e := range events {
		last = e
	}
	if last.Type != interfaces.AgentEventError || last.Error == nil || !strings.Contains(last.Error.Error(), "cannot start") {
		t.Errorf("last event = %+v", last)
	}
}

func TestAgentMicroservice_WaitForReadyTimeout(t *testing.T) {
	ms, err := CreateMicroservice(newCustomAgent(t, nil, nil), Config{})
	if err != nil {
		t.Fatalf("CreateMicroservice: %v", err)
	}
	err = ms.WaitForReady(60 * time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "failed to start") {
		t.Errorf("err = %v", err)
	}
}

func TestAgentMicroservice_StartPortInUse(t *testing.T) {
	first, err := CreateMicroservice(newCustomAgent(t, nil, nil), Config{})
	if err != nil {
		t.Fatalf("CreateMicroservice: %v", err)
	}
	if err := first.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = first.Stop() })
	mustBeReady(t, first)

	second, err := CreateMicroservice(newCustomAgent(t, nil, nil), Config{Port: first.GetPort()})
	if err != nil {
		t.Fatalf("CreateMicroservice: %v", err)
	}
	if err := second.Start(); err == nil || !strings.Contains(err.Error(), "failed to listen") {
		t.Errorf("Start on a used port = %v", err)
	}
}

func TestConvertGRPCResponseToAgentEvent(t *testing.T) {
	ts := time.UnixMilli(1700000000123)
	tests := []struct {
		name  string
		in    *pb.RunStreamResponse
		check func(t *testing.T, e interfaces.AgentStreamEvent)
	}{
		{
			name: "thinking with metadata and timestamp",
			in:   &pb.RunStreamResponse{EventType: pb.EventType_EVENT_TYPE_THINKING, Thinking: "t", Timestamp: ts.UnixMilli(), Metadata: map[string]string{"k": "v"}},
			check: func(t *testing.T, e interfaces.AgentStreamEvent) {
				if e.Type != interfaces.AgentEventThinking || e.ThinkingStep != "t" || !e.Timestamp.Equal(ts) || e.Metadata["k"] != "v" {
					t.Errorf("event = %+v", e)
				}
			},
		},
		{
			name: "content",
			in:   &pb.RunStreamResponse{EventType: pb.EventType_EVENT_TYPE_CONTENT, Chunk: "c"},
			check: func(t *testing.T, e interfaces.AgentStreamEvent) {
				if e.Type != interfaces.AgentEventContent || e.Content != "c" || e.Timestamp.IsZero() || e.Metadata != nil {
					t.Errorf("event = %+v", e)
				}
			},
		},
		{
			name: "tool call",
			in: &pb.RunStreamResponse{EventType: pb.EventType_EVENT_TYPE_TOOL_CALL, ToolCall: &pb.ToolCall{
				Id: "1", Name: "n", DisplayName: "N", Internal: true, Arguments: "{}", Result: "r", Status: "s",
			}},
			check: func(t *testing.T, e interfaces.AgentStreamEvent) {
				want := interfaces.ToolCallEvent{ID: "1", Name: "n", DisplayName: "N", Internal: true, Arguments: "{}", Result: "r", Status: "s"}
				if e.Type != interfaces.AgentEventToolCall || e.ToolCall == nil || *e.ToolCall != want {
					t.Errorf("event = %+v", e)
				}
			},
		},
		{
			name: "tool result without payload",
			in:   &pb.RunStreamResponse{EventType: pb.EventType_EVENT_TYPE_TOOL_RESULT},
			check: func(t *testing.T, e interfaces.AgentStreamEvent) {
				if e.Type != interfaces.AgentEventToolResult || e.ToolCall != nil {
					t.Errorf("event = %+v", e)
				}
			},
		},
		{
			name: "error with message",
			in:   &pb.RunStreamResponse{EventType: pb.EventType_EVENT_TYPE_ERROR, Error: "bad"},
			check: func(t *testing.T, e interfaces.AgentStreamEvent) {
				if e.Type != interfaces.AgentEventError || e.Error == nil || e.Error.Error() != "bad" {
					t.Errorf("event = %+v", e)
				}
			},
		},
		{
			name: "error without message",
			in:   &pb.RunStreamResponse{EventType: pb.EventType_EVENT_TYPE_ERROR},
			check: func(t *testing.T, e interfaces.AgentStreamEvent) {
				if e.Type != interfaces.AgentEventError || e.Error != nil {
					t.Errorf("event = %+v", e)
				}
			},
		},
		{
			name: "complete",
			in:   &pb.RunStreamResponse{EventType: pb.EventType_EVENT_TYPE_COMPLETE},
			check: func(t *testing.T, e interfaces.AgentStreamEvent) {
				if e.Type != interfaces.AgentEventComplete {
					t.Errorf("event = %+v", e)
				}
			},
		},
		{
			name: "unknown type falls back to content",
			in:   &pb.RunStreamResponse{EventType: pb.EventType(99), Chunk: "x"},
			check: func(t *testing.T, e interfaces.AgentStreamEvent) {
				if e.Type != interfaces.AgentEventContent || e.Content != "x" {
					t.Errorf("event = %+v", e)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, convertGRPCResponseToAgentEvent(tt.in))
		})
	}
}

func TestExecuteHandlers_IgnoresUnknownType(t *testing.T) {
	ms := &AgentMicroservice{}
	called := false
	ms.OnContent(func(string) { called = true })
	ms.executeHandlers(interfaces.AgentStreamEvent{Type: "other"})
	if called {
		t.Error("content handler ran for an unknown event type")
	}
}

func TestMicroserviceManager(t *testing.T) {
	mm := NewMicroserviceManager()
	svcA, err := CreateMicroservice(newCustomAgent(t, nil, nil), Config{})
	if err != nil {
		t.Fatalf("CreateMicroservice: %v", err)
	}
	svcB, err := CreateMicroservice(newCustomAgent(t, nil, nil), Config{})
	if err != nil {
		t.Fatalf("CreateMicroservice: %v", err)
	}

	if err := mm.RegisterService("a", svcA); err != nil {
		t.Fatalf("RegisterService: %v", err)
	}
	if err := mm.RegisterService("b", svcB); err != nil {
		t.Fatalf("RegisterService: %v", err)
	}
	if err := mm.RegisterService("a", svcB); err == nil {
		t.Error("duplicate RegisterService should fail")
	}

	names := mm.ListServices()
	slices.Sort(names)
	if !slices.Equal(names, []string{"a", "b"}) {
		t.Errorf("ListServices = %v", names)
	}
	if got, ok := mm.GetService("a"); !ok || got != svcA {
		t.Error("GetService(a) did not return the registered service")
	}
	if _, ok := mm.GetService("missing"); ok {
		t.Error("GetService(missing) reported found")
	}

	for name, call := range map[string]func(string) error{"StartService": mm.StartService, "StopService": mm.StopService} {
		if err := call("missing"); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("%s(missing) = %v", name, err)
		}
	}

	if err := mm.StartService("a"); err != nil {
		t.Fatalf("StartService: %v", err)
	}
	t.Cleanup(func() { _ = mm.StopAll() })
	mustBeReady(t, svcA)
	if !svcA.IsRunning() {
		t.Error("service a is not running after StartService")
	}
	if err := mm.StopService("a"); err != nil {
		t.Fatalf("StopService: %v", err)
	}

	// Use a fresh manager so StartAll only sees a service that never ran.
	mmB := NewMicroserviceManager()
	if err := mmB.RegisterService("b", svcB); err != nil {
		t.Fatalf("RegisterService: %v", err)
	}
	if err := mmB.StartAll(); err != nil {
		t.Fatalf("StartAll: %v", err)
	}
	mustBeReady(t, svcB)
	if !svcB.IsRunning() {
		t.Error("service b is not running after StartAll")
	}
	if err := mmB.StopAll(); err != nil {
		t.Fatalf("StopAll: %v", err)
	}
	if svcB.IsRunning() {
		t.Error("service b still running after StopAll")
	}
}

func TestMicroserviceManager_StartAllError(t *testing.T) {
	svc, err := CreateMicroservice(newCustomAgent(t, nil, nil), Config{})
	if err != nil {
		t.Fatalf("CreateMicroservice: %v", err)
	}
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = svc.Stop() })
	mustBeReady(t, svc)

	mm := NewMicroserviceManager()
	if err := mm.RegisterService("dup", svc); err != nil {
		t.Fatalf("RegisterService: %v", err)
	}
	if err := mm.StartAll(); err == nil || !strings.Contains(err.Error(), "failed to start service dup") {
		t.Errorf("StartAll = %v", err)
	}
}

// mustBeReady waits until ms answers health checks. Stopping a service before
// it serves races with its start goroutine, so tests always wait first.
func mustBeReady(t *testing.T, ms *AgentMicroservice) {
	t.Helper()
	if err := ms.WaitForReady(5 * time.Second); err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}
}
