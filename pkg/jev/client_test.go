package jev

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemOne(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/systemone", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var request map[string]interface{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, "jev-latest", request["model"])

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "req_123")
		_, _ = w.Write([]byte(`{
			"model":"jev-1.13.0",
			"answers":{
				"urgent":{"type":"noul","noul":0.97},
				"team":{"type":"choice","choice":"ops","confidence":0.91,"probabilities":{"ops":0.91,"billing":0.09}},
				"severity":{"type":"score","score":1.8,"confidence":0.84,"legend":{"0":"low","1":"medium","2":"high"},"probabilities":{"0":0.02,"1":0.16,"2":0.82}}
			},
			"usage":{"input_tokens":42,"output_tokens":0}
		}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL), WithRetry(0, 0))
	response, err := client.SystemOne(context.Background(), Request{
		State: map[string]interface{}{"ticket": "production is down"},
		Questions: map[string]Question{
			"urgent":   Noul("Does this need attention now?"),
			"team":     Choice("Which team?", map[string]interface{}{"ops": "incidents", "billing": "payments"}),
			"severity": Score("How severe?", "low", "medium", "high"),
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "jev-1.13.0", response.Model)
	assert.Equal(t, "req_123", response.RequestID)
	assert.Equal(t, "ops", response.Answers["team"].Choice)
	assert.Equal(t, 42, response.Usage.InputTokens)
}

// unreachableClient fails the test if any HTTP request escapes validation, so
// "before the network" is enforced rather than assumed.
func unreachableClient(t *testing.T) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("request reached the network; validation should have rejected it first")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	return NewClient("test-key", WithBaseURL(server.URL), WithRetry(0, 0))
}

func TestSystemOneRejectsInvalidRequestBeforeNetwork(t *testing.T) {
	client := unreachableClient(t)
	_, err := client.SystemOne(context.Background(), Request{
		State: "ticket",
		Questions: map[string]Question{
			"team": Choice("Which team?", map[string]interface{}{"only": nil}),
		},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestSystemOneRejectsTypedNilQuestion(t *testing.T) {
	var question *ChoiceQuestion
	client := unreachableClient(t)
	_, err := client.SystemOne(context.Background(), Request{
		State:     "ticket",
		Questions: map[string]Question{"team": question},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestSystemOneRequiresAPIKey(t *testing.T) {
	client := unreachableClient(t)
	client.apiKey = ""
	_, err := client.SystemOne(context.Background(), Request{})
	assert.ErrorIs(t, err, ErrNoAPIKey)
}

func TestSystemOneReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-request-id", "req_bad")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid criteria"}}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL), WithRetry(0, 0))
	_, err := client.SystemOne(context.Background(), choiceRequest())
	require.Error(t, err)
	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusUnprocessableEntity, apiErr.StatusCode)
	assert.Equal(t, "req_bad", apiErr.RequestID)
	assert.Equal(t, "invalid criteria", apiErr.Message)
}

func TestSystemOneRetriesTransientStatus(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{
			"model":"jev-latest",
			"answers":{"route":{"type":"choice","choice":"a","confidence":0.8,"probabilities":{"a":0.8,"b":0.2}}},
			"usage":{"input_tokens":3,"output_tokens":0}
		}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL), WithRetry(1, time.Millisecond))
	_, err := client.SystemOne(context.Background(), choiceRequest())
	require.NoError(t, err)
	assert.Equal(t, int32(2), attempts.Load())
}

func TestSystemOneValidatesResponseAgainstQuestion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"model":"jev-latest",
			"answers":{"route":{"type":"choice","choice":"invented","confidence":0.8,"probabilities":{"invented":0.8,"b":0.2}}},
			"usage":{"input_tokens":3,"output_tokens":0}
		}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL), WithRetry(0, 0))
	_, err := client.SystemOne(context.Background(), choiceRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidResponse)
}

func choiceRequest() Request {
	return Request{
		State: "route me",
		Questions: map[string]Question{
			"route": Choice("Where?", map[string]interface{}{"a": nil, "b": nil}),
		},
	}
}

// TestSystemOneClampsRetryAfter pins the fix for a server-controlled stall: the
// header asks for far longer than maxRetryDelay, and the client must not honour it.
func TestSystemOneClampsRetryAfter(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "86400")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{
			"model":"jev-latest",
			"answers":{"route":{"type":"choice","choice":"a","confidence":0.8,"probabilities":{"a":0.8,"b":0.2}}},
			"usage":{"input_tokens":3,"output_tokens":0}
		}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL), WithRetry(1, time.Millisecond))
	// Shrink the ceiling so the clamp is observable without a 30s test.
	client.maxDelay = 150 * time.Millisecond

	start := time.Now()
	_, err := client.SystemOne(context.Background(), choiceRequest())
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, int32(2), attempts.Load())
	assert.Less(t, elapsed, 5*time.Second,
		"Retry-After must be clamped to the configured ceiling before the client waits on it")
}

func TestNewClientDefaultsMaxDelay(t *testing.T) {
	assert.Equal(t, maxRetryDelay, NewClient("k").maxDelay)
}

func TestSystemOneHonoursContextCancellationDuringBackoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := NewClient("test-key", WithBaseURL(server.URL), WithRetry(3, time.Second))
	_, err := client.SystemOne(ctx, choiceRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRetryAfterParsing(t *testing.T) {
	assert.Equal(t, 7*time.Second, retryAfter(http.Header{"Retry-After": []string{"7"}}))
	assert.Equal(t, time.Duration(0), retryAfter(http.Header{"Retry-After": []string{"0"}}))
	assert.Equal(t, time.Duration(0), retryAfter(http.Header{"Retry-After": []string{"-5"}}))
	assert.Equal(t, time.Duration(0), retryAfter(http.Header{"Retry-After": []string{"soon"}}))
	assert.Equal(t, time.Duration(0), retryAfter(http.Header{}))
	// A date already in the past must not produce a negative delay.
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	assert.Equal(t, time.Duration(0), retryAfter(http.Header{"Retry-After": []string{past}}))
	future := retryAfter(http.Header{"Retry-After": []string{time.Now().Add(20 * time.Second).UTC().Format(http.TimeFormat)}})
	assert.Greater(t, future, 10*time.Second)
}

func TestSystemOneRejectsScoreOutsideRubric(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"model":"jev-latest",
			"answers":{"severity":{"type":"score","score":999,"confidence":0.9,"probabilities":{"0":0.1,"1":0.9}}},
			"usage":{"input_tokens":3,"output_tokens":0}
		}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL), WithRetry(0, 0))
	_, err := client.SystemOne(context.Background(), Request{
		State:     "ticket",
		Questions: map[string]Question{"severity": Score("How severe?", "low", "medium", "high")},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidResponse)
}

func TestSystemOneAcceptsScoreAtRubricBounds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"model":"jev-latest",
			"answers":{"severity":{"type":"score","score":2,"confidence":0.9,"probabilities":{"0":0.1,"2":0.9}}},
			"usage":{"input_tokens":3,"output_tokens":0}
		}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL), WithRetry(0, 0))
	response, err := client.SystemOne(context.Background(), Request{
		State:     "ticket",
		Questions: map[string]Question{"severity": Score("How severe?", "low", "medium", "high")},
	})
	require.NoError(t, err)
	assert.Equal(t, 2.0, *response.Answers["severity"].Score)
}

func TestSystemOneRejectsProbabilityForUnknownChoice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"model":"jev-latest",
			"answers":{"route":{"type":"choice","choice":"a","confidence":0.8,"probabilities":{"a":0.8,"invented":0.2}}},
			"usage":{"input_tokens":3,"output_tokens":0}
		}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL), WithRetry(0, 0))
	_, err := client.SystemOne(context.Background(), choiceRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidResponse)
	assert.ErrorContains(t, err, "invented")
}

func TestAPIErrorMessage(t *testing.T) {
	assert.Equal(t, "jev: API error 500: boom", (&APIError{StatusCode: 500, Message: "boom"}).Error())
	assert.Equal(t, "jev: API error 500 (request r1): boom", (&APIError{StatusCode: 500, RequestID: "r1", Message: "boom"}).Error())
}

func TestNoulWithCriteriaAndOptions(t *testing.T) {
	question := NoulWithCriteria("Is it urgent?", "page someone", "wait for business hours")
	require.NoError(t, question.validate())
	assert.Equal(t, QuestionTypeNoul, question.questionType())

	client := NewClient("k", WithModel("jev-1.13.0"), WithHTTPClient(&http.Client{Timeout: time.Second}))
	assert.Equal(t, "jev-1.13.0", client.model)
	assert.Equal(t, time.Second, client.httpClient.Timeout)
}
