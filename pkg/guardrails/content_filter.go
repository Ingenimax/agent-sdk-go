package guardrails

import (
	"context"
	"regexp"
	"strings"
)

// ContentFilter implements a guardrail that filters inappropriate content
type ContentFilter struct {
	blockedWords []string
	action       Action
	regex        *regexp.Regexp
}

// NewContentFilter creates a new content filter guardrail.
//
// Words are matched case-insensitively, on whole-word boundaries where the word
// has them. A word containing regex metacharacters is matched literally, and an
// empty word list matches nothing.
//
// None of that used to hold. The words were interpolated into the pattern raw
// despite the old comment claiming they were escaped, so an ordinary config
// value like "c++" panicked inside regexp.MustCompile at construction; and an
// empty list produced `\b()\b`, an empty alternative matching at every word
// boundary, which redacted the entire text it was meant to leave alone.
//
// The boundary anchors are applied per word rather than around the whole
// alternation, because `\b` only holds next to a word character: `\b(c\+\+)\b`
// compiles but can never match, since the position after "+" is a boundary only
// when a word character follows.
func NewContentFilter(blockedWords []string, action Action) *ContentFilter {
	alternatives := make([]string, 0, len(blockedWords))
	for _, word := range blockedWords {
		if word == "" {
			// An empty alternative matches everywhere, same as an empty list.
			continue
		}

		pattern := regexp.QuoteMeta(word)
		if isWordChar(rune(word[0])) {
			pattern = `\b` + pattern
		}
		if isWordChar(rune(word[len(word)-1])) {
			pattern += `\b`
		}
		alternatives = append(alternatives, pattern)
	}

	var regex *regexp.Regexp
	if len(alternatives) > 0 {
		regex = regexp.MustCompile(`(?i)(?:` + strings.Join(alternatives, "|") + `)`)
	}

	return &ContentFilter{
		blockedWords: blockedWords,
		action:       action,
		regex:        regex,
	}
}

// isWordChar reports whether r is what \b considers a word character.
func isWordChar(r rune) bool {
	return r == '_' ||
		(r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z')
}

// Type returns the type of guardrail
func (c *ContentFilter) Type() GuardrailType {
	return ContentFilterGuardrail
}

// CheckRequest checks if a request violates the guardrail
func (c *ContentFilter) CheckRequest(ctx context.Context, request string) (bool, string, error) {
	if c.regex != nil && c.regex.MatchString(request) {
		modified := c.regex.ReplaceAllString(request, "****")
		return true, modified, nil
	}
	return false, request, nil
}

// CheckResponse checks if a response violates the guardrail
func (c *ContentFilter) CheckResponse(ctx context.Context, response string) (bool, string, error) {
	if c.regex != nil && c.regex.MatchString(response) {
		modified := c.regex.ReplaceAllString(response, "****")
		return true, modified, nil
	}
	return false, response, nil
}

// Action returns the action to take when the guardrail is triggered
func (c *ContentFilter) Action() Action {
	return c.action
}
