package microservice

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

const (
	// memory used by ParseMultipartForm; larger parts may spill to disk automatically.
	maxMultipartMemoryBytes = 32 << 20 // 32MB
	// maximum bytes read per uploaded image file.
	maxImageBytes = 20 << 20 // 20MB
)

type uploadMode string

const (
	uploadModeDataURL uploadMode = "data_url"
	uploadModeStore   uploadMode = "store"
)

// HTTPServer provides HTTP/SSE endpoints for agent streaming
type HTTPServer struct {
	agent     *agent.Agent
	port      int
	server    *http.Server
	uploadDir string
}

// StreamRequest represents the JSON request for streaming
type StreamRequest struct {
	Input          string                   `json:"input"`
	ContentParts   []interfaces.ContentPart `json:"content_parts,omitempty"`
	OrgID          string                   `json:"org_id,omitempty"`
	ConversationID string                   `json:"conversation_id,omitempty"`
	Context        map[string]string        `json:"context,omitempty"`
	MaxIterations  int                      `json:"max_iterations,omitempty"`
}

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	Event     string      `json:"event"`
	Data      interface{} `json:"data"`
	ID        string      `json:"id,omitempty"`
	Retry     int         `json:"retry,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// StreamEventData represents the data structure for streaming events
type StreamEventData struct {
	Type         string                 `json:"type"`
	Content      string                 `json:"content,omitempty"`
	ThinkingStep string                 `json:"thinking_step,omitempty"`
	ToolCall     *ToolCallData          `json:"tool_call,omitempty"`
	Error        string                 `json:"error,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	IsFinal      bool                   `json:"is_final"`
	Timestamp    int64                  `json:"timestamp"`
}

// ToolCallData represents tool call information for HTTP/SSE
type ToolCallData struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
	Result    string `json:"result,omitempty"`
	Status    string `json:"status"`
}

// NewHTTPServer creates a new HTTP server for agent streaming
func NewHTTPServer(agent *agent.Agent, port int) *HTTPServer {
	// Security default: disable store mode unless explicitly enabled.
	// When set, uploaded files will be stored on disk and served via /api/v1/files/*.
	uploadDir := strings.TrimSpace(os.Getenv("AGENT_SDK_UPLOAD_DIR"))

	return &HTTPServer{
		agent:     agent,
		port:      port,
		uploadDir: uploadDir,
	}
}

// Start starts the HTTP server
func (h *HTTPServer) Start() error {
	mux := http.NewServeMux()

	// Add CORS middleware
	corsHandler := h.addCORS(mux)

	// Register endpoints
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/api/v1/agent/run", h.handleRun)
	mux.HandleFunc("/api/v1/agent/stream", h.handleStream)
	mux.HandleFunc("/api/v1/agent/metadata", h.handleMetadata)
	if h.uploadDir != "" {
		mux.HandleFunc("/api/v1/files/", h.handleFileDownload)
	}

	// Serve static files for browser example (if they exist)
	mux.Handle("/", http.FileServer(http.Dir("./web/")))

	h.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", h.port),
		Handler:      corsHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 15 * time.Minute, // Longer timeout for streaming
		IdleTimeout:  60 * time.Second,
	}

	fmt.Printf("HTTP server starting on port %d\n", h.port)
	fmt.Printf("Endpoints available:\n")
	fmt.Printf("  - POST /api/v1/agent/run (non-streaming)\n")
	fmt.Printf("  - POST /api/v1/agent/stream (SSE streaming)\n")
	fmt.Printf("  - GET /api/v1/agent/metadata\n")
	fmt.Printf("  - GET /health\n")

	return h.server.ListenAndServe()
}

// Stop stops the HTTP server
func (h *HTTPServer) Stop(ctx context.Context) error {
	if h.server != nil {
		return h.server.Shutdown(ctx)
	}
	return nil
}

// addCORS adds CORS headers to allow browser access
func (h *HTTPServer) addCORS(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

// handleHealth provides a health check endpoint
func (h *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"agent":  h.agent.GetName(),
		"time":   time.Now().Unix(),
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleRun provides non-streaming agent execution
func (h *HTTPServer) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, contentParts, err := h.parseAgentRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build context
	ctx := r.Context()
	if req.OrgID != "" {
		ctx = multitenancy.WithOrgID(ctx, req.OrgID)
	}
	if req.ConversationID != "" {
		ctx = memory.WithConversationID(ctx, req.ConversationID)
	}

	// Attach multimodal content parts (if provided)
	if len(contentParts) > 0 {
		ctx = interfaces.WithContextContentParts(ctx, contentParts...)
	}

	// Execute agent with detailed tracking
	response, err := h.agent.RunDetailed(ctx, req.Input)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Log detailed execution information
	{
		executionDetails := map[string]interface{}{
			"endpoint":          "agent_run",
			"agent_name":        response.AgentName,
			"model_used":        response.Model,
			"response_length":   len(response.Content),
			"llm_calls":         response.ExecutionSummary.LLMCalls,
			"tool_calls":        response.ExecutionSummary.ToolCalls,
			"sub_agent_calls":   response.ExecutionSummary.SubAgentCalls,
			"execution_time_ms": response.ExecutionSummary.ExecutionTimeMs,
			"used_tools":        response.ExecutionSummary.UsedTools,
			"used_sub_agents":   response.ExecutionSummary.UsedSubAgents,
		}
		if response.Usage != nil {
			executionDetails["input_tokens"] = response.Usage.InputTokens
			executionDetails["output_tokens"] = response.Usage.OutputTokens
			executionDetails["total_tokens"] = response.Usage.TotalTokens
			executionDetails["reasoning_tokens"] = response.Usage.ReasoningTokens
		}
		log.Printf("[HTTP Server] Agent execution completed via HTTP API: %+v", executionDetails)
	}

	// Return result with execution details
	w.Header().Set("Content-Type", "application/json")
	responseData := map[string]interface{}{
		"output":            response.Content,
		"agent":             response.AgentName,
		"execution_summary": response.ExecutionSummary,
	}
	if response.Usage != nil {
		responseData["usage"] = response.Usage
	}
	if err := json.NewEncoder(w).Encode(responseData); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleStream provides SSE streaming endpoint
func (h *HTTPServer) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, contentParts, err := h.parseAgentRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Get flusher for immediate response sending
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Build context
	ctx := r.Context()
	if req.OrgID != "" {
		ctx = multitenancy.WithOrgID(ctx, req.OrgID)
	}
	if req.ConversationID != "" {
		ctx = memory.WithConversationID(ctx, req.ConversationID)
	}

	// Attach multimodal content parts (if provided)
	if len(contentParts) > 0 {
		ctx = interfaces.WithContextContentParts(ctx, contentParts...)
	}

	// Check if agent supports streaming
	streamingAgent, ok := interface{}(h.agent).(interfaces.StreamingAgent)
	if !ok {
		// Fall back to non-streaming execution
		response, err := h.agent.RunDetailed(ctx, req.Input)
		if err != nil {
			h.sendSSEEvent(w, flusher, "error", StreamEventData{
				Type:    "error",
				Error:   err.Error(),
				IsFinal: true,
			})
			return
		}

		// Log detailed execution information for streaming fallback
		{
			executionDetails := map[string]interface{}{
				"endpoint":          "agent_stream_fallback",
				"agent_name":        response.AgentName,
				"model_used":        response.Model,
				"response_length":   len(response.Content),
				"llm_calls":         response.ExecutionSummary.LLMCalls,
				"tool_calls":        response.ExecutionSummary.ToolCalls,
				"sub_agent_calls":   response.ExecutionSummary.SubAgentCalls,
				"execution_time_ms": response.ExecutionSummary.ExecutionTimeMs,
				"used_tools":        response.ExecutionSummary.UsedTools,
				"used_sub_agents":   response.ExecutionSummary.UsedSubAgents,
			}
			if response.Usage != nil {
				executionDetails["input_tokens"] = response.Usage.InputTokens
				executionDetails["output_tokens"] = response.Usage.OutputTokens
				executionDetails["total_tokens"] = response.Usage.TotalTokens
				executionDetails["reasoning_tokens"] = response.Usage.ReasoningTokens
			}
			log.Printf("[HTTP Server] Agent execution completed via streaming fallback: %+v", executionDetails)
		}

		h.sendSSEEvent(w, flusher, "content", StreamEventData{
			Type:    "content",
			Content: response.Content,
			IsFinal: true,
		})
		return
	}

	// Start streaming
	eventChan, err := streamingAgent.RunStream(ctx, req.Input)
	if err != nil {
		h.sendSSEEvent(w, flusher, "error", StreamEventData{
			Type:    "error",
			Error:   err.Error(),
			IsFinal: true,
		})
		return
	}

	// Send initial connection event
	h.sendSSEEvent(w, flusher, "connected", StreamEventData{
		Type: "connected",
		Metadata: map[string]interface{}{
			"agent": h.agent.GetName(),
		},
	})

	// Stream events to client
	eventID := 0
	for event := range eventChan {
		eventID++

		// Convert agent event to HTTP event data
		eventData := h.convertAgentEventToHTTPEvent(event)

		// Determine event type for SSE
		var sseEventType string
		switch event.Type {
		case interfaces.AgentEventContent:
			sseEventType = "content"
		case interfaces.AgentEventThinking:
			sseEventType = "thinking"
		case interfaces.AgentEventToolCall:
			sseEventType = "tool_call"
		case interfaces.AgentEventToolResult:
			sseEventType = "tool_result"
		case interfaces.AgentEventError:
			sseEventType = "error"
		case interfaces.AgentEventComplete:
			sseEventType = "complete"
			eventData.IsFinal = true
		default:
			sseEventType = "content"
		}

		// Send SSE event
		h.sendSSEEventWithID(w, flusher, sseEventType, eventData, strconv.Itoa(eventID))

		// Check if client disconnected
		select {
		case <-ctx.Done():
			return
		default:
		}
	}

	// Send final completion event
	h.sendSSEEvent(w, flusher, "done", StreamEventData{
		Type:    "done",
		IsFinal: true,
	})
}

func hasAnyTextPart(parts []interfaces.ContentPart) bool {
	for _, p := range parts {
		if p.Type == "text" {
			return true
		}
	}
	return false
}

func (h *HTTPServer) parseAgentRequest(r *http.Request) (StreamRequest, []interfaces.ContentPart, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return h.parseMultipartAgentRequest(r)
	}
	return h.parseJSONAgentRequest(r)
}

func (h *HTTPServer) parseJSONAgentRequest(r *http.Request) (StreamRequest, []interfaces.ContentPart, error) {
	var req StreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return StreamRequest{}, nil, fmt.Errorf("Invalid JSON: %w", err)
	}

	if req.Input == "" && len(req.ContentParts) == 0 {
		return StreamRequest{}, nil, errors.New("Either 'input' or 'content_parts' is required")
	}

	contentParts := req.ContentParts
	if err := validateContentParts(contentParts); err != nil {
		return StreamRequest{}, nil, err
	}
	if len(contentParts) > 0 && req.Input != "" && !hasAnyTextPart(contentParts) {
		contentParts = append([]interfaces.ContentPart{interfaces.TextPart(req.Input)}, contentParts...)
	}

	return req, contentParts, nil
}

func (h *HTTPServer) parseMultipartAgentRequest(r *http.Request) (StreamRequest, []interfaces.ContentPart, error) {
	if err := r.ParseMultipartForm(maxMultipartMemoryBytes); err != nil {
		return StreamRequest{}, nil, fmt.Errorf("Invalid multipart form: %w", err)
	}

	var req StreamRequest
	req.Input = r.FormValue("input")
	req.OrgID = r.FormValue("org_id")
	req.ConversationID = r.FormValue("conversation_id")
	if maxIterStr := r.FormValue("max_iterations"); maxIterStr != "" {
		if v, err := strconv.Atoi(maxIterStr); err == nil {
			req.MaxIterations = v
		}
	}

	// Optional: allow passing JSON content_parts field in multipart.
	var contentParts []interfaces.ContentPart
	if rawParts := r.FormValue("content_parts"); strings.TrimSpace(rawParts) != "" {
		if err := json.Unmarshal([]byte(rawParts), &contentParts); err != nil {
			return StreamRequest{}, nil, fmt.Errorf("Invalid content_parts JSON: %w", err)
		}
		if err := validateContentParts(contentParts); err != nil {
			return StreamRequest{}, nil, err
		}
	}

	mode := uploadMode(strings.TrimSpace(r.FormValue("upload_mode")))
	if mode == "" {
		mode = uploadModeDataURL
	}
	detail := strings.TrimSpace(r.FormValue("detail")) // "low" | "high" | "auto" | ""

	files := []*multipart.FileHeader{}
	if r.MultipartForm != nil {
		files = append(files, r.MultipartForm.File["images"]...)
		files = append(files, r.MultipartForm.File["image"]...)
	}

	for _, fh := range files {
		part, err := h.contentPartFromUploadedFile(r, fh, mode, detail)
		if err != nil {
			return StreamRequest{}, nil, err
		}
		if part.Type != "" {
			contentParts = append(contentParts, part)
		}
	}

	if req.Input == "" && len(contentParts) == 0 {
		return StreamRequest{}, nil, errors.New("Either 'input' or 'content_parts' is required")
	}

	// Back-compat: if both input and content_parts are provided and no text part exists, prepend input.
	if len(contentParts) > 0 && req.Input != "" && !hasAnyTextPart(contentParts) {
		contentParts = append([]interfaces.ContentPart{interfaces.TextPart(req.Input)}, contentParts...)
	}

	return req, contentParts, nil
}

func (h *HTTPServer) contentPartFromUploadedFile(r *http.Request, fh *multipart.FileHeader, mode uploadMode, detail string) (interfaces.ContentPart, error) {
	if fh == nil {
		return interfaces.ContentPart{}, nil
	}
	if fh.Size > maxImageBytes {
		return interfaces.ContentPart{}, fmt.Errorf("image too large: %d bytes (max %d)", fh.Size, maxImageBytes)
	}

	f, err := fh.Open()
	if err != nil {
		return interfaces.ContentPart{}, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
	if err != nil {
		return interfaces.ContentPart{}, fmt.Errorf("failed to read uploaded file: %w", err)
	}
	if int64(len(data)) > maxImageBytes {
		return interfaces.ContentPart{}, fmt.Errorf("image too large: %d bytes (max %d)", len(data), maxImageBytes)
	}

	mimeType := fh.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if !isAllowedImageMIME(mimeType) {
		return interfaces.ContentPart{}, fmt.Errorf("unsupported image content-type: %s", mimeType)
	}

	switch mode {
	case uploadModeStore:
		if h.uploadDir == "" {
			return interfaces.ContentPart{}, errors.New("upload_mode=store is not enabled on this server")
		}
		if err := os.MkdirAll(h.uploadDir, 0o755); err != nil {
			return interfaces.ContentPart{}, fmt.Errorf("failed to create upload dir: %w", err)
		}
		name, err := randomFilename(filepath.Base(fh.Filename))
		if err != nil {
			return interfaces.ContentPart{}, fmt.Errorf("failed to generate filename: %w", err)
		}
		destPath := filepath.Join(h.uploadDir, name)
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return interfaces.ContentPart{}, fmt.Errorf("failed to save file: %w", err)
		}
		u := h.fileURL(r, name)
		return interfaces.ImageURLPart(u, detail), nil

	case uploadModeDataURL, "":
		encoded := base64.StdEncoding.EncodeToString(data)
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
		return interfaces.ImageURLPart(dataURL, detail), nil

	default:
		return interfaces.ContentPart{}, fmt.Errorf("unsupported upload_mode: %s", mode)
	}
}

func isAllowedImageMIME(mime string) bool {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}

func validateContentParts(parts []interfaces.ContentPart) error {
	for _, p := range parts {
		if p.Type != "image_url" {
			continue
		}
		if p.ImageURL == nil || strings.TrimSpace(p.ImageURL.URL) == "" {
			return errors.New("Invalid content_parts: image_url.url is required when type is 'image_url'")
		}
		u := strings.TrimSpace(p.ImageURL.URL)
		if !isAllowedImageURLScheme(u) {
			return fmt.Errorf("Invalid image_url: must start with http://, https://, or data:")
		}
		// Minimal additional hardening: if it's a data URL, require image/* prefix.
		if strings.HasPrefix(u, "data:") && !strings.HasPrefix(strings.ToLower(u), "data:image/") {
			return fmt.Errorf("Invalid image_url data URL: must be data:image/*")
		}
	}
	return nil
}

func isAllowedImageURLScheme(u string) bool {
	u = strings.ToLower(u)
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "data:")
}

func randomFilename(original string) (string, error) {
	// Keep a stable extension if present.
	ext := strings.ToLower(filepath.Ext(original))
	if ext == "" || strings.Contains(ext, "/") {
		ext = ""
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	hex := base64.RawURLEncoding.EncodeToString(b[:])
	return hex + ext, nil
}

func (h *HTTPServer) fileURL(r *http.Request, name string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		// Prefer proxy-provided scheme when present.
		scheme = strings.Split(xf, ",")[0]
		scheme = strings.TrimSpace(scheme)
	}
	host := r.Host
	return fmt.Sprintf("%s://%s/api/v1/files/%s", scheme, host, name)
}

// handleFileDownload serves a previously uploaded file by name.
// This is used by upload_mode=store so that providers can fetch the image by URL.
func (h *HTTPServer) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.uploadDir == "" {
		http.Error(w, "File serving not enabled", http.StatusNotFound)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/v1/files/")
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") || strings.Contains(name, string(os.PathSeparator)) {
		http.Error(w, "Invalid file name", http.StatusBadRequest)
		return
	}

	// Ensure file path stays within uploadDir.
	destPath := filepath.Join(h.uploadDir, name)
	rel, err := filepath.Rel(h.uploadDir, destPath)
	if err != nil || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	http.ServeFile(w, r, destPath)
}

// handleMetadata provides agent metadata
func (h *HTTPServer) handleMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check if agent supports streaming
	_, supportsStreaming := interface{}(h.agent).(interfaces.StreamingAgent)

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"name":               h.agent.GetName(),
		"description":        h.agent.GetDescription(),
		"supports_streaming": supportsStreaming,
		"capabilities": []string{
			"run",
			"stream",
			"metadata",
		},
		"endpoints": map[string]string{
			"run":      "/api/v1/agent/run",
			"stream":   "/api/v1/agent/stream",
			"metadata": "/api/v1/agent/metadata",
			"health":   "/health",
		},
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// convertAgentEventToHTTPEvent converts agent stream events to HTTP event format
func (h *HTTPServer) convertAgentEventToHTTPEvent(event interfaces.AgentStreamEvent) StreamEventData {
	eventData := StreamEventData{
		Type:     string(event.Type),
		Content:  event.Content,
		Metadata: event.Metadata,
		IsFinal:  false,
	}

	if event.ThinkingStep != "" {
		eventData.ThinkingStep = event.ThinkingStep
	}

	if event.ToolCall != nil {
		eventData.ToolCall = &ToolCallData{
			ID:        event.ToolCall.ID,
			Name:      event.ToolCall.Name,
			Arguments: event.ToolCall.Arguments,
			Result:    event.ToolCall.Result,
			Status:    event.ToolCall.Status,
		}
	}

	if event.Error != nil {
		eventData.Error = event.Error.Error()
	}

	return eventData
}

// sendSSEEvent sends a Server-Sent Event
func (h *HTTPServer) sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, data StreamEventData) {
	h.sendSSEEventWithID(w, flusher, eventType, data, "")
}

// sendSSEEventWithID sends a Server-Sent Event with ID
func (h *HTTPServer) sendSSEEventWithID(w http.ResponseWriter, flusher http.Flusher, eventType string, data StreamEventData, id string) {
	// Add timestamp
	data.Timestamp = time.Now().UnixMilli()

	// Convert data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		// Fallback to error event
		_, _ = fmt.Fprintf(w, "event: error\ndata: {\"error\": \"Failed to marshal event data\"}\n\n")
		flusher.Flush()
		return
	}

	// Write SSE event
	if id != "" {
		_, _ = fmt.Fprintf(w, "id: %s\n", id)
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", eventType)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", string(jsonData))

	flusher.Flush()
}
