package graphrag

import (
	"context"
	"encoding/json"
	"fmt"

	gr "github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ExtractTool extracts entities and relationships from text using an LLM.
type ExtractTool struct {
	store interfaces.GraphRAGStore
	llm   interfaces.LLM
}

// NewExtractTool creates a new extract tool.
func NewExtractTool(store interfaces.GraphRAGStore, llm interfaces.LLM) *ExtractTool {
	return &ExtractTool{
		store: store,
		llm:   llm,
	}
}

// Name returns the tool name.
func (t *ExtractTool) Name() string {
	return "graphrag_extract"
}

// Description returns the tool description.
func (t *ExtractTool) Description() string {
	return "Extract entities and relationships from text and optionally add them to the knowledge graph. " +
		"Uses AI to identify people, organizations, concepts, and their relationships from unstructured text. " +
		"Use this to automatically build the knowledge graph from documents, notes, or other text."
}

// Parameters returns the tool parameters.
func (t *ExtractTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"text": {
			Type:        "string",
			Description: "The text to extract entities and relationships from",
			Required:    true,
		},
		"store_results": {
			Type:        "boolean",
			Description: "Whether to automatically store the extracted entities and relationships in the graph (default: false)",
			Required:    false,
			Default:     false,
		},
		"schema_guided": {
			Type:        "boolean",
			Description: "Whether to use schema-guided extraction for more consistent results (default: true)",
			Required:    false,
			Default:     true,
		},
		"entity_types": {
			Type:        "array",
			Description: "Limit extraction to specific entity types (e.g., ['Person', 'Project'])",
			Required:    false,
			Items: &interfaces.ParameterSpec{
				Type: "string",
			},
		},
		"relationship_types": {
			Type:        "array",
			Description: "Limit extraction to specific relationship types (e.g., ['WORKS_ON', 'MANAGES'])",
			Required:    false,
			Items: &interfaces.ParameterSpec{
				Type: "string",
			},
		},
	}
}

// Run executes the tool (simple interface).
func (t *ExtractTool) Run(ctx context.Context, input string) (string, error) {
	// If input is just a plain string, treat it as the text to extract
	if !isJSON(input) {
		return t.Execute(ctx, fmt.Sprintf(`{"text": %q}`, input))
	}
	return t.Execute(ctx, input)
}

// Execute implements the tool interface with JSON arguments.
func (t *ExtractTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Text              string   `json:"text"`
		StoreResults      bool     `json:"store_results"`
		SchemaGuided      bool     `json:"schema_guided"`
		EntityTypes       []string `json:"entity_types"`
		RelationshipTypes []string `json:"relationship_types"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if params.Text == "" {
		return "", fmt.Errorf("text is required")
	}

	// Build extraction options
	opts := []interfaces.ExtractionOption{
		gr.WithSchemaGuided(params.SchemaGuided),
	}
	if len(params.EntityTypes) > 0 {
		opts = append(opts, gr.WithExtractionEntityTypes(params.EntityTypes...))
	}
	if len(params.RelationshipTypes) > 0 {
		opts = append(opts, gr.WithExtractionRelationshipTypes(params.RelationshipTypes...))
	}

	// Extract entities and relationships
	result, err := t.store.ExtractFromText(ctx, params.Text, t.llm, opts...)
	if err != nil {
		return "", fmt.Errorf("extraction failed: %w", err)
	}

	// Optionally store the results
	stored := false
	if params.StoreResults && len(result.Entities) > 0 {
		if err := t.store.StoreEntities(ctx, result.Entities); err != nil {
			return "", fmt.Errorf("failed to store entities: %w", err)
		}
		if len(result.Relationships) > 0 {
			if err := t.store.StoreRelationships(ctx, result.Relationships); err != nil {
				return "", fmt.Errorf("failed to store relationships: %w", err)
			}
		}
		stored = true
	}

	// Format output
	output := formatExtractionResult(result, stored)
	return output, nil
}

// formatExtractionResult formats an ExtractionResult as JSON.
func formatExtractionResult(result *interfaces.ExtractionResult, stored bool) string {
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

	type extractionOutput struct {
		Entities      []entityOutput       `json:"entities"`
		Relationships []relationshipOutput `json:"relationships"`
		Confidence    float32              `json:"confidence"`
		Stored        bool                 `json:"stored"`
		Summary       string               `json:"summary"`
	}

	output := extractionOutput{
		Entities:      make([]entityOutput, 0, len(result.Entities)),
		Relationships: make([]relationshipOutput, 0, len(result.Relationships)),
		Confidence:    result.Confidence,
		Stored:        stored,
	}

	// Add entities
	for _, e := range result.Entities {
		output.Entities = append(output.Entities, entityOutput{
			ID:          e.ID,
			Name:        e.Name,
			Type:        e.Type,
			Description: e.Description,
			Properties:  e.Properties,
		})
	}

	// Add relationships
	for _, r := range result.Relationships {
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
	storedText := ""
	if stored {
		storedText = " and stored in the knowledge graph"
	}
	output.Summary = fmt.Sprintf("Extracted %d entities and %d relationships with %.0f%% confidence%s.",
		len(output.Entities),
		len(output.Relationships),
		output.Confidence*100,
		storedText,
	)

	data, _ := json.MarshalIndent(output, "", "  ")
	return string(data)
}
