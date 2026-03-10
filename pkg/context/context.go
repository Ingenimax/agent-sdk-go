package context

import (
	"context"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Key represents a key for context values
type Key string

const (
	// OrganizationIDKey is the key for the organization ID
	OrganizationIDKey Key = "organization_id"

	// ConversationIDKey is the key for the conversation ID
	ConversationIDKey Key = "conversation_id"

	// UserIDKey is the key for the user ID
	UserIDKey Key = "user_id"

	// RequestIDKey is the key for the request ID
	RequestIDKey Key = "request_id"

	// MemoryKey is the key for the memory
	MemoryKey Key = "memory"

	// ToolsKey is the key for the tools
	ToolsKey Key = "tools"

	// DataStoreKey is the key for the data store
	DataStoreKey Key = "data_store"

	// VectorStoreKey is the key for the vector store
	VectorStoreKey Key = "vector_store"

	// LLMKey is the key for the LLM
	LLMKey Key = "llm"

	// EnvironmentKey is the key for environment variables
	EnvironmentKey Key = "environment"
)

// AgentContext represents the context for an agent
type AgentContext struct {
	ctx context.Context
}

// New creates a new agent context
func New() *AgentContext {
	return &AgentContext{
		ctx: context.Background(),
	}
}

// FromContext creates a new agent context from a standard context
func FromContext(ctx context.Context) *AgentContext {
	return &AgentContext{
		ctx: ctx,
	}
}

// WithOrganizationID sets the organization ID in the context
func (c *AgentContext) WithOrganizationID(orgID string) *AgentContext {
	c.ctx = context.WithValue(c.ctx, OrganizationIDKey, orgID)
	return c
}

// OrganizationID returns the organization ID from the context
func (c *AgentContext) OrganizationID() (string, bool) {
	orgID, ok := c.ctx.Value(OrganizationIDKey).(string)
	return orgID, ok
}

// WithConversationID sets the conversation ID in the context
func (c *AgentContext) WithConversationID(conversationID string) *AgentContext {
	c.ctx = context.WithValue(c.ctx, ConversationIDKey, conversationID)
	return c
}

// ConversationID returns the conversation ID from the context
func (c *AgentContext) ConversationID() (string, bool) {
	conversationID, ok := c.ctx.Value(ConversationIDKey).(string)
	return conversationID, ok
}

// WithUserID sets the user ID in the context
func (c *AgentContext) WithUserID(userID string) *AgentContext {
	c.ctx = context.WithValue(c.ctx, UserIDKey, userID)
	return c
}

// UserID returns the user ID from the context
func (c *AgentContext) UserID() (string, bool) {
	userID, ok := c.ctx.Value(UserIDKey).(string)
	return userID, ok
}

// WithRequestID sets the request ID in the context
func (c *AgentContext) WithRequestID(requestID string) *AgentContext {
	c.ctx = context.WithValue(c.ctx, RequestIDKey, requestID)
	return c
}

// RequestID returns the request ID from the context
func (c *AgentContext) RequestID() (string, bool) {
	requestID, ok := c.ctx.Value(RequestIDKey).(string)
	return requestID, ok
}

// WithMemory sets the memory in the context
func (c *AgentContext) WithMemory(memory interfaces.Memory) *AgentContext {
	c.ctx = context.WithValue(c.ctx, MemoryKey, memory)
	return c
}

// Memory returns the memory from the context
func (c *AgentContext) Memory() (interfaces.Memory, bool) {
	memory, ok := c.ctx.Value(MemoryKey).(interfaces.Memory)
	return memory, ok
}

// WithTools sets the tools in the context
func (c *AgentContext) WithTools(tools interfaces.ToolRegistry) *AgentContext {
	c.ctx = context.WithValue(c.ctx, ToolsKey, tools)
	return c
}

// Tools returns the tools from the context
func (c *AgentContext) Tools() (interfaces.ToolRegistry, bool) {
	tools, ok := c.ctx.Value(ToolsKey).(interfaces.ToolRegistry)
	return tools, ok
}

// WithDataStore sets the data store in the context
func (c *AgentContext) WithDataStore(dataStore interfaces.DataStore) *AgentContext {
	c.ctx = context.WithValue(c.ctx, DataStoreKey, dataStore)
	return c
}

// DataStore returns the data store from the context
func (c *AgentContext) DataStore() (interfaces.DataStore, bool) {
	dataStore, ok := c.ctx.Value(DataStoreKey).(interfaces.DataStore)
	return dataStore, ok
}

// WithVectorStore sets the vector store in the context
func (c *AgentContext) WithVectorStore(vectorStore interfaces.VectorStore) *AgentContext {
	c.ctx = context.WithValue(c.ctx, VectorStoreKey, vectorStore)
	return c
}

// VectorStore returns the vector store from the context
func (c *AgentContext) VectorStore() (interfaces.VectorStore, bool) {
	vectorStore, ok := c.ctx.Value(VectorStoreKey).(interfaces.VectorStore)
	return vectorStore, ok
}

// WithLLM sets the LLM in the context
func (c *AgentContext) WithLLM(llm interfaces.LLM) *AgentContext {
	c.ctx = context.WithValue(c.ctx, LLMKey, llm)
	return c
}

// LLM returns the LLM from the context
func (c *AgentContext) LLM() (interfaces.LLM, bool) {
	llm, ok := c.ctx.Value(LLMKey).(interfaces.LLM)
	return llm, ok
}

// WithEnvironment sets an environment variable in the context
func (c *AgentContext) WithEnvironment(key string, value interface{}) *AgentContext {
	env, ok := c.ctx.Value(EnvironmentKey).(map[string]interface{})
	if !ok {
		env = make(map[string]interface{})
	}
	env[key] = value
	c.ctx = context.WithValue(c.ctx, EnvironmentKey, env)
	return c
}

// Environment returns an environment variable from the context
func (c *AgentContext) Environment(key string) (interface{}, bool) {
	env, ok := c.ctx.Value(EnvironmentKey).(map[string]interface{})
	if !ok {
		return nil, false
	}
	value, ok := env[key]
	return value, ok
}

// WithTimeout sets a timeout for the context
func (c *AgentContext) WithTimeout(timeout time.Duration) (*AgentContext, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(c.ctx, timeout) // #nosec G118 - cancel func is returned to caller
	return &AgentContext{ctx: ctx}, cancel
}

// WithDeadline sets a deadline for the context
func (c *AgentContext) WithDeadline(deadline time.Time) (*AgentContext, context.CancelFunc) {
	ctx, cancel := context.WithDeadline(c.ctx, deadline) // #nosec G118 - cancel func is returned to caller
	return &AgentContext{ctx: ctx}, cancel
}

// WithCancel returns a new context that can be canceled
func (c *AgentContext) WithCancel() (*AgentContext, context.CancelFunc) {
	ctx, cancel := context.WithCancel(c.ctx) // #nosec G118 - cancel func is returned to caller
	return &AgentContext{ctx: ctx}, cancel
}

// Context returns the underlying context.Context
func (c *AgentContext) Context() context.Context {
	return c.ctx
}

// Done returns the done channel from the context
func (c *AgentContext) Done() <-chan struct{} {
	return c.ctx.Done()
}

// Err returns the error from the context
func (c *AgentContext) Err() error {
	return c.ctx.Err()
}

// Deadline returns the deadline from the context
func (c *AgentContext) Deadline() (time.Time, bool) {
	return c.ctx.Deadline()
}

// Value returns a value from the context
func (c *AgentContext) Value(key interface{}) interface{} {
	return c.ctx.Value(key)
}
