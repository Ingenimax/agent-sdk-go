package microservice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// partsLLM records the parts of each request and refuses the parts that
// supported rejects, the way a provider refuses media it cannot send.
type partsLLM struct {
	supported func(*interfaces.Part) bool

	mu    sync.Mutex
	parts [][]*interfaces.Part
}

// noPDF is a supported function for a model without document input.
func noPDF(p *interfaces.Part) bool { return !p.IsPDF() }

// anyPart is a supported function for a model that takes every part.
func anyPart(*interfaces.Part) bool { return true }

func (l *partsLLM) record(ctx context.Context, options []interfaces.GenerateOption) (string, error) {
	opts := &interfaces.GenerateOptions{}
	for _, o := range options {
		o(opts)
	}
	l.mu.Lock()
	l.parts = append(l.parts, opts.Parts)
	l.mu.Unlock()
	if err := interfaces.CheckMedia(ctx, opts, "parts-llm", l.supported); err != nil {
		return "", err
	}
	return fmt.Sprintf("saw %d parts", len(opts.Parts)), nil
}

// seen returns the parts of the last request.
func (l *partsLLM) seen() []*interfaces.Part {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.parts) == 0 {
		return nil
	}
	return l.parts[len(l.parts)-1]
}

func (l *partsLLM) Generate(ctx context.Context, _ string, options ...interfaces.GenerateOption) (string, error) {
	return l.record(ctx, options)
}

func (l *partsLLM) GenerateWithTools(ctx context.Context, _ string, _ []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	return l.record(ctx, options)
}

func (l *partsLLM) GenerateDetailed(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	text, err := l.Generate(ctx, prompt, options...)
	if err != nil {
		return nil, err
	}
	return &interfaces.LLMResponse{Content: text, Model: "parts-llm"}, nil
}

func (l *partsLLM) GenerateWithToolsDetailed(ctx context.Context, prompt string, _ []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return l.GenerateDetailed(ctx, prompt, options...)
}

func (l *partsLLM) Name() string            { return "parts-llm" }
func (l *partsLLM) SupportsStreaming() bool { return true }

func (l *partsLLM) GenerateStream(ctx context.Context, _ string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	text, err := l.record(ctx, options)
	if err != nil {
		return nil, err
	}
	ch := make(chan interfaces.StreamEvent, 2)
	ch <- interfaces.StreamEvent{Type: interfaces.StreamEventContentDelta, Content: text, Timestamp: time.Now()}
	ch <- interfaces.StreamEvent{Type: interfaces.StreamEventMessageStop, Timestamp: time.Now()}
	close(ch)
	return ch, nil
}

func (l *partsLLM) GenerateWithToolsStream(ctx context.Context, prompt string, _ []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	return l.GenerateStream(ctx, prompt, options...)
}

// newPartsAgent returns a local agent backed by a partsLLM.
func newPartsAgent(t *testing.T, supported func(*interfaces.Part) bool) (*agent.Agent, *partsLLM) {
	t.Helper()
	llm := &partsLLM{supported: supported}
	a, err := agent.NewAgent(agent.WithLLM(llm), agent.WithName("parts-agent"))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a, llm
}

// formFile is one file of a multipart request.
type formFile struct {
	field, name string
	data        []byte
}

// multipartBody builds a multipart/form-data body from text fields, in the
// given order, and files. It returns the body and its Content-Type.
func multipartBody(t *testing.T, fields [][2]string, files ...formFile) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range fields {
		if err := mw.WriteField(f[0], f[1]); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	for _, f := range files {
		w, err := mw.CreateFormFile(f.field, f.name)
		if err != nil {
			t.Fatalf("create file: %v", err)
		}
		if _, err := w.Write(f.data); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

// b64 returns data as standard base64, the JSON encoding of []byte.
func b64(data []byte) string { return base64.StdEncoding.EncodeToString(data) }

// inlineJSON returns a JSON part with inline data.
func inlineJSON(mimeType string, data []byte) string {
	return fmt.Sprintf(`{"inlineData":{"mimeType":%q,"data":%q}}`, mimeType, b64(data))
}

// describeParts summarizes parts as "text:hi,image/png:name".
func describeParts(parts []*interfaces.Part) string {
	var out []string
	for _, p := range parts {
		switch {
		case p.InlineData != nil:
			out = append(out, p.InlineData.MIMEType+":"+p.InlineData.DisplayName)
		case p.FileData != nil:
			out = append(out, p.FileData.MIMEType+"@"+p.FileData.FileURI)
		default:
			out = append(out, "text:"+p.Text)
		}
	}
	return strings.Join(out, ",")
}

func TestParseRunRequest_JSON(t *testing.T) {
	manyImages := make([]string, MaxRunRequestFiles+1)
	for i := range manyImages {
		manyImages[i] = inlineJSON("image/png", testutil.SamplePNG)
	}
	tests := []struct {
		name       string
		body       string
		wantParts  string
		wantStatus int
		wantErr    string
	}{
		{name: "text only", body: `{"input":"hi","org_id":"o","conversation_id":"c"}`, wantParts: "text:hi"},
		{
			name:      "content then input; detected type replaces the declared one",
			body:      `{"input":"what is this?","content":{"parts":[` + inlineJSON("image/jpeg", testutil.SamplePNG) + `]}}`,
			wantParts: "image/png:,text:what is this?",
		},
		{
			name:      "content only, with a user role",
			body:      `{"content":{"role":"user","parts":[{"text":"describe"},` + inlineJSON("application/pdf", testutil.SamplePDF) + `]}}`,
			wantParts: "text:describe,application/pdf:",
		},
		{
			name:      "https file data",
			body:      `{"input":"x","content":{"parts":[{"fileData":{"mimeType":"image/png","fileUri":"https://example.com/a.png"}}]}}`,
			wantParts: "image/png@https://example.com/a.png,text:x",
		},
		{name: "invalid JSON", body: `{`, wantStatus: http.StatusBadRequest, wantErr: "Invalid JSON"},
		{name: "no input", body: `{}`, wantStatus: http.StatusBadRequest, wantErr: "Input is required"},
		{name: "empty content", body: `{"content":{"parts":[]}}`, wantStatus: http.StatusBadRequest, wantErr: "Input is required"},
		{name: "model role", body: `{"content":{"role":"model","parts":[{"text":"x"}]}}`, wantStatus: http.StatusBadRequest, wantErr: `role must be "user"`},
		{name: "null part", body: `{"content":{"parts":[null]}}`, wantStatus: http.StatusBadRequest, wantErr: "part 0: the part is null"},
		{name: "misspelled field", body: `{"input":"x","content":{"parts":[{"inline_data":{}}]}}`, wantStatus: http.StatusBadRequest, wantErr: "the part is empty"},
		{name: "bmp", body: `{"content":{"parts":[` + inlineJSON("image/png", testutil.SampleBMP) + `]}}`, wantStatus: http.StatusBadRequest, wantErr: "inline data is image/bmp"},
		{name: "empty inline data", body: `{"content":{"parts":[{"inlineData":{"mimeType":"image/png"}}]}}`, wantStatus: http.StatusBadRequest, wantErr: "inline data is empty"},
		{name: "two fields", body: `{"content":{"parts":[{"text":"x","inlineData":{"mimeType":"image/png","data":"` + b64(testutil.SamplePNG) + `"}}]}}`, wantStatus: http.StatusBadRequest, wantErr: "more than one"},
		{name: "s3 file data", body: `{"content":{"parts":[{"fileData":{"mimeType":"image/png","fileUri":"s3://bucket/a.png"}}]}}`, wantStatus: http.StatusBadRequest, wantErr: "https URL"},
		{name: "unsupported file data type", body: `{"content":{"parts":[{"fileData":{"mimeType":"video/mp4","fileUri":"https://example.com/a.mp4"}}]}}`, wantStatus: http.StatusBadRequest, wantErr: `"video/mp4" is not supported`},
		{name: "too many files", body: `{"input":"x","content":{"parts":[` + strings.Join(manyImages, ",") + `]}}`, wantStatus: http.StatusBadRequest, wantErr: "Too many files"},
		{name: "oversized", body: `{"input":"` + strings.Repeat("a", MaxRunRequestBytes) + `"}`, wantStatus: http.StatusRequestEntityTooLarge, wantErr: "larger than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			req, content, err := parseRunRequest(httptest.NewRecorder(), r)
			if tt.wantErr != "" {
				var reqErr *requestError
				if !errors.As(err, &reqErr) || reqErr.status != tt.wantStatus || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want status %d and %q", err, tt.wantStatus, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRunRequest: %v", err)
			}
			if got := describeParts(content.Parts); got != tt.wantParts {
				t.Errorf("parts = %s, want %s", got, tt.wantParts)
			}
			if content.Role != interfaces.MessageRoleUser {
				t.Errorf("role = %q", content.Role)
			}
			if tt.name == "text only" && (req.OrgID != "o" || req.ConversationID != "c") {
				t.Errorf("request = %+v", req)
			}
		})
	}
}

func TestParseRunRequest_Multipart(t *testing.T) {
	png := formFile{field: "files", name: "cat.png", data: testutil.SamplePNG}
	pdf := formFile{field: "files", name: "report.pdf", data: testutil.SamplePDF}
	tooMany := make([]formFile, MaxRunRequestFiles+1)
	for i := range tooMany {
		tooMany[i] = png
	}
	tests := []struct {
		name       string
		fields     [][2]string
		files      []formFile
		wantParts  string
		wantStatus int
		wantErr    string
	}{
		{
			name:      "input, image, and pdf",
			fields:    [][2]string{{"input", "compare these"}, {"org_id", "o"}, {"conversation_id", "c"}, {"unknown", "ignored"}},
			files:     []formFile{png, pdf},
			wantParts: "image/png:cat.png,application/pdf:report.pdf,text:compare these",
		},
		{name: "files only", files: []formFile{pdf}, wantParts: "application/pdf:report.pdf"},
		{name: "input only", fields: [][2]string{{"input", "hi"}}, wantParts: "text:hi"},
		// The client's file name and Content-Type do not matter: the data does.
		{name: "type comes from the content", files: []formFile{{field: "files", name: "photo.pdf", data: testutil.SampleJPEG}}, wantParts: "image/jpeg:photo.pdf"},
		{name: "nothing", wantStatus: http.StatusBadRequest, wantErr: "Input is required"},
		{name: "wrong file field", files: []formFile{{field: "image", name: "a.png", data: testutil.SamplePNG}}, wantStatus: http.StatusBadRequest, wantErr: `Unexpected file field "image"`},
		{name: "unsupported type", files: []formFile{{field: "files", name: "a.bmp", data: testutil.SampleBMP}}, wantStatus: http.StatusBadRequest, wantErr: `File "a.bmp": unsupported file type image/bmp`},
		{name: "text file", files: []formFile{{field: "files", name: "notes.png", data: []byte("just text")}}, wantStatus: http.StatusBadRequest, wantErr: "unsupported file type text/plain"},
		{name: "empty file", files: []formFile{{field: "files", name: "empty.png"}}, wantStatus: http.StatusBadRequest, wantErr: "empty"},
		{name: "too many files", files: tooMany, wantStatus: http.StatusBadRequest, wantErr: "Too many files"},
		{name: "oversized", files: []formFile{{field: "files", name: "big.png", data: append(append([]byte{}, testutil.SamplePNG...), make([]byte, MaxRunRequestBytes)...)}}, wantStatus: http.StatusRequestEntityTooLarge, wantErr: "larger than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, contentType := multipartBody(t, tt.fields, tt.files...)
			r := httptest.NewRequest(http.MethodPost, "/", body)
			r.Header.Set("Content-Type", contentType)
			req, content, err := parseRunRequest(httptest.NewRecorder(), r)
			if tt.wantErr != "" {
				var reqErr *requestError
				if !errors.As(err, &reqErr) || reqErr.status != tt.wantStatus || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want status %d and %q", err, tt.wantStatus, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRunRequest: %v", err)
			}
			if got := describeParts(content.Parts); got != tt.wantParts {
				t.Errorf("parts = %s, want %s", got, tt.wantParts)
			}
			if len(tt.fields) > 1 && (req.OrgID != "o" || req.ConversationID != "c") {
				t.Errorf("request = %+v", req)
			}
		})
	}
}

func TestParseRunRequest_MalformedMultipart(t *testing.T) {
	tests := []struct {
		name, contentType, body string
	}{
		{name: "no boundary", contentType: "multipart/form-data", body: "x"},
		{name: "broken body", contentType: "multipart/form-data; boundary=b", body: "--b\r\nContent-Disposition: form-data; name=\"input\"\r\n\r\nhi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", tt.contentType)
			_, _, err := parseRunRequest(httptest.NewRecorder(), r)
			var reqErr *requestError
			if !errors.As(err, &reqErr) || reqErr.status != http.StatusBadRequest || !strings.Contains(err.Error(), "multipart") {
				t.Errorf("err = %v, want a 400 about the multipart form", err)
			}
		})
	}
}

func TestWriteRequestError(t *testing.T) {
	w := httptest.NewRecorder()
	writeRequestError(w, errors.New("plain"))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "plain") {
		t.Errorf("plain error = %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	writeRequestError(w, &requestError{status: http.StatusRequestEntityTooLarge, msg: "big"})
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d", w.Code)
	}
}

func TestRunErrorStatus(t *testing.T) {
	unsupported := fmt.Errorf("provider: %w", interfaces.ErrContentUnsupported)
	if got := runErrorStatus(unsupported, http.StatusInternalServerError); got != http.StatusUnprocessableEntity {
		t.Errorf("unsupported status = %d", got)
	}
	if got := runErrorStatus(errors.New("boom"), http.StatusInternalServerError); got != http.StatusInternalServerError {
		t.Errorf("other status = %d", got)
	}
}

// eventChan returns a closed channel holding events.
func eventChan(events ...interfaces.AgentStreamEvent) chan interfaces.AgentStreamEvent {
	ch := make(chan interfaces.AgentStreamEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch
}

// collect returns the Content of each event in seq.
func collect(seq func(func(interfaces.AgentStreamEvent) bool)) []string {
	var out []string
	for e := range seq {
		out = append(out, e.Content)
	}
	return out
}

func TestStreamEvents(t *testing.T) {
	text := interfaces.NewContentFromText("hi", interfaces.MessageRoleUser)
	media := interfaces.NewContentFromParts([]*interfaces.Part{
		interfaces.NewPartFromBytes(testutil.SamplePNG, "image/png"),
	}, interfaces.MessageRoleUser)
	unsupported := interfaces.AgentStreamEvent{Type: interfaces.AgentEventError, Error: fmt.Errorf("x: %w", interfaces.ErrContentUnsupported)}
	a := interfaces.AgentStreamEvent{Type: interfaces.AgentEventContent, Content: "a"}
	b := interfaces.AgentStreamEvent{Type: interfaces.AgentEventContent, Content: "b"}

	t.Run("text only streams as is", func(t *testing.T) {
		// Text cannot be refused as unsupported, so the error passes through.
		seq, err := streamEvents(context.Background(), text, eventChan(unsupported, a))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got := collect(seq); len(got) != 2 {
			t.Errorf("events = %v", got)
		}
	})
	t.Run("media replays the first event", func(t *testing.T) {
		seq, err := streamEvents(context.Background(), media, eventChan(a, b))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got := strings.Join(collect(seq), ","); got != "a,b" {
			t.Errorf("events = %s", got)
		}
	})
	t.Run("media refused", func(t *testing.T) {
		ch := make(chan interfaces.AgentStreamEvent, 3)
		ch <- unsupported
		ch <- a
		ch <- b
		close(ch)
		_, err := streamEvents(context.Background(), media, ch)
		if !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Fatalf("err = %v", err)
		}
		// The rest of the stream is drained in the background.
		deadline := time.Now().Add(2 * time.Second)
		for len(ch) > 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if len(ch) != 0 {
			t.Errorf("%d events were not drained", len(ch))
		}
	})
	t.Run("other first error is streamed", func(t *testing.T) {
		seq, err := streamEvents(context.Background(), media, eventChan(interfaces.AgentStreamEvent{Type: interfaces.AgentEventError, Error: errors.New("boom")}))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got := collect(seq); len(got) != 1 {
			t.Errorf("events = %v", got)
		}
	})
	t.Run("empty stream", func(t *testing.T) {
		seq, err := streamEvents(context.Background(), media, eventChan())
		if err != nil || len(collect(seq)) != 0 {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("cancelled before the first event", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		ch := make(chan interfaces.AgentStreamEvent)
		seq, err := streamEvents(ctx, media, ch)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		close(ch)
		if got := collect(seq); len(got) != 0 {
			t.Errorf("events = %v", got)
		}
	})
	t.Run("consumer stops early", func(t *testing.T) {
		for _, events := range [][]interfaces.AgentStreamEvent{{a, b}, {a}} {
			seq, _ := streamEvents(context.Background(), media, eventChan(events...))
			for range seq {
				break
			}
		}
		seq, _ := streamEvents(context.Background(), text, eventChan(a, b))
		for range seq {
			break
		}
	})
}

// post sends body to handler as a POST and returns the response.
func post(handler http.HandlerFunc, contentType string, body io.Reader) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/", body)
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}

// imageAndPDF returns a multipart body with input, a PNG, and a PDF.
func imageAndPDF(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	return multipartBody(t, [][2]string{{"input", "compare"}},
		formFile{field: "files", name: "cat.png", data: testutil.SamplePNG},
		formFile{field: "files", name: "report.pdf", data: testutil.SamplePDF})
}

func TestHTTPServer_RunWithContent(t *testing.T) {
	a, llm := newPartsAgent(t, anyPart)
	s := NewHTTPServer(a, 0)

	t.Run("json content", func(t *testing.T) {
		body := `{"input":"what is this?","content":{"parts":[` + inlineJSON("image/png", testutil.SamplePNG) + `]}}`
		w := post(s.handleRun, "application/json", strings.NewReader(body))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "saw 2 parts") {
			t.Fatalf("response = %d %s", w.Code, w.Body.String())
		}
		if got := describeParts(llm.seen()); got != "image/png:,text:what is this?" {
			t.Errorf("model saw %s", got)
		}
	})
	t.Run("multipart image and pdf", func(t *testing.T) {
		body, contentType := imageAndPDF(t)
		w := post(s.handleRun, contentType, body)
		if w.Code != http.StatusOK {
			t.Fatalf("response = %d %s", w.Code, w.Body.String())
		}
		if got := describeParts(llm.seen()); got != "image/png:cat.png,application/pdf:report.pdf,text:compare" {
			t.Errorf("model saw %s", got)
		}
	})
	t.Run("disallowed type", func(t *testing.T) {
		body, contentType := multipartBody(t, [][2]string{{"input", "x"}}, formFile{field: "files", name: "a.bmp", data: testutil.SampleBMP})
		w := post(s.handleRun, contentType, body)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "image/bmp") {
			t.Errorf("response = %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("oversized", func(t *testing.T) {
		w := post(s.handleRun, "application/json", strings.NewReader(`{"input":"`+strings.Repeat("a", MaxRunRequestBytes)+`"}`))
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d", w.Code)
		}
	})
}

func TestHTTPServer_RunUnsupportedContent(t *testing.T) {
	pdfOnly := `{"input":"summarize","content":{"parts":[` + inlineJSON("application/pdf", testutil.SamplePDF) + `]}}`

	t.Run("model refuses the part", func(t *testing.T) {
		a, _ := newPartsAgent(t, noPDF)
		w := post(NewHTTPServer(a, 0).handleRun, "application/json", strings.NewReader(pdfOnly))
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || !strings.Contains(body["error"], interfaces.ErrContentUnsupported.Error()) {
			t.Errorf("body = %s (%v)", w.Body.String(), err)
		}
	})
	t.Run("custom run function takes text only", func(t *testing.T) {
		a := newCustomAgent(t, func(context.Context, string, *agent.Agent) (string, error) { return "ok", nil }, nil)
		w := post(NewHTTPServer(a, 0).handleRun, "application/json", strings.NewReader(pdfOnly))
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, body %s", w.Code, w.Body.String())
		}
	})
}

func TestHTTPServer_StreamWithContent(t *testing.T) {
	a, llm := newPartsAgent(t, anyPart)
	body, contentType := imageAndPDF(t)
	w := post(NewHTTPServer(a, 0).handleStream, contentType, body)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("response = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	var names []string
	for _, e := range parseSSE(w.Body.String()) {
		names = append(names, e.event)
	}
	if got := strings.Join(names, ","); !strings.HasPrefix(got, "connected,content") || !strings.HasSuffix(got, "done") {
		t.Errorf("events = %s", got)
	}
	if !strings.Contains(w.Body.String(), "saw 3 parts") {
		t.Errorf("body = %s", w.Body.String())
	}
	if got := describeParts(llm.seen()); got != "image/png:cat.png,application/pdf:report.pdf,text:compare" {
		t.Errorf("model saw %s", got)
	}
}

func TestHTTPServer_StreamUnsupportedContent(t *testing.T) {
	tests := []struct {
		name  string
		agent func(t *testing.T) *agent.Agent
	}{
		{name: "model refuses the part", agent: func(t *testing.T) *agent.Agent {
			a, _ := newPartsAgent(t, noPDF)
			return a
		}},
		{name: "custom stream function takes text only", agent: func(t *testing.T) *agent.Agent {
			return newCustomAgent(t, nil, replayStream())
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, contentType := imageAndPDF(t)
			w := post(NewHTTPServer(tt.agent(t), 0).handleStream, contentType, body)
			if w.Code != http.StatusUnprocessableEntity || w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("response = %d %q %s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
			}
			if !strings.Contains(w.Body.String(), interfaces.ErrContentUnsupported.Error()) {
				t.Errorf("body = %s", w.Body.String())
			}
		})
	}
}

func TestHTTPServer_StreamBadRequest(t *testing.T) {
	a, _ := newPartsAgent(t, anyPart)
	body, contentType := multipartBody(t, nil, formFile{field: "files", name: "a.bmp", data: testutil.SampleBMP})
	w := post(NewHTTPServer(a, 0).handleStream, contentType, body)
	if w.Code != http.StatusBadRequest || strings.Contains(w.Header().Get("Content-Type"), "event-stream") {
		t.Errorf("response = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
}

func TestUIServer_RunWithContent(t *testing.T) {
	a, llm := newPartsAgent(t, noPDF)
	s := NewHTTPServerWithUI(a, 0, uiConfig())
	ts := serveUI(t, s)

	body, contentType := multipartBody(t, [][2]string{{"input", "what is this?"}, {"conversation_id", "c1"}},
		formFile{field: "files", name: "cat.png", data: testutil.SamplePNG})
	resp, err := http.Post(ts.URL+"/api/v1/agent/run", contentType, body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "saw 2 parts") {
		t.Fatalf("response = %d %s", resp.StatusCode, raw)
	}
	if got := describeParts(llm.seen()); got != "image/png:cat.png,text:what is this?" {
		t.Errorf("model saw %s", got)
	}
	if first := s.conversationHistory[0]; first.Content != "what is this?" || first.Metadata["attachments"] != 1 {
		t.Errorf("history = %+v", first)
	}

	// The model cannot take PDFs: a 422 with the UI's error body.
	pdf := `{"input":"summarize","content":{"parts":[` + inlineJSON("application/pdf", testutil.SamplePDF) + `]}}`
	resp, err = http.Post(ts.URL+"/api/v1/agent/run", "application/json", strings.NewReader(pdf))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	var failed map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&failed)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity || failed["output"] != "" || !strings.Contains(failed["error"], interfaces.ErrContentUnsupported.Error()) {
		t.Errorf("response = %d %v", resp.StatusCode, failed)
	}

	// Request errors are 4xx before the agent runs.
	resp, err = http.Post(ts.URL+"/api/v1/agent/run", "application/json", strings.NewReader(`{"input":"`+strings.Repeat("a", MaxRunRequestBytes)+`"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized status = %d", resp.StatusCode)
	}
}

func TestUIServer_StreamWithContent(t *testing.T) {
	a, llm := newPartsAgent(t, noPDF)
	s := NewHTTPServerWithUI(a, 0, uiConfig())
	ts := serveUI(t, s)

	body, contentType := multipartBody(t, [][2]string{{"input", "what is this?"}},
		formFile{field: "files", name: "cat.png", data: testutil.SamplePNG})
	resp, err := http.Post(ts.URL+"/api/v1/agent/stream", contentType, body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" || !strings.Contains(string(raw), "saw 2 parts") {
		t.Fatalf("response = %d %q %s", resp.StatusCode, resp.Header.Get("Content-Type"), raw)
	}
	if got := describeParts(llm.seen()); got != "image/png:cat.png,text:what is this?" {
		t.Errorf("model saw %s", got)
	}

	body, contentType = imageAndPDF(t)
	resp, err = http.Post(ts.URL+"/api/v1/agent/stream", contentType, body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	raw, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(raw), interfaces.ErrContentUnsupported.Error()) {
		t.Errorf("response = %d %s", resp.StatusCode, raw)
	}
	if last := s.conversationHistory[len(s.conversationHistory)-1]; last.Role != "error" {
		t.Errorf("last history entry = %+v", last)
	}

	// A custom stream function refuses media when the stream starts.
	custom := serveUI(t, NewHTTPServerWithUI(newCustomAgent(t, nil, replayStream()), 0, uiConfig()))
	body, contentType = imageAndPDF(t)
	resp, err = http.Post(custom.URL+"/api/v1/agent/stream", contentType, body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("custom stream status = %d", resp.StatusCode)
	}
}
