package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Tools returns the tools that expose a library to a model.
//
// Two tools, deliberately: one to read a skill's instructions, one to read a
// bundled file. Registering a tool per skill instead would put every skill's
// full schema in context on every request, which is exactly the cost
// progressive disclosure exists to avoid.
func Tools(lib *Library) []interfaces.Tool {
	return []interfaces.Tool{
		&loadSkillTool{lib: lib},
		&readResourceTool{lib: lib},
	}
}

// loadSkillTool returns a skill's full instructions.
type loadSkillTool struct{ lib *Library }

func (t *loadSkillTool) Name() string { return "load_skill" }

func (t *loadSkillTool) Description() string {
	var b strings.Builder
	b.WriteString("Load the full instructions for a skill. ")
	b.WriteString(t.lib.Catalog())
	return b.String()
}

func (t *loadSkillTool) Parameters() map[string]interfaces.ParameterSpec {
	names := make([]interface{}, 0, t.lib.Len())
	for _, s := range t.lib.List() {
		names = append(names, s.Name)
	}

	spec := interfaces.ParameterSpec{
		Type:        "string",
		Description: "The name of the skill to load",
		Required:    true,
	}
	// Constraining to the known names stops the model inventing a skill that
	// does not exist, which otherwise costs a whole turn to discover.
	if len(names) > 0 {
		spec.Enum = names
	}

	return map[string]interfaces.ParameterSpec{"name": spec}
}

func (t *loadSkillTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *loadSkillTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parsing arguments: %w", err)
	}
	if params.Name == "" {
		return "", fmt.Errorf("a skill name is required")
	}

	s, ok := t.lib.Get(params.Name)
	if !ok {
		// Listing what does exist turns a dead end into a recoverable turn.
		return fmt.Sprintf("No skill named %q. %s", params.Name, t.lib.Catalog()), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Skill: %s\n\n%s\n", s.Name, s.Instructions)

	if len(s.Resources) > 0 {
		b.WriteString("\n## Bundled resources\n\n")
		for _, r := range s.Resources {
			fmt.Fprintf(&b, "- %s\n", r)
		}
		b.WriteString("\nCall read_skill_resource to read one.\n")
	}

	return b.String(), nil
}

// readResourceTool returns a file bundled with a skill.
type readResourceTool struct{ lib *Library }

func (t *readResourceTool) Name() string { return "read_skill_resource" }

func (t *readResourceTool) Description() string {
	return "Read a file bundled with a skill. Call load_skill first to see what a skill provides."
}

func (t *readResourceTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"skill": {
			Type:        "string",
			Description: "The skill that owns the resource",
			Required:    true,
		},
		"path": {
			Type:        "string",
			Description: "The resource path, as listed by load_skill",
			Required:    true,
		},
	}
}

func (t *readResourceTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *readResourceTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		Skill string `json:"skill"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parsing arguments: %w", err)
	}

	s, ok := t.lib.Get(params.Skill)
	if !ok {
		return fmt.Sprintf("No skill named %q.", params.Skill), nil
	}

	content, err := s.ReadResource(params.Path)
	if err != nil {
		// Reported to the model rather than failing the run: a wrong path is a
		// recoverable mistake, and the resource list is right there.
		return fmt.Sprintf("Could not read %q from skill %q: %v", params.Path, params.Skill, err), nil
	}

	return content, nil
}

var (
	_ interfaces.Tool = (*loadSkillTool)(nil)
	_ interfaces.Tool = (*readResourceTool)(nil)
)
