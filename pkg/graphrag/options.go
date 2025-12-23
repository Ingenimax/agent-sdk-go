package graphrag

import "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"

// Re-export option functions from interfaces for convenience

// GraphStoreOption represents an option for graph store operations
type GraphStoreOption = interfaces.GraphStoreOption

// GraphSearchOption represents an option for graph search operations
type GraphSearchOption = interfaces.GraphSearchOption

// ExtractionOption represents an option for extraction operations
type ExtractionOption = interfaces.ExtractionOption

// Store options

// WithBatchSize sets the batch size for store operations
func WithBatchSize(size int) GraphStoreOption {
	return interfaces.WithGraphBatchSize(size)
}

// WithGenerateEmbeddings sets whether to generate embeddings
func WithGenerateEmbeddings(generate bool) GraphStoreOption {
	return interfaces.WithGenerateEmbeddings(generate)
}

// WithTenant sets the tenant for graph operations
func WithTenant(tenant string) GraphStoreOption {
	return interfaces.WithGraphTenant(tenant)
}

// Search options

// WithMinScore sets the minimum similarity score
func WithMinScore(score float32) GraphSearchOption {
	return interfaces.WithMinGraphScore(score)
}

// WithEntityTypes filters search by entity types
func WithEntityTypes(types ...string) GraphSearchOption {
	return interfaces.WithEntityTypes(types...)
}

// WithRelationshipTypes filters search by relationship types
func WithRelationshipTypes(types ...string) GraphSearchOption {
	return interfaces.WithRelationshipTypes(types...)
}

// WithMaxDepth sets maximum traversal depth
func WithMaxDepth(depth int) GraphSearchOption {
	return interfaces.WithMaxDepth(depth)
}

// WithIncludeRelationships includes relationships in results
func WithIncludeRelationships(include bool) GraphSearchOption {
	return interfaces.WithIncludeRelationships(include)
}

// WithSearchTenant sets the tenant for search operations
func WithSearchTenant(tenant string) GraphSearchOption {
	return interfaces.WithSearchTenant(tenant)
}

// WithMode sets the search mode (vector, keyword, hybrid)
func WithMode(mode GraphSearchMode) GraphSearchOption {
	return interfaces.WithSearchMode(mode)
}

// Extraction options

// WithSchemaGuided enables schema-guided extraction
func WithSchemaGuided(guided bool) ExtractionOption {
	return interfaces.WithSchemaGuided(guided)
}

// WithExtractionEntityTypes limits extraction to specific entity types
func WithExtractionEntityTypes(types ...string) ExtractionOption {
	return interfaces.WithExtractionEntityTypes(types...)
}

// WithExtractionRelationshipTypes limits extraction to specific relationship types
func WithExtractionRelationshipTypes(types ...string) ExtractionOption {
	return interfaces.WithExtractionRelationshipTypes(types...)
}

// WithMinConfidence sets the minimum extraction confidence
func WithMinConfidence(confidence float32) ExtractionOption {
	return interfaces.WithMinConfidence(confidence)
}

// WithMaxEntities limits the number of extracted entities
func WithMaxEntities(max int) ExtractionOption {
	return interfaces.WithMaxEntities(max)
}

// WithDedupThreshold sets the embedding similarity threshold for deduplication
func WithDedupThreshold(threshold float32) ExtractionOption {
	return interfaces.WithDedupThreshold(threshold)
}
