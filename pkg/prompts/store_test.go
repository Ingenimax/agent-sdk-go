package prompts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaults(t *testing.T) {
	tmpl := New("greet", "Greeting", "Hello")
	assert.Equal(t, "greet", tmpl.ID)
	assert.Equal(t, "Greeting", tmpl.Name)
	assert.Equal(t, "Hello", tmpl.Content)
	assert.Equal(t, "1.0.0", tmpl.Version)
	assert.Equal(t, GoTemplate, tmpl.Format)
	assert.Empty(t, tmpl.Tags)
	assert.Empty(t, tmpl.Metadata)
	assert.False(t, tmpl.CreatedAt.IsZero())
	assert.Equal(t, tmpl.CreatedAt, tmpl.UpdatedAt)
}

func TestNewWithOptions(t *testing.T) {
	meta := map[string]interface{}{"owner": "team"}
	tmpl := New("greet", "Greeting", "Hello",
		WithVersion("2.0.0"),
		WithDescription("says hello"),
		WithTags("a", "b"),
		WithMetadata(meta),
		WithFormat(HandlebarsTemplate),
	)
	assert.Equal(t, "2.0.0", tmpl.Version)
	assert.Equal(t, "says hello", tmpl.Description)
	assert.Equal(t, []string{"a", "b"}, tmpl.Tags)
	assert.Equal(t, meta, tmpl.Metadata)
	assert.Equal(t, HandlebarsTemplate, tmpl.Format)
}

func TestTemplateRender(t *testing.T) {
	tests := []struct {
		name    string
		content string
		data    map[string]interface{}
		want    string
		wantErr string
	}{
		{"plain", "Hello", nil, "Hello", ""},
		{"variable", "Hello {{.Name}}", map[string]interface{}{"Name": "Ana"}, "Hello Ana", ""},
		{"parse error", "Hello {{.Name", nil, "", "failed to parse template"},
		{"execute error", "{{template \"missing\"}}", nil, "", "failed to render template"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New("id", "n", tt.content).Render(tt.data)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTemplateRenderCachesParsedTemplate(t *testing.T) {
	tmpl := New("id", "n", "{{.X}}")
	got, err := tmpl.Render(map[string]interface{}{"X": 1})
	require.NoError(t, err)
	assert.Equal(t, "1", got)

	// Changing Content after the first render has no effect: the parse is cached.
	tmpl.Content = "changed"
	got, err = tmpl.Render(map[string]interface{}{"X": 2})
	require.NoError(t, err)
	assert.Equal(t, "2", got)
}

func TestParseTemplateFile(t *testing.T) {
	data := "name: Greeting\n" +
		"description: says hello\n" +
		"format: handlebars\n" +
		"tags: a , b,c\n" +
		"owner: team\n" +
		"no colon line\n" +
		"\n" +
		"---\n" +
		"Hello\n---\nWorld"

	tmpl, err := parseTemplateFile(data, "greet", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "greet", tmpl.ID)
	assert.Equal(t, "1.0.0", tmpl.Version)
	assert.Equal(t, "Greeting", tmpl.Name)
	assert.Equal(t, "says hello", tmpl.Description)
	assert.Equal(t, HandlebarsTemplate, tmpl.Format)
	assert.Equal(t, []string{"a", "b", "c"}, tmpl.Tags)
	assert.Equal(t, map[string]interface{}{"owner": "team"}, tmpl.Metadata)
	// Later separators are part of the content.
	assert.Equal(t, "Hello\n---\nWorld", tmpl.Content)
}

func TestParseTemplateFileInvalid(t *testing.T) {
	_, err := parseTemplateFile("no separator here", "id", "v")
	require.ErrorContains(t, err, "invalid template file format")
}

func TestSerializeTemplateRoundTrip(t *testing.T) {
	orig := New("greet", "Greeting", "Hello {{.Name}}",
		WithDescription("desc"),
		WithTags("x", "y"),
		WithMetadata(map[string]interface{}{"owner": "team"}),
	)

	parsed, err := parseTemplateFile(serializeTemplate(orig), orig.ID, orig.Version)
	require.NoError(t, err)
	assert.Equal(t, orig.Name, parsed.Name)
	assert.Equal(t, orig.Description, parsed.Description)
	assert.Equal(t, orig.Format, parsed.Format)
	assert.Equal(t, orig.Tags, parsed.Tags)
	assert.Equal(t, orig.Content, parsed.Content)
	assert.Equal(t, "team", parsed.Metadata["owner"])
}

func TestSerializeTemplateOmitsEmptyTags(t *testing.T) {
	assert.NotContains(t, serializeTemplate(New("id", "n", "c")), "tags:")
}

func TestMatchesFilter(t *testing.T) {
	tmpl := New("greet", "Greeting", "c",
		WithVersion("1.0.0"),
		WithTags("a", "b"),
		WithMetadata(map[string]interface{}{"owner": "team"}),
	)
	tests := []struct {
		name   string
		filter map[string]interface{}
		want   bool
	}{
		{"nil filter", nil, true},
		{"id match", map[string]interface{}{"id": "greet"}, true},
		{"id mismatch", map[string]interface{}{"id": "other"}, false},
		{"name match", map[string]interface{}{"name": "Greeting"}, true},
		{"name mismatch", map[string]interface{}{"name": "x"}, false},
		{"version match", map[string]interface{}{"version": "1.0.0"}, true},
		{"version mismatch", map[string]interface{}{"version": "2.0.0"}, false},
		{"tag match", map[string]interface{}{"tag": "b"}, true},
		{"tag mismatch", map[string]interface{}{"tag": "z"}, false},
		{"metadata match", map[string]interface{}{"owner": "team"}, true},
		{"metadata mismatch", map[string]interface{}{"owner": "other"}, false},
		{"metadata missing", map[string]interface{}{"missing": "x"}, false},
		{"all match", map[string]interface{}{"id": "greet", "tag": "a", "owner": "team"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, matchesFilter(tmpl, tt.filter))
		})
	}
}

func newTestStore(t *testing.T) *FileStore {
	t.Helper()
	store, err := NewFileStore(filepath.Join(t.TempDir(), "nested", "prompts"))
	require.NoError(t, err)
	return store
}

func TestNewFileStoreCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	_, err := NewFileStore(dir)
	require.NoError(t, err)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestNewFileStoreError(t *testing.T) {
	// A regular file blocks the directory from being created.
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	_, err := NewFileStore(filepath.Join(file, "sub"))
	require.ErrorContains(t, err, "failed to create directory")
}

func TestFileStoreSaveGetDelete(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	tmpl := New("greet", "Greeting", "Hello {{.Name}}", WithTags("t1"), WithDescription("d"))
	require.NoError(t, store.Save(ctx, tmpl))

	_, err := os.Stat(filepath.Join(store.basePath, "greet_1.0.0.tmpl"))
	require.NoError(t, err)

	got, err := store.Get(ctx, "greet", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "Greeting", got.Name)
	assert.Equal(t, "d", got.Description)
	assert.Equal(t, []string{"t1"}, got.Tags)
	assert.Equal(t, "Hello {{.Name}}", got.Content)

	require.NoError(t, store.Delete(ctx, "greet", "1.0.0"))
	_, err = store.Get(ctx, "greet", "1.0.0")
	require.ErrorContains(t, err, "failed to read template file")

	err = store.Delete(ctx, "greet", "1.0.0")
	require.ErrorContains(t, err, "failed to delete template file")
}

func TestFileStoreSaveSanitizesPath(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	tmpl := New("../../escape", "n", "c", WithVersion("../1.0"))
	require.NoError(t, store.Save(ctx, tmpl))
	assert.Equal(t, "escape", tmpl.ID)
	assert.Equal(t, "1.0", tmpl.Version)

	_, err := os.Stat(filepath.Join(store.basePath, "escape_1.0.tmpl"))
	require.NoError(t, err)

	// Get also strips directory parts from the id.
	got, err := store.Get(ctx, "../escape", "1.0")
	require.NoError(t, err)
	assert.Equal(t, "escape", got.ID)
}

func TestFileStoreGetInvalidFile(t *testing.T) {
	store := newTestStore(t)
	path := filepath.Join(store.basePath, "bad_1.tmpl")
	require.NoError(t, os.WriteFile(path, []byte("no separator"), 0o600))

	_, err := store.Get(context.Background(), "bad", "1")
	require.ErrorContains(t, err, "failed to parse template file")
}

func TestFileStoreList(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.Save(ctx, New("a", "A", "c", WithTags("x"))))
	require.NoError(t, store.Save(ctx, New("b", "B", "c", WithVersion("2.0"))))

	// Files that List must skip: wrong name shape, bad content, wrong extension.
	write := func(name, data string) {
		require.NoError(t, os.WriteFile(filepath.Join(store.basePath, name), []byte(data), 0o600))
	}
	write("too_many_parts.tmpl", "name: x\n---\nc")
	write("broken_1.tmpl", "no separator")
	write("other_1.txt", "name: x\n---\nc")

	all, err := store.List(ctx, nil)
	require.NoError(t, err)
	ids := make([]string, 0, len(all))
	for _, tmpl := range all {
		ids = append(ids, tmpl.ID)
	}
	assert.ElementsMatch(t, []string{"a", "b"}, ids)

	tagged, err := store.List(ctx, map[string]interface{}{"tag": "x"})
	require.NoError(t, err)
	require.Len(t, tagged, 1)
	assert.Equal(t, "a", tagged[0].ID)

	none, err := store.List(ctx, map[string]interface{}{"id": "zzz"})
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestFileStoreListSkipsUnreadableEntries(t *testing.T) {
	store := newTestStore(t)
	// A directory matching the glob cannot be read as a file.
	require.NoError(t, os.Mkdir(filepath.Join(store.basePath, "dir_1.tmpl"), 0o750))

	got, err := store.List(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestFileStoreListBadPattern(t *testing.T) {
	// A malformed glob pattern in the base path makes Glob fail.
	store := &FileStore{basePath: "[unclosed"}
	_, err := store.List(context.Background(), nil)
	require.ErrorContains(t, err, "failed to list template files")
}

func TestFileStoreSaveWriteError(t *testing.T) {
	store := &FileStore{basePath: filepath.Join(t.TempDir(), "missing")}
	err := store.Save(context.Background(), New("a", "A", "c"))
	require.ErrorContains(t, err, "failed to write template file")
}

// memStore is an in-memory TemplateStore for Manager tests.
type memStore struct {
	templates []*Template
	listErr   error
}

func (m *memStore) Get(_ context.Context, id, version string) (*Template, error) {
	for _, tmpl := range m.templates {
		if tmpl.ID == id && tmpl.Version == version {
			return tmpl, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *memStore) List(_ context.Context, filter map[string]interface{}) ([]*Template, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var out []*Template
	for _, tmpl := range m.templates {
		if matchesFilter(tmpl, filter) {
			out = append(out, tmpl)
		}
	}
	return out, nil
}

func (m *memStore) Save(_ context.Context, tmpl *Template) error {
	m.templates = append(m.templates, tmpl)
	return nil
}

func (m *memStore) Delete(_ context.Context, id, version string) error {
	for i, tmpl := range m.templates {
		if tmpl.ID == id && tmpl.Version == version {
			m.templates = append(m.templates[:i], m.templates[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func TestManager(t *testing.T) {
	ctx := context.Background()
	store := &memStore{}
	m := NewManager(store)

	require.NoError(t, m.Save(ctx, New("greet", "G", "v1 {{.N}}", WithVersion("1.0.0"))))
	require.NoError(t, m.Save(ctx, New("greet", "G", "v3 {{.N}}", WithVersion("3.0.0"))))
	require.NoError(t, m.Save(ctx, New("greet", "G", "v2 {{.N}}", WithVersion("2.0.0"))))
	require.NoError(t, m.Save(ctx, New("other", "O", "x")))

	got, err := m.Get(ctx, "greet", "2.0.0")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", got.Version)

	latest, err := m.GetLatest(ctx, "greet")
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", latest.Version)

	list, err := m.List(ctx, map[string]interface{}{"id": "greet"})
	require.NoError(t, err)
	assert.Len(t, list, 3)

	out, err := m.Render(ctx, "greet", "1.0.0", map[string]interface{}{"N": "a"})
	require.NoError(t, err)
	assert.Equal(t, "v1 a", out)

	out, err = m.RenderLatest(ctx, "greet", map[string]interface{}{"N": "b"})
	require.NoError(t, err)
	assert.Equal(t, "v3 b", out)

	require.NoError(t, m.Delete(ctx, "greet", "3.0.0"))
	latest, err = m.GetLatest(ctx, "greet")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", latest.Version)
}

func TestManagerErrors(t *testing.T) {
	ctx := context.Background()

	m := NewManager(&memStore{})
	_, err := m.GetLatest(ctx, "missing")
	require.ErrorContains(t, err, "template not found: missing")

	_, err = m.Render(ctx, "missing", "1", nil)
	require.Error(t, err)

	_, err = m.RenderLatest(ctx, "missing", nil)
	require.ErrorContains(t, err, "template not found")

	listErr := errors.New("list failed")
	m = NewManager(&memStore{listErr: listErr})
	_, err = m.GetLatest(ctx, "x")
	require.ErrorIs(t, err, listErr)
}

func TestManagerWithFileStore(t *testing.T) {
	ctx := context.Background()
	m := NewManager(newTestStore(t))
	require.NoError(t, m.Save(ctx, New("hi", "Hi", "Hi {{.Who}}")))

	out, err := m.RenderLatest(ctx, "hi", map[string]interface{}{"Who": "Bo"})
	require.NoError(t, err)
	assert.Equal(t, "Hi Bo", out)
}
