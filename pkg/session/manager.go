package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Manager creates and resolves sessions, and stamps the identity that memory
// scoping requires onto a context.
//
// This is the piece that makes sessions useful rather than decorative. Memory
// derives its scope key from multitenancy.GetOrgID and memory.GetConversationID
// and hard-fails when either is missing -- which is why a scheduled or
// background run, having no inbound request to inherit from, could not write to
// memory at all. Resolve produces a context carrying both.
type Manager struct {
	store   Store
	appName string
}

// ManagerOption configures a Manager.
type ManagerOption func(*Manager)

// WithAppName sets the application name used to scope "app:" state.
func WithAppName(name string) ManagerOption {
	return func(m *Manager) { m.appName = name }
}

// NewManager creates a session manager over a store.
func NewManager(store Store, options ...ManagerOption) *Manager {
	m := &Manager{store: store}
	for _, option := range options {
		option(m)
	}
	return m
}

// Create starts a new session.
func (m *Manager) Create(ctx context.Context, orgID, userID string) (*Session, error) {
	s := &Session{
		ID:        NewID(),
		OrgID:     orgID,
		UserID:    userID,
		AppName:   m.appName,
		State:     map[string]any{},
		CreatedAt: time.Now(),
	}
	if err := m.store.Create(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// Get returns an existing session.
func (m *Manager) Get(ctx context.Context, orgID, id string) (*Session, error) {
	return m.store.Get(ctx, orgID, id)
}

// Resume returns an existing session and a context scoped to it, ready to hand
// to an agent.
func (m *Manager) Resume(ctx context.Context, orgID, id string) (*Session, context.Context, error) {
	s, err := m.store.Get(ctx, orgID, id)
	if err != nil {
		return nil, ctx, err
	}
	return s, m.Bind(ctx, s), nil
}

// Bind stamps a session's identity onto a context so memory can scope to it.
//
// Call this before handing the context to an agent. Without it an agent with
// memory configured fails its first turn, because getConversationID requires
// both an org ID and a conversation ID.
func (m *Manager) Bind(ctx context.Context, s *Session) context.Context {
	if s == nil {
		return ctx
	}
	if s.OrgID != "" {
		ctx = multitenancy.WithOrgID(ctx, s.OrgID)
	}
	return memory.WithConversationID(ctx, s.ID)
}

// Save persists changes to a session.
func (m *Manager) Save(ctx context.Context, s *Session) error {
	return m.store.Update(ctx, s)
}

// List returns sessions matching the filter.
func (m *Manager) List(ctx context.Context, filter Filter) ([]*Session, error) {
	return m.store.List(ctx, filter)
}

// Delete removes a session.
func (m *Manager) Delete(ctx context.Context, orgID, id string) error {
	return m.store.Delete(ctx, orgID, id)
}

// Fork creates a new session seeded with another's state.
//
// The transcript is not copied: sessions reference messages by conversation ID
// rather than owning them, so a fork starts a fresh conversation carrying the
// parent's accumulated state. The parent is recorded under "forked_from".
func (m *Manager) Fork(ctx context.Context, orgID, id string) (*Session, error) {
	parent, err := m.store.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}

	child := parent.Clone()
	child.ID = NewID()
	child.CreatedAt = time.Now()
	child.UpdatedAt = time.Time{}
	child.MessageCount = 0
	child.TokenTotal = 0
	child.Set("forked_from", parent.ID)

	if err := m.store.Create(ctx, child); err != nil {
		return nil, err
	}
	return child, nil
}

// Touch records activity on a session: it refreshes UpdatedAt, adds to the
// message and token counters, and derives a title from the first message if the
// session does not have one yet.
func (m *Manager) Touch(ctx context.Context, s *Session, messages, tokens int, firstMessage string) error {
	s.MessageCount += messages
	s.TokenTotal += tokens

	if s.Title == "" && firstMessage != "" {
		s.Title = DeriveTitle(firstMessage)
	}

	return m.store.Update(ctx, s)
}

// DeriveTitle produces a short label from a message.
//
// Kept deliberately dumb -- truncation on a word boundary, no model call. A
// title is a listing convenience, and spending a model round trip (and its
// latency, cost and failure mode) on one is not a good trade.
func DeriveTitle(message string) string {
	const maxLen = 60

	title := strings.TrimSpace(strings.ReplaceAll(message, "\n", " "))
	for strings.Contains(title, "  ") {
		title = strings.ReplaceAll(title, "  ", " ")
	}

	if len(title) <= maxLen {
		return title
	}

	truncated := title[:maxLen]
	if idx := strings.LastIndex(truncated, " "); idx > maxLen/2 {
		truncated = truncated[:idx]
	}
	return truncated + "…"
}

// NewID mints a session ID.
func NewID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("sess_%d", time.Now().UnixNano())
	}
	return "sess_" + hex.EncodeToString(b[:])
}
