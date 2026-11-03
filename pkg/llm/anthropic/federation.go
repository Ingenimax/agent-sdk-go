package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	jwtBearerGrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer" // #nosec G101 - OAuth grant type URN, not a credential

	// Refresh schedule from the Anthropic Workload Identity Federation docs.
	// After advisoryRefreshWindow a failed exchange still serves the cached token;
	// after mandatoryRefreshWindow it returns an error.
	advisoryRefreshWindow  = 120 * time.Second
	mandatoryRefreshWindow = 30 * time.Second

	// exchangeTimeout bounds a single token exchange so a hung token endpoint
	// cannot block every request on the client for the HTTP client's full timeout.
	exchangeTimeout = 30 * time.Second

	// exchangeBackoff is how long a failed exchange is remembered. Within it the
	// cached token is served (advisory window) or the last error is returned
	// (mandatory window) instead of replaying the identity token on every request.
	exchangeBackoff = 10 * time.Second

	// maxTokenResponseBytes caps how much of a token endpoint response is read.
	maxTokenResponseBytes = 64 << 10

	// maxTokenErrorBytes caps how much of an error response is quoted in the returned
	// error, which propagates into the retry executor and is logged there.
	maxTokenErrorBytes = 512

	// maxTokenLifetime bounds expires_in. time.Duration is int64 nanoseconds, so an
	// unbounded expires_in of 2^34 seconds or more overflowed to a negative lifetime and
	// dated the token in the past. The cap is far above any real access-token lifetime,
	// so it rejects only values that are a bug or an attack.
	maxTokenLifetime = 30 * 24 * time.Hour
)

// IdentityTokenProvider returns the OIDC JWT issued to the workload by its identity provider.
// It is called on every token exchange. Identity tokens that carry a jti claim are single-use,
// so the provider must return a fresh token each time for long-running workloads.
type IdentityTokenProvider func(ctx context.Context) (string, error)

// IdentityTokenFile returns a provider that reads the JWT from path on every exchange,
// so rotated projected tokens (for example Kubernetes service-account tokens) are picked up.
func IdentityTokenFile(path string) IdentityTokenProvider {
	return func(ctx context.Context) (string, error) {
		// os.ReadFile cannot be interrupted, so at least do not start a read for an
		// exchange that has already been cancelled.
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("failed to read identity token file: %w", err)
		}
		data, err := os.ReadFile(path) // #nosec G304 -- path comes from client configuration
		if err != nil {
			return "", fmt.Errorf("failed to read identity token file: %w", err)
		}
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", fmt.Errorf("identity token file %s is empty", path)
		}
		return token, nil
	}
}

// IdentityToken returns a provider that always returns the given JWT.
//
// Most identity providers set a jti claim, which makes the token single-use: the first
// refresh after the access token's lifetime is rejected as a replay. Use this only for
// short-lived jobs that need a single exchange. Long-running workloads should use
// IdentityTokenFile or a custom IdentityTokenProvider that mints a fresh JWT per call.
func IdentityToken(token string) IdentityTokenProvider {
	return func(ctx context.Context) (string, error) {
		if token == "" {
			return "", fmt.Errorf("identity token is empty")
		}
		return token, nil
	}
}

// FederationOptions identifies the federation rule and service account to exchange the identity token for.
type FederationOptions struct {
	FederationRuleID string // fdrl_...
	OrganizationID   string // organization UUID
	ServiceAccountID string // svac_...
	// WorkspaceID (wrkspc_...) is required only when the rule is enabled for more than one workspace.
	WorkspaceID string
}

// WithFederationTokenProvider authenticates with Workload Identity Federation instead of an API key.
// The client exchanges the identity token for a short-lived Anthropic access token, sends it as a
// Bearer token, and refreshes it before it expires.
// See https://platform.claude.com/docs/en/manage-claude/workload-identity-federation
func WithFederationTokenProvider(provider IdentityTokenProvider, opts FederationOptions) Option {
	return func(c *AnthropicClient) {
		c.federation = newFederationTokenSource(provider, opts)
	}
}

// FederationFromEnv reads the Workload Identity Federation settings from the standard
// ANTHROPIC_* environment variables through getenv (os.Getenv when nil). It returns false unless
// ANTHROPIC_FEDERATION_RULE_ID, ANTHROPIC_ORGANIZATION_ID, ANTHROPIC_SERVICE_ACCOUNT_ID, and one of
// ANTHROPIC_IDENTITY_TOKEN_FILE or ANTHROPIC_IDENTITY_TOKEN are set.
func FederationFromEnv(getenv func(string) string) (IdentityTokenProvider, FederationOptions, bool) {
	if getenv == nil {
		getenv = os.Getenv
	}
	opts := FederationOptions{
		FederationRuleID: getenv("ANTHROPIC_FEDERATION_RULE_ID"),
		OrganizationID:   getenv("ANTHROPIC_ORGANIZATION_ID"),
		ServiceAccountID: getenv("ANTHROPIC_SERVICE_ACCOUNT_ID"),
		WorkspaceID:      getenv("ANTHROPIC_WORKSPACE_ID"),
	}
	if opts.FederationRuleID == "" || opts.OrganizationID == "" || opts.ServiceAccountID == "" {
		return nil, FederationOptions{}, false
	}

	if path := getenv("ANTHROPIC_IDENTITY_TOKEN_FILE"); path != "" {
		return IdentityTokenFile(path), opts, true
	}
	if token := getenv("ANTHROPIC_IDENTITY_TOKEN"); token != "" {
		return IdentityToken(token), opts, true
	}
	return nil, FederationOptions{}, false
}

// federationTokenSource caches the Anthropic access token minted by the token exchange.
// At most one exchange runs at a time; callers that need a token while one is in flight
// wait for it (or for their context) instead of starting a second exchange, so the same
// identity token is never presented twice.
type federationTokenSource struct {
	provider IdentityTokenProvider
	opts     FederationOptions
	now      func() time.Time

	mu          sync.Mutex
	token       string
	expiresAt   time.Time
	inflight    chan struct{} // closed when the running exchange finishes
	lastErr     error         // result of the last exchange, nil on success
	lastAttempt time.Time     // when the last exchange finished

	// configErr is a configuration the token endpoint is certain to reject. Options
	// cannot return errors, so it is held here and returned by every Token call
	// before the identity token is read or a request is sent.
	configErr error
}

func newFederationTokenSource(provider IdentityTokenProvider, opts FederationOptions) *federationTokenSource {
	return &federationTokenSource{
		provider:  provider,
		opts:      opts,
		now:       time.Now,
		configErr: validateFederation(provider, opts),
	}
}

// validateFederation reports a missing provider or required field.
func validateFederation(provider IdentityTokenProvider, opts FederationOptions) error {
	if provider == nil {
		return fmt.Errorf("workload identity federation: no identity token provider configured")
	}
	var missing []string
	if opts.FederationRuleID == "" {
		missing = append(missing, "federation_rule_id")
	}
	if opts.OrganizationID == "" {
		missing = append(missing, "organization_id")
	}
	if opts.ServiceAccountID == "" {
		missing = append(missing, "service_account_id")
	}
	if len(missing) > 0 {
		return fmt.Errorf("workload identity federation: missing required %s", strings.Join(missing, ", "))
	}
	return nil
}

type tokenExchangeRequest struct {
	GrantType        string `json:"grant_type"`
	Assertion        string `json:"assertion"`
	FederationRuleID string `json:"federation_rule_id"`
	OrganizationID   string `json:"organization_id"`
	ServiceAccountID string `json:"service_account_id"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
}

type tokenExchangeResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// Token returns a valid access token, exchanging a new identity token when the cached one is near expiry.
//
// The loop runs at most twice: once to start or join an exchange, and once to read its
// result. It used to be unbounded, and a token whose lifetime was at or below
// advisoryRefreshWindow could never satisfy resolveLocked, so every pass started another
// exchange: one call made tens of thousands of them and still returned an error.
func (s *federationTokenSource) Token(ctx context.Context, httpClient *http.Client, baseURL string) (string, error) {
	if s.configErr != nil {
		return "", s.configErr
	}
	for waited := false; ; waited = true {
		s.mu.Lock()
		token, settled, err := s.resolveLocked()
		if settled {
			s.mu.Unlock()
			return token, err
		}
		if waited {
			// resolveLocked settles after every completed exchange except one whose token
			// had already expired on arrival. Exchanging again cannot fix that.
			err := s.unservableLocked()
			s.mu.Unlock()
			return "", err
		}
		done := s.inflight
		if done == nil {
			done = make(chan struct{})
			s.inflight = done
			// WithoutCancel keeps the starting caller's values (trace, org) on the exchange
			// while detaching its lifetime, since the exchange is shared by every waiter.
			go s.refresh(context.WithoutCancel(ctx), httpClient, baseURL, done) // #nosec G118 - exchange is shared by all waiters, so it must outlive any single caller's ctx
		}
		s.mu.Unlock()

		select {
		case <-done:
		case <-ctx.Done():
			return "", fmt.Errorf("workload identity federation: waiting for token exchange: %w", ctx.Err())
		}
	}
}

// resolveLocked decides from the cached state alone. settled is false when an exchange is needed.
func (s *federationTokenSource) resolveLocked() (token string, settled bool, err error) {
	now := s.now()
	cached := s.token != "" && now.Before(s.expiresAt)
	// recentAttempt spaces exchanges, successful or not, by at least exchangeBackoff.
	recentAttempt := !s.lastAttempt.IsZero() && now.Sub(s.lastAttempt) < exchangeBackoff
	recentFailure := recentAttempt && s.lastErr != nil

	switch {
	case cached && now.Before(s.expiresAt.Add(-advisoryRefreshWindow)):
		return s.token, true, nil
	case cached && recentAttempt && s.lastErr == nil:
		// An exchange just succeeded and this is what it minted. A token already inside a
		// refresh window on arrival means the rule issues short-lived tokens; exchanging
		// again cannot produce a longer one, and doing so on every call is the storm
		// this case exists to prevent.
		return s.token, true, nil
	case cached && now.Before(s.expiresAt.Add(-mandatoryRefreshWindow)):
		// Advisory window: refresh, but keep serving the cached token after a recent failure.
		if recentFailure {
			return s.token, true, nil
		}
		return "", false, nil
	default:
		// Mandatory window or no token: the cached token is too close to expiry to serve.
		if recentFailure {
			return "", true, s.lastErr
		}
		return "", false, nil
	}
}

// unservableLocked explains why a completed exchange left nothing to serve.
func (s *federationTokenSource) unservableLocked() error {
	if s.lastErr != nil {
		return s.lastErr
	}
	return fmt.Errorf("workload identity federation: access token expired before it could be used; " +
		"the token exchange took longer than the token's expires_in")
}

// refresh runs one exchange with its own bounded context, so neither a caller's cancellation nor
// the HTTP client's long timeout affects the other callers waiting on it. parent supplies context
// values only; it is expected to carry no cancellation.
func (s *federationTokenSource) refresh(parent context.Context, httpClient *http.Client, baseURL string, done chan struct{}) {
	ctx, cancel := context.WithTimeout(parent, exchangeTimeout)
	defer cancel()

	start := s.now()
	token, expiresIn, err := s.exchange(ctx, httpClient, baseURL)

	s.mu.Lock()
	// Time the backoff from when the attempt finished. exchangeTimeout exceeds exchangeBackoff,
	// so timing it from the start left the backoff already over for exactly the slow,
	// hung-endpoint failures it exists to absorb.
	s.lastAttempt = s.now()
	s.lastErr = err
	if err == nil {
		s.token = token
		// Date the lifetime from before the request, which errs toward refreshing early.
		s.expiresAt = start.Add(expiresIn)
	}
	s.inflight = nil
	s.mu.Unlock()
	close(done)
}

func (s *federationTokenSource) exchange(ctx context.Context, httpClient *http.Client, baseURL string) (string, time.Duration, error) {
	if s.provider == nil {
		return "", 0, fmt.Errorf("workload identity federation: no identity token provider configured")
	}
	assertion, err := s.provider(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("workload identity federation: %w", err)
	}

	body, err := json.Marshal(tokenExchangeRequest{
		GrantType:        jwtBearerGrantType,
		Assertion:        assertion,
		FederationRuleID: s.opts.FederationRuleID,
		OrganizationID:   s.opts.OrganizationID,
		ServiceAccountID: s.opts.ServiceAccountID,
		WorkspaceID:      s.opts.WorkspaceID,
	})
	if err != nil {
		return "", 0, fmt.Errorf("workload identity federation: failed to marshal token request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(baseURL, "/")+"/v1/oauth/token", bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("workload identity federation: failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("workload identity federation: token exchange failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBytes))
	if err != nil {
		return "", 0, fmt.Errorf("workload identity federation: failed to read token response: %w", err)
	}
	// Drain what the limit left unread so the connection can return to the pool.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxTokenResponseBytes))
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("workload identity federation: token exchange returned status %d (request-id %s): %s",
			resp.StatusCode, resp.Header.Get("Request-Id"), errorExcerpt(respBody))
	}

	var tokenResp tokenExchangeResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", 0, fmt.Errorf("workload identity federation: failed to parse token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", 0, fmt.Errorf("workload identity federation: token response has no access_token")
	}
	if !strings.EqualFold(tokenResp.TokenType, "Bearer") {
		return "", 0, fmt.Errorf("workload identity federation: unsupported token_type %q", tokenResp.TokenType)
	}
	if tokenResp.ExpiresIn <= 0 || tokenResp.ExpiresIn > int64(maxTokenLifetime/time.Second) {
		return "", 0, fmt.Errorf("workload identity federation: token response has invalid expires_in %d", tokenResp.ExpiresIn)
	}
	return tokenResp.AccessToken, time.Duration(tokenResp.ExpiresIn) * time.Second, nil
}

// errorExcerpt bounds how much of an error response is quoted, keeping it valid UTF-8.
func errorExcerpt(body []byte) string {
	if len(body) <= maxTokenErrorBytes {
		return string(body)
	}
	return strings.ToValidUTF8(string(body[:maxTokenErrorBytes]), "") + "...(truncated)"
}

// setAuthHeader authenticates req with the federated access token, or with the API key otherwise.
func (c *AnthropicClient) setAuthHeader(ctx context.Context, req *http.Request) error {
	if c.federation == nil {
		req.Header.Set("X-API-Key", c.APIKey)
		return nil
	}
	token, err := c.federation.Token(ctx, c.HTTPClient, c.BaseURL)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}
