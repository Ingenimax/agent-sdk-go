package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage"
)

var pngImage = &interfaces.GeneratedImage{Data: []byte("fake png bytes"), MimeType: "image/png"}

func newStorage(t *testing.T, baseURL string) (*Storage, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := NewWithOptions(WithPath(dir), WithBaseURL(baseURL))
	require.NoError(t, err)
	return s, dir
}

func TestInitRegistersFactory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "imgs")

	s, err := storage.NewStorageFromConfig(storage.Config{Type: "local", Local: storage.LocalConfig{Path: dir}})

	require.NoError(t, err)
	assert.Equal(t, "local", s.Name())
}

func TestNew(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")

	s, err := New(storage.LocalConfig{Path: dir, BaseURL: "http://host/img/"})

	require.NoError(t, err)
	ls := s.(*Storage)
	assert.Equal(t, dir, ls.basePath)
	assert.Equal(t, "http://host/img", ls.baseURL)
	assert.DirExists(t, dir)
}

func TestNewFailsWhenPathIsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0600))

	_, err := New(storage.LocalConfig{Path: filepath.Join(file, "sub")})
	assert.ErrorContains(t, err, "failed to create storage directory")

	_, err = NewWithOptions(WithPath(filepath.Join(file, "sub")))
	assert.ErrorContains(t, err, "failed to create storage directory")
}

func TestName(t *testing.T) {
	s, _ := newStorage(t, "")
	assert.Equal(t, "local", s.Name())
}

func TestStoreReturnsFilePathWithoutBaseURL(t *testing.T) {
	s, dir := newStorage(t, "")
	ctx := context.Background()

	path, err := s.Store(ctx, pngImage, storage.StorageMetadata{OrgID: "org/1", ThreadID: "../t:2"})

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "org_1", "__t_2"), filepath.Dir(path))
	assert.True(t, strings.HasSuffix(path, "_"+hashData(pngImage.Data)[:12]+".png"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, pngImage.Data, data)

	got, err := s.Get(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, pngImage.Data, got)
}

func TestStoreReturnsURLWithBaseURL(t *testing.T) {
	s, dir := newStorage(t, "http://host/img/")
	ctx := context.Background()
	img := &interfaces.GeneratedImage{Data: []byte("jpeg"), MimeType: "image/jpeg"}

	url, err := s.Store(ctx, img, storage.StorageMetadata{OrgID: "org", ThreadID: "thread"})

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(url, "http://host/img/org/thread/"), url)
	assert.True(t, strings.HasSuffix(url, ".jpg"), url)

	// The URL maps back to a file inside the base directory.
	rel := strings.TrimPrefix(url, "http://host/img/")
	assert.FileExists(t, filepath.Join(dir, filepath.FromSlash(rel)))

	got, err := s.Get(ctx, url)
	require.NoError(t, err)
	assert.Equal(t, img.Data, got)

	require.NoError(t, s.Delete(ctx, url))
	assert.NoFileExists(t, filepath.Join(dir, filepath.FromSlash(rel)))
}

func TestStoreWithoutMetadataUsesBaseDir(t *testing.T) {
	s, dir := newStorage(t, "")

	path, err := s.Store(context.Background(), pngImage, storage.StorageMetadata{})

	require.NoError(t, err)
	assert.Equal(t, dir, filepath.Dir(path))
}

func TestStoreRejectsEmptyImage(t *testing.T) {
	s, _ := newStorage(t, "")

	for _, img := range []*interfaces.GeneratedImage{nil, {MimeType: "image/png"}} {
		_, err := s.Store(context.Background(), img, storage.StorageMetadata{})
		assert.EqualError(t, err, "image data is empty")
	}
}

func TestStoreFailsWhenDirectoryCannotBeCreated(t *testing.T) {
	s, dir := newStorage(t, "")
	// A file where the org directory should go blocks MkdirAll.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "org"), nil, 0600))

	_, err := s.Store(context.Background(), pngImage, storage.StorageMetadata{OrgID: "org"})

	assert.ErrorContains(t, err, "failed to create directory")
}

func TestStoreFailsWhenFileCannotBeWritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	s, dir := newStorage(t, "")
	require.NoError(t, os.Chmod(dir, 0500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	_, err := s.Store(context.Background(), pngImage, storage.StorageMetadata{})

	assert.ErrorContains(t, err, "failed to write image file")
}

func TestGetAndDeleteRelativePath(t *testing.T) {
	s, dir := newStorage(t, "")
	ctx := context.Background()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.png"), []byte("x"), 0600))

	got, err := s.Get(ctx, "a.png")
	require.NoError(t, err)
	assert.Equal(t, []byte("x"), got)

	require.NoError(t, s.Delete(ctx, "a.png"))
	assert.NoFileExists(t, filepath.Join(dir, "a.png"))

	// Deleting a missing file is not an error.
	assert.NoError(t, s.Delete(ctx, "a.png"))
}

func TestGetMissingFile(t *testing.T) {
	s, _ := newStorage(t, "")

	_, err := s.Get(context.Background(), "missing.png")

	assert.ErrorContains(t, err, "failed to read image file")
}

func TestUrlToFilePath(t *testing.T) {
	s := &Storage{basePath: "/base", baseURL: "http://host/img"}

	tests := []struct {
		in   string
		want string
	}{
		{"/abs/file.png", "/abs/file.png"},
		{"http://host/img/org/file.png", "/base/org/file.png"},
		{"org/file.png", "/base/org/file.png"},
		{"", "/base"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, s.urlToFilePath(tt.in), tt.in)
	}
}

func TestGetExtension(t *testing.T) {
	tests := map[string]string{
		"image/png":  ".png",
		"image/jpeg": ".jpg",
		"image/jpg":  ".jpg",
		"image/gif":  ".gif",
		"image/webp": ".webp",
		"image/bmp":  ".png",
		"":           ".png",
	}
	for mime, want := range tests {
		assert.Equal(t, want, getExtension(mime), mime)
	}
}

func TestHashData(t *testing.T) {
	// SHA-256 of "abc".
	assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", hashData([]byte("abc")))
}

func TestSanitizePath(t *testing.T) {
	assert.Equal(t, "__a_b_c_d", sanitizePath(`../a/b\c:d`))
	assert.Equal(t, "plain-id", sanitizePath("plain-id"))
}
