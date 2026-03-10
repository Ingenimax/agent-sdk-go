package a2a

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// Client discovers and communicates with remote A2A-compliant agents.
type Client struct {
	url         string
	card        *a2a.AgentCard
	a2aClient   *a2aclient.Client
	httpClient  *http.Client
	logger      logging.Logger
	timeout     time.Duration
	bearerToken string
}

// NewClient creates a new A2A client that connects to the agent at the given URL.
// It resolves the agent card from /.well-known/agent-card.json automatically.
func NewClient(ctx context.Context, agentURL string, opts ...ClientOption) (*Client, error) {
	c := &Client{
		url:     agentURL,
		logger:  logging.New(),
		timeout: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(c)
	}

	// Resolve agent card
	card, err := agentcard.DefaultResolver.Resolve(ctx, agentURL)
	if err != nil {
		return nil, fmt.Errorf("a2a client: failed to resolve agent card from %s: %w", agentURL, err)
	}
	c.card = card

	c.logger.Info(ctx, "A2A client: resolved agent card", map[string]interface{}{
		"agent_name":   card.Name,
		"agent_url":    agentURL,
		"skills_count": len(card.Skills),
		"streaming":    card.Capabilities.Streaming,
	})

	a2aC, err := a2aclient.NewFromCard(ctx, card, c.factoryOptions()...)
	if err != nil {
		return nil, fmt.Errorf("a2a client: failed to create client for %s: %w", agentURL, err)
	}
	c.a2aClient = a2aC

	return c, nil
}

// NewClientFromCard creates a new A2A client from an already-resolved agent card.
func NewClientFromCard(ctx context.Context, card *a2a.AgentCard, opts ...ClientOption) (*Client, error) {
	c := &Client{
		url:     card.URL,
		card:    card,
		logger:  logging.New(),
		timeout: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(c)
	}

	a2aC, err := a2aclient.NewFromCard(ctx, card, c.factoryOptions()...)
	if err != nil {
		return nil, fmt.Errorf("a2a client: failed to create client: %w", err)
	}
	c.a2aClient = a2aC

	return c, nil
}

// factoryOptions builds the a2aclient.FactoryOption slice from Client config.
func (c *Client) factoryOptions() []a2aclient.FactoryOption {
	c.httpClient = &http.Client{Timeout: c.timeout}
	opts := []a2aclient.FactoryOption{
		a2aclient.WithJSONRPCTransport(c.httpClient),
	}
	if c.bearerToken != "" {
		opts = append(opts, a2aclient.WithInterceptors(
			a2aclient.NewStaticCallMetaInjector(a2aclient.CallMeta{
				"authorization": []string{"Bearer " + c.bearerToken},
			}),
		))
	}
	return opts
}

// Card returns the resolved agent card.
func (c *Client) Card() *a2a.AgentCard {
	return c.card
}

// SendMessage sends a synchronous message and returns the result.
func (c *Client) SendMessage(ctx context.Context, text string, opts ...SendOption) (a2a.SendMessageResult, error) {
	cfg := applySendOptions(opts)
	msg := c.buildMessage(text, cfg)
	return c.a2aClient.SendMessage(ctx, &a2a.MessageSendParams{
		Message: msg,
	})
}

// SendMessageStream sends a message and returns a channel of streaming events.
func (c *Client) SendMessageStream(ctx context.Context, text string, opts ...SendOption) func(func(a2a.Event, error) bool) {
	cfg := applySendOptions(opts)
	msg := c.buildMessage(text, cfg)
	return c.a2aClient.SendStreamingMessage(ctx, &a2a.MessageSendParams{
		Message: msg,
	})
}

// buildMessage creates an a2a.Message with optional task/context references.
func (c *Client) buildMessage(text string, cfg sendConfig) *a2a.Message {
	if cfg.taskID != "" || cfg.contextID != "" {
		return a2a.NewMessageForTask(a2a.MessageRoleUser, a2a.TaskInfo{
			TaskID:    cfg.taskID,
			ContextID: cfg.contextID,
		}, a2a.TextPart{Text: text})
	}
	return a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: text})
}

// applySendOptions folds variadic options into a sendConfig.
func applySendOptions(opts []SendOption) sendConfig {
	var cfg sendConfig
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// GetTask retrieves a task by ID.
func (c *Client) GetTask(ctx context.Context, taskID a2a.TaskID) (*a2a.Task, error) {
	return c.a2aClient.GetTask(ctx, &a2a.TaskQueryParams{ID: taskID})
}

// CancelTask cancels a running task.
func (c *Client) CancelTask(ctx context.Context, taskID a2a.TaskID) (*a2a.Task, error) {
	return c.a2aClient.CancelTask(ctx, &a2a.TaskIDParams{ID: taskID})
}

// Close releases resources held by the client. It closes idle connections
// on the underlying HTTP transport. Safe to call multiple times.
func (c *Client) Close() {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}
