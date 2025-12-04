package embedding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultEmbeddingConfig(t *testing.T) {
	t.Run("with empty model", func(t *testing.T) {
		config := DefaultEmbeddingConfig("")
		assert.Equal(t, "text-embedding-3-small", config.Model)
		assert.Equal(t, 0, config.Dimensions)
		assert.Equal(t, "float", config.EncodingFormat)
		assert.Equal(t, "truncate", config.Truncation)
		assert.Equal(t, "cosine", config.SimilarityMetric)
		assert.Equal(t, float32(0.0), config.SimilarityThreshold)
	})

	t.Run("with custom model", func(t *testing.T) {
		config := DefaultEmbeddingConfig("custom-model")
		assert.Equal(t, "custom-model", config.Model)
	})
}

func TestCalculateSimilarity(t *testing.T) {
	t.Run("cosine similarity - identical vectors", func(t *testing.T) {
		vec1 := []float32{1.0, 0.0, 0.0}
		vec2 := []float32{1.0, 0.0, 0.0}

		similarity, err := CalculateSimilarity(vec1, vec2, "cosine")
		require.NoError(t, err)
		assert.InDelta(t, 1.0, similarity, 0.01)
	})

	t.Run("cosine similarity - orthogonal vectors", func(t *testing.T) {
		vec1 := []float32{1.0, 0.0, 0.0}
		vec2 := []float32{0.0, 1.0, 0.0}

		similarity, err := CalculateSimilarity(vec1, vec2, "cosine")
		require.NoError(t, err)
		assert.InDelta(t, 0.0, similarity, 0.01)
	})

	t.Run("euclidean distance - identical vectors", func(t *testing.T) {
		vec1 := []float32{1.0, 0.0, 0.0}
		vec2 := []float32{1.0, 0.0, 0.0}

		similarity, err := CalculateSimilarity(vec1, vec2, "euclidean")
		require.NoError(t, err)
		assert.InDelta(t, 1.0, similarity, 0.01)
	})

	t.Run("euclidean distance - different vectors", func(t *testing.T) {
		vec1 := []float32{0.0, 0.0, 0.0}
		vec2 := []float32{1.0, 1.0, 1.0}

		similarity, err := CalculateSimilarity(vec1, vec2, "euclidean")
		require.NoError(t, err)
		// Distance is sqrt(3) ≈ 1.73, similarity is 1/(1+3) ≈ 0.25
		assert.True(t, similarity > 0 && similarity < 1)
	})

	t.Run("dot product", func(t *testing.T) {
		vec1 := []float32{1.0, 2.0, 3.0}
		vec2 := []float32{4.0, 5.0, 6.0}

		similarity, err := CalculateSimilarity(vec1, vec2, "dot_product")
		require.NoError(t, err)
		// 1*4 + 2*5 + 3*6 = 4 + 10 + 18 = 32
		assert.InDelta(t, 32.0, similarity, 0.01)
	})

	t.Run("default metric (cosine)", func(t *testing.T) {
		vec1 := []float32{1.0, 0.0, 0.0}
		vec2 := []float32{1.0, 0.0, 0.0}

		similarity, err := CalculateSimilarity(vec1, vec2, "")
		require.NoError(t, err)
		assert.InDelta(t, 1.0, similarity, 0.01)
	})

	t.Run("dimension mismatch", func(t *testing.T) {
		vec1 := []float32{1.0, 0.0}
		vec2 := []float32{1.0, 0.0, 0.0}

		_, err := CalculateSimilarity(vec1, vec2, "cosine")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "same dimensions")
	})

	t.Run("unsupported metric", func(t *testing.T) {
		vec1 := []float32{1.0, 0.0}
		vec2 := []float32{1.0, 0.0}

		_, err := CalculateSimilarity(vec1, vec2, "unsupported")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported similarity metric")
	})
}

func TestCosineSimilarity(t *testing.T) {
	t.Run("identical unit vectors", func(t *testing.T) {
		vec1 := []float32{1.0, 0.0}
		vec2 := []float32{1.0, 0.0}

		similarity := cosineSimilarity(vec1, vec2)
		assert.InDelta(t, 1.0, similarity, 0.01)
	})

	t.Run("opposite unit vectors", func(t *testing.T) {
		vec1 := []float32{1.0, 0.0}
		vec2 := []float32{-1.0, 0.0}

		similarity := cosineSimilarity(vec1, vec2)
		assert.InDelta(t, -1.0, similarity, 0.01)
	})
}

func TestEuclideanDistance(t *testing.T) {
	t.Run("same point", func(t *testing.T) {
		vec1 := []float32{1.0, 2.0, 3.0}
		vec2 := []float32{1.0, 2.0, 3.0}

		similarity := euclideanDistance(vec1, vec2)
		assert.InDelta(t, 1.0, similarity, 0.01)
	})
}

func TestDotProduct(t *testing.T) {
	t.Run("basic calculation", func(t *testing.T) {
		vec1 := []float32{1.0, 2.0}
		vec2 := []float32{3.0, 4.0}

		result := dotProduct(vec1, vec2)
		// 1*3 + 2*4 = 3 + 8 = 11
		assert.InDelta(t, 11.0, result, 0.01)
	})

	t.Run("zero vectors", func(t *testing.T) {
		vec1 := []float32{0.0, 0.0}
		vec2 := []float32{1.0, 1.0}

		result := dotProduct(vec1, vec2)
		assert.InDelta(t, 0.0, result, 0.01)
	})
}
