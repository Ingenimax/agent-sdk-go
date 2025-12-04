package embedding

import (
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAI embedding model constants
const (
	// ModelTextEmbedding3Small is the smaller, faster OpenAI embedding model (1536 dimensions by default)
	ModelTextEmbedding3Small = "text-embedding-3-small"

	// ModelTextEmbedding3Large is the larger, more accurate OpenAI embedding model (3072 dimensions by default)
	ModelTextEmbedding3Large = "text-embedding-3-large"

	// ModelTextEmbeddingAda002 is the legacy OpenAI embedding model (1536 dimensions)
	ModelTextEmbeddingAda002 = "text-embedding-ada-002"

	// DefaultOpenAIEmbeddingModel is the default OpenAI embedding model
	DefaultOpenAIEmbeddingModel = ModelTextEmbedding3Small
)

// OpenAIEmbedder implements embedding generation using OpenAI API
type OpenAIEmbedder struct {
	client openai.Client
	model  string
	config EmbeddingConfig
}

// NewOpenAIEmbedder creates a new OpenAIEmbedder instance with default configuration
func NewOpenAIEmbedder(apiKey, model string) *OpenAIEmbedder {
	config := DefaultEmbeddingConfig(model)

	return &OpenAIEmbedder{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
		model:  config.Model,
		config: config,
	}
}

// NewOpenAIEmbedderWithConfig creates a new OpenAIEmbedder with custom configuration
func NewOpenAIEmbedderWithConfig(apiKey string, config EmbeddingConfig) *OpenAIEmbedder {
	// Ensure we have a valid model
	if config.Model == "" {
		config.Model = DefaultOpenAIEmbeddingModel
	}

	return &OpenAIEmbedder{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
		model:  config.Model,
		config: config,
	}
}

// Embed generates an embedding using OpenAI API with default configuration
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return e.EmbedWithConfig(ctx, text, e.config)
}

// EmbedWithConfig generates an embedding using OpenAI API with custom configuration
func (e *OpenAIEmbedder) EmbedWithConfig(ctx context.Context, text string, config EmbeddingConfig) ([]float32, error) {
	req := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String(text)},
		Model: openai.EmbeddingModel(config.Model),
	}

	// Apply configuration options if supported by the model
	if config.Dimensions > 0 {
		req.Dimensions = openai.Int(int64(config.Dimensions))
	}

	if config.EncodingFormat != "" {
		req.EncodingFormat = openai.EmbeddingNewParamsEncodingFormat(config.EncodingFormat)
	}

	if config.UserID != "" {
		req.User = openai.String(config.UserID)
	}

	resp, err := e.client.Embeddings.New(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, errors.New("no embedding data returned from API")
	}

	// Convert float64 to float32
	embedding := make([]float32, len(resp.Data[0].Embedding))
	for i, v := range resp.Data[0].Embedding {
		embedding[i] = float32(v)
	}

	return embedding, nil
}

// EmbedBatch generates embeddings for multiple texts using default configuration
func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return e.EmbedBatchWithConfig(ctx, texts, e.config)
}

// EmbedBatchWithConfig generates embeddings for multiple texts with custom configuration
func (e *OpenAIEmbedder) EmbedBatchWithConfig(ctx context.Context, texts []string, config EmbeddingConfig) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	req := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: texts},
		Model: openai.EmbeddingModel(config.Model),
	}

	// Apply configuration options if supported by the model
	if config.Dimensions > 0 {
		req.Dimensions = openai.Int(int64(config.Dimensions))
	}

	if config.EncodingFormat != "" {
		req.EncodingFormat = openai.EmbeddingNewParamsEncodingFormat(config.EncodingFormat)
	}

	if config.UserID != "" {
		req.User = openai.String(config.UserID)
	}

	resp, err := e.client.Embeddings.New(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, errors.New("no embedding data returned from API")
	}

	// Sort embeddings by index to ensure correct order
	embeddings := make([][]float32, len(texts))
	for _, data := range resp.Data {
		if int(data.Index) >= len(embeddings) {
			return nil, fmt.Errorf("invalid embedding index: %d", data.Index)
		}
		// Convert float64 to float32
		embedding := make([]float32, len(data.Embedding))
		for i, v := range data.Embedding {
			embedding[i] = float32(v)
		}
		embeddings[data.Index] = embedding
	}

	return embeddings, nil
}

// CalculateSimilarity calculates the similarity between two embeddings
func (e *OpenAIEmbedder) CalculateSimilarity(vec1, vec2 []float32, metric string) (float32, error) {
	if metric == "" {
		metric = e.config.SimilarityMetric
	}
	return CalculateSimilarity(vec1, vec2, metric)
}

// GetConfig returns the current configuration
func (e *OpenAIEmbedder) GetConfig() EmbeddingConfig {
	return e.config
}

// GetModel returns the model name being used
func (e *OpenAIEmbedder) GetModel() string {
	return e.model
}
