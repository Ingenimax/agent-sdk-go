package weaviate

import (
	"context"
	"fmt"

	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	"github.com/weaviate/weaviate/entities/models"

	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Search performs a search on the knowledge graph.
// It supports vector, keyword, and hybrid search modes.
func (s *Store) Search(ctx context.Context, query string, limit int, opts ...interfaces.GraphSearchOption) ([]interfaces.GraphSearchResult, error) {
	if query == "" {
		return nil, graphrag.ErrEmptyQuery
	}

	if limit <= 0 {
		limit = 10
	}

	options := applySearchOptions(opts)

	// Get tenant from options, context, or store default
	tenant := options.Tenant
	if tenant == "" {
		if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
			tenant = orgID
		} else {
			tenant = s.tenant
		}
	}

	// Build the query based on search mode
	var results []interfaces.GraphSearchResult
	var err error

	switch options.SearchMode {
	case interfaces.SearchModeKeyword:
		results, err = s.keywordSearch(ctx, query, limit, tenant, options)
	case interfaces.SearchModeVector:
		results, err = s.vectorSearch(ctx, query, limit, tenant, options)
	default: // Hybrid
		results, err = s.hybridSearch(ctx, query, limit, tenant, options)
	}

	if err != nil {
		return nil, err
	}

	// Filter by minimum score
	if options.MinScore > 0 {
		filtered := make([]interfaces.GraphSearchResult, 0, len(results))
		for _, r := range results {
			if r.Score >= options.MinScore {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	s.logger.Debug(ctx, "Search completed", map[string]interface{}{
		"query":  query,
		"mode":   options.SearchMode,
		"count":  len(results),
		"tenant": tenant,
	})

	return results, nil
}

// LocalSearch performs a search starting from a specific entity and traversing the graph.
func (s *Store) LocalSearch(ctx context.Context, query string, entityID string, depth int, opts ...interfaces.GraphSearchOption) ([]interfaces.GraphSearchResult, error) {
	if query == "" {
		return nil, graphrag.ErrEmptyQuery
	}

	if depth < 0 {
		return nil, graphrag.ErrInvalidDepth
	}
	if depth == 0 {
		depth = 2 // Default depth
	}

	options := applySearchOptions(opts)

	// First, perform a regular search to find relevant entities
	searchResults, err := s.Search(ctx, query, 10, opts...)
	if err != nil {
		return nil, err
	}

	// If an entity ID is provided, get context from that entity
	if entityID != "" {
		graphContext, err := s.TraverseFrom(ctx, entityID, depth, opts...)
		if err != nil && err != graphrag.ErrEntityNotFound {
			return nil, err
		}

		if graphContext != nil {
			// Enrich search results with context
			for i := range searchResults {
				searchResults[i].Context = graphContext.Entities
				if options.IncludeRelationships {
					searchResults[i].Path = graphContext.Relationships
				}
			}
		}
	} else if len(searchResults) > 0 {
		// Use the top result as the starting point
		topEntityID := searchResults[0].Entity.ID
		graphContext, err := s.TraverseFrom(ctx, topEntityID, depth, opts...)
		if err != nil && err != graphrag.ErrEntityNotFound {
			s.logger.Warn(ctx, "Failed to get context for top result", map[string]interface{}{
				"entityId": topEntityID,
				"error":    err.Error(),
			})
		}

		if graphContext != nil {
			searchResults[0].Context = graphContext.Entities
			if options.IncludeRelationships {
				searchResults[0].Path = graphContext.Relationships
			}
		}
	}

	s.logger.Debug(ctx, "Local search completed", map[string]interface{}{
		"query":    query,
		"entityId": entityID,
		"depth":    depth,
		"count":    len(searchResults),
	})

	return searchResults, nil
}

// GlobalSearch performs a community-based search across the knowledge graph.
// It groups entities by type and searches across communities.
func (s *Store) GlobalSearch(ctx context.Context, query string, communityLevel int, opts ...interfaces.GraphSearchOption) ([]interfaces.GraphSearchResult, error) {
	if query == "" {
		return nil, graphrag.ErrEmptyQuery
	}

	options := applySearchOptions(opts)

	// Get tenant from options, context, or store default
	tenant := options.Tenant
	if tenant == "" {
		if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
			tenant = orgID
		} else {
			tenant = s.tenant
		}
	}

	// Get all entity types (communities)
	entityTypes, err := s.discoverEntityTypes(ctx)
	if err != nil {
		s.logger.Warn(ctx, "Failed to discover entity types, using search without community grouping", map[string]interface{}{
			"error": err.Error(),
		})
		return s.Search(ctx, query, 20, opts...)
	}

	// If specific entity types are requested, filter to those
	if len(options.EntityTypes) > 0 {
		filtered := []string{}
		typeSet := make(map[string]bool)
		for _, t := range options.EntityTypes {
			typeSet[t] = true
		}
		for _, t := range entityTypes {
			if typeSet[t] {
				filtered = append(filtered, t)
			}
		}
		entityTypes = filtered
	}

	// Search within each entity type (community)
	allResults := []interfaces.GraphSearchResult{}
	resultsPerCommunity := 5

	for _, entityType := range entityTypes {
		// Search within this entity type
		typeOpts := append(opts, interfaces.WithEntityTypes(entityType))
		results, err := s.Search(ctx, query, resultsPerCommunity, typeOpts...)
		if err != nil {
			s.logger.Warn(ctx, "Failed to search in entity type", map[string]interface{}{
				"entityType": entityType,
				"error":      err.Error(),
			})
			continue
		}

		// Add community ID to results
		for i := range results {
			results[i].CommunityID = entityType
		}

		allResults = append(allResults, results...)
	}

	// Sort by score (already sorted within each community, but need global sort)
	// Simple bubble sort since we expect small result sets
	for i := 0; i < len(allResults); i++ {
		for j := i + 1; j < len(allResults); j++ {
			if allResults[j].Score > allResults[i].Score {
				allResults[i], allResults[j] = allResults[j], allResults[i]
			}
		}
	}

	s.logger.Debug(ctx, "Global search completed", map[string]interface{}{
		"query":       query,
		"communities": len(entityTypes),
		"count":       len(allResults),
		"tenant":      tenant,
	})

	return allResults, nil
}

// vectorSearch performs a pure vector similarity search.
func (s *Store) vectorSearch(ctx context.Context, query string, limit int, tenant string, options *interfaces.GraphSearchOptions) ([]interfaces.GraphSearchResult, error) {
	if s.embedder == nil {
		return nil, graphrag.ErrNoEmbedder
	}

	className := s.getEntityClassName()

	// Generate query embedding
	queryVector, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Build filter
	filter := buildSearchFilter(tenant, options.EntityTypes)

	// Build query
	nearVector := s.client.GraphQL().NearVectorArgBuilder().
		WithVector(queryVector)

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
				{Name: "certainty"},
				{Name: "vector"},
			}},
		).
		WithNearVector(nearVector).
		WithLimit(limit)

	if filter != nil {
		queryBuilder = queryBuilder.WithWhere(filter)
	}

	result, err := queryBuilder.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to execute vector search: %w", err)
	}

	return parseSearchResults(result, className), nil
}

// keywordSearch performs a BM25 keyword search.
func (s *Store) keywordSearch(ctx context.Context, query string, limit int, tenant string, options *interfaces.GraphSearchOptions) ([]interfaces.GraphSearchResult, error) {
	className := s.getEntityClassName()

	// Build filter
	filter := buildSearchFilter(tenant, options.EntityTypes)

	// Build BM25 query
	bm25 := s.client.GraphQL().Bm25ArgBuilder().
		WithQuery(query).
		WithProperties("name", "description", "entityType")

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
				{Name: "score"},
			}},
		).
		WithBM25(bm25).
		WithLimit(limit)

	if filter != nil {
		queryBuilder = queryBuilder.WithWhere(filter)
	}

	result, err := queryBuilder.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to execute keyword search: %w", err)
	}

	return parseSearchResults(result, className), nil
}

// hybridSearch performs a hybrid search combining vector and keyword search.
func (s *Store) hybridSearch(ctx context.Context, query string, limit int, tenant string, options *interfaces.GraphSearchOptions) ([]interfaces.GraphSearchResult, error) {
	className := s.getEntityClassName()

	s.logger.Info(ctx, "hybridSearch starting", map[string]interface{}{
		"query":       query,
		"limit":       limit,
		"tenant":      tenant,
		"entityTypes": options.EntityTypes,
		"className":   className,
		"hasEmbedder": s.embedder != nil,
	})

	// If no embedder is available, fall back to keyword search
	// Hybrid search without vectors doesn't work properly in Weaviate
	if s.embedder == nil {
		s.logger.Info(ctx, "No embedder configured, falling back to keyword search", nil)
		return s.keywordSearch(ctx, query, limit, tenant, options)
	}

	// Generate query vector
	queryVector, err := s.embedder.Embed(ctx, query)
	if err != nil {
		s.logger.Warn(ctx, "Failed to generate embedding for hybrid search, falling back to keyword", map[string]interface{}{
			"error": err.Error(),
		})
		return s.keywordSearch(ctx, query, limit, tenant, options)
	}

	// Build filter
	filter := buildSearchFilter(tenant, options.EntityTypes)
	s.logger.Info(ctx, "hybridSearch filter built", map[string]interface{}{
		"hasFilter": filter != nil,
	})

	// Build hybrid query with vector
	hybridBuilder := s.client.GraphQL().HybridArgumentBuilder().
		WithQuery(query).
		WithAlpha(0.5). // Equal weight to vector and keyword
		WithVector(queryVector)

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
				{Name: "score"},
				{Name: "vector"},
			}},
		).
		WithHybrid(hybridBuilder).
		WithLimit(limit)

	if filter != nil {
		queryBuilder = queryBuilder.WithWhere(filter)
	}

	result, err := queryBuilder.Do(ctx)
	if err != nil {
		s.logger.Error(ctx, "hybridSearch failed", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to execute hybrid search: %w", err)
	}

	// Log raw result
	if result.Data != nil {
		if getData, ok := result.Data["Get"].(map[string]interface{}); ok {
			if classData, ok := getData[className].([]interface{}); ok {
				s.logger.Info(ctx, "hybridSearch raw result", map[string]interface{}{
					"resultCount": len(classData),
				})
			} else {
				s.logger.Info(ctx, "hybridSearch no class data found", map[string]interface{}{
					"className": className,
					"getData":   getData,
				})
			}
		} else {
			s.logger.Info(ctx, "hybridSearch no Get data", map[string]interface{}{
				"data": result.Data,
			})
		}
	} else {
		s.logger.Info(ctx, "hybridSearch result.Data is nil", nil)
	}

	return parseSearchResults(result, className), nil
}

// buildSearchFilter creates a Weaviate filter for search queries.
func buildSearchFilter(tenant string, entityTypes []string) *filters.WhereBuilder {
	filterList := []*filters.WhereBuilder{}

	// Add tenant filter
	if tenant != "" {
		tenantFilter := filters.Where().
			WithPath([]string{"orgId"}).
			WithOperator(filters.Equal).
			WithValueString(tenant)
		filterList = append(filterList, tenantFilter)
	}

	// Add entity type filter
	if len(entityTypes) > 0 {
		if len(entityTypes) == 1 {
			typeFilter := filters.Where().
				WithPath([]string{"entityType"}).
				WithOperator(filters.Equal).
				WithValueString(entityTypes[0])
			filterList = append(filterList, typeFilter)
		} else {
			typeFilters := make([]*filters.WhereBuilder, len(entityTypes))
			for i, et := range entityTypes {
				typeFilters[i] = filters.Where().
					WithPath([]string{"entityType"}).
					WithOperator(filters.Equal).
					WithValueString(et)
			}
			typeOrFilter := filters.Where().
				WithOperator(filters.Or).
				WithOperands(typeFilters)
			filterList = append(filterList, typeOrFilter)
		}
	}

	if len(filterList) == 0 {
		return nil
	}

	if len(filterList) == 1 {
		return filterList[0]
	}

	return filters.Where().
		WithOperator(filters.And).
		WithOperands(filterList)
}

// parseSearchResults parses GraphQL response into GraphSearchResult slice.
func parseSearchResults(result *models.GraphQLResponse, className string) []interfaces.GraphSearchResult {
	results := []interfaces.GraphSearchResult{}

	if result.Data == nil {
		return results
	}

	getData, ok := result.Data["Get"].(map[string]interface{})
	if !ok {
		return results
	}

	classData, ok := getData[className].([]interface{})
	if !ok {
		return results
	}

	for _, item := range classData {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Parse entity
		entities := parseEntityResults(result, className)
		if len(entities) == 0 {
			// Parse entity from itemMap directly
			entity := parseEntityFromMap(itemMap)
			if entity.ID == "" {
				continue
			}

			searchResult := interfaces.GraphSearchResult{
				Entity: entity,
			}

			// Extract score from _additional
			if additional, ok := itemMap["_additional"].(map[string]interface{}); ok {
				if certainty, ok := additional["certainty"].(float64); ok {
					searchResult.Score = float32(certainty)
				} else if score, ok := additional["score"].(float64); ok {
					// Normalize BM25 score to 0-1 range (approximate)
					searchResult.Score = float32(score / (score + 1))
				}
			}

			results = append(results, searchResult)
		}
	}

	// If we didn't get results from the loop, try parsing entities directly
	if len(results) == 0 {
		entities := parseEntityResults(result, className)
		for _, entity := range entities {
			results = append(results, interfaces.GraphSearchResult{
				Entity: entity,
				Score:  0.5, // Default score if not available
			})
		}
	}

	return results
}

// parseEntityFromMap parses an entity from a map (used in search results).
func parseEntityFromMap(itemMap map[string]interface{}) interfaces.Entity {
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

	return entity
}
