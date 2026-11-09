package storage_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage"
)

// stubFactories replaces the factory hooks and restores them after the test.
func stubFactories(t *testing.T) (*storage.LocalConfig, *storage.GCSConfig) {
	t.Helper()
	origLocal, origGCS := storage.NewLocalStorage, storage.NewGCSStorage
	t.Cleanup(func() { storage.NewLocalStorage, storage.NewGCSStorage = origLocal, origGCS })

	var gotLocal storage.LocalConfig
	var gotGCS storage.GCSConfig
	storage.NewLocalStorage = func(cfg storage.LocalConfig) (storage.ImageStorage, error) {
		gotLocal = cfg
		return &testutil.FakeImageStorage{StorageName: "local"}, nil
	}
	storage.NewGCSStorage = func(cfg storage.GCSConfig) (storage.ImageStorage, error) {
		gotGCS = cfg
		return &testutil.FakeImageStorage{StorageName: "gcs"}, nil
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
			cfg := storage.Config{
				Type:  tt.typ,
				Local: storage.LocalConfig{Path: "/data"},
				GCS:   storage.GCSConfig{Bucket: "b"},
			}

			s, err := storage.NewStorageFromConfig(cfg)

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

	s, err := storage.NewStorageFromConfig(storage.Config{Type: "s3"})

	assert.Nil(t, s)
	assert.ErrorIs(t, err, interfaces.ErrStorageUploadFailed)
}

func TestNewStorageFromConfigPropagatesFactoryError(t *testing.T) {
	stubFactories(t)
	wantErr := errors.New("boom")
	storage.NewLocalStorage = func(storage.LocalConfig) (storage.ImageStorage, error) { return nil, wantErr }

	_, err := storage.NewStorageFromConfig(storage.Config{Type: "local"})

	assert.ErrorIs(t, err, wantErr)
}
