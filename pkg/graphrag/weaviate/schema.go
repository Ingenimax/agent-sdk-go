package weaviate

import (
	"context"
	"fmt"

	"github.com/weaviate/weaviate/entities/models"
)

// ensureSchema creates the entity and relationship collections if they don't exist.
func (s *Store) ensureSchema(ctx context.Context) error {
	// Create Entity collection
	if err := s.ensureEntitySchema(ctx); err != nil {
		return fmt.Errorf("failed to create entity schema: %w", err)
	}

	// Create Relationship collection
	if err := s.ensureRelationshipSchema(ctx); err != nil {
		return fmt.Errorf("failed to create relationship schema: %w", err)
	}

	return nil
}

// ensureEntitySchema creates the entity collection if it doesn't exist.
func (s *Store) ensureEntitySchema(ctx context.Context) error {
	className := s.getEntityClassName()

	// Check if class exists
	exists, err := s.classExists(ctx, className)
	if err != nil {
		return fmt.Errorf("failed to check if class exists: %w", err)
	}
	if exists {
		s.logger.Debug(ctx, "Entity class already exists", map[string]interface{}{
			"class": className,
		})
		return nil
	}

	class := &models.Class{
		Class:       className,
		Description: "Knowledge graph entities for GraphRAG",
		VectorIndexConfig: map[string]interface{}{
			"distance": "cosine",
		},
		Properties: []*models.Property{
			{
				Name:            "entityId",
				Description:     "Unique identifier for the entity",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(false),
			},
			{
				Name:            "name",
				Description:     "Human-readable name of the entity",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(true),
				Tokenization:    "word",
			},
			{
				Name:            "entityType",
				Description:     "Category of the entity (e.g., Person, Organization)",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(true),
			},
			{
				Name:            "description",
				Description:     "Detailed description of the entity (used for vectorization)",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(false),
				IndexSearchable: boolPtr(true),
				Tokenization:    "word",
			},
			{
				Name:            "properties",
				Description:     "Additional properties as JSON string",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(false),
				IndexSearchable: boolPtr(false),
			},
			{
				Name:            "orgId",
				Description:     "Organization ID for multi-tenancy",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(false),
			},
			{
				Name:            "createdAt",
				Description:     "Creation timestamp",
				DataType:        []string{"date"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(false),
			},
			{
				Name:            "updatedAt",
				Description:     "Last update timestamp",
				DataType:        []string{"date"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(false),
			},
		},
	}

	err = s.client.Schema().ClassCreator().WithClass(class).Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to create entity class: %w", err)
	}

	s.logger.Info(ctx, "Created entity class", map[string]interface{}{
		"class": className,
	})

	return nil
}

// ensureRelationshipSchema creates the relationship collection if it doesn't exist.
func (s *Store) ensureRelationshipSchema(ctx context.Context) error {
	className := s.getRelationshipClassName()

	// Check if class exists
	exists, err := s.classExists(ctx, className)
	if err != nil {
		return fmt.Errorf("failed to check if class exists: %w", err)
	}
	if exists {
		s.logger.Debug(ctx, "Relationship class already exists", map[string]interface{}{
			"class": className,
		})
		return nil
	}

	class := &models.Class{
		Class:       className,
		Description: "Knowledge graph relationships for GraphRAG",
		// Relationships don't need vector index by default
		VectorIndexConfig: map[string]interface{}{
			"skip": true,
		},
		Properties: []*models.Property{
			{
				Name:            "relationshipId",
				Description:     "Unique identifier for the relationship",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(false),
			},
			{
				Name:            "sourceId",
				Description:     "ID of the source entity",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(false),
			},
			{
				Name:            "targetId",
				Description:     "ID of the target entity",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(false),
			},
			{
				Name:            "relationshipType",
				Description:     "Type of relationship (e.g., WORKS_ON, MANAGES)",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(true),
			},
			{
				Name:            "description",
				Description:     "Description of the relationship",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(false),
				IndexSearchable: boolPtr(true),
			},
			{
				Name:            "strength",
				Description:     "Relationship strength (0.0 to 1.0)",
				DataType:        []string{"number"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(false),
			},
			{
				Name:            "properties",
				Description:     "Additional properties as JSON string",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(false),
				IndexSearchable: boolPtr(false),
			},
			{
				Name:            "orgId",
				Description:     "Organization ID for multi-tenancy",
				DataType:        []string{"text"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(false),
			},
			{
				Name:            "createdAt",
				Description:     "Creation timestamp",
				DataType:        []string{"date"},
				IndexFilterable: boolPtr(true),
				IndexSearchable: boolPtr(false),
			},
		},
	}

	err = s.client.Schema().ClassCreator().WithClass(class).Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to create relationship class: %w", err)
	}

	s.logger.Info(ctx, "Created relationship class", map[string]interface{}{
		"class": className,
	})

	return nil
}

// classExists checks if a class/collection already exists in Weaviate.
func (s *Store) classExists(ctx context.Context, className string) (bool, error) {
	schema, err := s.client.Schema().Getter().Do(ctx)
	if err != nil {
		return false, err
	}

	for _, class := range schema.Classes {
		if class.Class == className {
			return true, nil
		}
	}
	return false, nil
}

// DeleteSchema deletes the entity and relationship collections.
// Use with caution - this will delete all data!
func (s *Store) DeleteSchema(ctx context.Context) error {
	entityClass := s.getEntityClassName()
	relationshipClass := s.getRelationshipClassName()

	// Delete entity class
	exists, err := s.classExists(ctx, entityClass)
	if err != nil {
		return fmt.Errorf("failed to check entity class: %w", err)
	}
	if exists {
		if err := s.client.Schema().ClassDeleter().WithClassName(entityClass).Do(ctx); err != nil {
			return fmt.Errorf("failed to delete entity class: %w", err)
		}
		s.logger.Info(ctx, "Deleted entity class", map[string]interface{}{
			"class": entityClass,
		})
	}

	// Delete relationship class
	exists, err = s.classExists(ctx, relationshipClass)
	if err != nil {
		return fmt.Errorf("failed to check relationship class: %w", err)
	}
	if exists {
		if err := s.client.Schema().ClassDeleter().WithClassName(relationshipClass).Do(ctx); err != nil {
			return fmt.Errorf("failed to delete relationship class: %w", err)
		}
		s.logger.Info(ctx, "Deleted relationship class", map[string]interface{}{
			"class": relationshipClass,
		})
	}

	return nil
}

// Helper functions for creating pointers
func boolPtr(b bool) *bool {
	return &b
}
