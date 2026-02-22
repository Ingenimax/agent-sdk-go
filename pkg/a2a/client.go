package a2a

import (
	"context"
	"fmt"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// Client discovers and communicates with remote A2A-compliant agents.
type Client struct {
	url       string
	card      *a2a.AgentCard
	a2aClient *a2aclient.Client
	logger    logging.Logger
	timeout   time.Duration
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

	// Create the underlying a2a client
	a2aC, err := a2aclient.NewFromCard(ctx, card)
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

	a2aC, err := a2aclient.NewFromCard(ctx, card)
	if err != nil {
		return nil, fmt.Errorf("a2a client: failed to create client: %w", err)
	}
	c.a2aClient = a2aC

	return c, nil
}

// Card returns the resolved agent card.
func (c *Client) Card() *a2a.AgentCard {
	return c.card
}

// SendMessage sends a synchronous message and returns the result.
func (c *Client) SendMessage(ctx context.Context, text string) (a2a.SendMessageResult, error) {
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: text})
	return c.a2aClient.SendMessage(ctx, &a2a.MessageSendParams{
		Message: msg,
	})
}

// SendMessageStream sends a message and returns a channel of streaming events.
func (c *Client) SendMessageStream(ctx context.Context, text string) func(func(a2a.Event, error) bool) {
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: text})
	return c.a2aClient.SendStreamingMessage(ctx, &a2a.MessageSendParams{
		Message: msg,
	})
}

// GetTask retrieves a task by ID.
func (c *Client) GetTask(ctx context.Context, taskID a2a.TaskID) (*a2a.Task, error) {
	return c.a2aClient.GetTask(ctx, &a2a.TaskQueryParams{ID: taskID})
}

// CancelTask cancels a running task.
func (c *Client) CancelTask(ctx context.Context, taskID a2a.TaskID) (*a2a.Task, error) {
	return c.a2aClient.CancelTask(ctx, &a2a.TaskIDParams{ID: taskID})
}
