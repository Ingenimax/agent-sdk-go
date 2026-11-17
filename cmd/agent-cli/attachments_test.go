package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// writeMedia writes data to name in a temp dir and returns the path.
func writeMedia(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

func TestParseAttachmentFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []Attachment
		wantErr string
	}{
		{
			name: "both forms, in order",
			args: []string{"--image-path", "a.png", "--prompt", "hi", "--image-url=https://x.test/b.png", "--pdf-path", "c.pdf", "--image-path=d.jpg"},
			want: []Attachment{
				{Flag: flagImagePath, Value: "a.png"},
				{Flag: flagImageURL, Value: "https://x.test/b.png"},
				{Flag: flagPDFPath, Value: "c.pdf"},
				{Flag: flagImagePath, Value: "d.jpg"},
			},
		},
		{name: "none", args: []string{"--prompt=hi"}},
		{name: "missing value", args: []string{"--prompt", "hi", "--image-path"}, wantErr: "--image-path needs a value"},
		{name: "empty value", args: []string{"--prompt", "hi", "--pdf-path="}, wantErr: "--pdf-path needs a value"},
		{name: "empty separate value", args: []string{"--prompt", "hi", "--image-url", ""}, wantErr: "--image-url needs a value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withArgs(t, tt.args...)
			got, err := parseDirectExecutionFlags()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got.Prompt != "hi" {
				t.Errorf("prompt = %q", got.Prompt)
			}
			if len(got.Attachments) != len(tt.want) {
				t.Fatalf("attachments = %+v, want %+v", got.Attachments, tt.want)
			}
			for i := range tt.want {
				if got.Attachments[i] != tt.want[i] {
					t.Errorf("attachment %d = %+v, want %+v", i, got.Attachments[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildPromptContent(t *testing.T) {
	png := writeMedia(t, "chart.png", testutil.SamplePNG)
	pdf := writeMedia(t, "report.pdf", testutil.SamplePDF)
	pdfNamedPNG := writeMedia(t, "sneaky.png", testutil.SamplePDF)
	bmp := writeMedia(t, "old.bmp", testutil.SampleBMP)
	longDataURL := "data:image/png;base64," + strings.Repeat("!", 200)
	longMissingPath := "/no/such/" + strings.Repeat("dir/", 30) + "file.pdf"

	tests := []struct {
		name        string
		attachments []Attachment
		wantParts   string
		wantErr     string
	}{
		{name: "prompt only", wantParts: "text"},
		{
			name: "attachments first, then the prompt",
			attachments: []Attachment{
				{Flag: flagImagePath, Value: png},
				{Flag: flagPDFPath, Value: pdf},
				{Flag: flagImageURL, Value: "https://x.test/cat.webp"},
				{Flag: flagImageURL, Value: "data:image/gif;base64," + base64.StdEncoding.EncodeToString(testutil.SampleGIF)},
			},
			wantParts: "image/png:chart.png,application/pdf:report.pdf,image/webp@https://x.test/cat.webp,image/gif:,text",
		},
		{name: "pdf passed as an image", attachments: []Attachment{{Flag: flagImagePath, Value: pdfNamedPNG}}, wantErr: "is application/pdf, not an image; attach PDFs with --pdf-path"},
		{name: "image passed as a pdf", attachments: []Attachment{{Flag: flagPDFPath, Value: png}}, wantErr: "is image/png, not a PDF; attach images with --image-path"},
		{name: "unsupported type", attachments: []Attachment{{Flag: flagImagePath, Value: bmp}}, wantErr: "unsupported file type image/bmp"},
		{name: "missing file keeps the whole path", attachments: []Attachment{{Flag: flagPDFPath, Value: longMissingPath}}, wantErr: "--pdf-path " + longMissingPath + ":"},
		{name: "url without a type", attachments: []Attachment{{Flag: flagImageURL, Value: "https://x.test/image"}}, wantErr: "cannot tell the image type"},
		{name: "long value is shortened", attachments: []Attachment{{Flag: flagImageURL, Value: longDataURL}}, wantErr: longDataURL[:80] + "...: invalid base64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := buildPromptContent("describe", tt.attachments)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildPromptContent: %v", err)
			}
			var got []string
			for _, p := range content.Parts {
				switch {
				case p.InlineData != nil:
					got = append(got, p.InlineData.MIMEType+":"+p.InlineData.DisplayName)
				case p.FileData != nil:
					got = append(got, p.FileData.MIMEType+"@"+p.FileData.FileURI)
				default:
					got = append(got, "text")
				}
			}
			if strings.Join(got, ",") != tt.wantParts {
				t.Errorf("parts = %s, want %s", strings.Join(got, ","), tt.wantParts)
			}
			if content.Role != interfaces.MessageRoleUser || content.Text() != "describe" {
				t.Errorf("content = %+v", content)
			}
		})
	}
}

func TestExecuteDirectPrompt_SendsImage(t *testing.T) {
	isolateCLI(t)
	f := startFakeOllama(t, "a bar chart")
	png := writeMedia(t, "chart.png", testutil.SamplePNG)
	withArgs(t, "--prompt", "What is this?", "--image-path", png)

	out := captureStdout(t, main)
	if !strings.Contains(out, "a bar chart") {
		t.Errorf("output = %q", out)
	}
	var sent struct {
		Images []string `json:"images"`
	}
	if err := json.Unmarshal([]byte(f.lastBody()), &sent); err != nil {
		t.Fatalf("decode request %q: %v", f.lastBody(), err)
	}
	if len(sent.Images) != 1 || sent.Images[0] != base64.StdEncoding.EncodeToString(testutil.SamplePNG) {
		t.Errorf("images = %v", sent.Images)
	}
}

func TestExecuteDirectPrompt_RefusedAttachments(t *testing.T) {
	tests := []struct {
		name string
		args func(t *testing.T) []string
	}{
		{
			// Ollama takes images only, so it refuses the PDF before sending.
			name: "provider cannot send a pdf",
			args: func(t *testing.T) []string {
				return []string{"--prompt", "summarize", "--pdf-path", writeMedia(t, "report.pdf", testutil.SamplePDF)}
			},
		},
		{
			name: "unsupported file",
			args: func(t *testing.T) []string {
				return []string{"--prompt", "describe", "--image-path", writeMedia(t, "old.bmp", testutil.SampleBMP)}
			},
		},
		{
			name: "missing flag value",
			args: func(*testing.T) []string { return []string{"--prompt", "describe", "--image-url"} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateCLI(t)
			f := startFakeOllama(t, "unused")
			withArgs(t, tt.args(t)...)
			out := captureStdout(t, main)
			if f.calls() != 0 {
				t.Errorf("LLM was called %d times", f.calls())
			}
			if strings.Contains(out, "Response:") {
				t.Errorf("output = %q", out)
			}
		})
	}
}
