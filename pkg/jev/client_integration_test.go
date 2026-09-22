package jev

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSystemOneIntegration(t *testing.T) {
	apiKey := os.Getenv("TYPESAFE_API_KEY")
	if apiKey == "" {
		t.Skip("TYPESAFE_API_KEY is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	response, err := NewClient(apiKey).SystemOne(ctx, Request{
		State: map[string]interface{}{
			"ticket": "The production deployment is returning 500 errors and customers cannot check out.",
		},
		Questions: map[string]Question{
			"urgent": Noul("Does this ticket need immediate attention?"),
			"team": Choice("Which team should handle this ticket?", map[string]interface{}{
				"billing":   "Invoices, payments, and refunds",
				"technical": "Bugs, outages, and production incidents",
				"general":   "General questions that do not fit another team",
			}),
			"severity": Score("How severe is the customer impact?", "low", "medium", "high"),
		},
	})
	require.NoError(t, err)
	require.Equal(t, QuestionTypeNoul, response.Answers["urgent"].Type)
	require.Equal(t, QuestionTypeChoice, response.Answers["team"].Type)
	require.Equal(t, QuestionTypeScore, response.Answers["severity"].Type)

	t.Logf(
		"model=%s urgent=%.3f team=%s confidence=%.3f severity=%.3f",
		response.Model,
		*response.Answers["urgent"].Noul,
		response.Answers["team"].Choice,
		response.Answers["team"].Confidence,
		*response.Answers["severity"].Score,
	)
}
