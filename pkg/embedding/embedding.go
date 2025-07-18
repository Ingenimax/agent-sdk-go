package embedding

import (
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// EmbeddingConfig contains configuration options for embedding generation
type EmbeddingConfig struct {
	// Model is the embedding model to use
	Model string

	// Dimensions specifies the dimensionality of the embedding vectors
	// Only supported by some models (e.g., text-embedding-3-*)
	Dimensions int

	// EncodingFormat specifies the format of the embedding vectors
	// Options: "float", "base64"
	EncodingFormat string

	// Truncation controls how the input text is handled if it exceeds the model's token limit
	// Options: "none" (error on overflow), "truncate" (truncate to limit)
	Truncation string

	// SimilarityMetric specifies the similarity metric to use when comparing embeddings
	// Options: "cosine" (default), "euclidean", "dot_product"
	SimilarityMetric string

	// SimilarityThreshold specifies the minimum similarity score for search results
	SimilarityThreshold float32

	// UserID is an optional identifier for tracking embedding usage
	UserID string
}

// DefaultEmbeddingConfig returns a default configuration for embedding generation
func DefaultEmbeddingConfig(model string) EmbeddingConfig {
	// Use provided model or fall back to default
	if model == "" {
		model = "text-embedding-3-small"
	}

	return EmbeddingConfig{
		Model:               model,
		Dimensions:          0, // Use model default
		EncodingFormat:      "float",
		Truncation:          "truncate",
		SimilarityMetric:    "cosine",
		SimilarityThreshold: 0.0, // Default to no threshold
	}
}

// Client defines the interface for an embedding client
type Client interface {
	// Embed generates an embedding for the given text
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedWithConfig generates an embedding with custom configuration
	EmbedWithConfig(ctx context.Context, text string, config EmbeddingConfig) ([]float32, error)

	// EmbedBatch generates embeddings for multiple texts
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedBatchWithConfig generates embeddings for multiple texts with custom configuration
	EmbedBatchWithConfig(ctx context.Context, texts []string, config EmbeddingConfig) ([][]float32, error)

	// CalculateSimilarity calculates the similarity between two embeddings
	CalculateSimilarity(vec1, vec2 []float32, metric string) (float32, error)
}

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
		config.Model = "text-embedding-3-small" // Default model if not specified
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
	if len(vec1) != len(vec2) {
		return 0, errors.New("embedding vectors must have the same dimensions")
	}

	if metric == "" {
		metric = e.config.SimilarityMetric
	}

	switch metric {
	case "cosine":
		return cosineSimilarity(vec1, vec2), nil
	case "euclidean":
		return euclideanDistance(vec1, vec2), nil
	case "dot_product":
		return dotProduct(vec1, vec2), nil
	default:
		return 0, fmt.Errorf("unsupported similarity metric: %s", metric)
	}
}

// cosineSimilarity calculates the cosine similarity between two vectors
func cosineSimilarity(vec1, vec2 []float32) float32 {
	var dotProd, mag1, mag2 float32

	for i := 0; i < len(vec1); i++ {
		dotProd += vec1[i] * vec2[i]
		mag1 += vec1[i] * vec1[i]
		mag2 += vec2[i] * vec2[i]
	}

	mag1 = float32(float64(mag1) + 1e-9) // Avoid division by zero
	mag2 = float32(float64(mag2) + 1e-9) // Avoid division by zero

	return dotProd / (float32(float64(mag1) * float64(mag2)))
}

// euclideanDistance calculates the euclidean distance between two vectors
// Returns a similarity score (1 - normalized distance)
func euclideanDistance(vec1, vec2 []float32) float32 {
	var sum float32

	for i := 0; i < len(vec1); i++ {
		diff := vec1[i] - vec2[i]
		sum += diff * diff
	}

	// Convert distance to similarity (1 - normalized distance)
	// Using a simple normalization approach
	distance := float32(float64(sum) + 1e-9)
	return 1.0 / (1.0 + distance)
}

// dotProduct calculates the dot product between two vectors
func dotProduct(vec1, vec2 []float32) float32 {
	var sum float32

	for i := 0; i < len(vec1); i++ {
		sum += vec1[i] * vec2[i]
	}

	return sum
}

// GetConfig returns the current configuration
func (e *OpenAIEmbedder) GetConfig() EmbeddingConfig {
	return e.config
}
