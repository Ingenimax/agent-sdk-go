package anthropic

import (
	"encoding/json"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// cacheRequestBuilder transforms standard requests into cacheable requests
// when prompt caching is enabled.
type cacheRequestBuilder struct {
	config *interfaces.CacheConfig
}

// newCacheRequestBuilder creates a new cache request builder with the given config.
func newCacheRequestBuilder(config *interfaces.CacheConfig) *cacheRequestBuilder {
	return &cacheRequestBuilder{config: config}
}

// HasCacheOptions returns true if any caching options are enabled.
func (b *cacheRequestBuilder) HasCacheOptions() bool {
	if b.config == nil {
		return false
	}
	return b.config.CacheSystemMessage || b.config.CacheTools || b.config.CacheConversation
}

// getCacheControl returns the cache control block based on config TTL.
func (b *cacheRequestBuilder) getCacheControl() *CacheControl {
	if b.config != nil && b.config.CacheTTL != "" {
		return NewCacheControlWithTTL(b.config.CacheTTL)
	}
	return NewCacheControl()
}

// BuildSystemContent converts a system message string to cacheable content blocks.
// If caching is disabled, returns the original string as JSON.
// If caching is enabled, returns an array of content blocks with cache_control.
func (b *cacheRequestBuilder) BuildSystemContent(system string) (json.RawMessage, error) {
	if b.config == nil || !b.config.CacheSystemMessage {
		// Return as simple string for backwards compatibility
		return json.Marshal(system)
	}

	// Convert to array of content blocks with cache_control
	content := []CacheableSystemContent{
		{
			Type:         "text",
			Text:         system,
			CacheControl: b.getCacheControl(),
		},
	}
	return json.Marshal(content)
}

// BuildMessages converts messages to cacheable format.
// If CacheConversation is enabled, puts cache_control on the last message.
// Returns the messages as JSON that can be used in the request.
func (b *cacheRequestBuilder) BuildMessages(messages []Message) (json.RawMessage, error) {
	if b.config == nil || !b.config.CacheConversation || len(messages) == 0 {
		// No caching needed, return as-is
		return json.Marshal(messages)
	}

	// Convert to mixed format: regular messages + last message as cacheable
	result := make([]interface{}, len(messages))

	for i := 0; i < len(messages)-1; i++ {
		result[i] = messages[i]
	}

	// Last message gets cache_control
	lastMsg := messages[len(messages)-1]
	result[len(messages)-1] = CacheableMessage{
		Role: lastMsg.Role,
		Content: []CacheableContent{
			{
				Type:         "text",
				Text:         messageTextContent(lastMsg),
				CacheControl: b.getCacheControl(),
			},
		},
	}

	return json.Marshal(result)
}

// BuildTools converts tools to cacheable format.
// If CacheTools is enabled, puts cache_control on the last tool.
// Returns the tools as JSON that can be used in the request.
func (b *cacheRequestBuilder) BuildTools(tools []Tool) (json.RawMessage, error) {
	if b.config == nil || !b.config.CacheTools || len(tools) == 0 {
		// No caching needed, return as-is
		return json.Marshal(tools)
	}

	// Convert to mixed format: regular tools + last tool as cacheable
	result := make([]interface{}, len(tools))

	for i := 0; i < len(tools)-1; i++ {
		result[i] = tools[i]
	}

	// Last tool gets cache_control
	lastTool := tools[len(tools)-1]
	result[len(tools)-1] = CacheableTool{
		Name:         lastTool.Name,
		Description:  lastTool.Description,
		InputSchema:  lastTool.InputSchema,
		CacheControl: b.getCacheControl(),
	}

	return json.Marshal(result)
}

// CacheableCompletionRequest is a version of CompletionRequest that uses
// json.RawMessage for fields that may need different JSON structures
// depending on whether caching is enabled.
type CacheableCompletionRequest struct {
	Model            string          `json:"model,omitempty"`
	Messages         json.RawMessage `json:"messages"`
	MaxTokens        int             `json:"max_tokens,omitempty"`
	Temperature      float64         `json:"temperature,omitempty"`
	TopP             float64         `json:"top_p,omitempty"`
	TopK             int             `json:"top_k,omitempty"`
	StopSequences    []string        `json:"stop_sequences,omitempty"`
	System           json.RawMessage `json:"system,omitempty"`
	Tools            json.RawMessage `json:"tools,omitempty"`
	ToolChoice       interface{}     `json:"tool_choice,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	AnthropicVersion string          `json:"anthropic_version,omitempty"`
	Thinking         *ReasoningSpec  `json:"thinking,omitempty"`
}

// BuildCacheableRequest creates a CacheableCompletionRequest from a standard
// CompletionRequest, applying cache_control where configured.
func (b *cacheRequestBuilder) BuildCacheableRequest(req *CompletionRequest) (*CacheableCompletionRequest, error) {
	cacheableReq := &CacheableCompletionRequest{
		Model:            req.Model,
		MaxTokens:        req.MaxTokens,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		TopK:             req.TopK,
		StopSequences:    req.StopSequences,
		ToolChoice:       req.ToolChoice,
		Stream:           req.Stream,
		AnthropicVersion: req.AnthropicVersion,
		Thinking:         req.Thinking,
	}

	// Build system content
	if req.System != "" {
		systemContent, err := b.BuildSystemContent(req.System)
		if err != nil {
			return nil, err
		}
		cacheableReq.System = systemContent
	}

	// Build messages
	messages, err := b.BuildMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	cacheableReq.Messages = messages

	// Build tools if present
	if len(req.Tools) > 0 {
		tools, err := b.BuildTools(req.Tools)
		if err != nil {
			return nil, err
		}
		cacheableReq.Tools = tools
	}

	return cacheableReq, nil
}
