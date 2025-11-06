package microservice

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
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
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Model        string   `json:"model"`
	Status       string   `json:"status"`
	Tools        []string `json:"tools"`
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
	ID             string                 `json:"id"`
	Role           string                 `json:"role"`
	Content        string                 `json:"content"`
	Timestamp      int64                  `json:"timestamp"`
	ConversationID string                 `json:"conversation_id,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// ConversationInfo represents conversation metadata
type ConversationInfo struct {
	ID           string `json:"id"`
	MessageCount int    `json:"message_count"`
	LastActivity int64  `json:"last_activity"`
	LastMessage  string `json:"last_message,omitempty"`
}

// MemoryResponse represents the response structure for memory endpoints
type MemoryResponse struct {
	Mode           string             `json:"mode"` // "conversations" or "messages"
	Conversations  []ConversationInfo `json:"conversations,omitempty"`
	Messages       []MemoryEntry      `json:"messages,omitempty"`
	Total          int                `json:"total"`
	Limit          int                `json:"limit"`
	Offset         int                `json:"offset"`
	ConversationID string             `json:"conversation_id,omitempty"`
}

// DelegateRequest represents a request to delegate to a sub-agent
type DelegateRequest struct {
	SubAgentID     string            `json:"sub_agent_id"`
	Task           string            `json:"task"`
	Context        map[string]string `json:"context,omitempty"`
	ConversationID string            `json:"conversation_id,omitempty"`
}

// Embed UI files (will be populated at build time)
//
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
			_ = json.NewEncoder(w).Encode(files)
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
					_ = file.Close()
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
	fmt.Printf("  - GET /health\n")

	if h.uiConfig.Enabled {
		fmt.Printf("UI-specific endpoints:\n")
		fmt.Printf("  - GET /api/v1/agent/config\n")
		fmt.Printf("  - GET /api/v1/agent/subagents\n")
		fmt.Printf("  - POST /api/v1/agent/delegate\n")
		fmt.Printf("  - GET /api/v1/memory\n")
		fmt.Printf("  - GET /api/v1/memory/search\n")
		fmt.Printf("  - GET /api/v1/tools\n")
	}

	return h.server.ListenAndServe()
}

// registerAPIEndpoints registers all API endpoints
func (h *HTTPServerWithUI) registerAPIEndpoints(mux *http.ServeMux) {
	// Health check (always available)
	mux.HandleFunc("/health", h.handleHealth)

	// Core agent endpoints (always available)
	mux.HandleFunc("/api/v1/agent/run", h.withOrgContext(h.handleRun))
	mux.HandleFunc("/api/v1/agent/stream", h.withOrgContext(h.handleStream))
	mux.HandleFunc("/api/v1/agent/metadata", h.handleMetadata)

	// UI-specific endpoints (only when UI is enabled)
	if h.uiConfig.Enabled {
		mux.HandleFunc("/api/v1/agent/config", h.handleConfig)
		mux.HandleFunc("/api/v1/agent/subagents", h.handleSubAgents)
		mux.HandleFunc("/api/v1/agent/delegate", h.withOrgContext(h.handleDelegate))
		mux.HandleFunc("/api/v1/memory", h.withOrgContext(h.handleMemory))
		mux.HandleFunc("/api/v1/memory/search", h.withOrgContext(h.handleMemorySearch))
		mux.HandleFunc("/api/v1/tools", h.handleTools)
		mux.HandleFunc("/ws/chat", h.handleWebSocketChat)
	}
}

// handleConfig provides detailed agent configuration
func (h *HTTPServerWithUI) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Get agent tools - directly from agent interface
	tools := h.getToolNames()

	// Get system prompt - handle remote agents differently
	systemPrompt := h.getSystemPrompt()

	// Get model info - try to get from LLM
	model := h.getModelName()

	// Get memory info - directly from agent interface
	memInfo := h.getMemoryInfo()

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
	_ = ctx // TODO: Use ctx when implementing actual delegation logic

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

	conversationID := r.URL.Query().Get("conversation_id")

	var response MemoryResponse

	if conversationID != "" {
		// Get messages for specific conversation
		log.Printf("Getting messages for conversation: %s", conversationID)
		response = h.getConversationMessagesWithContext(r.Context(), conversationID, limit, offset)
	} else {
		// Get all conversations
		log.Println("Getting all conversations")
		response = h.getAllConversationsWithContext(r.Context(), limit, offset)
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
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

	// Check if agent is remote and handle accordingly
	if h.agent.IsRemote() {
		// For remote agents, get tools from system prompt or use alternative method
		// Parse system prompt to extract tool information
		systemPrompt := h.getSystemPrompt()
		toolNames := h.parseToolsFromSystemPrompt(systemPrompt)
		for _, toolName := range toolNames {
			tools = append(tools, map[string]interface{}{
				"name":        toolName,
				"description": "Remote agent tool",
				"enabled":     true,
			})
		}
	} else {
		// Get tools from local agent
		agentTools := h.agent.GetTools()
		for _, tool := range agentTools {
			tools = append(tools, map[string]interface{}{
				"name":        tool.Name(),
				"description": tool.Description(),
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
	subAgents := []SubAgentInfo{}

	// Check if agent is remote
	if h.agent.IsRemote() {
		// For remote agents, parse from system prompt
		systemPrompt := h.getSystemPrompt()
		toolNames := h.parseToolsFromSystemPrompt(systemPrompt)

		for _, toolName := range toolNames {
			if strings.HasSuffix(toolName, "_agent") {
				agentName := strings.TrimSuffix(toolName, "_agent")
				subAgent := SubAgentInfo{
					ID:           toolName,
					Name:         agentName,
					Description:  h.getToolDescriptionFromSystemPrompt(toolName, systemPrompt),
					Model:        "Remote",
					Status:       "active",
					Tools:        []string{toolName},
					Capabilities: []string{"Remote sub-agent"},
				}
				subAgents = append(subAgents, subAgent)
			}
		}
	} else {
		// Get sub-agents directly from the agent instance
		agentSubAgents := h.agent.GetSubAgents()
		for _, subAgent := range agentSubAgents {
			subAgentInfo := SubAgentInfo{
				ID:           subAgent.GetName(),
				Name:         subAgent.GetName(),
				Description:  subAgent.GetDescription(),
				Model:        h.getSubAgentModel(subAgent),
				Status:       "active", // Sub-agents are active if they're registered
				Tools:        h.getSubAgentTools(subAgent),
				Capabilities: []string{"Sub-agent"},
			}
			subAgents = append(subAgents, subAgentInfo)
		}

		// Also check tools for sub-agent tools (tools that end with _agent)
		tools := h.agent.GetTools()
		for _, tool := range tools {
			toolName := tool.Name()
			// Check if this tool represents a sub-agent (ends with _agent)
			if strings.HasSuffix(toolName, "_agent") {
				// Extract the agent name by removing _agent suffix
				agentName := strings.TrimSuffix(toolName, "_agent")

				// Check if we already have this sub-agent from GetSubAgents()
				found := false
				for _, existing := range subAgents {
					if existing.ID == toolName || existing.Name == agentName {
						found = true
						break
					}
				}

				if !found {
					subAgent := SubAgentInfo{
						ID:           toolName,
						Name:         agentName,
						Description:  tool.Description(),
						Model:        "Unknown",
						Status:       "active",
						Tools:        []string{toolName},
						Capabilities: []string{"Tool-based sub-agent"},
					}
					subAgents = append(subAgents, subAgent)
				}
			}
		}
	}

	return subAgents
}

// getSubAgentModel extracts model information from a sub-agent
func (h *HTTPServerWithUI) getSubAgentModel(subAgent *agent.Agent) string {
	if subAgent.IsRemote() {
		return "Remote Agent"
	}

	llm := subAgent.GetLLM()
	if llm == nil {
		return "No LLM"
	}

	// Try to get model from LLM if it supports GetModel method
	if modelGetter, ok := llm.(interface{ GetModel() string }); ok {
		model := modelGetter.GetModel()
		if model != "" {
			return model
		}
	}

	// Fallback to LLM name
	name := llm.Name()
	if name != "" {
		return name
	}

	return "Unknown"
}

// getSubAgentTools gets the tools available to a sub-agent
func (h *HTTPServerWithUI) getSubAgentTools(subAgent *agent.Agent) []string {
	tools := subAgent.GetTools()
	toolNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Name())
	}
	return toolNames
}

// parseToolsFromSystemPrompt extracts tool names from system prompt for remote agents
func (h *HTTPServerWithUI) parseToolsFromSystemPrompt(systemPrompt string) []string {
	tools := []string{}

	// Look for common patterns in system prompt
	lines := strings.Split(systemPrompt, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for patterns like "### ToolName_agent" or "- **Usage**: `ToolName_agent`"
		if strings.Contains(line, "_agent") {
			// Extract tool names ending with _agent
			words := strings.Fields(line)
			for _, word := range words {
				// Clean up word (remove markdown, punctuation)
				word = strings.Trim(word, "#*`-:.,!?()[]{}\"'")
				if strings.HasSuffix(word, "_agent") {
					// Check if not already added
					found := false
					for _, existingTool := range tools {
						if existingTool == word {
							found = true
							break
						}
					}
					if !found {
						tools = append(tools, word)
					}
				}
			}
		}
	}

	return tools
}

// getToolDescriptionFromSystemPrompt extracts tool description from system prompt
func (h *HTTPServerWithUI) getToolDescriptionFromSystemPrompt(toolName, systemPrompt string) string {
	lines := strings.Split(systemPrompt, "\n")

	for i, line := range lines {
		if strings.Contains(line, toolName) {
			// Look for description in nearby lines
			for j := i; j < len(lines) && j < i+5; j++ {
				if strings.Contains(lines[j], "Purpose") && strings.Contains(lines[j], ":") {
					parts := strings.SplitN(lines[j], ":", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
			// Fallback to generic description
			return fmt.Sprintf("%s sub-agent", strings.TrimSuffix(toolName, "_agent"))
		}
	}

	return "Sub-agent tool"
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

// getAllConversationsWithContext gets all conversations with request context (but ignores org isolation)
func (h *HTTPServerWithUI) getAllConversationsWithContext(ctx context.Context, limit, offset int) MemoryResponse {
	// For admin/debug view, we want to see all conversations from all orgs
	return h.getAllConversationsFromAllOrgs(limit, offset)
}

// getConversationMessagesWithContext gets messages with request context (but searches all orgs)
func (h *HTTPServerWithUI) getConversationMessagesWithContext(ctx context.Context, conversationID string, limit, offset int) MemoryResponse {
	// For admin/debug view, search across all orgs for the conversation
	return h.getConversationMessagesFromAllOrgs(conversationID, limit, offset)
}

// getAllConversationsFromAllOrgs gets conversations from all organizations
func (h *HTTPServerWithUI) getAllConversationsFromAllOrgs(limit, offset int) MemoryResponse {
	// Handle remote agents by making HTTP calls to their memory endpoint
	if h.agent.IsRemote() {
		log.Println("Fetching conversations from remote agent memory")
		return h.getRemoteMemoryConversations(limit, offset)
	}

	// Check if memory supports cross-org operations
	if adminMem, ok := h.agent.GetMemory().(interfaces.AdminConversationMemory); ok {
		log.Println("Fetching conversations from admin conversation memory across all orgs")
		return h.buildConversationListFromAllOrgs(adminMem, limit, offset)
	}

	// Fallback: build conversation list from local history (all orgs)
	return h.buildConversationListFromLocalAllOrgs(limit, offset)
}

// getConversationMessagesFromAllOrgs searches for conversation across all orgs
func (h *HTTPServerWithUI) getConversationMessagesFromAllOrgs(conversationID string, limit, offset int) MemoryResponse {
	// Handle remote agents by making HTTP calls to their memory endpoint
	if h.agent.IsRemote() {
		log.Printf("Fetching messages for conversation %s from remote agent memory", conversationID)
		return h.getRemoteMemoryMessages(conversationID, limit, offset)
	}

	// Check if memory supports cross-org operations
	if adminMem, ok := h.agent.GetMemory().(interfaces.AdminConversationMemory); ok {
		log.Printf("Fetching messages for conversation %s from admin conversation memory across all orgs", conversationID)
		return h.buildMessageListFromAllOrgs(adminMem, conversationID, limit, offset)
	}

	// Fallback: get messages from local history (search all orgs)
	return h.buildMessageListFromLocalAllOrgs(conversationID, limit, offset)
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

// getToolNames extracts tool names from the agent
func (h *HTTPServerWithUI) getToolNames() []string {
	tools := h.agent.GetTools()
	toolNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Name())
	}
	return toolNames
}

// getModelName extracts the model name from the agent's LLM
func (h *HTTPServerWithUI) getModelName() string {
	// For remote agents, try to get LLM info from metadata
	if h.agent.IsRemote() {
		if metadata, err := h.agent.GetRemoteMetadata(); err == nil && metadata != nil {
			if llmModel, ok := metadata["llm_model"]; ok && llmModel != "" && llmModel != "unknown" {
				return llmModel
			}
			if llmName, ok := metadata["llm_name"]; ok && llmName != "" && llmName != "unknown" {
				return llmName + " (model not specified)"
			}
		}
		return "Remote agent - metadata unavailable"
	}

	// For local agents, get from LLM directly
	llm := h.agent.GetLLM()
	if llm == nil {
		return "No LLM configured"
	}

	// Try to get model from LLM if it supports GetModel method
	if modelGetter, ok := llm.(interface{ GetModel() string }); ok {
		model := modelGetter.GetModel()
		if model != "" {
			return model
		}
	}

	// Fallback to LLM name if GetModel is not available or returns empty
	name := llm.Name()
	if name != "" {
		return name + " (model not specified)"
	}

	return "Unknown LLM"
}

// getMemoryInfo extracts memory information from the agent
func (h *HTTPServerWithUI) getMemoryInfo() MemoryInfo {
	// For remote agents, try to get memory info from metadata
	if h.agent.IsRemote() {
		if metadata, err := h.agent.GetRemoteMetadata(); err == nil && metadata != nil {
			if memoryType, ok := metadata["memory"]; ok && memoryType != "" && memoryType != "none" {
				return MemoryInfo{
					Type:   memoryType,
					Status: "active",
					// Entry count not available from remote metadata yet
				}
			}
		}
		return MemoryInfo{
			Type:   "none",
			Status: "inactive",
		}
	}

	// For local agents, check memory directly
	memory := h.agent.GetMemory()
	if memory == nil {
		return MemoryInfo{
			Type:   "none",
			Status: "inactive",
		}
	}

	// Memory is present, try to get more details
	memInfo := MemoryInfo{
		Type:   "conversation",
		Status: "active",
	}

	// Try to get entry count if the memory supports it
	ctx := context.Background()
	if messages, err := memory.GetMessages(ctx); err == nil {
		memInfo.EntryCount = len(messages)
	}

	return memInfo
}

// getSystemPrompt gets system prompt, handling remote agents
func (h *HTTPServerWithUI) getSystemPrompt() string {
	// For remote agents, try to get from metadata
	if h.agent.IsRemote() {
		if metadata, err := h.agent.GetRemoteMetadata(); err == nil && metadata != nil {
			if systemPrompt, ok := metadata["system_prompt"]; ok && systemPrompt != "" {
				return systemPrompt
			}
		}
		return "Remote agent - system prompt unavailable"
	}

	// For local agents, get directly
	systemPrompt := h.agent.GetSystemPrompt()
	if systemPrompt == "" {
		systemPrompt = "No system prompt configured"
	}
	return systemPrompt
}

// addToConversationHistory adds an entry to local conversation history
func (h *HTTPServerWithUI) addToConversationHistory(role, content string, metadata map[string]interface{}) {
	entry := MemoryEntry{
		ID:        fmt.Sprintf("local_%d", time.Now().UnixNano()),
		Role:      role,
		Content:   content,
		Timestamp: time.Now().UnixMilli(),
		Metadata:  metadata,
	}

	h.conversationHistory = append(h.conversationHistory, entry)

	// Keep only last 1000 entries to avoid memory issues
	if len(h.conversationHistory) > 1000 {
		h.conversationHistory = h.conversationHistory[len(h.conversationHistory)-1000:]
	}
}

// handleRun handles non-streaming agent requests and captures conversations
func (h *HTTPServerWithUI) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Input == "" {
		http.Error(w, "Input is required", http.StatusBadRequest)
		return
	}

	// Set up context with org ID if provided
	ctx := r.Context()
	if req.OrgID != "" {
		ctx = multitenancy.WithOrgID(ctx, req.OrgID)
	}

	// Add conversation ID if provided
	if req.ConversationID != "" {
		ctx = memory.WithConversationID(ctx, req.ConversationID)
	}

	// Add user input to conversation history
	h.addToConversationHistory("user", req.Input, map[string]interface{}{
		"conversation_id": req.ConversationID,
		"org_id":          req.OrgID,
	})

	// Execute agent
	result, err := h.agent.Run(ctx, req.Input)

	// Add response to conversation history
	if err != nil {
		h.addToConversationHistory("error", err.Error(), map[string]interface{}{
			"conversation_id": req.ConversationID,
			"org_id":          req.OrgID,
		})

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  err.Error(),
			"output": "",
		})
		return
	}

	h.addToConversationHistory("assistant", result, map[string]interface{}{
		"conversation_id": req.ConversationID,
		"org_id":          req.OrgID,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"output": result,
		"error":  "",
	})
}

// handleStream handles streaming agent requests and captures conversations
func (h *HTTPServerWithUI) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Input == "" {
		http.Error(w, "Input is required", http.StatusBadRequest)
		return
	}

	// Set up SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Set up context with org ID if provided
	ctx := r.Context()
	if req.OrgID != "" {
		ctx = multitenancy.WithOrgID(ctx, req.OrgID)
	}

	// Add conversation ID if provided
	if req.ConversationID != "" {
		ctx = memory.WithConversationID(ctx, req.ConversationID)
	}

	// Add user input to conversation history
	h.addToConversationHistory("user", req.Input, map[string]interface{}{
		"conversation_id": req.ConversationID,
		"org_id":          req.OrgID,
	})

	// Check if agent supports streaming
	streamingAgent, ok := interface{}(h.agent).(interfaces.StreamingAgent)
	if !ok {
		// Fall back to non-streaming
		result, err := h.agent.Run(ctx, req.Input)

		if err != nil {
			h.addToConversationHistory("error", err.Error(), map[string]interface{}{
				"conversation_id": req.ConversationID,
				"org_id":          req.OrgID,
			})

			event := SSEEvent{
				Event:     "error",
				Data:      StreamEventData{Type: "error", Content: err.Error(), IsFinal: true},
				Timestamp: time.Now().UnixMilli(),
			}
			h.sendSSEEvent(w, event)
			return
		}

		h.addToConversationHistory("assistant", result, map[string]interface{}{
			"conversation_id": req.ConversationID,
			"org_id":          req.OrgID,
		})

		event := SSEEvent{
			Event:     "content",
			Data:      StreamEventData{Type: "content", Content: result, IsFinal: true},
			Timestamp: time.Now().UnixMilli(),
		}
		h.sendSSEEvent(w, event)
		return
	}

	// Stream events from agent
	eventChan, err := streamingAgent.RunStream(ctx, req.Input)
	if err != nil {
		h.addToConversationHistory("error", err.Error(), map[string]interface{}{
			"conversation_id": req.ConversationID,
			"org_id":          req.OrgID,
		})

		event := SSEEvent{
			Event:     "error",
			Data:      StreamEventData{Type: "error", Content: err.Error(), IsFinal: true},
			Timestamp: time.Now().UnixMilli(),
		}
		h.sendSSEEvent(w, event)
		return
	}

	var fullResponse strings.Builder
	for agentEvent := range eventChan {
		// Collect content for conversation history
		if agentEvent.Content != "" && agentEvent.Type == interfaces.AgentEventContent {
			fullResponse.WriteString(agentEvent.Content)
		}

		// Convert agent event to stream event data
		eventData := StreamEventData{
			Type:         string(agentEvent.Type),
			Content:      agentEvent.Content,
			ThinkingStep: agentEvent.ThinkingStep,
			IsFinal:      agentEvent.Type == interfaces.AgentEventComplete,
		}

		if agentEvent.ToolCall != nil {
			eventData.ToolCall = &ToolCallData{
				ID:        agentEvent.ToolCall.ID,
				Name:      agentEvent.ToolCall.Name,
				Arguments: agentEvent.ToolCall.Arguments,
				Result:    agentEvent.ToolCall.Result,
				Status:    agentEvent.ToolCall.Status,
			}
		}

		if agentEvent.Error != nil {
			eventData.Error = agentEvent.Error.Error()
		}

		if agentEvent.Metadata != nil {
			eventData.Metadata = agentEvent.Metadata
		}

		event := SSEEvent{
			Event:     string(agentEvent.Type),
			Data:      eventData,
			Timestamp: agentEvent.Timestamp.UnixMilli(),
		}

		h.sendSSEEvent(w, event)

		// Flush for real-time streaming
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	// Add final response to conversation history
	if fullResponse.Len() > 0 {
		h.addToConversationHistory("assistant", fullResponse.String(), map[string]interface{}{
			"conversation_id": req.ConversationID,
			"org_id":          req.OrgID,
		})
	}
}

// sendSSEEvent sends a server-sent event
func (h *HTTPServerWithUI) sendSSEEvent(w http.ResponseWriter, event SSEEvent) {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return
	}

	_, _ = fmt.Fprintf(w, "event: %s\n", event.Event)
	_, _ = fmt.Fprintf(w, "data: %s\n", string(data))
	if event.ID != "" {
		_, _ = fmt.Fprintf(w, "id: %s\n", event.ID)
	}
	_, _ = fmt.Fprintf(w, "\n")
}

// buildConversationListFromAllOrgs builds conversation list from all organizations
func (h *HTTPServerWithUI) buildConversationListFromAllOrgs(adminMem interfaces.AdminConversationMemory, limit, offset int) MemoryResponse {
	orgConversations, err := adminMem.GetAllConversationsAcrossOrgs()
	if err != nil {
		// Return empty response on error
		return MemoryResponse{
			Mode:          "conversations",
			Conversations: []ConversationInfo{},
			Total:         0,
			Limit:         limit,
			Offset:        offset,
		}
	}

	var allConversationInfos []ConversationInfo

	// Iterate through all orgs and their conversations
	for orgID, conversations := range orgConversations {
		for _, convID := range conversations {
			// Get messages to determine last activity and message count
			messages, foundOrgID, err := adminMem.GetConversationMessagesAcrossOrgs(convID)
			if err != nil || foundOrgID != orgID {
				continue
			}

			if len(messages) > 0 {
				lastMessage := messages[len(messages)-1]

				// Truncate last message content for preview
				lastContent := lastMessage.Content
				if len(lastContent) > 100 {
					lastContent = lastContent[:100] + "..."
				}

				// Include orgID in conversation display
				displayID := fmt.Sprintf("[%s] %s", orgID, convID)

				allConversationInfos = append(allConversationInfos, ConversationInfo{
					ID:           convID, // Keep original ID for API calls
					MessageCount: len(messages),
					LastActivity: time.Now().Unix(), // TODO: get actual timestamp from last message
					LastMessage:  displayID + ": " + lastContent,
				})
			}
		}
	}

	// Apply pagination
	total := len(allConversationInfos)
	start := offset
	end := offset + limit
	if start >= total {
		allConversationInfos = []ConversationInfo{}
	} else {
		if end > total {
			end = total
		}
		allConversationInfos = allConversationInfos[start:end]
	}

	return MemoryResponse{
		Mode:          "conversations",
		Conversations: allConversationInfos,
		Total:         total,
		Limit:         limit,
		Offset:        offset,
	}
}

// buildConversationListFromLocalAllOrgs builds conversation list from local history across all orgs
func (h *HTTPServerWithUI) buildConversationListFromLocalAllOrgs(limit, offset int) MemoryResponse {
	// Group local conversation history by conversation ID (ignoring org isolation)
	conversationMap := make(map[string][]MemoryEntry)

	for _, entry := range h.conversationHistory {
		convID := entry.ConversationID
		if convID == "" {
			convID = "default"
		}
		conversationMap[convID] = append(conversationMap[convID], entry)
	}

	var conversationInfos []ConversationInfo
	for convID, entries := range conversationMap {
		if len(entries) > 0 {
			lastEntry := entries[len(entries)-1]
			lastContent := lastEntry.Content
			if len(lastContent) > 100 {
				lastContent = lastContent[:100] + "..."
			}

			conversationInfos = append(conversationInfos, ConversationInfo{
				ID:           convID,
				MessageCount: len(entries),
				LastActivity: lastEntry.Timestamp,
				LastMessage:  lastContent,
			})
		}
	}

	// Apply pagination
	total := len(conversationInfos)
	start := offset
	end := offset + limit
	if start >= total {
		conversationInfos = []ConversationInfo{}
	} else {
		if end > total {
			end = total
		}
		conversationInfos = conversationInfos[start:end]
	}

	return MemoryResponse{
		Mode:          "conversations",
		Conversations: conversationInfos,
		Total:         total,
		Limit:         limit,
		Offset:        offset,
	}
}

// buildMessageListFromAllOrgs builds message list from all organizations
func (h *HTTPServerWithUI) buildMessageListFromAllOrgs(adminMem interfaces.AdminConversationMemory, conversationID string, limit, offset int) MemoryResponse {
	messages, orgID, err := adminMem.GetConversationMessagesAcrossOrgs(conversationID)
	if err != nil {
		// Return empty response on error
		return MemoryResponse{
			Mode:           "messages",
			Messages:       []MemoryEntry{},
			Total:          0,
			Limit:          limit,
			Offset:         offset,
			ConversationID: conversationID,
		}
	}

	var memoryEntries []MemoryEntry

	for i, msg := range messages {
		// Extract tool calls if present
		toolCallsInfo := ""
		if len(msg.ToolCalls) > 0 {
			toolCallsInfo = fmt.Sprintf(" [%d tool calls]", len(msg.ToolCalls))
		}

		// Include org info in the message content
		content := msg.Content + toolCallsInfo
		if orgID != "" {
			content = fmt.Sprintf("[%s] %s", orgID, content)
		}

		memoryEntries = append(memoryEntries, MemoryEntry{
			ID:             fmt.Sprintf("agent_msg_%d", i),
			Role:           string(msg.Role),
			Content:        content,
			Timestamp:      time.Now().Unix(), // TODO: get actual timestamp from message
			ConversationID: conversationID,
			Metadata:       msg.Metadata,
		})
	}

	// Apply pagination
	total := len(memoryEntries)
	start := offset
	end := offset + limit
	if start >= total {
		memoryEntries = []MemoryEntry{}
	} else {
		if end > total {
			end = total
		}
		memoryEntries = memoryEntries[start:end]
	}

	return MemoryResponse{
		Mode:           "messages",
		Messages:       memoryEntries,
		Total:          total,
		Limit:          limit,
		Offset:         offset,
		ConversationID: conversationID,
	}
}

// buildMessageListFromLocalAllOrgs builds message list from local history across all orgs
func (h *HTTPServerWithUI) buildMessageListFromLocalAllOrgs(conversationID string, limit, offset int) MemoryResponse {
	var filteredEntries []MemoryEntry

	// Filter entries by conversation ID (ignoring org isolation)
	for _, entry := range h.conversationHistory {
		entryConvID := entry.ConversationID
		if entryConvID == "" {
			entryConvID = "default"
		}
		if entryConvID == conversationID {
			filteredEntries = append(filteredEntries, entry)
		}
	}

	// Apply pagination
	total := len(filteredEntries)
	start := offset
	end := offset + limit
	if start >= total {
		filteredEntries = []MemoryEntry{}
	} else {
		if end > total {
			end = total
		}
		filteredEntries = filteredEntries[start:end]
	}

	return MemoryResponse{
		Mode:           "messages",
		Messages:       filteredEntries,
		Total:          total,
		Limit:          limit,
		Offset:         offset,
		ConversationID: conversationID,
	}
}

// getRemoteMemoryConversations gets conversations from a remote agent via HTTP
func (h *HTTPServerWithUI) getRemoteMemoryConversations(limit, offset int) MemoryResponse {
	remoteURL := h.agent.GetRemoteURL()
	if remoteURL == "" {
		return MemoryResponse{
			Mode:          "conversations",
			Conversations: []ConversationInfo{},
			Total:         0,
			Limit:         limit,
			Offset:        offset,
		}
	}

	// Make HTTP request to remote agent's memory endpoint
	url := fmt.Sprintf("%s/api/v1/memory?limit=%d&offset=%d", remoteURL, limit, offset)

	// #nosec G107 - URL is constructed from validated parameters
	resp, err := http.Get(url)
	if err != nil {
		return MemoryResponse{
			Mode:          "conversations",
			Conversations: []ConversationInfo{},
			Total:         0,
			Limit:         limit,
			Offset:        offset,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return MemoryResponse{
			Mode:          "conversations",
			Conversations: []ConversationInfo{},
			Total:         0,
			Limit:         limit,
			Offset:        offset,
		}
	}

	var remoteResponse MemoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&remoteResponse); err != nil {
		return MemoryResponse{
			Mode:          "conversations",
			Conversations: []ConversationInfo{},
			Total:         0,
			Limit:         limit,
			Offset:        offset,
		}
	}

	return remoteResponse
}

// getRemoteMemoryMessages gets messages for a specific conversation from a remote agent via HTTP
func (h *HTTPServerWithUI) getRemoteMemoryMessages(conversationID string, limit, offset int) MemoryResponse {
	remoteURL := h.agent.GetRemoteURL()
	if remoteURL == "" {
		return MemoryResponse{
			Mode:           "messages",
			Messages:       []MemoryEntry{},
			Total:          0,
			Limit:          limit,
			Offset:         offset,
			ConversationID: conversationID,
		}
	}

	// Make HTTP request to remote agent's memory endpoint for specific conversation
	url := fmt.Sprintf("%s/api/v1/memory?conversation_id=%s&limit=%d&offset=%d",
		remoteURL, conversationID, limit, offset)

	// #nosec G107 - URL is constructed from validated parameters
	resp, err := http.Get(url)
	if err != nil {
		return MemoryResponse{
			Mode:           "messages",
			Messages:       []MemoryEntry{},
			Total:          0,
			Limit:          limit,
			Offset:         offset,
			ConversationID: conversationID,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return MemoryResponse{
			Mode:           "messages",
			Messages:       []MemoryEntry{},
			Total:          0,
			Limit:          limit,
			Offset:         offset,
			ConversationID: conversationID,
		}
	}

	var remoteResponse MemoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&remoteResponse); err != nil {
		return MemoryResponse{
			Mode:           "messages",
			Messages:       []MemoryEntry{},
			Total:          0,
			Limit:          limit,
			Offset:         offset,
			ConversationID: conversationID,
		}
	}

	return remoteResponse
}
