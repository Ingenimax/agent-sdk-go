package consolidation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Consolidator turns a transcript into a Plan.
type Consolidator struct {
	llm   interfaces.LLM
	store Store

	maxMessages   int
	minConfidence float64
}

// Option configures a Consolidator.
type Option func(*Consolidator)

// WithMaxMessages bounds how many recent messages are examined. Defaults to 100.
//
// A bound matters: consolidation is one LLM call over a whole conversation, and
// an unbounded one grows without limit in both cost and latency.
func WithMaxMessages(n int) Option {
	return func(c *Consolidator) {
		if n > 0 {
			c.maxMessages = n
		}
	}
}

// WithMinConfidence discards proposed facts below a confidence threshold.
// Defaults to 0.6.
func WithMinConfidence(min float64) Option {
	return func(c *Consolidator) {
		if min >= 0 && min <= 1 {
			c.minConfidence = min
		}
	}
}

// New creates a consolidator.
func New(llm interfaces.LLM, store Store, options ...Option) *Consolidator {
	c := &Consolidator{
		llm:           llm,
		store:         store,
		maxMessages:   100,
		minConfidence: 0.6,
	}
	for _, option := range options {
		option(c)
	}
	return c
}

// Plan reads a conversation and proposes durable facts, writing nothing.
//
// Existing facts for the org are supplied to the model so it can merge,
// supersede, or decline to repeat them -- without that context every run would
// re-propose the same statements.
func (c *Consolidator) Plan(ctx context.Context, orgID, conversationID string, messages []interfaces.Message) (Plan, error) {
	if len(messages) == 0 {
		return Plan{}, nil
	}

	if len(messages) > c.maxMessages {
		messages = messages[len(messages)-c.maxMessages:]
	}

	existing, err := c.store.List(ctx, orgID, "")
	if err != nil {
		return Plan{}, fmt.Errorf("consolidation: reading existing facts: %w", err)
	}

	prompt := buildPrompt(messages, existing)

	response, err := c.llm.Generate(ctx, prompt)
	if err != nil {
		return Plan{}, fmt.Errorf("consolidation: %w", err)
	}

	plan, err := parsePlan(response)
	if err != nil {
		return Plan{}, err
	}

	plan.MessagesRead = len(messages)

	// Stamp and filter. A fact below the confidence threshold is dropped here
	// rather than written and second-guessed later.
	kept := plan.Facts[:0]
	for _, f := range plan.Facts {
		if f.Confidence < c.minConfidence {
			plan.Skipped = append(plan.Skipped,
				fmt.Sprintf("%s (confidence %.2f below threshold %.2f)", f.Statement, f.Confidence, c.minConfidence))
			continue
		}
		if strings.TrimSpace(f.Statement) == "" {
			continue
		}
		f.ID = FactID(f.Subject, f.Statement)
		f.ConversationID = conversationID
		f.CreatedAt = time.Now()
		if f.Kind == "" {
			f.Kind = KindNew
		}
		kept = append(kept, f)
	}
	plan.Facts = kept

	return plan, nil
}

// Apply writes a plan's facts to the store.
//
// Separate from Plan on purpose: a caller reviews a Diff and decides. Memory is
// the record of what was said, and a model rewriting it with no diff, no
// provenance and no undo is not a default worth having.
func (c *Consolidator) Apply(ctx context.Context, orgID string, plan Plan) error {
	if plan.Empty() {
		return nil
	}
	return c.store.Put(ctx, orgID, plan.Facts)
}

// Consolidate plans and applies in one step, for callers who have already
// decided to trust it.
func (c *Consolidator) Consolidate(ctx context.Context, orgID, conversationID string, messages []interfaces.Message) (Plan, error) {
	plan, err := c.Plan(ctx, orgID, conversationID, messages)
	if err != nil {
		return Plan{}, err
	}
	if err := c.Apply(ctx, orgID, plan); err != nil {
		return plan, err
	}
	return plan, nil
}

// Recall returns consolidated facts, ready to include in a prompt.
func (c *Consolidator) Recall(ctx context.Context, orgID, subject string) ([]Fact, error) {
	return c.store.List(ctx, orgID, subject)
}

// FactID derives a stable ID from a fact's subject and statement, so re-running
// consolidation over the same conversation updates a fact rather than
// accumulating near-duplicates of it.
func FactID(subject, statement string) string {
	normalized := strings.ToLower(strings.TrimSpace(subject)) + "|" +
		strings.ToLower(strings.TrimSpace(statement))
	sum := sha256.Sum256([]byte(normalized))
	return "fact_" + hex.EncodeToString(sum[:8])
}

func buildPrompt(messages []interfaces.Message, existing []Fact) string {
	var b strings.Builder

	b.WriteString(`You are consolidating an assistant's memory of a conversation.

Read the transcript and extract durable facts worth remembering across future
conversations. A durable fact is a stable preference, decision, constraint or
piece of context -- not a passing detail of this exchange.

For each fact decide a kind:
  new          something not already recorded
  merged       several statements of the same thing, combined
  corrected    a later statement contradicts an earlier recorded fact
  generalized  a specific episode whose general form is the useful part

Rules:
- Do not restate an existing fact unchanged.
- If a fact contradicts an existing one, use "corrected" and list the superseded
  fact's id in "supersedes".
- Quote the source text in "evidence".
- Give an honest confidence between 0 and 1. Low confidence is fine and useful;
  inflated confidence is not.
- If nothing is worth keeping, return an empty facts array. That is a valid and
  common answer.

Reply with JSON only, in this shape:
{"facts":[{"subject":"...","statement":"...","kind":"new","confidence":0.9,
"supersedes":["fact_..."],"evidence":"..."}],"skipped":["..."]}
`)

	if len(existing) > 0 {
		b.WriteString("\nAlready recorded facts:\n")
		for _, f := range existing {
			fmt.Fprintf(&b, "  %s [%s] %s: %s\n", f.ID, f.Kind, f.Subject, f.Statement)
		}
	}

	b.WriteString("\nTranscript:\n")
	for _, m := range messages {
		content := m.Content
		if content == "" && len(m.ToolCalls) > 0 {
			names := make([]string, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				names = append(names, tc.Name)
			}
			content = "(called tools: " + strings.Join(names, ", ") + ")"
		}
		if content == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", m.Role, content)
	}

	return b.String()
}

// parsePlan reads the model's JSON reply, tolerating the code fences models
// commonly add despite being asked for raw JSON.
func parsePlan(response string) (Plan, error) {
	cleaned := strings.TrimSpace(response)
	if idx := strings.Index(cleaned, "```"); idx >= 0 {
		cleaned = cleaned[idx+3:]
		cleaned = strings.TrimPrefix(cleaned, "json")
		if end := strings.Index(cleaned, "```"); end >= 0 {
			cleaned = cleaned[:end]
		}
	}
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return Plan{}, nil
	}

	var payload struct {
		Facts   []Fact   `json:"facts"`
		Skipped []string `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(cleaned), &payload); err != nil {
		return Plan{}, fmt.Errorf("consolidation: model did not return usable JSON: %w", err)
	}

	return Plan{Facts: payload.Facts, Skipped: payload.Skipped}, nil
}
