// Package consolidation reviews an agent's accumulated memory when it is idle
// and distils it into durable facts.
//
// The shape of the problem: a long conversation accumulates the same fact
// stated several ways, facts that later turn out to be wrong, and episodic
// detail ("I asked X at 14:02") whose value is the general statement inside it.
// Re-reading all of that on every turn is expensive and gets worse over time.
//
// Consolidation runs off the critical path, reads a transcript, and proposes a
// set of durable facts: merges of duplicates, corrections where a later
// statement contradicts an earlier one, and generalisations of episodic detail.
//
// # It proposes; it does not silently rewrite
//
// A Plan is produced first and applied only when asked. Memory is the record of
// what was said, and a model quietly rewriting it -- with no diff, no
// provenance and no undo -- is not a trade worth making by default. Apply
// writes facts to a separate Store; the transcript is never mutated.
package consolidation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// FactKind classifies what a consolidation step does.
type FactKind string

const (
	// KindNew is a fact not previously recorded.
	KindNew FactKind = "new"
	// KindMerged combines duplicates into one statement.
	KindMerged FactKind = "merged"
	// KindCorrected supersedes an earlier fact contradicted by a later one.
	KindCorrected FactKind = "corrected"
	// KindGeneralized promotes episodic detail into a durable statement.
	KindGeneralized FactKind = "generalized"
)

// Fact is one durable statement distilled from a conversation.
type Fact struct {
	// ID is stable for a given Subject+Statement, so re-running consolidation
	// updates a fact rather than duplicating it.
	ID string `json:"id"`

	// Subject is what the fact is about, used for grouping and lookup.
	Subject string `json:"subject"`

	// Statement is the fact itself, in plain language.
	Statement string `json:"statement"`

	// Kind records how the fact was arrived at.
	Kind FactKind `json:"kind"`

	// Confidence is the model's stated confidence, 0..1.
	Confidence float64 `json:"confidence"`

	// Supersedes lists fact IDs this one replaces.
	Supersedes []string `json:"supersedes,omitempty"`

	// Evidence quotes the source text, so a human reviewing a plan can check
	// the fact against what was actually said rather than trusting it.
	Evidence string `json:"evidence,omitempty"`

	// ConversationID is where the fact came from.
	ConversationID string `json:"conversation_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// Plan is a proposed set of changes, before anything is written.
type Plan struct {
	// Facts are the proposed durable statements.
	Facts []Fact

	// Skipped records what the model declined to consolidate and why, so an
	// empty plan can be distinguished from a failed one.
	Skipped []string

	// MessagesRead is how many messages the plan was derived from.
	MessagesRead int
}

// Empty reports whether the plan would change nothing.
func (p Plan) Empty() bool { return len(p.Facts) == 0 }

// Diff renders the plan for review.
func (p Plan) Diff() string {
	if p.Empty() {
		return fmt.Sprintf("No facts proposed from %d messages.", p.MessagesRead)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d fact(s) proposed from %d messages:\n\n", len(p.Facts), p.MessagesRead)

	for _, f := range p.Facts {
		fmt.Fprintf(&b, "  [%s] %s\n      %s (confidence %.2f)\n",
			f.Kind, f.Subject, f.Statement, f.Confidence)
		if len(f.Supersedes) > 0 {
			fmt.Fprintf(&b, "      supersedes: %s\n", strings.Join(f.Supersedes, ", "))
		}
	}

	if len(p.Skipped) > 0 {
		fmt.Fprintf(&b, "\nSkipped:\n")
		for _, s := range p.Skipped {
			fmt.Fprintf(&b, "  - %s\n", s)
		}
	}
	return b.String()
}

// Store holds consolidated facts. It is deliberately separate from the
// transcript: consolidation adds a derived layer rather than editing the record
// of what was said.
type Store interface {
	// Put writes or replaces facts.
	Put(ctx context.Context, orgID string, facts []Fact) error

	// List returns facts for an org, optionally filtered by subject.
	List(ctx context.Context, orgID, subject string) ([]Fact, error)

	// Delete removes facts by ID.
	Delete(ctx context.Context, orgID string, ids []string) error
}

// InMemoryStore keeps facts in process.
type InMemoryStore struct {
	mu    sync.RWMutex
	facts map[string]map[string]Fact // orgID -> factID -> Fact
}

// NewInMemoryStore creates an in-process fact store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{facts: map[string]map[string]Fact{}}
}

// Put implements Store.
func (s *InMemoryStore) Put(_ context.Context, orgID string, facts []Fact) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.facts[orgID] == nil {
		s.facts[orgID] = map[string]Fact{}
	}
	for _, f := range facts {
		// A superseding fact removes what it replaces, so a corrected statement
		// does not sit alongside the thing it corrected.
		for _, supersededID := range f.Supersedes {
			delete(s.facts[orgID], supersededID)
		}
		s.facts[orgID][f.ID] = f
	}
	return nil
}

// List implements Store.
func (s *InMemoryStore) List(_ context.Context, orgID, subject string) ([]Fact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Fact
	for _, f := range s.facts[orgID] {
		if subject != "" && !strings.EqualFold(f.Subject, subject) {
			continue
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		return out[i].Statement < out[j].Statement
	})
	return out, nil
}

// Delete implements Store.
func (s *InMemoryStore) Delete(_ context.Context, orgID string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.facts[orgID], id)
	}
	return nil
}

var _ Store = (*InMemoryStore)(nil)
