// Package graphrag provides graph-based retrieval-augmented generation capabilities.
//
// GraphRAG extends traditional RAG by leveraging knowledge graphs for enhanced context retrieval.
// Unlike pure vector search, GraphRAG maintains explicit relationships between entities, enabling:
//
//   - Relationship-aware retrieval
//   - Multi-hop graph traversal
//   - Community-based global search
//   - Entity extraction and knowledge building
package graphrag

import (
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Type aliases for convenience - the canonical types are in interfaces package
type (
	// Entity represents a node in the knowledge graph
	Entity = interfaces.Entity

	// Relationship represents an edge connecting two entities
	Relationship = interfaces.Relationship

	// GraphSearchResult represents a search result from the knowledge graph
	GraphSearchResult = interfaces.GraphSearchResult

	// GraphContext represents context around a central entity from graph traversal
	GraphContext = interfaces.GraphContext

	// GraphPath represents a path between two entities
	GraphPath = interfaces.GraphPath

	// ExtractionResult contains extracted entities and relationships from text
	ExtractionResult = interfaces.ExtractionResult

	// GraphSchema defines the structure of the knowledge graph
	GraphSchema = interfaces.GraphSchema

	// EntityTypeSchema defines an entity type in the schema
	EntityTypeSchema = interfaces.EntityTypeSchema

	// RelationshipTypeSchema defines a relationship type in the schema
	RelationshipTypeSchema = interfaces.RelationshipTypeSchema

	// PropertySchema defines a property in the schema
	PropertySchema = interfaces.PropertySchema

	// GraphStoreOptions contains options for storing graph data
	GraphStoreOptions = interfaces.GraphStoreOptions

	// GraphSearchOptions contains options for searching graph data
	GraphSearchOptions = interfaces.GraphSearchOptions

	// ExtractionOptions contains options for extraction operations
	ExtractionOptions = interfaces.ExtractionOptions

	// RelationshipDirection specifies the direction for relationship queries
	RelationshipDirection = interfaces.RelationshipDirection

	// GraphSearchMode specifies the type of search to perform
	GraphSearchMode = interfaces.GraphSearchMode
)

// Direction constants
const (
	DirectionOutgoing = interfaces.DirectionOutgoing
	DirectionIncoming = interfaces.DirectionIncoming
	DirectionBoth     = interfaces.DirectionBoth
)

// Search mode constants
const (
	SearchModeVector  = interfaces.SearchModeVector
	SearchModeKeyword = interfaces.SearchModeKeyword
	SearchModeHybrid  = interfaces.SearchModeHybrid
)

// Config holds configuration for GraphRAG providers
type Config struct {
	// Provider is the backend provider ("weaviate", "neo4j")
	Provider string `json:"provider" yaml:"provider"`

	// Host is the hostname of the backend server
	Host string `json:"host" yaml:"host"`

	// Scheme is the URL scheme (http or https)
	Scheme string `json:"scheme" yaml:"scheme"`

	// APIKey is the authentication key
	APIKey string `json:"api_key" yaml:"api_key"`

	// ClassPrefix is the prefix for collection/class names
	ClassPrefix string `json:"class_prefix" yaml:"class_prefix"`

	// Schema is the optional schema definition
	Schema *GraphSchema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Provider:    "weaviate",
		Host:        "localhost:8080",
		Scheme:      "http",
		ClassPrefix: "Graph",
	}
}
