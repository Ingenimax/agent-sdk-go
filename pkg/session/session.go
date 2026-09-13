// Package session gives a conversation a durable identity, metadata and
// scoped state.
//
// Scoping in the SDK is ambient: memory derives an "orgID:conversationID" key
// from the context and hard-fails if either is missing. That is enough to
// isolate transcripts and no more. There is no object describing a
// conversation, nothing to list with titles and timestamps, nowhere to keep a
// fact that should outlive one turn, and no way to resume by ID with anything
// other than a bare transcript.
//
// A Session supplies that. It deliberately does NOT own the transcript:
// pkg/memory already stores messages, keyed by the same conversation ID a
// Session is identified by. Duplicating messages here would create two sources
// of truth for the same data.
//
//	Session.ID == the conversation ID used by pkg/memory
//
// # State scoping
//
// State keys are scoped by prefix, following the model Google's ADK uses:
//
//	"user:"   shared across every session belonging to one user
//	"app:"    shared across every session in the application
//	"temp:"   never persisted; dropped on save
//	(none)    private to this session
//
// The prefixes are resolved at the storage layer, so a caller reads and writes
// one flat namespace.
package session

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned when no session matches an ID.
var ErrNotFound = errors.New("session not found")

// State key prefixes.
const (
	// UserPrefix scopes a key to the user, across all their sessions.
	UserPrefix = "user:"
	// AppPrefix scopes a key to the application, across all users.
	AppPrefix = "app:"
	// TempPrefix marks a key as never persisted.
	TempPrefix = "temp:"
)

// Session is one conversation.
type Session struct {
	// ID is the conversation ID. It is the same value pkg/memory scopes the
	// transcript by, so a session and its messages are never out of step.
	ID string

	// OrgID is the owning organization, matching pkg/multitenancy.
	OrgID string

	// UserID identifies the end user, and scopes "user:" state.
	UserID string

	// AppName scopes "app:" state. Optional.
	AppName string

	// Title is a human-readable label, usually derived from the first message.
	Title string

	// State is the session's scoped key-value store. Read it through Get and
	// write it through Set so prefixes are handled consistently.
	State map[string]any

	CreatedAt time.Time
	UpdatedAt time.Time

	// MessageCount and TokenTotal are denormalised counters, maintained by the
	// caller. They exist so a session list can show useful numbers without
	// loading every transcript.
	MessageCount int
	TokenTotal   int
}

// Get reads a state key.
func (s *Session) Get(key string) (any, bool) {
	if s.State == nil {
		return nil, false
	}
	v, ok := s.State[key]
	return v, ok
}

// GetString reads a state key as a string.
func (s *Session) GetString(key string) string {
	v, ok := s.Get(key)
	if !ok {
		return ""
	}
	str, _ := v.(string)
	return str
}

// Set writes a state key. Prefixes determine how far the value is shared; see
// the package documentation.
func (s *Session) Set(key string, value any) {
	if s.State == nil {
		s.State = map[string]any{}
	}
	s.State[key] = value
}

// Delete removes a state key.
func (s *Session) Delete(key string) {
	delete(s.State, key)
}

// Clone returns a deep copy, so a caller cannot mutate a store's internals
// through a returned session.
func (s *Session) Clone() *Session {
	if s == nil {
		return nil
	}
	clone := *s
	clone.State = make(map[string]any, len(s.State))
	for k, v := range s.State {
		clone.State[k] = v
	}
	return &clone
}

// Filter narrows a session listing.
type Filter struct {
	OrgID  string
	UserID string

	// Limit caps the number of sessions returned. Zero means no limit.
	Limit int
}

// Store persists sessions.
//
// Implementations must treat state prefixes as described in the package
// documentation: "temp:" keys are dropped on save, and "user:"/"app:" keys are
// shared across the sessions in that scope rather than copied into each.
type Store interface {
	// Create stores a new session. It returns an error if the ID already exists.
	Create(ctx context.Context, s *Session) error

	// Get returns a session by ID, with shared state merged in.
	Get(ctx context.Context, orgID, id string) (*Session, error)

	// Update replaces a session's mutable fields.
	Update(ctx context.Context, s *Session) error

	// Delete removes a session and its private state.
	Delete(ctx context.Context, orgID, id string) error

	// List returns sessions matching the filter, most recently updated first.
	// The returned sessions carry metadata but not state.
	List(ctx context.Context, filter Filter) ([]*Session, error)
}

// splitState separates a flat state map into the three persisted scopes,
// discarding "temp:" keys.
func splitState(state map[string]any) (private, user, app map[string]any) {
	private = map[string]any{}
	user = map[string]any{}
	app = map[string]any{}

	for k, v := range state {
		switch {
		case strings.HasPrefix(k, TempPrefix):
			// Deliberately dropped: temp state exists for the duration of one
			// invocation and must never reach storage.
		case strings.HasPrefix(k, UserPrefix):
			user[k] = v
		case strings.HasPrefix(k, AppPrefix):
			app[k] = v
		default:
			private[k] = v
		}
	}
	return private, user, app
}

// mergeState recombines the scopes into the flat namespace callers see.
func mergeState(private, user, app map[string]any) map[string]any {
	merged := make(map[string]any, len(private)+len(user)+len(app))
	for k, v := range app {
		merged[k] = v
	}
	for k, v := range user {
		merged[k] = v
	}
	for k, v := range private {
		merged[k] = v
	}
	return merged
}

// InMemoryStore keeps sessions in process. Suitable for tests, single-process
// deployments and local development.
type InMemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session       // keyed orgID/id
	private  map[string]map[string]any // keyed orgID/id
	user     map[string]map[string]any // keyed orgID/userID
	app      map[string]map[string]any // keyed appName
}

// NewInMemoryStore creates an in-process session store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		sessions: map[string]*Session{},
		private:  map[string]map[string]any{},
		user:     map[string]map[string]any{},
		app:      map[string]map[string]any{},
	}
}

func sessionKey(orgID, id string) string  { return orgID + "/" + id }
func userKey(orgID, userID string) string { return orgID + "/" + userID }

// Create implements Store.
func (s *InMemoryStore) Create(_ context.Context, sess *Session) error {
	if sess == nil || sess.ID == "" {
		return fmt.Errorf("session requires an ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := sessionKey(sess.OrgID, sess.ID)
	if _, exists := s.sessions[key]; exists {
		return fmt.Errorf("session %s already exists", sess.ID)
	}

	now := time.Now()
	stored := sess.Clone()
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now
	}
	stored.UpdatedAt = now

	private, user, app := splitState(stored.State)
	stored.State = nil

	s.sessions[key] = stored
	s.private[key] = private
	s.mergeShared(stored, user, app)

	return nil
}

// Get implements Store.
func (s *InMemoryStore) Get(_ context.Context, orgID, id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := sessionKey(orgID, id)
	stored, ok := s.sessions[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}

	out := stored.Clone()
	out.State = mergeState(
		s.private[key],
		s.user[userKey(orgID, stored.UserID)],
		s.app[stored.AppName],
	)
	return out, nil
}

// Update implements Store.
func (s *InMemoryStore) Update(_ context.Context, sess *Session) error {
	if sess == nil || sess.ID == "" {
		return fmt.Errorf("session requires an ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := sessionKey(sess.OrgID, sess.ID)
	existing, ok := s.sessions[key]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, sess.ID)
	}

	stored := sess.Clone()
	stored.CreatedAt = existing.CreatedAt
	stored.UpdatedAt = time.Now()

	private, user, app := splitState(stored.State)
	stored.State = nil

	s.sessions[key] = stored
	s.private[key] = private
	s.mergeShared(stored, user, app)

	return nil
}

// mergeShared folds user- and app-scoped keys into their shared maps.
// Caller must hold the write lock.
func (s *InMemoryStore) mergeShared(sess *Session, user, app map[string]any) {
	if len(user) > 0 {
		uk := userKey(sess.OrgID, sess.UserID)
		if s.user[uk] == nil {
			s.user[uk] = map[string]any{}
		}
		for k, v := range user {
			s.user[uk][k] = v
		}
	}
	if len(app) > 0 {
		if s.app[sess.AppName] == nil {
			s.app[sess.AppName] = map[string]any{}
		}
		for k, v := range app {
			s.app[sess.AppName][k] = v
		}
	}
}

// Delete implements Store.
func (s *InMemoryStore) Delete(_ context.Context, orgID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := sessionKey(orgID, id)
	if _, ok := s.sessions[key]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}

	// Only private state is removed. User- and app-scoped state belongs to
	// other sessions too, so deleting one session must not take it with them.
	delete(s.sessions, key)
	delete(s.private, key)
	return nil
}

// List implements Store.
func (s *InMemoryStore) List(_ context.Context, filter Filter) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*Session
	for _, stored := range s.sessions {
		if filter.OrgID != "" && stored.OrgID != filter.OrgID {
			continue
		}
		if filter.UserID != "" && stored.UserID != filter.UserID {
			continue
		}
		meta := stored.Clone()
		meta.State = nil
		out = append(out, meta)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})

	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

var _ Store = (*InMemoryStore)(nil)
