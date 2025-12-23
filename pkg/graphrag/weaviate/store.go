// Package weaviate provides a Weaviate-based implementation of the GraphRAG interface.
//
// This implementation stores entities and relationships in separate Weaviate collections,
// using vector embeddings for semantic search and metadata filtering for graph traversal.
package weaviate

import (
	"context"
	"fmt"

	"github.com/weaviate/weaviate-go-client/v5/weaviate"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/auth"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"

	"github.com/Ingenimax/agent-sdk-go/pkg/embedding"
	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// Store implements the GraphRAGStore interface using Weaviate as the backend.
type Store struct {
	client      *weaviate.Client
	classPrefix string
	embedder    embedding.Client
	logger      logging.Logger
	tenant      string
	schema      *interfaces.GraphSchema
}

// Config holds configuration for the Weaviate GraphRAG store.
type Config struct {
	// Host is the hostname of the Weaviate server (e.g., "localhost:8080")
	Host string

	// Scheme is the URL scheme ("http" or "https")
	Scheme string

	// APIKey is the authentication key for Weaviate Cloud
	APIKey string

	// ClassPrefix is the prefix for entity/relationship collections (default: "Graph")
	ClassPrefix string
}

// Option represents an option for configuring the Store.
type Option func(*Store)

// WithClassPrefix sets the class prefix for entity/relationship collections.
func WithClassPrefix(prefix string) Option {
	return func(s *Store) {
		s.classPrefix = prefix
	}
}

// WithEmbedder sets the embedder for generating vectors.
func WithEmbedder(embedder embedding.Client) Option {
	return func(s *Store) {
		s.embedder = embedder
	}
}

// WithLogger sets the logger for the store.
func WithLogger(logger logging.Logger) Option {
	return func(s *Store) {
		s.logger = logger
	}
}

// WithStoreTenant sets the default tenant.
func WithStoreTenant(tenant string) Option {
	return func(s *Store) {
		s.tenant = tenant
	}
}

// WithSchema sets an initial schema for the store.
func WithSchema(schema *interfaces.GraphSchema) Option {
	return func(s *Store) {
		s.schema = schema
	}
}

// New creates a new Weaviate GraphRAG store.
func New(config *Config, options ...Option) (*Store, error) {
	if config == nil {
		config = &Config{
			Host:        "localhost:8080",
			Scheme:      "http",
			ClassPrefix: "Graph",
		}
	}

	store := &Store{
		classPrefix: "Graph",
		logger:      logging.New(),
	}

	// Override classPrefix from config if provided
	if config.ClassPrefix != "" {
		store.classPrefix = config.ClassPrefix
	}

	// Apply options
	for _, option := range options {
		option(store)
	}

	// Create Weaviate client configuration
	cfg := weaviate.Config{
		Host:   config.Host,
		Scheme: config.Scheme,
	}

	// Add API key authentication if provided
	if config.APIKey != "" {
		cfg.AuthConfig = auth.ApiKey{Value: config.APIKey}
	}

	// Create Weaviate client
	client, err := weaviate.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", graphrag.ErrConnectionFailed, err)
	}

	store.client = client

	// Ensure schema exists
	if err := store.ensureSchema(context.Background()); err != nil {
		store.logger.Warn(context.Background(), "Failed to ensure schema, collections may need to be created manually", map[string]interface{}{
			"error": err.Error(),
		})
	}

	return store, nil
}

// getEntityClassName returns the entity collection name.
func (s *Store) getEntityClassName() string {
	return s.classPrefix + "Entity"
}

// getRelationshipClassName returns the relationship collection name.
func (s *Store) getRelationshipClassName() string {
	return s.classPrefix + "Relationship"
}

// SetTenant sets the current tenant for multi-tenancy operations.
func (s *Store) SetTenant(tenant string) {
	s.tenant = tenant
}

// GetTenant returns the current tenant.
func (s *Store) GetTenant() string {
	return s.tenant
}

// Close closes the store connection.
// Note: Weaviate client doesn't require explicit closing.
func (s *Store) Close() error {
	return nil
}

// ApplySchema stores the schema definition for use in extraction and validation.
func (s *Store) ApplySchema(ctx context.Context, schema interfaces.GraphSchema) error {
	s.schema = &schema
	s.logger.Info(ctx, "Applied graph schema", map[string]interface{}{
		"entity_types":       len(schema.EntityTypes),
		"relationship_types": len(schema.RelationshipTypes),
	})
	return nil
}

// DiscoverSchema infers schema from existing data in the graph.
func (s *Store) DiscoverSchema(ctx context.Context) (*interfaces.GraphSchema, error) {
	// If we have a stored schema, return it
	if s.schema != nil {
		return s.schema, nil
	}

	schema := &interfaces.GraphSchema{
		EntityTypes:       []interfaces.EntityTypeSchema{},
		RelationshipTypes: []interfaces.RelationshipTypeSchema{},
	}

	// Discover entity types by querying unique entityType values
	entityTypes, err := s.discoverEntityTypes(ctx)
	if err != nil {
		s.logger.Warn(ctx, "Failed to discover entity types", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		for _, et := range entityTypes {
			schema.EntityTypes = append(schema.EntityTypes, interfaces.EntityTypeSchema{
				Name:        et,
				Description: fmt.Sprintf("Discovered entity type: %s", et),
			})
		}
	}

	// Discover relationship types
	relTypes, err := s.discoverRelationshipTypes(ctx)
	if err != nil {
		s.logger.Warn(ctx, "Failed to discover relationship types", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		for _, rt := range relTypes {
			schema.RelationshipTypes = append(schema.RelationshipTypes, interfaces.RelationshipTypeSchema{
				Name:        rt,
				Description: fmt.Sprintf("Discovered relationship type: %s", rt),
			})
		}
	}

	return schema, nil
}

// discoverEntityTypes queries for unique entity types in the collection.
func (s *Store) discoverEntityTypes(ctx context.Context) ([]string, error) {
	className := s.getEntityClassName()

	// Use aggregate query to get unique entity types
	result, err := s.client.GraphQL().Aggregate().
		WithClassName(className).
		WithFields(
			graphql.Field{Name: "entityType", Fields: []graphql.Field{{Name: "count"}}},
		).
		WithGroupBy("entityType").
		Do(ctx)

	if err != nil {
		return nil, err
	}

	types := []string{}

	// Parse the aggregate response
	if result.Data != nil {
		if aggData, ok := result.Data["Aggregate"].(map[string]interface{}); ok {
			if classData, ok := aggData[className].([]interface{}); ok {
				for _, item := range classData {
					if group, ok := item.(map[string]interface{}); ok {
						if groupedBy, ok := group["groupedBy"].(map[string]interface{}); ok {
							if value, ok := groupedBy["value"].(string); ok {
								types = append(types, value)
							}
						}
					}
				}
			}
		}
	}

	return types, nil
}

// discoverRelationshipTypes queries for unique relationship types in the collection.
func (s *Store) discoverRelationshipTypes(ctx context.Context) ([]string, error) {
	className := s.getRelationshipClassName()

	// Use aggregate query to get unique relationship types
	result, err := s.client.GraphQL().Aggregate().
		WithClassName(className).
		WithFields(
			graphql.Field{Name: "relationshipType", Fields: []graphql.Field{{Name: "count"}}},
		).
		WithGroupBy("relationshipType").
		Do(ctx)

	if err != nil {
		return nil, err
	}

	types := []string{}

	// Parse the aggregate response
	if result.Data != nil {
		if aggData, ok := result.Data["Aggregate"].(map[string]interface{}); ok {
			if classData, ok := aggData[className].([]interface{}); ok {
				for _, item := range classData {
					if group, ok := item.(map[string]interface{}); ok {
						if groupedBy, ok := group["groupedBy"].(map[string]interface{}); ok {
							if value, ok := groupedBy["value"].(string); ok {
								types = append(types, value)
							}
						}
					}
				}
			}
		}
	}

	return types, nil
}

// applyStoreOptions applies GraphStoreOptions and returns the effective options.
func applyStoreOptions(opts []interfaces.GraphStoreOption) *interfaces.GraphStoreOptions {
	options := &interfaces.GraphStoreOptions{
		BatchSize:          100, // Default batch size
		GenerateEmbeddings: true,
	}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

// applySearchOptions applies GraphSearchOptions and returns the effective options.
func applySearchOptions(opts []interfaces.GraphSearchOption) *interfaces.GraphSearchOptions {
	options := &interfaces.GraphSearchOptions{
		MinScore:             0.0,
		MaxDepth:             2,
		IncludeRelationships: false,
		SearchMode:           interfaces.SearchModeHybrid,
	}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

// applyExtractionOptions applies ExtractionOptions and returns the effective options.
func applyExtractionOptions(opts []interfaces.ExtractionOption) *interfaces.ExtractionOptions {
	options := &interfaces.ExtractionOptions{
		SchemaGuided:   false,
		MinConfidence:  0.5,
		MaxEntities:    50,
		DedupThreshold: 0.92,
	}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

// CountAllEntities is a diagnostic method that counts all entities without any filter.
// This is useful for debugging to verify data is actually stored.
func (s *Store) CountAllEntities(ctx context.Context) (int, error) {
	className := s.getEntityClassName()

	// Use aggregate query to count all entities
	result, err := s.client.GraphQL().Aggregate().
		WithClassName(className).
		WithFields(
			graphql.Field{Name: "meta", Fields: []graphql.Field{{Name: "count"}}},
		).
		Do(ctx)

	if err != nil {
		s.logger.Error(ctx, "CountAllEntities failed", map[string]interface{}{
			"error": err.Error(),
		})
		return 0, err
	}

	s.logger.Info(ctx, "CountAllEntities raw result", map[string]interface{}{
		"data": result.Data,
	})

	// Parse the aggregate response
	if result.Data != nil {
		if aggData, ok := result.Data["Aggregate"].(map[string]interface{}); ok {
			if classData, ok := aggData[className].([]interface{}); ok {
				if len(classData) > 0 {
					if item, ok := classData[0].(map[string]interface{}); ok {
						if meta, ok := item["meta"].(map[string]interface{}); ok {
							if count, ok := meta["count"].(float64); ok {
								return int(count), nil
							}
						}
					}
				}
			}
		}
	}

	return 0, nil
}

// ListAllEntities is a diagnostic method that lists all entities without any filter.
func (s *Store) ListAllEntities(ctx context.Context, limit int) ([]interfaces.Entity, error) {
	className := s.getEntityClassName()

	if limit <= 0 {
		limit = 100
	}

	// Query all entities without filter
	result, err := s.client.GraphQL().Get().
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
		).
		WithLimit(limit).
		Do(ctx)

	if err != nil {
		s.logger.Error(ctx, "ListAllEntities failed", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, err
	}

	// Log raw result
	if result.Data != nil {
		if getData, ok := result.Data["Get"].(map[string]interface{}); ok {
			if classData, ok := getData[className].([]interface{}); ok {
				s.logger.Info(ctx, "ListAllEntities raw result", map[string]interface{}{
					"count": len(classData),
				})
			} else {
				s.logger.Warn(ctx, "ListAllEntities no class data", map[string]interface{}{
					"className": className,
					"getData":   getData,
				})
			}
		}
	}

	return parseEntityResults(result, className), nil
}
