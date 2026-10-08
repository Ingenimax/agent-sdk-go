package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the TypeSafe AI API root.
	DefaultBaseURL = "https://api.typesafe.ai"
	// DefaultModel tracks TypeSafe's current Jev alias.
	DefaultModel    = "jev-latest"
	defaultTimeout  = 10 * time.Second
	maxResponseSize = 4 << 20
	// maxRetryDelay bounds a single backoff sleep. Retry-After is chosen by the
	// server, so it is clamped to this before the client waits on it.
	maxRetryDelay = 30 * time.Second
)

var (
	// ErrNoAPIKey means a request was attempted without a TypeSafe API key.
	ErrNoAPIKey = errors.New("jev: no API key configured")
	// ErrInvalidRequest means the request cannot satisfy the System One contract.
	ErrInvalidRequest = errors.New("jev: invalid request")
	// ErrInvalidResponse means TypeSafe returned a response that does not match the request.
	ErrInvalidResponse = errors.New("jev: invalid response")
)

// APIError describes a non-success response from TypeSafe AI.
type APIError struct {
	StatusCode int
	RequestID  string
	Message    string
}

func (e *APIError) Error() string {
	if e.RequestID == "" {
		return fmt.Sprintf("jev: API error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("jev: API error %d (request %s): %s", e.StatusCode, e.RequestID, e.Message)
}

// Client calls TypeSafe AI's System One endpoint.
type Client struct {
	apiKey         string
	baseURL        string
	model          string
	httpClient     *http.Client
	maxRetries     int
	initialBackoff time.Duration
	// maxDelay caps any single backoff sleep. Defaults to maxRetryDelay.
	maxDelay time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the TypeSafe API root, primarily for gateways and tests.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(baseURL, "/") }
}

// WithModel changes the default model used when Request.Model is empty.
func WithModel(model string) Option {
	return func(c *Client) { c.model = model }
}

// WithHTTPClient replaces the HTTP transport. The client is used as supplied.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) { c.httpClient = httpClient }
}

// WithRetry configures retries after the initial attempt and their first delay.
// Only connection failures, HTTP 408/429, and HTTP 5xx responses are retried.
// No single backoff sleep exceeds maxRetryDelay, including one requested by a
// Retry-After response header.
func WithRetry(maxRetries int, initialBackoff time.Duration) Option {
	return func(c *Client) {
		c.maxRetries = maxRetries
		c.initialBackoff = initialBackoff
	}
}

// NewClient creates a Jev client. The API key is only sent to the configured base URL.
func NewClient(apiKey string, options ...Option) *Client {
	c := &Client{
		apiKey:         apiKey,
		baseURL:        DefaultBaseURL,
		model:          DefaultModel,
		httpClient:     &http.Client{Timeout: defaultTimeout},
		maxRetries:     2,
		initialBackoff: 500 * time.Millisecond,
		maxDelay:       maxRetryDelay,
	}
	for _, option := range options {
		option(c)
	}
	return c
}

// SystemOne evaluates state against all named questions in a single Jev call.
func (c *Client) SystemOne(ctx context.Context, request Request) (*Response, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, ErrNoAPIKey
	}
	if c.httpClient == nil {
		return nil, fmt.Errorf("%w: HTTP client is nil", ErrInvalidRequest)
	}
	if request.Model == "" {
		request.Model = c.model
	}
	if err := validateRequest(request); err != nil {
		return nil, err
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("%w: state or questions are not JSON-compatible: %v", ErrInvalidRequest, err)
	}

	maxAttempts := max(c.maxRetries, 0)
	delay := c.initialBackoff
	// Every exit is a return, so the loop needs no condition of its own: the
	// attempt == maxAttempts check below is what bounds it.
	for attempt := 0; ; attempt++ {
		response, retryAfter, retryable, err := c.attempt(ctx, body, request)
		if err == nil {
			return response, nil
		}
		if !retryable || attempt == maxAttempts {
			return nil, err
		}
		if retryAfter > delay {
			delay = retryAfter
		}
		// Clamp before waiting, not after: retryAfter comes from the server.
		delay = min(delay, c.maxDelay)
		if err := wait(ctx, delay); err != nil {
			return nil, err
		}
		delay *= 2
	}
}

func (c *Client) attempt(ctx context.Context, body []byte, request Request) (*Response, time.Duration, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return nil, 0, false, fmt.Errorf("jev: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "agent-sdk-go/jev")

	httpResponse, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, false, ctx.Err()
		}
		return nil, 0, true, fmt.Errorf("jev: request failed: %w", err)
	}
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseSize+1))
	if err != nil {
		return nil, 0, true, fmt.Errorf("jev: read response: %w", err)
	}
	if len(responseBody) > maxResponseSize {
		return nil, 0, false, fmt.Errorf("%w: response exceeds %d bytes", ErrInvalidResponse, maxResponseSize)
	}

	requestID := httpResponse.Header.Get("x-request-id")
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		apiErr := decodeAPIError(httpResponse.StatusCode, requestID, responseBody)
		return nil, retryAfter(httpResponse.Header), isRetryableStatus(httpResponse.StatusCode), apiErr
	}

	var response Response
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, 0, false, fmt.Errorf("%w: decode JSON: %v", ErrInvalidResponse, err)
	}
	response.RequestID = requestID
	if err := validateResponse(request, &response); err != nil {
		return nil, 0, false, err
	}
	return &response, 0, false, nil
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.Model) == "" {
		return fmt.Errorf("%w: model must not be empty", ErrInvalidRequest)
	}
	if len(request.Questions) == 0 {
		return fmt.Errorf("%w: at least one question is required", ErrInvalidRequest)
	}
	for name, question := range request.Questions {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%w: question names must not be empty", ErrInvalidRequest)
		}
		if question == nil || nilQuestion(question) {
			return fmt.Errorf("%w: question %q is nil", ErrInvalidRequest, name)
		}
		if err := question.validate(); err != nil {
			return fmt.Errorf("%w: question %q: %v", ErrInvalidRequest, name, err)
		}
	}
	return nil
}

func validateResponse(request Request, response *Response) error {
	for name, question := range request.Questions {
		answer, ok := response.Answers[name]
		if !ok {
			return fmt.Errorf("%w: missing answer %q", ErrInvalidResponse, name)
		}
		if answer.Type != question.questionType() {
			return fmt.Errorf("%w: answer %q has type %q, want %q", ErrInvalidResponse, name, answer.Type, question.questionType())
		}
		switch question.questionType() {
		case QuestionTypeNoul:
			if answer.Noul == nil || !probability(*answer.Noul) {
				return fmt.Errorf("%w: answer %q has invalid noul probability", ErrInvalidResponse, name)
			}
		case QuestionTypeChoice:
			criteria := choiceCriteria(question)
			if _, ok := criteria[answer.Choice]; !ok {
				return fmt.Errorf("%w: answer %q selected unknown choice %q", ErrInvalidResponse, name, answer.Choice)
			}
			if !probability(answer.Confidence) || !validProbabilities(answer.Probabilities) {
				return fmt.Errorf("%w: answer %q has invalid probabilities", ErrInvalidResponse, name)
			}
			// Callers route on this distribution, so every key has to be an
			// outcome the caller actually offered.
			for label := range answer.Probabilities {
				if _, ok := criteria[label]; !ok {
					return fmt.Errorf("%w: answer %q has a probability for unknown choice %q", ErrInvalidResponse, name, label)
				}
			}
		case QuestionTypeScore:
			if answer.Score == nil || !probability(answer.Confidence) || !validProbabilities(answer.Probabilities) {
				return fmt.Errorf("%w: answer %q has invalid score data", ErrInvalidResponse, name)
			}
			// Scores index the rubric, so the first level is 0 and the last is
			// len(criteria)-1. Callers index label slices by this value.
			if highest := float64(scoreLevels(question) - 1); *answer.Score < 0 || *answer.Score > highest {
				return fmt.Errorf("%w: answer %q scored %v outside the 0..%v rubric", ErrInvalidResponse, name, *answer.Score, highest)
			}
		}
	}
	return nil
}

func nilQuestion(question Question) bool {
	switch typed := question.(type) {
	case *NoulQuestion:
		return typed == nil
	case *ChoiceQuestion:
		return typed == nil
	case *ScoreQuestion:
		return typed == nil
	default:
		return false
	}
}

func choiceCriteria(question Question) map[string]interface{} {
	switch typed := question.(type) {
	case ChoiceQuestion:
		return typed.Criteria
	case *ChoiceQuestion:
		return typed.Criteria
	default:
		return nil
	}
}

func scoreLevels(question Question) int {
	switch typed := question.(type) {
	case ScoreQuestion:
		return len(typed.Criteria)
	case *ScoreQuestion:
		return len(typed.Criteria)
	default:
		return 0
	}
}

func probability(value float64) bool { return value >= 0 && value <= 1 }

func validProbabilities(values map[string]float64) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if !probability(value) {
			return false
		}
	}
	return true
}

func decodeAPIError(status int, requestID string, body []byte) *APIError {
	var payload struct {
		Error   interface{} `json:"error"`
		Message string      `json:"message"`
		Detail  string      `json:"detail"`
	}
	_ = json.Unmarshal(body, &payload)
	message := payload.Message
	if message == "" {
		message = payload.Detail
	}
	if message == "" {
		switch value := payload.Error.(type) {
		case string:
			message = value
		case map[string]interface{}:
			message, _ = value["message"].(string)
		}
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return &APIError{StatusCode: status, RequestID: requestID, Message: message}
}

func isRetryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func retryAfter(header http.Header) time.Duration {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(time.Until(date), 0)
	}
	return 0
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(max(delay, 0))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
