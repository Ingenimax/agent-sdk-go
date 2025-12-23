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

// StoreRelationships stores multiple relationships in the knowledge graph.
func (s *Store) StoreRelationships(ctx context.Context, relationships []interfaces.Relationship, opts ...interfaces.GraphStoreOption) error {
	if len(relationships) == 0 {
		return nil
	}

	options := applyStoreOptions(opts)
	className := s.getRelationshipClassName()

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

	for i := range relationships {
		rel := &relationships[i]

		// Validate relationship
		if rel.ID == "" {
			return fmt.Errorf("%w: relationship at index %d", graphrag.ErrInvalidRelationshipID, i)
		}
		if rel.SourceID == "" {
			return fmt.Errorf("%w: relationship %s", graphrag.ErrMissingSourceID, rel.ID)
		}
		if rel.TargetID == "" {
			return fmt.Errorf("%w: relationship %s", graphrag.ErrMissingTargetID, rel.ID)
		}
		if rel.Type == "" {
			return fmt.Errorf("%w: relationship %s", graphrag.ErrMissingRelationshipType, rel.ID)
		}
		if rel.Strength < 0 || rel.Strength > 1 {
			return fmt.Errorf("%w: relationship %s has strength %.2f", graphrag.ErrInvalidStrength, rel.ID, rel.Strength)
		}

		// Default strength to 1.0 if not set
		strength := rel.Strength
		if strength == 0 {
			strength = 1.0
		}

		// Serialize properties to JSON
		propertiesJSON := ""
		if rel.Properties != nil {
			data, err := json.Marshal(rel.Properties)
			if err != nil {
				return fmt.Errorf("failed to serialize properties for relationship %s: %w", rel.ID, err)
			}
			propertiesJSON = string(data)
		}

		// Set timestamp
		if rel.CreatedAt.IsZero() {
			rel.CreatedAt = time.Now()
		}

		// Create object properties
		props := map[string]interface{}{
			"relationshipId":   rel.ID,
			"sourceId":         rel.SourceID,
			"targetId":         rel.TargetID,
			"relationshipType": rel.Type,
			"description":      rel.Description,
			"strength":         strength,
			"properties":       propertiesJSON,
			"orgId":            tenant,
			"createdAt":        rel.CreatedAt.Format(time.RFC3339),
		}

		// Create Weaviate object
		obj := &models.Object{
			Class:      className,
			Properties: props,
		}

		batch.WithObjects(obj)
		batchCount++

		// Execute batch if size reached
		if batchCount >= batchSize {
			result, err := batch.Do(ctx)
			if err != nil {
				return fmt.Errorf("failed to store relationship batch: %w", err)
			}

			// Check for individual object errors
			for _, res := range result {
				if res.Result != nil && res.Result.Errors != nil {
					for _, objErr := range res.Result.Errors.Error {
						s.logger.Error(ctx, "Failed to store relationship", map[string]interface{}{
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

	// Store remaining relationships
	if batchCount > 0 {
		result, err := batch.Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to store final relationship batch: %w", err)
		}

		// Check for individual object errors
		for _, res := range result {
			if res.Result != nil && res.Result.Errors != nil {
				for _, objErr := range res.Result.Errors.Error {
					s.logger.Error(ctx, "Failed to store relationship", map[string]interface{}{
						"error": objErr.Message,
					})
				}
			}
		}
	}

	s.logger.Info(ctx, "Stored relationships", map[string]interface{}{
		"count":  len(relationships),
		"tenant": tenant,
	})

	return nil
}

// GetRelationships retrieves relationships for an entity based on direction.
func (s *Store) GetRelationships(ctx context.Context, entityID string, direction interfaces.RelationshipDirection, opts ...interfaces.GraphSearchOption) ([]interfaces.Relationship, error) {
	if entityID == "" {
		return nil, graphrag.ErrInvalidEntityID
	}

	options := applySearchOptions(opts)
	className := s.getRelationshipClassName()

	// Get tenant from options, context, or store default
	tenant := options.Tenant
	if tenant == "" {
		if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
			tenant = orgID
		} else {
			tenant = s.tenant
		}
	}

	// Build filter based on direction
	filter := buildRelationshipDirectionFilter(entityID, direction, tenant, options.RelationshipTypes)

	// Query for relationships
	queryBuilder := s.client.GraphQL().Get().
		WithClassName(className).
		WithFields(
			graphql.Field{Name: "relationshipId"},
			graphql.Field{Name: "sourceId"},
			graphql.Field{Name: "targetId"},
			graphql.Field{Name: "relationshipType"},
			graphql.Field{Name: "description"},
			graphql.Field{Name: "strength"},
			graphql.Field{Name: "properties"},
			graphql.Field{Name: "orgId"},
			graphql.Field{Name: "createdAt"},
		)

	if filter != nil {
		queryBuilder = queryBuilder.WithWhere(filter)
	}

	result, err := queryBuilder.Do(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query relationships: %w", err)
	}

	relationships := parseRelationshipResults(result, className)

	s.logger.Debug(ctx, "Retrieved relationships", map[string]interface{}{
		"entityId":  entityID,
		"direction": direction,
		"count":     len(relationships),
	})

	return relationships, nil
}

// DeleteRelationship deletes a relationship by its ID.
func (s *Store) DeleteRelationship(ctx context.Context, id string, opts ...interfaces.GraphStoreOption) error {
	if id == "" {
		return graphrag.ErrInvalidRelationshipID
	}

	options := applyStoreOptions(opts)
	className := s.getRelationshipClassName()

	// Get tenant from options, context, or store default
	tenant := options.Tenant
	if tenant == "" {
		if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
			tenant = orgID
		} else {
			tenant = s.tenant
		}
	}

	// Find the Weaviate UUID for this relationship
	filter := buildRelationshipIDFilter(id, tenant)

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
		return fmt.Errorf("failed to find relationship: %w", err)
	}

	// Extract UUID
	uuid := extractUUID(result, className)
	if uuid == "" {
		return graphrag.ErrRelationshipNotFound
	}

	// Delete the relationship
	err = s.client.Data().Deleter().
		WithClassName(className).
		WithID(uuid).
		Do(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete relationship: %w", err)
	}

	s.logger.Info(ctx, "Deleted relationship", map[string]interface{}{
		"relationshipId": id,
		"tenant":         tenant,
	})

	return nil
}

// buildRelationshipIDFilter creates a Weaviate filter for relationship ID and optional tenant.
func buildRelationshipIDFilter(relationshipID, tenant string) *filters.WhereBuilder {
	relFilter := filters.Where().
		WithPath([]string{"relationshipId"}).
		WithOperator(filters.Equal).
		WithValueString(relationshipID)

	if tenant != "" {
		tenantFilter := filters.Where().
			WithPath([]string{"orgId"}).
			WithOperator(filters.Equal).
			WithValueString(tenant)

		return filters.Where().
			WithOperator(filters.And).
			WithOperands([]*filters.WhereBuilder{relFilter, tenantFilter})
	}

	return relFilter
}

// buildRelationshipDirectionFilter creates a filter for relationship queries based on direction.
func buildRelationshipDirectionFilter(entityID string, direction interfaces.RelationshipDirection, tenant string, relTypes []string) *filters.WhereBuilder {
	var directionFilter *filters.WhereBuilder

	switch direction {
	case interfaces.DirectionOutgoing:
		directionFilter = filters.Where().
			WithPath([]string{"sourceId"}).
			WithOperator(filters.Equal).
			WithValueString(entityID)
	case interfaces.DirectionIncoming:
		directionFilter = filters.Where().
			WithPath([]string{"targetId"}).
			WithOperator(filters.Equal).
			WithValueString(entityID)
	default: // DirectionBoth
		sourceFilter := filters.Where().
			WithPath([]string{"sourceId"}).
			WithOperator(filters.Equal).
			WithValueString(entityID)
		targetFilter := filters.Where().
			WithPath([]string{"targetId"}).
			WithOperator(filters.Equal).
			WithValueString(entityID)
		directionFilter = filters.Where().
			WithOperator(filters.Or).
			WithOperands([]*filters.WhereBuilder{sourceFilter, targetFilter})
	}

	filterList := []*filters.WhereBuilder{directionFilter}

	// Add tenant filter if specified
	if tenant != "" {
		tenantFilter := filters.Where().
			WithPath([]string{"orgId"}).
			WithOperator(filters.Equal).
			WithValueString(tenant)
		filterList = append(filterList, tenantFilter)
	}

	// Add relationship type filter if specified
	if len(relTypes) > 0 {
		if len(relTypes) == 1 {
			typeFilter := filters.Where().
				WithPath([]string{"relationshipType"}).
				WithOperator(filters.Equal).
				WithValueString(relTypes[0])
			filterList = append(filterList, typeFilter)
		} else {
			typeFilters := make([]*filters.WhereBuilder, len(relTypes))
			for i, rt := range relTypes {
				typeFilters[i] = filters.Where().
					WithPath([]string{"relationshipType"}).
					WithOperator(filters.Equal).
					WithValueString(rt)
			}
			typeOrFilter := filters.Where().
				WithOperator(filters.Or).
				WithOperands(typeFilters)
			filterList = append(filterList, typeOrFilter)
		}
	}

	// Combine all filters with AND
	if len(filterList) == 1 {
		return filterList[0]
	}

	return filters.Where().
		WithOperator(filters.And).
		WithOperands(filterList)
}

// parseRelationshipResults parses GraphQL response into Relationship slice.
func parseRelationshipResults(result *models.GraphQLResponse, className string) []interfaces.Relationship {
	relationships := []interfaces.Relationship{}

	if result.Data == nil {
		return relationships
	}

	getData, ok := result.Data["Get"].(map[string]interface{})
	if !ok {
		return relationships
	}

	classData, ok := getData[className].([]interface{})
	if !ok {
		return relationships
	}

	for _, item := range classData {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		rel := interfaces.Relationship{}

		if v, ok := itemMap["relationshipId"].(string); ok {
			rel.ID = v
		}
		if v, ok := itemMap["sourceId"].(string); ok {
			rel.SourceID = v
		}
		if v, ok := itemMap["targetId"].(string); ok {
			rel.TargetID = v
		}
		if v, ok := itemMap["relationshipType"].(string); ok {
			rel.Type = v
		}
		if v, ok := itemMap["description"].(string); ok {
			rel.Description = v
		}
		if v, ok := itemMap["strength"].(float64); ok {
			rel.Strength = float32(v)
		}
		if v, ok := itemMap["orgId"].(string); ok {
			rel.OrgID = v
		}

		// Parse properties from JSON
		if propsStr, ok := itemMap["properties"].(string); ok && propsStr != "" {
			var props map[string]interface{}
			if err := json.Unmarshal([]byte(propsStr), &props); err == nil {
				rel.Properties = props
			}
		}

		// Parse timestamp
		if v, ok := itemMap["createdAt"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				rel.CreatedAt = t
			}
		}

		relationships = append(relationships, rel)
	}

	return relationships
}
