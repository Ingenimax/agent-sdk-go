package agent

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	graphragtools "github.com/Ingenimax/agent-sdk-go/pkg/tools/graphrag"
)

// WithGraphRAG adds GraphRAG capabilities to the agent.
// When a GraphRAGStore is provided, the agent automatically registers
// GraphRAG tools (search, add_entity, add_relationship, get_context, extract).
func WithGraphRAG(store interfaces.GraphRAGStore) Option {
	return func(a *Agent) {
		if store == nil {
			return
		}

		// Store reference for direct access
		a.graphRAGStore = store

		// Create and register GraphRAG tools
		tools := createGraphRAGTools(store, a.llm)
		a.tools = deduplicateTools(append(a.tools, tools...))

		if a.logger != nil {
			a.logger.Info(context.Background(), "GraphRAG enabled", map[string]interface{}{
				"tools": len(tools),
			})
		}
	}
}

// createGraphRAGTools creates the standard GraphRAG tools.
func createGraphRAGTools(store interfaces.GraphRAGStore, llm interfaces.LLM) []interfaces.Tool {
	tools := []interfaces.Tool{
		graphragtools.NewSearchTool(store),
		graphragtools.NewAddEntityTool(store),
		graphragtools.NewAddRelationshipTool(store),
		graphragtools.NewGetContextTool(store),
	}

	// Only add extract tool if LLM is available
	if llm != nil {
		tools = append(tools, graphragtools.NewExtractTool(store, llm))
	}

	return tools
}

// GetGraphRAGStore returns the GraphRAG store if configured.
// Returns nil if GraphRAG is not enabled.
func (a *Agent) GetGraphRAGStore() interfaces.GraphRAGStore {
	return a.graphRAGStore
}

// HasGraphRAG returns true if the agent has GraphRAG capabilities enabled.
func (a *Agent) HasGraphRAG() bool {
	return a.graphRAGStore != nil
}
