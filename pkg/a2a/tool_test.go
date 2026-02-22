package a2a

import (
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
)

func TestSanitizeToolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Recipe Agent", "recipe_agent"},
		{"hello-world", "hello_world"},
		{"Test123", "test123"},
		{"Special!@#$chars", "special____chars"},
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
	text := extractResultText(task)
	if !strings.Contains(text, "Hello") || !strings.Contains(text, "World") {
		t.Errorf("expected text to contain Hello and World, got %q", text)
	}
}

func TestExtractResultText_Message(t *testing.T) {
	msg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: "response text"})
	text := extractResultText(msg)
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
	text := extractResultText(task)
	if text != "status msg" {
		t.Errorf("expected 'status msg', got %q", text)
	}
}
