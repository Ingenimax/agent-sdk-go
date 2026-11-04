package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm"
)

// tokenServer fakes POST /v1/oauth/token and, for client tests, POST /v1/messages.
type tokenServer struct {
	*httptest.Server
	requests  atomic.Int32 // every exchange attempt
	minted    atomic.Int32 // successful exchanges; the Nth mints "token-N"
	fail      atomic.Bool  // reply 401 to exchanges
	expiresIn atomic.Int64
	tokenType atomic.Value  // string; empty means "Bearer"
	gate      chan struct{} // when set, exchanges block until it is closed

	mu            sync.Mutex
	lastExchange  tokenExchangeRequest
	lastMessages  http.Header
	messagesCalls int
}

func newTokenServer(t *testing.T) *tokenServer {
	t.Helper()
	s := &tokenServer{}
	s.expiresIn.Store(600)
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth/token":
			s.handleToken(t, w, r)
		case "/v1/messages":
			s.mu.Lock()
			s.lastMessages = r.Header.Clone()
			s.messagesCalls++
			s.mu.Unlock()
			_ = json.NewEncoder(w).Encode(CompletionResponse{
				ID: "msg_1", Type: "message", Role: "assistant", Model: "m", StopReason: "end_turn",
				Content: []ContentBlock{{Type: "text", Text: "hello"}},
			})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *tokenServer) handleToken(t *testing.T, w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)
	if r.Method != http.MethodPost {
		t.Errorf("token endpoint method = %s, want POST", r.Method)
	}
	if gate := s.gate; gate != nil {
		<-gate
	}
	if s.fail.Load() {
		w.Header().Set("Request-Id", "req_123")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"Authentication failed"}}`))
		return
	}
	s.mu.Lock()
	if err := json.NewDecoder(r.Body).Decode(&s.lastExchange); err != nil {
		t.Errorf("failed to decode token request: %v", err)
	}
	s.mu.Unlock()

	tokenType := "Bearer"
	if v, ok := s.tokenType.Load().(string); ok && v != "" {
		tokenType = v
	}
	_ = json.NewEncoder(w).Encode(tokenExchangeResponse{
		AccessToken: fmt.Sprintf("token-%d", s.minted.Add(1)),
		TokenType:   tokenType,
		ExpiresIn:   s.expiresIn.Load(),
	})
}

func (s *tokenServer) token(source *federationTokenSource) (string, error) {
	return source.Token(context.Background(), s.Client(), s.URL)
}

func newTestSource(provider IdentityTokenProvider, now *time.Time) *federationTokenSource {
	source := newFederationTokenSource(provider, FederationOptions{
		FederationRuleID: "fdrl_1",
		OrganizationID:   "org-1",
		ServiceAccountID: "svac_1",
		WorkspaceID:      "wrkspc_1",
	})
	if now != nil {
		source.now = func() time.Time { return *now }
	}
	return source
}

func TestFederationTokenExchangeRequest(t *testing.T) {
	server := newTokenServer(t)
	source := newTestSource(IdentityToken("jwt-1"), nil)

	token, err := server.token(source)
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token != "token-1" {
		t.Errorf("Token() = %q, want token-1", token)
	}

	want := tokenExchangeRequest{
		GrantType:        jwtBearerGrantType,
		Assertion:        "jwt-1",
		FederationRuleID: "fdrl_1",
		OrganizationID:   "org-1",
		ServiceAccountID: "svac_1",
		WorkspaceID:      "wrkspc_1",
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.lastExchange != want {
		t.Errorf("token request = %+v, want %+v", server.lastExchange, want)
	}
}

func TestFederationTokenRefresh(t *testing.T) {
	server := newTokenServer(t)
	start := time.Unix(1_000_000, 0)
	now := start
	source := newTestSource(IdentityToken("jwt"), &now)
	expiry := start.Add(600 * time.Second)

	mustToken := func(step, want string, wantRequests int32) {
		t.Helper()
		got, err := server.token(source)
		if err != nil || got != want {
			t.Fatalf("%s: Token() = %q, %v, want %q", step, got, err, want)
		}
		if n := server.requests.Load(); n != wantRequests {
			t.Fatalf("%s: exchange requests = %d, want %d", step, n, wantRequests)
		}
	}
	mustError := func(step string, wantRequests int32) {
		t.Helper()
		if _, err := server.token(source); err == nil {
			t.Fatalf("%s: Token() error = nil, want error", step)
		}
		if n := server.requests.Load(); n != wantRequests {
			t.Fatalf("%s: exchange requests = %d, want %d", step, n, wantRequests)
		}
	}

	mustToken("first", "token-1", 1)

	now = start.Add(5 * time.Minute)
	mustToken("cached", "token-1", 1)

	// Advisory window with a failing endpoint: cached token is served, and the
	// failure is not retried on every request.
	server.fail.Store(true)
	now = expiry.Add(-90 * time.Second)
	mustToken("advisory failure", "token-1", 2)
	for i := 0; i < 5; i++ {
		mustToken("advisory backoff", "token-1", 2)
	}
	now = now.Add(exchangeBackoff)
	mustToken("advisory retry after backoff", "token-1", 3)

	// Mandatory window with a failing endpoint: error, again without a retry storm.
	now = expiry.Add(-20 * time.Second)
	mustError("mandatory failure", 4)
	mustError("mandatory backoff", 4)

	// Endpoint recovers: a new token is minted once the backoff elapses.
	server.fail.Store(false)
	now = now.Add(exchangeBackoff)
	mustToken("refreshed", "token-2", 5)
	now = now.Add(time.Minute)
	mustToken("refreshed cached", "token-2", 5)
}

func TestFederationTokenConcurrentCallers(t *testing.T) {
	server := newTokenServer(t)
	server.gate = make(chan struct{})
	source := newTestSource(IdentityToken("jwt"), nil)

	const callers = 20
	results := make(chan string, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := server.token(source)
			if err != nil {
				t.Errorf("Token() error = %v", err)
			}
			results <- token
		}()
	}

	// Let every caller queue up behind the blocked exchange before releasing it.
	deadline := time.Now().Add(5 * time.Second)
	for server.requests.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(server.gate)
	wg.Wait()
	close(results)

	for token := range results {
		if token != "token-1" {
			t.Errorf("Token() = %q, want token-1", token)
		}
	}
	if n := server.requests.Load(); n != 1 {
		t.Errorf("exchange requests = %d, want 1", n)
	}
}

func TestFederationTokenWaitHonorsContext(t *testing.T) {
	server := newTokenServer(t)
	server.gate = make(chan struct{})
	source := newTestSource(IdentityToken("jwt"), nil)

	first := make(chan error, 1)
	go func() {
		_, err := server.token(source)
		first <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for server.requests.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := source.Token(ctx, server.Client(), server.URL)
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Errorf("waiting Token() error = %v, want deadline exceeded", err)
	}

	close(server.gate)
	if err := <-first; err != nil {
		t.Errorf("exchanging Token() error = %v", err)
	}
}

func TestFederationTokenExchangeErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(s *tokenServer)
		want  []string
	}{
		{
			name:  "http error carries status, request id and body",
			setup: func(s *tokenServer) { s.fail.Store(true) },
			want:  []string{"401", "req_123", "authentication_error"},
		},
		{
			name:  "zero expires_in",
			setup: func(s *tokenServer) { s.expiresIn.Store(0) },
			want:  []string{"expires_in"},
		},
		{
			name:  "unsupported token_type",
			setup: func(s *tokenServer) { s.tokenType.Store("MAC") },
			want:  []string{"token_type", "MAC"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTokenServer(t)
			tt.setup(server)
			_, err := server.token(newTestSource(IdentityToken("jwt"), nil))
			if err == nil {
				t.Fatal("Token() error = nil, want error")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err.Error(), want)
				}
			}
			if strings.Contains(err.Error(), "jwt") {
				t.Errorf("error %q leaks the identity token", err.Error())
			}
		})
	}
}

func TestIdentityTokenFileRereadsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	provider := IdentityTokenFile(path)

	if _, err := provider(context.Background()); err == nil {
		t.Error("missing file: error = nil, want error")
	}

	for _, want := range []string{"jwt-a", "jwt-b"} {
		if err := os.WriteFile(path, []byte(want+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := provider(context.Background())
		if err != nil || got != want {
			t.Errorf("provider() = %q, %v, want %q", got, err, want)
		}
	}
}

func TestFederationFromEnv(t *testing.T) {
	base := map[string]string{
		"ANTHROPIC_FEDERATION_RULE_ID": "fdrl_1",
		"ANTHROPIC_ORGANIZATION_ID":    "org-1",
		"ANTHROPIC_SERVICE_ACCOUNT_ID": "svac_1",
		"ANTHROPIC_WORKSPACE_ID":       "wrkspc_1",
	}
	with := func(extra map[string]string, drop ...string) func(string) string {
		env := map[string]string{}
		for k, v := range base {
			env[k] = v
		}
		for k, v := range extra {
			env[k] = v
		}
		for _, k := range drop {
			delete(env, k)
		}
		return func(key string) string { return env[key] }
	}

	tests := []struct {
		name      string
		getenv    func(string) string
		wantOK    bool
		wantToken string
	}{
		{"no identity token", with(nil), false, ""},
		{"inline identity token", with(map[string]string{"ANTHROPIC_IDENTITY_TOKEN": "jwt-env"}), true, "jwt-env"},
		{"missing rule id", with(map[string]string{"ANTHROPIC_IDENTITY_TOKEN": "jwt"}, "ANTHROPIC_FEDERATION_RULE_ID"), false, ""},
		{"missing service account", with(map[string]string{"ANTHROPIC_IDENTITY_TOKEN": "jwt"}, "ANTHROPIC_SERVICE_ACCOUNT_ID"), false, ""},
		{"workspace is optional", with(map[string]string{"ANTHROPIC_IDENTITY_TOKEN": "jwt"}, "ANTHROPIC_WORKSPACE_ID"), true, "jwt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, opts, ok := FederationFromEnv(tt.getenv)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if opts.FederationRuleID != "fdrl_1" || opts.OrganizationID != "org-1" || opts.ServiceAccountID != "svac_1" {
				t.Errorf("opts = %+v", opts)
			}
			got, err := provider(context.Background())
			if err != nil || got != tt.wantToken {
				t.Errorf("provider() = %q, %v, want %q", got, err, tt.wantToken)
			}
		})
	}

	t.Run("token file wins over inline token", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "token")
		if err := os.WriteFile(path, []byte("jwt-file"), 0o600); err != nil {
			t.Fatal(err)
		}
		provider, _, ok := FederationFromEnv(with(map[string]string{
			"ANTHROPIC_IDENTITY_TOKEN_FILE": path,
			"ANTHROPIC_IDENTITY_TOKEN":      "jwt-env",
		}))
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if got, _ := provider(context.Background()); got != "jwt-file" {
			t.Errorf("provider() = %q, want jwt-file", got)
		}
	})
}

func newFederatedClient(server *tokenServer) *AnthropicClient {
	return NewClient("",
		WithModel("m"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithFederationTokenProvider(IdentityToken("jwt"), FederationOptions{
			FederationRuleID: "fdrl_1",
			OrganizationID:   "org-1",
			ServiceAccountID: "svac_1",
		}),
	)
}

func TestClientRequestBuildersUseFederatedBearerToken(t *testing.T) {
	server := newTokenServer(t)
	client := newFederatedClient(server)

	for _, create := range []func(context.Context, *CompletionRequest, string) (*http.Request, error){
		client.createHTTPRequest,
		client.createStreamingHTTPRequest,
	} {
		req, err := create(context.Background(), &CompletionRequest{Model: "m"}, "/v1/messages")
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Errorf("Authorization = %q, want Bearer token-1", got)
		}
		if got := req.Header.Get("X-API-Key"); got != "" {
			t.Errorf("X-API-Key = %q, want empty", got)
		}
	}
	if n := server.requests.Load(); n != 1 {
		t.Errorf("exchange requests = %d, want 1", n)
	}
}

func TestClientGenerateAndChatUseFederatedBearerToken(t *testing.T) {
	server := newTokenServer(t)
	client := newFederatedClient(server)
	ctx := context.Background()

	calls := map[string]func() (string, error){
		"Generate": func() (string, error) { return client.Generate(ctx, "hi") },
		"Chat": func() (string, error) {
			return client.Chat(ctx, []llm.Message{{Role: "user", Content: "hi"}}, nil)
		},
	}
	for name, call := range calls {
		got, err := call()
		if err != nil || got != "hello" {
			t.Fatalf("%s() = %q, %v, want hello", name, got, err)
		}
		server.mu.Lock()
		headers := server.lastMessages
		server.mu.Unlock()
		if got := headers.Get("Authorization"); got != "Bearer token-1" {
			t.Errorf("%s: Authorization = %q, want Bearer token-1", name, got)
		}
		if got := headers.Get("X-API-Key"); got != "" {
			t.Errorf("%s: X-API-Key = %q, want empty", name, got)
		}
	}
	if n := server.requests.Load(); n != 1 {
		t.Errorf("exchange requests = %d, want 1 (token should be cached across calls)", n)
	}
}

func TestClientUsesAPIKeyWithoutFederation(t *testing.T) {
	client := NewClient("sk-test")
	req, err := client.createHTTPRequest(context.Background(), &CompletionRequest{Model: "m"}, "/v1/messages")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if got := req.Header.Get("X-API-Key"); got != "sk-test" {
		t.Errorf("X-API-Key = %q, want sk-test", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty", got)
	}
}
