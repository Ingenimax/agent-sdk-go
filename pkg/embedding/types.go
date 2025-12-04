package embedding

import (
	"context"
	"errors"
	"fmt"
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

// CalculateSimilarity is a standalone function to calculate similarity between two embeddings
func CalculateSimilarity(vec1, vec2 []float32, metric string) (float32, error) {
	if len(vec1) != len(vec2) {
		return 0, errors.New("embedding vectors must have the same dimensions")
	}

	switch metric {
	case "cosine", "":
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
