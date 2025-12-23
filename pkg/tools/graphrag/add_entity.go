package graphrag

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// AddEntityTool adds entities to the knowledge graph.
type AddEntityTool struct {
	store interfaces.GraphRAGStore
}

// NewAddEntityTool creates a new add entity tool.
func NewAddEntityTool(store interfaces.GraphRAGStore) *AddEntityTool {
	return &AddEntityTool{store: store}
}

// Name returns the tool name.
func (t *AddEntityTool) Name() string {
	return "graphrag_add_entity"
}

// Description returns the tool description.
func (t *AddEntityTool) Description() string {
	return "Add a new entity to the knowledge graph. Entities represent people, organizations, " +
		"projects, concepts, or other important objects. Use this to build the knowledge graph " +
		"with new information about entities."
}

// Parameters returns the tool parameters.
func (t *AddEntityTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"name": {
			Type:        "string",
			Description: "The name of the entity (e.g., 'John Smith', 'Project Alpha')",
			Required:    true,
		},
		"type": {
			Type:        "string",
			Description: "The type/category of the entity (e.g., 'Person', 'Organization', 'Project', 'Concept')",
			Required:    true,
		},
		"description": {
			Type:        "string",
			Description: "A detailed description of the entity that captures its key characteristics",
			Required:    true,
		},
		"properties": {
			Type:        "object",
			Description: "Additional properties for the entity as key-value pairs (e.g., {\"role\": \"Engineer\", \"department\": \"R&D\"})",
			Required:    false,
		},
		"id": {
			Type:        "string",
			Description: "Optional custom ID for the entity. If not provided, a UUID will be generated.",
			Required:    false,
		},
	}
}

// Run executes the tool (simple interface).
func (t *AddEntityTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute implements the tool interface with JSON arguments.
func (t *AddEntityTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		ID          string                 `json:"id"`
		Name        string                 `json:"name"`
		Type        string                 `json:"type"`
		Description string                 `json:"description"`
		Properties  map[string]interface{} `json:"properties"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Validate required fields
	if params.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if params.Type == "" {
		return "", fmt.Errorf("type is required")
	}
	if params.Description == "" {
		return "", fmt.Errorf("description is required")
	}

	// Generate ID if not provided
	if params.ID == "" {
		params.ID = uuid.New().String()
	}

	now := time.Now()
	entity := interfaces.Entity{
		ID:          params.ID,
		Name:        params.Name,
		Type:        params.Type,
		Description: params.Description,
		Properties:  params.Properties,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := t.store.StoreEntities(ctx, []interfaces.Entity{entity}); err != nil {
		return "", fmt.Errorf("failed to store entity: %w", err)
	}

	result := map[string]interface{}{
		"success":   true,
		"entity_id": entity.ID,
		"message":   fmt.Sprintf("Entity '%s' of type '%s' added successfully", entity.Name, entity.Type),
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return string(output), nil
}
