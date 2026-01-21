package imagegen

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage"
)

// Tool implements image generation as a tool for agents.
// When multi-turn editing is enabled, it automatically manages sessions
// to allow iterative image refinement through conversation.
type Tool struct {
	generator     interfaces.ImageGenerator
	storage       storage.ImageStorage
	maxPromptLen  int
	defaultAspect string
	defaultFormat string

	// Multi-turn editing support
	multiTurnEditor   interfaces.MultiTurnImageEditor
	multiTurnEnabled  bool
	multiTurnModel    string
	sessions          map[string]*sessionEntry
	sessionsMu        sync.RWMutex
	sessionTimeout    time.Duration
	maxSessionsPerOrg int
}

// sessionEntry tracks an active multi-turn editing session
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

// WithDefaultAspectRatio sets the default aspect ratio
func WithDefaultAspectRatio(ratio string) Option {
	return func(t *Tool) {
		t.defaultAspect = ratio
	}
}

// WithDefaultFormat sets the default output format
func WithDefaultFormat(format string) Option {
	return func(t *Tool) {
		t.defaultFormat = format
	}
}

// WithMultiTurnEditor enables multi-turn image editing support.
// When enabled, the tool automatically manages sessions for iterative image refinement.
func WithMultiTurnEditor(editor interfaces.MultiTurnImageEditor) Option {
	return func(t *Tool) {
		if editor != nil && editor.SupportsMultiTurnImageEditing() {
			t.multiTurnEditor = editor
			t.multiTurnEnabled = true
			t.sessions = make(map[string]*sessionEntry)
			// Start background cleanup
			go t.cleanupExpiredSessions()
		}
	}
}

// WithMultiTurnModel sets the model to use for multi-turn editing sessions
func WithMultiTurnModel(model string) Option {
	return func(t *Tool) {
		t.multiTurnModel = model
	}
}

// WithSessionTimeout sets how long sessions remain active without use
func WithSessionTimeout(timeout time.Duration) Option {
	return func(t *Tool) {
		t.sessionTimeout = timeout
	}
}

// WithMaxSessionsPerOrg limits concurrent sessions per organization
func WithMaxSessionsPerOrg(max int) Option {
	return func(t *Tool) {
		t.maxSessionsPerOrg = max
	}
}

// New creates a new image generation tool
func New(generator interfaces.ImageGenerator, storage storage.ImageStorage, options ...Option) *Tool {
	tool := &Tool{
		generator:         generator,
		storage:           storage,
		maxPromptLen:      2000,
		defaultAspect:     "1:1",
		defaultFormat:     "png",
		sessionTimeout:    30 * time.Minute,
		maxSessionsPerOrg: 10,
	}

	for _, opt := range options {
		opt(tool)
	}

	return tool
}

// Name returns the tool name
func (t *Tool) Name() string {
	return "generate_image"
}

// DisplayName returns a human-friendly name
func (t *Tool) DisplayName() string {
	return "Image Generator"
}

// Description returns what the tool does
func (t *Tool) Description() string {
	if t.multiTurnEnabled {
		return `Generate and edit images through conversation. Supports iterative refinement.

Actions:
- generate: Create a new image (starts a session automatically for multi-turn editing)
- edit: Modify the current image in the active session
- end_session: Close the current editing session

The tool automatically maintains a session per conversation, allowing you to refine images iteratively.`
	}
	return "Generate images from text descriptions using AI. Provide a detailed prompt describing the image you want to create. Returns the URL of the generated image."
}

// Internal returns false as this is a user-visible tool
func (t *Tool) Internal() bool {
	return false
}

// Parameters returns the tool's parameter specifications
func (t *Tool) Parameters() map[string]interfaces.ParameterSpec {
	params := map[string]interfaces.ParameterSpec{
		"prompt": {
			Type:        "string",
			Description: "A detailed text description of the image to generate or the modification to apply.",
			Required:    true,
		},
		"aspect_ratio": {
			Type:        "string",
			Description: "The aspect ratio of the output image",
			Required:    false,
			Default:     t.defaultAspect,
			Enum:        []interface{}{"1:1", "16:9", "9:16", "4:3", "3:4", "2:3", "3:2", "21:9"},
		},
	}

	// Add multi-turn specific parameters
	if t.multiTurnEnabled {
		params["action"] = interfaces.ParameterSpec{
			Type:        "string",
			Description: "The action to perform: 'generate' creates a new image (default), 'edit' modifies the current image, 'end_session' closes the editing session",
			Required:    false,
			Default:     "generate",
			Enum:        []interface{}{"generate", "edit", "end_session"},
		}
		params["image_size"] = interfaces.ParameterSpec{
			Type:        "string",
			Description: "Output image resolution (for multi-turn editing)",
			Required:    false,
			Default:     "1K",
			Enum:        []interface{}{"1K", "2K", "4K"},
		}
	} else {
		params["output_format"] = interfaces.ParameterSpec{
			Type:        "string",
			Description: "The output image format",
			Required:    false,
			Default:     t.defaultFormat,
			Enum:        []interface{}{"png", "jpeg"},
		}
	}

	return params
}

// Run executes the tool with the given input
func (t *Tool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute implements the tool execution
func (t *Tool) Execute(ctx context.Context, args string) (string, error) {
	// Parse arguments
	var params struct {
		Prompt       string `json:"prompt"`
		Action       string `json:"action,omitempty"`
		AspectRatio  string `json:"aspect_ratio,omitempty"`
		OutputFormat string `json:"output_format,omitempty"`
		ImageSize    string `json:"image_size,omitempty"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Set defaults
	if params.Action == "" {
		params.Action = "generate"
	}
	if params.AspectRatio == "" {
		params.AspectRatio = t.defaultAspect
	}

	// Use multi-turn editing if enabled
	if t.multiTurnEnabled {
		return t.executeMultiTurn(ctx, params.Action, params.Prompt, params.AspectRatio, params.ImageSize)
	}

	// Standard single-shot generation
	return t.executeSingleShot(ctx, params.Prompt, params.AspectRatio, params.OutputFormat)
}

// executeSingleShot performs standard one-shot image generation
func (t *Tool) executeSingleShot(ctx context.Context, prompt, aspectRatio, outputFormat string) (string, error) {
	// Validate prompt
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	if len(prompt) > t.maxPromptLen {
		return "", fmt.Errorf("prompt exceeds maximum length of %d characters", t.maxPromptLen)
	}

	// Set defaults
	if outputFormat == "" {
		outputFormat = t.defaultFormat
	}

	// Build request
	request := interfaces.ImageGenerationRequest{
		Prompt: prompt,
		Options: &interfaces.ImageGenerationOptions{
			NumberOfImages: 1,
			AspectRatio:    aspectRatio,
			OutputFormat:   outputFormat,
		},
	}

	// Generate image
	response, err := t.generator.GenerateImage(ctx, request)
	if err != nil {
		return "", fmt.Errorf("image generation failed: %w", err)
	}

	if len(response.Images) == 0 {
		return "", fmt.Errorf("no images were generated")
	}

	// Store image if storage is configured
	if t.storage != nil {
		metadata := storage.StorageMetadata{
			Prompt:    prompt,
			CreatedAt: time.Now(),
		}

		url, err := t.storage.Store(ctx, &response.Images[0], metadata)
		if err != nil {
			// Log warning but don't fail - return base64 instead
			fmt.Printf("[imagegen] Storage failed, using base64: %v\n", err)
			return t.formatResultWithBase64(response, prompt), nil
		}
		response.Images[0].URL = url
		fmt.Printf("[imagegen] Image stored at: %s\n", url)
		// Format result with URL
		return t.formatResult(response, prompt, url), nil
	}

	// No storage configured - return base64 embedded image
	fmt.Printf("[imagegen] No storage configured, using base64\n")
	return t.formatResultWithBase64(response, prompt), nil
}

// executeMultiTurn handles multi-turn image editing with automatic session management
func (t *Tool) executeMultiTurn(ctx context.Context, action, prompt, aspectRatio, imageSize string) (string, error) {
	// Get session key from context (org + thread)
	sessionKey := t.getSessionKey(ctx)

	switch action {
	case "generate":
		// For "generate", we start a new session (closing any existing one)
		return t.generateWithSession(ctx, sessionKey, prompt, aspectRatio, imageSize)

	case "edit":
		// For "edit", we continue the existing session
		return t.editInSession(ctx, sessionKey, prompt, aspectRatio, imageSize)

	case "end_session":
		// Close the session
		return t.endSession(ctx, sessionKey)

	default:
		return "", fmt.Errorf("unknown action: %s. Valid actions: generate, edit, end_session", action)
	}
}

// getSessionKey returns a unique key for the current context.
// Uses org ID if available, otherwise defaults to "default".
// Note: In a multi-thread scenario, you may want to extend this
// to include thread information from the context.
func (t *Tool) getSessionKey(ctx context.Context) string {
	orgID, _ := multitenancy.GetOrgID(ctx)

	if orgID == "" {
		orgID = "default"
	}

	return orgID
}

// generateWithSession creates a new session and generates an initial image
func (t *Tool) generateWithSession(ctx context.Context, sessionKey, prompt, aspectRatio, imageSize string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt is required for generating an image")
	}

	if len(prompt) > t.maxPromptLen {
		return "", fmt.Errorf("prompt exceeds maximum length of %d characters", t.maxPromptLen)
	}

	// Close any existing session for this key
	t.sessionsMu.Lock()
	if existing, ok := t.sessions[sessionKey]; ok {
		_ = existing.session.Close()
		delete(t.sessions, sessionKey)
	}
	t.sessionsMu.Unlock()

	// Check session limits
	orgID, _ := multitenancy.GetOrgID(ctx)
	if t.maxSessionsPerOrg > 0 {
		t.sessionsMu.RLock()
		orgCount := 0
		for _, entry := range t.sessions {
			if entry.orgID == orgID {
				orgCount++
			}
		}
		t.sessionsMu.RUnlock()

		if orgCount >= t.maxSessionsPerOrg {
			return "", fmt.Errorf("maximum number of concurrent sessions (%d) reached", t.maxSessionsPerOrg)
		}
	}

	// Create new session
	session, err := t.multiTurnEditor.CreateImageEditSession(ctx, &interfaces.ImageEditSessionOptions{
		Model: t.multiTurnModel,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create editing session: %w", err)
	}

	// Store session
	t.sessionsMu.Lock()
	t.sessions[sessionKey] = &sessionEntry{
		session:   session,
		lastUsed:  time.Now(),
		orgID:     orgID,
		createdAt: time.Now(),
	}
	t.sessionsMu.Unlock()

	// Generate initial image
	if imageSize == "" {
		imageSize = "1K"
	}

	resp, err := session.SendMessage(ctx, prompt, &interfaces.ImageEditOptions{
		AspectRatio: aspectRatio,
		ImageSize:   imageSize,
	})
	if err != nil {
		// Clean up session on error
		t.sessionsMu.Lock()
		delete(t.sessions, sessionKey)
		t.sessionsMu.Unlock()
		_ = session.Close()
		return "", fmt.Errorf("failed to generate initial image: %w", err)
	}

	return t.formatMultiTurnResponse(ctx, resp, prompt, true)
}

// editInSession modifies the current image in an existing session
func (t *Tool) editInSession(ctx context.Context, sessionKey, prompt, aspectRatio, imageSize string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt is required for editing")
	}

	if len(prompt) > t.maxPromptLen {
		return "", fmt.Errorf("prompt exceeds maximum length of %d characters", t.maxPromptLen)
	}

	// Get session
	t.sessionsMu.RLock()
	entry, exists := t.sessions[sessionKey]
	t.sessionsMu.RUnlock()

	if !exists {
		// No active session - start one automatically
		return t.generateWithSession(ctx, sessionKey, prompt, aspectRatio, imageSize)
	}

	// Check session timeout
	if time.Since(entry.lastUsed) > t.sessionTimeout {
		// Session expired, clean it up and start fresh
		t.sessionsMu.Lock()
		delete(t.sessions, sessionKey)
		t.sessionsMu.Unlock()
		_ = entry.session.Close()
		return t.generateWithSession(ctx, sessionKey, prompt, aspectRatio, imageSize)
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

	return t.formatMultiTurnResponse(ctx, resp, prompt, false)
}

// endSession closes the current editing session
func (t *Tool) endSession(ctx context.Context, sessionKey string) (string, error) {
	t.sessionsMu.Lock()
	defer t.sessionsMu.Unlock()

	entry, exists := t.sessions[sessionKey]
	if !exists {
		return "No active editing session to close.", nil
	}

	// Get stats before closing
	historyLen := len(entry.session.GetHistory())
	duration := time.Since(entry.createdAt)

	// Close and remove session
	_ = entry.session.Close()
	delete(t.sessions, sessionKey)

	return fmt.Sprintf("Editing session closed.\n\nSession duration: %v\nTotal turns: %d",
		duration.Round(time.Second), historyLen/2), nil
}

// formatMultiTurnResponse formats the response from a multi-turn editing session
func (t *Tool) formatMultiTurnResponse(ctx context.Context, resp *interfaces.ImageEditResponse, prompt string, isInitial bool) (string, error) {
	var result string

	if isInitial {
		result = "Image generated successfully.\n\n"
	} else {
		result = "Image edited successfully.\n\n"
	}

	// Add text response if present
	if resp.Text != "" {
		result += fmt.Sprintf("Model: %s\n\n", resp.Text)
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

				// Get org ID from context if available
				if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
					metadata.OrgID = orgID
				}

				url, err := t.storage.Store(ctx, &image, metadata)
				if err != nil {
					// Log warning but continue with base64
					fmt.Printf("[imagegen] Storage failed, using base64: %v\n", err)
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

	result += "\nYou can continue editing this image with action='edit', or use action='end_session' when done."

	return result, nil
}

// formatImageURL formats an image with its URL
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

// formatImageBase64 formats an image as base64 data URI
// For large images, it skips embedding to avoid token limits and UI performance issues
func (t *Tool) formatImageBase64(image *interfaces.GeneratedImage, index int) string {
	result := ""
	if index > 0 {
		result = fmt.Sprintf("\n--- Image %d ---\n", index+1)
	}

	// Check image size - if too large, don't embed full base64 to avoid:
	// 1. Token limits in LLM conversation history
	// 2. UI performance issues (re-rendering large base64 on each keystroke)
	// A typical 1K image is ~500KB base64
	const maxBase64Size = 50000 // ~50KB limit - be conservative for UI performance
	if len(image.Base64) > maxBase64Size {
		// Image too large for embedding - provide info but not the data
		result += "[Image generated successfully]\n\n"
		result += fmt.Sprintf("Format: %s\n", image.MimeType)
		result += fmt.Sprintf("Size: %d bytes (%.1f KB)\n", len(image.Data), float64(len(image.Data))/1024)
		result += "\nNote: Image was generated but is too large to display inline.\n"
		result += "Configure GCS storage in your agents.yaml to get shareable image URLs.\n"
		return result
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
		for sessionKey, entry := range t.sessions {
			if now.Sub(entry.lastUsed) > t.sessionTimeout {
				_ = entry.session.Close()
				delete(t.sessions, sessionKey)
			}
		}
		t.sessionsMu.Unlock()
	}
}

// GetActiveSessions returns the number of active sessions (useful for monitoring)
func (t *Tool) GetActiveSessions() int {
	if !t.multiTurnEnabled {
		return 0
	}
	t.sessionsMu.RLock()
	defer t.sessionsMu.RUnlock()
	return len(t.sessions)
}

// formatResult creates a human-readable result string with URL
// The image is formatted using markdown syntax so UIs can render it
func (t *Tool) formatResult(response *interfaces.ImageGenerationResponse, prompt, imageURL string) string {
	result := fmt.Sprintf("Successfully generated image for prompt: \"%s\"\n\n", truncateString(prompt, 100))

	if imageURL != "" {
		// Use markdown image syntax for UI rendering
		result += fmt.Sprintf("![Generated image](%s)\n\n", imageURL)
	}

	result += fmt.Sprintf("Format: %s\n", response.Images[0].MimeType)
	result += fmt.Sprintf("Size: %d bytes\n", len(response.Images[0].Data))

	if response.Usage != nil {
		result += fmt.Sprintf("\nTokens used: %d input, %d output\n",
			response.Usage.InputTokens, response.Usage.OutputTokens)
	}

	return result
}

// formatResultWithBase64 creates a result string with base64 data (fallback when storage fails)
// The image is embedded as a data URI using markdown syntax for UI rendering
func (t *Tool) formatResultWithBase64(response *interfaces.ImageGenerationResponse, prompt string) string {
	result := fmt.Sprintf("Successfully generated image for prompt: \"%s\"\n\n", truncateString(prompt, 100))

	// Create data URI for direct embedding in markdown
	dataURI := fmt.Sprintf("data:%s;base64,%s", response.Images[0].MimeType, response.Images[0].Base64)
	result += fmt.Sprintf("![Generated image](%s)\n\n", dataURI)

	result += fmt.Sprintf("Format: %s\n", response.Images[0].MimeType)
	result += fmt.Sprintf("Size: %d bytes\n", len(response.Images[0].Data))

	return result
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// GetImageReference returns an ImageReference for storing in memory
func GetImageReference(image *interfaces.GeneratedImage, prompt string) interfaces.ImageReference {
	return interfaces.ImageReference{
		URL:       image.URL,
		MimeType:  image.MimeType,
		Prompt:    prompt,
		CreatedAt: time.Now(),
	}
}
