package prompts

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// TemplateFormat represents the format of a template
type TemplateFormat string

const (
	// GoTemplate uses Go's text/template package
	GoTemplate TemplateFormat = "go_template"

	// HandlebarsTemplate uses handlebars-style templates
	HandlebarsTemplate TemplateFormat = "handlebars"
)

// Template represents a prompt template
type Template struct {
	ID          string
	Name        string
	Description string
	Content     string
	Version     string
	Format      TemplateFormat
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Tags        []string
	Metadata    map[string]interface{}

	// Parsed template (cached)
	parsed *template.Template
}

// TemplateStore is an interface for storing and retrieving templates
type TemplateStore interface {
	// Get retrieves a template by ID and version
	Get(ctx context.Context, id string, version string) (*Template, error)

	// List returns all templates matching the given filter
	List(ctx context.Context, filter map[string]interface{}) ([]*Template, error)

	// Save stores a template
	Save(ctx context.Context, tmpl *Template) error

	// Delete removes a template
	Delete(ctx context.Context, id string, version string) error
}

// TemplateOption is a function that configures a template
type TemplateOption func(*Template)

// WithVersion sets the template version
func WithVersion(version string) TemplateOption {
	return func(t *Template) {
		t.Version = version
	}
}

// WithDescription sets the template description
func WithDescription(description string) TemplateOption {
	return func(t *Template) {
		t.Description = description
	}
}

// WithTags sets the template tags
func WithTags(tags ...string) TemplateOption {
	return func(t *Template) {
		t.Tags = tags
	}
}

// WithMetadata sets the template metadata
func WithMetadata(metadata map[string]interface{}) TemplateOption {
	return func(t *Template) {
		t.Metadata = metadata
	}
}

// WithFormat sets the template format
func WithFormat(format TemplateFormat) TemplateOption {
	return func(t *Template) {
		t.Format = format
	}
}

// New creates a new template
func New(id string, name string, content string, options ...TemplateOption) *Template {
	now := time.Now()

	tmpl := &Template{
		ID:        id,
		Name:      name,
		Content:   content,
		Version:   "1.0.0",
		Format:    GoTemplate,
		CreatedAt: now,
		UpdatedAt: now,
		Tags:      []string{},
		Metadata:  map[string]interface{}{},
	}

	for _, option := range options {
		option(tmpl)
	}

	return tmpl
}

// Render renders the template with the given data
func (t *Template) Render(data map[string]interface{}) (string, error) {
	var err error

	// Parse template if not already parsed
	if t.parsed == nil {
		t.parsed, err = template.New(t.ID).Parse(t.Content)
		if err != nil {
			return "", fmt.Errorf("failed to parse template: %w", err)
		}
	}

	// Render template
	var buf bytes.Buffer
	err = t.parsed.Execute(&buf, data)
	if err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return buf.String(), nil
}

// FileStore implements TemplateStore using the local file system
type FileStore struct {
	basePath string
}

// NewFileStore creates a new file store
func NewFileStore(basePath string) (*FileStore, error) {
	// Create directory if it doesn't exist
	err := os.MkdirAll(basePath, 0750)
	if err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	return &FileStore{
		basePath: basePath,
	}, nil
}

// Get retrieves a template by ID and version
func (s *FileStore) Get(ctx context.Context, id string, version string) (*Template, error) {
	// Sanitize id and version to prevent path traversal
	id = filepath.Base(id)
	version = filepath.Base(version)

	// Construct file path
	filePath := filepath.Join(s.basePath, fmt.Sprintf("%s_%s.tmpl", id, version))

	// Ensure the file is within the basePath
	absBasePath, err := filepath.Abs(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute base path: %w", err)
	}

	if !isPathSafe(filePath, absBasePath) {
		return nil, fmt.Errorf("invalid template path")
	}

	// Read file
	data, err := ioutil.ReadFile(filePath) // #nosec G304 - Path is validated with isPathSafe() before use
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	// Parse template
	tmpl, err := parseTemplateFile(string(data), id, version)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template file: %w", err)
	}

	return tmpl, nil
}

// List returns all templates matching the given filter
func (s *FileStore) List(ctx context.Context, filter map[string]interface{}) ([]*Template, error) {
	// Get all template files
	pattern := filepath.Join(s.basePath, "*.tmpl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list template files: %w", err)
	}

	// Get absolute base path for validation
	absBasePath, err := filepath.Abs(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute base path: %w", err)
	}

	// Parse each file
	var templates []*Template
	for _, file := range files {
		// Verify file is within basePath
		if !isPathSafe(file, absBasePath) {
			continue
		}

		// Extract ID and version from filename
		filename := filepath.Base(file)
		parts := strings.Split(strings.TrimSuffix(filename, ".tmpl"), "_")
		if len(parts) != 2 {
			continue
		}

		id := parts[0]
		version := parts[1]

		// Read file
		data, err := ioutil.ReadFile(file) // #nosec G304 - Path is validated with isPathSafe() before use
		if err != nil {
			continue
		}

		// Parse template
		tmpl, err := parseTemplateFile(string(data), id, version)
		if err != nil {
			continue
		}

		// Apply filter
		if matchesFilter(tmpl, filter) {
			templates = append(templates, tmpl)
		}
	}

	return templates, nil
}

// Save stores a template
func (s *FileStore) Save(ctx context.Context, tmpl *Template) error {
	// Sanitize id and version to prevent path traversal
	tmpl.ID = filepath.Base(tmpl.ID)
	tmpl.Version = filepath.Base(tmpl.Version)

	// Update timestamp
	tmpl.UpdatedAt = time.Now()

	// Serialize template
	data := serializeTemplate(tmpl)

	// Construct file path
	filePath := filepath.Join(s.basePath, fmt.Sprintf("%s_%s.tmpl", tmpl.ID, tmpl.Version))

	// Ensure the file is within the basePath
	absBasePath, err := filepath.Abs(s.basePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute base path: %w", err)
	}

	if !isPathSafe(filePath, absBasePath) {
		return fmt.Errorf("invalid template path")
	}

	// Write file with secure permissions
	err = ioutil.WriteFile(filePath, []byte(data), 0600)
	if err != nil {
		return fmt.Errorf("failed to write template file: %w", err)
	}

	return nil
}

// Delete removes a template
func (s *FileStore) Delete(ctx context.Context, id string, version string) error {
	// Sanitize id and version to prevent path traversal
	id = filepath.Base(id)
	version = filepath.Base(version)

	// Construct file path
	filePath := filepath.Join(s.basePath, fmt.Sprintf("%s_%s.tmpl", id, version))

	// Ensure the file is within the basePath
	absBasePath, err := filepath.Abs(s.basePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute base path: %w", err)
	}

	if !isPathSafe(filePath, absBasePath) {
		return fmt.Errorf("invalid template path")
	}

	// Delete file
	err = os.Remove(filePath)
	if err != nil {
		return fmt.Errorf("failed to delete template file: %w", err)
	}

	return nil
}

// parseTemplateFile parses a template file
func parseTemplateFile(data string, id string, version string) (*Template, error) {
	// Split into sections
	sections := strings.Split(data, "---\n")
	if len(sections) < 2 {
		return nil, fmt.Errorf("invalid template file format")
	}

	// Parse metadata
	metadata := sections[0]
	content := strings.Join(sections[1:], "---\n")

	// Parse metadata lines
	lines := strings.Split(metadata, "\n")
	tmpl := &Template{
		ID:        id,
		Version:   version,
		Content:   content,
		Format:    GoTemplate,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Tags:      []string{},
		Metadata:  map[string]interface{}{},
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "name":
			tmpl.Name = value
		case "description":
			tmpl.Description = value
		case "format":
			tmpl.Format = TemplateFormat(value)
		case "tags":
			tmpl.Tags = strings.Split(value, ",")
			for i, tag := range tmpl.Tags {
				tmpl.Tags[i] = strings.TrimSpace(tag)
			}
		default:
			tmpl.Metadata[key] = value
		}
	}

	return tmpl, nil
}

// serializeTemplate serializes a template to a string
func serializeTemplate(tmpl *Template) string {
	var buf bytes.Buffer

	// Write metadata
	buf.WriteString(fmt.Sprintf("name: %s\n", tmpl.Name))
	buf.WriteString(fmt.Sprintf("description: %s\n", tmpl.Description))
	buf.WriteString(fmt.Sprintf("format: %s\n", tmpl.Format))

	if len(tmpl.Tags) > 0 {
		buf.WriteString(fmt.Sprintf("tags: %s\n", strings.Join(tmpl.Tags, ", ")))
	}

	for key, value := range tmpl.Metadata {
		buf.WriteString(fmt.Sprintf("%s: %v\n", key, value))
	}

	// Write content
	buf.WriteString("---\n")
	buf.WriteString(tmpl.Content)

	return buf.String()
}

// matchesFilter checks if a template matches the given filter
func matchesFilter(tmpl *Template, filter map[string]interface{}) bool {
	for key, value := range filter {
		switch key {
		case "id":
			if tmpl.ID != value {
				return false
			}
		case "name":
			if tmpl.Name != value {
				return false
			}
		case "version":
			if tmpl.Version != value {
				return false
			}
		case "tag":
			found := false
			for _, tag := range tmpl.Tags {
				if tag == value {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		default:
			metaValue, ok := tmpl.Metadata[key]
			if !ok || metaValue != value {
				return false
			}
		}
	}

	return true
}

// Manager manages prompt templates
type Manager struct {
	store TemplateStore
}

// NewManager creates a new template manager
func NewManager(store TemplateStore) *Manager {
	return &Manager{
		store: store,
	}
}

// Get retrieves a template by ID and version
func (m *Manager) Get(ctx context.Context, id string, version string) (*Template, error) {
	return m.store.Get(ctx, id, version)
}

// GetLatest retrieves the latest version of a template by ID
func (m *Manager) GetLatest(ctx context.Context, id string) (*Template, error) {
	templates, err := m.store.List(ctx, map[string]interface{}{
		"id": id,
	})
	if err != nil {
		return nil, err
	}

	if len(templates) == 0 {
		return nil, fmt.Errorf("template not found: %s", id)
	}

	// Find the latest version
	latest := templates[0]
	for _, tmpl := range templates[1:] {
		if tmpl.Version > latest.Version {
			latest = tmpl
		}
	}

	return latest, nil
}

// List returns all templates matching the given filter
func (m *Manager) List(ctx context.Context, filter map[string]interface{}) ([]*Template, error) {
	return m.store.List(ctx, filter)
}

// Save stores a template
func (m *Manager) Save(ctx context.Context, tmpl *Template) error {
	return m.store.Save(ctx, tmpl)
}

// Delete removes a template
func (m *Manager) Delete(ctx context.Context, id string, version string) error {
	return m.store.Delete(ctx, id, version)
}

// Render renders a template with the given data
func (m *Manager) Render(ctx context.Context, id string, version string, data map[string]interface{}) (string, error) {
	tmpl, err := m.Get(ctx, id, version)
	if err != nil {
		return "", err
	}

	return tmpl.Render(data)
}

// RenderLatest renders the latest version of a template with the given data
func (m *Manager) RenderLatest(ctx context.Context, id string, data map[string]interface{}) (string, error) {
	tmpl, err := m.GetLatest(ctx, id)
	if err != nil {
		return "", err
	}

	return tmpl.Render(data)
}

// isPathSafe checks if a file path is safe to access
func isPathSafe(filePath string, basePath string) bool {
	// Get absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}

	// Ensure path is within base directory
	return strings.HasPrefix(absPath, basePath)
}
