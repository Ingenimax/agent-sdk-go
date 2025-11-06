package agent

import (
	"fmt"
)

// validateSubAgents checks for circular dependencies in sub-agents
func (a *Agent) validateSubAgents() error {
	visited := make(map[string]bool)
	recursionStack := make(map[string]bool)

	return a.checkCircularDependency(visited, recursionStack)
}

// checkCircularDependency performs DFS to detect circular dependencies
func (a *Agent) checkCircularDependency(visited, recursionStack map[string]bool) error {
	agentID := a.getUniqueID()

	// Mark current agent as visited and add to recursion stack
	visited[agentID] = true
	recursionStack[agentID] = true

	// Check all sub-agents
	for _, subAgent := range a.subAgents {
		subAgentID := subAgent.getUniqueID()

		// If sub-agent is in recursion stack, we have a circular dependency
		if recursionStack[subAgentID] {
			return fmt.Errorf("circular dependency detected: %s -> %s", a.name, subAgent.name)
		}

		// If sub-agent hasn't been visited, recursively check it
		if !visited[subAgentID] {
			if err := subAgent.checkCircularDependency(visited, recursionStack); err != nil {
				return err
			}
		}
	}

	// Remove from recursion stack before returning
	delete(recursionStack, agentID)

	return nil
}

// getUniqueID returns a unique identifier for the agent
func (a *Agent) getUniqueID() string {
	if a.name != "" {
		return a.name
	}
	// Use pointer address as fallback for unnamed agents
	return fmt.Sprintf("%p", a)
}

// HasSubAgent checks if a specific agent is a sub-agent
func (a *Agent) HasSubAgent(name string) bool {
	for _, subAgent := range a.subAgents {
		if subAgent.name == name {
			return true
		}
	}
	return false
}

// GetSubAgent retrieves a sub-agent by name
func (a *Agent) GetSubAgent(name string) (*Agent, bool) {
	for _, subAgent := range a.subAgents {
		if subAgent.name == name {
			return subAgent, true
		}
	}
	return nil, false
}

// validateAgentTree validates the entire agent tree for issues
func validateAgentTree(root *Agent, maxDepth int) error {
	// Check for circular dependencies
	if err := root.validateSubAgents(); err != nil {
		return err
	}

	// Check maximum depth
	depth := calculateMaxDepth(root, 0)
	if depth > maxDepth {
		return fmt.Errorf("agent tree depth %d exceeds maximum allowed depth %d", depth, maxDepth)
	}

	// Validate each agent has required components
	return validateAgentComponents(root)
}

// calculateMaxDepth calculates the maximum depth of the agent tree
func calculateMaxDepth(agent *Agent, currentDepth int) int {
	if len(agent.subAgents) == 0 {
		return currentDepth
	}

	maxDepth := currentDepth
	for _, subAgent := range agent.subAgents {
		depth := calculateMaxDepth(subAgent, currentDepth+1)
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return maxDepth
}

// validateAgentComponents ensures all agents have required components
func validateAgentComponents(agent *Agent) error {
	// Remote agents don't need a local LLM
	if !agent.isRemote {
		// Check if local agent has LLM (required for local agents)
		if agent.llm == nil {
			return fmt.Errorf("agent %s is missing required LLM", agent.name)
		}
	}

	// Validate name is set for better debugging
	if agent.name == "" {
		return fmt.Errorf("agent is missing a name, which is recommended for sub-agents")
	}

	// Recursively validate sub-agents
	for _, subAgent := range agent.subAgents {
		if err := validateAgentComponents(subAgent); err != nil {
			return err
		}
	}

	return nil
}
