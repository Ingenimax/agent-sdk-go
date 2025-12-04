package embedding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenAIModelConstants(t *testing.T) {
	assert.Equal(t, "text-embedding-3-small", ModelTextEmbedding3Small)
	assert.Equal(t, "text-embedding-3-large", ModelTextEmbedding3Large)
	assert.Equal(t, "text-embedding-ada-002", ModelTextEmbeddingAda002)
	assert.Equal(t, ModelTextEmbedding3Small, DefaultOpenAIEmbeddingModel)
}

func TestNewOpenAIEmbedder(t *testing.T) {
	embedder := NewOpenAIEmbedder("test-api-key", "")
	assert.NotNil(t, embedder)
	assert.Equal(t, DefaultOpenAIEmbeddingModel, embedder.model)
	assert.Equal(t, DefaultOpenAIEmbeddingModel, embedder.config.Model)
}

func TestNewOpenAIEmbedderWithModel(t *testing.T) {
	embedder := NewOpenAIEmbedder("test-api-key", ModelTextEmbedding3Large)
	assert.NotNil(t, embedder)
	assert.Equal(t, ModelTextEmbedding3Large, embedder.model)
}

func TestNewOpenAIEmbedderWithConfig(t *testing.T) {
	config := EmbeddingConfig{
		Model:            ModelTextEmbedding3Large,
		Dimensions:       1024,
		SimilarityMetric: "dot_product",
	}
	embedder := NewOpenAIEmbedderWithConfig("test-api-key", config)
	assert.NotNil(t, embedder)
	assert.Equal(t, ModelTextEmbedding3Large, embedder.model)
	assert.Equal(t, 1024, embedder.config.Dimensions)
}

func TestNewOpenAIEmbedderWithConfig_DefaultModel(t *testing.T) {
	config := EmbeddingConfig{
		Dimensions: 512,
	}
	embedder := NewOpenAIEmbedderWithConfig("test-api-key", config)
	assert.NotNil(t, embedder)
	assert.Equal(t, DefaultOpenAIEmbeddingModel, embedder.model)
}

func TestOpenAIEmbedder_GetConfig(t *testing.T) {
	config := DefaultEmbeddingConfig(ModelTextEmbedding3Small)
	embedder := &OpenAIEmbedder{
		config: config,
	}
	assert.Equal(t, config, embedder.GetConfig())
}

func TestOpenAIEmbedder_GetModel(t *testing.T) {
	embedder := &OpenAIEmbedder{
		model: ModelTextEmbedding3Large,
	}
	assert.Equal(t, ModelTextEmbedding3Large, embedder.GetModel())
}

func TestOpenAIEmbedder_CalculateSimilarity(t *testing.T) {
	embedder := &OpenAIEmbedder{
		config: DefaultEmbeddingConfig(""),
	}

	t.Run("cosine similarity", func(t *testing.T) {
		vec1 := []float32{1.0, 0.0, 0.0}
		vec2 := []float32{1.0, 0.0, 0.0}

		similarity, err := embedder.CalculateSimilarity(vec1, vec2, "cosine")
		assert.NoError(t, err)
		assert.InDelta(t, 1.0, similarity, 0.01)
	})

	t.Run("default metric from config", func(t *testing.T) {
		vec1 := []float32{1.0, 0.0, 0.0}
		vec2 := []float32{1.0, 0.0, 0.0}

		similarity, err := embedder.CalculateSimilarity(vec1, vec2, "")
		assert.NoError(t, err)
		assert.InDelta(t, 1.0, similarity, 0.01)
	})
}
