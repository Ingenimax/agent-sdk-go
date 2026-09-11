package consolidation

import (
	"context"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ActivityTracker records when a conversation was last written to, so the
// scheduler can tell which are idle.
//
// It is a Memory decorator with no LLM, no goroutine and no failure mode of its
// own: it stamps a timestamp and delegates. Keeping it that dumb means enabling
// consolidation cannot break the write path.
type ActivityTracker struct {
	inner interfaces.Memory

	mu       sync.Mutex
	lastSeen map[string]time.Time
	dirty    map[string]int
}

// NewActivityTracker wraps a Memory to record write activity.
func NewActivityTracker(inner interfaces.Memory) *ActivityTracker {
	return &ActivityTracker{
		inner:    inner,
		lastSeen: map[string]time.Time{},
		dirty:    map[string]int{},
	}
}

// AddMessage implements interfaces.Memory.
func (t *ActivityTracker) AddMessage(ctx context.Context, message interfaces.Message) error {
	if err := t.inner.AddMessage(ctx, message); err != nil {
		return err
	}
	if key, ok := scopeKey(ctx); ok {
		t.mu.Lock()
		t.lastSeen[key] = time.Now()
		t.dirty[key]++
		t.mu.Unlock()
	}
	return nil
}

// GetMessages implements interfaces.Memory.
func (t *ActivityTracker) GetMessages(ctx context.Context, options ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return t.inner.GetMessages(ctx, options...)
}

// Clear implements interfaces.Memory.
func (t *ActivityTracker) Clear(ctx context.Context) error {
	if key, ok := scopeKey(ctx); ok {
		t.mu.Lock()
		delete(t.lastSeen, key)
		delete(t.dirty, key)
		t.mu.Unlock()
	}
	return t.inner.Clear(ctx)
}

// Unwrap returns the wrapped Memory, satisfying interfaces.MemoryUnwrapper so
// this decorator does not hide the optional capabilities of what it wraps.
func (t *ActivityTracker) Unwrap() interfaces.Memory { return t.inner }

// IdleScope is a conversation that has gone quiet.
type IdleScope struct {
	// Key is the memory scope key, "orgID:conversationID".
	Key string
	// OrgID and ConversationID are the key's parts.
	OrgID          string
	ConversationID string
	// Writes is how many messages have arrived since it was last consolidated.
	Writes int
	// LastSeen is when the last write happened.
	LastSeen time.Time
}

// IdleScopes returns conversations that have been quiet for at least idleFor
// and have accumulated at least minWrites messages.
//
// Both conditions matter. Idle time alone would consolidate a conversation that
// has barely started; write count alone would consolidate one mid-exchange,
// competing with the user for the same transcript.
func (t *ActivityTracker) IdleScopes(idleFor time.Duration, minWrites int) []IdleScope {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := time.Now().Add(-idleFor)
	var idle []IdleScope

	for key, seen := range t.lastSeen {
		if seen.After(cutoff) {
			continue
		}
		if t.dirty[key] < minWrites {
			continue
		}
		orgID, conversationID := splitScopeKey(key)
		idle = append(idle, IdleScope{
			Key:            key,
			OrgID:          orgID,
			ConversationID: conversationID,
			Writes:         t.dirty[key],
			LastSeen:       seen,
		})
	}
	return idle
}

// MarkConsolidated resets a scope's write counter, so it is not reconsolidated
// until there is new material.
func (t *ActivityTracker) MarkConsolidated(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dirty[key] = 0
}

var _ interfaces.Memory = (*ActivityTracker)(nil)
var _ interfaces.MemoryUnwrapper = (*ActivityTracker)(nil)

// Scheduler consolidates idle conversations on a timer. This is the "auto" in
// auto-dream.
type Scheduler struct {
	tracker      *ActivityTracker
	consolidator *Consolidator

	interval  time.Duration
	idleFor   time.Duration
	minWrites int
	autoApply bool

	onPlan func(scope IdleScope, plan Plan)

	mu      sync.Mutex
	running bool
	stop    chan struct{}
	done    chan struct{}
}

// SchedulerOption configures a Scheduler.
type SchedulerOption func(*Scheduler)

// WithInterval sets how often idle conversations are looked for. Defaults to
// 5 minutes.
func WithInterval(d time.Duration) SchedulerOption {
	return func(s *Scheduler) {
		if d > 0 {
			s.interval = d
		}
	}
}

// WithIdleFor sets how quiet a conversation must be. Defaults to 15 minutes.
//
// Zero is permitted and means "consolidate whatever is eligible now". That is
// the right setting when RunOnce is driven by an external scheduler, which has
// already decided it is time -- an in-process idle threshold would then be
// second-guessing it.
func WithIdleFor(d time.Duration) SchedulerOption {
	return func(s *Scheduler) {
		if d >= 0 {
			s.idleFor = d
		}
	}
}

// WithMinWrites sets how many new messages must have accumulated. Defaults to 4.
func WithMinWrites(n int) SchedulerOption {
	return func(s *Scheduler) {
		if n > 0 {
			s.minWrites = n
		}
	}
}

// WithAutoApply writes consolidated facts without review.
//
// Off by default. A model rewriting an agent's memory unattended, with no diff
// and no undo, should be a choice someone makes rather than something that
// happens because consolidation was switched on.
func WithAutoApply() SchedulerOption {
	return func(s *Scheduler) { s.autoApply = true }
}

// WithPlanCallback registers a function called with every plan produced,
// whether or not it is applied. This is the review hook.
func WithPlanCallback(fn func(scope IdleScope, plan Plan)) SchedulerOption {
	return func(s *Scheduler) { s.onPlan = fn }
}

// NewScheduler creates an idle-consolidation scheduler.
func NewScheduler(tracker *ActivityTracker, consolidator *Consolidator, options ...SchedulerOption) *Scheduler {
	s := &Scheduler{
		tracker:      tracker,
		consolidator: consolidator,
		interval:     5 * time.Minute,
		idleFor:      15 * time.Minute,
		minWrites:    4,
	}
	for _, option := range options {
		option(s)
	}
	return s
}

// Start begins the timer. It returns immediately. Calling Start twice is a
// no-op.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	stop, done := s.stop, s.done
	s.mu.Unlock()

	go func() {
		defer close(done)

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.RunOnce(ctx)
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop halts the timer and waits for the current pass to finish.
//
// It waits rather than returning immediately so a caller shutting down can be
// sure no consolidation is still writing.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stop)
	done := s.done
	s.mu.Unlock()

	<-done
}

// RunOnce performs a single consolidation pass. Exported so consolidation can
// be driven by an external scheduler -- a cron job, a Kubernetes CronJob -- in
// a multi-replica deployment where an in-process timer would fire N times.
func (s *Scheduler) RunOnce(ctx context.Context) {
	for _, scope := range s.tracker.IdleScopes(s.idleFor, s.minWrites) {
		s.consolidateScope(ctx, scope)
	}
}

func (s *Scheduler) consolidateScope(ctx context.Context, scope IdleScope) {
	// Consolidation needs the same ambient scoping every memory read does.
	scopedCtx := withScope(ctx, scope.OrgID, scope.ConversationID)

	messages, err := s.tracker.GetMessages(scopedCtx)
	if err != nil || len(messages) == 0 {
		return
	}

	plan, err := s.consolidator.Plan(scopedCtx, scope.OrgID, scope.ConversationID, messages)
	if err != nil {
		// A failed pass must never lose anything: nothing has been written, and
		// the scope stays dirty so the next pass retries it.
		return
	}

	if s.onPlan != nil {
		s.onPlan(scope, plan)
	}

	if s.autoApply && !plan.Empty() {
		if err := s.consolidator.Apply(scopedCtx, scope.OrgID, plan); err != nil {
			return
		}
	}

	s.tracker.MarkConsolidated(scope.Key)
}
