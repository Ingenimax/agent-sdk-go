package a2a

import (
	"github.com/a2aproject/a2a-go/a2a"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// CardBuilder constructs an A2A AgentCard from agent-sdk-go Agent metadata.
type CardBuilder struct {
	name             string
	description      string
	url              string
	version          string
	providerOrg      string
	providerURL      string
	documentationURL string
	streaming        bool
	inputModes       []string
	outputModes      []string
	skills           []a2a.AgentSkill
	tools            []interfaces.Tool
}

// NewCardBuilder creates a new CardBuilder with required fields.
// It panics if name, description, or url is empty.
func NewCardBuilder(name, description, url string, opts ...CardOption) *CardBuilder {
	if name == "" {
		panic("a2a: NewCardBuilder requires a non-empty name")
	}
	if description == "" {
		panic("a2a: NewCardBuilder requires a non-empty description")
	}
	if url == "" {
		panic("a2a: NewCardBuilder requires a non-empty url")
	}
	b := &CardBuilder{
		name:        name,
		description: description,
		url:         url,
		version:     "1.0.0",
		streaming:   true,
		inputModes:  []string{"text/plain"},
		outputModes: []string{"text/plain"},
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// AddSkill adds an explicit skill to the card.
func (b *CardBuilder) AddSkill(skill a2a.AgentSkill) *CardBuilder {
	b.skills = append(b.skills, skill)
	return b
}

// SetTools sets the agent's tools, which will be converted to skills on Build().
func (b *CardBuilder) SetTools(tools []interfaces.Tool) *CardBuilder {
	b.tools = tools
	return b
}

// Build constructs the AgentCard.
func (b *CardBuilder) Build() *a2a.AgentCard {
	skills := make([]a2a.AgentSkill, 0, len(b.skills)+len(b.tools))
	skills = append(skills, b.skills...)

	for _, t := range b.tools {
		skill := a2a.AgentSkill{
			ID:          t.Name(),
			Name:        toolDisplayName(t),
			Description: t.Description(),
			Tags:        []string{"tool"},
		}
		skills = append(skills, skill)
	}

	// A2A requires at least one skill.
	if len(skills) == 0 {
		skills = append(skills, a2a.AgentSkill{
			ID:          "default",
			Name:        b.name,
			Description: b.description,
			Tags:        []string{"general"},
		})
	}

	card := &a2a.AgentCard{
		Name:               b.name,
		Description:        b.description,
		URL:                b.url,
		Version:            b.version,
		PreferredTransport: a2a.TransportProtocolJSONRPC,
		DefaultInputModes:  b.inputModes,
		DefaultOutputModes: b.outputModes,
		Capabilities: a2a.AgentCapabilities{
			Streaming: b.streaming,
		},
		Skills: skills,
	}

	if b.providerOrg != "" {
		card.Provider = &a2a.AgentProvider{
			Org: b.providerOrg,
			URL: b.providerURL,
		}
	}

	if b.documentationURL != "" {
		card.DocumentationURL = b.documentationURL
	}

	return card
}

func toolDisplayName(t interfaces.Tool) string {
	if dn, ok := t.(interfaces.ToolWithDisplayName); ok {
		return dn.DisplayName()
	}
	return t.Name()
}
