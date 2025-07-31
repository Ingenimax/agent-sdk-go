package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/embedding"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
	"github.com/google/uuid"
)

func main() {
	// Create a logger
	logger := logging.New()

	ctx := multitenancy.WithOrgID(context.Background(), "exampleorg")

	// Load configuration
	cfg := config.Get()

	// Check if OpenAI API key is set
	if cfg.LLM.OpenAI.APIKey == "" {
		logger.Error(ctx, "OpenAI API key is not set. Please set the OPENAI_API_KEY environment variable.", nil)
		return
	}

	// Initialize the OpenAIEmbedder with the API key and model from config
	logger.Info(ctx, "Initializing OpenAI embedder", map[string]interface{}{
		"model": cfg.LLM.OpenAI.EmbeddingModel,
	})
	embedder := embedding.NewOpenAIEmbedder(cfg.LLM.OpenAI.APIKey, cfg.LLM.OpenAI.EmbeddingModel)

	// Create a more explicit configuration for Weaviate
	logger.Info(ctx, "Initializing Weaviate client", map[string]interface{}{
		"host":   cfg.VectorStore.Weaviate.Host,
		"scheme": cfg.VectorStore.Weaviate.Scheme,
	})

	// Check if Weaviate host is set
	if cfg.VectorStore.Weaviate.Host == "" {
		logger.Error(ctx, "Weaviate host is not set. Please set the WEAVIATE_HOST environment variable.", nil)
		return
	}

	// Check if Weaviate API key is set for cloud instances
	if cfg.VectorStore.Weaviate.APIKey == "" && cfg.VectorStore.Weaviate.Host != "localhost:8080" {
		logger.Warn(ctx, "Weaviate API key is not set. This may be required for cloud instances.", nil)
	}

	store := weaviate.New(
		&interfaces.VectorStoreConfig{
			Host:   cfg.VectorStore.Weaviate.Host,
			APIKey: cfg.VectorStore.Weaviate.APIKey,
			Scheme: cfg.VectorStore.Weaviate.Scheme,
		},
		weaviate.WithClassPrefix("TestDoc"),
		weaviate.WithEmbedder(embedder),
		weaviate.WithLogger(logger),
	)

	docs := []interfaces.Document{
		{
			ID:      uuid.New().String(),
			Content: "The quick brown fox jumps over the lazy dog",
			Metadata: map[string]interface{}{
				"source":    "example",
				"type":      "pangram",
				"wordCount": 9,                              // int field
				"isClassic": true,                           // boolean field
				"rating":    4.8,                            // float field
				"tags":      []string{"example", "pangram"}, // array field
			},
		},
		{
			ID:      uuid.New().String(),
			Content: "To be or not to be, that is the question",
			Metadata: map[string]interface{}{
				"source":    "literature",
				"type":      "quote",
				"author":    "Shakespeare",
				"year":      1603,                                             // int field
				"isClassic": true,                                             // boolean field
				"rating":    4.9,                                              // float field
				"tags":      []string{"literature", "shakespeare", "classic"}, // array field
			},
		},
		{
			ID:      uuid.New().String(),
			Content: "Hello, World! This is a simple greeting.",
			Metadata: map[string]interface{}{
				"source":    "programming",
				"type":      "example",
				"wordCount": 8,                                      // int field
				"isClassic": false,                                  // boolean field
				"rating":    3.5,                                    // float field
				"tags":      []string{"programming", "hello-world"}, // array field
			},
		},
	}

	// Embedding generation
	for idx, doc := range docs {
		vector, err := embedder.Embed(ctx, doc.Content)
		if err != nil {
			logger.Error(ctx, "Embedding failed", map[string]interface{}{"error": err.Error()})
			return
		}
		docs[idx].Vector = vector
	}

	logger.Info(ctx, "Storing documents with Weaviate auto-schema...", map[string]interface{}{
		"documentCount": len(docs),
		"note":          "Weaviate will automatically create the optimal schema from document metadata",
	})
	if err := store.Store(ctx, docs); err != nil {
		logger.Error(ctx, "Failed to store documents", map[string]interface{}{"error": err.Error()})
		return
	}
	logger.Info(ctx, "Documents stored successfully with auto-generated schema!", nil)

	// Add a delay to ensure documents are indexed
	logger.Info(ctx, "Waiting for documents to be indexed...", nil)
	time.Sleep(2 * time.Second)

	logger.Info(ctx, "Searching for 'fox jumps'...", nil)
	results, err := store.Search(ctx, "fox jumps", 5)
	if err != nil {
		logger.Error(ctx, "Failed to search documents", map[string]interface{}{"error": err.Error()})
		return
	}

	logger.Info(ctx, "Search results (auto-discovery - all fields):", map[string]interface{}{"count": len(results)})
	for i, result := range results {
		logger.Info(ctx, fmt.Sprintf("Result %d:", i+1), map[string]interface{}{
			"content":  result.Document.Content,
			"score":    result.Score,
			"metadata": result.Document.Metadata,
		})
	}

	// Example 2: Search with specific fields only
	logger.Info(ctx, "Searching with specific fields only (content + source)...", nil)
	specificFieldResults, err := store.Search(ctx, "fox jumps", 5,
		interfaces.WithFields("content", "source"),
	)
	if err != nil {
		logger.Error(ctx, "Failed to search with specific fields", map[string]interface{}{"error": err.Error()})
		return
	}

	logger.Info(ctx, "Search results (specific fields only):", map[string]interface{}{"count": len(specificFieldResults)})
	for i, result := range specificFieldResults {
		logger.Info(ctx, fmt.Sprintf("Specific fields result %d:", i+1), map[string]interface{}{
			"content":  result.Document.Content,
			"score":    result.Score,
			"metadata": result.Document.Metadata, // Should only contain 'source' field
		})
	}

	// Example 3: Search by vector with auto field discovery
	logger.Info(ctx, "Searching by vector with auto field discovery...", nil)
	testVector, err := embedder.Embed(ctx, "quick brown animal")
	if err != nil {
		logger.Error(ctx, "Failed to generate test vector", map[string]interface{}{"error": err.Error()})
		return
	}

	vectorResults, err := store.SearchByVector(ctx, testVector, 3)
	if err != nil {
		logger.Error(ctx, "Failed to search by vector", map[string]interface{}{"error": err.Error()})
		return
	}

	logger.Info(ctx, "Vector search results (auto-discovery):", map[string]interface{}{"count": len(vectorResults)})
	for i, result := range vectorResults {
		logger.Info(ctx, fmt.Sprintf("Vector result %d:", i+1), map[string]interface{}{
			"content":  result.Document.Content,
			"score":    result.Score,
			"metadata": result.Document.Metadata,
		})
	}

	// Example 4: Search with filters
	logger.Info(ctx, "Searching with filters (only classic documents)...", nil)
	filteredResults, err := store.Search(ctx, "fox jumps", 5,
		interfaces.WithFilters(map[string]interface{}{
			"isClassic": true,
		}),
	)
	if err != nil {
		logger.Error(ctx, "Failed to search with filters", map[string]interface{}{"error": err.Error()})
		return
	}

	logger.Info(ctx, "Filtered search results:", map[string]interface{}{"count": len(filteredResults)})
	for i, result := range filteredResults {
		logger.Info(ctx, fmt.Sprintf("Filtered result %d:", i+1), map[string]interface{}{
			"content":  result.Document.Content,
			"score":    result.Score,
			"metadata": result.Document.Metadata,
		})
	}

	logger.Info(ctx, "Dynamic field selection examples completed successfully!", nil)

	// Cleanup
	var ids []string
	for _, doc := range docs {
		ids = append(ids, doc.ID)
	}
	if err := store.Delete(ctx, ids); err != nil {
		logger.Error(ctx, "Cleanup failed", map[string]interface{}{"error": err.Error()})
		return
	}
	logger.Info(ctx, "Cleanup successful", nil)
}
