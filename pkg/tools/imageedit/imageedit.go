package imageedit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage"
)

// Tool implements multi-turn image editing as an agent tool.
// It manages sessions that allow iterative image creation and refinement
// through conversation, maintaining context between edits.
type Tool struct {
	editor         interfaces.MultiTurnImageEditor
	storage        storage.ImageStorage
	sessions       map[string]*sessionEntry
	sessionsMu     sync.RWMutex
	maxPromptLen   int
	sessionTimeout time.Duration
	maxSessions    int
	defaultModel   string
}

type sessionEntry struct {
	session   interfaces.ImageEditSession
	lastUsed  time.Time
	orgID     string
	createdAt time.Time
}

// Option represents an option for configuring the tool
type Option func(*Tool)

// WithMaxPromptLength sets maximum prompt length
func WithMaxPromptLength(maxLen int) Option {
	return func(t *Tool) {
		t.maxPromptLen = maxLen
	}
}

// WithSessionTimeout sets the session timeout duration
func WithSessionTimeout(timeout time.Duration) Option {
	return func(t *Tool) {
		t.sessionTimeout = timeout
	}
}

// WithMaxSessions sets the maximum number of concurrent sessions
func WithMaxSessions(max int) Option {
	return func(t *Tool) {
		t.maxSessions = max
	}
}

// WithDefaultModel sets the default model for new sessions
func WithDefaultModel(model string) Option {
	return func(t *Tool) {
		t.defaultModel = model
	}
}

// New creates a new multi-turn image editing tool.
func New(editor interfaces.MultiTurnImageEditor, storage storage.ImageStorage, options ...Option) *Tool {
	tool := &Tool{
		editor:         editor,
		storage:        storage,
		sessions:       make(map[string]*sessionEntry),
		maxPromptLen:   2000,
		sessionTimeout: 30 * time.Minute,
		maxSessions:    10,
		defaultModel:   "", // Will use editor's default
	}

	for _, opt := range options {
		opt(tool)
	}

	// Start background cleanup goroutine
	go tool.cleanupExpiredSessions()

	return tool
}

// Name returns the tool name
func (t *Tool) Name() string {
	return "edit_image"
}

// DisplayName returns a human-friendly name
func (t *Tool) DisplayName() string {
	return "Image Editor"
}

// Description returns what the tool does
func (t *Tool) Description() string {
	return `Multi-turn image editing tool for iterative image creation and refinement through conversation.

Actions:
- start_session: Begin a new editing session, optionally with an initial prompt to generate the first image
- edit: Send a modification request to an existing session (requires session_id)
- end_session: Close a session when done (requires session_id)

The session maintains conversation context, allowing you to progressively refine images based on previous results.`
}

// Internal returns false as this is a user-visible tool
func (t *Tool) Internal() bool {
	return false
}

// Parameters returns the tool's parameter specifications
func (t *Tool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"action": {
			Type:        "string",
			Description: "The action to perform: start_session (create new session), edit (modify existing image), or end_session (close session)",
			Required:    true,
			Enum:        []interface{}{"start_session", "edit", "end_session"},
		},
		"session_id": {
			Type:        "string",
			Description: "Session ID (required for 'edit' and 'end_session' actions). Returned when starting a new session.",
			Required:    false,
		},
		"prompt": {
			Type:        "string",
			Description: "Text description for image generation or modification request. Required for 'start_session' (initial image) and 'edit' actions.",
			Required:    false,
		},
		"aspect_ratio": {
			Type:        "string",
			Description: "Output image aspect ratio",
			Required:    false,
			Default:     "1:1",
			Enum:        []interface{}{"1:1", "2:3", "3:2", "16:9", "21:9"},
		},
		"image_size": {
			Type:        "string",
			Description: "Output image resolution",
			Required:    false,
			Default:     "1K",
			Enum:        []interface{}{"1K", "2K", "4K"},
		},
	}
}

// Run executes the tool with the given input
func (t *Tool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute implements the tool execution
func (t *Tool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Action      string `json:"action"`
		SessionID   string `json:"session_id,omitempty"`
		Prompt      string `json:"prompt,omitempty"`
		AspectRatio string `json:"aspect_ratio,omitempty"`
		ImageSize   string `json:"image_size,omitempty"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	switch params.Action {
	case "start_session":
		return t.startSession(ctx, params.Prompt, params.AspectRatio, params.ImageSize)
	case "edit":
		if params.SessionID == "" {
			return "", fmt.Errorf("session_id is required for 'edit' action")
		}
		if params.Prompt == "" {
			return "", fmt.Errorf("prompt is required for 'edit' action")
		}
		return t.editImage(ctx, params.SessionID, params.Prompt, params.AspectRatio, params.ImageSize)
	case "end_session":
		if params.SessionID == "" {
			return "", fmt.Errorf("session_id is required for 'end_session' action")
		}
		return t.endSession(ctx, params.SessionID)
	default:
		return "", fmt.Errorf("unknown action: %s. Valid actions: start_session, edit, end_session", params.Action)
	}
}

func (t *Tool) startSession(ctx context.Context, prompt, aspectRatio, imageSize string) (string, error) {
	// Get organization ID for session tracking
	orgID, _ := multitenancy.GetOrgID(ctx)

	// Check session limits
	t.sessionsMu.RLock()
	orgSessionCount := 0
	for _, entry := range t.sessions {
		if entry.orgID == orgID {
			orgSessionCount++
		}
	}
	t.sessionsMu.RUnlock()

	if orgSessionCount >= t.maxSessions {
		return "", fmt.Errorf("maximum number of concurrent sessions (%d) reached. Please end an existing session first", t.maxSessions)
	}

	// Create session options
	sessionOpts := &interfaces.ImageEditSessionOptions{
		Model: t.defaultModel,
	}

	// Create new session
	session, err := t.editor.CreateImageEditSession(ctx, sessionOpts)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	// Generate session ID
	sessionID := uuid.New().String()

	// Store session
	t.sessionsMu.Lock()
	t.sessions[sessionID] = &sessionEntry{
		session:   session,
		lastUsed:  time.Now(),
		orgID:     orgID,
		createdAt: time.Now(),
	}
	t.sessionsMu.Unlock()

	// If prompt provided, generate initial image
	if prompt != "" {
		if len(prompt) > t.maxPromptLen {
			// Clean up session on validation error
			t.sessionsMu.Lock()
			delete(t.sessions, sessionID)
			t.sessionsMu.Unlock()
			_ = session.Close()
			return "", fmt.Errorf("prompt exceeds maximum length of %d characters", t.maxPromptLen)
		}

		resp, err := session.SendMessage(ctx, prompt, &interfaces.ImageEditOptions{
			AspectRatio: aspectRatio,
			ImageSize:   imageSize,
		})
		if err != nil {
			// Clean up session on error
			t.sessionsMu.Lock()
			delete(t.sessions, sessionID)
			t.sessionsMu.Unlock()
			_ = session.Close()
			return "", fmt.Errorf("failed to generate initial image: %w", err)
		}
		return t.formatResponse(ctx, sessionID, resp, prompt, true)
	}

	return fmt.Sprintf(`Image editing session started successfully.

Session ID: %s

Use this session ID with action='edit' to generate and modify images. The session maintains conversation context, so you can iteratively refine your images.

When done, use action='end_session' to close the session.`, sessionID), nil
}

func (t *Tool) editImage(ctx context.Context, sessionID, prompt, aspectRatio, imageSize string) (string, error) {
	// Validate prompt length
	if len(prompt) > t.maxPromptLen {
		return "", fmt.Errorf("prompt exceeds maximum length of %d characters", t.maxPromptLen)
	}

	// Get session
	t.sessionsMu.RLock()
	entry, exists := t.sessions[sessionID]
	t.sessionsMu.RUnlock()

	if !exists {
		return "", fmt.Errorf("%w: %s", interfaces.ErrSessionNotFound, sessionID)
	}

	// Check session timeout
	if time.Since(entry.lastUsed) > t.sessionTimeout {
		// Session expired, clean it up
		t.sessionsMu.Lock()
		delete(t.sessions, sessionID)
		t.sessionsMu.Unlock()
		_ = entry.session.Close()
		return "", fmt.Errorf("%w: session %s has expired after %v of inactivity", interfaces.ErrSessionExpired, sessionID, t.sessionTimeout)
	}

	// Update last used time
	entry.lastUsed = time.Now()

	// Send edit request
	resp, err := entry.session.SendMessage(ctx, prompt, &interfaces.ImageEditOptions{
		AspectRatio: aspectRatio,
		ImageSize:   imageSize,
	})
	if err != nil {
		return "", fmt.Errorf("failed to edit image: %w", err)
	}

	return t.formatResponse(ctx, sessionID, resp, prompt, false)
}

func (t *Tool) endSession(ctx context.Context, sessionID string) (string, error) {
	t.sessionsMu.Lock()
	defer t.sessionsMu.Unlock()

	entry, exists := t.sessions[sessionID]
	if !exists {
		return "", fmt.Errorf("%w: %s", interfaces.ErrSessionNotFound, sessionID)
	}

	// Get history count before closing
	historyLen := len(entry.session.GetHistory())
	duration := time.Since(entry.createdAt)

	// Close and remove session
	_ = entry.session.Close()
	delete(t.sessions, sessionID)

	return fmt.Sprintf(`Session %s closed successfully.

Session duration: %v
Total turns: %d

The session context has been cleared.`, sessionID, duration.Round(time.Second), historyLen/2), nil
}

func (t *Tool) formatResponse(ctx context.Context, sessionID string, resp *interfaces.ImageEditResponse, prompt string, isInitial bool) (string, error) {
	var result string

	if isInitial {
		result = fmt.Sprintf("Image editing session started with initial image.\n\nSession ID: %s\n\n", sessionID)
	} else {
		result = fmt.Sprintf("Image edited successfully.\n\nSession ID: %s\n\n", sessionID)
	}

	// Add text response if present
	if resp.Text != "" {
		result += fmt.Sprintf("Model response: %s\n\n", resp.Text)
	}

	// Process images
	if len(resp.Images) > 0 {
		for i, image := range resp.Images {
			// Try to store image if storage is configured
			if t.storage != nil {
				metadata := storage.StorageMetadata{
					Prompt:    prompt,
					CreatedAt: time.Now(),
				}

				// Get org and thread IDs from context if available
				if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
					metadata.OrgID = orgID
				}

				url, err := t.storage.Store(ctx, &image, metadata)
				if err != nil {
					// Log warning but continue with base64
					fmt.Printf("[imageedit] Storage failed, using base64: %v\n", err)
					result += t.formatImageBase64(&image, i)
				} else {
					result += t.formatImageURL(url, &image, i)
				}
			} else {
				// No storage configured - use base64
				result += t.formatImageBase64(&image, i)
			}
		}
	} else {
		result += "No image was generated in this turn.\n"
	}

	// Add usage info
	if resp.Usage != nil {
		result += fmt.Sprintf("\nTokens used: %d input, %d output\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}

	result += fmt.Sprintf("\nUse action='edit' with session_id='%s' to continue refining, or action='end_session' to close.", sessionID)

	return result, nil
}

func (t *Tool) formatImageURL(url string, image *interfaces.GeneratedImage, index int) string {
	result := ""
	if index > 0 {
		result = fmt.Sprintf("\n--- Image %d ---\n", index+1)
	}
	// Use markdown image syntax for UI rendering
	result += fmt.Sprintf("![Generated image](%s)\n\n", url)
	result += fmt.Sprintf("Format: %s\n", image.MimeType)
	result += fmt.Sprintf("Size: %d bytes\n", len(image.Data))
	return result
}

func (t *Tool) formatImageBase64(image *interfaces.GeneratedImage, index int) string {
	result := ""
	if index > 0 {
		result = fmt.Sprintf("\n--- Image %d ---\n", index+1)
	}
	// Create data URI for direct embedding in markdown
	dataURI := fmt.Sprintf("data:%s;base64,%s", image.MimeType, image.Base64)
	result += fmt.Sprintf("![Generated image](%s)\n\n", dataURI)
	result += fmt.Sprintf("Format: %s\n", image.MimeType)
	result += fmt.Sprintf("Size: %d bytes\n", len(image.Data))
	return result
}

// cleanupExpiredSessions runs periodically to clean up expired sessions
func (t *Tool) cleanupExpiredSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		t.sessionsMu.Lock()
		now := time.Now()
		for sessionID, entry := range t.sessions {
			if now.Sub(entry.lastUsed) > t.sessionTimeout {
				_ = entry.session.Close()
				delete(t.sessions, sessionID)
			}
		}
		t.sessionsMu.Unlock()
	}
}

// GetActiveSessions returns the number of active sessions (useful for monitoring)
func (t *Tool) GetActiveSessions() int {
	t.sessionsMu.RLock()
	defer t.sessionsMu.RUnlock()
	return len(t.sessions)
}

// GetActiveSessionsForOrg returns the number of active sessions for a specific organization
func (t *Tool) GetActiveSessionsForOrg(orgID string) int {
	t.sessionsMu.RLock()
	defer t.sessionsMu.RUnlock()

	count := 0
	for _, entry := range t.sessions {
		if entry.orgID == orgID {
			count++
		}
	}
	return count
}
