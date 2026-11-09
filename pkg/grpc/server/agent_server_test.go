package server

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/pb"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// fakeLLM returns a fixed response or error for every call.
type fakeLLM struct {
	response string
	err      error
	model    string
}

func (f *fakeLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	return f.response, f.err
}

func (f *fakeLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	return f.response, f.err
}

func (f *fakeLLM) GenerateDetailed(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &interfaces.LLMResponse{Content: f.response}, nil
}

func (f *fakeLLM) GenerateWithToolsDetailed(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return f.GenerateDetailed(ctx, prompt, options...)
}

func (f *fakeLLM) Name() string            { return "fake-llm" }
func (f *fakeLLM) SupportsStreaming() bool { return false }
func (f *fakeLLM) GetModel() string        { return f.model }

// fakeTool records the arguments it was executed with.
type fakeTool struct {
	mu   sync.Mutex
	args []string
}

func (t *fakeTool) Name() string        { return "echo" }
func (t *fakeTool) Description() string { return "echoes input" }
func (t *fakeTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}
func (t *fakeTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{"text": {Type: "string", Description: "text", Required: true}}
}
func (t *fakeTool) Execute(ctx context.Context, args string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.args = append(t.args, args)
	return "echoed", nil
}

func (t *fakeTool) calls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.args)
}

// capturedContext holds values read from the context passed to the agent.
type capturedContext struct {
	orgID          string
	conversationID string
	jwt            any
	custom         any
}

func captureContext(ctx context.Context) capturedContext {
	var c capturedContext
	c.orgID, _ = multitenancy.GetOrgID(ctx)
	c.conversationID, _ = memory.GetConversationID(ctx)
	c.jwt = ctx.Value(JWTTokenKey)
	c.custom = ctx.Value(contextKey("tenant"))
	return c
}

func newTestAgent(t *testing.T, opts ...agent.Option) *agent.Agent {
	t.Helper()
	base := []agent.Option{
		agent.WithLLM(&fakeLLM{response: "llm says hi", model: "fake-model"}),
		agent.WithName("test-agent"),
	}
	a, err := agent.NewAgent(append(base, opts...)...)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a
}

// startBufconn serves srv over an in-memory listener and returns a client.
func startBufconn(t *testing.T, srv *AgentServer) pb.AgentServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	pb.RegisterAgentServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewAgentServiceClient(conn)
}

func withBearer(ctx context.Context, value string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", value)
}

func TestRun_EmptyInput(t *testing.T) {
	client := startBufconn(t, NewAgentServer(newTestAgent(t)))

	_, err := client.Run(context.Background(), &pb.RunRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestRun_PropagatesContext(t *testing.T) {
	tests := []struct {
		name    string
		auth    string
		wantJWT any
	}{
		{name: "bearer token", auth: "Bearer abc.def", wantJWT: "abc.def"},
		{name: "non bearer token is ignored", auth: "Basic xyz", wantJWT: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got capturedContext
			a := newTestAgent(t, agent.WithCustomRunFunction(func(ctx context.Context, input string, _ *agent.Agent) (string, error) {
				got = captureContext(ctx)
				return "echo: " + input, nil
			}))
			client := startBufconn(t, NewAgentServer(a))

			resp, err := client.Run(withBearer(context.Background(), tt.auth), &pb.RunRequest{
				Input:          "hello",
				OrgId:          "org-1",
				ConversationId: "conv-1",
				Context:        map[string]string{"tenant": "acme"},
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if resp.Output != "echo: hello" || resp.Error != "" {
				t.Errorf("unexpected response: %+v", resp)
			}
			if resp.Metadata["agent_name"] != "test-agent" {
				t.Errorf("agent_name = %q", resp.Metadata["agent_name"])
			}
			want := capturedContext{orgID: "org-1", conversationID: "conv-1", jwt: tt.wantJWT, custom: "acme"}
			if got != want {
				t.Errorf("context = %+v, want %+v", got, want)
			}
		})
	}
}

func TestRun_AgentErrorIsReturnedInBody(t *testing.T) {
	a := newTestAgent(t, agent.WithCustomRunFunction(func(context.Context, string, *agent.Agent) (string, error) {
		return "", errors.New("boom")
	}))
	client := startBufconn(t, NewAgentServer(a))

	resp, err := client.Run(context.Background(), &pb.RunRequest{Input: "hi"})
	if err != nil {
		t.Fatalf("Run returned transport error: %v", err)
	}
	if resp.Error != "boom" || resp.Output != "" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func recvAll(t *testing.T, stream grpc.ServerStreamingClient[pb.RunStreamResponse]) ([]*pb.RunStreamResponse, error) {
	t.Helper()
	var out []*pb.RunStreamResponse
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, msg)
	}
}

func TestRunStream_EmptyInput(t *testing.T) {
	client := startBufconn(t, NewAgentServer(newTestAgent(t)))

	stream, err := client.RunStream(context.Background(), &pb.RunRequest{})
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	_, err = recvAll(t, stream)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestRunStream_ConvertsEvents(t *testing.T) {
	ts := time.UnixMilli(1700000000000)
	var got capturedContext
	events := []interfaces.AgentStreamEvent{
		{Type: interfaces.AgentEventContent, Content: "hello", Timestamp: ts, Metadata: map[string]interface{}{"s": "v", "n": 42}},
		{Type: interfaces.AgentEventThinking, ThinkingStep: "pondering", Timestamp: ts},
		{Type: interfaces.AgentEventToolCall, Timestamp: ts, ToolCall: &interfaces.ToolCallEvent{
			ID: "c1", Name: "echo", DisplayName: "Echo", Internal: true, Arguments: `{"a":1}`, Result: "r", Status: "executing",
		}},
		{Type: interfaces.AgentEventToolResult, Timestamp: ts, Error: errors.New("tool failed")},
		{Type: interfaces.AgentEventError, Timestamp: ts, Error: errors.New("agent failed")},
		{Type: interfaces.AgentEventType("unknown"), Content: "x", Timestamp: ts},
		{Type: interfaces.AgentEventComplete, Timestamp: ts},
	}
	a := newTestAgent(t, agent.WithCustomRunStreamFunction(func(ctx context.Context, input string, _ *agent.Agent) (<-chan interfaces.AgentStreamEvent, error) {
		got = captureContext(ctx)
		ch := make(chan interfaces.AgentStreamEvent, len(events))
		for _, e := range events {
			ch <- e
		}
		close(ch)
		return ch, nil
	}))
	client := startBufconn(t, NewAgentServer(a))

	stream, err := client.RunStream(withBearer(context.Background(), "Bearer tok"), &pb.RunRequest{
		Input: "go", OrgId: "org-2", ConversationId: "conv-2", Context: map[string]string{"tenant": "t"},
	})
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	msgs, err := recvAll(t, stream)
	if err != nil {
		t.Fatalf("recv: %v", err)
	}

	want := capturedContext{orgID: "org-2", conversationID: "conv-2", jwt: "tok", custom: "t"}
	if got != want {
		t.Errorf("context = %+v, want %+v", got, want)
	}

	// One message per event plus the trailing completion message.
	if len(msgs) != len(events)+1 {
		t.Fatalf("got %d messages, want %d", len(msgs), len(events)+1)
	}

	wantTypes := []pb.EventType{
		pb.EventType_EVENT_TYPE_CONTENT,
		pb.EventType_EVENT_TYPE_THINKING,
		pb.EventType_EVENT_TYPE_TOOL_CALL,
		pb.EventType_EVENT_TYPE_TOOL_RESULT,
		pb.EventType_EVENT_TYPE_ERROR,
		pb.EventType_EVENT_TYPE_CONTENT,
		pb.EventType_EVENT_TYPE_COMPLETE,
		pb.EventType_EVENT_TYPE_COMPLETE,
	}
	for i, m := range msgs {
		if m.EventType != wantTypes[i] {
			t.Errorf("msg %d type = %v, want %v", i, m.EventType, wantTypes[i])
		}
	}

	if msgs[0].Chunk != "hello" || msgs[0].Timestamp != ts.UnixMilli() {
		t.Errorf("content msg = %+v", msgs[0])
	}
	if msgs[0].Metadata["s"] != "v" || msgs[0].Metadata["n"] != "42" {
		t.Errorf("metadata = %v", msgs[0].Metadata)
	}
	if msgs[1].Thinking != "pondering" {
		t.Errorf("thinking = %q", msgs[1].Thinking)
	}
	tc := msgs[2].ToolCall
	if tc == nil || tc.Id != "c1" || tc.Name != "echo" || tc.DisplayName != "Echo" || !tc.Internal || tc.Arguments != `{"a":1}` || tc.Result != "r" || tc.Status != "executing" {
		t.Errorf("tool call = %+v", tc)
	}
	// Tool result errors keep the tool result type and carry no error text.
	if msgs[3].Error != "" {
		t.Errorf("tool result error = %q, want empty", msgs[3].Error)
	}
	if msgs[4].Error != "agent failed" {
		t.Errorf("error msg = %q", msgs[4].Error)
	}
	for i, m := range msgs {
		final := i >= len(events)-1
		if m.IsFinal != final {
			t.Errorf("msg %d IsFinal = %v, want %v", i, m.IsFinal, final)
		}
	}
}

func TestRunStream_StartError(t *testing.T) {
	a := newTestAgent(t, agent.WithCustomRunStreamFunction(func(context.Context, string, *agent.Agent) (<-chan interfaces.AgentStreamEvent, error) {
		return nil, errors.New("cannot stream")
	}))
	client := startBufconn(t, NewAgentServer(a))

	stream, err := client.RunStream(context.Background(), &pb.RunRequest{Input: "go"})
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	_, err = recvAll(t, stream)
	if status.Code(err) != codes.Internal || !strings.Contains(err.Error(), "cannot stream") {
		t.Fatalf("expected Internal error, got %v", err)
	}
}

func TestConvertEventType(t *testing.T) {
	s := NewAgentServer(nil)
	tests := map[interfaces.AgentEventType]pb.EventType{
		interfaces.AgentEventContent:    pb.EventType_EVENT_TYPE_CONTENT,
		interfaces.AgentEventThinking:   pb.EventType_EVENT_TYPE_THINKING,
		interfaces.AgentEventToolCall:   pb.EventType_EVENT_TYPE_TOOL_CALL,
		interfaces.AgentEventToolResult: pb.EventType_EVENT_TYPE_TOOL_RESULT,
		interfaces.AgentEventError:      pb.EventType_EVENT_TYPE_ERROR,
		interfaces.AgentEventComplete:   pb.EventType_EVENT_TYPE_COMPLETE,
		"something-else":                pb.EventType_EVENT_TYPE_CONTENT,
	}
	for in, want := range tests {
		if got := s.convertEventType(in); got != want {
			t.Errorf("convertEventType(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestGetMetadata(t *testing.T) {
	tests := []struct {
		name       string
		llm        *fakeLLM
		opts       []agent.Option
		wantModel  string
		wantTools  string
		wantMemory string
	}{
		{
			name:       "model from GetModel",
			llm:        &fakeLLM{model: "fake-model"},
			wantModel:  "fake-model",
			wantTools:  "0",
			wantMemory: "none",
		},
		{
			name:       "empty model falls back to llm name",
			llm:        &fakeLLM{},
			opts:       []agent.Option{agent.WithTools(&fakeTool{}), agent.WithMemory(memory.NewConversationBuffer())},
			wantModel:  "fake-llm",
			wantTools:  "1",
			wantMemory: "conversation",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := append([]agent.Option{
				agent.WithLLM(tt.llm),
				agent.WithName("meta-agent"),
				agent.WithDescription("describes things"),
				agent.WithSystemPrompt("be helpful"),
			}, tt.opts...)
			a, err := agent.NewAgent(opts...)
			if err != nil {
				t.Fatalf("NewAgent: %v", err)
			}
			client := startBufconn(t, NewAgentServer(a))

			resp, err := client.GetMetadata(context.Background(), &pb.MetadataRequest{})
			if err != nil {
				t.Fatalf("GetMetadata: %v", err)
			}
			if resp.Name != "meta-agent" || resp.Description != "describes things" || resp.SystemPrompt != "be helpful" {
				t.Errorf("unexpected identity: %+v", resp)
			}
			p := resp.Properties
			if p["llm_name"] != "fake-llm" || p["llm_model"] != tt.wantModel || p["tool_count"] != tt.wantTools || p["memory"] != tt.wantMemory {
				t.Errorf("properties = %v", p)
			}
			if strings.Join(resp.Capabilities, ",") != "run,metadata,health" {
				t.Errorf("capabilities = %v", resp.Capabilities)
			}
		})
	}
}

func TestCapabilitiesAndHealth(t *testing.T) {
	client := startBufconn(t, NewAgentServer(newTestAgent(t)))
	ctx := context.Background()

	caps, err := client.GetCapabilities(ctx, &pb.CapabilitiesRequest{})
	if err != nil {
		t.Fatalf("GetCapabilities: %v", err)
	}
	if !caps.SupportsStreaming || !caps.SupportsMemory || !caps.SupportsExecutionPlans {
		t.Errorf("capabilities = %+v", caps)
	}

	health, err := client.Health(ctx, &pb.HealthRequest{})
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if health.Status != pb.HealthResponse_SERVING {
		t.Errorf("health status = %v", health.Status)
	}
}

func TestReady(t *testing.T) {
	unnamed, err := agent.NewAgent(agent.WithLLM(&fakeLLM{}))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	tests := []struct {
		name      string
		agent     *agent.Agent
		wantReady bool
		wantMsg   string
	}{
		{name: "nil agent", agent: nil, wantReady: false, wantMsg: "Agent is not initialized"},
		{name: "unnamed agent", agent: unnamed, wantReady: false, wantMsg: "Agent name is not set"},
		{name: "named agent", agent: newTestAgent(t), wantReady: true, wantMsg: "Agent service is ready"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := NewAgentServer(tt.agent).Ready(context.Background(), &pb.ReadinessRequest{})
			if err != nil {
				t.Fatalf("Ready: %v", err)
			}
			if resp.Ready != tt.wantReady || resp.Message != tt.wantMsg {
				t.Errorf("got %+v", resp)
			}
		})
	}
}

const planJSON = `{"description":"echo plan","steps":[{"toolName":"echo","description":"say it","input":"hi","parameters":{"text":"hi","n":2}}]}`

func TestGenerateExecutionPlan(t *testing.T) {
	a := newTestAgent(t,
		agent.WithLLM(&fakeLLM{response: "Here is the plan: " + planJSON}),
		agent.WithTools(&fakeTool{}),
	)
	client := startBufconn(t, NewAgentServer(a))

	resp, err := client.GenerateExecutionPlan(withBearer(context.Background(), "Bearer t"), &pb.PlanRequest{
		Input: "do it", OrgId: "o", ConversationId: "c", Context: map[string]string{"k": "v"},
	})
	if err != nil {
		t.Fatalf("GenerateExecutionPlan: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.PlanId == "" || !strings.HasPrefix(resp.FormattedPlan, "Execution Plan: ") {
		t.Errorf("unexpected plan: %+v", resp)
	}
	if len(resp.Steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(resp.Steps))
	}
	step := resp.Steps[0]
	if step.Id != "step_1" || step.ToolName != "echo" || step.Description != "say it" {
		t.Errorf("step = %+v", step)
	}
	if step.Parameters["text"] != "hi" || step.Parameters["n"] != "2" {
		t.Errorf("parameters = %v", step.Parameters)
	}
}

func TestGenerateExecutionPlan_Error(t *testing.T) {
	// An agent without tools cannot plan.
	client := startBufconn(t, NewAgentServer(newTestAgent(t)))

	resp, err := client.GenerateExecutionPlan(context.Background(), &pb.PlanRequest{Input: "do it"})
	if err != nil {
		t.Fatalf("GenerateExecutionPlan: %v", err)
	}
	if !strings.HasPrefix(resp.Error, "Failed to generate execution plan:") {
		t.Errorf("error = %q", resp.Error)
	}
}

// storedPlanID runs the agent once so it stores a plan awaiting approval.
func storedPlanID(t *testing.T, a *agent.Agent) string {
	t.Helper()
	if _, err := a.Run(context.Background(), "make a plan"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tasks := a.ListTasks()
	if len(tasks) != 1 {
		t.Fatalf("stored plans = %d, want 1", len(tasks))
	}
	return tasks[0].TaskID
}

func TestApproveExecutionPlan(t *testing.T) {
	tests := []struct {
		name          string
		llm           *fakeLLM
		req           func(id string) *pb.ApprovalRequest
		wantResult    string
		wantErrPrefix string
		wantToolCalls int
	}{
		{
			name:          "unknown plan",
			llm:           &fakeLLM{response: planJSON},
			req:           func(string) *pb.ApprovalRequest { return &pb.ApprovalRequest{PlanId: "missing", Approved: true} },
			wantErrPrefix: "Plan with ID missing not found",
		},
		{
			name:       "rejected",
			llm:        &fakeLLM{response: planJSON},
			req:        func(id string) *pb.ApprovalRequest { return &pb.ApprovalRequest{PlanId: id} },
			wantResult: "Plan rejected by user",
		},
		{
			name:          "approved executes the plan",
			llm:           &fakeLLM{response: planJSON},
			req:           func(id string) *pb.ApprovalRequest { return &pb.ApprovalRequest{PlanId: id, Approved: true} },
			wantToolCalls: 1,
		},
		{
			name: "approved with modifications",
			llm:  &fakeLLM{response: planJSON},
			req: func(id string) *pb.ApprovalRequest {
				return &pb.ApprovalRequest{PlanId: id, Approved: true, Modifications: "louder"}
			},
			wantToolCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &fakeTool{}
			a := newTestAgent(t, agent.WithLLM(tt.llm), agent.WithTools(tool))
			id := storedPlanID(t, a)
			client := startBufconn(t, NewAgentServer(a))

			resp, err := client.ApproveExecutionPlan(context.Background(), tt.req(id))
			if err != nil {
				t.Fatalf("ApproveExecutionPlan: %v", err)
			}
			if tt.wantErrPrefix != "" {
				if !strings.HasPrefix(resp.Error, tt.wantErrPrefix) {
					t.Errorf("error = %q, want prefix %q", resp.Error, tt.wantErrPrefix)
				}
				return
			}
			if resp.Error != "" {
				t.Fatalf("unexpected error: %s", resp.Error)
			}
			if tt.wantResult != "" && resp.Result != tt.wantResult {
				t.Errorf("result = %q, want %q", resp.Result, tt.wantResult)
			}
			if tool.calls() != tt.wantToolCalls {
				t.Errorf("tool calls = %d, want %d", tool.calls(), tt.wantToolCalls)
			}
		})
	}
}

func TestApproveExecutionPlan_ModifyError(t *testing.T) {
	llm := &fakeLLM{response: planJSON}
	a := newTestAgent(t, agent.WithLLM(llm), agent.WithTools(&fakeTool{}))
	id := storedPlanID(t, a)
	client := startBufconn(t, NewAgentServer(a))

	// The modification call reuses the LLM, which now returns no JSON.
	llm.response = "no json here"
	resp, err := client.ApproveExecutionPlan(context.Background(), &pb.ApprovalRequest{PlanId: id, Approved: true, Modifications: "x"})
	if err != nil {
		t.Fatalf("ApproveExecutionPlan: %v", err)
	}
	if !strings.HasPrefix(resp.Error, "Failed to modify plan:") {
		t.Errorf("error = %q", resp.Error)
	}
}

func TestStartStopLifecycle(t *testing.T) {
	srv := NewAgentServer(newTestAgent(t))
	if srv.GetPort() != 0 {
		t.Fatalf("GetPort before start = %d, want 0", srv.GetPort())
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	served := make(chan error, 1)
	go func() { served <- srv.StartWithListener(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hc := grpc_health_v1.NewHealthClient(conn)
	for _, svc := range []string{"", "AgentService"} {
		resp, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: svc}, grpc.WaitForReady(true))
		if err != nil {
			t.Fatalf("health check %q: %v", svc, err)
		}
		if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("health %q = %v, want SERVING", svc, resp.Status)
		}
	}

	if got, want := srv.GetPort(), lis.Addr().(*net.TCPAddr).Port; got != want {
		t.Errorf("GetPort = %d, want %d", got, want)
	}

	srv.Stop()
	if err := <-served; err != nil {
		t.Errorf("serve returned %v after Stop", err)
	}
}

func TestStart_InvalidPort(t *testing.T) {
	err := NewAgentServer(newTestAgent(t)).Start(-1)
	if err == nil || !strings.Contains(err.Error(), "failed to listen on port -1") {
		t.Fatalf("expected listen error, got %v", err)
	}
}

func TestStop_NotStarted(t *testing.T) {
	// Stop on a server that never started must not panic.
	NewAgentServer(nil).Stop()
	(&AgentServer{}).Stop()
}

func TestFormatExecutionPlan(t *testing.T) {
	got := formatExecutionPlan(struct{ A int }{A: 1})
	if got != "Execution Plan: {A:1}" {
		t.Errorf("formatExecutionPlan = %q", got)
	}
}
