package orchestration

import (
	"context"
	"errors"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/jev"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeJevClient struct {
	response *jev.Response
	err      error
	request  jev.Request
}

func (f *fakeJevClient) SystemOne(_ context.Context, request jev.Request) (*jev.Response, error) {
	f.request = request
	return f.response, f.err
}

func TestJevRouterRouteDetailed(t *testing.T) {
	client := &fakeJevClient{response: &jev.Response{
		Model: "jev-1.13.0",
		Answers: map[string]jev.Answer{
			"route": {
				Type:          jev.QuestionTypeChoice,
				Choice:        "research",
				Confidence:    0.93,
				Probabilities: map[string]float64{"research": 0.93, "billing": 0.07},
			},
		},
		Usage: jev.Usage{InputTokens: 12},
	}}
	router, err := NewJevRouter(client, WithJevMinimumConfidence(0.8))
	require.NoError(t, err)

	decision, err := router.RouteDetailed(context.Background(), "Find the latest release", map[string]interface{}{
		"agents": map[string]string{
			"research": "Find current facts",
			"billing":  "Handle invoices",
		},
		"routing_state": map[string]interface{}{"customer_tier": "pro"},
	})

	require.NoError(t, err)
	assert.Equal(t, "research", decision.AgentID)
	assert.Equal(t, 0.93, decision.Confidence)
	assert.Equal(t, 12, decision.Usage.InputTokens)

	state := client.request.State.(map[string]interface{})
	assert.Equal(t, "Find the latest release", state["query"])
	assert.Contains(t, state, "context")
	assert.Contains(t, client.request.Questions, "route")
}

func TestJevRouterImplementsRouter(t *testing.T) {
	client := &fakeJevClient{response: &jev.Response{
		Answers: map[string]jev.Answer{
			"route": {
				Type:          jev.QuestionTypeChoice,
				Choice:        "a",
				Confidence:    0.75,
				Probabilities: map[string]float64{"a": 0.75, "b": 0.25},
			},
		},
	}}
	built, err := NewJevRouter(client)
	require.NoError(t, err)
	var router Router = built

	agentID, err := router.Route(context.Background(), "hello", map[string]interface{}{
		"agents": map[string]string{"a": "A", "b": "B"},
	})
	require.NoError(t, err)
	assert.Equal(t, "a", agentID)
}

func TestJevRouterRejectsLowConfidence(t *testing.T) {
	client := &fakeJevClient{response: &jev.Response{
		Answers: map[string]jev.Answer{
			"route": {
				Type:          jev.QuestionTypeChoice,
				Choice:        "a",
				Confidence:    0.6,
				Probabilities: map[string]float64{"a": 0.6, "b": 0.4},
			},
		},
	}}
	router, err := NewJevRouter(client, WithJevMinimumConfidence(0.8))
	require.NoError(t, err)
	_, err = router.Route(
		context.Background(), "hello", map[string]interface{}{
			"agents": map[string]string{"a": "A", "b": "B"},
		},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJevLowConfidence)
}

func TestJevRouterRejectsMissingAgents(t *testing.T) {
	router, err := NewJevRouter(&fakeJevClient{})
	require.NoError(t, err)
	_, err = router.Route(context.Background(), "hello", nil)
	assert.ErrorIs(t, err, ErrJevRouterAgents)
}

func TestJevRouterWrapsClientError(t *testing.T) {
	client := &fakeJevClient{err: errors.New("offline")}
	router, err := NewJevRouter(client)
	require.NoError(t, err)
	_, err = router.Route(context.Background(), "hello", map[string]interface{}{
		"agents": map[string]string{"a": "A", "b": "B"},
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "offline")
}

func TestNewJevRouterRejectsBadConfidence(t *testing.T) {
	_, err := NewJevRouter(&fakeJevClient{}, WithJevMinimumConfidence(5))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJevRouterConfidenceRange)

	_, err = NewJevRouter(&fakeJevClient{}, WithJevMinimumConfidence(-0.1))
	assert.ErrorIs(t, err, ErrJevRouterConfidenceRange)
}

func TestNewJevRouterRejectsNilClient(t *testing.T) {
	_, err := NewJevRouter(nil)
	require.Error(t, err)
}

func TestJevRouterNilLoggerKeepsDefault(t *testing.T) {
	client := &fakeJevClient{response: &jev.Response{
		Answers: map[string]jev.Answer{
			"route": {
				Type:          jev.QuestionTypeChoice,
				Choice:        "a",
				Confidence:    0.9,
				Probabilities: map[string]float64{"a": 0.9, "b": 0.1},
			},
		},
	}}
	router, err := NewJevRouter(client, WithJevRouterLogger(nil))
	require.NoError(t, err)

	// Logging happens only once routing succeeds, so this is the path a nil
	// logger used to panic on.
	agentID, err := router.Route(context.Background(), "hello", map[string]interface{}{
		"agents": map[string]string{"a": "A", "b": "B"},
	})
	require.NoError(t, err)
	assert.Equal(t, "a", agentID)
}

func TestJevRouterRejectsEmptyAgentID(t *testing.T) {
	router, err := NewJevRouter(&fakeJevClient{})
	require.NoError(t, err)
	_, err = router.Route(context.Background(), "hello", map[string]interface{}{
		"agents": map[string]string{"": "nameless", "b": "B"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJevRouterAgentID)
	assert.NotErrorIs(t, err, ErrJevRouterAgents)
}
