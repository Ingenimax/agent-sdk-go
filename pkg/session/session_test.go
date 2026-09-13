package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func newManager(t *testing.T) *Manager {
	t.Helper()
	return NewManager(NewInMemoryStore(), WithAppName("testapp"))
}

func TestCreateAndGet(t *testing.T) {
	m := newManager(t)
	ctx := context.Background()

	created, err := m.Create(ctx, "org1", "user1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create() produced no ID")
	}

	got, err := m.Get(ctx, "org1", created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.UserID != "user1" {
		t.Errorf("UserID = %q, want %q", got.UserID, "user1")
	}
	if got.AppName != "testapp" {
		t.Errorf("AppName = %q, want %q", got.AppName, "testapp")
	}
}

func TestGetUnknownSession(t *testing.T) {
	m := newManager(t)
	if _, err := m.Get(context.Background(), "org1", "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(unknown) error = %v, want ErrNotFound", err)
	}
}

// TestBindSuppliesTheScopingMemoryRequires is the reason sessions matter rather
// than being decorative.
//
// memory.getConversationID needs BOTH an org ID and a conversation ID and hard-
// fails without either, which is why a background or scheduled run -- having no
// inbound request to inherit from -- could not write to memory at all.
func TestBindSuppliesTheScopingMemoryRequires(t *testing.T) {
	m := newManager(t)
	ctx := context.Background()

	s, err := m.Create(ctx, "org1", "user1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	bound := m.Bind(ctx, s)

	orgID, err := multitenancy.GetOrgID(bound)
	if err != nil {
		t.Fatalf("GetOrgID() error = %v", err)
	}
	if orgID != "org1" {
		t.Errorf("org ID = %q, want %q", orgID, "org1")
	}

	convID, ok := memory.GetConversationID(bound)
	if !ok {
		t.Fatal("no conversation ID on the bound context")
	}
	if convID != s.ID {
		t.Errorf("conversation ID = %q, want the session ID %q", convID, s.ID)
	}

	// A bound context must actually work with memory.
	mem := memory.NewConversationBuffer()
	if err := mem.AddMessage(bound, interfaces.Message{
		Role:    interfaces.MessageRoleUser,
		Content: "hello",
	}); err != nil {
		t.Errorf("memory rejected a session-bound context: %v", err)
	}
}

// TestSessionIDIsTheConversationID pins the decision not to introduce a second
// ID space. A mapping table between session and conversation would be one more
// thing to keep in step for no benefit.
func TestSessionIDIsTheConversationID(t *testing.T) {
	m := newManager(t)
	ctx := context.Background()

	s, _ := m.Create(ctx, "org1", "user1")
	bound := m.Bind(ctx, s)

	convID, _ := memory.GetConversationID(bound)
	if convID != s.ID {
		t.Errorf("conversation ID %q differs from session ID %q: these must be the "+
			"same value, or a session and its transcript can drift apart", convID, s.ID)
	}
}

func TestStatePrefixScoping(t *testing.T) {
	store := NewInMemoryStore()
	m := NewManager(store, WithAppName("testapp"))
	ctx := context.Background()

	first, _ := m.Create(ctx, "org1", "user1")
	first.Set("private_key", "only mine")
	first.Set(UserPrefix+"timezone", "Europe/Oslo")
	first.Set(AppPrefix+"model", "gpt-4o")
	first.Set(TempPrefix+"scratch", "discard me")
	if err := m.Save(ctx, first); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// temp: must never survive a save.
	reloaded, _ := m.Get(ctx, "org1", first.ID)
	if _, ok := reloaded.Get(TempPrefix + "scratch"); ok {
		t.Error("a temp: key was persisted; it must be dropped on save")
	}
	if got := reloaded.GetString("private_key"); got != "only mine" {
		t.Errorf("private key = %q, want it preserved", got)
	}

	// user: and app: reach a different session belonging to the same user.
	second, _ := m.Create(ctx, "org1", "user1")
	loaded, _ := m.Get(ctx, "org1", second.ID)

	if got := loaded.GetString(UserPrefix + "timezone"); got != "Europe/Oslo" {
		t.Errorf("user-scoped key = %q, want it shared across the user's sessions", got)
	}
	if got := loaded.GetString(AppPrefix + "model"); got != "gpt-4o" {
		t.Errorf("app-scoped key = %q, want it shared across the application", got)
	}
	if _, ok := loaded.Get("private_key"); ok {
		t.Error("an unprefixed key leaked into another session; it must stay private")
	}
}

func TestDeleteKeepsSharedState(t *testing.T) {
	store := NewInMemoryStore()
	m := NewManager(store, WithAppName("testapp"))
	ctx := context.Background()

	first, _ := m.Create(ctx, "org1", "user1")
	first.Set(UserPrefix+"timezone", "Europe/Oslo")
	_ = m.Save(ctx, first)

	second, _ := m.Create(ctx, "org1", "user1")

	if err := m.Delete(ctx, "org1", first.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	loaded, err := m.Get(ctx, "org1", second.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got := loaded.GetString(UserPrefix + "timezone"); got != "Europe/Oslo" {
		t.Error("deleting one session removed user-scoped state that belongs to " +
			"the user's other sessions too")
	}
}

func TestListIsScopedAndOrdered(t *testing.T) {
	m := newManager(t)
	ctx := context.Background()

	a, _ := m.Create(ctx, "org1", "user1")
	b, _ := m.Create(ctx, "org1", "user1")
	_, _ = m.Create(ctx, "org1", "user2")
	_, _ = m.Create(ctx, "org2", "user1")

	// Touch b so it is the most recently updated.
	_ = m.Save(ctx, b)

	got, err := m.List(ctx, Filter{OrgID: "org1", UserID: "user1"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() returned %d sessions, want 2 (scoped to org1/user1)", len(got))
	}
	if got[0].ID != b.ID {
		t.Errorf("first session = %q, want the most recently updated %q", got[0].ID, b.ID)
	}
	_ = a
}

func TestListRespectsLimit(t *testing.T) {
	m := newManager(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, _ = m.Create(ctx, "org1", "user1")
	}

	got, _ := m.List(ctx, Filter{OrgID: "org1", Limit: 2})
	if len(got) != 2 {
		t.Errorf("List() returned %d sessions, want 2", len(got))
	}
}

func TestForkCarriesStateNotTranscript(t *testing.T) {
	m := newManager(t)
	ctx := context.Background()

	parent, _ := m.Create(ctx, "org1", "user1")
	parent.Set("topic", "tide pools")
	parent.MessageCount = 12
	_ = m.Save(ctx, parent)

	child, err := m.Fork(ctx, "org1", parent.ID)
	if err != nil {
		t.Fatalf("Fork() error = %v", err)
	}

	if child.ID == parent.ID {
		t.Error("a fork must have its own ID, or it shares the parent's transcript")
	}
	if got := child.GetString("topic"); got != "tide pools" {
		t.Errorf("forked state topic = %q, want it carried from the parent", got)
	}
	if child.MessageCount != 0 {
		t.Errorf("MessageCount = %d, want 0: a fork starts a fresh conversation", child.MessageCount)
	}
	if got := child.GetString("forked_from"); got != parent.ID {
		t.Errorf("forked_from = %q, want the parent ID", got)
	}
}

func TestTouchDerivesTitleOnce(t *testing.T) {
	m := newManager(t)
	ctx := context.Background()

	s, _ := m.Create(ctx, "org1", "user1")
	if err := m.Touch(ctx, s, 2, 150, "How do tide pools form on rocky shores?"); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}
	if s.Title == "" {
		t.Fatal("Touch did not derive a title from the first message")
	}
	first := s.Title

	_ = m.Touch(ctx, s, 2, 100, "a completely different follow-up question")
	if s.Title != first {
		t.Errorf("title changed to %q; it should be derived once from the first message", s.Title)
	}
	if s.MessageCount != 4 {
		t.Errorf("MessageCount = %d, want 4", s.MessageCount)
	}
	if s.TokenTotal != 250 {
		t.Errorf("TokenTotal = %d, want 250", s.TokenTotal)
	}
}

func TestDeriveTitleTruncatesOnAWordBoundary(t *testing.T) {
	long := strings.Repeat("word ", 40)
	title := DeriveTitle(long)

	if len(title) > 64 {
		t.Errorf("title is %d chars, want it truncated", len(title))
	}
	if strings.HasSuffix(strings.TrimSuffix(title, "…"), " ") {
		t.Error("title should not end on a dangling space")
	}
	if got := DeriveTitle("short one"); got != "short one" {
		t.Errorf("short titles should pass through unchanged; got %q", got)
	}
}

func TestCloneIsDeep(t *testing.T) {
	s := &Session{ID: "s1", State: map[string]any{"k": "v"}}
	clone := s.Clone()
	clone.Set("k", "changed")

	if s.GetString("k") != "v" {
		t.Error("mutating a clone changed the original; Clone must be deep")
	}
}
