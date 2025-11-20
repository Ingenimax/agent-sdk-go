package microservice

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// HTTPServer provides HTTP/SSE endpoints for agent streaming
type HTTPServer struct {
	agent  *agent.Agent
	port   int
	server *http.Server
}

// StreamRequest represents the JSON request for streaming
type StreamRequest struct {
	Input          string            `json:"input"`
	OrgID          string            `json:"org_id,omitempty"`
	ConversationID string            `json:"conversation_id,omitempty"`
	Context        map[string]string `json:"context,omitempty"`
	MaxIterations  int               `json:"max_iterations,omitempty"`
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
	return &HTTPServer{
		agent: agent,
		port:  port,
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

	// Serve static files for browser example (if they exist)
	mux.Handle("/", http.FileServer(http.Dir("./web/")))

	h.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", h.port),
		Handler:      corsHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // Longer timeout for streaming
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

	var req StreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if req.Input == "" {
		http.Error(w, "Input is required", http.StatusBadRequest)
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

	// Parse request
	var req StreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if req.Input == "" {
		http.Error(w, "Input is required", http.StatusBadRequest)
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
