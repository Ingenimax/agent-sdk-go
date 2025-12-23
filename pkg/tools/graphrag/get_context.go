package graphrag

import (
	"context"
	"encoding/json"
	"fmt"

	gr "github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// GetContextTool retrieves context around an entity via graph traversal.
type GetContextTool struct {
	store interfaces.GraphRAGStore
}

// NewGetContextTool creates a new get context tool.
func NewGetContextTool(store interfaces.GraphRAGStore) *GetContextTool {
	return &GetContextTool{store: store}
}

// Name returns the tool name.
func (t *GetContextTool) Name() string {
	return "graphrag_get_context"
}

// Description returns the tool description.
func (t *GetContextTool) Description() string {
	return "Get detailed context around a specific entity by traversing the knowledge graph. " +
		"Returns the entity along with all connected entities and relationships within the specified depth. " +
		"Use this to understand an entity's connections and relationships in the knowledge graph."
}

// Parameters returns the tool parameters.
func (t *GetContextTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"entity_id": {
			Type:        "string",
			Description: "The ID of the entity to get context for",
			Required:    true,
		},
		"depth": {
			Type:        "number",
			Description: "The depth of graph traversal (1-5, default: 2). Higher values return more connected entities.",
			Required:    false,
			Default:     2,
		},
		"relationship_types": {
			Type:        "array",
			Description: "Filter to only include specific relationship types (e.g., ['WORKS_ON', 'MANAGES'])",
			Required:    false,
			Items: &interfaces.ParameterSpec{
				Type: "string",
			},
		},
	}
}

// Run executes the tool (simple interface).
func (t *GetContextTool) Run(ctx context.Context, input string) (string, error) {
	// If input is just a plain string, treat it as the entity ID
	if !isJSON(input) {
		return t.Execute(ctx, fmt.Sprintf(`{"entity_id": %q}`, input))
	}
	return t.Execute(ctx, input)
}

// Execute implements the tool interface with JSON arguments.
func (t *GetContextTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		EntityID          string   `json:"entity_id"`
		Depth             int      `json:"depth"`
		RelationshipTypes []string `json:"relationship_types"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if params.EntityID == "" {
		return "", fmt.Errorf("entity_id is required")
	}

	// Set defaults
	if params.Depth <= 0 {
		params.Depth = 2
	}
	if params.Depth > 5 {
		params.Depth = 5
	}

	// Build options
	opts := []interfaces.GraphSearchOption{}
	if len(params.RelationshipTypes) > 0 {
		opts = append(opts, gr.WithRelationshipTypes(params.RelationshipTypes...))
	}

	// Traverse from the entity
	graphContext, err := t.store.TraverseFrom(ctx, params.EntityID, params.Depth, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to get context: %w", err)
	}

	// Format output
	output := formatGraphContext(graphContext)
	return output, nil
}

// formatGraphContext formats a GraphContext as JSON.
func formatGraphContext(graphCtx *interfaces.GraphContext) string {
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

	type contextOutput struct {
		CentralEntity entityOutput         `json:"central_entity"`
		Depth         int                  `json:"depth"`
		Entities      []entityOutput       `json:"entities"`
		Relationships []relationshipOutput `json:"relationships"`
		Summary       string               `json:"summary"`
	}

	output := contextOutput{
		CentralEntity: entityOutput{
			ID:          graphCtx.CentralEntity.ID,
			Name:        graphCtx.CentralEntity.Name,
			Type:        graphCtx.CentralEntity.Type,
			Description: graphCtx.CentralEntity.Description,
			Properties:  graphCtx.CentralEntity.Properties,
		},
		Depth:         graphCtx.Depth,
		Entities:      make([]entityOutput, 0, len(graphCtx.Entities)),
		Relationships: make([]relationshipOutput, 0, len(graphCtx.Relationships)),
	}

	// Add entities (excluding central entity)
	for _, e := range graphCtx.Entities {
		if e.ID == graphCtx.CentralEntity.ID {
			continue
		}
		output.Entities = append(output.Entities, entityOutput{
			ID:          e.ID,
			Name:        e.Name,
			Type:        e.Type,
			Description: e.Description,
			Properties:  e.Properties,
		})
	}

	// Add relationships
	for _, r := range graphCtx.Relationships {
		output.Relationships = append(output.Relationships, relationshipOutput{
			ID:          r.ID,
			SourceID:    r.SourceID,
			TargetID:    r.TargetID,
			Type:        r.Type,
			Description: r.Description,
			Strength:    r.Strength,
		})
	}

	// Generate summary
	output.Summary = fmt.Sprintf("Entity '%s' (%s) has %d connected entities and %d relationships within depth %d.",
		graphCtx.CentralEntity.Name,
		graphCtx.CentralEntity.Type,
		len(output.Entities),
		len(output.Relationships),
		graphCtx.Depth,
	)

	data, _ := json.MarshalIndent(output, "", "  ")
	return string(data)
}
