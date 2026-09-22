package orchestration

import (
	"context"
	"errors"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/jev"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

var (
	// ErrJevRouterAgents means the routing context omitted usable agent descriptions.
	ErrJevRouterAgents = errors.New("jev router: context must contain at least two agents")
	// ErrJevLowConfidence means Jev's choice did not reach the configured threshold.
	ErrJevLowConfidence = errors.New("jev router: confidence below threshold")
)

const defaultJevRouterInstructions = "Which specialized agent should handle this query?"

// JevSystemOne is the subset of the Jev client used by JevRouter.
// The interface keeps routing code straightforward to test with a fake client.
type JevSystemOne interface {
	SystemOne(ctx context.Context, request jev.Request) (*jev.Response, error)
}

// JevRoutingDecision exposes the calibrated data behind a routing choice.
type JevRoutingDecision struct {
	AgentID       string
	Confidence    float64
	Probabilities map[string]float64
	Model         string
	Usage         jev.Usage
}

// JevRouter routes requests with Jev instead of spending a generative LLM call.
type JevRouter struct {
	client            JevSystemOne
	minimumConfidence float64
	instructions      interface{}
	logger            logging.Logger
}

// JevRouterOption configures a JevRouter.
type JevRouterOption func(*JevRouter)

// WithJevMinimumConfidence rejects routing decisions below threshold.
func WithJevMinimumConfidence(threshold float64) JevRouterOption {
	return func(r *JevRouter) { r.minimumConfidence = threshold }
}

// WithJevRouterInstructions customizes the choice question sent to Jev.
func WithJevRouterInstructions(instructions interface{}) JevRouterOption {
	return func(r *JevRouter) { r.instructions = instructions }
}

// WithJevRouterLogger sets the router logger.
func WithJevRouterLogger(logger logging.Logger) JevRouterOption {
	return func(r *JevRouter) { r.logger = logger }
}

// NewJevRouter creates a Jev-backed orchestration Router.
func NewJevRouter(client JevSystemOne, options ...JevRouterOption) *JevRouter {
	router := &JevRouter{
		client:       client,
		instructions: defaultJevRouterInstructions,
		logger:       logging.New(),
	}
	for _, option := range options {
		option(router)
	}
	return router
}

// Route implements Router.
func (r *JevRouter) Route(ctx context.Context, query string, routingContext map[string]interface{}) (string, error) {
	decision, err := r.RouteDetailed(ctx, query, routingContext)
	if err != nil {
		return "", err
	}
	return decision.AgentID, nil
}

// RouteDetailed returns the selected agent and Jev's confidence metadata.
//
// routingContext must contain an "agents" map from agent ID to description. An
// optional "routing_state" value is included alongside the query as Jev state.
func (r *JevRouter) RouteDetailed(ctx context.Context, query string, routingContext map[string]interface{}) (*JevRoutingDecision, error) {
	if r.client == nil {
		return nil, errors.New("jev router: client is nil")
	}
	if r.minimumConfidence < 0 || r.minimumConfidence > 1 {
		return nil, fmt.Errorf("jev router: minimum confidence must be between 0 and 1")
	}

	agents, ok := routingContext["agents"].(map[string]string)
	if !ok || len(agents) < 2 {
		return nil, ErrJevRouterAgents
	}
	criteria := make(map[string]interface{}, len(agents))
	for id, description := range agents {
		if id == "" {
			return nil, fmt.Errorf("%w: agent IDs must not be empty", ErrJevRouterAgents)
		}
		criteria[id] = description
	}

	state := map[string]interface{}{"query": query}
	if extra, exists := routingContext["routing_state"]; exists {
		state["context"] = extra
	}

	response, err := r.client.SystemOne(ctx, jev.Request{
		State: state,
		Questions: map[string]jev.Question{
			"route": jev.Choice(r.instructions, criteria),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("jev router: classify request: %w", err)
	}
	if response == nil {
		return nil, errors.New("jev router: client returned a nil response")
	}
	answer, ok := response.Answers["route"]
	if !ok || answer.Type != jev.QuestionTypeChoice {
		return nil, fmt.Errorf("jev router: response did not include a choice answer named route")
	}
	if _, ok := agents[answer.Choice]; !ok {
		return nil, fmt.Errorf("jev router: selected unknown agent %q", answer.Choice)
	}
	if answer.Confidence < r.minimumConfidence {
		return nil, fmt.Errorf("%w: got %.3f, require %.3f", ErrJevLowConfidence, answer.Confidence, r.minimumConfidence)
	}

	decision := &JevRoutingDecision{
		AgentID:       answer.Choice,
		Confidence:    answer.Confidence,
		Probabilities: answer.Probabilities,
		Model:         response.Model,
		Usage:         response.Usage,
	}
	r.logger.Info(ctx, "Query routed by Jev", map[string]interface{}{
		"agent_id":   decision.AgentID,
		"confidence": decision.Confidence,
		"model":      decision.Model,
	})
	return decision, nil
}

var _ Router = (*JevRouter)(nil)
