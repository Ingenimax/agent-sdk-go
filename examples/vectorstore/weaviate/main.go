package main

import (
	"context"
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
				"source": "example",
				"type":   "pangram",
			},
		},
		{
			ID:      uuid.New().String(),
			Content: "To be or not to be, that is the question",
			Metadata: map[string]interface{}{
				"source": "example",
				"type":   "quote",
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

	logger.Info(ctx, "Storing documents with embeddings...", nil)
	if err := store.Store(ctx, docs); err != nil {
		logger.Error(ctx, "Failed to store documents", map[string]interface{}{"error": err.Error()})
		return
	}

	// Add a delay to ensure documents are indexed
	logger.Info(ctx, "Waiting for documents to be indexed...", nil)
	time.Sleep(2 * time.Second)

	logger.Info(ctx, "Searching for 'fox jumps'...", nil)
	results, err := store.Search(ctx, "fox jumps", 5, interfaces.WithEmbedding(true))
	if err != nil {
		logger.Error(ctx, "Search failed", map[string]interface{}{
			"error": err.Error(),
		})

		// Try a different search approach as fallback
		logger.Info(ctx, "Trying alternative search approach...", nil)
		results, err = store.Search(ctx, "fox", 5, interfaces.WithEmbedding(true))
		if err != nil {
			logger.Error(ctx, "Alternative search also failed", map[string]interface{}{"error": err.Error()})

			// Continue with cleanup even if search failed
			logger.Info(ctx, "Proceeding to cleanup...", nil)
			goto cleanup
		}
	}

	if len(results) == 0 {
		logger.Info(ctx, "No results found with embedding search.", nil)
	} else {
		logger.Info(ctx, "Search results:", map[string]interface{}{"results": results})
		for _, r := range results {
			logger.Info(ctx, "Search result", map[string]interface{}{"result": r})
		}
	}

cleanup:
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
