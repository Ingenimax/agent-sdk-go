# Enhanced Embedding Package

This package provides advanced embedding generation and manipulation capabilities for the Agent SDK. It includes features for configuring embedding models, batch processing, similarity calculations, and metadata filtering.

## Features

- **Configurable Embedding Generation**: Fine-tune embedding parameters such as dimensions, encoding format, and truncation behavior.
- **Batch Processing**: Generate embeddings for multiple texts in a single API call.
- **Similarity Calculations**: Calculate similarity between embeddings using different metrics (cosine, euclidean, dot product).
- **Advanced Metadata Filtering**: Create complex filter conditions for precise document retrieval.

## Usage

### Basic Embedding Generation

```go
// Create an embedder with default configuration
embedder := embedding.NewOpenAIEmbedder(apiKey, "text-embedding-3-small")

// Generate an embedding
vector, err := embedder.Embed(ctx, "Your text here")
if err != nil {
    // Handle error
}
```

### Custom Configuration

```go
// Create a custom configuration
config := embedding.DefaultEmbeddingConfig()
config.Model = "text-embedding-3-large"
config.Dimensions = 1536
config.SimilarityMetric = "cosine"

// Create an embedder with custom configuration
embedder := embedding.NewOpenAIEmbedderWithConfig(apiKey, config)

// Generate an embedding with custom configuration
vector, err := embedder.EmbedWithConfig(ctx, "Your text here", config)
if err != nil {
    // Handle error
}
```

### Batch Processing

```go
// Generate embeddings for multiple texts
texts := []string{
    "First text",
    "Second text",
    "Third text",
}

vectors, err := embedder.EmbedBatch(ctx, texts)
if err != nil {
    // Handle error
}
```

### Similarity Calculation

```go
// Calculate similarity between two vectors
similarity, err := embedder.CalculateSimilarity(vector1, vector2, "cosine")
if err != nil {
    // Handle error
}
```

## Metadata Filtering

The package includes powerful metadata filtering capabilities for precise document retrieval.

### Simple Filters

```go
// Create a simple filter
filter := embedding.NewMetadataFilter("category", "=", "science")

// Create a filter group
filterGroup := embedding.NewMetadataFilterGroup("and", filter)
```

### Complex Filters

```go
// Create a complex filter group
filterGroup := embedding.NewMetadataFilterGroup("and",
    embedding.NewMetadataFilter("category", "=", "science"),
    embedding.NewMetadataFilter("published_date", ">", "2023-01-01"),
)

// Add another filter
filterGroup.AddFilter(embedding.NewMetadataFilter("author", "=", "John Doe"))

// Create a sub-group with OR logic
subGroup := embedding.NewMetadataFilterGroup("or",
    embedding.NewMetadataFilter("tags", "contains", "physics"),
    embedding.NewMetadataFilter("tags", "contains", "chemistry"),
)

// Add the sub-group to the main group
filterGroup.AddSubGroup(subGroup)
```

### Using Filters with Vector Store

```go
// Convert filter group to map for vector store
filterMap := embedding.FilterToMap(filterGroup)

// Use with vector store search
results, err := store.Search(ctx, "query", 10,
    interfaces.WithEmbedding(true),
    interfaces.WithFilters(filterMap),
)
```

### Filtering Documents in Memory

```go
// Filter documents in memory
filteredDocs := embedding.ApplyFilters(documents, filterGroup)
```

## Supported Operators

### Comparison Operators

- `=`, `==`, `eq`: Equal
- `!=`, `<>`, `ne`: Not equal
- `>`, `gt`: Greater than
- `>=`, `gte`: Greater than or equal
- `<`, `lt`: Less than
- `<=`, `lte`: Less than or equal
- `contains`: String contains
- `in`: Value in collection
- `not_in`: Value not in collection

### Logical Operators

- `and`: All conditions must be true
- `or`: At least one condition must be true

## Configuration Options

### Embedding Models

- `text-embedding-3-small`: Smaller, faster model (1536 dimensions by default)
- `text-embedding-3-large`: Larger, more accurate model (3072 dimensions by default)
- `text-embedding-ada-002`: Legacy model (1536 dimensions)

### Dimensions

Specify the dimensionality of the embedding vectors. Only supported by some models.

### Encoding Format

- `float`: Standard floating-point format
- `base64`: Base64-encoded format for more compact storage

### Truncation

- `none`: Error on token limit overflow
- `truncate`: Truncate text to fit within token limit

### Similarity Metrics

- `cosine`: Cosine similarity (default)
- `euclidean`: Euclidean distance (converted to similarity score)
- `dot_product`: Dot product
