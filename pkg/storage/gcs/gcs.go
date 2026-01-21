package gcs

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	imgstorage "github.com/Ingenimax/agent-sdk-go/pkg/storage"
)

func init() {
	// Register the GCS storage factory
	imgstorage.NewGCSStorage = New
}

// Storage implements ImageStorage for Google Cloud Storage
type Storage struct {
	client              *storage.Client
	bucket              string
	prefix              string
	signedURLExpiration time.Duration
	useSignedURLs       bool
}

// New creates a new GCS storage backend
func New(cfg imgstorage.GCSConfig) (imgstorage.ImageStorage, error) {
	ctx := context.Background()

	// Build client options
	var opts []option.ClientOption

	// CredentialsJSON takes precedence over CredentialsFile
	if cfg.CredentialsJSON != "" {
		credentialsJSON := parseCredentialsJSON(cfg.CredentialsJSON)
		fmt.Printf("[gcs] Using credentials JSON (length=%d, starts_with_brace=%v)\n",
			len(credentialsJSON), len(credentialsJSON) > 0 && credentialsJSON[0] == '{')
		//nolint:staticcheck // SA1019: WithCredentialsJSON is deprecated but needed for programmatic credentials
		opts = append(opts, option.WithCredentialsJSON([]byte(credentialsJSON)))
	} else if cfg.CredentialsFile != "" {
		//nolint:staticcheck // SA1019: WithCredentialsFile is deprecated but needed for file-based credentials
		fmt.Printf("[gcs] Using credentials file: %s\n", cfg.CredentialsFile)
		opts = append(opts, option.WithCredentialsFile(cfg.CredentialsFile))
	} else {
		fmt.Println("[gcs] No credentials provided, using Application Default Credentials")
	}

	// Create GCS client
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	// Validate bucket exists
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("GCS bucket name is required")
	}

	s := &Storage{
		client:              client,
		bucket:              cfg.Bucket,
		prefix:              strings.TrimSuffix(cfg.Prefix, "/"),
		signedURLExpiration: cfg.SignedURLExpiration,
		useSignedURLs:       cfg.UseSignedURLs,
	}

	// Set defaults
	if s.signedURLExpiration == 0 {
		s.signedURLExpiration = 24 * time.Hour
	}

	return s, nil
}

// Name returns the storage backend name
func (s *Storage) Name() string {
	return "gcs"
}

// Store saves an image to GCS and returns an accessible URL
func (s *Storage) Store(ctx context.Context, image *interfaces.GeneratedImage, metadata imgstorage.StorageMetadata) (string, error) {
	if image == nil || len(image.Data) == 0 {
		return "", fmt.Errorf("image data is empty")
	}

	// Build object path: prefix/orgID/threadID/timestamp_hash.ext
	objectPath := s.prefix
	if metadata.OrgID != "" {
		objectPath = joinPath(objectPath, sanitizePath(metadata.OrgID))
	}
	if metadata.ThreadID != "" {
		objectPath = joinPath(objectPath, sanitizePath(metadata.ThreadID))
	}

	// Generate filename: timestamp_hash.ext
	ext := getExtension(image.MimeType)
	hash := hashData(image.Data)[:12]
	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d_%s%s", timestamp, hash, ext)
	objectPath = joinPath(objectPath, filename)

	// Get bucket handle
	bucket := s.client.Bucket(s.bucket)
	obj := bucket.Object(objectPath)

	// Create writer
	wc := obj.NewWriter(ctx)
	wc.ContentType = image.MimeType

	// Add metadata
	wc.Metadata = map[string]string{
		"prompt": truncateString(metadata.Prompt, 500),
	}
	if metadata.OrgID != "" {
		wc.Metadata["org_id"] = metadata.OrgID
	}
	if metadata.ThreadID != "" {
		wc.Metadata["thread_id"] = metadata.ThreadID
	}
	if metadata.MessageID != "" {
		wc.Metadata["message_id"] = metadata.MessageID
	}

	// Write data
	if _, err := wc.Write(image.Data); err != nil {
		return "", fmt.Errorf("failed to write to GCS: %w", err)
	}
	if err := wc.Close(); err != nil {
		return "", fmt.Errorf("failed to close GCS writer: %w", err)
	}

	// Generate URL
	if s.useSignedURLs {
		return s.generateSignedURL(ctx, objectPath)
	}

	// Return public URL (requires bucket to be public or have appropriate IAM)
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.bucket, objectPath), nil
}

// Delete removes an image from GCS
func (s *Storage) Delete(ctx context.Context, url string) error {
	objectPath := s.urlToObjectPath(url)
	if objectPath == "" {
		return fmt.Errorf("invalid URL or object path")
	}

	bucket := s.client.Bucket(s.bucket)
	obj := bucket.Object(objectPath)

	if err := obj.Delete(ctx); err != nil {
		if err == storage.ErrObjectNotExist {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete from GCS: %w", err)
	}

	return nil
}

// Get retrieves image data from GCS
func (s *Storage) Get(ctx context.Context, url string) ([]byte, error) {
	objectPath := s.urlToObjectPath(url)
	if objectPath == "" {
		return nil, fmt.Errorf("invalid URL or object path")
	}

	bucket := s.client.Bucket(s.bucket)
	obj := bucket.Object(objectPath)

	rc, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read from GCS: %w", err)
	}
	defer func() {
		_ = rc.Close()
	}()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to read object data: %w", err)
	}

	return data, nil
}

// generateSignedURL creates a signed URL for the object
func (s *Storage) generateSignedURL(ctx context.Context, objectPath string) (string, error) {
	opts := &storage.SignedURLOptions{
		Scheme:  storage.SigningSchemeV4,
		Method:  "GET",
		Expires: time.Now().Add(s.signedURLExpiration),
	}

	url, err := s.client.Bucket(s.bucket).SignedURL(objectPath, opts)
	if err != nil {
		// Fall back to public URL if signing fails
		return fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.bucket, objectPath), nil
	}

	return url, nil
}

// urlToObjectPath extracts the object path from a URL
func (s *Storage) urlToObjectPath(url string) string {
	// Handle direct object paths
	if !strings.HasPrefix(url, "http") {
		return url
	}

	// Handle GCS URLs: https://storage.googleapis.com/bucket/path
	prefix := fmt.Sprintf("https://storage.googleapis.com/%s/", s.bucket)
	if strings.HasPrefix(url, prefix) {
		path := strings.TrimPrefix(url, prefix)
		// Remove query parameters (for signed URLs)
		if idx := strings.Index(path, "?"); idx != -1 {
			path = path[:idx]
		}
		return path
	}

	// Handle signed URLs with the bucket in the path
	if strings.Contains(url, s.bucket) {
		// Extract path after bucket name
		parts := strings.SplitN(url, s.bucket+"/", 2)
		if len(parts) == 2 {
			path := parts[1]
			// Remove query parameters
			if idx := strings.Index(path, "?"); idx != -1 {
				path = path[:idx]
			}
			return path
		}
	}

	return ""
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
	s = strings.ReplaceAll(s, "..", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}

// joinPath joins path components with forward slashes
func joinPath(base, path string) string {
	if base == "" {
		return path
	}
	if path == "" {
		return base
	}
	return base + "/" + path
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// parseCredentialsJSON parses credentials that may be base64 encoded or raw JSON
func parseCredentialsJSON(creds string) string {
	// Try to decode as base64 first
	if decoded, err := base64.StdEncoding.DecodeString(creds); err == nil {
		// Check if decoded content looks like JSON
		if len(decoded) > 0 && decoded[0] == '{' {
			return string(decoded)
		}
	}
	// Return as-is (assuming it's raw JSON)
	return creds
}
