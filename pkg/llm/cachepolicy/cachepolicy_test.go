package cachepolicy

import (
	"strings"
	"testing"
)

// TestControlAndReportingAreSeparate is the reason this package exists rather
// than a "supports caching" boolean.
//
// OpenAI offers no control at all yet reports cache reads; Anthropic offers
// control AND reporting; Gemini does neither in this client. A single boolean
// would misrepresent at least one of them, and a caller would configure caching
// that silently does nothing.
func TestControlAndReportingAreSeparate(t *testing.T) {
	anthropic := For("anthropic")
	if anthropic.Control != Explicit {
		t.Errorf("anthropic control = %v, want Explicit", anthropic.Control)
	}
	if !anthropic.Honors() {
		t.Error("anthropic should honor an explicit CacheConfig")
	}

	openai := For("openai")
	if openai.Control != Automatic {
		t.Errorf("openai control = %v, want Automatic", openai.Control)
	}
	if openai.Honors() {
		t.Error("openai must not claim to honor CacheConfig; it has no control surface")
	}
	if !openai.ReportsUsage {
		t.Error("openai does report cached tokens, and that is the only evidence a " +
			"caller has that caching happened")
	}
}

func TestUnknownProviderAssumesNoCaching(t *testing.T) {
	c := For("some-new-provider")
	if c.Control != None {
		t.Errorf("control = %v, want None for an unknown provider", c.Control)
	}
	if c.Honors() {
		t.Error("an unknown provider must not be assumed to honor caching: expecting " +
			"savings that never arrive is the worse failure")
	}
}

func TestProviderLookupIsCaseAndSpaceInsensitive(t *testing.T) {
	if !For("  Anthropic  ").Honors() {
		t.Error("provider lookup should tolerate case and surrounding whitespace")
	}
}

// TestCheckConfigWarnsOnInertConfiguration is the practical entry point: catch
// a setting that does nothing at startup, rather than from a flat bill.
func TestCheckConfigWarnsOnInertConfiguration(t *testing.T) {
	if err := CheckConfig("anthropic", true); err != nil {
		t.Errorf("anthropic honors caching, so no warning is expected: %v", err)
	}

	err := CheckConfig("gemini", true)
	if err == nil {
		t.Fatal("configuring caching on a provider that ignores it should be reported")
	}
	if !strings.Contains(err.Error(), "gemini") {
		t.Errorf("error = %v, want it to name the provider", err)
	}

	if err := CheckConfig("gemini", false); err != nil {
		t.Errorf("no caching requested, so no warning is expected: %v", err)
	}
}

func TestExplainCoversEveryProvider(t *testing.T) {
	explanation := Explain()
	for _, provider := range Providers() {
		if !strings.Contains(explanation, provider) {
			t.Errorf("Explain() omits %q", provider)
		}
	}
}

func TestAnthropicNotesTheExtendedTTLLimitation(t *testing.T) {
	notes := For("anthropic").Notes
	if !strings.Contains(notes, "1h") {
		t.Error("the anthropic notes should record that extended TTL is unsupported, " +
			"since that was a silent billing surprise")
	}
}

func TestSupportStringsAreStable(t *testing.T) {
	for support, want := range map[Support]string{
		None:      "none",
		Automatic: "automatic",
		Explicit:  "explicit",
	} {
		if got := support.String(); got != want {
			t.Errorf("Support(%d).String() = %q, want %q", support, got, want)
		}
	}
}
