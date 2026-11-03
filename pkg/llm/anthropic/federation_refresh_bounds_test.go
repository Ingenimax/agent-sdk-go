package anthropic

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// freshJWTs mints a distinct assertion per call, like a provider that requests a
// new token from its identity provider on every exchange. With a reused JWT the
// second exchange would fail on jti replay and end the loop by accident, hiding
// the bug these tests pin.
func freshJWTs() IdentityTokenProvider {
	var n atomic.Int32
	return func(context.Context) (string, error) {
		return fmt.Sprintf("jwt-%d", n.Add(1)), nil
	}
}

// boundedCtx keeps a regression from hanging the suite: before the fix these
// cases looped until the context expired.
func boundedCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestFederationShortLivedTokensAreBounded pins the fix for an unbounded exchange
// loop. A token whose lifetime is at or below advisoryRefreshWindow could never
// satisfy resolveLocked, so every pass of Token's loop started another exchange:
// one call made ~30,000 exchanges in 1.5s and still returned an error.
func TestFederationShortLivedTokensAreBounded(t *testing.T) {
	for _, expiresIn := range []int64{1, 25, 30, 31, 60, 119, 120, 121, 600} {
		t.Run(fmt.Sprintf("expires_in=%d", expiresIn), func(t *testing.T) {
			server := newTokenServer(t)
			server.expiresIn.Store(expiresIn)
			source := newTestSource(freshJWTs(), nil)

			token, err := source.Token(boundedCtx(t), server.Client(), server.URL)
			if n := server.requests.Load(); n != 1 {
				t.Fatalf("one Token() call made %d exchanges, want exactly 1", n)
			}
			if err != nil {
				t.Fatalf("Token() error = %v, want the token just minted", err)
			}
			if token != "token-1" {
				t.Fatalf("Token() = %q, want token-1", token)
			}

			// Calls right after a successful exchange reuse it rather than
			// exchanging again, whatever the token's lifetime.
			for i := 0; i < 5; i++ {
				if got, err := source.Token(boundedCtx(t), server.Client(), server.URL); err != nil || got != "token-1" {
					t.Fatalf("repeat call %d: Token() = %q, %v, want token-1", i, got, err)
				}
			}
			if n := server.requests.Load(); n != 1 {
				t.Errorf("six calls made %d exchanges, want 1", n)
			}
		})
	}
}

// TestFederationShortLivedTokensRefreshAtMostOncePerBackoff checks that a rule
// issuing tokens too short to leave the refresh windows is throttled to one
// exchange per exchangeBackoff, not one per request.
func TestFederationShortLivedTokensRefreshAtMostOncePerBackoff(t *testing.T) {
	server := newTokenServer(t)
	server.expiresIn.Store(60)

	var clock atomic.Int64
	clock.Store(time.Unix(1_000_000, 0).UnixNano())
	source := newTestSource(freshJWTs(), nil)
	source.now = func() time.Time { return time.Unix(0, clock.Load()) }

	call := func() string {
		t.Helper()
		token, err := source.Token(boundedCtx(t), server.Client(), server.URL)
		if err != nil {
			t.Fatalf("Token() error = %v", err)
		}
		return token
	}

	call()
	for i := 0; i < 50; i++ {
		clock.Add(int64(100 * time.Millisecond)) // 5s total, inside one backoff
		call()
	}
	if n := server.requests.Load(); n != 1 {
		t.Fatalf("50 calls within %s made %d exchanges, want 1", exchangeBackoff, n)
	}

	clock.Add(int64(exchangeBackoff))
	if got := call(); got != "token-2" {
		t.Errorf("after the backoff Token() = %q, want a refreshed token-2", got)
	}
	if n := server.requests.Load(); n != 2 {
		t.Errorf("exchanges after the backoff = %d, want 2", n)
	}
}

// TestFederationRejectsOverflowingExpiresIn pins the upper bound. time.Duration
// is int64 nanoseconds, so expires_in >= 2^34 seconds silently became a negative
// lifetime, dating expiresAt in the past and feeding the loop above.
func TestFederationRejectsOverflowingExpiresIn(t *testing.T) {
	for _, expiresIn := range []int64{1 << 34, 1 << 40, 1<<62 + 1} {
		t.Run(fmt.Sprintf("expires_in=%d", expiresIn), func(t *testing.T) {
			server := newTokenServer(t)
			server.expiresIn.Store(expiresIn)
			source := newTestSource(freshJWTs(), nil)

			_, err := source.Token(boundedCtx(t), server.Client(), server.URL)
			if err == nil || !strings.Contains(err.Error(), "expires_in") {
				t.Fatalf("Token() error = %v, want an expires_in validation error", err)
			}
			if n := server.requests.Load(); n != 1 {
				t.Errorf("exchanges = %d, want 1", n)
			}
		})
	}
}

// TestFederationTokenExpiredOnArrivalIsBounded covers the residual case the
// bound exists for: an exchange slower than the token's own lifetime yields a
// token already expired on arrival. Exchanging again cannot help, so Token must
// report that instead of looping.
func TestFederationTokenExpiredOnArrivalIsBounded(t *testing.T) {
	server := newTokenServer(t)
	server.expiresIn.Store(1)

	var clock atomic.Int64
	clock.Store(time.Unix(1_000_000, 0).UnixNano())
	slow := func(context.Context) (string, error) {
		clock.Add(int64(5 * time.Second)) // the exchange outlives the token
		return "jwt", nil
	}
	source := newTestSource(slow, nil)
	source.now = func() time.Time { return time.Unix(0, clock.Load()) }

	_, err := source.Token(boundedCtx(t), server.Client(), server.URL)
	if err == nil {
		t.Fatal("Token() returned an already-expired token")
	}
	if strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("Token() looped until the deadline: %v", err)
	}
	if n := server.requests.Load(); n != 1 {
		t.Errorf("exchanges = %d, want 1", n)
	}
}

// TestFederationBackoffMeasuredFromCompletion pins the backoff fix.
// lastAttempt was recorded when the exchange *started*, but exchangeTimeout
// (30s) exceeds exchangeBackoff (10s), so an exchange that took longer than the
// backoff to fail left the backoff already expired and the next caller
// exchanged again at once.
func TestFederationBackoffMeasuredFromCompletion(t *testing.T) {
	server := newTokenServer(t)
	server.fail.Store(true)

	var clock atomic.Int64
	clock.Store(time.Unix(1_000_000, 0).UnixNano())
	slow := func(context.Context) (string, error) {
		clock.Add(int64(20 * time.Second)) // longer than exchangeBackoff
		return "jwt", nil
	}
	source := newTestSource(slow, nil)
	source.now = func() time.Time { return time.Unix(0, clock.Load()) }

	for i := 0; i < 3; i++ {
		if _, err := source.Token(boundedCtx(t), server.Client(), server.URL); err == nil {
			t.Fatalf("call %d: Token() error = nil, want the exchange failure", i)
		}
	}
	if n := server.requests.Load(); n != 1 {
		t.Fatalf("three calls inside the backoff made %d exchanges, want 1", n)
	}

	clock.Add(int64(exchangeBackoff))
	_, _ = source.Token(boundedCtx(t), server.Client(), server.URL)
	if n := server.requests.Load(); n != 2 {
		t.Errorf("exchanges after the backoff = %d, want 2", n)
	}
}

// TestFederationRejectsIncompleteConfigWithoutExchanging covers construction
// validation: a missing required field should fail fast with an actionable
// message, not read the identity token and send a request the server rejects.
func TestFederationRejectsIncompleteConfigWithoutExchanging(t *testing.T) {
	full := FederationOptions{FederationRuleID: "fdrl_1", OrganizationID: "org-1", ServiceAccountID: "svac_1"}

	for _, tc := range []struct {
		name     string
		provider IdentityTokenProvider
		opts     FederationOptions
		want     string
	}{
		{"nil provider", nil, full, "identity token provider"},
		{"no rule", freshJWTs(), FederationOptions{OrganizationID: "org-1", ServiceAccountID: "svac_1"}, "federation_rule_id"},
		{"no org", freshJWTs(), FederationOptions{FederationRuleID: "fdrl_1", ServiceAccountID: "svac_1"}, "organization_id"},
		{"no service account", freshJWTs(), FederationOptions{FederationRuleID: "fdrl_1", OrganizationID: "org-1"}, "service_account_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newTokenServer(t)
			source := newFederationTokenSource(tc.provider, tc.opts)

			_, err := source.Token(boundedCtx(t), server.Client(), server.URL)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Token() error = %v, want it to name %q", err, tc.want)
			}
			if n := server.requests.Load(); n != 0 {
				t.Errorf("an incomplete config still made %d exchanges, want 0", n)
			}
		})
	}
}

// TestIdentityTokenFileHonorsCancelledContext checks the provider respects the
// context it is handed, so a cancelled exchange does not touch the filesystem.
func TestIdentityTokenFileHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := IdentityTokenFile("/nonexistent/should/not/be/read")(ctx); err == nil ||
		!strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Errorf("IdentityTokenFile with a cancelled ctx error = %v, want context.Canceled", err)
	}
}

type traceKey struct{}

// TestFederationExchangeKeepsCallerContextValues checks the exchange inherits the
// starting caller's values (trace span, org ID) while still outliving its
// cancellation. It previously ran on context.Background() and carried neither.
func TestFederationExchangeKeepsCallerContextValues(t *testing.T) {
	server := newTokenServer(t)
	var seen atomic.Value
	provider := func(ctx context.Context) (string, error) {
		if v, ok := ctx.Value(traceKey{}).(string); ok {
			seen.Store(v)
		}
		return "jwt", nil
	}
	source := newTestSource(provider, nil)

	ctx := context.WithValue(boundedCtx(t), traceKey{}, "span-42")
	if _, err := source.Token(ctx, server.Client(), server.URL); err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got, _ := seen.Load().(string); got != "span-42" {
		t.Errorf("provider saw trace value %q, want span-42", got)
	}
}

func TestErrorExcerptBoundsAndKeepsUTF8(t *testing.T) {
	short := []byte(`{"error":"nope"}`)
	if got := errorExcerpt(short); got != string(short) {
		t.Errorf("short body altered: %q", got)
	}

	long := []byte(strings.Repeat("é", maxTokenErrorBytes)) // 2 bytes per rune
	got := errorExcerpt(long)
	if len(got) > maxTokenErrorBytes+len("...(truncated)") {
		t.Errorf("excerpt is %d bytes, want at most %d", len(got), maxTokenErrorBytes+len("...(truncated)"))
	}
	if !strings.HasSuffix(got, "...(truncated)") {
		t.Errorf("excerpt %q lacks the truncation marker", got[len(got)-20:])
	}
	if !utf8ValidString(got) {
		t.Error("excerpt split a multi-byte rune and is not valid UTF-8")
	}
}

func utf8ValidString(s string) bool { return strings.ToValidUTF8(s, "�") == s }
