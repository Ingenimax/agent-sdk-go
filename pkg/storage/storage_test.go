package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// fakeStorage is a minimal ImageStorage that only reports its name.
type fakeStorage struct{ name string }

func (f fakeStorage) Store(context.Context, *interfaces.GeneratedImage, StorageMetadata) (string, error) {
	return "", nil
}
func (f fakeStorage) Delete(context.Context, string) error        { return nil }
func (f fakeStorage) Get(context.Context, string) ([]byte, error) { return nil, nil }
func (f fakeStorage) Name() string                                { return f.name }

// stubFactories replaces the factory hooks and restores them after the test.
func stubFactories(t *testing.T) (*LocalConfig, *GCSConfig) {
	t.Helper()
	origLocal, origGCS := NewLocalStorage, NewGCSStorage
	t.Cleanup(func() { NewLocalStorage, NewGCSStorage = origLocal, origGCS })

	var gotLocal LocalConfig
	var gotGCS GCSConfig
	NewLocalStorage = func(cfg LocalConfig) (ImageStorage, error) {
		gotLocal = cfg
		return fakeStorage{name: "local"}, nil
	}
	NewGCSStorage = func(cfg GCSConfig) (ImageStorage, error) {
		gotGCS = cfg
		return fakeStorage{name: "gcs"}, nil
	}
	return &gotLocal, &gotGCS
}

func TestNewStorageFromConfig(t *testing.T) {
	tests := []struct {
		typ      string
		wantName string
	}{
		{"", "local"},
		{"local", "local"},
		{"gcs", "gcs"},
	}

	for _, tt := range tests {
		t.Run("type="+tt.typ, func(t *testing.T) {
			gotLocal, gotGCS := stubFactories(t)
			cfg := Config{
				Type:  tt.typ,
				Local: LocalConfig{Path: "/data"},
				GCS:   GCSConfig{Bucket: "b"},
			}

			s, err := NewStorageFromConfig(cfg)

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, s.Name())
			if tt.wantName == "local" {
				assert.Equal(t, cfg.Local, *gotLocal)
			} else {
				assert.Equal(t, cfg.GCS, *gotGCS)
			}
		})
	}
}

func TestNewStorageFromConfigUnknownType(t *testing.T) {
	stubFactories(t)

	s, err := NewStorageFromConfig(Config{Type: "s3"})

	assert.Nil(t, s)
	assert.ErrorIs(t, err, interfaces.ErrStorageUploadFailed)
}

func TestNewStorageFromConfigPropagatesFactoryError(t *testing.T) {
	stubFactories(t)
	wantErr := errors.New("boom")
	NewLocalStorage = func(LocalConfig) (ImageStorage, error) { return nil, wantErr }

	_, err := NewStorageFromConfig(Config{Type: "local"})

	assert.ErrorIs(t, err, wantErr)
}
