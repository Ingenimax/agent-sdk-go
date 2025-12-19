package agent

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
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
			name:  "file path with JSON content",
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
			name:  "file with invalid JSON",
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
