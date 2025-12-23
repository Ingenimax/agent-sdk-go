package weaviate

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	"github.com/weaviate/weaviate/entities/models"

	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// StoreEntities stores multiple entities in the knowledge graph.
func (s *Store) StoreEntities(ctx context.Context, entities []interfaces.Entity, opts ...interfaces.GraphStoreOption) error {
	if len(entities) == 0 {
		return nil
	}

	options := applyStoreOptions(opts)
	className := s.getEntityClassName()

	// Get tenant from options, context, or store default
	tenant := options.Tenant
	if tenant == "" {
		if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
			tenant = orgID
		} else {
			tenant = s.tenant
		}
	}

	// Create batch
	batch := s.client.Batch().ObjectsBatcher()
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	batchCount := 0

	for i := range entities {
		entity := &entities[i]

		// Validate entity
		if entity.ID == "" {
			return fmt.Errorf("%w: entity at index %d", graphrag.ErrInvalidEntityID, i)
		}
		if entity.Name == "" {
			return fmt.Errorf("%w: entity %s", graphrag.ErrMissingEntityName, entity.ID)
		}
		if entity.Type == "" {
			return fmt.Errorf("%w: entity %s", graphrag.ErrMissingEntityType, entity.ID)
		}

		// Generate embedding if needed
		var vector []float32
		if options.GenerateEmbeddings && s.embedder != nil {
			// Use description for embedding, fall back to name if description is empty
			textToEmbed := entity.Description
			if textToEmbed == "" {
				textToEmbed = entity.Name
			}

			embedding, err := s.embedder.Embed(ctx, textToEmbed)
			if err != nil {
				return fmt.Errorf("failed to generate embedding for entity %s: %w", entity.ID, err)
			}
			vector = embedding
		} else if len(entity.Embedding) > 0 {
			vector = entity.Embedding
		}

		// Serialize properties to JSON
		propertiesJSON := ""
		if entity.Properties != nil {
			data, err := json.Marshal(entity.Properties)
			if err != nil {
				return fmt.Errorf("failed to serialize properties for entity %s: %w", entity.ID, err)
			}
			propertiesJSON = string(data)
		}

		// Set timestamps
		now := time.Now()
		if entity.CreatedAt.IsZero() {
			entity.CreatedAt = now
		}
		if entity.UpdatedAt.IsZero() {
			entity.UpdatedAt = now
		}

		// Create object properties
		props := map[string]interface{}{
			"entityId":    entity.ID,
			"name":        entity.Name,
			"entityType":  entity.Type,
			"description": entity.Description,
			"properties":  propertiesJSON,
			"orgId":       tenant,
			"createdAt":   entity.CreatedAt.Format(time.RFC3339),
			"updatedAt":   entity.UpdatedAt.Format(time.RFC3339),
		}

		// Create Weaviate object
		obj := &models.Object{
			Class:      className,
			Properties: props,
		}

		if len(vector) > 0 {
			obj.Vector = vector
		}

		batch.WithObjects(obj)
		batchCount++

		// Execute batch if size reached
		if batchCount >= batchSize {
			result, err := batch.Do(ctx)
			if err != nil {
				return fmt.Errorf("failed to store entity batch: %w", err)
			}

			// Check for individual object errors
			for _, res := range result {
				if res.Result != nil && res.Result.Errors != nil {
					for _, objErr := range res.Result.Errors.Error {
						s.logger.Error(ctx, "Failed to store entity", map[string]interface{}{
							"error": objErr.Message,
						})
					}
				}
			}

			// Reset batch
			batch = s.client.Batch().ObjectsBatcher()
			batchCount = 0
		}
	}

	// Store remaining entities
	if batchCount > 0 {
		result, err := batch.Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to store final entity batch: %w", err)
		}

		s.logger.Info(ctx, "Batch result received", map[string]interface{}{
			"resultCount": len(result),
			"batchCount":  batchCount,
		})

		// Check for individual object errors
		successCount := 0
		for _, res := range result {
			if res.Result != nil && res.Result.Errors != nil {
				for _, objErr := range res.Result.Errors.Error {
					s.logger.Error(ctx, "Failed to store entity", map[string]interface{}{
						"error": objErr.Message,
					})
				}
			} else {
				successCount++
			}
		}
		s.logger.Info(ctx, "Batch store completed", map[string]interface{}{
			"successCount": successCount,
			"totalCount":   len(result),
		})
	}

	s.logger.Info(ctx, "Stored entities", map[string]interface{}{
		"count":  len(entities),
		"tenant": tenant,
	})

	return nil
}

// GetEntity retrieves an entity by its ID.
func (s *Store) GetEntity(ctx context.Context, id string, opts ...interfaces.GraphStoreOption) (*interfaces.Entity, error) {
	if id == "" {
		return nil, graphrag.ErrInvalidEntityID
	}

	options := applyStoreOptions(opts)
	className := s.getEntityClassName()

	// Get tenant from options, context, or store default
	tenant := options.Tenant
	if tenant == "" {
		if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
			tenant = orgID
		} else {
			tenant = s.tenant
		}
	}

	s.logger.Info(ctx, "GetEntity starting", map[string]interface{}{
		"entityId":  id,
		"tenant":    tenant,
		"className": className,
	})

	// Build filter for entityId and optionally orgId
	filter := buildEntityIDFilter(id, tenant)

	// Query for the entity
	queryBuilder := s.client.GraphQL().Get().
		WithClassName(className).
		WithFields(
			graphql.Field{Name: "entityId"},
			graphql.Field{Name: "name"},
			graphql.Field{Name: "entityType"},
			graphql.Field{Name: "description"},
			graphql.Field{Name: "properties"},
			graphql.Field{Name: "orgId"},
			graphql.Field{Name: "createdAt"},
			graphql.Field{Name: "updatedAt"},
			graphql.Field{Name: "_additional", Fields: []graphql.Field{
				{Name: "id"},
				{Name: "vector"},
			}},
		).
		WithLimit(1)

	if filter != nil {
		queryBuilder = queryBuilder.WithWhere(filter)
	}

	result, err := queryBuilder.Do(ctx)

	if err != nil {
		s.logger.Error(ctx, "GetEntity query failed", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to query entity: %w", err)
	}

	// Log raw result
	if result.Data != nil {
		if getData, ok := result.Data["Get"].(map[string]interface{}); ok {
			if classData, ok := getData[className].([]interface{}); ok {
				s.logger.Info(ctx, "GetEntity raw result", map[string]interface{}{
					"resultCount": len(classData),
				})
			} else {
				s.logger.Info(ctx, "GetEntity no class data found", map[string]interface{}{
					"className": className,
					"getData":   getData,
				})
			}
		}
	}

	// Parse result
	entities := parseEntityResults(result, className)
	if len(entities) == 0 {
		s.logger.Warn(ctx, "GetEntity returned 0 entities", map[string]interface{}{
			"entityId": id,
			"tenant":   tenant,
		})
		return nil, graphrag.ErrEntityNotFound
	}

	s.logger.Info(ctx, "GetEntity found entity", map[string]interface{}{
		"entityId": entities[0].ID,
		"name":     entities[0].Name,
		"orgId":    entities[0].OrgID,
	})

	return &entities[0], nil
}

// UpdateEntity updates an existing entity.
func (s *Store) UpdateEntity(ctx context.Context, entity interfaces.Entity, opts ...interfaces.GraphStoreOption) error {
	if entity.ID == "" {
		return graphrag.ErrInvalidEntityID
	}

	options := applyStoreOptions(opts)
	className := s.getEntityClassName()

	// Get tenant from options, context, or store default
	tenant := options.Tenant
	if tenant == "" {
		if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
			tenant = orgID
		} else {
			tenant = s.tenant
		}
	}

	// First, find the Weaviate UUID for this entity
	filter := buildEntityIDFilter(entity.ID, tenant)

	queryBuilder := s.client.GraphQL().Get().
		WithClassName(className).
		WithFields(
			graphql.Field{Name: "_additional", Fields: []graphql.Field{
				{Name: "id"},
			}},
		).
		WithLimit(1)

	if filter != nil {
		queryBuilder = queryBuilder.WithWhere(filter)
	}

	result, err := queryBuilder.Do(ctx)

	if err != nil {
		return fmt.Errorf("failed to find entity: %w", err)
	}

	// Extract UUID
	uuid := extractUUID(result, className)
	if uuid == "" {
		return graphrag.ErrEntityNotFound
	}

	// Generate new embedding if needed
	var vector []float32
	if options.GenerateEmbeddings && s.embedder != nil {
		textToEmbed := entity.Description
		if textToEmbed == "" {
			textToEmbed = entity.Name
		}

		embedding, err := s.embedder.Embed(ctx, textToEmbed)
		if err != nil {
			return fmt.Errorf("failed to generate embedding: %w", err)
		}
		vector = embedding
	} else if len(entity.Embedding) > 0 {
		vector = entity.Embedding
	}

	// Serialize properties
	propertiesJSON := ""
	if entity.Properties != nil {
		data, err := json.Marshal(entity.Properties)
		if err != nil {
			return fmt.Errorf("failed to serialize properties: %w", err)
		}
		propertiesJSON = string(data)
	}

	// Update timestamp
	entity.UpdatedAt = time.Now()

	// Update the entity
	props := map[string]interface{}{
		"entityId":    entity.ID,
		"name":        entity.Name,
		"entityType":  entity.Type,
		"description": entity.Description,
		"properties":  propertiesJSON,
		"orgId":       tenant,
		"updatedAt":   entity.UpdatedAt.Format(time.RFC3339),
	}

	err = s.client.Data().Updater().
		WithClassName(className).
		WithID(uuid).
		WithProperties(props).
		WithVector(vector).
		Do(ctx)

	if err != nil {
		return fmt.Errorf("failed to update entity: %w", err)
	}

	s.logger.Info(ctx, "Updated entity", map[string]interface{}{
		"entityId": entity.ID,
		"tenant":   tenant,
	})

	return nil
}

// DeleteEntity deletes an entity by its ID.
func (s *Store) DeleteEntity(ctx context.Context, id string, opts ...interfaces.GraphStoreOption) error {
	if id == "" {
		return graphrag.ErrInvalidEntityID
	}

	options := applyStoreOptions(opts)
	className := s.getEntityClassName()

	// Get tenant from options, context, or store default
	tenant := options.Tenant
	if tenant == "" {
		if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
			tenant = orgID
		} else {
			tenant = s.tenant
		}
	}

	// Find the Weaviate UUID for this entity
	filter := buildEntityIDFilter(id, tenant)

	queryBuilder := s.client.GraphQL().Get().
		WithClassName(className).
		WithFields(
			graphql.Field{Name: "_additional", Fields: []graphql.Field{
				{Name: "id"},
			}},
		).
		WithLimit(1)

	if filter != nil {
		queryBuilder = queryBuilder.WithWhere(filter)
	}

	result, err := queryBuilder.Do(ctx)

	if err != nil {
		return fmt.Errorf("failed to find entity: %w", err)
	}

	// Extract UUID
	uuid := extractUUID(result, className)
	if uuid == "" {
		return graphrag.ErrEntityNotFound
	}

	// Delete the entity
	err = s.client.Data().Deleter().
		WithClassName(className).
		WithID(uuid).
		Do(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete entity: %w", err)
	}

	s.logger.Info(ctx, "Deleted entity", map[string]interface{}{
		"entityId": id,
		"tenant":   tenant,
	})

	return nil
}

// buildEntityIDFilter creates a Weaviate filter for entity ID and optional tenant.
func buildEntityIDFilter(entityID, tenant string) *filters.WhereBuilder {
	entityFilter := filters.Where().
		WithPath([]string{"entityId"}).
		WithOperator(filters.Equal).
		WithValueString(entityID)

	if tenant != "" {
		tenantFilter := filters.Where().
			WithPath([]string{"orgId"}).
			WithOperator(filters.Equal).
			WithValueString(tenant)

		return filters.Where().
			WithOperator(filters.And).
			WithOperands([]*filters.WhereBuilder{entityFilter, tenantFilter})
	}

	return entityFilter
}

// extractUUID extracts the Weaviate UUID from a GraphQL response.
func extractUUID(result *models.GraphQLResponse, className string) string {
	if result.Data == nil {
		return ""
	}

	getData, ok := result.Data["Get"].(map[string]interface{})
	if !ok {
		return ""
	}

	classData, ok := getData[className].([]interface{})
	if !ok || len(classData) == 0 {
		return ""
	}

	firstItem, ok := classData[0].(map[string]interface{})
	if !ok {
		return ""
	}

	additional, ok := firstItem["_additional"].(map[string]interface{})
	if !ok {
		return ""
	}

	id, ok := additional["id"].(string)
	if !ok {
		return ""
	}

	return id
}

// parseEntityResults parses GraphQL response into Entity slice.
func parseEntityResults(result *models.GraphQLResponse, className string) []interfaces.Entity {
	entities := []interfaces.Entity{}

	if result.Data == nil {
		return entities
	}

	getData, ok := result.Data["Get"].(map[string]interface{})
	if !ok {
		return entities
	}

	classData, ok := getData[className].([]interface{})
	if !ok {
		return entities
	}

	for _, item := range classData {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		entity := interfaces.Entity{}

		if v, ok := itemMap["entityId"].(string); ok {
			entity.ID = v
		}
		if v, ok := itemMap["name"].(string); ok {
			entity.Name = v
		}
		if v, ok := itemMap["entityType"].(string); ok {
			entity.Type = v
		}
		if v, ok := itemMap["description"].(string); ok {
			entity.Description = v
		}
		if v, ok := itemMap["orgId"].(string); ok {
			entity.OrgID = v
		}

		// Parse properties from JSON
		if propsStr, ok := itemMap["properties"].(string); ok && propsStr != "" {
			var props map[string]interface{}
			if err := json.Unmarshal([]byte(propsStr), &props); err == nil {
				entity.Properties = props
			}
		}

		// Parse timestamps
		if v, ok := itemMap["createdAt"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				entity.CreatedAt = t
			}
		}
		if v, ok := itemMap["updatedAt"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				entity.UpdatedAt = t
			}
		}

		// Extract vector from _additional
		if additional, ok := itemMap["_additional"].(map[string]interface{}); ok {
			if vector, ok := additional["vector"].([]interface{}); ok {
				entity.Embedding = make([]float32, len(vector))
				for i, v := range vector {
					if f, ok := v.(float64); ok {
						entity.Embedding[i] = float32(f)
					}
				}
			}
		}

		entities = append(entities, entity)
	}

	return entities
}
