# Enhanced Embedding Package

This package provides advanced embedding generation and manipulation capabilities for the Agent SDK. It includes features for configuring embedding models, batch processing, similarity calculations, and metadata filtering.

## Supported Providers

- **OpenAI**: text-embedding-3-small, text-embedding-3-large, text-embedding-ada-002
- **Google Gemini/Vertex AI**: text-embedding-004, text-embedding-005, text-multilingual-embedding-002

## Features

- **Configurable Embedding Generation**: Fine-tune embedding parameters such as dimensions, encoding format, and truncation behavior.
- **Batch Processing**: Generate embeddings for multiple texts in a single API call.
- **Similarity Calculations**: Calculate similarity between embeddings using different metrics (cosine, euclidean, dot product).
- **Advanced Metadata Filtering**: Create complex filter conditions for precise document retrieval.
- **Multi-Provider Support**: Use OpenAI or Google Gemini/Vertex AI for embedding generation.

## Usage

### OpenAI Embedding Generation

```go
// Create an embedder with default configuration
embedder := embedding.NewOpenAIEmbedder(apiKey, "text-embedding-3-small")

// Generate an embedding
vector, err := embedder.Embed(ctx, "Your text here")
if err != nil {
    // Handle error
}
```

### Gemini/Vertex AI Embedding Generation

```go
// Create a Gemini embedder with API key (Gemini API)
embedder, err := embedding.NewGeminiEmbedder(ctx,
    embedding.WithGeminiAPIKey(apiKey),
    embedding.WithGeminiModel(embedding.ModelTextEmbedding004),
)
if err != nil {
    // Handle error
}

// Generate an embedding
vector, err := embedder.Embed(ctx, "Your text here")
if err != nil {
    // Handle error
}

// Create a Vertex AI embedder with project credentials
vertexEmbedder, err := embedding.NewGeminiEmbedder(ctx,
    embedding.WithGeminiBackend(genai.BackendVertexAI),
    embedding.WithGeminiProjectID("your-project-id"),
    embedding.WithGeminiLocation("us-central1"),
    embedding.WithGeminiCredentialsFile("/path/to/service-account.json"),
)
if err != nil {
    // Handle error
}

// Use task type for optimized embeddings
queryEmbedder, err := embedding.NewGeminiEmbedder(ctx,
    embedding.WithGeminiAPIKey(apiKey),
    embedding.WithGeminiTaskType("RETRIEVAL_QUERY"),
)
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

### OpenAI Embedding Models

- `text-embedding-3-small`: Smaller, faster model (1536 dimensions by default)
- `text-embedding-3-large`: Larger, more accurate model (3072 dimensions by default)
- `text-embedding-ada-002`: Legacy model (1536 dimensions)

### Gemini/Vertex AI Embedding Models

- `text-embedding-004`: Latest text embedding model (768 dimensions)
- `text-embedding-005`: Newest text embedding model (768 dimensions)
- `text-multilingual-embedding-002`: Multilingual text embedding (768 dimensions)

### Gemini Task Types

Use task types to optimize embeddings for specific use cases:

- `RETRIEVAL_QUERY`: Optimized for search queries
- `RETRIEVAL_DOCUMENT`: Optimized for documents to be searched
- `SEMANTIC_SIMILARITY`: For comparing text similarity
- `CLASSIFICATION`: For classification tasks
- `CLUSTERING`: For clustering documents
- `QUESTION_ANSWERING`: For Q&A applications
- `FACT_VERIFICATION`: For fact-checking applications

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
