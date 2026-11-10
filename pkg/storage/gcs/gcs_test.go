package gcs

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	imgstorage "github.com/Ingenimax/agent-sdk-go/pkg/storage"
)

const testBucket = "test-bucket"

// storedObject is one object held by fakeGCS.
type storedObject struct {
	data        []byte
	contentType string
	metadata    map[string]string
}

// fakeGCS is an in-memory stand-in for the parts of the GCS API the
// storage client uses: multipart upload, media download, and delete.
type fakeGCS struct {
	mu      sync.Mutex
	objects map[string]storedObject
	// failStatus, when set, is returned for every request.
	failStatus int
}

func (f *fakeGCS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.failStatus != 0 {
		writeError(w, f.failStatus)
		return
	}

	uploadPrefix := "/upload/storage/v1/b/" + testBucket + "/o"
	jsonPrefix := "/storage/v1/b/" + testBucket + "/o/"
	xmlPrefix := "/" + testBucket + "/"

	switch {
	case r.Method == http.MethodPost && r.URL.Path == uploadPrefix:
		f.upload(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, jsonPrefix):
		name := strings.TrimPrefix(r.URL.Path, jsonPrefix)
		if _, ok := f.objects[name]; !ok {
			writeError(w, http.StatusNotFound)
			return
		}
		delete(f.objects, name)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, jsonPrefix):
		f.download(w, strings.TrimPrefix(r.URL.Path, jsonPrefix))
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, xmlPrefix):
		f.download(w, strings.TrimPrefix(r.URL.Path, xmlPrefix))
	default:
		http.Error(w, "unexpected request "+r.Method+" "+r.URL.String(), http.StatusBadRequest)
	}
}

func (f *fakeGCS) upload(w http.ResponseWriter, r *http.Request) {
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mr := multipart.NewReader(r.Body, params["boundary"])

	// The first part is the JSON object resource, the second the media.
	var attrs struct {
		Name        string            `json:"name"`
		ContentType string            `json:"contentType"`
		Metadata    map[string]string `json:"metadata"`
	}
	part, err := mr.NextPart()
	if err == nil {
		err = json.NewDecoder(part).Decode(&attrs)
	}
	if err == nil {
		part, err = mr.NextPart()
	}
	var data []byte
	if err == nil {
		data, err = io.ReadAll(part)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.objects[attrs.Name] = storedObject{data: data, contentType: attrs.ContentType, metadata: attrs.Metadata}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"bucket":      testBucket,
		"name":        attrs.Name,
		"contentType": attrs.ContentType,
		"metadata":    attrs.Metadata,
		"size":        strconv.Itoa(len(data)),
	})
}

func (f *fakeGCS) download(w http.ResponseWriter, name string) {
	obj, ok := f.objects[name]
	if !ok {
		writeError(w, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", obj.contentType)
	_, _ = w.Write(obj.data)
}

func writeError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"code": status, "message": http.StatusText(status)},
	})
}

// newFakeStorage starts a fake GCS server and builds a Storage through New,
// pointed at the server with STORAGE_EMULATOR_HOST.
func newFakeStorage(t *testing.T, cfg imgstorage.GCSConfig) (*Storage, *fakeGCS) {
	t.Helper()
	fake := &fakeGCS{objects: map[string]storedObject{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	t.Setenv("STORAGE_EMULATOR_HOST", srv.URL)

	cfg.Bucket = testBucket
	s, err := New(cfg)
	require.NoError(t, err)
	gs := s.(*Storage)
	t.Cleanup(func() { _ = gs.client.Close() })
	return gs, fake
}

func TestInitRegistersFactory(t *testing.T) {
	t.Setenv("STORAGE_EMULATOR_HOST", "http://127.0.0.1:1")

	s, err := imgstorage.NewStorageFromConfig(imgstorage.Config{Type: "gcs", GCS: imgstorage.GCSConfig{Bucket: testBucket}})

	require.NoError(t, err)
	t.Cleanup(func() { _ = s.(*Storage).client.Close() })
	assert.Equal(t, "gcs", s.Name())
}

func TestNewDefaults(t *testing.T) {
	s, _ := newFakeStorage(t, imgstorage.GCSConfig{Prefix: "images/"})

	assert.Equal(t, testBucket, s.bucket)
	assert.Equal(t, "images", s.prefix)
	assert.Equal(t, 24*time.Hour, s.signedURLExpiration)
	assert.False(t, s.useSignedURLs)
}

func TestNewKeepsSignedURLSettings(t *testing.T) {
	s, _ := newFakeStorage(t, imgstorage.GCSConfig{SignedURLExpiration: time.Minute, UseSignedURLs: true})

	assert.Equal(t, time.Minute, s.signedURLExpiration)
	assert.True(t, s.useSignedURLs)
}

func TestNewRequiresBucket(t *testing.T) {
	t.Setenv("STORAGE_EMULATOR_HOST", "http://127.0.0.1:1")

	_, err := New(imgstorage.GCSConfig{})

	assert.EqualError(t, err, "GCS bucket name is required")
}

func TestNewCredentialErrors(t *testing.T) {
	// Without the emulator variable, the client must load the given credentials.
	t.Setenv("STORAGE_EMULATOR_HOST", "")

	tests := []struct {
		name string
		cfg  imgstorage.GCSConfig
	}{
		{"missing credentials file", imgstorage.GCSConfig{Bucket: testBucket, CredentialsFile: filepath.Join(t.TempDir(), "missing.json")}},
		{"invalid credentials JSON", imgstorage.GCSConfig{Bucket: testBucket, CredentialsJSON: "not json"}},
		{"invalid base64 credentials JSON", imgstorage.GCSConfig{Bucket: testBucket, CredentialsJSON: base64.StdEncoding.EncodeToString([]byte("{bad"))}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg)
			assert.ErrorContains(t, err, "failed to create GCS client")
		})
	}
}

func TestStoreGetDelete(t *testing.T) {
	s, fake := newFakeStorage(t, imgstorage.GCSConfig{Prefix: "images"})
	ctx := context.Background()
	img := &interfaces.GeneratedImage{Data: []byte("jpeg bytes"), MimeType: "image/jpeg"}
	meta := imgstorage.StorageMetadata{
		OrgID:     "org/1",
		ThreadID:  "thread:2",
		MessageID: "msg-3",
		Prompt:    strings.Repeat("p", 600),
	}

	url, err := s.Store(ctx, img, meta)
	require.NoError(t, err)

	wantPrefix := "https://storage.googleapis.com/" + testBucket + "/images/org_1/thread_2/"
	require.True(t, strings.HasPrefix(url, wantPrefix), url)
	assert.True(t, strings.HasSuffix(url, "_"+hashData(img.Data)[:12]+".jpg"), url)

	name := strings.TrimPrefix(url, "https://storage.googleapis.com/"+testBucket+"/")
	require.Contains(t, fake.objects, name)
	obj := fake.objects[name]
	assert.Equal(t, img.Data, obj.data)
	assert.Equal(t, "image/jpeg", obj.contentType)
	assert.Equal(t, map[string]string{
		"prompt":     strings.Repeat("p", 500),
		"org_id":     "org/1",
		"thread_id":  "thread:2",
		"message_id": "msg-3",
	}, obj.metadata)

	got, err := s.Get(ctx, url)
	require.NoError(t, err)
	assert.Equal(t, img.Data, got)

	// A plain object path works the same as the public URL.
	got, err = s.Get(ctx, name)
	require.NoError(t, err)
	assert.Equal(t, img.Data, got)

	require.NoError(t, s.Delete(ctx, url))
	assert.NotContains(t, fake.objects, name)

	// Deleting an object that no longer exists is not an error.
	assert.NoError(t, s.Delete(ctx, url))
}

func TestStoreWithoutPrefixOrMetadata(t *testing.T) {
	s, fake := newFakeStorage(t, imgstorage.GCSConfig{})

	url, err := s.Store(context.Background(), &interfaces.GeneratedImage{Data: []byte("x"), MimeType: "image/png"}, imgstorage.StorageMetadata{Prompt: "cat"})

	require.NoError(t, err)
	name := strings.TrimPrefix(url, "https://storage.googleapis.com/"+testBucket+"/")
	assert.NotContains(t, name, "/")
	assert.Equal(t, map[string]string{"prompt": "cat"}, fake.objects[name].metadata)
}

func TestStoreWithSignedURLsUploadsObject(t *testing.T) {
	// The emulator client cannot sign, so this only checks the upload.
	// TestGenerateSignedURL covers signing.
	s, fake := newFakeStorage(t, imgstorage.GCSConfig{UseSignedURLs: true})

	url, err := s.Store(context.Background(), &interfaces.GeneratedImage{Data: []byte("x"), MimeType: "image/png"}, imgstorage.StorageMetadata{})

	require.Len(t, fake.objects, 1)
	if err == nil {
		assert.Contains(t, fake.objects, s.urlToObjectPath(url))
	}
}

func TestStoreRejectsEmptyImage(t *testing.T) {
	s := &Storage{bucket: testBucket}

	for _, img := range []*interfaces.GeneratedImage{nil, {MimeType: "image/png"}} {
		_, err := s.Store(context.Background(), img, imgstorage.StorageMetadata{})
		assert.EqualError(t, err, "image data is empty")
	}
}

func TestStoreUploadFailure(t *testing.T) {
	s, fake := newFakeStorage(t, imgstorage.GCSConfig{})
	fake.failStatus = http.StatusForbidden

	_, err := s.Store(context.Background(), &interfaces.GeneratedImage{Data: []byte("x"), MimeType: "image/png"}, imgstorage.StorageMetadata{})

	assert.ErrorContains(t, err, "failed to close GCS writer")
}

func TestGetMissingObject(t *testing.T) {
	s, _ := newFakeStorage(t, imgstorage.GCSConfig{})

	_, err := s.Get(context.Background(), "missing.png")

	assert.ErrorContains(t, err, "failed to read from GCS")
	assert.ErrorIs(t, err, storage.ErrObjectNotExist)
}

func TestDeleteFailure(t *testing.T) {
	s, fake := newFakeStorage(t, imgstorage.GCSConfig{})
	fake.failStatus = http.StatusForbidden

	err := s.Delete(context.Background(), "a.png")

	assert.ErrorContains(t, err, "failed to delete from GCS")
}

func TestGetAndDeleteRejectUnknownURL(t *testing.T) {
	s := &Storage{bucket: testBucket}
	ctx := context.Background()

	_, err := s.Get(ctx, "https://example.com/other/a.png")
	assert.EqualError(t, err, "invalid URL or object path")

	err = s.Delete(ctx, "https://example.com/other/a.png")
	assert.EqualError(t, err, "invalid URL or object path")
}

func TestGenerateSignedURL(t *testing.T) {
	// A service account key lets the client sign URLs locally, with no network.
	t.Setenv("STORAGE_EMULATOR_HOST", "")
	client, err := storage.NewClient(context.Background(), option.WithAuthCredentialsJSON(option.ServiceAccount, serviceAccountJSON(t)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	s := &Storage{client: client, bucket: testBucket, signedURLExpiration: time.Hour}

	url, err := s.generateSignedURL(context.Background(), "images/a.png")

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(url, "https://storage.googleapis.com/"+testBucket+"/images/a.png?"), url)
	assert.Contains(t, url, "X-Goog-Signature=")
	assert.Contains(t, url, "X-Goog-Expires=")

	// A signed URL maps back to its object path.
	assert.Equal(t, "images/a.png", s.urlToObjectPath(url))
}

// serviceAccountJSON builds service account credentials with a fresh key.
func serviceAccountJSON(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	data, err := json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     "test-project",
		"private_key_id": "key-1",
		"private_key":    string(pemKey),
		"client_email":   "signer@test-project.iam.gserviceaccount.com",
		"client_id":      "1",
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
	require.NoError(t, err)
	return data
}

func TestUrlToObjectPath(t *testing.T) {
	s := &Storage{bucket: testBucket}

	tests := []struct {
		in   string
		want string
	}{
		{"images/a.png", "images/a.png"},
		{"https://storage.googleapis.com/test-bucket/images/a.png", "images/a.png"},
		{"https://storage.googleapis.com/test-bucket/images/a.png?X-Goog-Signature=abc", "images/a.png"},
		{"https://test-bucket.storage.googleapis.com/x/test-bucket/images/a.png?sig=1", "images/a.png"},
		{"https://example.com/other/a.png", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, s.urlToObjectPath(tt.in), tt.in)
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
	}
	for mt, want := range tests {
		assert.Equal(t, want, getExtension(mt), mt)
	}
}

func TestHelpers(t *testing.T) {
	// SHA-256 of "abc".
	assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", hashData([]byte("abc")))

	assert.Equal(t, "__a_b_c_d", sanitizePath(`../a/b\c:d`))

	assert.Equal(t, "b", joinPath("", "b"))
	assert.Equal(t, "a", joinPath("a", ""))
	assert.Equal(t, "a/b", joinPath("a", "b"))

	assert.Equal(t, "abc", truncateString("abc", 5))
	assert.Equal(t, "abc", truncateString("abc", 3))
	assert.Equal(t, "ab", truncateString("abc", 2))
}

func TestParseCredentialsJSON(t *testing.T) {
	raw := `{"type":"service_account"}`

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"raw JSON", raw, raw},
		{"base64 JSON", base64.StdEncoding.EncodeToString([]byte(raw)), raw},
		{"base64 non-JSON stays encoded", base64.StdEncoding.EncodeToString([]byte("hello")), base64.StdEncoding.EncodeToString([]byte("hello"))},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseCredentialsJSON(tt.in))
		})
	}
}
