package consolidation

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func scopedCtx() context.Context {
	ctx := multitenancy.WithOrgID(context.Background(), "org1")
	return memory.WithConversationID(ctx, "conv1")
}

func transcript() []interfaces.Message {
	return []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "I always deploy on Tuesdays"},
		{Role: interfaces.MessageRoleAssistant, Content: "Noted."},
		{Role: interfaces.MessageRoleUser, Content: "Actually we moved to Thursdays"},
	}
}

const planJSON = `{"facts":[
  {"subject":"deploy schedule","statement":"Deploys happen on Thursdays","kind":"corrected",
   "confidence":0.9,"evidence":"Actually we moved to Thursdays"}
],"skipped":["small talk"]}`

func TestPlanProposesWithoutWriting(t *testing.T) {
	store := NewInMemoryStore()
	llm := &testutil.FakeLLM{DefaultResponse: planJSON}
	c := New(llm, store)

	plan, err := c.Plan(scopedCtx(), "org1", "conv1", transcript())
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Facts) != 1 {
		t.Fatalf("plan has %d facts, want 1", len(plan.Facts))
	}
	if plan.Facts[0].Kind != KindCorrected {
		t.Errorf("kind = %q, want %q", plan.Facts[0].Kind, KindCorrected)
	}
	if plan.MessagesRead != 3 {
		t.Errorf("MessagesRead = %d, want 3", plan.MessagesRead)
	}

	// Nothing may have been written yet.
	stored, _ := store.List(context.Background(), "org1", "")
	if len(stored) != 0 {
		t.Errorf("Plan wrote %d facts; it must propose only -- a model rewriting "+
			"memory with no diff and no undo is not a safe default", len(stored))
	}
}

func TestApplyWritesTheFacts(t *testing.T) {
	store := NewInMemoryStore()
	c := New(&testutil.FakeLLM{DefaultResponse: planJSON}, store)

	plan, _ := c.Plan(scopedCtx(), "org1", "conv1", transcript())
	if err := c.Apply(context.Background(), "org1", plan); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	stored, _ := store.List(context.Background(), "org1", "")
	if len(stored) != 1 {
		t.Fatalf("stored %d facts, want 1", len(stored))
	}
	if !strings.Contains(stored[0].Statement, "Thursdays") {
		t.Errorf("stored statement = %q", stored[0].Statement)
	}
}

func TestPlanDoesNotTouchTheTranscript(t *testing.T) {
	store := NewInMemoryStore()
	c := New(&testutil.FakeLLM{DefaultResponse: planJSON}, store)

	mem := memory.NewConversationBuffer()
	ctx := scopedCtx()
	for _, m := range transcript() {
		if err := mem.AddMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	before, _ := mem.GetMessages(ctx)
	plan, _ := c.Plan(ctx, "org1", "conv1", before)
	_ = c.Apply(ctx, "org1", plan)
	after, _ := mem.GetMessages(ctx)

	if len(before) != len(after) {
		t.Errorf("the transcript changed from %d to %d messages; consolidation adds "+
			"a derived layer and must never edit the record of what was said",
			len(before), len(after))
	}
}

func TestLowConfidenceFactsAreSkipped(t *testing.T) {
	store := NewInMemoryStore()
	llm := &testutil.FakeLLM{DefaultResponse: `{"facts":[
	  {"subject":"guess","statement":"maybe true","kind":"new","confidence":0.2}
	]}`}
	c := New(llm, store, WithMinConfidence(0.6))

	plan, err := c.Plan(scopedCtx(), "org1", "conv1", transcript())
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Facts) != 0 {
		t.Errorf("a fact below the confidence threshold was kept: %+v", plan.Facts)
	}
	if len(plan.Skipped) == 0 {
		t.Error("a dropped fact should be recorded in Skipped, so an empty plan can " +
			"be told apart from a filtered one")
	}
}

func TestFactIDIsStableSoReRunsUpdate(t *testing.T) {
	a := FactID("deploy schedule", "Deploys happen on Thursdays")
	b := FactID("  Deploy Schedule  ", "deploys happen on thursdays")
	if a != b {
		t.Errorf("FactID is not stable across case and whitespace: %q vs %q", a, b)
	}
	if FactID("x", "one") == FactID("x", "two") {
		t.Error("different statements must get different IDs")
	}
}

func TestSupersedingRemovesTheOldFact(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	oldID := FactID("deploy schedule", "Deploys happen on Tuesdays")
	if err := store.Put(ctx, "org1", []Fact{{
		ID: oldID, Subject: "deploy schedule",
		Statement: "Deploys happen on Tuesdays", Kind: KindNew, Confidence: 0.9,
	}}); err != nil {
		t.Fatal(err)
	}

	if err := store.Put(ctx, "org1", []Fact{{
		ID:         FactID("deploy schedule", "Deploys happen on Thursdays"),
		Subject:    "deploy schedule",
		Statement:  "Deploys happen on Thursdays",
		Kind:       KindCorrected,
		Supersedes: []string{oldID},
	}}); err != nil {
		t.Fatal(err)
	}

	facts, _ := store.List(ctx, "org1", "")
	if len(facts) != 1 {
		t.Fatalf("store holds %d facts, want 1: a correction must remove what it "+
			"supersedes, not sit alongside it", len(facts))
	}
	if !strings.Contains(facts[0].Statement, "Thursdays") {
		t.Errorf("surviving fact = %q, want the correction", facts[0].Statement)
	}
}

func TestModelCodeFencesAreTolerated(t *testing.T) {
	store := NewInMemoryStore()
	llm := &testutil.FakeLLM{DefaultResponse: "```json\n" + planJSON + "\n```"}
	c := New(llm, store)

	plan, err := c.Plan(scopedCtx(), "org1", "conv1", transcript())
	if err != nil {
		t.Fatalf("a fenced JSON reply should be tolerated: %v", err)
	}
	if len(plan.Facts) != 1 {
		t.Errorf("plan has %d facts, want 1", len(plan.Facts))
	}
}

func TestEmptyPlanIsNotAnError(t *testing.T) {
	c := New(&testutil.FakeLLM{DefaultResponse: `{"facts":[]}`}, NewInMemoryStore())

	plan, err := c.Plan(scopedCtx(), "org1", "conv1", transcript())
	if err != nil {
		t.Fatalf("an empty plan is a normal outcome: %v", err)
	}
	if !plan.Empty() {
		t.Error("plan should report itself empty")
	}
	if !strings.Contains(plan.Diff(), "No facts") {
		t.Errorf("Diff() = %q, want it to say nothing was proposed", plan.Diff())
	}
}

func TestExistingFactsAreShownToTheModel(t *testing.T) {
	store := NewInMemoryStore()
	_ = store.Put(context.Background(), "org1", []Fact{{
		ID: "fact_existing", Subject: "tz", Statement: "User is in Oslo", Kind: KindNew,
	}})

	llm := &testutil.FakeLLM{DefaultResponse: `{"facts":[]}`}
	c := New(llm, store)

	if _, err := c.Plan(scopedCtx(), "org1", "conv1", transcript()); err != nil {
		t.Fatal(err)
	}

	prompts := llm.Prompts()
	if len(prompts) == 0 || !strings.Contains(prompts[0], "User is in Oslo") {
		t.Error("existing facts must be shown to the model, or every run re-proposes " +
			"the same statements")
	}
}

// TestActivityTrackerDoesNotBreakWrites pins the property that makes enabling
// consolidation safe: the tracker has no LLM, no goroutine and no failure mode
// of its own.
func TestActivityTrackerDoesNotBreakWrites(t *testing.T) {
	inner := memory.NewConversationBuffer()
	tracker := NewActivityTracker(inner)
	ctx := scopedCtx()

	for _, m := range transcript() {
		if err := tracker.AddMessage(ctx, m); err != nil {
			t.Fatalf("AddMessage() error = %v", err)
		}
	}

	msgs, err := tracker.GetMessages(ctx)
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("read back %d messages, want 3", len(msgs))
	}
}

func TestActivityTrackerPreservesCapabilities(t *testing.T) {
	tracker := NewActivityTracker(memory.NewConversationBuffer())
	if interfaces.UnwrapMemory(tracker) == interfaces.Memory(tracker) {
		t.Error("the tracker must unwrap to the memory it decorates, or it hides " +
			"that memory's optional capabilities")
	}
}

func TestIdleScopesRequireBothQuietAndVolume(t *testing.T) {
	tracker := NewActivityTracker(memory.NewConversationBuffer())
	ctx := scopedCtx()

	for i := 0; i < 5; i++ {
		_ = tracker.AddMessage(ctx, interfaces.Message{
			Role: interfaces.MessageRoleUser, Content: "hi",
		})
	}

	// Not yet idle.
	if got := tracker.IdleScopes(time.Hour, 1); len(got) != 0 {
		t.Errorf("a busy conversation was reported idle: %+v", got)
	}

	// Idle, but below the write threshold.
	if got := tracker.IdleScopes(0, 100); len(got) != 0 {
		t.Errorf("a conversation below the write threshold was reported: %+v", got)
	}

	// Both conditions met.
	idle := tracker.IdleScopes(0, 1)
	if len(idle) != 1 {
		t.Fatalf("IdleScopes returned %d, want 1", len(idle))
	}
	if idle[0].OrgID != "org1" || idle[0].ConversationID != "conv1" {
		t.Errorf("scope = %+v, want org1/conv1", idle[0])
	}
	if idle[0].Writes != 5 {
		t.Errorf("Writes = %d, want 5", idle[0].Writes)
	}
}

func TestMarkConsolidatedStopsReprocessing(t *testing.T) {
	tracker := NewActivityTracker(memory.NewConversationBuffer())
	ctx := scopedCtx()
	for i := 0; i < 5; i++ {
		_ = tracker.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "hi"})
	}

	idle := tracker.IdleScopes(0, 1)
	tracker.MarkConsolidated(idle[0].Key)

	if got := tracker.IdleScopes(0, 1); len(got) != 0 {
		t.Error("a consolidated scope was reported again with no new messages")
	}
}

func TestSchedulerRunOnceConsolidatesIdleConversations(t *testing.T) {
	store := NewInMemoryStore()
	tracker := NewActivityTracker(memory.NewConversationBuffer())
	c := New(&testutil.FakeLLM{DefaultResponse: planJSON}, store)

	ctx := scopedCtx()
	for _, m := range transcript() {
		_ = tracker.AddMessage(ctx, m)
	}

	var seen []Plan
	var mu sync.Mutex
	s := NewScheduler(tracker, c,
		WithIdleFor(0), WithMinWrites(1), WithAutoApply(),
		WithPlanCallback(func(_ IdleScope, p Plan) {
			mu.Lock()
			seen = append(seen, p)
			mu.Unlock()
		}),
	)

	s.RunOnce(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("the plan callback fired %d times, want 1", len(seen))
	}
	facts, _ := store.List(context.Background(), "org1", "")
	if len(facts) != 1 {
		t.Errorf("stored %d facts, want 1 with auto-apply on", len(facts))
	}
}

// TestSchedulerDoesNotApplyWithoutOptIn guards the default. A model rewriting an
// agent's memory unattended should be a choice, not a consequence of switching
// consolidation on.
func TestSchedulerDoesNotApplyWithoutOptIn(t *testing.T) {
	store := NewInMemoryStore()
	tracker := NewActivityTracker(memory.NewConversationBuffer())
	c := New(&testutil.FakeLLM{DefaultResponse: planJSON}, store)

	ctx := scopedCtx()
	for _, m := range transcript() {
		_ = tracker.AddMessage(ctx, m)
	}

	var got Plan
	var mu sync.Mutex
	s := NewScheduler(tracker, c, WithIdleFor(0), WithMinWrites(1),
		WithPlanCallback(func(_ IdleScope, p Plan) {
			mu.Lock()
			got = p
			mu.Unlock()
		}))

	s.RunOnce(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(got.Facts) == 0 {
		t.Error("a plan should still be produced for review")
	}
	facts, _ := store.List(context.Background(), "org1", "")
	if len(facts) != 0 {
		t.Errorf("%d facts were written without WithAutoApply", len(facts))
	}
}

func TestSchedulerStartStopLeavesNoGoroutine(t *testing.T) {
	tracker := NewActivityTracker(memory.NewConversationBuffer())
	c := New(&testutil.FakeLLM{DefaultResponse: `{"facts":[]}`}, NewInMemoryStore())

	s := NewScheduler(tracker, c, WithInterval(10*time.Millisecond), WithIdleFor(0), WithMinWrites(1))
	s.Start(context.Background())
	s.Start(context.Background()) // second Start must be a no-op
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() { s.Stop(); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return: the scheduler goroutine outlived it")
	}
}

func TestSchedulerStopsWithContext(t *testing.T) {
	tracker := NewActivityTracker(memory.NewConversationBuffer())
	c := New(&testutil.FakeLLM{DefaultResponse: `{"facts":[]}`}, NewInMemoryStore())

	ctx, cancel := context.WithCancel(context.Background())
	s := NewScheduler(tracker, c, WithInterval(10*time.Millisecond))
	s.Start(ctx)
	cancel()
	time.Sleep(50 * time.Millisecond)
	s.Stop() // must not hang
}

func TestRecallReturnsStoredFacts(t *testing.T) {
	store := NewInMemoryStore()
	c := New(&testutil.FakeLLM{DefaultResponse: planJSON}, store)

	plan, _ := c.Plan(scopedCtx(), "org1", "conv1", transcript())
	_ = c.Apply(context.Background(), "org1", plan)

	facts, err := c.Recall(context.Background(), "org1", "deploy schedule")
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if len(facts) != 1 {
		t.Errorf("Recall returned %d facts, want 1", len(facts))
	}
}
