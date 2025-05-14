# Advanced Embedding Example

This example demonstrates advanced usage of the embedding functionality in the Agent SDK, including custom configurations, metadata filtering, and vector store integration.

## Features

- Custom embedding configuration with specified dimensions
- Rich document metadata for advanced filtering
- Weaviate vector store integration
- Advanced metadata filtering with complex queries
- Similarity calculation between embeddings
- Batch embedding for efficient processing

## Usage

### Prerequisites

- Set the `OPENAI_API_KEY` environment variable with your OpenAI API key
- Configure Weaviate connection details in your configuration

```bash
export OPENAI_API_KEY=your_openai_api_key
```

### Running the Example

```bash
go run main.go
```

## Code Explanation

### Creating an Embedder with Custom Configuration

```go
embeddingConfig := embedding.DefaultEmbeddingConfig(cfg.LLM.OpenAI.EmbeddingModel)
embeddingConfig.Dimensions = 1536 // Specify dimensions for more precise embeddings
embeddingConfig.SimilarityMetric = "cosine"
embeddingConfig.SimilarityThreshold = 0.6 // Set a similarity threshold

embedder := embedding.NewOpenAIEmbedderWithConfig(cfg.LLM.OpenAI.APIKey, embeddingConfig)
```

### Creating a Vector Store

```go
store := weaviate.New(
    &interfaces.VectorStoreConfig{
        Host:   cfg.VectorStore.Weaviate.Host,
        APIKey: cfg.VectorStore.Weaviate.APIKey,
    },
    weaviate.WithClassPrefix("AdvancedDoc"),
    weaviate.WithEmbedder(embedder),
)
```

### Creating Documents with Rich Metadata

```go
docs := []interfaces.Document{
    {
        ID:      uuid.New().String(),
        Content: "The quick brown fox jumps over the lazy dog",
        Metadata: map[string]interface{}{
            "source":      "example",
            "type":        "pangram",
            "language":    "english",
            "word_count":  9,
            "created_at":  "2023-01-01",
            "category":    "animal",
            "tags":        []string{"fox", "dog", "quick"},
            "is_complete": true,
        },
    },
    // Additional documents...
}
```

### Generating and Storing Embeddings

```go
// Generate embeddings
for idx, doc := range docs {
    vector, err := embedder.EmbedWithConfig(ctx, doc.Content, embeddingConfig)
    if err != nil {
        log.Fatalf("Embedding failed: %v", err)
    }
    docs[idx].Vector = vector
}

// Store documents
if err := store.Store(ctx, docs); err != nil {
    log.Fatalf("Failed to store documents: %v", err)
}
```

### Basic Search

```go
results, err := store.Search(ctx, "fox jumps", 5, interfaces.WithEmbedding(true))
```

### Search with Metadata Filters

```go
filters := map[string]interface{}{
    "source": "shakespeare",
}
results, err = store.Search(ctx, "wisdom", 5,
    interfaces.WithEmbedding(true),
    interfaces.WithFilters(filters),
)
```

### Advanced Filtering

```go
filterGroup := embedding.NewMetadataFilterGroup("and",
    embedding.NewMetadataFilter("word_count", ">", 8),
    embedding.NewMetadataFilter("type", "=", "quote"),
)

weaviateFilters := embedding.FilterToWeaviateFormat(filterGroup)

results, err = store.Search(ctx, "question", 5,
    interfaces.WithEmbedding(true),
    interfaces.WithFilters(weaviateFilters),
)
```

### Similarity Calculation

```go
similarity, err := embedder.CalculateSimilarity(docs[0].Vector, docs[1].Vector, "cosine")
```

### Batch Embedding

```go
texts := []string{
    "This is the first text for batch embedding",
    "This is the second text for batch embedding",
    "This is the third text for batch embedding",
}
batchEmbeddings, err := embedder.EmbedBatch(ctx, texts)
```

## Advanced Features

### Filter Helpers

The example demonstrates several approaches to creating filters:

1. Simple key-value filters:
```go
filters := map[string]interface{}{"source": "shakespeare"}
```

2. Filter groups with complex conditions:
```go
filterGroup := embedding.NewMetadataFilterGroup("and",
    embedding.NewMetadataFilter("word_count", ">", 8),
    embedding.NewMetadataFilter("type", "=", "quote"),
)
```

3. Weaviate-specific filter helpers:
```go
wordCountFilter := embedding.CreateWeaviateFilter("word_count", ">", 8)
typeFilter := embedding.CreateWeaviateFilter("type", "=", "quote")
combinedFilter := embedding.CreateWeaviateAndFilter(wordCountFilter, typeFilter)
```

### Similarity Metrics

The example supports different similarity metrics:
- Cosine similarity (default)
- Euclidean distance
- Dot product

## Customization

You can customize this example by:
- Changing the embedding model or dimensions
- Adding more complex metadata filters
- Using different similarity metrics
- Implementing custom vector stores
- Adding more advanced search capabilities 