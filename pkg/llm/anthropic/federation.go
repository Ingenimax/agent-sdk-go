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
	jwtBearerGrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"

	// Refresh schedule from the Anthropic Workload Identity Federation docs.
	// After advisoryRefreshWindow a failed exchange still serves the cached token;
	// after mandatoryRefreshWindow it returns an error.
	advisoryRefreshWindow  = 120 * time.Second
	mandatoryRefreshWindow = 30 * time.Second
)

// IdentityTokenProvider returns the OIDC JWT issued to the workload by its identity provider.
// It is called on every token exchange, so it must return a fresh token each time.
type IdentityTokenProvider func(ctx context.Context) (string, error)

// IdentityTokenFile returns a provider that reads the JWT from path on every exchange,
// so rotated projected tokens (for example Kubernetes service-account tokens) are picked up.
func IdentityTokenFile(path string) IdentityTokenProvider {
	return func(ctx context.Context) (string, error) {
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
		c.federation = &federationTokenSource{
			provider: provider,
			opts:     opts,
			now:      time.Now,
		}
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
type federationTokenSource struct {
	provider IdentityTokenProvider
	opts     FederationOptions
	now      func() time.Time

	// mu also serializes exchanges, so concurrent requests never present the same JWT twice.
	mu        sync.Mutex
	token     string
	expiresAt time.Time
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
func (s *federationTokenSource) Token(ctx context.Context, httpClient *http.Client, baseURL string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if s.token != "" && now.Before(s.expiresAt.Add(-advisoryRefreshWindow)) {
		return s.token, nil
	}

	token, expiresIn, err := s.exchange(ctx, httpClient, baseURL)
	if err != nil {
		if s.token != "" && now.Before(s.expiresAt.Add(-mandatoryRefreshWindow)) {
			return s.token, nil
		}
		return "", err
	}

	s.token = token
	s.expiresAt = now.Add(expiresIn)
	return s.token, nil
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("workload identity federation: failed to read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("workload identity federation: token exchange returned status %d (request-id %s): %s",
			resp.StatusCode, resp.Header.Get("Request-Id"), string(respBody))
	}

	var tokenResp tokenExchangeResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", 0, fmt.Errorf("workload identity federation: failed to parse token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", 0, fmt.Errorf("workload identity federation: token response has no access_token")
	}
	return tokenResp.AccessToken, time.Duration(tokenResp.ExpiresIn) * time.Second, nil
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
