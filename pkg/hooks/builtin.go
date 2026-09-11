package hooks

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Logging returns a plugin that records every tool call and its outcome.
func Logging(log func(format string, args ...any)) Plugin {
	if log == nil {
		log = func(string, ...any) {}
	}
	return Plugin{
		Name: "logging",
		BeforeTool: func(_ context.Context, call ToolCall) (Outcome, error) {
			log("tool call: %s args=%s agent=%s", call.Tool, call.Arguments, call.AgentName)
			return Outcome{}, nil
		},
		AfterTool: func(_ context.Context, call ToolCall, result string) (string, error) {
			log("tool result: %s bytes=%d", call.Tool, len(result))
			return result, nil
		},
		OnToolError: func(_ context.Context, call ToolCall, err error) (string, bool) {
			log("tool error: %s err=%v", call.Tool, err)
			return "", false
		},
	}
}

// AllowList returns a plugin that denies any tool not named.
//
// This is the smallest useful kill switch: an agent handed a broad tool set can
// be restricted per deployment without rebuilding the tool set.
func AllowList(allowed ...string) Plugin {
	permitted := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		permitted[name] = struct{}{}
	}

	return Plugin{
		Name: "allowlist",
		BeforeTool: func(_ context.Context, call ToolCall) (Outcome, error) {
			if _, ok := permitted[call.Tool]; ok {
				return Outcome{}, nil
			}
			return Outcome{
				Decision: Deny,
				Message: fmt.Sprintf(
					"The tool %q is not available in this deployment. Do not try it again; use one of the tools you were given.",
					call.Tool),
			}, nil
		},
	}
}

// Redact returns a plugin that masks matches in tool results before the model
// sees them.
//
// Results are where secrets most often leak: a tool reads a config file or a
// log line and hands it straight to the model, which may then repeat it.
func Redact(replacement string, patterns ...*regexp.Regexp) Plugin {
	if replacement == "" {
		replacement = "[REDACTED]"
	}
	return Plugin{
		Name: "redact",
		AfterTool: func(_ context.Context, _ ToolCall, result string) (string, error) {
			for _, pattern := range patterns {
				if pattern == nil {
					continue
				}
				result = pattern.ReplaceAllString(result, replacement)
			}
			return result, nil
		},
	}
}

// TruncateResults returns a plugin that caps how much of a tool result reaches
// the model.
//
// An unbounded tool result is the most common way a context window is blown:
// one directory listing or query dump can dwarf the conversation.
func TruncateResults(maxBytes int) Plugin {
	if maxBytes <= 0 {
		maxBytes = 8000
	}
	return Plugin{
		Name: "truncate",
		AfterTool: func(_ context.Context, _ ToolCall, result string) (string, error) {
			if len(result) <= maxBytes {
				return result, nil
			}
			// Say what was dropped. A silent truncation looks to the model like
			// the data simply ended, and it will reason on a partial answer
			// without knowing.
			return fmt.Sprintf("%s\n\n[truncated: %d of %d bytes shown]",
				result[:maxBytes], maxBytes, len(result)), nil
		},
	}
}

// RetryOnError returns a plugin that reports a tool failure to the model as a
// readable result instead of an error, so it can adapt rather than see an
// opaque failure.
func RetryOnError() Plugin {
	return Plugin{
		Name: "retry-on-error",
		OnToolError: func(_ context.Context, call ToolCall, err error) (string, bool) {
			return fmt.Sprintf(
				"The tool %q failed: %v. Consider a different approach or different arguments.",
				call.Tool, err), true
		},
	}
}

// Metrics collects per-tool call counts, failures and durations.
type Metrics struct {
	mu    sync.Mutex
	calls map[string]int
	fails map[string]int
	total map[string]time.Duration
}

// NewMetrics creates a metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		calls: map[string]int{},
		fails: map[string]int{},
		total: map[string]time.Duration{},
	}
}

// Plugin returns the plugin that feeds this collector.
func (m *Metrics) Plugin() Plugin {
	return Plugin{
		Name: "metrics",
		BeforeTool: func(_ context.Context, call ToolCall) (Outcome, error) {
			m.mu.Lock()
			m.calls[call.Tool]++
			m.mu.Unlock()
			return Outcome{}, nil
		},
		OnToolError: func(_ context.Context, call ToolCall, _ error) (string, bool) {
			m.mu.Lock()
			m.fails[call.Tool]++
			m.mu.Unlock()
			return "", false
		},
	}
}

// Calls returns how many times each tool was invoked.
func (m *Metrics) Calls() map[string]int {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]int, len(m.calls))
	for k, v := range m.calls {
		out[k] = v
	}
	return out
}

// Failures returns how many times each tool failed.
func (m *Metrics) Failures() map[string]int {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]int, len(m.fails))
	for k, v := range m.fails {
		out[k] = v
	}
	return out
}

// CommonSecretPatterns returns regexps for values that should not reach a model
// in a tool result.
//
// Deliberately conservative: a pattern that over-matches corrupts legitimate
// output, which is its own failure.
func CommonSecretPatterns() []*regexp.Regexp {
	return []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(sk|pk)-[A-Za-z0-9]{20,}\b`),              // API keys
		regexp.MustCompile(`(?i)\bAKIA[0-9A-Z]{16}\b`),                      // AWS access key ID
		regexp.MustCompile(`(?i)\bghp_[A-Za-z0-9]{36}\b`),                   // GitHub PAT
		regexp.MustCompile(`\b[0-9]{3}-[0-9]{2}-[0-9]{4}\b`),                // US SSN
		regexp.MustCompile(`(?i)\b(?:password|passwd|secret)\s*[=:]\s*\S+`), // key=value secrets
	}
}

// Describe renders a registry's plugins for logging or a status endpoint.
func (r *Registry) Describe() string {
	names := r.Names()
	if len(names) == 0 {
		return "no plugins registered"
	}
	return "plugins: " + strings.Join(names, ", ")
}
