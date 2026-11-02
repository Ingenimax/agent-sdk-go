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
	"sync/atomic"
	"testing"
	"time"
)

// newTokenServer returns a server that mints "token-N" on the Nth exchange and records the last request body.
func newTokenServer(t *testing.T, expiresIn int64, fail *atomic.Bool, lastBody *tokenExchangeRequest) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/oauth/token" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if fail != nil && fail.Load() {
			w.Header().Set("Request-Id", "req_123")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"Authentication failed"}}`))
			return
		}
		if lastBody != nil {
			if err := json.NewDecoder(r.Body).Decode(lastBody); err != nil {
				t.Errorf("failed to decode token request: %v", err)
			}
		}
		n := calls.Add(1)
		_ = json.NewEncoder(w).Encode(tokenExchangeResponse{
			AccessToken: fmt.Sprintf("token-%d", n),
			TokenType:   "Bearer",
			ExpiresIn:   expiresIn,
		})
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestFederationTokenExchangeRequest(t *testing.T) {
	var body tokenExchangeRequest
	server, _ := newTokenServer(t, 600, nil, &body)

	source := &federationTokenSource{
		provider: IdentityToken("jwt-1"),
		opts: FederationOptions{
			FederationRuleID: "fdrl_1",
			OrganizationID:   "org-1",
			ServiceAccountID: "svac_1",
			WorkspaceID:      "wrkspc_1",
		},
		now: time.Now,
	}

	token, err := source.Token(context.Background(), server.Client(), server.URL)
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
	if body != want {
		t.Errorf("token request = %+v, want %+v", body, want)
	}
}

func TestFederationTokenRefresh(t *testing.T) {
	var fail atomic.Bool
	server, calls := newTokenServer(t, 600, &fail, nil)

	now := time.Unix(1_000_000, 0)
	source := &federationTokenSource{
		provider: IdentityToken("jwt"),
		now:      func() time.Time { return now },
	}
	token := func() (string, error) {
		return source.Token(context.Background(), server.Client(), server.URL)
	}

	if got, err := token(); err != nil || got != "token-1" {
		t.Fatalf("first Token() = %q, %v", got, err)
	}

	// Well before expiry: cached token, no new exchange.
	now = now.Add(5 * time.Minute)
	if got, err := token(); err != nil || got != "token-1" || calls.Load() != 1 {
		t.Fatalf("cached Token() = %q, %v, exchanges = %d", got, err, calls.Load())
	}

	// Inside the advisory window with a failing endpoint: cached token is still served.
	now = time.Unix(1_000_000, 0).Add(600*time.Second - 90*time.Second)
	fail.Store(true)
	if got, err := token(); err != nil || got != "token-1" {
		t.Fatalf("advisory Token() = %q, %v, want cached token", got, err)
	}

	// Inside the mandatory window with a failing endpoint: error.
	now = time.Unix(1_000_000, 0).Add(600*time.Second - 20*time.Second)
	if _, err := token(); err == nil {
		t.Fatal("mandatory Token() error = nil, want error")
	}

	// Endpoint recovers: a new token is minted.
	fail.Store(false)
	if got, err := token(); err != nil || got != "token-2" {
		t.Fatalf("refreshed Token() = %q, %v, want token-2", got, err)
	}
}

func TestFederationTokenExchangeError(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	server, _ := newTokenServer(t, 600, &fail, nil)

	source := &federationTokenSource{provider: IdentityToken("jwt"), now: time.Now}
	_, err := source.Token(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("Token() error = nil, want error")
	}
	for _, want := range []string{"401", "req_123", "authentication_error"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err.Error(), want)
		}
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

func TestClientUsesFederatedBearerToken(t *testing.T) {
	server, _ := newTokenServer(t, 600, nil, nil)

	client := NewClient("",
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithFederationTokenProvider(IdentityToken("jwt"), FederationOptions{
			FederationRuleID: "fdrl_1",
			OrganizationID:   "org-1",
			ServiceAccountID: "svac_1",
		}),
	)

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
