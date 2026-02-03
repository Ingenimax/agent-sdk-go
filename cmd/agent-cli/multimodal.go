package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func buildImageContentParts(imageURLs []string, imagePaths []string, detail string) ([]interfaces.ContentPart, error) {
	parts := make([]interfaces.ContentPart, 0, len(imageURLs)+len(imagePaths))

	// URLs (http/https/data)
	for _, u := range imageURLs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if !isAllowedImageURLScheme(u) {
			return nil, fmt.Errorf("invalid --image-url: must start with http://, https://, or data")
		}
		if strings.HasPrefix(strings.ToLower(u), "data:") && !strings.HasPrefix(strings.ToLower(u), "data:image/") {
			return nil, fmt.Errorf("invalid --image-url data URL: must be data:image/*")
		}
		parts = append(parts, interfaces.ImageURLPart(u, detail))
	}

	// Local files → data URL (scoped to current directory to prevent path traversal)
	root, err := os.OpenRoot(".")
	if err != nil {
		return nil, fmt.Errorf("failed to open current directory: %w", err)
	}
	defer func() { _ = root.Close() }()

	for _, p := range imagePaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		f, err := root.Open(p)
		if err != nil {
			return nil, fmt.Errorf("failed to read image file %q: %w", p, err)
		}
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read image file %q: %w", p, err)
		}
		mime := http.DetectContentType(data)
		if !isAllowedImageMIME(mime) {
			return nil, fmt.Errorf("unsupported image mime type %q for file %q", mime, filepath.Base(p))
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		dataURL := fmt.Sprintf("data:%s;base64,%s", mime, b64)
		parts = append(parts, interfaces.ImageURLPart(dataURL, detail))
	}

	return parts, nil
}

func isAllowedImageURLScheme(u string) bool {
	l := strings.ToLower(strings.TrimSpace(u))
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "data:")
}

func isAllowedImageMIME(mime string) bool {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}
