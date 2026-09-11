// Package cachepolicy describes what prompt caching a provider actually
// supports, so callers are not misled about whether caching happened.
//
// Providers differ fundamentally, and an abstraction that hides that does more
// harm than good:
//
//   - Anthropic caches on explicit cache_control breakpoints the caller places.
//   - OpenAI and Azure OpenAI cache automatically, with no control surface, but
//     do report how many prompt tokens were served from cache.
//   - Gemini has both implicit caching and a separate explicit cached-content
//     API with its own lifecycle.
//   - Several providers do nothing at all.
//
// A single CacheConfig applied uniformly would therefore be a no-op on most
// providers while looking configured. This package makes the difference
// inspectable instead: ask what a provider supports before assuming a setting
// took effect.
package cachepolicy

import (
	"fmt"
	"sort"
	"strings"
)

// Support describes how much control a provider offers over caching.
type Support int

const (
	// None means the provider does no prompt caching.
	None Support = iota

	// Automatic means the provider caches on its own. Configuration has no
	// effect, but usage may still be reported.
	Automatic

	// Explicit means the caller marks what to cache and the setting is honored.
	Explicit
)

func (s Support) String() string {
	switch s {
	case Explicit:
		return "explicit"
	case Automatic:
		return "automatic"
	default:
		return "none"
	}
}

// Capability describes one provider's caching behaviour.
type Capability struct {
	// Provider is the provider name, matching interfaces.LLM.Name().
	Provider string

	// Control is how much say the caller has.
	Control Support

	// ReportsUsage is whether cache hits appear in TokenUsage.
	//
	// This is tracked separately from Control on purpose: OpenAI offers no
	// control at all yet reports cache reads, which is exactly the combination
	// a single "supports caching" boolean would misrepresent.
	ReportsUsage bool

	// Notes explains anything a caller would otherwise get wrong.
	Notes string
}

// Honors reports whether an explicit CacheConfig has any effect here.
func (c Capability) Honors() bool { return c.Control == Explicit }

// capabilities is the provider matrix.
//
// Deliberately a table rather than per-provider interface methods: it is small,
// it changes rarely, and having it in one place makes the differences legible
// at a glance instead of scattered across eight packages.
var capabilities = map[string]Capability{
	"anthropic": {
		Provider:     "anthropic",
		Control:      Explicit,
		ReportsUsage: true,
		Notes: "Caches at explicit cache_control breakpoints. Extended (1h) TTL is not " +
			"supported: it needs an anthropic-beta header this client does not send.",
	},
	"openai": {
		Provider:     "openai",
		Control:      Automatic,
		ReportsUsage: true,
		Notes: "Caches long prompt prefixes automatically. CacheConfig has no effect; " +
			"cache reads are reported as CacheReadInputTokens.",
	},
	"azureopenai": {
		Provider:     "azureopenai",
		Control:      Automatic,
		ReportsUsage: true,
		Notes:        "Same automatic prefix caching as OpenAI. CacheConfig has no effect.",
	},
	"gemini": {
		Provider:     "gemini",
		Control:      None,
		ReportsUsage: false,
		Notes: "Gemini supports implicit caching and an explicit cached-content API, " +
			"neither of which this client uses yet. CacheConfig has no effect.",
	},
	"bedrock":  {Provider: "bedrock", Notes: "No prompt caching in this client."},
	"deepseek": {Provider: "deepseek", Notes: "No prompt caching in this client."},
	"ollama":   {Provider: "ollama", Notes: "No prompt caching in this client."},
	"vllm":     {Provider: "vllm", Notes: "No prompt caching in this client."},
	"vertex":   {Provider: "vertex", Notes: "No prompt caching in this client."},
}

// For returns the caching capability of a provider.
//
// An unknown provider reports no support, which is the safe answer: assuming a
// provider caches when it does not leads a caller to expect savings that never
// arrive.
func For(provider string) Capability {
	if c, ok := capabilities[strings.ToLower(strings.TrimSpace(provider))]; ok {
		return c
	}
	return Capability{
		Provider: provider,
		Notes:    "Unknown provider; assumed to have no prompt caching.",
	}
}

// Providers returns every provider in the matrix, sorted.
func Providers() []string {
	out := make([]string, 0, len(capabilities))
	for name := range capabilities {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Explain renders the matrix, for a status endpoint or a startup log.
func Explain() string {
	var b strings.Builder
	b.WriteString("Prompt caching support by provider:\n")
	for _, name := range Providers() {
		c := capabilities[name]
		fmt.Fprintf(&b, "  %-12s control=%-9s reports_usage=%-5t  %s\n",
			c.Provider, c.Control, c.ReportsUsage, c.Notes)
	}
	return b.String()
}

// CheckConfig reports whether an explicit caching configuration will do
// anything on a provider.
//
// Call it at startup rather than discovering from a flat bill that a setting
// was inert. Returns nil when the configuration is honored or when no caching
// was requested.
func CheckConfig(provider string, requested bool) error {
	if !requested {
		return nil
	}
	c := For(provider)
	if c.Honors() {
		return nil
	}
	return fmt.Errorf(
		"prompt caching was configured but %s does not honor it (control=%s): %s",
		c.Provider, c.Control, c.Notes)
}
