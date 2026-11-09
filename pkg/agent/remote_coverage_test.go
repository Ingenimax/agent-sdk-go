package agent

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/pb"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// fakeAgentService is an in-process gRPC agent service. Run answers
// "remote:<input>"; an input of "fail" returns an application error.
type fakeAgentService struct {
	pb.UnimplementedAgentServiceServer

	mu        sync.Mutex
	lastOrg   string
	lastAuth  string
	callCount int
}

func (s *fakeAgentService) record(ctx context.Context, req *pb.RunRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callCount++
	s.lastOrg = req.OrgId
	s.lastAuth = ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("authorization"); len(v) > 0 {
			s.lastAuth = v[0]
		}
	}
}

func (s *fakeAgentService) seen() (org, auth string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastOrg, s.lastAuth
}

func (s *fakeAgentService) Run(ctx context.Context, req *pb.RunRequest) (*pb.RunResponse, error) {
	s.record(ctx, req)
	if req.Input == "fail" {
		return &pb.RunResponse{Error: "remote refused"}, nil
	}
	return &pb.RunResponse{Output: "remote:" + req.Input}, nil
}

func (s *fakeAgentService) RunStream(req *pb.RunRequest, stream grpc.ServerStreamingServer[pb.RunStreamResponse]) error {
	s.record(stream.Context(), req)
	if req.Input == "fail" {
		return stream.Send(&pb.RunStreamResponse{Error: "remote refused"})
	}
	for _, resp := range []*pb.RunStreamResponse{
		{EventType: pb.EventType_EVENT_TYPE_THINKING, Thinking: "hmm"},
		{EventType: pb.EventType_EVENT_TYPE_CONTENT, Chunk: "remote:" + req.Input},
	} {
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
	return nil
}

func (s *fakeAgentService) GetMetadata(context.Context, *pb.MetadataRequest) (*pb.MetadataResponse, error) {
	return &pb.MetadataResponse{
		Name:         "far-away",
		Description:  "a remote agent",
		SystemPrompt: "be remote",
		Properties:   map[string]string{"region": "eu"},
	}, nil
}

// startFakeAgentService serves fakeAgentService on a loopback port and
// returns its address.
func startFakeAgentService(t *testing.T) (string, *fakeAgentService) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	svc := &fakeAgentService{}
	srv := grpc.NewServer()
	pb.RegisterAgentServiceServer(srv, svc)
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), svc
}

func newRemoteAgent(t *testing.T, options ...Option) (*Agent, *fakeAgentService) {
	t.Helper()
	addr, svc := startFakeAgentService(t)
	a, err := NewAgent(append([]Option{WithURL(addr), WithRemoteTimeout(10 * time.Second)}, options...)...)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect() })
	return a, svc
}

func collectEvents(t *testing.T, events <-chan interfaces.AgentStreamEvent) (content, thinking string, errs []error) {
	t.Helper()
	for e := range events {
		content += e.Content
		thinking += e.ThinkingStep
		if e.Error != nil {
			errs = append(errs, e.Error)
		}
	}
	return content, thinking, errs
}

func TestRemoteAgentMetadataAndAccessors(t *testing.T) {
	a, _ := newRemoteAgent(t)

	if !a.IsRemote() || a.GetRemoteURL() == "" {
		t.Errorf("IsRemote = %v, URL = %q", a.IsRemote(), a.GetRemoteURL())
	}
	// Name and description are fetched from the remote side when unset.
	if a.GetName() != "far-away" || a.GetDescription() != "a remote agent" {
		t.Errorf("name/description = %q / %q", a.GetName(), a.GetDescription())
	}
	md, err := a.GetRemoteMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if md["name"] != "far-away" || md["system_prompt"] != "be remote" || md["region"] != "eu" {
		t.Errorf("metadata = %+v", md)
	}

	// Locally set values win over remote metadata.
	b, _ := newRemoteAgent(t, WithName("mine"), WithDescription("my desc"))
	if b.GetName() != "mine" || b.GetDescription() != "my desc" {
		t.Errorf("name/description = %q / %q", b.GetName(), b.GetDescription())
	}

	if err := a.Disconnect(); err != nil {
		t.Errorf("Disconnect() error = %v", err)
	}
}

func TestRemoteAgentRunPaths(t *testing.T) {
	a, svc := newRemoteAgent(t, WithOrgID("org-1"), WithName("far"))
	ctx := context.Background()

	got, err := a.Run(ctx, "hello")
	if err != nil || got != "remote:hello" {
		t.Fatalf("Run() = %q, %v", got, err)
	}
	if org, _ := svc.seen(); org != "org-1" {
		t.Errorf("org sent = %q", org)
	}

	resp, err := a.RunDetailed(ctx, "detail")
	if err != nil || resp.Content != "remote:detail" {
		t.Fatalf("RunDetailed() = %+v, %v", resp, err)
	}
	if resp.ExecutionSummary.SubAgentCalls != 1 || len(resp.ExecutionSummary.UsedSubAgents) != 1 || resp.ExecutionSummary.UsedSubAgents[0] != "far" {
		t.Errorf("execution summary = %+v", resp.ExecutionSummary)
	}

	got, err = a.RunWithAuth(ctx, "secure", "tok")
	if err != nil || got != "remote:secure" {
		t.Fatalf("RunWithAuth() = %q, %v", got, err)
	}
	if _, auth := svc.seen(); auth != "Bearer tok" {
		t.Errorf("auth header = %q", auth)
	}

	detailed, err := a.RunWithAuthDetailed(ctx, "secure2", "tok2")
	if err != nil || detailed.Content != "remote:secure2" || detailed.Metadata["auth_enabled"] != true {
		t.Fatalf("RunWithAuthDetailed() = %+v, %v", detailed, err)
	}
	if detailed.ExecutionSummary.SubAgentCalls != 1 {
		t.Errorf("execution summary = %+v", detailed.ExecutionSummary)
	}

	if _, err := a.Run(ctx, "fail"); err == nil || !strings.Contains(err.Error(), "remote refused") {
		t.Errorf("Run(fail) error = %v", err)
	}
	if _, err := a.RunWithAuth(ctx, "fail", "tok"); err == nil || !strings.Contains(err.Error(), "remote refused") {
		t.Errorf("RunWithAuth(fail) error = %v", err)
	}

	// Remote agents take text only.
	if _, err := a.RunWithContent(ctx, imageContent("look")); err == nil {
		t.Error("expected error for media input to a remote agent")
	}
	if _, err := a.RunWithContentStream(ctx, imageContent("look")); err == nil {
		t.Error("expected error for media stream input to a remote agent")
	}
}

func TestRemoteAgentStreaming(t *testing.T) {
	a, svc := newRemoteAgent(t, WithOrgID("org-2"))
	ctx := context.Background()

	events, err := a.RunStream(ctx, "stream")
	if err != nil {
		t.Fatal(err)
	}
	content, thinking, errs := collectEvents(t, events)
	if content != "remote:stream" || thinking != "hmm" || len(errs) != 0 {
		t.Errorf("stream = %q / %q / %v", content, thinking, errs)
	}
	if org, _ := svc.seen(); org != "org-2" {
		t.Errorf("org sent = %q", org)
	}

	events, err = a.RunStreamWithAuth(ctx, "auth-stream", "tok")
	if err != nil {
		t.Fatal(err)
	}
	content, _, errs = collectEvents(t, events)
	if content != "remote:auth-stream" || len(errs) != 0 {
		t.Errorf("auth stream = %q / %v", content, errs)
	}
	if _, auth := svc.seen(); auth != "Bearer tok" {
		t.Errorf("auth header = %q", auth)
	}

	events, err = a.RunStream(ctx, "fail")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, errs := collectEvents(t, events); len(errs) != 1 || !strings.Contains(errs[0].Error(), "remote refused") {
		t.Errorf("fail stream errors = %v", errs)
	}
}

func TestRemoteAgentUnreachable(t *testing.T) {
	// Grab a free port and close it so nothing listens there.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	if err := lis.Close(); err != nil {
		t.Fatal(err)
	}

	// Connection failure is not fatal; a default name is set.
	a, err := NewAgent(WithURL(addr))
	if err != nil {
		t.Fatal(err)
	}
	if a.GetName() != "Remote-Agent" {
		t.Errorf("name = %q", a.GetName())
	}

	if _, err := NewAgent(WithURL("")); err == nil || !strings.Contains(err.Error(), "URL is required") {
		t.Errorf("empty URL error = %v", err)
	}
}

func TestRemoteHelpersWithoutClient(t *testing.T) {
	a := &Agent{isRemote: true, orgID: "o"}
	ctx := context.Background()
	want := "remote client not initialized"

	if _, err := a.runRemoteWithTracking(ctx, "x"); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("runRemoteWithTracking error = %v", err)
	}
	if _, err := a.runRemoteWithAuthTracking(ctx, "x", "t"); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("runRemoteWithAuthTracking error = %v", err)
	}
	if _, err := a.runRemoteStreamWithAuth(ctx, "x", "t"); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("runRemoteStreamWithAuth error = %v", err)
	}
	if _, err := a.runRemoteStream(ctx, "x"); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("runRemoteStream error = %v", err)
	}
	if _, err := a.Run(ctx, "x"); err == nil {
		t.Error("expected Run to fail without a client")
	}
	if _, err := a.RunWithAuth(ctx, "x", "t"); err == nil {
		t.Error("expected RunWithAuth to fail without a client")
	}
	if _, err := a.GetRemoteMetadata(); err == nil {
		t.Error("expected GetRemoteMetadata to fail without a client")
	}
	if err := a.Disconnect(); err != nil {
		t.Errorf("Disconnect() without client = %v", err)
	}

	local := &Agent{}
	if _, err := local.GetRemoteMetadata(); err == nil || !strings.Contains(err.Error(), "not a remote agent") {
		t.Errorf("local GetRemoteMetadata error = %v", err)
	}
}

func TestToolFactoryAgentTool(t *testing.T) {
	addr, _ := startFakeAgentService(t)
	tf := NewToolFactory()

	tool, err := tf.CreateToolWithParentConfig(
		ToolConfigYAML{Type: "agent", Name: "helper", Description: "remote helper", URL: addr, Timeout: "5s"},
		&AgentConfig{RequirePlanApproval: ptr(false)},
	)
	if err != nil {
		t.Fatal(err)
	}
	wrapper, ok := tool.(*AgentToolWrapper)
	if !ok {
		t.Fatalf("tool type = %T", tool)
	}
	t.Cleanup(func() { _ = wrapper.agent.Disconnect() })
	if wrapper.agent.requirePlanApproval {
		t.Error("plan approval should be inherited from the parent config")
	}
	got, err := tool.Execute(context.Background(), "hi")
	if err != nil || got != "remote:hi" {
		t.Fatalf("Execute() = %q, %v", got, err)
	}
}
