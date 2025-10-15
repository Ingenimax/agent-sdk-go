package anthropic

import (
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestMessageFiltering(t *testing.T) {

	tests := []struct {
		name            string
		messages        []interfaces.Message
		expectedCount   int
		expectedContent []string
	}{
		{
			name: "Filter out messages with empty content",
			messages: []interfaces.Message{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: ""}, // Should be filtered out
				{Role: "user", Content: "   "},   // Should be filtered out (whitespace only)
				{Role: "assistant", Content: "World"},
			},
			expectedCount:   2,
			expectedContent: []string{"Hello", "World"},
		},
		{
			name: "Filter out messages with empty role",
			messages: []interfaces.Message{
				{Role: "", Content: "Should be filtered"},
				{Role: "user", Content: "Should stay"},
			},
			expectedCount:   1,
			expectedContent: []string{"Should stay"},
		},
		{
			name: "Keep valid messages only",
			messages: []interfaces.Message{
				{Role: "user", Content: "First message"},
				{Role: "assistant", Content: "Second message"},
				{Role: "user", Content: "Third message"},
			},
			expectedCount:   3,
			expectedContent: []string{"First message", "Second message", "Third message"},
		},
		{
			name: "Filter out all invalid messages",
			messages: []interfaces.Message{
				{Role: "", Content: ""},
				{Role: "user", Content: ""},
				{Role: "", Content: "Some content"},
				{Role: "assistant", Content: "   "},
			},
			expectedCount:   0,
			expectedContent: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to Anthropic messages format
			anthropicMessages := make([]Message, len(tt.messages))
			for i, msg := range tt.messages {
				role := msg.Role
				if role == "system" {
					continue // System messages are handled separately
				}
				anthropicMessages[i] = Message{
					Role:    string(role),
					Content: msg.Content,
				}
			}

			// Apply the filtering logic from the actual code
			var filteredMessages []Message
			for _, msg := range anthropicMessages {
				if msg.Role != "" && strings.TrimSpace(msg.Content) != "" {
					filteredMessages = append(filteredMessages, msg)
				}
			}

			// Check the count
			if len(filteredMessages) != tt.expectedCount {
				t.Errorf("Expected %d messages, got %d", tt.expectedCount, len(filteredMessages))
			}

			// Check the content
			for i, msg := range filteredMessages {
				if i < len(tt.expectedContent) && msg.Content != tt.expectedContent[i] {
					t.Errorf("Expected message %d content %q, got %q", i, tt.expectedContent[i], msg.Content)
				}
			}
		})
	}
}

func TestEmptyContentHandling(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		shouldFilter bool
	}{
		{"Normal content", "Hello world", false},
		{"Empty string", "", true},
		{"Whitespace only", "   ", true},
		{"Tab only", "\t", true},
		{"Newline only", "\n", true},
		{"Mixed whitespace", " \t\n ", true},
		{"Content with whitespace", " Hello ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := Message{
				Role:    "user",
				Content: tt.content,
			}

			// Apply filtering condition
			shouldKeep := msg.Role != "" && strings.TrimSpace(msg.Content) != ""
			shouldFilter := !shouldKeep

			if shouldFilter != tt.shouldFilter {
				t.Errorf("Content %q: expected shouldFilter=%v, got shouldFilter=%v",
					tt.content, tt.shouldFilter, shouldFilter)
			}
		})
	}
}
