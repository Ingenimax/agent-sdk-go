package guardrails

import (
	"context"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// PipelineSatisfiesTheAgentContract is the guard that matters most in this
// package: it is what makes every other guardrail here reachable.
//
// agent.WithGuardrails takes an interfaces.Guardrails (ProcessInput /
// ProcessOutput). Pipeline exposed only ProcessRequest / ProcessResponse, and
// nothing in the module implemented the interface -- so the SDK's guardrails
// package could be constructed but never attached to an agent.
var _ interfaces.Guardrails = (*Pipeline)(nil)

func newTestPipeline(g ...Guardrail) *Pipeline {
	return NewPipeline(g, logging.New())
}

// --- ContentFilter -------------------------------------------------------

// TestContentFilterWithNoWordsRedactsNothing guards a destructive default.
//
// The constructor joined the word list with "|" and interpolated it raw, so an
// empty list produced `\b()\b` -- an empty alternative that matches at every
// word boundary. Filtering with no blocked words replaced the entire text with
// asterisks instead of leaving it alone.
func TestContentFilterWithNoWordsRedactsNothing(t *testing.T) {
	for _, words := range [][]string{nil, {}, {""}} {
		filter := NewContentFilter(words, RedactAction)

		triggered, modified, err := filter.CheckRequest(context.Background(), "the quick brown fox")
		if err != nil {
			t.Fatalf("CheckRequest() error = %v", err)
		}
		if triggered {
			t.Errorf("words=%q: an empty blocked-word list must not trigger", words)
		}
		if modified != "the quick brown fox" {
			t.Errorf("words=%q: text was rewritten to %q; an empty list must leave it alone",
				words, modified)
		}
	}
}

// TestContentFilterTreatsBlockedWordsLiterally guards a construction-time panic.
//
// The comment in the constructor said "Escape special characters" but no
// escaping happened, so any blocked word containing regex metacharacters
// panicked inside regexp.MustCompile -- taking down the process at startup for
// an ordinary config value like "c++".
func TestContentFilterTreatsBlockedWordsLiterally(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewContentFilter panicked on a word with regex metacharacters: %v", r)
		}
	}()

	filter := NewContentFilter([]string{"c++", "a.b", "(secret)"}, RedactAction)

	triggered, modified, err := filter.CheckRequest(context.Background(), "I know c++ well")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if !triggered {
		t.Fatal("a literal blocked word was not matched")
	}
	if strings.Contains(modified, "c++") {
		t.Errorf("modified = %q, want the blocked word redacted", modified)
	}

	// "a.b" must match literally, not as "a<any char>b".
	triggered, _, err = filter.CheckRequest(context.Background(), "axb")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if triggered {
		t.Error(`"a.b" matched "axb": the dot was treated as a wildcard rather than a literal`)
	}
}

func TestContentFilterMatchesWholeWordsCaseInsensitively(t *testing.T) {
	filter := NewContentFilter([]string{"badword"}, RedactAction)
	ctx := context.Background()

	triggered, modified, err := filter.CheckRequest(ctx, "This is a BadWord here")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if !triggered {
		t.Fatal("matching should be case-insensitive")
	}
	if !strings.Contains(modified, "****") {
		t.Errorf("modified = %q, want the match replaced", modified)
	}

	// Substring of a longer word must not match.
	triggered, _, err = filter.CheckRequest(ctx, "badwordsmith")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if triggered {
		t.Error("matched inside a longer word; the filter is word-boundary anchored")
	}
}

// --- Pipeline actions ----------------------------------------------------

func TestPipelineBlockActionRejectsTheInput(t *testing.T) {
	p := newTestPipeline(NewContentFilter([]string{"forbidden"}, BlockAction))

	_, err := p.ProcessInput(context.Background(), "this is forbidden")
	if err == nil {
		t.Fatal("ProcessInput() returned no error for a blocking guardrail")
	}
	if !strings.Contains(err.Error(), string(ContentFilterGuardrail)) {
		t.Errorf("error = %v, want it to name the guardrail that blocked", err)
	}
}

func TestPipelineRedactActionRewritesTheInput(t *testing.T) {
	p := newTestPipeline(NewContentFilter([]string{"secret"}, RedactAction))

	got, err := p.ProcessInput(context.Background(), "the secret is out")
	if err != nil {
		t.Fatalf("ProcessInput() error = %v", err)
	}
	if strings.Contains(got, "secret") {
		t.Errorf("ProcessInput() = %q, want the blocked word redacted", got)
	}
}

// TestPipelineWarnActionPassesContentThrough pins that warn is observe-only.
// A caller choosing WarnAction over RedactAction is explicitly asking for the
// original text to reach the model.
func TestPipelineWarnActionPassesContentThrough(t *testing.T) {
	p := newTestPipeline(NewContentFilter([]string{"secret"}, WarnAction))

	got, err := p.ProcessInput(context.Background(), "the secret is out")
	if err != nil {
		t.Fatalf("ProcessInput() error = %v", err)
	}
	if got != "the secret is out" {
		t.Errorf("ProcessInput() = %q, want the input unchanged under WarnAction", got)
	}
}

// TestPipelineAppliesGuardrailsInOrder asserts each guardrail sees the previous
// one's output, so redactions compose instead of overwriting each other.
func TestPipelineAppliesGuardrailsInOrder(t *testing.T) {
	p := newTestPipeline(
		NewContentFilter([]string{"alpha"}, RedactAction),
		NewContentFilter([]string{"beta"}, RedactAction),
	)

	got, err := p.ProcessInput(context.Background(), "alpha and beta")
	if err != nil {
		t.Fatalf("ProcessInput() error = %v", err)
	}
	if strings.Contains(got, "alpha") || strings.Contains(got, "beta") {
		t.Errorf("ProcessInput() = %q, want both guardrails applied", got)
	}
}

func TestPipelineProcessOutputGuardsResponses(t *testing.T) {
	p := newTestPipeline(NewContentFilter([]string{"leaked"}, RedactAction))

	got, err := p.ProcessOutput(context.Background(), "here is the leaked value")
	if err != nil {
		t.Fatalf("ProcessOutput() error = %v", err)
	}
	if strings.Contains(got, "leaked") {
		t.Errorf("ProcessOutput() = %q, want the response redacted", got)
	}
}

func TestAddGuardrailAffectsSubsequentCalls(t *testing.T) {
	p := newTestPipeline()
	ctx := context.Background()

	got, err := p.ProcessInput(ctx, "nothing blocked here")
	if err != nil {
		t.Fatalf("ProcessInput() error = %v", err)
	}
	if got != "nothing blocked here" {
		t.Errorf("an empty pipeline rewrote its input to %q", got)
	}

	p.AddGuardrail(NewContentFilter([]string{"blocked"}, BlockAction))
	if _, err := p.ProcessInput(ctx, "nothing blocked here"); err == nil {
		t.Error("the guardrail added by AddGuardrail did not run")
	}
}

// --- PiiFilter -----------------------------------------------------------

func TestPiiFilterRedactsEachSupportedKind(t *testing.T) {
	filter := NewPiiFilter(RedactAction)
	ctx := context.Background()

	for _, tc := range []struct{ name, input, leak string }{
		{"email", "write to alice@example.com today", "alice@example.com"},
		{"ssn", "ssn 123-45-6789 on file", "123-45-6789"},
		{"credit_card", "card 4111-1111-1111-1111 expires soon", "4111-1111-1111-1111"},
		{"ip_address", "connect to 192.168.1.10 now", "192.168.1.10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			triggered, modified, err := filter.CheckRequest(ctx, tc.input)
			if err != nil {
				t.Fatalf("CheckRequest() error = %v", err)
			}
			if !triggered {
				t.Fatalf("%s was not detected in %q", tc.name, tc.input)
			}
			if strings.Contains(modified, tc.leak) {
				t.Errorf("modified = %q, still contains the %s", modified, tc.name)
			}
		})
	}
}

func TestPiiFilterLeavesCleanTextAlone(t *testing.T) {
	filter := NewPiiFilter(RedactAction)

	triggered, modified, err := filter.CheckRequest(context.Background(), "what is the weather today")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if triggered {
		t.Error("clean text triggered the PII filter")
	}
	if modified != "what is the weather today" {
		t.Errorf("modified = %q, want the input untouched", modified)
	}
}

// TestPiiFilterIsDeterministic guards against the map-ordering trap. The
// patterns live in a map, and Go randomises map iteration, so if two patterns
// ever overlap the redacted output would differ run to run.
func TestPiiFilterIsDeterministic(t *testing.T) {
	filter := NewPiiFilter(RedactAction)
	input := "mail alice@example.com, ssn 123-45-6789, card 4111-1111-1111-1111, host 10.0.0.1"

	_, first, err := filter.CheckRequest(context.Background(), input)
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	for i := 0; i < 50; i++ {
		_, got, err := filter.CheckRequest(context.Background(), input)
		if err != nil {
			t.Fatalf("CheckRequest() error = %v", err)
		}
		if got != first {
			t.Fatalf("redaction is not deterministic:\n  %q\n  %q", first, got)
		}
	}
}

// --- TokenLimit ----------------------------------------------------------

func TestTokenLimitTruncatesFromTheEndByDefault(t *testing.T) {
	limit := NewTokenLimit(3, nil, RedactAction, "")

	triggered, modified, err := limit.CheckRequest(context.Background(), "one two three four five")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if !triggered {
		t.Fatal("a request over the token limit did not trigger")
	}
	if !strings.HasPrefix(modified, "one two three") {
		t.Errorf("modified = %q, want the first 3 tokens kept", modified)
	}
}

func TestTokenLimitStartModeKeepsTheTail(t *testing.T) {
	limit := NewTokenLimit(2, nil, RedactAction, "start")

	_, modified, err := limit.CheckRequest(context.Background(), "one two three four")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if modified != "three four" {
		t.Errorf("modified = %q, want the last 2 tokens", modified)
	}
}

func TestTokenLimitLeavesShortTextAlone(t *testing.T) {
	limit := NewTokenLimit(10, nil, RedactAction, "end")

	triggered, modified, err := limit.CheckRequest(context.Background(), "short enough")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if triggered {
		t.Error("text under the limit triggered the guardrail")
	}
	if modified != "short enough" {
		t.Errorf("modified = %q, want the input untouched", modified)
	}
}

// --- RateLimit -----------------------------------------------------------

func TestRateLimitTriggersAfterTheConfiguredCount(t *testing.T) {
	limit := NewRateLimit(2, BlockAction)
	ctx := multitenancy.WithOrgID(context.Background(), "org-1")

	for i := 0; i < 2; i++ {
		triggered, _, err := limit.CheckRequest(ctx, "hello")
		if err != nil {
			t.Fatalf("CheckRequest() error = %v", err)
		}
		if triggered {
			t.Fatalf("request %d triggered the limit early", i+1)
		}
	}

	triggered, _, err := limit.CheckRequest(ctx, "hello")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if !triggered {
		t.Error("the request past the limit did not trigger")
	}
}

// TestRateLimitCountsPerOrganization asserts one tenant cannot exhaust another
// tenant's budget.
func TestRateLimitCountsPerOrganization(t *testing.T) {
	limit := NewRateLimit(1, BlockAction)

	first := multitenancy.WithOrgID(context.Background(), "org-1")
	second := multitenancy.WithOrgID(context.Background(), "org-2")

	if _, _, err := limit.CheckRequest(first, "hello"); err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if triggered, _, err := limit.CheckRequest(first, "hello"); err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	} else if !triggered {
		t.Fatal("org-1 was not rate limited after exceeding its budget")
	}

	triggered, _, err := limit.CheckRequest(second, "hello")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if triggered {
		t.Error("org-2 was rate limited by org-1's traffic: counts must be per tenant")
	}
}

// TestRateLimitSubstitutesADiagnosticUnderRedactAction documents a sharp edge
// rather than a fix.
//
// CheckRequest returns its "rate limit exceeded" message in the modified-text
// slot. Under RedactAction the pipeline treats that slot as replacement content,
// so the user's prompt is swapped for the diagnostic and sent to the model.
// RateLimit is only meaningful with BlockAction.
func TestRateLimitSubstitutesADiagnosticUnderRedactAction(t *testing.T) {
	p := newTestPipeline(NewRateLimit(0, RedactAction))

	got, err := p.ProcessInput(multitenancy.WithOrgID(context.Background(), "org-1"), "real user prompt")
	if err != nil {
		t.Fatalf("ProcessInput() error = %v", err)
	}
	if !strings.Contains(got, "Rate limit exceeded") {
		t.Skip("RateLimit no longer substitutes a diagnostic; this trap is gone")
	}
	t.Logf("known sharp edge: RedactAction replaced the prompt with %q -- use BlockAction", got)
}

// --- ToolRestriction -----------------------------------------------------

// TestToolRestrictionOnlyMatchesProse pins what this guardrail actually does,
// because its name suggests far more than it delivers: it scans text for the
// literal phrase "use tool <name>". It does not and cannot prevent a model from
// invoking a tool through the provider's tool-calling API.
func TestToolRestrictionOnlyMatchesProse(t *testing.T) {
	restriction := NewToolRestriction([]string{"search"}, RedactAction)
	ctx := context.Background()

	triggered, modified, err := restriction.CheckRequest(ctx, "please use tool delete_everything now")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if !triggered {
		t.Fatal("a disallowed tool named in prose was not caught")
	}
	if !strings.Contains(modified, "RESTRICTED TOOL") {
		t.Errorf("modified = %q, want the restriction marker", modified)
	}

	// An allowed tool passes.
	triggered, _, err = restriction.CheckRequest(ctx, "please use tool search")
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if triggered {
		t.Error("an allowed tool was restricted")
	}

	// A real tool call does not go through this guardrail at all.
	triggered, _, err = restriction.CheckRequest(ctx, `{"tool_calls":[{"name":"delete_everything"}]}`)
	if err != nil {
		t.Fatalf("CheckRequest() error = %v", err)
	}
	if triggered {
		t.Error("unexpectedly matched a structured tool call; update the doc comment")
	}
}

func TestToolRestrictionIgnoresResponses(t *testing.T) {
	restriction := NewToolRestriction([]string{"search"}, BlockAction)

	triggered, _, err := restriction.CheckResponse(context.Background(), "use tool delete_everything")
	if err != nil {
		t.Fatalf("CheckResponse() error = %v", err)
	}
	if triggered {
		t.Error("tool restrictions are request-side only")
	}
}
