package a2a

import (
	"context"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

type mockTool struct {
	name        string
	description string
	displayName string
}

func (t *mockTool) Name() string                                             { return t.name }
func (t *mockTool) Description() string                                      { return t.description }
func (t *mockTool) Run(_ context.Context, _ string) (string, error)          { return "", nil }
func (t *mockTool) Execute(_ context.Context, _ string) (string, error)      { return "", nil }
func (t *mockTool) Parameters() map[string]interfaces.ParameterSpec          { return nil }
func (t *mockTool) DisplayName() string                                      { return t.displayName }

func TestCardBuilder_Build(t *testing.T) {
	card := NewCardBuilder("TestAgent", "A test agent", "http://localhost:9000/a2a",
		WithVersion("2.0.0"),
		WithProviderInfo("Ingenimax", "https://ingenimax.com"),
		WithStreaming(true),
		WithDocumentationURL("https://docs.example.com"),
		WithInputModes("text/plain", "application/json"),
		WithOutputModes("text/plain"),
	).Build()

	if card.Name != "TestAgent" {
		t.Errorf("expected name TestAgent, got %s", card.Name)
	}
	if card.Description != "A test agent" {
		t.Errorf("expected description 'A test agent', got %s", card.Description)
	}
	if card.URL != "http://localhost:9000/a2a" {
		t.Errorf("expected URL http://localhost:9000/a2a, got %s", card.URL)
	}
	if card.Version != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %s", card.Version)
	}
	if card.Provider == nil || card.Provider.Org != "Ingenimax" {
		t.Errorf("expected provider org Ingenimax, got %v", card.Provider)
	}
	if !card.Capabilities.Streaming {
		t.Error("expected streaming enabled")
	}
	if card.DocumentationURL != "https://docs.example.com" {
		t.Errorf("expected documentation URL, got %s", card.DocumentationURL)
	}
	if len(card.DefaultInputModes) != 2 {
		t.Errorf("expected 2 input modes, got %d", len(card.DefaultInputModes))
	}
	// Should have default skill when no tools or skills added
	if len(card.Skills) != 1 || card.Skills[0].ID != "default" {
		t.Errorf("expected default skill, got %v", card.Skills)
	}
}

func TestCardBuilder_WithTools(t *testing.T) {
	tools := []interfaces.Tool{
		&mockTool{name: "web_search", description: "Search the web", displayName: "Web Search"},
		&mockTool{name: "calculator", description: "Do math", displayName: "Calculator"},
	}

	card := NewCardBuilder("ToolAgent", "Agent with tools", "http://localhost:9000").
		SetTools(tools).
		Build()

	if len(card.Skills) != 2 {
		t.Fatalf("expected 2 skills from tools, got %d", len(card.Skills))
	}
	if card.Skills[0].ID != "web_search" {
		t.Errorf("expected skill ID web_search, got %s", card.Skills[0].ID)
	}
	if card.Skills[0].Name != "Web Search" {
		t.Errorf("expected skill name 'Web Search', got %s", card.Skills[0].Name)
	}
	if card.Skills[1].ID != "calculator" {
		t.Errorf("expected skill ID calculator, got %s", card.Skills[1].ID)
	}
}

func TestCardBuilder_WithExplicitSkills(t *testing.T) {
	card := NewCardBuilder("SkillAgent", "Agent with explicit skills", "http://localhost:9000").
		AddSkill(a2a.AgentSkill{
			ID:          "translate",
			Name:        "Translate",
			Description: "Translate text between languages",
			Tags:        []string{"translation", "language"},
		}).
		Build()

	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(card.Skills))
	}
	if card.Skills[0].ID != "translate" {
		t.Errorf("expected skill ID translate, got %s", card.Skills[0].ID)
	}
}

func TestCardBuilder_DefaultValues(t *testing.T) {
	card := NewCardBuilder("Minimal", "Minimal agent", "http://localhost:9000").Build()

	if card.Version != "1.0.0" {
		t.Errorf("expected default version 1.0.0, got %s", card.Version)
	}
	if card.PreferredTransport != a2a.TransportProtocolJSONRPC {
		t.Errorf("expected JSONRPC transport, got %s", card.PreferredTransport)
	}
	if len(card.DefaultInputModes) != 1 || card.DefaultInputModes[0] != "text/plain" {
		t.Errorf("expected default input mode text/plain, got %v", card.DefaultInputModes)
	}
}

func TestCardBuilder_EmptyName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty name")
		}
	}()
	NewCardBuilder("", "desc", "http://localhost")
}

func TestCardBuilder_EmptyDescription(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty description")
		}
	}()
	NewCardBuilder("name", "", "http://localhost")
}

func TestCardBuilder_EmptyURL(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty url")
		}
	}()
	NewCardBuilder("name", "desc", "")
}
