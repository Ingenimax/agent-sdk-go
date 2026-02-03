package storage

import (
	"context"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ImageStorage defines the interface for storing and retrieving generated images
type ImageStorage interface {
	// Store saves an image and returns an accessible URL
	Store(ctx context.Context, image *interfaces.GeneratedImage, metadata StorageMetadata) (string, error)

	// Delete removes an image by URL
	Delete(ctx context.Context, url string) error

	// Get retrieves image data by URL (optional, may not be supported by all backends)
	Get(ctx context.Context, url string) ([]byte, error)

	// Name returns the storage backend name
	Name() string
}

// StorageMetadata contains metadata for stored images
type StorageMetadata struct {
	// OrgID is the organization ID for multi-tenancy
	OrgID string

	// ThreadID is the conversation thread ID
	ThreadID string

	// MessageID is the message ID
	MessageID string

	// Prompt is the original prompt used to generate the image
	Prompt string

	// Tags contains custom tags for the image
	Tags map[string]string

	// CreatedAt is the timestamp when the image was created
	CreatedAt time.Time
}

// Config contains configuration for storage backends
type Config struct {
	// Type is the storage backend type ("local", "gcs")
	Type string

	// Local storage configuration
	Local LocalConfig

	// GCS storage configuration
	GCS GCSConfig
}

// LocalConfig contains configuration for local filesystem storage
type LocalConfig struct {
	// Path is the base directory for storing images
	Path string

	// BaseURL is the URL prefix for accessing stored images (optional)
	// If empty, file paths will be returned instead of URLs
	BaseURL string
}

// GCSConfig contains configuration for Google Cloud Storage
type GCSConfig struct {
	// Bucket is the GCS bucket name
	Bucket string

	// Prefix is the path prefix within the bucket
	Prefix string

	// CredentialsFile is the path to the service account JSON file (optional)
	// If empty, uses Application Default Credentials
	CredentialsFile string

	// CredentialsJSON is the service account JSON content (optional)
	// Can be raw JSON or base64 encoded. Takes precedence over CredentialsFile.
	CredentialsJSON string

	// SignedURLExpiration is the duration for signed URLs (default: 24h)
	SignedURLExpiration time.Duration

	// UseSignedURLs determines whether to return signed URLs or public URLs
	UseSignedURLs bool
}

// NewStorageFromConfig creates a storage backend from configuration
func NewStorageFromConfig(cfg Config) (ImageStorage, error) {
	switch cfg.Type {
	case "local", "":
		return NewLocalStorage(cfg.Local)
	case "gcs":
		return NewGCSStorage(cfg.GCS)
	default:
		return nil, interfaces.ErrStorageUploadFailed
	}
}

// NewLocalStorage creates a new local filesystem storage
// This is a placeholder that will be implemented in the local package
var NewLocalStorage func(cfg LocalConfig) (ImageStorage, error)

// NewGCSStorage creates a new GCS storage
// This is a placeholder that will be implemented in the gcs package
var NewGCSStorage func(cfg GCSConfig) (ImageStorage, error)
