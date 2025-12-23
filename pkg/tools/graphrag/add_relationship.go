package graphrag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// AddRelationshipTool adds relationships between entities in the knowledge graph.
type AddRelationshipTool struct {
	store interfaces.GraphRAGStore
}

// NewAddRelationshipTool creates a new add relationship tool.
func NewAddRelationshipTool(store interfaces.GraphRAGStore) *AddRelationshipTool {
	return &AddRelationshipTool{store: store}
}

// Name returns the tool name.
func (t *AddRelationshipTool) Name() string {
	return "graphrag_add_relationship"
}

// Description returns the tool description.
func (t *AddRelationshipTool) Description() string {
	return "Add a relationship between two entities in the knowledge graph. Relationships " +
		"represent connections like 'WORKS_ON', 'MANAGES', 'BELONGS_TO', 'RELATED_TO'. " +
		"Both source and target entities must already exist in the graph."
}

// Parameters returns the tool parameters.
func (t *AddRelationshipTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"source_id": {
			Type:        "string",
			Description: "The ID of the source entity (the entity the relationship starts from)",
			Required:    true,
		},
		"target_id": {
			Type:        "string",
			Description: "The ID of the target entity (the entity the relationship points to)",
			Required:    true,
		},
		"type": {
			Type:        "string",
			Description: "The type of relationship (e.g., 'WORKS_ON', 'MANAGES', 'BELONGS_TO', 'COLLABORATES_WITH')",
			Required:    true,
		},
		"description": {
			Type:        "string",
			Description: "A description of the relationship providing context",
			Required:    false,
		},
		"strength": {
			Type:        "number",
			Description: "The strength of the relationship from 0.0 to 1.0 (default: 1.0). Higher values indicate stronger relationships.",
			Required:    false,
			Default:     1.0,
		},
		"properties": {
			Type:        "object",
			Description: "Additional properties for the relationship as key-value pairs",
			Required:    false,
		},
		"id": {
			Type:        "string",
			Description: "Optional custom ID for the relationship. If not provided, a UUID will be generated.",
			Required:    false,
		},
	}
}

// Run executes the tool (simple interface).
func (t *AddRelationshipTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute implements the tool interface with JSON arguments.
func (t *AddRelationshipTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		ID          string                 `json:"id"`
		SourceID    string                 `json:"source_id"`
		TargetID    string                 `json:"target_id"`
		Type        string                 `json:"type"`
		Description string                 `json:"description"`
		Strength    float32                `json:"strength"`
		Properties  map[string]interface{} `json:"properties"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Validate required fields
	if params.SourceID == "" {
		return "", fmt.Errorf("source_id is required")
	}
	if params.TargetID == "" {
		return "", fmt.Errorf("target_id is required")
	}
	if params.Type == "" {
		return "", fmt.Errorf("type is required")
	}

	// Normalize relationship type to uppercase
	params.Type = strings.ToUpper(strings.ReplaceAll(params.Type, " ", "_"))

	// Set defaults
	if params.Strength == 0 {
		params.Strength = 1.0
	}
	if params.Strength < 0 || params.Strength > 1 {
		return "", fmt.Errorf("strength must be between 0.0 and 1.0")
	}

	// Generate ID if not provided
	if params.ID == "" {
		params.ID = uuid.New().String()
	}

	relationship := interfaces.Relationship{
		ID:          params.ID,
		SourceID:    params.SourceID,
		TargetID:    params.TargetID,
		Type:        params.Type,
		Description: params.Description,
		Strength:    params.Strength,
		Properties:  params.Properties,
		CreatedAt:   time.Now(),
	}

	if err := t.store.StoreRelationships(ctx, []interfaces.Relationship{relationship}); err != nil {
		return "", fmt.Errorf("failed to store relationship: %w", err)
	}

	result := map[string]interface{}{
		"success":         true,
		"relationship_id": relationship.ID,
		"message": fmt.Sprintf("Relationship '%s' from '%s' to '%s' added successfully",
			relationship.Type, relationship.SourceID, relationship.TargetID),
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return string(output), nil
}
