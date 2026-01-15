package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage"
)

func init() {
	// Register the local storage factory
	storage.NewLocalStorage = New
}

// Storage implements ImageStorage for local filesystem
type Storage struct {
	basePath string
	baseURL  string
}

// Option represents an option for configuring local storage
type Option func(*Storage)

// WithPath sets the base path for storing images
func WithPath(path string) Option {
	return func(s *Storage) {
		s.basePath = path
	}
}

// WithBaseURL sets the base URL for accessing images
func WithBaseURL(url string) Option {
	return func(s *Storage) {
		s.baseURL = strings.TrimSuffix(url, "/")
	}
}

// New creates a new local filesystem storage
func New(cfg storage.LocalConfig) (storage.ImageStorage, error) {
	s := &Storage{
		basePath: cfg.Path,
		baseURL:  strings.TrimSuffix(cfg.BaseURL, "/"),
	}

	// Set defaults
	if s.basePath == "" {
		s.basePath = "/tmp/generated_images"
	}

	// Ensure base directory exists
	if err := os.MkdirAll(s.basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return s, nil
}

// NewWithOptions creates a new local storage with functional options
func NewWithOptions(options ...Option) (*Storage, error) {
	s := &Storage{
		basePath: "/tmp/generated_images",
	}

	for _, opt := range options {
		opt(s)
	}

	// Ensure base directory exists
	if err := os.MkdirAll(s.basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return s, nil
}

// Name returns the storage backend name
func (s *Storage) Name() string {
	return "local"
}

// Store saves an image to the local filesystem
func (s *Storage) Store(ctx context.Context, image *interfaces.GeneratedImage, metadata storage.StorageMetadata) (string, error) {
	if image == nil || len(image.Data) == 0 {
		return "", fmt.Errorf("image data is empty")
	}

	// Build directory path: basePath/orgID/threadID/
	dirPath := s.basePath
	if metadata.OrgID != "" {
		dirPath = filepath.Join(dirPath, sanitizePath(metadata.OrgID))
	}
	if metadata.ThreadID != "" {
		dirPath = filepath.Join(dirPath, sanitizePath(metadata.ThreadID))
	}

	// Ensure directory exists
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate filename: timestamp_hash.ext
	ext := getExtension(image.MimeType)
	hash := hashData(image.Data)[:12]
	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d_%s%s", timestamp, hash, ext)

	// Full file path
	filePath := filepath.Join(dirPath, filename)

	// Write file
	if err := os.WriteFile(filePath, image.Data, 0644); err != nil {
		return "", fmt.Errorf("failed to write image file: %w", err)
	}

	// Return URL or file path
	if s.baseURL != "" {
		// Build relative path from basePath
		relPath, err := filepath.Rel(s.basePath, filePath)
		if err != nil {
			return "", fmt.Errorf("failed to get relative path: %w", err)
		}
		// Convert to URL path (use forward slashes)
		urlPath := strings.ReplaceAll(relPath, string(filepath.Separator), "/")
		return fmt.Sprintf("%s/%s", s.baseURL, urlPath), nil
	}

	return filePath, nil
}

// Delete removes an image from the local filesystem
func (s *Storage) Delete(ctx context.Context, url string) error {
	filePath := s.urlToFilePath(url)
	if filePath == "" {
		return fmt.Errorf("invalid URL or file path")
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil // Already deleted
	}

	return os.Remove(filePath)
}

// Get retrieves image data from the local filesystem
func (s *Storage) Get(ctx context.Context, url string) ([]byte, error) {
	filePath := s.urlToFilePath(url)
	if filePath == "" {
		return nil, fmt.Errorf("invalid URL or file path")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image file: %w", err)
	}

	return data, nil
}

// urlToFilePath converts a URL or file path to an absolute file path
func (s *Storage) urlToFilePath(url string) string {
	// If it's already an absolute path
	if filepath.IsAbs(url) {
		return url
	}

	// If it's a URL, extract the path
	if s.baseURL != "" && strings.HasPrefix(url, s.baseURL) {
		relPath := strings.TrimPrefix(url, s.baseURL)
		relPath = strings.TrimPrefix(relPath, "/")
		return filepath.Join(s.basePath, relPath)
	}

	// Assume it's a relative path
	return filepath.Join(s.basePath, url)
}

// getExtension returns the file extension for a MIME type
func getExtension(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

// hashData returns a SHA256 hash of the data
func hashData(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// sanitizePath removes potentially dangerous characters from path components
func sanitizePath(s string) string {
	// Replace dangerous characters
	s = strings.ReplaceAll(s, "..", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}
