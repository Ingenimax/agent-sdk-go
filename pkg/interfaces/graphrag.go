package interfaces

import (
	"context"
	"time"
)

// GraphRAGStore defines the interface for graph-based retrieval-augmented generation
type GraphRAGStore interface {
	// Entity CRUD operations
	StoreEntities(ctx context.Context, entities []Entity, options ...GraphStoreOption) error
	GetEntity(ctx context.Context, id string, options ...GraphStoreOption) (*Entity, error)
	UpdateEntity(ctx context.Context, entity Entity, options ...GraphStoreOption) error
	DeleteEntity(ctx context.Context, id string, options ...GraphStoreOption) error

	// Relationship CRUD operations
	StoreRelationships(ctx context.Context, relationships []Relationship, options ...GraphStoreOption) error
	GetRelationships(ctx context.Context, entityID string, direction RelationshipDirection, options ...GraphSearchOption) ([]Relationship, error)
	DeleteRelationship(ctx context.Context, id string, options ...GraphStoreOption) error

	// Search operations
	Search(ctx context.Context, query string, limit int, options ...GraphSearchOption) ([]GraphSearchResult, error)
	LocalSearch(ctx context.Context, query string, entityID string, depth int, options ...GraphSearchOption) ([]GraphSearchResult, error)
	GlobalSearch(ctx context.Context, query string, communityLevel int, options ...GraphSearchOption) ([]GraphSearchResult, error)

	// Graph traversal
	TraverseFrom(ctx context.Context, entityID string, depth int, options ...GraphSearchOption) (*GraphContext, error)
	ShortestPath(ctx context.Context, sourceID, targetID string, options ...GraphSearchOption) (*GraphPath, error)

	// Entity/Relationship extraction (requires LLM)
	ExtractFromText(ctx context.Context, text string, llm LLM, options ...ExtractionOption) (*ExtractionResult, error)

	// Schema management
	ApplySchema(ctx context.Context, schema GraphSchema) error
	DiscoverSchema(ctx context.Context) (*GraphSchema, error)

	// Multi-tenancy
	SetTenant(tenant string)
	GetTenant() string

	// Lifecycle
	Close() error
}

// RelationshipDirection specifies the direction for relationship queries
type RelationshipDirection string

const (
	// DirectionOutgoing returns relationships where the entity is the source
	DirectionOutgoing RelationshipDirection = "outgoing"
	// DirectionIncoming returns relationships where the entity is the target
	DirectionIncoming RelationshipDirection = "incoming"
	// DirectionBoth returns relationships in both directions
	DirectionBoth RelationshipDirection = "both"
)

// Entity represents a node in the knowledge graph
type Entity struct {
	// ID is the unique identifier for the entity
	ID string `json:"id"`

	// Name is the human-readable name of the entity
	Name string `json:"name"`

	// Type categorizes the entity (e.g., "Person", "Organization", "Concept")
	Type string `json:"type"`

	// Description provides detailed information about the entity
	Description string `json:"description"`

	// Embedding is the vector representation of the entity
	Embedding []float32 `json:"embedding,omitempty"`

	// Properties contains additional key-value attributes
	Properties map[string]interface{} `json:"properties,omitempty"`

	// OrgID is the organization ID for multi-tenancy
	OrgID string `json:"org_id,omitempty"`

	// CreatedAt is the creation timestamp
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the last update timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// Relationship represents an edge connecting two entities
type Relationship struct {
	// ID is the unique identifier for the relationship
	ID string `json:"id"`

	// SourceID is the ID of the source entity
	SourceID string `json:"source_id"`

	// TargetID is the ID of the target entity
	TargetID string `json:"target_id"`

	// Type describes the relationship (e.g., "WORKS_ON", "MANAGES", "LOCATED_IN")
	Type string `json:"type"`

	// Description provides context about the relationship
	Description string `json:"description"`

	// Strength indicates the relationship strength (0.0 to 1.0, default 1.0)
	Strength float32 `json:"strength"`

	// Properties contains additional attributes
	Properties map[string]interface{} `json:"properties,omitempty"`

	// OrgID is the organization ID for multi-tenancy
	OrgID string `json:"org_id,omitempty"`

	// CreatedAt is the creation timestamp
	CreatedAt time.Time `json:"created_at"`
}

// GraphSearchResult represents a search result from the knowledge graph
type GraphSearchResult struct {
	// Entity is the found entity
	Entity Entity `json:"entity"`

	// Score is the relevance score (0-1, higher is more relevant)
	Score float32 `json:"score"`

	// Context contains related entities from graph traversal
	Context []Entity `json:"context,omitempty"`

	// Path represents the relationship path to this entity (for local search)
	Path []Relationship `json:"path,omitempty"`

	// CommunityID is the community identifier (for global search)
	CommunityID string `json:"community_id,omitempty"`
}

// GraphContext represents context around a central entity from graph traversal
type GraphContext struct {
	// CentralEntity is the starting point of the traversal
	CentralEntity Entity `json:"central_entity"`

	// Entities are all entities discovered within the traversal depth
	Entities []Entity `json:"entities"`

	// Relationships are all relationships discovered in the traversal
	Relationships []Relationship `json:"relationships"`

	// Depth is the actual traversal depth reached
	Depth int `json:"depth"`
}

// GraphPath represents a path between two entities
type GraphPath struct {
	// Source is the starting entity
	Source Entity `json:"source"`

	// Target is the destination entity
	Target Entity `json:"target"`

	// Entities are intermediate entities (ordered)
	Entities []Entity `json:"entities"`

	// Relationships are relationships connecting the entities (ordered)
	Relationships []Relationship `json:"relationships"`

	// Length is the number of hops
	Length int `json:"length"`
}

// ExtractionResult contains extracted entities and relationships from text
type ExtractionResult struct {
	// Entities are the extracted entities
	Entities []Entity `json:"entities"`

	// Relationships are the extracted relationships
	Relationships []Relationship `json:"relationships"`

	// SourceText is the original text
	SourceText string `json:"source_text"`

	// Confidence is the overall extraction confidence (0-1)
	Confidence float32 `json:"confidence"`
}

// GraphSchema defines the structure of the knowledge graph
type GraphSchema struct {
	// EntityTypes are the allowed entity type definitions
	EntityTypes []EntityTypeSchema `json:"entity_types"`

	// RelationshipTypes are the allowed relationship type definitions
	RelationshipTypes []RelationshipTypeSchema `json:"relationship_types"`
}

// EntityTypeSchema defines an entity type in the schema
type EntityTypeSchema struct {
	// Name is the type name (e.g., "Person")
	Name string `json:"name"`

	// Description describes the entity type
	Description string `json:"description"`

	// Properties defines the expected properties
	Properties []PropertySchema `json:"properties,omitempty"`
}

// RelationshipTypeSchema defines a relationship type in the schema
type RelationshipTypeSchema struct {
	// Name is the relationship type name (e.g., "WORKS_ON")
	Name string `json:"name"`

	// Description describes the relationship type
	Description string `json:"description"`

	// SourceTypes are the allowed source entity types
	SourceTypes []string `json:"source_types,omitempty"`

	// TargetTypes are the allowed target entity types
	TargetTypes []string `json:"target_types,omitempty"`

	// Properties defines the expected properties
	Properties []PropertySchema `json:"properties,omitempty"`
}

// PropertySchema defines a property in the schema
type PropertySchema struct {
	// Name is the property name
	Name string `json:"name"`

	// Type is the data type: "string", "number", "boolean", "datetime"
	Type string `json:"type"`

	// Required indicates whether the property is required
	Required bool `json:"required"`

	// Description describes the property
	Description string `json:"description"`

	// Default is the default value (optional)
	Default interface{} `json:"default,omitempty"`
}

// GraphStoreOption represents an option for graph store operations
type GraphStoreOption func(*GraphStoreOptions)

// GraphSearchOption represents an option for graph search operations
type GraphSearchOption func(*GraphSearchOptions)

// ExtractionOption represents an option for extraction operations
type ExtractionOption func(*ExtractionOptions)

// GraphStoreOptions contains options for storing graph data
type GraphStoreOptions struct {
	// BatchSize is the number of items to store in each batch
	BatchSize int

	// GenerateEmbeddings indicates whether to generate embeddings
	GenerateEmbeddings bool

	// Tenant is the tenant name for native multi-tenancy
	Tenant string
}

// GraphSearchOptions contains options for searching graph data
type GraphSearchOptions struct {
	// MinScore is the minimum similarity score (0-1)
	MinScore float32

	// EntityTypes filters by entity types
	EntityTypes []string

	// RelationshipTypes filters by relationship types
	RelationshipTypes []string

	// MaxDepth limits traversal depth (default: 2)
	MaxDepth int

	// IncludeRelationships includes relationships in search results
	IncludeRelationships bool

	// Tenant is the tenant name for native multi-tenancy
	Tenant string

	// SearchMode specifies the search mode
	SearchMode GraphSearchMode
}

// GraphSearchMode specifies the type of search to perform
type GraphSearchMode string

const (
	// SearchModeVector uses vector similarity search
	SearchModeVector GraphSearchMode = "vector"
	// SearchModeKeyword uses keyword/BM25 search
	SearchModeKeyword GraphSearchMode = "keyword"
	// SearchModeHybrid combines vector and keyword search
	SearchModeHybrid GraphSearchMode = "hybrid"
)

// ExtractionOptions contains options for extraction operations
type ExtractionOptions struct {
	// SchemaGuided indicates whether to use schema-guided extraction
	SchemaGuided bool

	// EntityTypes limits extraction to specific entity types
	EntityTypes []string

	// RelationshipTypes limits extraction to specific relationship types
	RelationshipTypes []string

	// MinConfidence filters entities/relationships by minimum confidence
	MinConfidence float32

	// MaxEntities limits the number of entities to extract
	MaxEntities int

	// DedupThreshold is the embedding similarity threshold for deduplication
	DedupThreshold float32
}

// WithGraphBatchSize sets the batch size for store operations
func WithGraphBatchSize(size int) GraphStoreOption {
	return func(o *GraphStoreOptions) {
		o.BatchSize = size
	}
}

// WithGenerateEmbeddings sets whether to generate embeddings
func WithGenerateEmbeddings(generate bool) GraphStoreOption {
	return func(o *GraphStoreOptions) {
		o.GenerateEmbeddings = generate
	}
}

// WithGraphTenant sets the tenant for graph operations
func WithGraphTenant(tenant string) GraphStoreOption {
	return func(o *GraphStoreOptions) {
		o.Tenant = tenant
	}
}

// WithMinGraphScore sets the minimum similarity score
func WithMinGraphScore(score float32) GraphSearchOption {
	return func(o *GraphSearchOptions) {
		o.MinScore = score
	}
}

// WithEntityTypes filters search by entity types
func WithEntityTypes(types ...string) GraphSearchOption {
	return func(o *GraphSearchOptions) {
		o.EntityTypes = types
	}
}

// WithRelationshipTypes filters search by relationship types
func WithRelationshipTypes(types ...string) GraphSearchOption {
	return func(o *GraphSearchOptions) {
		o.RelationshipTypes = types
	}
}

// WithMaxDepth sets maximum traversal depth
func WithMaxDepth(depth int) GraphSearchOption {
	return func(o *GraphSearchOptions) {
		o.MaxDepth = depth
	}
}

// WithIncludeRelationships includes relationships in results
func WithIncludeRelationships(include bool) GraphSearchOption {
	return func(o *GraphSearchOptions) {
		o.IncludeRelationships = include
	}
}

// WithSearchTenant sets the tenant for search operations
func WithSearchTenant(tenant string) GraphSearchOption {
	return func(o *GraphSearchOptions) {
		o.Tenant = tenant
	}
}

// WithSearchMode sets the search mode
func WithSearchMode(mode GraphSearchMode) GraphSearchOption {
	return func(o *GraphSearchOptions) {
		o.SearchMode = mode
	}
}

// WithSchemaGuided enables schema-guided extraction
func WithSchemaGuided(guided bool) ExtractionOption {
	return func(o *ExtractionOptions) {
		o.SchemaGuided = guided
	}
}

// WithExtractionEntityTypes limits extraction to specific entity types
func WithExtractionEntityTypes(types ...string) ExtractionOption {
	return func(o *ExtractionOptions) {
		o.EntityTypes = types
	}
}

// WithExtractionRelationshipTypes limits extraction to specific relationship types
func WithExtractionRelationshipTypes(types ...string) ExtractionOption {
	return func(o *ExtractionOptions) {
		o.RelationshipTypes = types
	}
}

// WithMinConfidence sets the minimum extraction confidence
func WithMinConfidence(confidence float32) ExtractionOption {
	return func(o *ExtractionOptions) {
		o.MinConfidence = confidence
	}
}

// WithMaxEntities limits the number of extracted entities
func WithMaxEntities(max int) ExtractionOption {
	return func(o *ExtractionOptions) {
		o.MaxEntities = max
	}
}

// WithDedupThreshold sets the embedding similarity threshold for deduplication
func WithDedupThreshold(threshold float32) ExtractionOption {
	return func(o *ExtractionOptions) {
		o.DedupThreshold = threshold
	}
}
