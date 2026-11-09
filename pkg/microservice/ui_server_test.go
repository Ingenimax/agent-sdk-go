package microservice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// uiConfig returns a config with every UI feature on.
func uiConfig() *UIConfig {
	return &UIConfig{
		Enabled:     true,
		DefaultPath: "/",
		Theme:       "dark",
		Features:    UIFeatures{Chat: true, Memory: true, AgentInfo: true, Settings: true, Traces: true},
	}
}

// serveUI exposes the UI server's API routes on a test server.
func serveUI(t *testing.T, s *HTTPServerWithUI) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	s.registerAPIEndpoints(mux)
	ts := httptest.NewServer(s.addCORS(mux))
	t.Cleanup(ts.Close)
	return ts
}

// doJSON sends a request and decodes a JSON response into out.
func doJSON(t *testing.T, method, url, body string, out any) int {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
	}
	return resp.StatusCode
}

// namedTool is a tool with a fixed name and description.
func namedTool(name, desc string) interfaces.Tool {
	return &MockTestTool{name: name, description: desc}
}

// fakeDataStore only needs a type name for detection.
type fakeDataStore struct{ interfaces.DataStore }

func TestNewHTTPServerWithUI_Defaults(t *testing.T) {
	s := NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 1234, nil)
	if !s.uiConfig.Enabled || s.uiConfig.Theme != "light" || s.uiConfig.Features.Traces {
		t.Errorf("default config = %+v", s.uiConfig)
	}
	if s.uiFS == nil {
		t.Error("embedded UI filesystem missing")
	}
	if s.GetTraceCollector() != nil {
		t.Error("trace collector created with traces disabled")
	}
	if s.port != 1234 {
		t.Errorf("port = %d", s.port)
	}
}

func TestNewHTTPServerWithUI_TraceCollector(t *testing.T) {
	existing := NewUITraceCollector(nil, nil, nil)
	tests := []struct {
		name        string
		tracer      interfaces.Tracer
		wantSame    bool
		wantWrapped bool
	}{
		{name: "no tracer"},
		{name: "agent already uses a collector", tracer: existing, wantSame: true},
		{name: "foreign tracer", tracer: &noopTracer{}, wantWrapped: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []agent.Option
			if tt.tracer != nil {
				opts = append(opts, agent.WithTracer(tt.tracer))
			}
			cfg := uiConfig()
			s := NewHTTPServerWithUI(newCustomAgent(t, nil, nil, opts...), 0, cfg)
			c := s.GetTraceCollector()
			if c == nil {
				t.Fatal("trace collector not created")
			}
			if cfg.Tracing == nil || cfg.Tracing.RetentionCount != 100 {
				t.Errorf("default tracing config = %+v", cfg.Tracing)
			}
			if tt.wantSame && c != existing {
				t.Error("existing collector was not reused")
			}
			if tt.wantWrapped && c.wrappedTracer != tt.tracer {
				t.Error("foreign tracer was not wrapped")
			}
		})
	}
}

// noopTracer is a tracer that is not a UITraceCollector.
type noopTracer struct{}

func (*noopTracer) StartSpan(ctx context.Context, _ string) (context.Context, interfaces.Span) {
	return ctx, &noOpSpan{}
}

func (*noopTracer) StartTraceSession(ctx context.Context, _ string) (context.Context, interfaces.Span) {
	return ctx, &noOpSpan{}
}

func TestUIServer_ConfigEndpoint(t *testing.T) {
	sub := newCustomAgent(t, nil, nil, agent.WithName("researcher"), agent.WithLLM(&MockLLMWithGetModel{model: "sub-model"}), agent.WithTools(namedTool("lookup", "")))
	a := newCustomAgent(t, nil, nil,
		agent.WithLLM(&MockLLMWithGetModel{model: "gpt-x"}),
		agent.WithSystemPrompt("be nice"),
		agent.WithAgents(sub),
		agent.WithTools(namedTool("writer_agent", "writes things"), namedTool("calc", "math")),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithDataStore(fakeDataStore{}),
	)
	ts := serveUI(t, NewHTTPServerWithUI(a, 0, uiConfig()))

	var cfg AgentConfigResponse
	if code := doJSON(t, http.MethodGet, ts.URL+"/api/v1/agent/config", "", &cfg); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if cfg.Name != "custom-agent" || cfg.Model != "gpt-x" || cfg.SystemPrompt != "be nice" || cfg.UITheme != "dark" {
		t.Errorf("config = %+v", cfg)
	}
	if cfg.Memory.Type != "buffer" || cfg.Memory.Status != "active" {
		t.Errorf("memory = %+v", cfg.Memory)
	}
	if cfg.DataStore != (DataStoreInfo{Type: "database", Status: "active"}) {
		t.Errorf("datastore = %+v", cfg.DataStore)
	}

	byID := map[string]SubAgentInfo{}
	for _, s := range cfg.SubAgents {
		byID[s.ID] = s
	}
	if len(cfg.SubAgents) != 2 {
		t.Fatalf("sub agents = %+v", cfg.SubAgents)
	}
	if r := byID["researcher"]; r.Model != "sub-model" || len(r.Tools) != 1 || r.Tools[0] != "lookup" {
		t.Errorf("researcher = %+v", r)
	}
	if w := byID["writer_agent"]; w.Name != "writer" || w.Description != "writes things" || w.Model != "Unknown" {
		t.Errorf("writer = %+v", w)
	}

	var subs map[string]any
	if code := doJSON(t, http.MethodGet, ts.URL+"/api/v1/agent/subagents", "", &subs); code != http.StatusOK || subs["count"] != float64(2) {
		t.Errorf("subagents = %d %v", code, subs)
	}

	var tools struct {
		Tools []map[string]any `json:"tools"`
		Count int              `json:"count"`
	}
	if code := doJSON(t, http.MethodGet, ts.URL+"/api/v1/tools", "", &tools); code != http.StatusOK {
		t.Fatalf("tools status = %d", code)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool["name"].(string)] = true
	}
	if !names["calc"] || !names["writer_agent"] || !names["researcher_agent"] || tools.Count != len(tools.Tools) {
		t.Errorf("tools = %+v", tools)
	}
}

func TestUIServer_MethodChecks(t *testing.T) {
	ts := serveUI(t, NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 0, uiConfig()))
	tests := []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/api/v1/agent/config", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/agent/subagents", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/agent/delegate", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/memory", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/memory/search?q=x", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/memory/search", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/tools", http.StatusMethodNotAllowed},
		{http.MethodGet, "/ws/chat", http.StatusNotImplemented},
		{http.MethodGet, "/api/v1/agent/run", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/agent/stream", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/traces", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/traces/stats", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/traces/", http.StatusBadRequest},
		{http.MethodPut, "/api/v1/traces/abc", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/traces/missing", http.StatusNotFound},
		{http.MethodDelete, "/api/v1/traces/missing", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			if got := doJSON(t, tt.method, ts.URL+tt.path, "", nil); got != tt.want {
				t.Errorf("status = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestUIServer_RoutesHiddenWhenUIDisabled(t *testing.T) {
	cfg := uiConfig()
	cfg.Enabled = false
	ts := serveUI(t, NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 0, cfg))
	if got := doJSON(t, http.MethodGet, ts.URL+"/api/v1/agent/config", "", nil); got != http.StatusNotFound {
		t.Errorf("config status = %d, want 404", got)
	}
	if got := doJSON(t, http.MethodGet, ts.URL+"/health", "", nil); got != http.StatusOK {
		t.Errorf("health status = %d", got)
	}
}

func TestUIServer_Delegate(t *testing.T) {
	ts := serveUI(t, NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 0, uiConfig()))
	if got := doJSON(t, http.MethodPost, ts.URL+"/api/v1/agent/delegate", "{", nil); got != http.StatusBadRequest {
		t.Errorf("bad JSON status = %d", got)
	}
	var resp map[string]any
	code := doJSON(t, http.MethodPost, ts.URL+"/api/v1/agent/delegate", `{"sub_agent_id":"s1","task":"do","conversation_id":"c"}`, &resp)
	if code != http.StatusOK || resp["status"] != "delegated" || resp["sub_agent_id"] != "s1" || resp["task"] != "do" {
		t.Errorf("delegate = %d %v", code, resp)
	}
}

func TestUIServer_RunRecordsLocalHistory(t *testing.T) {
	var gotOrg string
	a := newCustomAgent(t, func(ctx context.Context, input string, _ *agent.Agent) (string, error) {
		gotOrg, _ = multitenancy.GetOrgID(ctx)
		if input == "fail" {
			return "", errors.New("run failed")
		}
		return "echo " + input, nil
	}, nil)
	s := NewHTTPServerWithUI(a, 0, uiConfig())
	ts := serveUI(t, s)

	var ok map[string]any
	if code := doJSON(t, http.MethodPost, ts.URL+"/api/v1/agent/run", `{"input":"hello","conversation_id":"c1"}`, &ok); code != http.StatusOK {
		t.Fatalf("run status = %d", code)
	}
	if ok["output"] != "echo hello" || ok["error"] != "" {
		t.Errorf("run = %v", ok)
	}
	// withOrgContext fills in a default org when the request has none.
	if gotOrg != "default-org" {
		t.Errorf("org = %q, want default-org", gotOrg)
	}

	var failed map[string]any
	doJSON(t, http.MethodPost, ts.URL+"/api/v1/agent/run", `{"input":"fail","org_id":"o2","conversation_id":"c2"}`, &failed)
	if failed["error"] != "run failed" || gotOrg != "o2" {
		t.Errorf("failed run = %v org=%q", failed, gotOrg)
	}

	for _, tt := range []struct{ body string }{{"{"}, {`{"input":""}`}} {
		if code := doJSON(t, http.MethodPost, ts.URL+"/api/v1/agent/run", tt.body, nil); code != http.StatusBadRequest {
			t.Errorf("body %q status = %d", tt.body, code)
		}
	}

	// Each run records the user input and the reply or error.
	var roles []string
	for _, e := range s.conversationHistory {
		roles = append(roles, e.Role+":"+e.Metadata["conversation_id"].(string))
	}
	if strings.Join(roles, ",") != "user:c1,assistant:c1,user:c2,error:c2" {
		t.Errorf("history = %v", roles)
	}

	var search struct {
		Results []MemoryEntry `json:"results"`
		Count   int           `json:"count"`
	}
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory/search?q=ECHO", "", &search)
	if search.Count != 1 || search.Results[0].Content != "echo hello" {
		t.Errorf("search = %+v", search)
	}
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory/search?q=user", "", &search)
	if search.Count != 2 || search.Results[0].Content != "fail" {
		t.Errorf("role search = %+v", search)
	}
}

func TestSearchConversationHistory_EmptyQuery(t *testing.T) {
	s := NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 0, uiConfig())
	for _, c := range []string{"a", "b", "c"} {
		s.addToConversationHistory("user", c, nil)
	}
	got := s.searchConversationHistory(context.Background(), "")
	if len(got) != 3 || got[0].Content != "c" || got[2].Content != "a" {
		t.Errorf("results = %+v", got)
	}
}

func TestGetConversationHistory_LocalPagination(t *testing.T) {
	s := NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 0, uiConfig())
	for _, c := range []string{"1", "2", "3", "4", "5"} {
		s.addToConversationHistory("user", c, nil)
	}
	tests := []struct {
		limit, offset int
		want          string
	}{
		{limit: 2, offset: 0, want: "5,4"},
		{limit: 2, offset: 3, want: "2,1"},
		{limit: 10, offset: 4, want: "1"},
		{limit: 2, offset: 5, want: ""},
	}
	for _, tt := range tests {
		got, durable := s.getConversationHistory(context.Background(), tt.limit, tt.offset)
		var contents []string
		for _, e := range got {
			contents = append(contents, e.Content)
		}
		if strings.Join(contents, ",") != tt.want || durable {
			t.Errorf("limit=%d offset=%d: got %v durable=%v, want %s", tt.limit, tt.offset, contents, durable, tt.want)
		}
	}
}

func TestAddToConversationHistory_CapsAtOneThousand(t *testing.T) {
	s := NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 0, uiConfig())
	for range 1005 {
		s.addToConversationHistory("user", "x", nil)
	}
	if len(s.conversationHistory) != 1000 {
		t.Errorf("history length = %d", len(s.conversationHistory))
	}
}

func TestUIServer_Stream(t *testing.T) {
	ts := time.UnixMilli(1700000000000)
	a := newCustomAgent(t, nil, replayStream(
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventContent, Content: "Hel", Timestamp: ts},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventThinking, ThinkingStep: "think", Content: "ignored", Timestamp: ts},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventToolCall, ToolCall: &interfaces.ToolCallEvent{ID: "1", Name: "n"}, Metadata: map[string]any{"m": 1}, Timestamp: ts},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventError, Error: errors.New("oops"), Timestamp: ts},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventContent, Content: "lo", Timestamp: ts},
		interfaces.AgentStreamEvent{Type: interfaces.AgentEventComplete, Timestamp: ts},
	))
	s := NewHTTPServerWithUI(a, 0, uiConfig())
	srv := serveUI(t, s)

	resp, err := http.Post(srv.URL+"/api/v1/agent/stream", "application/json", strings.NewReader(`{"input":"hi","org_id":"o","conversation_id":"c"}`))
	if err != nil {
		t.Fatalf("POST stream: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	events := parseSSE(string(body))
	var names []string
	for _, e := range events {
		names = append(names, e.event)
	}
	if strings.Join(names, ",") != "content,thinking,tool_call,error,content,complete" {
		t.Fatalf("events = %v", names)
	}
	var tool, final StreamEventData
	_ = json.Unmarshal([]byte(events[2].data), &tool)
	_ = json.Unmarshal([]byte(events[5].data), &final)
	if tool.ToolCall == nil || tool.ToolCall.Name != "n" || tool.Metadata["m"] != float64(1) {
		t.Errorf("tool event = %+v", tool)
	}
	if !final.IsFinal {
		t.Error("complete event is not final")
	}
	if !strings.Contains(events[3].data, `"error":"oops"`) {
		t.Errorf("error event = %s", events[3].data)
	}

	// Only content events are joined into the stored reply.
	if n := len(s.conversationHistory); n != 2 || s.conversationHistory[1].Content != "Hello" {
		t.Errorf("history = %+v", s.conversationHistory)
	}
}

func TestUIServer_StreamErrors(t *testing.T) {
	a := newCustomAgent(t, nil, func(context.Context, string, *agent.Agent) (<-chan interfaces.AgentStreamEvent, error) {
		return nil, errors.New("stream refused")
	})
	s := NewHTTPServerWithUI(a, 0, uiConfig())
	srv := serveUI(t, s)

	for _, body := range []string{"{", `{"input":""}`} {
		resp, err := http.Post(srv.URL+"/api/v1/agent/stream", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %q status = %d", body, resp.StatusCode)
		}
	}

	resp, err := http.Post(srv.URL+"/api/v1/agent/stream", "application/json", strings.NewReader(`{"input":"x"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	events := parseSSE(string(raw))
	if len(events) != 1 || events[0].event != "error" || !strings.Contains(events[0].data, "stream refused") {
		t.Errorf("events = %+v", events)
	}
	if last := s.conversationHistory[len(s.conversationHistory)-1]; last.Role != "error" {
		t.Errorf("last history entry = %+v", last)
	}
}

func TestUIServer_SendSSEEventSkipsUnmarshalableData(t *testing.T) {
	s := NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 0, uiConfig())
	w := httptest.NewRecorder()
	s.sendSSEEvent(w, SSEEvent{Event: "bad", Data: make(chan int)})
	if w.Body.Len() != 0 {
		t.Errorf("wrote %q for unmarshalable data", w.Body.String())
	}
	s.sendSSEEvent(w, SSEEvent{Event: "ok", Data: 1, ID: "7"})
	if w.Body.String() != "event: ok\ndata: 1\nid: 7\n\n" {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestUIServer_AdminMemory(t *testing.T) {
	buf := memory.NewConversationBuffer()
	add := func(org, conv, content string) {
		ctx := memory.WithConversationID(multitenancy.WithOrgID(context.Background(), org), conv)
		if err := buf.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: content}); err != nil {
			t.Fatalf("AddMessage: %v", err)
		}
	}
	add("org-a", "conv-1", "first")
	add("org-a", "conv-1", strings.Repeat("x", 150))
	add("org-b", "conv-2", "other org")
	_ = buf.AddMessage(memory.WithConversationID(multitenancy.WithOrgID(context.Background(), "org-b"), "conv-2"),
		interfaces.Message{Role: interfaces.MessageRoleAssistant, Content: "tools", ToolCalls: []interfaces.ToolCall{{ID: "t"}}})

	ts := serveUI(t, NewHTTPServerWithUI(newCustomAgent(t, nil, nil, agent.WithMemory(buf)), 0, uiConfig()))

	var convs MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory", "", &convs)
	if convs.Source != memorySourceDurable || convs.Total != 2 {
		t.Fatalf("conversations = %+v", convs)
	}
	for _, c := range convs.Conversations {
		switch c.ID {
		case "conv-1":
			if c.MessageCount != 2 || !strings.HasPrefix(c.LastMessage, "[org-a] conv-1: ") || !strings.HasSuffix(c.LastMessage, "...") {
				t.Errorf("conv-1 = %+v", c)
			}
		case "conv-2":
			if c.MessageCount != 2 {
				t.Errorf("conv-2 = %+v", c)
			}
		default:
			t.Errorf("unexpected conversation %q", c.ID)
		}
	}

	var page MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?limit=1&offset=1", "", &page)
	if len(page.Conversations) != 1 || page.Total != 2 {
		t.Errorf("page = %+v", page)
	}
	var pastPage MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?offset=5", "", &pastPage)
	if len(pastPage.Conversations) != 0 {
		t.Errorf("past end = %+v", pastPage)
	}

	var msgs MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?conversation_id=conv-2", "", &msgs)
	if msgs.Source != memorySourceDurable || msgs.Total != 2 || len(msgs.Messages) != 2 {
		t.Fatalf("messages = %+v", msgs)
	}
	if msgs.Messages[0].Content != "[org-b] other org" || msgs.Messages[1].Content != "[org-b] tools [1 tool calls]" {
		t.Errorf("contents = %q, %q", msgs.Messages[0].Content, msgs.Messages[1].Content)
	}
	var pastMsgs MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?conversation_id=conv-2&offset=9", "", &pastMsgs)
	if len(pastMsgs.Messages) != 0 || pastMsgs.Total != 2 {
		t.Errorf("messages past end = %+v", pastMsgs)
	}

	var missing MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?conversation_id=nope", "", &missing)
	if missing.Total != 0 || missing.ConversationID != "nope" {
		t.Errorf("missing conversation = %+v", missing)
	}
}

// brokenAdminMemory fails every cross-org call.
type brokenAdminMemory struct{ interfaces.Memory }

func (brokenAdminMemory) GetAllConversations(context.Context) ([]string, error) { return nil, nil }
func (brokenAdminMemory) GetConversationMessages(context.Context, string) ([]interfaces.Message, error) {
	return nil, nil
}
func (brokenAdminMemory) GetMemoryStatistics(context.Context) (int, int, error) { return 0, 0, nil }
func (brokenAdminMemory) GetAllConversationsAcrossOrgs() (map[string][]string, error) {
	return nil, errors.New("down")
}
func (brokenAdminMemory) GetConversationMessagesAcrossOrgs(string) ([]interfaces.Message, string, error) {
	return nil, "", errors.New("down")
}
func (brokenAdminMemory) GetMemoryStatisticsAcrossOrgs() (int, int, error) {
	return 0, 0, errors.New("down")
}

func TestUIServer_AdminMemoryErrors(t *testing.T) {
	s := NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 0, uiConfig())
	mem := brokenAdminMemory{}

	convs := s.buildConversationListFromAllOrgs(mem, 10, 2)
	if convs.Mode != "conversations" || len(convs.Conversations) != 0 || convs.Limit != 10 || convs.Offset != 2 {
		t.Errorf("conversations = %+v", convs)
	}
	msgs := s.buildMessageListFromAllOrgs(mem, "c", 10, 0)
	if msgs.Mode != "messages" || len(msgs.Messages) != 0 || msgs.ConversationID != "c" {
		t.Errorf("messages = %+v", msgs)
	}
}

func TestDetectMemoryType(t *testing.T) {
	s := &HTTPServerWithUI{}
	tests := []struct {
		name string
		mem  interfaces.Memory
		want string
	}{
		{name: "closer means redis", mem: closerMemory{}, want: "redis"},
		{name: "buffer", mem: memory.NewConversationBuffer(), want: "buffer"},
		{name: "admin memory", mem: brokenAdminMemory{}, want: "conversation"},
		{name: "plain", mem: &scopeRecordingMemory{}, want: "memory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.detectMemoryType(tt.mem); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// closerMemory has a Close method, which the UI treats as Redis.
type closerMemory struct{ interfaces.Memory }

func (closerMemory) Close() error { return nil }

func TestInferAzureModelFromDeployment(t *testing.T) {
	tests := map[string]string{
		"prod-GPT-4o":          "gpt-4o",
		"gpt-4o-mini-east":     "gpt-4o-mini",
		"gpt4-turbo":           "gpt-4-turbo",
		"my-gpt-4":             "gpt-4",
		"gpt-35-turbo":         "gpt-3.5-turbo",
		"o1-preview-x":         "o1-preview",
		"o1-mini":              "o1-mini",
		"text-embedding-large": "text-embedding-ada-002",
		"dalle":                "dall-e-3",
		"llama":                "",
	}
	for in, want := range tests {
		if got := inferAzureModelFromDeployment(in); got != want {
			t.Errorf("inferAzureModelFromDeployment(%q) = %q, want %q", in, got, want)
		}
	}
}

// azureLLM reports itself as Azure OpenAI.
type azureLLM struct {
	MockLLM
	deployment string
}

func (a *azureLLM) Name() string     { return "azure-openai" }
func (a *azureLLM) GetModel() string { return a.deployment }

func TestGetModelName_Azure(t *testing.T) {
	tests := map[string]string{
		"prod-gpt-4o": "gpt-4o (deployment: prod-gpt-4o)",
		"custom":      "Azure OpenAI (deployment: custom)",
	}
	for deployment, want := range tests {
		s := &HTTPServerWithUI{HTTPServer: HTTPServer{agent: newCustomAgent(t, nil, nil, agent.WithLLM(&azureLLM{deployment: deployment}))}}
		if got := s.getModelName(); got != want {
			t.Errorf("deployment %q: got %q, want %q", deployment, got, want)
		}
	}
}

func TestExtractTimestampAndConversationID(t *testing.T) {
	s := &HTTPServerWithUI{}
	when := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name string
		meta map[string]any
		want int64
	}{
		{name: "milliseconds int64", meta: map[string]any{"timestamp": int64(1234)}, want: 1234},
		{name: "nanoseconds int64", meta: map[string]any{"timestamp": when.UnixNano()}, want: when.UnixMilli()},
		{name: "float64", meta: map[string]any{"timestamp": float64(99)}, want: 99},
		{name: "RFC3339 string", meta: map[string]any{"timestamp": when.Format(time.RFC3339)}, want: when.UnixMilli()},
	}
	for _, tt := range tests {
		if got := s.extractTimestamp(tt.meta); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
	before := time.Now().UnixMilli()
	for _, meta := range []map[string]any{nil, {"timestamp": "garbage"}} {
		if got := s.extractTimestamp(meta); got < before {
			t.Errorf("fallback timestamp %d is before now", got)
		}
	}

	ids := []struct {
		meta map[string]any
		want string
	}{
		{nil, "default"},
		{map[string]any{"conversation_id": "a"}, "a"},
		{map[string]any{"conversationId": "b"}, "b"},
		{map[string]any{"other": 1}, "default"},
	}
	for _, tt := range ids {
		if got := s.extractConversationID(tt.meta); got != tt.want {
			t.Errorf("extractConversationID(%v) = %q, want %q", tt.meta, got, tt.want)
		}
	}
}

func TestSystemPromptParsing(t *testing.T) {
	s := &HTTPServerWithUI{}
	prompt := "### research_agent\n- **Purpose**: finds facts\n- Use `research_agent` or (writer_agent).\nplain line"
	tools := s.parseToolsFromSystemPrompt(prompt)
	if strings.Join(tools, ",") != "research_agent,writer_agent" {
		t.Errorf("tools = %v", tools)
	}
	if got := s.getToolDescriptionFromSystemPrompt("research_agent", prompt); got != "finds facts" {
		t.Errorf("research description = %q", got)
	}
	if got := s.getToolDescriptionFromSystemPrompt("writer_agent", prompt); got != "writer sub-agent" {
		t.Errorf("writer description = %q", got)
	}
	if got := s.getToolDescriptionFromSystemPrompt("absent_agent", prompt); got != "Sub-agent tool" {
		t.Errorf("absent description = %q", got)
	}
}

func TestGetSystemPrompt_Empty(t *testing.T) {
	s := &HTTPServerWithUI{HTTPServer: HTTPServer{agent: newCustomAgent(t, nil, nil)}}
	if got := s.getSystemPrompt(); got != "No system prompt configured" {
		t.Errorf("got %q", got)
	}
}

func TestGetSubAgentModel(t *testing.T) {
	s := &HTTPServerWithUI{}
	tests := []struct {
		name string
		llm  interfaces.LLM
		want string
	}{
		{name: "empty model uses name", llm: &MockLLMWithEmptyModel{}, want: "mock-llm"},
		{name: "empty name", llm: &MockLLMWithEmptyName{}, want: "Unknown"},
	}
	for _, tt := range tests {
		sub := newCustomAgent(t, nil, nil, agent.WithLLM(tt.llm))
		if got := s.getSubAgentModel(sub); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
	remote := newRemoteAgent(t, startGRPCAgent(t, newCustomAgent(t, nil, nil)))
	if got := s.getSubAgentModel(remote); got != "Remote Agent" {
		t.Errorf("remote: got %q", got)
	}
}

func TestUIServer_Traces(t *testing.T) {
	s := NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 0, uiConfig())
	c := s.GetTraceCollector()
	ctx, root := c.StartTraceSession(context.Background(), "session")
	_, child := c.StartSpan(ctx, "llm.generate")
	child.End()
	root.End()
	ts := serveUI(t, s)

	var list struct {
		Traces []UITrace `json:"traces"`
		Total  int       `json:"total"`
		Limit  int       `json:"limit"`
		Offset int       `json:"offset"`
	}
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/traces?limit=x&offset=-2", "", &list)
	if list.Total != 1 || len(list.Traces) != 1 || list.Limit != 50 || list.Offset != 0 {
		t.Fatalf("list = %+v", list)
	}
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/traces?limit=5&offset=1", "", &list)
	if list.Limit != 5 || list.Offset != 1 || len(list.Traces) != 0 {
		t.Errorf("paged list = %+v", list)
	}

	id := c.traces[firstKey(c)].ID
	var trace UITrace
	if code := doJSON(t, http.MethodGet, ts.URL+"/api/v1/traces/"+id, "", &trace); code != http.StatusOK || trace.ID != id {
		t.Errorf("get trace = %d %+v", code, trace)
	}

	var stats map[string]any
	if code := doJSON(t, http.MethodGet, ts.URL+"/api/v1/traces/stats", "", &stats); code != http.StatusOK || stats["total_traces"] != float64(1) {
		t.Errorf("stats = %d %v", code, stats)
	}

	var deleted map[string]string
	if code := doJSON(t, http.MethodDelete, ts.URL+"/api/v1/traces/"+id, "", &deleted); code != http.StatusOK || deleted["status"] != "deleted" {
		t.Errorf("delete = %d %v", code, deleted)
	}
	if code := doJSON(t, http.MethodGet, ts.URL+"/api/v1/traces/"+id, "", nil); code != http.StatusNotFound {
		t.Errorf("get after delete = %d", code)
	}
}

func TestHandleTrace_InvalidPath(t *testing.T) {
	s := NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 0, uiConfig())
	w := httptest.NewRecorder()
	s.handleTrace(w, httptest.NewRequest(http.MethodGet, "/elsewhere", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d", w.Code)
	}
}

// firstKey returns any trace ID held by c.
func firstKey(c *UITraceCollector) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for k := range c.traces {
		return k
	}
	return ""
}

func TestUIServer_RemoteAgent(t *testing.T) {
	backend := newCustomAgent(t, nil, nil,
		agent.WithLLM(&MockLLMWithGetModel{model: "remote-model"}),
		agent.WithSystemPrompt("Delegate with research_agent.\nPurpose: research"),
		agent.WithMemory(memory.NewConversationBuffer()),
	)
	remote := newRemoteAgent(t, startGRPCAgent(t, backend))
	s := NewHTTPServerWithUI(remote, 0, uiConfig())
	ts := serveUI(t, s)

	var cfg AgentConfigResponse
	if code := doJSON(t, http.MethodGet, ts.URL+"/api/v1/agent/config", "", &cfg); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if cfg.Model != "remote-model" || !strings.HasPrefix(cfg.SystemPrompt, "Delegate with") {
		t.Errorf("config = %+v", cfg)
	}
	if cfg.Memory != (MemoryInfo{Type: "conversation", Status: "active"}) {
		t.Errorf("memory = %+v", cfg.Memory)
	}
	if cfg.DataStore != (DataStoreInfo{Type: "none", Status: "inactive"}) {
		t.Errorf("datastore = %+v", cfg.DataStore)
	}
	if len(cfg.SubAgents) != 1 || cfg.SubAgents[0].ID != "research_agent" || cfg.SubAgents[0].Model != "Remote" {
		t.Errorf("sub agents = %+v", cfg.SubAgents)
	}

	var tools struct {
		Tools []map[string]any `json:"tools"`
	}
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/tools", "", &tools)
	if len(tools.Tools) != 1 || tools.Tools[0]["name"] != "research_agent" || tools.Tools[0]["description"] != "Remote agent tool" {
		t.Errorf("tools = %+v", tools.Tools)
	}

	// The remote URL is a bare gRPC address, so the HTTP memory fetch fails
	// and the handler answers with an empty durable response.
	var convs MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?limit=3", "", &convs)
	if convs.Source != memorySourceDurable || convs.Total != 0 || convs.Limit != 3 {
		t.Errorf("conversations = %+v", convs)
	}
	var msgs MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?conversation_id=c", "", &msgs)
	if msgs.Source != memorySourceDurable || msgs.ConversationID != "c" || msgs.Total != 0 {
		t.Errorf("messages = %+v", msgs)
	}
}

func TestUIServer_RemoteMetadataFallbacks(t *testing.T) {
	// A backend with no LLM model, memory or prompt exercises the
	// "unavailable" branches of the remote metadata readers.
	backend := newCustomAgent(t, nil, nil, agent.WithLLM(&MockLLMWithEmptyName{}))
	remote := newRemoteAgent(t, startGRPCAgent(t, backend))
	s := &HTTPServerWithUI{HTTPServer: HTTPServer{agent: remote}}

	if got := s.getModelName(); got != "Remote agent - metadata unavailable" {
		t.Errorf("model = %q", got)
	}
	if got := s.getSystemPrompt(); got != "Remote agent - system prompt unavailable" {
		t.Errorf("system prompt = %q", got)
	}
	if got := s.getMemoryInfo(context.Background()); got != (MemoryInfo{Type: "none", Status: "inactive"}) {
		t.Errorf("memory = %+v", got)
	}
}

func TestUIServer_RemoteModelFromLLMName(t *testing.T) {
	backend := newCustomAgent(t, nil, nil, agent.WithLLM(&MockLLMWithEmptyModel{}))
	remote := newRemoteAgent(t, startGRPCAgent(t, backend))
	s := &HTTPServerWithUI{HTTPServer: HTTPServer{agent: remote}}
	// The gRPC server reports the LLM name as the model when GetModel is empty.
	if got := s.getModelName(); got != "mock-llm" {
		t.Errorf("model = %q", got)
	}
}

func TestRemoteMemory_NoURL(t *testing.T) {
	s := &HTTPServerWithUI{HTTPServer: HTTPServer{agent: newCustomAgent(t, nil, nil)}}
	if r := s.getRemoteMemoryConversations(5, 1); r.Mode != "conversations" || r.Limit != 5 || r.Offset != 1 {
		t.Errorf("conversations = %+v", r)
	}
	if r := s.getRemoteMemoryMessages("c", 5, 1); r.Mode != "messages" || r.ConversationID != "c" {
		t.Errorf("messages = %+v", r)
	}
}

func TestHTTPServerWithUI_StartServesUI(t *testing.T) {
	s := NewHTTPServerWithUI(newCustomAgent(t, nil, nil), freePort(t), uiConfig())
	done := make(chan error, 1)
	go func() { done <- s.Start() }()
	url := baseURL(s.port)
	waitForHTTP(t, url+"/health")
	t.Cleanup(func() {
		_ = s.Stop(context.Background())
		<-done
	})

	get := func(path string) (int, string) {
		resp, err := http.Get(url + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	code, index := get("/")
	if code != http.StatusOK || !strings.Contains(strings.ToLower(index), "<html") {
		t.Errorf("index = %d", code)
	}
	if code, body := get("/some/spa/route"); code != http.StatusOK || body != index {
		t.Errorf("SPA fallback = %d, same as index: %v", code, body == index)
	}
	if code, _ := get("/favicon.ico"); code != http.StatusOK {
		t.Errorf("favicon = %d", code)
	}
	code, files := get("/debug/files")
	if code != http.StatusOK || !strings.Contains(files, "index.html") {
		t.Errorf("debug files = %d %s", code, files)
	}
	if code, _ := get("/api/v1/traces/stats"); code != http.StatusOK {
		t.Errorf("trace stats = %d", code)
	}
}

func TestUIServer_LocalMemoryListing(t *testing.T) {
	s := NewHTTPServerWithUI(newCustomAgent(t, nil, nil), 0, uiConfig())
	s.conversationHistory = []MemoryEntry{
		{Role: "user", Content: "a1", ConversationID: "c1", Timestamp: 1},
		{Role: "assistant", Content: strings.Repeat("y", 120), ConversationID: "c1", Timestamp: 2},
		{Role: "user", Content: "b1", ConversationID: "c2", Timestamp: 3},
		{Role: "user", Content: "loose", Timestamp: 4},
	}
	ts := serveUI(t, s)

	var convs MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?limit=bad&offset=-1", "", &convs)
	if convs.Mode != "conversations" || convs.Source != memorySourceLocal || convs.Total != 3 || convs.Limit != 100 || convs.Offset != 0 {
		t.Fatalf("conversations = %+v", convs)
	}
	byID := map[string]ConversationInfo{}
	for _, c := range convs.Conversations {
		byID[c.ID] = c
	}
	if c := byID["c1"]; c.MessageCount != 2 || c.LastActivity != 2 || len(c.LastMessage) != 103 {
		t.Errorf("c1 = %+v", c)
	}
	if c := byID["default"]; c.MessageCount != 1 || c.LastMessage != "loose" {
		t.Errorf("default = %+v", c)
	}

	var msgs MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?conversation_id=c1&limit=1&offset=1", "", &msgs)
	if msgs.Mode != "messages" || msgs.Total != 2 || len(msgs.Messages) != 1 || msgs.Messages[0].Role != "assistant" {
		t.Errorf("messages = %+v", msgs)
	}
	var loose MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?conversation_id=default", "", &loose)
	if loose.Total != 1 || loose.Messages[0].Content != "loose" {
		t.Errorf("default messages = %+v", loose)
	}

	var pastConvs MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?offset=10", "", &pastConvs)
	if len(pastConvs.Conversations) != 0 || pastConvs.Total != 3 {
		t.Errorf("past end = %+v", pastConvs)
	}
	var pastMsgs MemoryResponse
	doJSON(t, http.MethodGet, ts.URL+"/api/v1/memory?conversation_id=c1&offset=10", "", &pastMsgs)
	if len(pastMsgs.Messages) != 0 || pastMsgs.Total != 2 {
		t.Errorf("messages past end = %+v", pastMsgs)
	}
}
