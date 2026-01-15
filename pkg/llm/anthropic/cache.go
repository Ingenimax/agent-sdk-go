package anthropic

// CacheControl represents Anthropic's cache_control block for prompt caching.
// When added to a content block, it marks that block as a cache breakpoint,
// caching everything up to and including that block.
type CacheControl struct {
	Type string `json:"type"`          // "ephemeral" is the only supported value
	TTL  string `json:"ttl,omitempty"` // "5m" (default) or "1h"
}

// CacheableContent represents a content block that can have cache_control.
// Used for message content when caching is enabled.
type CacheableContent struct {
	Type         string        `json:"type"`                    // "text", "image", "tool_use", etc.
	Text         string        `json:"text,omitempty"`          // For text content
	CacheControl *CacheControl `json:"cache_control,omitempty"` // Optional cache control
}

// CacheableSystemContent represents a system message content block with cache_control.
// System messages use a slightly different structure than regular messages.
type CacheableSystemContent struct {
	Type         string        `json:"type"`                    // "text"
	Text         string        `json:"text"`                    // System message text
	CacheControl *CacheControl `json:"cache_control,omitempty"` // Optional cache control
}

// CacheableMessage represents a message with array-based content for caching.
// When caching is enabled, messages use content arrays instead of simple strings.
type CacheableMessage struct {
	Role    string             `json:"role"`    // "user" or "assistant"
	Content []CacheableContent `json:"content"` // Array of content blocks
}

// CacheableTool represents a tool definition with cache_control support.
// The cache_control is placed on the last tool to cache all tool definitions.
type CacheableTool struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	InputSchema  map[string]interface{} `json:"input_schema"`
	CacheControl *CacheControl          `json:"cache_control,omitempty"`
}

// NewCacheControl creates a new CacheControl with the default 5-minute TTL.
func NewCacheControl() *CacheControl {
	return &CacheControl{Type: "ephemeral"}
}

// NewCacheControlWithTTL creates a new CacheControl with a specific TTL.
// Valid TTL values are "5m" (default) or "1h".
func NewCacheControlWithTTL(ttl string) *CacheControl {
	if ttl == "" || ttl == "5m" {
		return &CacheControl{Type: "ephemeral"}
	}
	return &CacheControl{Type: "ephemeral", TTL: ttl}
}
