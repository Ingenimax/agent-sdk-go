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

func TestSystemOneRejectsInvalidRequestBeforeNetwork(t *testing.T) {
	client := NewClient("test-key", WithRetry(0, 0))
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
	client := NewClient("test-key", WithRetry(0, 0))
	_, err := client.SystemOne(context.Background(), Request{
		State:     "ticket",
		Questions: map[string]Question{"team": question},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestSystemOneRequiresAPIKey(t *testing.T) {
	_, err := NewClient("").SystemOne(context.Background(), Request{})
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
