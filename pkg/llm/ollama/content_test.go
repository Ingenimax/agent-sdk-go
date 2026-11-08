package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestGenerateSendsImagesAsBase64(t *testing.T) {
	var got GenerateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(GenerateResponse{Response: "a cat", Done: true})
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithModel("llava"))
	parts := interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1, 2}, "image/png"), interfaces.NewPartFromText("describe"))
	if _, err := client.Generate(context.Background(), "ignored", parts); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(got.Images) != 1 || got.Images[0] != "AQI=" {
		t.Errorf("request images = %v, want [AQI=]", got.Images)
	}
	if !strings.Contains(got.Prompt, "describe") || strings.Contains(got.Prompt, "ignored") {
		t.Errorf("request prompt = %q, want the text part", got.Prompt)
	}
}

func TestGenerateRefusesImageURLs(t *testing.T) {
	client := NewClient(WithBaseURL("http://127.0.0.1:0"), WithModel("llava"))
	_, err := client.Generate(context.Background(), "describe", interfaces.WithParts(interfaces.NewPartFromURI("https://example.com/a.png", "image/png")))
	if !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("Generate() error = %v, want ErrContentUnsupported", err)
	}
}

func TestGenerateJSONKeepsTextParts(t *testing.T) {
	var got GenerateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(GenerateResponse{Response: "{}", Done: true})
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithModel("llava"))
	format := func(o *interfaces.GenerateOptions) {
		o.ResponseFormat = &interfaces.ResponseFormat{Type: interfaces.ResponseFormatJSON, Name: "answer"}
	}
	parts := interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "image/png"), interfaces.NewPartFromText("describe"))
	if _, err := client.Generate(context.Background(), "ignored", parts, format); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !strings.Contains(got.Prompt, "describe") || strings.Contains(got.Prompt, "ignored") {
		t.Errorf("request prompt = %q, want the text part", got.Prompt)
	}
}
