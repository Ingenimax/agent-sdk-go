package microservice

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// UIConfig represents UI configuration options
type UIConfig struct {
	Enabled     bool       `json:"enabled"`
	DefaultPath string     `json:"default_path"`
	DevMode     bool       `json:"dev_mode"`
	Theme       string     `json:"theme"`
	Features    UIFeatures `json:"features"`
}

// UIFeatures represents available UI features
type UIFeatures struct {
	Chat      bool `json:"chat"`
	Memory    bool `json:"memory"`
	AgentInfo bool `json:"agent_info"`
	Settings  bool `json:"settings"`
}

// HTTPServerWithUI extends HTTPServer with embedded UI
type HTTPServerWithUI struct {
	HTTPServer // Embed the base HTTPServer
	uiConfig   *UIConfig
	uiFS       fs.FS

	// Simple in-memory conversation storage
	conversationHistory []MemoryEntry
}

// SubAgentInfo represents sub-agent information for UI
type SubAgentInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Model       string   `json:"model"`
	Status      string   `json:"status"`
	Tools       []string `json:"tools"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// AgentConfigResponse represents detailed agent configuration
type AgentConfigResponse struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Model        string                 `json:"model"`
	SystemPrompt string                 `json:"system_prompt"`
	Tools        []string               `json:"tools"`
	Memory       MemoryInfo             `json:"memory"`
	SubAgents    []SubAgentInfo         `json:"sub_agents,omitempty"`
	Features     UIFeatures             `json:"features"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// MemoryInfo represents memory system information
type MemoryInfo struct {
	Type        string `json:"type"`
	Status      string `json:"status"`
	EntryCount  int    `json:"entry_count,omitempty"`
	MaxCapacity int    `json:"max_capacity,omitempty"`
}

// MemoryEntry represents a memory entry for the browser
type MemoryEntry struct {
	ID           string                 `json:"id"`
	Role         string                 `json:"role"`
	Content      string                 `json:"content"`
	Timestamp    int64                  `json:"timestamp"`
	ConversationID string               `json:"conversation_id,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// DelegateRequest represents a request to delegate to a sub-agent
type DelegateRequest struct {
	SubAgentID     string            `json:"sub_agent_id"`
	Task           string            `json:"task"`
	Context        map[string]string `json:"context,omitempty"`
	ConversationID string            `json:"conversation_id,omitempty"`
}

// Embed UI files (will be populated at build time)
//go:embed all:ui-nextjs/out
var defaultUIFiles embed.FS

// NewHTTPServerWithUI creates a new HTTP server with embedded UI
func NewHTTPServerWithUI(agent *agent.Agent, port int, config *UIConfig) *HTTPServerWithUI {
	if config == nil {
		config = &UIConfig{
			Enabled:     true,
			DefaultPath: "/",
			DevMode:     false,
			Theme:       "light",
			Features: UIFeatures{
				Chat:      true,
				Memory:    true,
				AgentInfo: true,
				Settings:  true,
			},
		}
	}

	// Extract the embedded UI files
	var uiFS fs.FS
	var err error
	uiFS, err = fs.Sub(defaultUIFiles, "ui-nextjs/out")
	if err != nil {
		// Fallback to serving from local directory in dev mode
		if config.DevMode {
			uiFS = os.DirFS("./pkg/microservice/ui-nextjs/out")
		}
	}

	return &HTTPServerWithUI{
		HTTPServer: HTTPServer{
			agent: agent,
			port:  port,
		},
		uiConfig:            config,
		uiFS:                uiFS,
		conversationHistory: make([]MemoryEntry, 0),
	}
}

// Start starts the HTTP server with UI
func (h *HTTPServerWithUI) Start() error {
	mux := http.NewServeMux()

	// Add CORS middleware
	corsHandler := h.addCORS(mux)

	// Register API endpoints
	h.registerAPIEndpoints(mux)

	// Debug endpoint to list embedded files
	mux.HandleFunc("/debug/files", func(w http.ResponseWriter, r *http.Request) {
		if h.uiFS != nil {
			var files []string
			err := fs.WalkDir(h.uiFS, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				files = append(files, path)
				return nil
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(files)
		} else {
			http.Error(w, "No UI filesystem", http.StatusNotFound)
		}
	})

	// Serve UI if enabled
	if h.uiConfig.Enabled && h.uiFS != nil {
		// Serve the embedded UI files
		fileServer := http.FileServer(http.FS(h.uiFS))

		// Handle static assets specifically
		mux.Handle("/_next/", fileServer)
		mux.Handle("/favicon.ico", fileServer)

		// Handle root and everything else
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// For non-API requests, serve the index.html
			if !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/health") {
				// Try to serve the file first
				if file, err := h.uiFS.Open(strings.TrimPrefix(r.URL.Path, "/")); err == nil {
					file.Close()
					fileServer.ServeHTTP(w, r)
					return
				}
				// Fallback to index.html for SPA routing
				r.URL.Path = "/"
			}
			fileServer.ServeHTTP(w, r)
		})
	}

	h.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", h.port),
		Handler:      corsHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // Longer timeout for streaming
		IdleTimeout:  60 * time.Second,
	}

	fmt.Printf("HTTP server with UI starting on port %d\n", h.port)
	if h.uiConfig.Enabled {
		fmt.Printf("UI available at: http://localhost:%d%s\n", h.port, h.uiConfig.DefaultPath)
	}
	fmt.Printf("API endpoints available:\n")
	fmt.Printf("  - POST /api/v1/agent/run (non-streaming)\n")
	fmt.Printf("  - POST /api/v1/agent/stream (SSE streaming)\n")
	fmt.Printf("  - GET /api/v1/agent/metadata\n")
	fmt.Printf("  - GET /api/v1/agent/config\n")
	fmt.Printf("  - GET /api/v1/agent/subagents\n")
	fmt.Printf("  - POST /api/v1/agent/delegate\n")
	fmt.Printf("  - GET /api/v1/memory\n")
	fmt.Printf("  - GET /api/v1/memory/search\n")
	fmt.Printf("  - GET /api/v1/tools\n")
	fmt.Printf("  - GET /health\n")

	return h.server.ListenAndServe()
}


// registerAPIEndpoints registers all API endpoints
func (h *HTTPServerWithUI) registerAPIEndpoints(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("/health", h.handleHealth)

	// Agent endpoints with org context middleware
	mux.HandleFunc("/api/v1/agent/run", h.withOrgContext(h.handleRun))
	mux.HandleFunc("/api/v1/agent/stream", h.withOrgContext(h.handleStream))
	mux.HandleFunc("/api/v1/agent/metadata", h.handleMetadata)
	mux.HandleFunc("/api/v1/agent/config", h.handleConfig)
	mux.HandleFunc("/api/v1/agent/subagents", h.handleSubAgents)
	mux.HandleFunc("/api/v1/agent/delegate", h.withOrgContext(h.handleDelegate))

	// Memory endpoints
	mux.HandleFunc("/api/v1/memory", h.handleMemory)
	mux.HandleFunc("/api/v1/memory/search", h.handleMemorySearch)

	// Tools endpoint
	mux.HandleFunc("/api/v1/tools", h.handleTools)

	// WebSocket endpoint for real-time chat
	mux.HandleFunc("/ws/chat", h.handleWebSocketChat)
}

// handleConfig provides detailed agent configuration
func (h *HTTPServerWithUI) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Get agent tools
	tools := []string{}
	if toolsGetter, ok := interface{}(h.agent).(interface{ GetTools() []string }); ok {
		tools = toolsGetter.GetTools()
	}

	// Get system prompt
	systemPrompt := ""
	if promptGetter, ok := interface{}(h.agent).(interface{ GetSystemPrompt() string }); ok {
		systemPrompt = promptGetter.GetSystemPrompt()
	}

	// Get model info
	model := "unknown"
	if llm := h.agent.GetLLM(); llm != nil {
		if modelGetter, ok := llm.(interface{ GetModel() string }); ok {
			model = modelGetter.GetModel()
		}
	}

	// Get memory info
	memInfo := MemoryInfo{
		Type:   "none",
		Status: "inactive",
	}
	if memGetter, ok := interface{}(h.agent).(interface{ GetMemory() interfaces.Memory }); ok {
		if mem := memGetter.GetMemory(); mem != nil {
			memInfo.Type = "conversation"
			memInfo.Status = "active"
		}
	}

	response := AgentConfigResponse{
		Name:         h.agent.GetName(),
		Description:  h.agent.GetDescription(),
		Model:        model,
		SystemPrompt: systemPrompt,
		Tools:        tools,
		Memory:       memInfo,
		Features:     h.uiConfig.Features,
		SubAgents:    h.getSubAgentsList(),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleSubAgents provides list of sub-agents
func (h *HTTPServerWithUI) handleSubAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	subAgents := h.getSubAgentsList()

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"sub_agents": subAgents,
		"count":      len(subAgents),
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleDelegate handles delegation to sub-agents
func (h *HTTPServerWithUI) handleDelegate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DelegateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Build context
	ctx := r.Context()
	if req.ConversationID != "" {
		ctx = memory.WithConversationID(ctx, req.ConversationID)
	}

	// Here you would implement the actual delegation logic
	// For now, we'll return a placeholder response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "delegated",
		"sub_agent_id": req.SubAgentID,
		"task":         req.Task,
		"result":       "Sub-agent delegation not yet implemented",
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleMemory provides memory browser functionality
func (h *HTTPServerWithUI) handleMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Parse query parameters
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get conversation history
	entries := h.getConversationHistory(limit, offset)

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": entries,
		"total":   len(h.conversationHistory),
		"limit":   limit,
		"offset":  offset,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleMemorySearch provides memory search functionality
func (h *HTTPServerWithUI) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Search conversation history
	results := h.searchConversationHistory(query)

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"query":   query,
		"results": results,
		"count":   len(results),
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleTools provides list of available tools
func (h *HTTPServerWithUI) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	tools := []map[string]interface{}{}

	// Get tools from agent if available
	if toolsGetter, ok := interface{}(h.agent).(interface{ GetTools() []string }); ok {
		for _, toolName := range toolsGetter.GetTools() {
			tools = append(tools, map[string]interface{}{
				"name":        toolName,
				"description": fmt.Sprintf("Tool: %s", toolName),
				"enabled":     true,
			})
		}
	}

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": tools,
		"count": len(tools),
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleWebSocketChat handles WebSocket connections for real-time chat
func (h *HTTPServerWithUI) handleWebSocketChat(w http.ResponseWriter, r *http.Request) {
	// WebSocket implementation would go here
	// For now, return not implemented
	http.Error(w, "WebSocket not yet implemented", http.StatusNotImplemented)
}

// getSubAgentsList returns list of sub-agents
func (h *HTTPServerWithUI) getSubAgentsList() []SubAgentInfo {
	// This would need actual implementation based on your agent structure
	// For now, return empty list
	return []SubAgentInfo{}
}

// addConversationEntry adds a new entry to conversation history
func (h *HTTPServerWithUI) addConversationEntry(role, content, conversationID string) {
	entry := MemoryEntry{
		ID:             fmt.Sprintf("mem_%d_%s", time.Now().UnixNano(), role),
		Role:           role,
		Content:        content,
		Timestamp:      time.Now().UnixMilli(),
		ConversationID: conversationID,
		Metadata:       make(map[string]interface{}),
	}

	h.conversationHistory = append(h.conversationHistory, entry)

	// Keep only last 1000 entries to prevent memory bloat
	if len(h.conversationHistory) > 1000 {
		h.conversationHistory = h.conversationHistory[len(h.conversationHistory)-1000:]
	}
}

// getConversationHistory returns conversation history with pagination
func (h *HTTPServerWithUI) getConversationHistory(limit, offset int) []MemoryEntry {
	// First, try to get from agent's memory system if available
	if memGetter, ok := interface{}(h.agent).(interface{ GetMemory() interfaces.Memory }); ok {
		if mem := memGetter.GetMemory(); mem != nil {
			return h.getMemoryFromAgent(mem, limit, offset)
		}
	}

	// Fallback to our in-memory storage
	total := len(h.conversationHistory)

	if offset >= total {
		return []MemoryEntry{}
	}

	end := offset + limit
	if end > total {
		end = total
	}

	// Return most recent entries first (reverse order)
	result := make([]MemoryEntry, 0, end-offset)
	for i := total - 1 - offset; i >= total-end; i-- {
		if i >= 0 {
			result = append(result, h.conversationHistory[i])
		}
	}

	return result
}

// getMemoryFromAgent retrieves memory from the agent's memory system (Redis, etc.)
func (h *HTTPServerWithUI) getMemoryFromAgent(mem interfaces.Memory, limit, offset int) []MemoryEntry {
	ctx := context.Background()

	// Try to get messages from the agent's memory system
	messages, err := mem.GetMessages(ctx, interfaces.WithLimit(limit+offset))
	if err != nil {
		// If we can't get from agent memory, fall back to our local storage
		return h.conversationHistory
	}

	// Convert agent memory messages to UI memory entries
	entries := make([]MemoryEntry, 0, len(messages))
	for i, msg := range messages {
		// Skip offset entries
		if i < offset {
			continue
		}

		entry := MemoryEntry{
			ID:             fmt.Sprintf("agent_mem_%d", i),
			Role:           string(msg.Role),
			Content:        msg.Content,
			Timestamp:      h.extractTimestamp(msg.Metadata),
			ConversationID: h.extractConversationID(msg.Metadata),
			Metadata:       msg.Metadata,
		}
		entries = append(entries, entry)
	}

	// If we got entries from agent memory, return them
	if len(entries) > 0 {
		return entries
	}

	// Otherwise fall back to local storage
	return h.conversationHistory
}

// extractTimestamp extracts timestamp from message metadata
func (h *HTTPServerWithUI) extractTimestamp(metadata map[string]interface{}) int64 {
	if metadata == nil {
		return time.Now().UnixMilli()
	}

	// Try different timestamp formats
	if ts, ok := metadata["timestamp"].(int64); ok {
		// Convert nanoseconds to milliseconds if needed
		if ts > 1e15 { // If it looks like nanoseconds
			return ts / 1e6
		}
		return ts
	}

	if ts, ok := metadata["timestamp"].(float64); ok {
		return int64(ts)
	}

	if timeStr, ok := metadata["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
			return t.UnixMilli()
		}
	}

	return time.Now().UnixMilli()
}

// extractConversationID extracts conversation ID from message metadata
func (h *HTTPServerWithUI) extractConversationID(metadata map[string]interface{}) string {
	if metadata == nil {
		return "default"
	}

	if convID, ok := metadata["conversation_id"].(string); ok {
		return convID
	}

	if convID, ok := metadata["conversationId"].(string); ok {
		return convID
	}

	return "default"
}

// searchConversationHistory searches through conversation history
func (h *HTTPServerWithUI) searchConversationHistory(query string) []MemoryEntry {
	if query == "" {
		return h.getConversationHistory(50, 0)
	}

	query = strings.ToLower(query)
	var results []MemoryEntry

	for i := len(h.conversationHistory) - 1; i >= 0; i-- {
		entry := h.conversationHistory[i]
		if strings.Contains(strings.ToLower(entry.Content), query) ||
			strings.Contains(strings.ToLower(entry.Role), query) {
			results = append(results, entry)
			if len(results) >= 50 { // Limit search results
				break
			}
		}
	}

	return results
}

// Helper methods inherited from HTTPServer

// withOrgContext adds organization context to HTTP requests
func (h *HTTPServerWithUI) withOrgContext(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Check if organization ID is already in context
		if !multitenancy.HasOrgID(ctx) {
			// Add default organization ID
			ctx = multitenancy.WithOrgID(ctx, "default-org")
			r = r.WithContext(ctx)
		}

		handler(w, r)
	}
}




