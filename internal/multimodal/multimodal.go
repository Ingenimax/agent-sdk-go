// Package multimodal turns files and URLs into content parts that the
// module's providers can send. The agent CLI and the HTTP API both use it, so
// they accept the same types and detect them the same way.
//
// The type of file data always comes from the data itself, through
// http.DetectContentType, never from a file name or a type a client declared.
package multimodal

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// PDFMIMEType is the MIME type of a PDF document.
const PDFMIMEType = "application/pdf"

// imageMIMETypes are the image types that every provider in this module with
// image input accepts.
var imageMIMETypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

// imageExtensions maps the URL path extensions of supported images to their
// MIME types.
var imageExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// ErrUnsupportedType is wrapped by the errors for data whose type is not one
// of SupportedMIMETypes.
var ErrUnsupportedType = errors.New("unsupported file type")

// SupportedMIMETypes returns the types a file can have: PNG, JPEG, GIF, and
// WebP images, and PDF documents.
func SupportedMIMETypes() []string {
	return append(slices.Clone(imageMIMETypes), PDFMIMEType)
}

// IsSupported reports whether mimeType is one of SupportedMIMETypes.
func IsSupported(mimeType string) bool {
	return IsImage(mimeType) || mimeType == PDFMIMEType
}

// IsImage reports whether mimeType is a supported image type.
func IsImage(mimeType string) bool {
	return slices.Contains(imageMIMETypes, mimeType)
}

// DetectMIMEType returns the MIME type of data, detected from its first bytes
// by http.DetectContentType, without parameters such as a charset.
func DetectMIMEType(data []byte) string {
	mimeType, _, _ := strings.Cut(http.DetectContentType(data), ";")
	return strings.TrimSpace(mimeType)
}

// NewInlinePart returns a part that sends data inline, with the MIME type
// detected from the data. displayName optionally names the data, such as a
// file name; OpenAI and Bedrock send it with PDFs.
//
// It returns an error that wraps ErrUnsupportedType when the detected type is
// not one of SupportedMIMETypes.
func NewInlinePart(data []byte, displayName string) (*interfaces.Part, error) {
	if len(data) == 0 {
		return nil, errors.New("the data is empty")
	}
	mimeType := DetectMIMEType(data)
	if !IsSupported(mimeType) {
		return nil, unsupported(mimeType)
	}
	part := interfaces.NewPartFromBytes(data, mimeType)
	part.InlineData.DisplayName = displayName
	return part, nil
}

// unsupported returns the error for data of an unsupported type.
func unsupported(mimeType string) error {
	return fmt.Errorf("%w %s: supported types are %s", ErrUnsupportedType, mimeType, strings.Join(SupportedMIMETypes(), ", "))
}

// ReadFile reads the local file at name into an inline part, with the MIME
// type detected from the content and the file's base name as the display
// name. It refuses anything that is not a regular file, and files larger than
// maxBytes.
func ReadFile(name string, maxBytes int64) (*interfaces.Part, error) {
	// #nosec G304 -- the caller reads a file the user named on its own command line.
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if info.Size() > maxBytes {
		return nil, tooLarge(info.Size(), maxBytes)
	}
	// The limit also guards against a file that grows after Stat.
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, tooLarge(int64(len(data)), maxBytes)
	}
	return NewInlinePart(data, filepath.Base(name))
}

// tooLarge returns the error for a file over the size limit.
func tooLarge(size, maxBytes int64) error {
	return fmt.Errorf("the file is %d bytes, over the limit of %d bytes", size, maxBytes)
}

// NewImageURLPart returns the part for an image URL.
//
// A data URL, such as "data:image/png;base64,...", must be base64 encoded. It
// is decoded and sent inline, with the type detected from the data.
//
// An http or https URL is sent as a URI for the provider to fetch. The image
// is not downloaded here, so its type comes from the extension of the URL
// path: .png, .jpg, .jpeg, .gif, or .webp.
func NewImageURLPart(rawURL string) (*interfaces.Part, error) {
	if scheme, _, ok := strings.Cut(rawURL, ":"); ok && strings.EqualFold(scheme, "data") {
		return dataURLPart(rawURL)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q: use http, https, or data", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("the URL has no host")
	}
	mimeType, ok := imageExtensions[strings.ToLower(path.Ext(u.Path))]
	if !ok {
		return nil, errors.New("cannot tell the image type from the URL: use a URL whose path ends in .png, .jpg, .jpeg, .gif, or .webp, or download the image and attach the file")
	}
	return interfaces.NewPartFromURI(rawURL, mimeType), nil
}

// dataURLPart decodes a base64 data URL into an inline image part.
func dataURLPart(rawURL string) (*interfaces.Part, error) {
	header, payload, ok := strings.Cut(rawURL[len("data:"):], ",")
	if !ok {
		return nil, errors.New("invalid data URL: no comma before the data")
	}
	if !strings.HasSuffix(strings.ToLower(header), ";base64") {
		return nil, errors.New("the data URL must be base64 encoded")
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 in data URL: %w", err)
	}
	part, err := NewInlinePart(data, "")
	if err != nil {
		return nil, err
	}
	if !part.IsImage() {
		return nil, fmt.Errorf("the data URL holds %s, not an image", part.MIMEType())
	}
	return part, nil
}
