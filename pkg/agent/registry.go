package agent

import (
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Registry manages a collection of agents
type Registry struct {
	agents map[string]interfaces.Agent
}

// NewRegistry creates a new agent registry
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]interfaces.Agent),
	}
}

// Register registers an agent with the given ID
func (r *Registry) Register(id string, agent interfaces.Agent) {
	r.agents[id] = agent
}

// Get retrieves an agent by ID
func (r *Registry) Get(id string) (interfaces.Agent, bool) {
	agent, exists := r.agents[id]
	return agent, exists
}

// Unregister removes an agent from the registry
func (r *Registry) Unregister(id string) {
	delete(r.agents, id)
}

// List returns all registered agent IDs
func (r *Registry) List() []string {
	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	return ids
}
