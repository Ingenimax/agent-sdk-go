package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	smithybearer "github.com/aws/smithy-go/auth/bearer"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

// converseReply is one canned Converse response: a status, an optional error
// type header, and a JSON body.
type converseReply struct {
	status    int
	errorType string
	body      string
}

// converseServer is a fake Bedrock runtime endpoint. It answers each request
// with the next reply and records the request paths and bodies.
type converseServer struct {
	mu      sync.Mutex
	replies []converseReply
	paths   []string
	bodies  []map[string]any
}

func (s *converseServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)

	s.mu.Lock()
	s.paths = append(s.paths, r.URL.Path)
	s.bodies = append(s.bodies, body)
	reply := converseReply{status: http.StatusInternalServerError, body: `{"message":"no reply left"}`}
	if len(s.replies) > 0 {
		reply = s.replies[0]
		s.replies = s.replies[1:]
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if reply.errorType != "" {
		w.Header().Set("X-Amzn-ErrorType", reply.errorType)
	}
	w.WriteHeader(reply.status)
	_, _ = io.WriteString(w, reply.body)
}

// textReply is a Converse response with one assistant text block.
func textReply(text string) converseReply {
	return converseReply{status: http.StatusOK, body: `{"output":{"message":{"role":"assistant","content":[{"text":"` + text + `"}]}},"stopReason":"end_turn","usage":{"inputTokens":3,"outputTokens":4,"totalTokens":7}}`}
}

// newServerClient returns a client whose Converse calls go to a fake server
// that answers with replies, in order.
func newServerClient(t *testing.T, replies ...converseReply) (*Client, *converseServer) {
	t.Helper()
	fake := &converseServer{replies: replies}
	server := httptest.NewTLSServer(fake)
	t.Cleanup(server.Close)

	client, err := NewClient(context.Background(), "", WithModel("test-model"), WithAWSConfig(aws.Config{
		Region:                  "us-east-1",
		Credentials:             aws.AnonymousCredentials{},
		BearerAuthTokenProvider: smithybearer.StaticTokenProvider{Token: smithybearer.Token{Value: "test-key"}},
		AuthSchemePreference:    []string{"httpBearerAuth"},
		HTTPClient:              server.Client(),
		BaseEndpoint:            aws.String(server.URL),
		RetryMaxAttempts:        1,
	}))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client, fake
}

// errMemory is a memory whose GetMessages returns its messages for the first
// ok calls, then fails.
type errMemory struct {
	ok       int
	calls    *int
	messages []interfaces.Message
}

func (m errMemory) AddMessage(context.Context, interfaces.Message) error { return nil }
func (m errMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	*m.calls++
	if *m.calls > m.ok {
		return nil, errors.New("memory down")
	}
	return m.messages, nil
}
func (m errMemory) Clear(context.Context) error { return nil }

func failingMemory(ok int, messages ...interfaces.Message) errMemory {
	return errMemory{ok: ok, calls: new(int), messages: messages}
}

func TestNewClientNeedsAPIKey(t *testing.T) {
	if _, err := NewClient(context.Background(), ""); err == nil {
		t.Fatal("NewClient() with no key and no AWS config = nil error, want an error")
	}
}

func TestNewClientWithAWSConfigKeepsRegionAndModel(t *testing.T) {
	client, err := NewClient(context.Background(), "", WithModel("m"), WithRegion("eu-west-1"), WithLogger(logging.New()),
		WithAWSConfig(aws.Config{Credentials: aws.AnonymousCredentials{}}))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client.Region != "eu-west-1" || client.Model != "m" {
		t.Errorf("client region %q, model %q; want eu-west-1 and m", client.Region, client.Model)
	}
	if client.Name() != "bedrock" || client.SupportsStreaming() {
		t.Errorf("Name() = %q, SupportsStreaming() = %v; want bedrock and false", client.Name(), client.SupportsStreaming())
	}
}

func TestWithRegionIgnoresEmpty(t *testing.T) {
	opts := &clientOptions{region: DefaultRegion}
	WithRegion("")(opts)
	if opts.region != DefaultRegion {
		t.Errorf("region = %q, want %q", opts.region, DefaultRegion)
	}
}

// A part Converse cannot take must fail the call before any request. The
// test client has no Converse client, so a request would panic.
func TestGenerateRefusesUnsupportedContent(t *testing.T) {
	client := newTestClient()
	withZip := interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "application/zip"))

	if _, err := client.Generate(context.Background(), "x", withZip); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("Generate() error = %v, want ErrContentUnsupported", err)
	}
	if _, err := client.GenerateWithTools(context.Background(), "x", nil, withZip); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("GenerateWithTools() error = %v, want ErrContentUnsupported", err)
	}
}

func TestGenerateRefusesWhenMemoryFails(t *testing.T) {
	client := newTestClient()
	withMemory := interfaces.WithMemory(failingMemory(0))
	if _, err := client.Generate(context.Background(), "x", withMemory); err == nil || !strings.Contains(err.Error(), "memory down") {
		t.Errorf("Generate() error = %v, want the memory error", err)
	}
}

func TestGenerateDetailedSendsConverseRequest(t *testing.T) {
	client, fake := newServerClient(t, textReply("a cat"))
	parts := interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1, 2}, "image/png"), interfaces.NewPartFromText("describe"))
	system := func(o *interfaces.GenerateOptions) { o.SystemMessage = "be brief" }

	resp, err := client.GenerateDetailed(context.Background(), "ignored", parts, system)
	if err != nil {
		t.Fatalf("GenerateDetailed() error = %v", err)
	}
	if resp.Content != "a cat" || resp.StopReason != "end_turn" || resp.Model != "test-model" {
		t.Errorf("response = %+v, want a cat, end_turn, test-model", resp)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 7 || resp.Usage.InputTokens != 3 {
		t.Errorf("usage = %+v, want 3 in and 7 total", resp.Usage)
	}
	if len(fake.paths) != 1 || fake.paths[0] != "/model/test-model/converse" {
		t.Fatalf("paths = %v, want one Converse call", fake.paths)
	}

	body := fake.bodies[0]
	messages, _ := body["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages = %v, want one user turn", body["messages"])
	}
	content, _ := messages[0].(map[string]any)["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("content = %v, want the image and the text", content)
	}
	if _, ok := content[0].(map[string]any)["image"]; !ok {
		t.Errorf("block 0 = %v, want an image", content[0])
	}
	if text := content[1].(map[string]any)["text"]; text != "describe" {
		t.Errorf("block 1 text = %v, want describe", text)
	}
	system0, _ := body["system"].([]any)
	if len(system0) != 1 || system0[0].(map[string]any)["text"] != "be brief" {
		t.Errorf("system = %v, want be brief", body["system"])
	}
}

func TestGenerateReturnsText(t *testing.T) {
	client, _ := newServerClient(t, textReply("hello"))
	got, err := client.Generate(context.Background(), "hi")
	if err != nil || got != "hello" {
		t.Errorf("Generate() = %q, %v; want hello", got, err)
	}
}

func TestGenerateReturnsConverseError(t *testing.T) {
	client, _ := newServerClient(t, converseReply{status: http.StatusBadRequest, errorType: "ValidationException", body: `{"message":"bad model"}`})
	if _, err := client.Generate(context.Background(), "hi"); err == nil || !strings.Contains(err.Error(), "bad model") {
		t.Errorf("Generate() error = %v, want the Converse error", err)
	}
	if _, err := client.GenerateWithTools(context.Background(), "hi", nil); err == nil {
		t.Error("GenerateWithTools() error = nil, want the Converse error")
	}
}

func TestGenerateRetriesWithoutDeprecatedSamplingParams(t *testing.T) {
	client, fake := newServerClient(t,
		converseReply{status: http.StatusBadRequest, errorType: "ValidationException", body: "{\"message\":\"`temperature` is deprecated for this model.\"}"},
		textReply("ok"),
	)
	got, err := client.Generate(context.Background(), "hi")
	if err != nil || got != "ok" {
		t.Fatalf("Generate() = %q, %v; want ok", got, err)
	}
	if len(fake.bodies) != 2 {
		t.Fatalf("made %d calls, want 2", len(fake.bodies))
	}
	first, _ := fake.bodies[0]["inferenceConfig"].(map[string]any)
	second, _ := fake.bodies[1]["inferenceConfig"].(map[string]any)
	if _, ok := first["temperature"]; !ok {
		t.Errorf("first inferenceConfig = %v, want a temperature", first)
	}
	if _, ok := second["temperature"]; ok {
		t.Errorf("retry inferenceConfig = %v, want no temperature", second)
	}
}

func TestGenerateWithRetryExecutor(t *testing.T) {
	client, _ := newServerClient(t, textReply("ok"))
	opts := &clientOptions{}
	WithRetry(retry.WithInitialInterval(time.Millisecond))(opts)
	client.retryExecutor = opts.retry
	if got, err := client.Generate(context.Background(), "hi"); err != nil || got != "ok" {
		t.Errorf("Generate() = %q, %v; want ok", got, err)
	}
}

func TestGenerateEmptyOutputIsAnError(t *testing.T) {
	client, _ := newServerClient(t, converseReply{status: http.StatusOK, body: `{"stopReason":"end_turn"}`})
	if _, err := client.Generate(context.Background(), "hi"); err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Errorf("Generate() error = %v, want an empty response error", err)
	}
}

// fixedTool returns a fixed result or error and records its arguments.
type fixedTool struct {
	name   string
	result string
	err    error
	mu     sync.Mutex
	args   []string
}

func (f *fixedTool) Name() string        { return f.name }
func (f *fixedTool) Description() string { return "a tool" }
func (f *fixedTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"tags": {Type: "array", Description: "tags", Default: []string{"a"}, Enum: []interface{}{"a", "b"}, Items: &interfaces.ParameterSpec{Type: "string", Enum: []interface{}{"a"}}},
	}
}
func (f *fixedTool) Run(ctx context.Context, input string) (string, error) {
	return f.Execute(ctx, input)
}
func (f *fixedTool) Execute(_ context.Context, args string) (string, error) {
	f.mu.Lock()
	f.args = append(f.args, args)
	f.mu.Unlock()
	return f.result, f.err
}

func toolUseReply(name string) converseReply {
	return converseReply{status: http.StatusOK, body: `{"output":{"message":{"role":"assistant","content":[{"text":"calling"},{"toolUse":{"toolUseId":"t1","name":"` + name + `","input":{"city":"Paris"}}}]}},"stopReason":"tool_use","usage":{"inputTokens":1,"outputTokens":2,"totalTokens":3}}`}
}

func TestGenerateWithToolsDetailedRunsToolLoop(t *testing.T) {
	client, fake := newServerClient(t, toolUseReply("weather"), textReply("sunny"))
	tool := &fixedTool{name: "weather", result: "21C"}

	resp, err := client.GenerateWithToolsDetailed(context.Background(), "weather?", []interfaces.Tool{tool})
	if err != nil {
		t.Fatalf("GenerateWithToolsDetailed() error = %v", err)
	}
	if resp.Content != "sunny" || resp.Metadata["iterations"] != 2 {
		t.Errorf("response = %+v, want sunny after 2 iterations", resp)
	}
	if resp.Usage.InputTokens != 4 || resp.Usage.OutputTokens != 6 || resp.Usage.TotalTokens != 10 {
		t.Errorf("usage = %+v, want 4 in, 6 out, 10 total", resp.Usage)
	}
	if len(tool.args) != 1 || tool.args[0] != `{"city":"Paris"}` {
		t.Errorf("tool args = %v, want the city", tool.args)
	}

	if _, ok := fake.bodies[0]["toolConfig"]; !ok {
		t.Errorf("first request = %v, want a toolConfig", fake.bodies[0])
	}
	messages, _ := fake.bodies[1]["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("second request has %d messages, want user, assistant, tool result", len(messages))
	}
	last := messages[2].(map[string]any)
	content := last["content"].([]any)
	result := content[0].(map[string]any)["toolResult"].(map[string]any)
	if result["toolUseId"] != "t1" {
		t.Errorf("tool result = %v, want the result for t1", result)
	}
}

func TestGenerateWithToolsStopsAtMaxIterations(t *testing.T) {
	client, _ := newServerClient(t, toolUseReply("missing"))
	maxOne := func(o *interfaces.GenerateOptions) { o.MaxIterations = 1 }
	_, err := client.GenerateWithTools(context.Background(), "x", []interfaces.Tool{&fixedTool{name: "weather"}}, maxOne)
	if err == nil || !strings.Contains(err.Error(), "exceeded max tool iterations (1)") {
		t.Errorf("GenerateWithTools() error = %v, want a max iterations error", err)
	}
}

func TestGenerateWithToolsStopsWhenContextIsDone(t *testing.T) {
	client, fake := newServerClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.GenerateWithTools(ctx, "x", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("GenerateWithTools() error = %v, want context.Canceled", err)
	}
	if len(fake.paths) != 0 {
		t.Errorf("made %d requests, want none", len(fake.paths))
	}
}

func TestGenerateWithToolsEmptyOutputIsAnError(t *testing.T) {
	client, _ := newServerClient(t, converseReply{status: http.StatusOK, body: `{"stopReason":"end_turn"}`})
	if _, err := client.GenerateWithTools(context.Background(), "x", nil); err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Errorf("GenerateWithTools() error = %v, want an empty response error", err)
	}
}
