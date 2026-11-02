package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

func TestParseGoogleCredentials(t *testing.T) {
	validJSON := `{"type":"service_account","project_id":"test-project"}`

	tests := []struct {
		name        string
		input       string
		setupFunc   func() (string, func())
		wantContent string
		wantErr     bool
	}{
		{
			name:        "raw JSON content",
			input:       validJSON,
			wantContent: validJSON,
			wantErr:     false,
		},
		{
			name:        "base64 encoded JSON",
			input:       base64.StdEncoding.EncodeToString([]byte(validJSON)),
			wantContent: validJSON,
			wantErr:     false,
		},
		{
			name: "file path with JSON content",
			setupFunc: func() (string, func()) {
				tmpDir := t.TempDir()
				tmpFile := filepath.Join(tmpDir, "credentials.json")
				err := os.WriteFile(tmpFile, []byte(validJSON), 0644)
				if err != nil {
					t.Fatalf("failed to create temp file: %v", err)
				}
				return tmpFile, func() {}
			},
			wantContent: validJSON,
			wantErr:     false,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   "not valid json",
			wantErr: true,
		},
		{
			name:    "base64 of invalid JSON",
			input:   base64.StdEncoding.EncodeToString([]byte("not valid json")),
			wantErr: true,
		},
		{
			name: "file with invalid JSON",
			setupFunc: func() (string, func()) {
				tmpDir := t.TempDir()
				tmpFile := filepath.Join(tmpDir, "invalid.json")
				err := os.WriteFile(tmpFile, []byte("not valid json"), 0644)
				if err != nil {
					t.Fatalf("failed to create temp file: %v", err)
				}
				return tmpFile, func() {}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.input
			cleanup := func() {}

			if tt.setupFunc != nil {
				var cleanupFunc func()
				input, cleanupFunc = tt.setupFunc()
				cleanup = cleanupFunc
			}
			defer cleanup()

			got, err := parseGoogleCredentials(input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGoogleCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.wantContent {
				t.Errorf("parseGoogleCredentials() = %v, want %v", got, tt.wantContent)
			}
		})
	}
}

func TestParseGoogleCredentialsFormats(t *testing.T) {
	validJSON := `{"type":"service_account","project_id":"test-project","private_key":"-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n"}`

	// Test 1: Raw JSON with newlines and special characters
	t.Run("raw JSON with special characters", func(t *testing.T) {
		got, err := parseGoogleCredentials(validJSON)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != validJSON {
			t.Errorf("content mismatch")
		}
	})

	// Test 2: Base64 encoded
	t.Run("base64 encoded", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte(validJSON))
		got, err := parseGoogleCredentials(encoded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != validJSON {
			t.Errorf("content mismatch")
		}
	})

	// Test 3: File path
	t.Run("file path", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "service-account.json")
		if err := os.WriteFile(tmpFile, []byte(validJSON), 0644); err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}

		got, err := parseGoogleCredentials(tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != validJSON {
			t.Errorf("content mismatch")
		}
	})
}

func TestCreateAnthropicClientFederation(t *testing.T) {
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_FEDERATION_RULE_ID", "ANTHROPIC_ORGANIZATION_ID", "ANTHROPIC_SERVICE_ACCOUNT_ID", "ANTHROPIC_WORKSPACE_ID", "ANTHROPIC_IDENTITY_TOKEN_FILE", "ANTHROPIC_IDENTITY_TOKEN", "VERTEX_AI_PROJECT"} {
		t.Setenv(key, "")
	}

	tests := []struct {
		name    string
		config  map[string]interface{}
		env     map[string]string
		wantErr bool
	}{
		{
			name: "federation from config",
			config: map[string]interface{}{
				"federation_rule_id": "fdrl_1",
				"organization_id":    "org-1",
				"service_account_id": "svac_1",
				"identity_token":     "jwt",
			},
		},
		{
			name: "federation from env",
			env: map[string]string{
				"ANTHROPIC_FEDERATION_RULE_ID":  "fdrl_1",
				"ANTHROPIC_ORGANIZATION_ID":     "org-1",
				"ANTHROPIC_SERVICE_ACCOUNT_ID":  "svac_1",
				"ANTHROPIC_IDENTITY_TOKEN_FILE": "/var/run/secrets/anthropic.com/token",
			},
		},
		{
			name: "config fills missing env fields",
			config: map[string]interface{}{
				"federation_rule_id": "fdrl_1",
				"identity_token":     "jwt",
			},
			env: map[string]string{
				"ANTHROPIC_ORGANIZATION_ID":    "org-1",
				"ANTHROPIC_SERVICE_ACCOUNT_ID": "svac_1",
			},
		},
		{
			name: "incomplete federation config",
			config: map[string]interface{}{
				"federation_rule_id": "fdrl_1",
				"organization_id":    "org-1",
			},
			wantErr: true,
		},
		{
			name:    "no credentials",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			client, err := createAnthropicClient(&LLMProviderYAML{Provider: "anthropic", Model: "claude-sonnet-4-5", Config: tt.config})
			if (err != nil) != tt.wantErr {
				t.Fatalf("createAnthropicClient() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			anthropicClient, ok := client.(*anthropic.AnthropicClient)
			if !ok {
				t.Fatalf("createAnthropicClient() returned %T, want *anthropic.AnthropicClient", client)
			}
			if anthropicClient.APIKey != "" {
				t.Errorf("APIKey = %q, want empty when federation is used", anthropicClient.APIKey)
			}
		})
	}
}

// TestCreateAnthropicClientFederationConfigBeatsEnv proves the YAML value is the one exchanged
// when the same field is set in both config and environment.
func TestCreateAnthropicClientFederationConfigBeatsEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("VERTEX_AI_PROJECT", "")
	t.Setenv("ANTHROPIC_IDENTITY_TOKEN_FILE", "")
	t.Setenv("ANTHROPIC_IDENTITY_TOKEN", "jwt-env")
	t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "fdrl_env")
	t.Setenv("ANTHROPIC_ORGANIZATION_ID", "org-env")
	t.Setenv("ANTHROPIC_SERVICE_ACCOUNT_ID", "svac_env")

	var exchange map[string]string
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth/token":
			if err := json.NewDecoder(r.Body).Decode(&exchange); err != nil {
				t.Errorf("decode token request: %v", err)
			}
			_, _ = w.Write([]byte(`{"access_token":"oat-1","token_type":"Bearer","expires_in":600}`))
		case "/v1/messages":
			authorization = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"m","stop_reason":"end_turn","content":[{"type":"text","text":"hello"}]}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := createAnthropicClient(&LLMProviderYAML{Provider: "anthropic", Model: "m", Config: map[string]interface{}{
		"base_url":           server.URL,
		"federation_rule_id": "fdrl_config",
		"service_account_id": "svac_config",
		"identity_token":     "jwt-config",
		// organization_id intentionally left to the environment
	}})
	if err != nil {
		t.Fatalf("createAnthropicClient() error = %v", err)
	}
	if got, err := client.Generate(context.Background(), "hi"); err != nil || got != "hello" {
		t.Fatalf("Generate() = %q, %v, want hello", got, err)
	}

	want := map[string]string{
		"assertion":          "jwt-config",
		"federation_rule_id": "fdrl_config",
		"service_account_id": "svac_config",
		"organization_id":    "org-env",
	}
	for key, value := range want {
		if exchange[key] != value {
			t.Errorf("exchange[%q] = %q, want %q", key, exchange[key], value)
		}
	}
	if authorization != "Bearer oat-1" {
		t.Errorf("Authorization = %q, want Bearer oat-1", authorization)
	}
}
