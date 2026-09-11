package hooks

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func decorate(t *testing.T, r *Registry, tool interfaces.Tool) interfaces.Tool {
	t.Helper()
	out := r.Decorate("test-agent")([]interfaces.Tool{tool})
	if len(out) != 1 {
		t.Fatalf("Decorate returned %d tools, want 1", len(out))
	}
	return out[0]
}

func TestBeforeToolCanDeny(t *testing.T) {
	inner := &testutil.FakeTool{ToolName: "deploy", Result: "deployed"}
	r := NewRegistry(Plugin{
		Name: "guard",
		BeforeTool: func(context.Context, ToolCall) (Outcome, error) {
			return Outcome{Decision: Deny, Message: "not in production"}, nil
		},
	})

	out, err := decorate(t, r, inner).Execute(context.Background(), "{}")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out != "not in production" {
		t.Errorf("result = %q, want the denial message", out)
	}
	if inner.CallCount() != 0 {
		t.Error("a denied tool was still executed")
	}
}

// TestDenialIsAResultNotAnError pins a deliberate choice. The providers convert
// a tool error into a tool-result message and continue the loop, so returning an
// error would not stop anything -- it would just look like a broken tool.
func TestDenialIsAResultNotAnError(t *testing.T) {
	r := NewRegistry(Plugin{
		Name: "guard",
		BeforeTool: func(context.Context, ToolCall) (Outcome, error) {
			return Outcome{Decision: Deny}, nil
		},
	})

	out, err := decorate(t, r, &testutil.FakeTool{ToolName: "x"}).Execute(context.Background(), "{}")
	if err != nil {
		t.Errorf("a denial should not surface as an error: %v", err)
	}
	if !strings.Contains(out, "guard") {
		t.Errorf("default denial message %q should name the plugin that denied", out)
	}
}

// TestZeroOutcomeAllows guards the most dangerous default in the package. A hook
// that returns early, or one written before a field existed, must not block
// every tool call.
func TestZeroOutcomeAllows(t *testing.T) {
	inner := &testutil.FakeTool{ToolName: "x", Result: "ran"}
	r := NewRegistry(Plugin{
		Name: "noop",
		BeforeTool: func(context.Context, ToolCall) (Outcome, error) {
			return Outcome{}, nil // the zero value
		},
	})

	out, err := decorate(t, r, inner).Execute(context.Background(), "{}")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out != "ran" {
		t.Errorf("result = %q, want the tool to have run: the zero Outcome must allow", out)
	}
}

func TestBeforeToolCanModifyArguments(t *testing.T) {
	inner := &testutil.FakeTool{ToolName: "search", Result: "ok"}
	r := NewRegistry(Plugin{
		Name: "rewriter",
		BeforeTool: func(_ context.Context, call ToolCall) (Outcome, error) {
			return Outcome{Decision: Modify, Arguments: `{"q":"rewritten"}`}, nil
		},
	})

	if _, err := decorate(t, r, inner).Execute(context.Background(), `{"q":"original"}`); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	calls := inner.Calls()
	if len(calls) != 1 || calls[0] != `{"q":"rewritten"}` {
		t.Errorf("tool received %v, want the rewritten arguments", calls)
	}
}

func TestModificationsAccumulateAcrossPlugins(t *testing.T) {
	inner := &testutil.FakeTool{ToolName: "x", Result: "ok"}
	r := NewRegistry(
		Plugin{Name: "first", BeforeTool: func(_ context.Context, c ToolCall) (Outcome, error) {
			return Outcome{Decision: Modify, Arguments: c.Arguments + "+first"}, nil
		}},
		Plugin{Name: "second", BeforeTool: func(_ context.Context, c ToolCall) (Outcome, error) {
			return Outcome{Decision: Modify, Arguments: c.Arguments + "+second"}, nil
		}},
	)

	if _, err := decorate(t, r, inner).Execute(context.Background(), "base"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := inner.Calls()[0]; got != "base+first+second" {
		t.Errorf("tool received %q, want each plugin to see the previous one's edit", got)
	}
}

func TestFirstDenialShortCircuits(t *testing.T) {
	var secondRan atomic.Bool
	inner := &testutil.FakeTool{ToolName: "x"}

	r := NewRegistry(
		Plugin{Name: "denier", BeforeTool: func(context.Context, ToolCall) (Outcome, error) {
			return Outcome{Decision: Deny, Message: "no"}, nil
		}},
		Plugin{Name: "later", BeforeTool: func(context.Context, ToolCall) (Outcome, error) {
			secondRan.Store(true)
			return Outcome{}, nil
		}},
	)

	if _, err := decorate(t, r, inner).Execute(context.Background(), "{}"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if secondRan.Load() {
		t.Error("a plugin ran after the call was already denied")
	}
}

func TestAfterToolCanRewriteResults(t *testing.T) {
	inner := &testutil.FakeTool{ToolName: "x", Result: "the key is sk-abcdefghijklmnopqrstuvwxyz123"}
	r := NewRegistry(Redact("[REDACTED]", CommonSecretPatterns()...))

	out, err := decorate(t, r, inner).Execute(context.Background(), "{}")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(out, "sk-abcdefghijklmnopqrstuvwxyz123") {
		t.Errorf("result %q still contains the secret", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("result %q should show the redaction", out)
	}
}

// TestPanicInAHookDoesNotKillTheRun guards containment. A logging plugin that
// dereferences a nil map should produce a failed tool call, not a dead agent
// with no explanation.
func TestPanicInAHookDoesNotKillTheRun(t *testing.T) {
	inner := &testutil.FakeTool{ToolName: "x", Result: "ran"}
	r := NewRegistry(Plugin{
		Name: "buggy",
		BeforeTool: func(context.Context, ToolCall) (Outcome, error) {
			var m map[string]string
			m["boom"] = "panics" //nolint:staticcheck // deliberate: assignment to a nil map
			return Outcome{}, nil
		},
	})

	_, err := decorate(t, r, inner).Execute(context.Background(), "{}")
	if err == nil {
		t.Fatal("a panicking hook should surface as an error, not be swallowed")
	}
	if !strings.Contains(err.Error(), "buggy") {
		t.Errorf("error %v should name the plugin that panicked", err)
	}
}

// TestPanicInAfterToolDoesNotFailTheCall asserts the asymmetry is deliberate:
// an AfterTool hook is observational, so a crash in metrics must not discard a
// tool result that was produced successfully.
func TestPanicInAfterToolDoesNotFailTheCall(t *testing.T) {
	inner := &testutil.FakeTool{ToolName: "x", Result: "real result"}
	r := NewRegistry(Plugin{
		Name: "buggy-metrics",
		AfterTool: func(context.Context, ToolCall, string) (string, error) {
			var m map[string]string
			m["boom"] = "panics" //nolint:staticcheck // deliberate
			return "", nil
		},
	})

	out, err := decorate(t, r, inner).Execute(context.Background(), "{}")
	if err != nil {
		t.Fatalf("an observational hook must not fail the call: %v", err)
	}
	if out != "real result" {
		t.Errorf("result = %q, want the tool's real result preserved", out)
	}
}

func TestOnToolErrorCanSubstituteAResult(t *testing.T) {
	inner := &testutil.FakeTool{ToolName: "flaky", Err: errors.New("upstream down")}
	r := NewRegistry(RetryOnError())

	out, err := decorate(t, r, inner).Execute(context.Background(), "{}")
	if err != nil {
		t.Fatalf("the error should have been handled: %v", err)
	}
	if !strings.Contains(out, "flaky") || !strings.Contains(out, "upstream down") {
		t.Errorf("substituted result %q should describe the failure to the model", out)
	}
}

func TestUnhandledToolErrorPropagates(t *testing.T) {
	sentinel := errors.New("upstream down")
	inner := &testutil.FakeTool{ToolName: "flaky", Err: sentinel}
	r := NewRegistry(Logging(nil)) // observes but does not handle

	if _, err := decorate(t, r, inner).Execute(context.Background(), "{}"); !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want the original error to propagate when unhandled", err)
	}
}

func TestAllowListDeniesUnlistedTools(t *testing.T) {
	blocked := &testutil.FakeTool{ToolName: "delete_everything", Result: "boom"}
	r := NewRegistry(AllowList("search", "read"))

	out, err := decorate(t, r, blocked).Execute(context.Background(), "{}")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if blocked.CallCount() != 0 {
		t.Error("an unlisted tool was executed")
	}
	if !strings.Contains(out, "delete_everything") {
		t.Errorf("denial %q should name the tool", out)
	}
}

func TestTruncateAnnouncesWhatItDropped(t *testing.T) {
	long := strings.Repeat("x", 500)
	inner := &testutil.FakeTool{ToolName: "dump", Result: long}
	r := NewRegistry(TruncateResults(100))

	out, err := decorate(t, r, inner).Execute(context.Background(), "{}")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(out) >= len(long) {
		t.Error("the result was not truncated")
	}
	if !strings.Contains(out, "truncated") {
		t.Error("truncation must be announced: a silent cut looks to the model " +
			"like the data simply ended")
	}
}

func TestMetricsCountCallsAndFailures(t *testing.T) {
	m := NewMetrics()
	good := &testutil.FakeTool{ToolName: "good", Result: "ok"}
	bad := &testutil.FakeTool{ToolName: "bad", Err: errors.New("nope")}

	r := NewRegistry(m.Plugin(), RetryOnError())

	_, _ = decorate(t, r, good).Execute(context.Background(), "{}")
	_, _ = decorate(t, r, good).Execute(context.Background(), "{}")
	_, _ = decorate(t, r, bad).Execute(context.Background(), "{}")

	if got := m.Calls()["good"]; got != 2 {
		t.Errorf("good calls = %d, want 2", got)
	}
	if got := m.Failures()["bad"]; got != 1 {
		t.Errorf("bad failures = %d, want 1", got)
	}
}

func TestHookedToolForwardsOptionalInterfacesAndUnwraps(t *testing.T) {
	inner := &testutil.FakeTool{ToolName: "raw", Display: "Pretty", IsInternal: true}
	wrapped := decorate(t, NewRegistry(Logging(nil)), inner)

	named, ok := wrapped.(interfaces.ToolWithDisplayName)
	if !ok || named.DisplayName() != "Pretty" {
		t.Error("DisplayName was lost through the hook decorator")
	}
	internal, ok := wrapped.(interfaces.InternalTool)
	if !ok || !internal.Internal() {
		t.Error("Internal was lost through the hook decorator")
	}
	unwrapper, ok := wrapped.(interface{ Unwrap() interfaces.Tool })
	if !ok || unwrapper.Unwrap() != interfaces.Tool(inner) {
		t.Error("the hooked tool does not unwrap to the original")
	}
}

func TestRegistryDescribesItsPlugins(t *testing.T) {
	r := NewRegistry(Logging(nil), AllowList("x"))
	got := r.Describe()
	if !strings.Contains(got, "logging") || !strings.Contains(got, "allowlist") {
		t.Errorf("Describe() = %q, want it to name the registered plugins", got)
	}
	if NewRegistry().Describe() == "" {
		t.Error("Describe() on an empty registry should still say something")
	}
}

var _ = regexp.MustCompile
