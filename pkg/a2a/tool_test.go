package a2a

import (
	"context"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestSanitizeToolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Recipe Agent", "recipe_agent"},
		{"hello-world", "hello_world"},
		{"Test123", "test123"},
		{"Special!@#$chars", "special_chars"},
		{"!!!", "remote_agent"},
		{"", "remote_agent"},
		{"123agent", "agent_123agent"},
		{"---hello---", "hello"},
	}

	for _, tt := range tests {
		result := sanitizeToolName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeToolName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractResultText_Task(t *testing.T) {
	task := &a2a.Task{
		ID:        a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Artifacts: []*a2a.Artifact{
			{
				ID:    a2a.NewArtifactID(),
				Parts: a2a.ContentParts{a2a.TextPart{Text: "Hello"}},
			},
			{
				ID:    a2a.NewArtifactID(),
				Parts: a2a.ContentParts{a2a.TextPart{Text: "World"}},
			},
		},
	}
	text := ExtractResultText(task)
	if !strings.Contains(text, "Hello") || !strings.Contains(text, "World") {
		t.Errorf("expected text to contain Hello and World, got %q", text)
	}
}

func TestExtractResultText_Message(t *testing.T) {
	msg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: "response text"})
	text := ExtractResultText(msg)
	if text != "response text" {
		t.Errorf("expected 'response text', got %q", text)
	}
}

func TestExtractResultText_EmptyTask(t *testing.T) {
	task := &a2a.Task{
		ID:        a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Status: a2a.TaskStatus{
			State:   a2a.TaskStateCompleted,
			Message: a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: "status msg"}),
		},
	}
	text := ExtractResultText(task)
	if text != "status msg" {
		t.Errorf("expected 'status msg', got %q", text)
	}
}

func TestPartToText_DataPart(t *testing.T) {
	logger := logging.New()
	ctx := context.Background()

	dp := a2a.DataPart{Data: map[string]any{"count": 42, "active": true}}
	text := partToText(ctx, logger, dp)
	if !strings.Contains(text, "42") || !strings.Contains(text, "active") {
		t.Errorf("expected JSON representation, got %q", text)
	}
}

func TestPartToText_FilePartURI(t *testing.T) {
	logger := logging.New()
	ctx := context.Background()

	fp := a2a.FilePart{File: a2a.FileURI{
		FileMeta: a2a.FileMeta{Name: "report.pdf"},
		URI:      "https://example.com/report.pdf",
	}}
	text := partToText(ctx, logger, fp)
	if !strings.Contains(text, "report.pdf") {
		t.Errorf("expected file name in text, got %q", text)
	}
}

func TestPartToText_FilePartURINoName(t *testing.T) {
	logger := logging.New()
	ctx := context.Background()

	fp := a2a.FilePart{File: a2a.FileURI{
		URI: "https://example.com/unnamed.bin",
	}}
	text := partToText(ctx, logger, fp)
	if !strings.Contains(text, "https://example.com/unnamed.bin") {
		t.Errorf("expected URI as fallback name, got %q", text)
	}
}

func TestPartToText_FilePartBytes(t *testing.T) {
	logger := logging.New()
	ctx := context.Background()

	fp := a2a.FilePart{File: a2a.FileBytes{
		FileMeta: a2a.FileMeta{Name: "image.png"},
		Bytes:    "aGVsbG8gd29ybGQ=",
	}}
	text := partToText(ctx, logger, fp)
	if !strings.Contains(text, "image.png") {
		t.Errorf("expected file name, got %q", text)
	}
	if !strings.Contains(text, "base64") {
		t.Errorf("expected base64 indicator, got %q", text)
	}
}

func TestPartToText_FilePartBytesNoName(t *testing.T) {
	logger := logging.New()
	ctx := context.Background()

	fp := a2a.FilePart{File: a2a.FileBytes{
		Bytes: "AQID",
	}}
	text := partToText(ctx, logger, fp)
	if !strings.Contains(text, "unnamed") {
		t.Errorf("expected 'unnamed' fallback, got %q", text)
	}
}

func TestWithToolName(t *testing.T) {
	// We need a mock client for this test. Create a minimal one.
	c := &Client{
		card: &a2a.AgentCard{
			Name:        "Agent A",
			Description: "test agent",
		},
		logger: logging.New(),
	}

	// Without override
	tool1 := NewRemoteAgentTool(c)
	if tool1.Name() != "agent_a" {
		t.Errorf("expected 'agent_a', got %q", tool1.Name())
	}

	// With override
	tool2 := NewRemoteAgentTool(c, WithToolName("custom_name"))
	if tool2.Name() != "custom_name" {
		t.Errorf("expected 'custom_name', got %q", tool2.Name())
	}

	// Two tools with same agent but different names (no collision)
	tool3 := NewRemoteAgentTool(c, WithToolName("agent_a_v2"))
	if tool2.Name() == tool3.Name() {
		t.Error("expected different names for disambiguated tools")
	}
}

func TestExtractResultText_TaskWithDataPart(t *testing.T) {
	task := &a2a.Task{
		ID:        a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Artifacts: []*a2a.Artifact{
			{
				ID: a2a.NewArtifactID(),
				Parts: a2a.ContentParts{
					a2a.TextPart{Text: "Here is data:"},
					a2a.DataPart{Data: map[string]any{"result": "success"}},
				},
			},
		},
	}
	text := ExtractResultText(task)
	if !strings.Contains(text, "Here is data:") {
		t.Error("expected text part in result")
	}
	if !strings.Contains(text, "success") {
		t.Error("expected data part converted to JSON in result")
	}
}

func TestExtractResultText_TaskWithFilePart(t *testing.T) {
	task := &a2a.Task{
		ID:        a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Artifacts: []*a2a.Artifact{
			{
				ID: a2a.NewArtifactID(),
				Parts: a2a.ContentParts{
					a2a.FilePart{File: a2a.FileURI{
						FileMeta: a2a.FileMeta{Name: "output.csv"},
						URI:      "https://example.com/output.csv",
					}},
				},
			},
		},
	}
	text := ExtractResultText(task)
	if !strings.Contains(text, "output.csv") {
		t.Errorf("expected file name in result, got %q", text)
	}
}
