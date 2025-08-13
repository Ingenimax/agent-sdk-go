package agent

import (
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestFormatHistoryIntoPrompt(t *testing.T) {
	tests := []struct {
		name     string
		history  []interfaces.Message
		expected string
	}{
		{
			name: "basic conversation with clear role markers",
			history: []interfaces.Message{
				{Role: "user", Content: "Hello, how are you?"},
				{Role: "assistant", Content: "I'm doing well, thank you!"},
				{Role: "user", Content: "What can you help me with?"},
			},
			expected: "USER: Hello, how are you?\n\nASSISTANT: I'm doing well, thank you!\n\nUSER: What can you help me with?",
		},
		{
			name: "conversation with tool messages",
			history: []interfaces.Message{
				{Role: "user", Content: "list which clusters I have available"},
				{Role: "assistant", Content: `{"reasoning":["User is requesting a list of available clusters"]}`},
				{Role: "tool", Content: `{"query": "list all EKS clusters", "output": "eks-cluster-1"}`},
				{Role: "assistant", Content: "You have eks-cluster-1 available"},
			},
			expected: "USER: list which clusters I have available\n\nASSISTANT: {\"reasoning\":[\"User is requesting a list of available clusters\"]}\n\nTOOL: {\"query\": \"list all EKS clusters\", \"output\": \"eks-cluster-1\"}\n\nASSISTANT: You have eks-cluster-1 available",
		},
		{
			name: "single message",
			history: []interfaces.Message{
				{Role: "user", Content: "Hello"},
			},
			expected: "USER: Hello",
		},
		{
			name: "system message included",
			history: []interfaces.Message{
				{Role: "system", Content: "You are a helpful assistant"},
				{Role: "user", Content: "Hi"},
				{Role: "assistant", Content: "Hello!"},
			},
			expected: "SYSTEM: You are a helpful assistant\n\nUSER: Hi\n\nASSISTANT: Hello!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatHistoryIntoPrompt(tt.history)
			if result != tt.expected {
				t.Errorf("formatHistoryIntoPrompt() mismatch\nGot:\n%s\n\nExpected:\n%s", result, tt.expected)
				
				// Show differences for debugging
				gotLines := strings.Split(result, "\n")
				expectedLines := strings.Split(tt.expected, "\n")
				
				maxLines := len(gotLines)
				if len(expectedLines) > maxLines {
					maxLines = len(expectedLines)
				}
				
				for i := 0; i < maxLines; i++ {
					gotLine := ""
					expectedLine := ""
					
					if i < len(gotLines) {
						gotLine = gotLines[i]
					}
					if i < len(expectedLines) {
						expectedLine = expectedLines[i]
					}
					
					if gotLine != expectedLine {
						t.Errorf("Line %d differs:\nGot:      '%s'\nExpected: '%s'", i+1, gotLine, expectedLine)
					}
				}
			}
		})
	}
}