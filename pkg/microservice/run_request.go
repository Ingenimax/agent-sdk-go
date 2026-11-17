package microservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/internal/multimodal"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// MaxRunRequestBytes caps the body of a run or stream request, files and
// base64 included. Base64 makes inline data in a JSON request about a third
// larger than the file it encodes.
const MaxRunRequestBytes = 32 << 20

// MaxRunRequestFiles caps the number of files, or of inline and file data
// parts, in one run or stream request.
const MaxRunRequestFiles = 10

// filesField is the multipart/form-data field that carries uploaded files.
const filesField = "files"

// requestError is an error in a run or stream request, with the HTTP status
// to answer it with.
type requestError struct {
	status int
	msg    string
}

func (e *requestError) Error() string { return e.msg }

// badRequest returns a requestError with status 400.
func badRequest(format string, args ...any) error {
	return &requestError{status: http.StatusBadRequest, msg: fmt.Sprintf(format, args...)}
}

// readError returns the requestError for an error reading the body: 413 when
// the body is over MaxRunRequestBytes, and 400 with the given message
// otherwise.
func readError(err error, format string, args ...any) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return &requestError{
			status: http.StatusRequestEntityTooLarge,
			msg:    fmt.Sprintf("Request body is larger than %d bytes", tooLarge.Limit),
		}
	}
	return badRequest(format, args...)
}

// writeRequestError answers a request with err: its status and message when
// it is a requestError, and 400 otherwise.
func writeRequestError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	var reqErr *requestError
	if errors.As(err, &reqErr) {
		status = reqErr.status
	}
	http.Error(w, err.Error(), status)
}

// runErrorStatus returns the HTTP status for an error from running the agent:
// 422 when the agent or its model cannot take a part of the content, and
// fallback otherwise.
func runErrorStatus(err error, fallback int) int {
	if errors.Is(err, interfaces.ErrContentUnsupported) {
		return http.StatusUnprocessableEntity
	}
	return fallback
}

// writeJSONError answers with status and a JSON body of fields plus the error
// message under "error".
func writeJSONError(w http.ResponseWriter, status int, err error, fields map[string]any) {
	body := map[string]any{"error": err.Error()}
	for k, v := range fields {
		body[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// parseRunRequest reads a run or stream request and returns its fields and the
// content to run the agent with.
//
// A multipart/form-data body has the text fields input, org_id, and
// conversation_id, and up to MaxRunRequestFiles files in the "files" field.
// Any other body is read as JSON, the original format: a StreamRequest whose
// optional content field carries an interfaces.Content.
//
// The content is the parts of the request's content, or its files, followed
// by input as a text part. The body is capped at MaxRunRequestBytes. Errors
// are requestErrors that carry the HTTP status for the response.
func parseRunRequest(w http.ResponseWriter, r *http.Request) (*StreamRequest, *interfaces.Content, error) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxRunRequestBytes)

	var req StreamRequest
	var err error
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType == "multipart/form-data" {
		err = parseMultipartRequest(r, &req)
	} else if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		err = readError(decodeErr, "Invalid JSON: %v", decodeErr)
	}
	if err != nil {
		return nil, nil, err
	}

	content, err := runContent(&req)
	if err != nil {
		return nil, nil, err
	}
	return &req, content, nil
}

// parseMultipartRequest reads a multipart/form-data body into req. Uploaded
// files become inline parts of req.Content, in the order they were sent.
func parseMultipartRequest(r *http.Request, req *StreamRequest) error {
	reader, err := r.MultipartReader()
	if err != nil {
		return badRequest("Invalid multipart form: %v", err)
	}

	var files []*interfaces.Part
	for {
		formPart, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return readError(err, "Invalid multipart form: %v", err)
		}
		file, err := readFormPart(formPart, req, len(files))
		_ = formPart.Close()
		if err != nil {
			return err
		}
		if file != nil {
			files = append(files, file)
		}
	}

	if len(files) > 0 {
		req.Content = &interfaces.Content{Parts: files}
	}
	return nil
}

// readFormPart reads one part of a multipart form. A text field is stored on
// req; fields other than input, org_id, and conversation_id are ignored. A
// file is returned as an inline part, with its type detected from its content.
// fileCount is the number of files read before this part.
func readFormPart(formPart *multipart.Part, req *StreamRequest, fileCount int) (*interfaces.Part, error) {
	name, fileName := formPart.FormName(), formPart.FileName()
	if fileName == "" {
		value, err := io.ReadAll(formPart)
		if err != nil {
			return nil, readError(err, "Invalid multipart field %q: %v", name, err)
		}
		switch name {
		case "input":
			req.Input = string(value)
		case "org_id":
			req.OrgID = string(value)
		case "conversation_id":
			req.ConversationID = string(value)
		}
		return nil, nil
	}

	if name != filesField {
		return nil, badRequest("Unexpected file field %q: upload files in the %q field", name, filesField)
	}
	if fileCount >= MaxRunRequestFiles {
		return nil, badRequest("Too many files: send at most %d per request", MaxRunRequestFiles)
	}
	data, err := io.ReadAll(formPart)
	if err != nil {
		return nil, readError(err, "Invalid file %q: %v", fileName, err)
	}
	part, err := multimodal.NewInlinePart(data, fileName)
	if err != nil {
		return nil, badRequest("File %q: %v", fileName, err)
	}
	return part, nil
}

// runContent returns the content to run for a request: the parts of its
// content, followed by its input as a text part. Every part is checked with
// checkPart, and at most MaxRunRequestFiles parts may carry data.
func runContent(req *StreamRequest) (*interfaces.Content, error) {
	var parts []*interfaces.Part
	if req.Content != nil {
		if req.Content.Role != "" && req.Content.Role != interfaces.MessageRoleUser {
			return nil, badRequest("Content role must be %q or empty, got %q", interfaces.MessageRoleUser, req.Content.Role)
		}
		parts = append(parts, req.Content.Parts...)
	}

	media := 0
	for i, p := range parts {
		if err := checkPart(p); err != nil {
			return nil, badRequest("Content part %d: %v", i, err)
		}
		if !p.IsText() {
			media++
		}
	}
	if media > MaxRunRequestFiles {
		return nil, badRequest("Too many files: send at most %d per request", MaxRunRequestFiles)
	}

	if req.Input != "" {
		parts = append(parts, interfaces.NewPartFromText(req.Input))
	}
	if len(parts) == 0 {
		return nil, badRequest("Input is required: send input text, content, or files")
	}
	return interfaces.NewContentFromParts(parts, interfaces.MessageRoleUser), nil
}

// checkPart reports whether a part of a request's content is one the server
// accepts.
//
// Inline data must be a supported type, detected from the data. The detected
// type replaces the declared one, so the model is told what the data really
// is. File data must declare a supported type and point at an https URL: other
// schemes, such as s3:// or gs://, would have a provider read storage with the
// server's credentials on behalf of the client. A part with no field set is
// refused, because it is usually a misspelled field, such as inline_data.
func checkPart(p *interfaces.Part) error {
	switch {
	case p == nil:
		return errors.New("the part is null")
	case p.InlineData != nil:
		if len(p.InlineData.Data) == 0 {
			return errors.New("inline data is empty")
		}
		mimeType := multimodal.DetectMIMEType(p.InlineData.Data)
		if !multimodal.IsSupported(mimeType) {
			return fmt.Errorf("inline data is %s: supported types are %s", mimeType, strings.Join(multimodal.SupportedMIMETypes(), ", "))
		}
		p.InlineData.MIMEType = mimeType
	case p.FileData != nil:
		if !multimodal.IsSupported(p.FileData.MIMEType) {
			return fmt.Errorf("file data type %q is not supported: supported types are %s", p.FileData.MIMEType, strings.Join(multimodal.SupportedMIMETypes(), ", "))
		}
		if u, err := url.Parse(p.FileData.FileURI); err != nil || u.Scheme != "https" || u.Host == "" {
			return errors.New("file data needs an https URL")
		}
	case p.Text == "":
		return errors.New("the part is empty: set text, inlineData, or fileData")
	}
	return p.Validate()
}

// streamEvents returns the events of a run stream as a sequence.
//
// When the content has media, it first waits for the first event: a model
// that cannot take a part refuses it before it sends anything else. That
// refusal is returned as the error, so the handler can answer with a 4xx
// status instead of starting an SSE response, and the rest of the stream is
// drained. Otherwise the first event leads the sequence. Text-only streams
// are not delayed.
func streamEvents(ctx context.Context, content *interfaces.Content, events <-chan interfaces.AgentStreamEvent) (iter.Seq[interfaces.AgentStreamEvent], error) {
	var first []interfaces.AgentStreamEvent
	if interfaces.HasMedia(content.Parts) {
		select {
		case event, ok := <-events:
			if ok && event.Type == interfaces.AgentEventError && errors.Is(event.Error, interfaces.ErrContentUnsupported) {
				go func() {
					for range events {
					}
				}()
				return nil, event.Error
			}
			if ok {
				first = append(first, event)
			}
		case <-ctx.Done():
		}
	}
	return func(yield func(interfaces.AgentStreamEvent) bool) {
		for _, event := range first {
			if !yield(event) {
				return
			}
		}
		for event := range events {
			if !yield(event) {
				return
			}
		}
	}, nil
}
