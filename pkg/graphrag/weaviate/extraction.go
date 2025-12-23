package weaviate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ExtractFromText extracts entities and relationships from text using an LLM.
func (s *Store) ExtractFromText(ctx context.Context, text string, llm interfaces.LLM, opts ...interfaces.ExtractionOption) (*interfaces.ExtractionResult, error) {
	if llm == nil {
		return nil, graphrag.ErrNoLLM
	}

	if text == "" {
		return &interfaces.ExtractionResult{
			Entities:      []interfaces.Entity{},
			Relationships: []interfaces.Relationship{},
			SourceText:    text,
			Confidence:    0,
		}, nil
	}

	options := applyExtractionOptions(opts)

	// Build the extraction prompt
	prompt := s.buildExtractionPrompt(text, options)

	// Call LLM to extract entities and relationships
	response, err := llm.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", graphrag.ErrExtractionFailed, err)
	}

	// Parse the LLM response
	result, err := s.parseExtractionResponse(response, text, options)
	if err != nil {
		s.logger.Warn(ctx, "Failed to parse extraction response, attempting fallback", map[string]interface{}{
			"error": err.Error(),
		})
		// Try to extract what we can
		result = &interfaces.ExtractionResult{
			Entities:      []interfaces.Entity{},
			Relationships: []interfaces.Relationship{},
			SourceText:    text,
			Confidence:    0.3,
		}
	}

	// Deduplicate entities if embedder is available and threshold is set
	if s.embedder != nil && options.DedupThreshold > 0 && len(result.Entities) > 1 {
		deduped, err := s.deduplicateEntities(ctx, result.Entities, options.DedupThreshold)
		if err != nil {
			s.logger.Warn(ctx, "Failed to deduplicate entities", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			result.Entities = deduped
		}
	}

	// Apply entity limit
	if options.MaxEntities > 0 && len(result.Entities) > options.MaxEntities {
		result.Entities = result.Entities[:options.MaxEntities]
	}

	s.logger.Info(ctx, "Extracted entities and relationships", map[string]interface{}{
		"entities":      len(result.Entities),
		"relationships": len(result.Relationships),
		"confidence":    result.Confidence,
	})

	return result, nil
}

// buildExtractionPrompt constructs the prompt for entity/relationship extraction.
func (s *Store) buildExtractionPrompt(text string, options *interfaces.ExtractionOptions) string {
	var sb strings.Builder

	sb.WriteString("Extract entities and relationships from the following text.\n\n")

	if options.SchemaGuided && s.schema != nil {
		sb.WriteString("Use the following schema:\n\n")
		sb.WriteString("Entity Types:\n")
		for _, et := range s.schema.EntityTypes {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", et.Name, et.Description))
		}
		sb.WriteString("\nRelationship Types:\n")
		for _, rt := range s.schema.RelationshipTypes {
			sb.WriteString(fmt.Sprintf("- %s: %s (from %v to %v)\n", rt.Name, rt.Description, rt.SourceTypes, rt.TargetTypes))
		}
		sb.WriteString("\n")
	} else if len(options.EntityTypes) > 0 || len(options.RelationshipTypes) > 0 {
		if len(options.EntityTypes) > 0 {
			sb.WriteString(fmt.Sprintf("Focus on entity types: %s\n", strings.Join(options.EntityTypes, ", ")))
		}
		if len(options.RelationshipTypes) > 0 {
			sb.WriteString(fmt.Sprintf("Focus on relationship types: %s\n", strings.Join(options.RelationshipTypes, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`Respond with a JSON object containing:
{
  "entities": [
    {
      "name": "entity name",
      "type": "entity type",
      "description": "brief description of the entity"
    }
  ],
  "relationships": [
    {
      "source": "source entity name",
      "target": "target entity name",
      "type": "RELATIONSHIP_TYPE",
      "description": "brief description of the relationship"
    }
  ],
  "confidence": 0.8
}

Text to analyze:
`)
	sb.WriteString(text)
	sb.WriteString("\n\nJSON response:")

	return sb.String()
}

// parseExtractionResponse parses the LLM response into an ExtractionResult.
func (s *Store) parseExtractionResponse(response, sourceText string, options *interfaces.ExtractionOptions) (*interfaces.ExtractionResult, error) {
	// Try to extract JSON from the response
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	var parsed struct {
		Entities []struct {
			Name        string                 `json:"name"`
			Type        string                 `json:"type"`
			Description string                 `json:"description"`
			Properties  map[string]interface{} `json:"properties,omitempty"`
		} `json:"entities"`
		Relationships []struct {
			Source      string                 `json:"source"`
			Target      string                 `json:"target"`
			Type        string                 `json:"type"`
			Description string                 `json:"description"`
			Strength    float32                `json:"strength,omitempty"`
			Properties  map[string]interface{} `json:"properties,omitempty"`
		} `json:"relationships"`
		Confidence float32 `json:"confidence"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Convert to entities
	now := time.Now()
	entityNameToID := make(map[string]string)
	entities := make([]interfaces.Entity, 0, len(parsed.Entities))

	for _, e := range parsed.Entities {
		if e.Name == "" {
			continue
		}

		// Filter by min confidence if specified
		if options.MinConfidence > 0 && parsed.Confidence < options.MinConfidence {
			continue
		}

		// Filter by entity types if specified
		if len(options.EntityTypes) > 0 {
			found := false
			for _, t := range options.EntityTypes {
				if strings.EqualFold(e.Type, t) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		id := uuid.New().String()
		entityNameToID[e.Name] = id

		entity := interfaces.Entity{
			ID:          id,
			Name:        e.Name,
			Type:        e.Type,
			Description: e.Description,
			Properties:  e.Properties,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		entities = append(entities, entity)
	}

	// Convert to relationships
	relationships := make([]interfaces.Relationship, 0, len(parsed.Relationships))

	for _, r := range parsed.Relationships {
		if r.Source == "" || r.Target == "" || r.Type == "" {
			continue
		}

		// Filter by relationship types if specified
		if len(options.RelationshipTypes) > 0 {
			found := false
			for _, t := range options.RelationshipTypes {
				if strings.EqualFold(r.Type, t) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		sourceID, ok := entityNameToID[r.Source]
		if !ok {
			// Source entity not found, skip
			continue
		}

		targetID, ok := entityNameToID[r.Target]
		if !ok {
			// Target entity not found, skip
			continue
		}

		strength := r.Strength
		if strength == 0 {
			strength = 1.0
		}

		rel := interfaces.Relationship{
			ID:          uuid.New().String(),
			SourceID:    sourceID,
			TargetID:    targetID,
			Type:        strings.ToUpper(r.Type),
			Description: r.Description,
			Strength:    strength,
			Properties:  r.Properties,
			CreatedAt:   now,
		}
		relationships = append(relationships, rel)
	}

	confidence := parsed.Confidence
	if confidence == 0 {
		confidence = 0.7 // Default confidence if not provided
	}

	return &interfaces.ExtractionResult{
		Entities:      entities,
		Relationships: relationships,
		SourceText:    sourceText,
		Confidence:    confidence,
	}, nil
}

// extractJSON extracts JSON content from a text response.
func extractJSON(text string) string {
	// Try to find JSON object in the response
	start := strings.Index(text, "{")
	if start == -1 {
		return ""
	}

	// Find matching closing brace
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}

	return ""
}

// deduplicateEntities removes duplicate entities using embedding similarity.
func (s *Store) deduplicateEntities(ctx context.Context, entities []interfaces.Entity, threshold float32) ([]interfaces.Entity, error) {
	if len(entities) <= 1 {
		return entities, nil
	}

	// Generate embeddings for all entities
	texts := make([]string, len(entities))
	for i, e := range entities {
		texts[i] = e.Name + " " + e.Description
	}

	embeddings, err := s.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embeddings: %w", err)
	}

	// Deduplicate using cosine similarity
	unique := []interfaces.Entity{}
	uniqueEmbeddings := [][]float32{}

	for i, entity := range entities {
		isDuplicate := false

		for j, existing := range uniqueEmbeddings {
			similarity, err := s.embedder.CalculateSimilarity(embeddings[i], existing, "cosine")
			if err != nil {
				continue
			}

			if similarity >= threshold {
				// Merge: prefer longer names and combine properties
				if len(entity.Name) > len(unique[j].Name) {
					unique[j].Name = entity.Name
				}
				if len(entity.Description) > len(unique[j].Description) {
					unique[j].Description = entity.Description
				}
				// Merge properties
				if entity.Properties != nil {
					if unique[j].Properties == nil {
						unique[j].Properties = make(map[string]interface{})
					}
					for k, v := range entity.Properties {
						if _, exists := unique[j].Properties[k]; !exists {
							unique[j].Properties[k] = v
						}
					}
				}
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			unique = append(unique, entity)
			uniqueEmbeddings = append(uniqueEmbeddings, embeddings[i])
		}
	}

	return unique, nil
}
