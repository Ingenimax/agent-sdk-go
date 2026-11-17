package multimodal

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
)

func TestDetectMIMEType(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "png", data: testutil.SamplePNG, want: "image/png"},
		{name: "jpeg", data: testutil.SampleJPEG, want: "image/jpeg"},
		{name: "gif", data: testutil.SampleGIF, want: "image/gif"},
		{name: "webp", data: testutil.SampleWebP, want: "image/webp"},
		{name: "pdf", data: testutil.SamplePDF, want: "application/pdf"},
		{name: "bmp", data: testutil.SampleBMP, want: "image/bmp"},
		{name: "text drops the charset", data: []byte("hello"), want: "text/plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectMIMEType(tt.data); got != tt.want {
				t.Errorf("DetectMIMEType = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSupportedMIMETypes(t *testing.T) {
	got := SupportedMIMETypes()
	want := "image/png,image/jpeg,image/gif,image/webp,application/pdf"
	if strings.Join(got, ",") != want {
		t.Fatalf("SupportedMIMETypes = %v", got)
	}
	// The result is a copy, so callers cannot change the allowlist.
	got[0] = "text/plain"
	if !IsSupported("image/png") || IsSupported("text/plain") {
		t.Error("changing the result changed the allowlist")
	}
	if !IsImage("image/webp") || IsImage(PDFMIMEType) {
		t.Error("IsImage is wrong")
	}
}

func TestNewInlinePart(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantType string
		wantErr  string
	}{
		{name: "png", data: testutil.SamplePNG, wantType: "image/png"},
		{name: "pdf", data: testutil.SamplePDF, wantType: "application/pdf"},
		{name: "empty", data: nil, wantErr: "empty"},
		{name: "bmp is refused", data: testutil.SampleBMP, wantErr: "unsupported file type image/bmp"},
		{name: "text is refused", data: []byte("plain words"), wantErr: "unsupported file type text/plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			part, err := NewInlinePart(tt.data, "upload.bin")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewInlinePart: %v", err)
			}
			if part.InlineData.MIMEType != tt.wantType || part.InlineData.DisplayName != "upload.bin" {
				t.Errorf("part = %+v", part.InlineData)
			}
			if err := part.Validate(); err != nil {
				t.Errorf("part is invalid: %v", err)
			}
		})
	}

	_, err := NewInlinePart(testutil.SampleBMP, "")
	if !errors.Is(err, ErrUnsupportedType) {
		t.Errorf("err = %v, want ErrUnsupportedType", err)
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}
	// The extension lies: the type comes from the content.
	png := write("picture.pdf", testutil.SamplePNG)
	pdf := write("report.pdf", testutil.SamplePDF)
	bmp := write("old.bmp", testutil.SampleBMP)
	big := write("big.png", append(append([]byte{}, testutil.SamplePNG...), make([]byte, 64)...))

	tests := []struct {
		name     string
		path     string
		max      int64
		wantType string
		wantErr  string
	}{
		{name: "png named .pdf", path: png, max: 1 << 10, wantType: "image/png"},
		{name: "pdf", path: pdf, max: 1 << 10, wantType: "application/pdf"},
		{name: "unsupported type", path: bmp, max: 1 << 10, wantErr: "unsupported file type image/bmp"},
		{name: "over the limit", path: big, max: 16, wantErr: "over the limit of 16 bytes"},
		{name: "missing", path: filepath.Join(dir, "missing.png"), max: 1 << 10, wantErr: "no such file"},
		{name: "directory", path: dir, max: 1 << 10, wantErr: "not a regular file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			part, err := ReadFile(tt.path, tt.max)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if part.MIMEType() != tt.wantType || part.InlineData.DisplayName != filepath.Base(tt.path) {
				t.Errorf("part = %+v", part.InlineData)
			}
		})
	}
}

func TestNewImageURLPart(t *testing.T) {
	dataURL := func(mime string, data []byte) string {
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	tests := []struct {
		name     string
		url      string
		wantType string
		wantURI  bool
		wantErr  string
	}{
		{name: "https png", url: "https://example.com/a/cat.PNG?size=2", wantType: "image/png", wantURI: true},
		{name: "http jpg", url: "http://example.com/cat.jpg", wantType: "image/jpeg", wantURI: true},
		{name: "jpeg", url: "https://example.com/cat.jpeg", wantType: "image/jpeg", wantURI: true},
		{name: "gif", url: "https://example.com/cat.gif", wantType: "image/gif", wantURI: true},
		{name: "webp", url: "https://example.com/cat.webp", wantType: "image/webp", wantURI: true},
		{name: "no extension", url: "https://example.com/image?id=3", wantErr: "cannot tell the image type"},
		{name: "pdf url", url: "https://example.com/report.pdf", wantErr: "cannot tell the image type"},
		{name: "file scheme", url: "file:///etc/passwd", wantErr: `unsupported URL scheme "file"`},
		{name: "no scheme", url: "cat.png", wantErr: `unsupported URL scheme ""`},
		{name: "no host", url: "https:///cat.png", wantErr: "no host"},
		{name: "bad url", url: "https://exa mple.com/cat.png", wantErr: "invalid URL"},
		// The declared type is ignored: the data decides.
		{name: "data url", url: dataURL("image/jpeg", testutil.SamplePNG), wantType: "image/png"},
		{name: "data url upper case scheme", url: "DATA:image/png;base64," + base64.StdEncoding.EncodeToString(testutil.SampleGIF), wantType: "image/gif"},
		{name: "data url pdf", url: dataURL("application/pdf", testutil.SamplePDF), wantErr: "not an image"},
		{name: "data url bmp", url: dataURL("image/bmp", testutil.SampleBMP), wantErr: "unsupported file type image/bmp"},
		{name: "data url not base64", url: "data:image/png,rawbytes", wantErr: "must be base64"},
		{name: "data url no comma", url: "data:image/png;base64", wantErr: "no comma"},
		{name: "data url bad base64", url: "data:image/png;base64,!!!", wantErr: "invalid base64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			part, err := NewImageURLPart(tt.url)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewImageURLPart: %v", err)
			}
			if part.MIMEType() != tt.wantType {
				t.Errorf("MIME type = %q, want %q", part.MIMEType(), tt.wantType)
			}
			if tt.wantURI != (part.FileData != nil) {
				t.Errorf("part = %+v, want a URI part: %v", part, tt.wantURI)
			}
			if tt.wantURI && part.FileData.FileURI != tt.url {
				t.Errorf("URI = %q", part.FileData.FileURI)
			}
		})
	}
}
