// Package skill loads SKILL.md capability bundles from disk and exposes them to
// an agent.
//
// A skill is a folder holding a SKILL.md whose YAML frontmatter carries at
// least a name and a description, followed by markdown instructions and
// optional bundled resources:
//
//	skills/
//	  incident-triage/
//	    SKILL.md
//	    runbook.md
//	    queries/slow-requests.sql
//
// # Progressive disclosure
//
// Only each skill's name and description sit in the model's context. The
// instructions load when the model chooses a skill and calls load_skill. That
// property is the whole point: twenty skills cost twenty one-line descriptions
// rather than twenty documents, so a large library stays affordable.
//
// # What this package will not do
//
// It does not execute anything. A skill is instructions and reference material;
// bundled files are read, never run. A format that silently executes code from
// a folder someone cloned is a supply-chain problem, and loading a skill is not
// a decision a user makes consciously enough to carry that risk.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Skill is one loaded capability bundle.
type Skill struct {
	// Name identifies the skill to the model. Required.
	Name string `yaml:"name"`

	// Description tells the model when to use it. Required, and the only part
	// besides the name that is always in context -- so it is what determines
	// whether the skill is ever chosen.
	Description string `yaml:"description"`

	// Version is optional metadata.
	Version string `yaml:"version,omitempty"`

	// Tags are optional grouping labels.
	Tags []string `yaml:"tags,omitempty"`

	// Instructions is the markdown body, loaded on demand.
	Instructions string `yaml:"-"`

	// Dir is the skill's directory on disk.
	Dir string `yaml:"-"`

	// Resources are the bundled files, relative to Dir.
	Resources []string `yaml:"-"`
}

// Validate reports whether a skill is usable.
func (s *Skill) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("skill requires a name")
	}
	if strings.TrimSpace(s.Description) == "" {
		// Without a description the model has nothing to select on, so the
		// skill can never be chosen. Better to reject it at load than to ship a
		// library with a silently unreachable entry.
		return fmt.Errorf("skill %q requires a description", s.Name)
	}
	return nil
}

// Library is a set of loaded skills.
type Library struct {
	mu     sync.RWMutex
	skills map[string]*Skill
	order  []string
}

// NewLibrary creates an empty library.
func NewLibrary() *Library {
	return &Library{skills: map[string]*Skill{}}
}

// Add puts a skill in the library, replacing any skill of the same name.
func (l *Library) Add(s *Skill) error {
	if err := s.Validate(); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.skills[s.Name]; !exists {
		l.order = append(l.order, s.Name)
	}
	l.skills[s.Name] = s
	return nil
}

// Get returns a skill by name.
func (l *Library) Get(name string) (*Skill, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	s, ok := l.skills[name]
	return s, ok
}

// List returns every skill, in load order.
func (l *Library) List() []*Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()

	out := make([]*Skill, 0, len(l.order))
	for _, name := range l.order {
		out = append(out, l.skills[name])
	}
	return out
}

// Len returns how many skills are loaded.
func (l *Library) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.skills)
}

// Catalog renders the name-and-description index that sits in the model's
// context.
//
// This is the entire context cost of a skill library until one is invoked.
func (l *Library) Catalog() string {
	skills := l.List()
	if len(skills) == 0 {
		return "No skills are available."
	}

	var b strings.Builder
	b.WriteString("Available skills. Call load_skill with a name to read its full instructions.\n\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
	}
	return b.String()
}

// LoadDir loads every skill under root.
//
// A directory containing a SKILL.md is a skill. Directories are walked one
// level deep, which matches how skill collections are laid out in practice and
// avoids surprising recursion into unrelated trees.
func LoadDir(root string) (*Library, error) {
	lib := NewLibrary()
	if err := lib.LoadDir(root); err != nil {
		return nil, err
	}
	return lib, nil
}

// LoadDir adds every skill under root to the library.
func (l *Library) LoadDir(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("reading skills directory: %w", err)
	}

	var failures []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(root, entry.Name())
		manifest := filepath.Join(dir, "SKILL.md")
		if _, err := os.Stat(manifest); err != nil {
			continue // not a skill directory
		}

		s, err := LoadFile(manifest)
		if err != nil {
			// One malformed skill should not make the rest unavailable, but the
			// failure must be reported rather than swallowed.
			failures = append(failures, fmt.Sprintf("%s: %v", entry.Name(), err))
			continue
		}
		if err := l.Add(s); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", entry.Name(), err))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("some skills failed to load: %s", strings.Join(failures, "; "))
	}
	return nil
}

// LoadFile loads a single SKILL.md.
func LoadFile(path string) (*Skill, error) {
	// #nosec G304 -- path comes from a directory walk the operator pointed at,
	// or from an explicit caller argument.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading skill: %w", err)
	}

	s, err := Parse(string(data))
	if err != nil {
		return nil, err
	}

	s.Dir = filepath.Dir(path)
	s.Resources, err = listResources(s.Dir)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Parse reads a SKILL.md's frontmatter and body.
func Parse(content string) (*Skill, error) {
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var s Skill
	if err := yaml.Unmarshal([]byte(frontmatter), &s); err != nil {
		return nil, fmt.Errorf("parsing skill frontmatter: %w", err)
	}

	s.Instructions = strings.TrimSpace(body)
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// splitFrontmatter separates a leading `---` delimited YAML block from the body.
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	trimmed := strings.TrimLeft(content, " \t\r\n")
	if !strings.HasPrefix(trimmed, "---") {
		return "", "", fmt.Errorf("skill is missing YAML frontmatter: expected a leading '---' block")
	}

	rest := strings.TrimPrefix(trimmed, "---")
	rest = strings.TrimLeft(rest, "\r\n")

	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("skill frontmatter is not closed: expected a trailing '---'")
	}

	frontmatter = rest[:idx]
	body = rest[idx+len("\n---"):]
	body = strings.TrimLeft(body, "-")
	return frontmatter, body, nil
}

// listResources returns the bundled files in a skill directory, excluding the
// manifest itself.
func listResources(dir string) ([]string, error) {
	var resources []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		if rel == "SKILL.md" {
			return nil
		}
		resources = append(resources, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing skill resources: %w", err)
	}

	sort.Strings(resources)
	return resources, nil
}

// ReadResource returns a bundled file's contents.
//
// The path is confined to the skill's directory: a resource name that escapes
// it, by traversal or symlink, is refused. A skill folder arrives by git clone,
// so treating its contents as trusted paths would let one read arbitrary files
// off the host.
func (s *Skill) ReadResource(name string) (string, error) {
	if s.Dir == "" {
		return "", fmt.Errorf("skill %q has no directory", s.Name)
	}

	root, err := filepath.Abs(s.Dir)
	if err != nil {
		return "", err
	}
	// Resolve the ROOT too, not just the target. On macOS a temp directory is
	// itself a symlink (/var -> /private/var), so comparing a resolved target
	// against an unresolved root rejects perfectly legitimate paths. This is the
	// same defect that makes prompts.isPathSafe unsound in the other direction.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	target, err := filepath.Abs(filepath.Join(root, name))
	if err != nil {
		return "", err
	}

	// Resolve symlinks before the containment check, or a link inside the
	// directory can point anywhere.
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	}

	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resource %q is outside skill %q", name, s.Name)
	}

	// #nosec G304 -- containment verified immediately above.
	data, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("reading resource %q: %w", name, err)
	}
	return string(data), nil
}
