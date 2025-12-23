// Package graphrag provides tools for interacting with GraphRAG knowledge graphs.
package graphrag

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// SearchTool implements graph-based search in the knowledge graph.
type SearchTool struct {
	store interfaces.GraphRAGStore
}

// NewSearchTool creates a new search tool.
func NewSearchTool(store interfaces.GraphRAGStore) *SearchTool {
	return &SearchTool{store: store}
}

// Name returns the tool name.
func (t *SearchTool) Name() string {
	return "graphrag_search"
}

// Description returns the tool description.
func (t *SearchTool) Description() string {
	return "Search the knowledge graph for entities and relationships matching a query. " +
		"Supports local search (entity-focused with graph traversal) and global search (community-focused). " +
		"Use this to find information about people, organizations, concepts, and their relationships."
}

// Parameters returns the tool parameters.
func (t *SearchTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: "The search query to find relevant entities and relationships",
			Required:    true,
		},
		"search_type": {
			Type:        "string",
			Description: "Type of search: 'local' (entity-focused with graph context), 'global' (across all communities), or 'hybrid' (combined vector and keyword)",
			Required:    false,
			Default:     "hybrid",
			Enum:        []interface{}{"local", "global", "hybrid"},
		},
		"limit": {
			Type:        "number",
			Description: "Maximum number of results to return (default: 10)",
			Required:    false,
			Default:     10,
		},
		"entity_types": {
			Type:        "array",
			Description: "Filter results by specific entity types (e.g., ['Person', 'Project'])",
			Required:    false,
			Items: &interfaces.ParameterSpec{
				Type: "string",
			},
		},
		"depth": {
			Type:        "number",
			Description: "For local search, the depth of graph traversal (default: 2, max: 5)",
			Required:    false,
			Default:     2,
		},
	}
}

// Run executes the search (simple interface).
func (t *SearchTool) Run(ctx context.Context, input string) (string, error) {
	// If input is just a plain string, treat it as the query
	if !isJSON(input) {
		return t.Execute(ctx, fmt.Sprintf(`{"query": %q}`, input))
	}
	return t.Execute(ctx, input)
}

// Execute implements the tool interface with JSON arguments.
func (t *SearchTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Query       string   `json:"query"`
		SearchType  string   `json:"search_type"`
		Limit       int      `json:"limit"`
		EntityTypes []string `json:"entity_types"`
		Depth       int      `json:"depth"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if params.Query == "" {
		return "", fmt.Errorf("query parameter is required")
	}

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = 10
	}
	if params.Depth <= 0 {
		params.Depth = 2
	}
	if params.SearchType == "" {
		params.SearchType = "hybrid"
	}

	// Build search options
	opts := []interfaces.GraphSearchOption{
		graphrag.WithIncludeRelationships(true),
	}
	if len(params.EntityTypes) > 0 {
		opts = append(opts, graphrag.WithEntityTypes(params.EntityTypes...))
	}

	// Debug: Log the tenant being used for search
	if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
		log.Printf("[graphrag_search] Using tenant from context: %q", orgID)
	} else {
		log.Printf("[graphrag_search] No tenant in context, will use store default")
	}
	log.Printf("[graphrag_search] Query: %q, SearchType: %s, Limit: %d", params.Query, params.SearchType, params.Limit)

	var results []interfaces.GraphSearchResult
	var err error

	switch params.SearchType {
	case "local":
		// Local search with graph traversal
		opts = append(opts, graphrag.WithMaxDepth(params.Depth))
		results, err = t.store.LocalSearch(ctx, params.Query, "", params.Depth, opts...)
	case "global":
		// Global community-based search
		results, err = t.store.GlobalSearch(ctx, params.Query, 1, opts...)
	default: // hybrid
		// Standard hybrid search
		opts = append(opts, graphrag.WithMode(graphrag.SearchModeHybrid))
		results, err = t.store.Search(ctx, params.Query, params.Limit, opts...)
	}

	if err != nil {
		log.Printf("[graphrag_search] Search failed: %v", err)
		return "", fmt.Errorf("search failed: %w", err)
	}

	log.Printf("[graphrag_search] Found %d results", len(results))
	for i, r := range results {
		log.Printf("[graphrag_search] Result %d: ID=%s, Name=%s, Type=%s, OrgID=%s",
			i, r.Entity.ID, r.Entity.Name, r.Entity.Type, r.Entity.OrgID)
	}

	// Format output
	output := formatSearchResults(results)
	return output, nil
}

// formatSearchResults formats search results as JSON.
func formatSearchResults(results []interfaces.GraphSearchResult) string {
	type entityOutput struct {
		ID          string                 `json:"id"`
		Name        string                 `json:"name"`
		Type        string                 `json:"type"`
		Description string                 `json:"description,omitempty"`
		Properties  map[string]interface{} `json:"properties,omitempty"`
	}

	type relationshipOutput struct {
		ID          string  `json:"id"`
		SourceID    string  `json:"source_id"`
		TargetID    string  `json:"target_id"`
		Type        string  `json:"type"`
		Description string  `json:"description,omitempty"`
		Strength    float32 `json:"strength,omitempty"`
	}

	type resultOutput struct {
		Entity        entityOutput         `json:"entity"`
		Score         float32              `json:"score"`
		CommunityID   string               `json:"community_id,omitempty"`
		Context       []entityOutput       `json:"context,omitempty"`
		Relationships []relationshipOutput `json:"relationships,omitempty"`
	}

	output := make([]resultOutput, 0, len(results))

	for _, r := range results {
		result := resultOutput{
			Entity: entityOutput{
				ID:          r.Entity.ID,
				Name:        r.Entity.Name,
				Type:        r.Entity.Type,
				Description: r.Entity.Description,
				Properties:  r.Entity.Properties,
			},
			Score:       r.Score,
			CommunityID: r.CommunityID,
		}

		// Add context entities
		if len(r.Context) > 0 {
			result.Context = make([]entityOutput, 0, len(r.Context))
			for _, e := range r.Context {
				result.Context = append(result.Context, entityOutput{
					ID:          e.ID,
					Name:        e.Name,
					Type:        e.Type,
					Description: e.Description,
				})
			}
		}

		// Add relationships
		if len(r.Path) > 0 {
			result.Relationships = make([]relationshipOutput, 0, len(r.Path))
			for _, rel := range r.Path {
				result.Relationships = append(result.Relationships, relationshipOutput{
					ID:          rel.ID,
					SourceID:    rel.SourceID,
					TargetID:    rel.TargetID,
					Type:        rel.Type,
					Description: rel.Description,
					Strength:    rel.Strength,
				})
			}
		}

		output = append(output, result)
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	return string(data)
}

// isJSON checks if a string looks like JSON.
func isJSON(s string) bool {
	s = trimWhitespace(s)
	return (len(s) > 0 && (s[0] == '{' || s[0] == '['))
}

// trimWhitespace removes leading and trailing whitespace.
func trimWhitespace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
