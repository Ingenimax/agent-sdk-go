package weaviate

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// TraverseFrom performs a breadth-first traversal from a starting entity.
// Note: Weaviate doesn't support native graph traversal, so this requires
// multiple queries (one per hop level).
func (s *Store) TraverseFrom(ctx context.Context, entityID string, depth int, opts ...interfaces.GraphSearchOption) (*interfaces.GraphContext, error) {
	if entityID == "" {
		return nil, graphrag.ErrInvalidEntityID
	}

	if depth < 0 {
		return nil, graphrag.ErrInvalidDepth
	}
	if depth == 0 {
		depth = 2 // Default depth
	}
	if depth > 5 {
		return nil, graphrag.ErrMaxDepthExceeded
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

	result := &interfaces.GraphContext{
		Depth:         depth,
		Entities:      []interfaces.Entity{},
		Relationships: []interfaces.Relationship{},
	}

	visited := make(map[string]bool)
	currentLevel := []string{entityID}

	for d := 0; d <= depth && len(currentLevel) > 0; d++ {
		nextLevel := []string{}

		for _, id := range currentLevel {
			if visited[id] {
				continue
			}
			visited[id] = true

			// Get the entity
			storeOpts := []interfaces.GraphStoreOption{}
			if tenant != "" {
				storeOpts = append(storeOpts, interfaces.WithGraphTenant(tenant))
			}

			entity, err := s.GetEntity(ctx, id, storeOpts...)
			if err != nil {
				s.logger.Debug(ctx, "Entity not found during traversal", map[string]interface{}{
					"entityId": id,
					"error":    err.Error(),
				})
				continue
			}

			// Set central entity for the first entity
			if d == 0 {
				result.CentralEntity = *entity
			}
			result.Entities = append(result.Entities, *entity)

			// Get outgoing relationships (we traverse in both directions for completeness)
			searchOpts := []interfaces.GraphSearchOption{}
			if tenant != "" {
				searchOpts = append(searchOpts, interfaces.WithSearchTenant(tenant))
			}
			if len(options.RelationshipTypes) > 0 {
				searchOpts = append(searchOpts, interfaces.WithRelationshipTypes(options.RelationshipTypes...))
			}

			outgoing, err := s.GetRelationships(ctx, id, interfaces.DirectionOutgoing, searchOpts...)
			if err != nil {
				s.logger.Debug(ctx, "Failed to get outgoing relationships", map[string]interface{}{
					"entityId": id,
					"error":    err.Error(),
				})
			} else {
				for _, rel := range outgoing {
					result.Relationships = append(result.Relationships, rel)
					if !visited[rel.TargetID] {
						nextLevel = append(nextLevel, rel.TargetID)
					}
				}
			}

			// Also get incoming relationships for complete graph context
			incoming, err := s.GetRelationships(ctx, id, interfaces.DirectionIncoming, searchOpts...)
			if err != nil {
				s.logger.Debug(ctx, "Failed to get incoming relationships", map[string]interface{}{
					"entityId": id,
					"error":    err.Error(),
				})
			} else {
				for _, rel := range incoming {
					// Avoid duplicates
					isDuplicate := false
					for _, existing := range result.Relationships {
						if existing.ID == rel.ID {
							isDuplicate = true
							break
						}
					}
					if !isDuplicate {
						result.Relationships = append(result.Relationships, rel)
						if !visited[rel.SourceID] {
							nextLevel = append(nextLevel, rel.SourceID)
						}
					}
				}
			}
		}

		currentLevel = nextLevel
	}

	// If no entities were found, return error
	if len(result.Entities) == 0 {
		return nil, graphrag.ErrEntityNotFound
	}

	s.logger.Debug(ctx, "Graph traversal completed", map[string]interface{}{
		"startEntityId": entityID,
		"depth":         depth,
		"entities":      len(result.Entities),
		"relationships": len(result.Relationships),
	})

	return result, nil
}

// ShortestPath finds the shortest path between two entities using BFS.
func (s *Store) ShortestPath(ctx context.Context, sourceID, targetID string, opts ...interfaces.GraphSearchOption) (*interfaces.GraphPath, error) {
	if sourceID == "" || targetID == "" {
		return nil, graphrag.ErrInvalidEntityID
	}

	if sourceID == targetID {
		// Same entity - return empty path
		entity, err := s.GetEntity(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		return &interfaces.GraphPath{
			Source:        *entity,
			Target:        *entity,
			Entities:      []interfaces.Entity{},
			Relationships: []interfaces.Relationship{},
			Length:        0,
		}, nil
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

	// BFS to find shortest path
	maxDepth := options.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 5 // Default max depth for path finding
	}

	type pathNode struct {
		entityID string
		path     []string
		rels     []string
	}

	visited := make(map[string]bool)
	queue := []pathNode{{entityID: sourceID, path: []string{sourceID}, rels: []string{}}}

	searchOpts := []interfaces.GraphSearchOption{}
	if tenant != "" {
		searchOpts = append(searchOpts, interfaces.WithSearchTenant(tenant))
	}
	if len(options.RelationshipTypes) > 0 {
		searchOpts = append(searchOpts, interfaces.WithRelationshipTypes(options.RelationshipTypes...))
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.entityID == targetID {
			// Found the path - build the result
			return s.buildPathResult(ctx, current.path, current.rels, tenant)
		}

		if visited[current.entityID] {
			continue
		}
		visited[current.entityID] = true

		// Check depth limit
		if len(current.path) > maxDepth {
			continue
		}

		// Get relationships
		rels, err := s.GetRelationships(ctx, current.entityID, interfaces.DirectionBoth, searchOpts...)
		if err != nil {
			continue
		}

		for _, rel := range rels {
			nextID := rel.TargetID
			if rel.TargetID == current.entityID {
				nextID = rel.SourceID
			}

			if !visited[nextID] {
				newPath := make([]string, len(current.path)+1)
				copy(newPath, current.path)
				newPath[len(current.path)] = nextID

				newRels := make([]string, len(current.rels)+1)
				copy(newRels, current.rels)
				newRels[len(current.rels)] = rel.ID

				queue = append(queue, pathNode{
					entityID: nextID,
					path:     newPath,
					rels:     newRels,
				})
			}
		}
	}

	return nil, graphrag.ErrPathNotFound
}

// buildPathResult constructs a GraphPath from entity and relationship IDs.
func (s *Store) buildPathResult(ctx context.Context, entityIDs []string, relIDs []string, tenant string) (*interfaces.GraphPath, error) {
	if len(entityIDs) < 2 {
		return nil, graphrag.ErrPathNotFound
	}

	storeOpts := []interfaces.GraphStoreOption{}
	if tenant != "" {
		storeOpts = append(storeOpts, interfaces.WithGraphTenant(tenant))
	}

	// Get source entity
	source, err := s.GetEntity(ctx, entityIDs[0], storeOpts...)
	if err != nil {
		return nil, err
	}

	// Get target entity
	target, err := s.GetEntity(ctx, entityIDs[len(entityIDs)-1], storeOpts...)
	if err != nil {
		return nil, err
	}

	// Get intermediate entities
	intermediate := []interfaces.Entity{}
	for i := 1; i < len(entityIDs)-1; i++ {
		entity, err := s.GetEntity(ctx, entityIDs[i], storeOpts...)
		if err != nil {
			s.logger.Warn(ctx, "Failed to get intermediate entity", map[string]interface{}{
				"entityId": entityIDs[i],
				"error":    err.Error(),
			})
			continue
		}
		intermediate = append(intermediate, *entity)
	}

	// Get relationships
	relationships := []interfaces.Relationship{}
	for _, relID := range relIDs {
		// We need to find the relationship by ID
		// This is a limitation - we'll try to find it from the stored relationships
		// For now, we'll create a placeholder
		rel := interfaces.Relationship{ID: relID}
		relationships = append(relationships, rel)
	}

	return &interfaces.GraphPath{
		Source:        *source,
		Target:        *target,
		Entities:      intermediate,
		Relationships: relationships,
		Length:        len(entityIDs) - 1,
	}, nil
}
