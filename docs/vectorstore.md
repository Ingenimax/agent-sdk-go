# Vector Store

This document explains how to use the Vector Store component of the Agent SDK.

## Overview

Vector stores are used to store and retrieve vector embeddings, which are numerical representations of text that capture semantic meaning. They enable semantic search and retrieval of information based on similarity.

## Supported Vector Stores

### Weaviate

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create Weaviate vector store
store := weaviate.New(
    cfg.VectorStore.Weaviate.URL,
    weaviate.WithAPIKey(cfg.VectorStore.Weaviate.APIKey),
    weaviate.WithClassName("Document"),
)
```

### Pinecone

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/pinecone"
    "github.com/Ingenimax/agent-sdk-go/pkg/config"
)

// Get configuration
cfg := config.Get()

// Create Pinecone vector store
store := pinecone.New(
    cfg.VectorStore.Pinecone.APIKey,
    cfg.VectorStore.Pinecone.Environment,
    cfg.VectorStore.Pinecone.Index,
)
```

## Using Vector Stores

### Adding Documents

Add documents to the vector store:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Create documents
docs := []interfaces.Document{
    {
        ID:      "doc1",
        Content: "This is the first document about artificial intelligence.",
        Metadata: map[string]interface{}{
            "source": "article",
            "author": "John Doe",
        },
    },
    {
        ID:      "doc2",
        Content: "This is the second document about machine learning.",
        Metadata: map[string]interface{}{
            "source": "book",
            "author": "Jane Smith",
        },
    },
}

// Add documents to the vector store
err := store.AddDocuments(context.Background(), docs)
if err != nil {
    log.Fatalf("Failed to add documents: %v", err)
}
```

### Searching Documents

Search for documents by similarity:

```go
// Search for documents similar to a query
results, err := store.Search(
    context.Background(),
    "What is artificial intelligence?",
    interfaces.WithLimit(5),
)
if err != nil {
    log.Fatalf("Failed to search documents: %v", err)
}

// Print search results
for _, result := range results {
    fmt.Printf("Document ID: %s\n", result.ID)
    fmt.Printf("Content: %s\n", result.Content)
    fmt.Printf("Score: %f\n", result.Score)
    fmt.Println("Metadata:", result.Metadata)
    fmt.Println()
}
```

### Retrieving Documents

Retrieve documents by ID:

```go
// Retrieve documents by ID
docs, err := store.GetDocuments(
    context.Background(),
    []string{"doc1", "doc2"},
)
if err != nil {
    log.Fatalf("Failed to retrieve documents: %v", err)
}

// Print retrieved documents
for _, doc := range docs {
    fmt.Printf("Document ID: %s\n", doc.ID)
    fmt.Printf("Content: %s\n", doc.Content)
    fmt.Println("Metadata:", doc.Metadata)
    fmt.Println()
}
```

### Deleting Documents

Delete documents from the vector store:

```go
// Delete documents by ID
err := store.DeleteDocuments(
    context.Background(),
    []string{"doc1"},
)
if err != nil {
    log.Fatalf("Failed to delete documents: %v", err)
}
```

## Configuration Options

### Weaviate Options

```go
// Set the API key
weaviate.WithAPIKey("your-api-key")

// Set the scheme (http or https)
weaviate.WithScheme("https")

// Set the host
weaviate.WithHost("localhost:8080")

// Set the class name
weaviate.WithClassName("Document")

// Set the organization ID for multi-tenancy
weaviate.WithOrgID("org-123")
```

### Pinecone Options

```go
// Set the namespace
pinecone.WithNamespace("default")

// Set the organization ID for multi-tenancy
pinecone.WithOrgID("org-123")
```

## Multi-tenancy with Vector Stores

When using vector stores with multi-tenancy, you can specify the organization ID:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create context with organization ID
ctx := context.Background()
ctx = multitenancy.WithOrgID(ctx, "org-123")

// Add documents for this organization
err := store.AddDocuments(ctx, docs)

// Search documents for this organization
results, err := store.Search(ctx, "artificial intelligence")
```

## Creating Custom Vector Store Implementations

You can implement custom vector stores by implementing the `interfaces.VectorStore` interface:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// CustomVectorStore is a custom vector store implementation
type CustomVectorStore struct {
    // Add your fields here
}

// NewCustomVectorStore creates a new custom vector store
func NewCustomVectorStore() *CustomVectorStore {
    return &CustomVectorStore{}
}

// AddDocuments adds documents to the vector store
func (s *CustomVectorStore) AddDocuments(ctx context.Context, documents []interfaces.Document) error {
    // Implement your logic to add documents
    return nil
}

// Search searches for documents similar to the query
func (s *CustomVectorStore) Search(ctx context.Context, query string, options ...interfaces.SearchOption) ([]interfaces.SearchResult, error) {
    // Apply options
    opts := &interfaces.SearchOptions{}
    for _, option := range options {
        option(opts)
    }

    // Implement your search logic
    return []interfaces.SearchResult{
        {
            ID:      "doc1",
            Content: "This is a document about artificial intelligence.",
            Score:   0.95,
            Metadata: map[string]interface{}{
                "source": "article",
            },
        },
    }, nil
}

// GetDocuments retrieves documents by ID
func (s *CustomVectorStore) GetDocuments(ctx context.Context, ids []string) ([]interfaces.Document, error) {
    // Implement your logic to retrieve documents
    return []interfaces.Document{
        {
            ID:      "doc1",
            Content: "This is a document about artificial intelligence.",
            Metadata: map[string]interface{}{
                "source": "article",
            },
        },
    }, nil
}

// DeleteDocuments deletes documents by ID
func (s *CustomVectorStore) DeleteDocuments(ctx context.Context, ids []string) error {
    // Implement your logic to delete documents
    return nil
}

// Name returns the name of the vector store
func (s *CustomVectorStore) Name() string {
    return "custom-vector-store"
}
```

## Using Vector Stores with Embeddings

Vector stores typically work with embedding models to convert text to vectors:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/embedding"
    "github.com/Ingenimax/agent-sdk-go/pkg/embedding/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
)

// Create embedding model
embedder := openai.NewEmbedder(cfg.Embedding.OpenAI.APIKey)

// Create vector store with embedder
store := weaviate.New(
    cfg.VectorStore.Weaviate.URL,
    weaviate.WithAPIKey(cfg.VectorStore.Weaviate.APIKey),
    weaviate.WithEmbedder(embedder),
)

// Add documents (embeddings will be generated automatically)
err := store.AddDocuments(context.Background(), docs)

// Search (query will be embedded automatically)
results, err := store.Search(context.Background(), "artificial intelligence")
```

## Example: Complete Vector Store Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/config"
    "github.com/Ingenimax/agent-sdk-go/pkg/embedding/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
)

func main() {
    // Get configuration
    cfg := config.Get()

    // Create embedding model
    embedder := openai.NewEmbedder(cfg.Embedding.OpenAI.APIKey)

    // Create vector store
    store := weaviate.New(
        cfg.VectorStore.Weaviate.URL,
        weaviate.WithAPIKey(cfg.VectorStore.Weaviate.APIKey),
        weaviate.WithEmbedder(embedder),
    )

    // Create context
    ctx := context.Background()

    // Create documents
    docs := []interfaces.Document{
        {
            ID:      "doc1",
            Content: "Artificial intelligence (AI) is intelligence demonstrated by machines.",
            Metadata: map[string]interface{}{
                "source": "wikipedia",
                "topic":  "AI",
            },
        },
        {
            ID:      "doc2",
            Content: "Machine learning is a subset of artificial intelligence.",
            Metadata: map[string]interface{}{
                "source": "textbook",
                "topic":  "ML",
            },
        },
        {
            ID:      "doc3",
            Content: "Deep learning is a type of machine learning based on artificial neural networks.",
            Metadata: map[string]interface{}{
                "source": "research paper",
                "topic":  "DL",
            },
        },
    }

    // Add documents to the vector store
    err := store.AddDocuments(ctx, docs)
    if err != nil {
        log.Fatalf("Failed to add documents: %v", err)
    }
    fmt.Println("Added documents to vector store")

    // Search for documents
    results, err := store.Search(
        ctx,
        "What is artificial intelligence?",
        interfaces.WithLimit(2),
    )
    if err != nil {
        log.Fatalf("Failed to search documents: %v", err)
    }

    // Print search results
    fmt.Println("Search results:")
    for i, result := range results {
        fmt.Printf("%d. Document ID: %s (Score: %.4f)\n", i+1, result.ID, result.Score)
        fmt.Printf("   Content: %s\n", result.Content)
        fmt.Printf("   Metadata: %v\n", result.Metadata)
    }
}
