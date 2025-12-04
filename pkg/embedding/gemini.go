package embedding

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/auth/credentials"
	"google.golang.org/genai"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// Gemini embedding model constants
const (
	// ModelTextEmbedding004 is the latest text embedding model (768 dimensions)
	ModelTextEmbedding004 = "text-embedding-004"

	// ModelTextEmbedding005 is the newest text embedding model (768 dimensions)
	ModelTextEmbedding005 = "text-embedding-005"

	// ModelTextMultilingualEmbedding002 is for multilingual text (768 dimensions)
	ModelTextMultilingualEmbedding002 = "text-multilingual-embedding-002"

	// DefaultGeminiEmbeddingModel is the default embedding model
	DefaultGeminiEmbeddingModel = ModelTextEmbedding004
)

// GeminiEmbedder implements embedding generation using Google Gemini/Vertex AI API
type GeminiEmbedder struct {
	client          *genai.Client
	model           string
	config          EmbeddingConfig
	backend         genai.Backend
	projectID       string
	location        string
	credentialsFile string
	credentialsJSON []byte
	apiKey          string
	logger          logging.Logger
	taskType        string // Optional task type for better embeddings
}

// GeminiEmbedderOption represents an option for configuring the Gemini embedder
type GeminiEmbedderOption func(*GeminiEmbedder)

// WithGeminiModel sets the embedding model for the Gemini embedder
func WithGeminiModel(model string) GeminiEmbedderOption {
	return func(e *GeminiEmbedder) {
		e.model = model
		e.config.Model = model
	}
}

// WithGeminiAPIKey sets the API key for Gemini API backend
func WithGeminiAPIKey(apiKey string) GeminiEmbedderOption {
	return func(e *GeminiEmbedder) {
		e.apiKey = apiKey
	}
}

// WithGeminiBackend sets the backend for the Gemini embedder
func WithGeminiBackend(backend genai.Backend) GeminiEmbedderOption {
	return func(e *GeminiEmbedder) {
		e.backend = backend
	}
}

// WithGeminiProjectID sets the GCP project ID for Vertex AI backend
func WithGeminiProjectID(projectID string) GeminiEmbedderOption {
	return func(e *GeminiEmbedder) {
		e.projectID = projectID
	}
}

// WithGeminiLocation sets the GCP location for Vertex AI backend
func WithGeminiLocation(location string) GeminiEmbedderOption {
	return func(e *GeminiEmbedder) {
		e.location = location
	}
}

// WithGeminiCredentialsFile sets the path to a service account key file for Vertex AI authentication
func WithGeminiCredentialsFile(credentialsFile string) GeminiEmbedderOption {
	return func(e *GeminiEmbedder) {
		e.credentialsFile = credentialsFile
	}
}

// WithGeminiCredentialsJSON sets the service account key JSON bytes for Vertex AI authentication
func WithGeminiCredentialsJSON(credentialsJSON []byte) GeminiEmbedderOption {
	return func(e *GeminiEmbedder) {
		e.credentialsJSON = credentialsJSON
	}
}

// WithGeminiLogger sets the logger for the Gemini embedder
func WithGeminiLogger(logger logging.Logger) GeminiEmbedderOption {
	return func(e *GeminiEmbedder) {
		e.logger = logger
	}
}

// WithGeminiConfig sets the embedding configuration for the Gemini embedder
func WithGeminiConfig(config EmbeddingConfig) GeminiEmbedderOption {
	return func(e *GeminiEmbedder) {
		e.config = config
		if config.Model != "" {
			e.model = config.Model
		}
	}
}

// WithGeminiTaskType sets the task type for better embedding optimization
// Valid values: "RETRIEVAL_QUERY", "RETRIEVAL_DOCUMENT", "SEMANTIC_SIMILARITY",
// "CLASSIFICATION", "CLUSTERING", "QUESTION_ANSWERING", "FACT_VERIFICATION"
func WithGeminiTaskType(taskType string) GeminiEmbedderOption {
	return func(e *GeminiEmbedder) {
		e.taskType = taskType
	}
}

// WithGeminiClient injects an already initialized genai.Client
func WithGeminiClient(existing *genai.Client) GeminiEmbedderOption {
	return func(e *GeminiEmbedder) {
		e.client = existing
	}
}

// NewGeminiEmbedder creates a new Gemini embedder with the provided options
func NewGeminiEmbedder(ctx context.Context, options ...GeminiEmbedderOption) (*GeminiEmbedder, error) {
	// Create embedder with default options
	embedder := &GeminiEmbedder{
		model:    DefaultGeminiEmbeddingModel,
		backend:  genai.BackendGeminiAPI,
		location: "us-central1", // Default Vertex AI location
		logger:   logging.New(),
		config:   DefaultGeminiEmbeddingConfig(""),
	}

	// Apply options
	for _, option := range options {
		option(embedder)
	}

	// Update config model if not set
	if embedder.config.Model == "" {
		embedder.config.Model = embedder.model
	}

	// Validate that only one credential type is provided
	credentialTypesProvided := 0
	if embedder.credentialsFile != "" {
		credentialTypesProvided++
	}
	if len(embedder.credentialsJSON) > 0 {
		credentialTypesProvided++
	}

	if credentialTypesProvided > 1 {
		return nil, fmt.Errorf("only one credential type can be provided: choose between WithGeminiCredentialsFile or WithGeminiCredentialsJSON")
	}

	// If an existing client was injected, use it
	if embedder.client != nil {
		return embedder, nil
	}

	// Create the genai client
	clientConfig := &genai.ClientConfig{
		Backend: embedder.backend,
	}

	// Configure based on backend type
	switch embedder.backend {
	case genai.BackendGeminiAPI:
		if embedder.apiKey == "" {
			return nil, fmt.Errorf("API key is required for Gemini API backend")
		}
		clientConfig.APIKey = embedder.apiKey

	case genai.BackendVertexAI:
		// Validate that at least one authentication method is provided
		if embedder.projectID == "" && embedder.credentialsFile == "" && len(embedder.credentialsJSON) == 0 && embedder.apiKey == "" {
			return nil, fmt.Errorf("project ID, credentials file, credentials JSON, or API key are required for Vertex AI backend")
		}

		// Handle service account credentials
		if embedder.credentialsFile != "" {
			creds, err := credentials.DetectDefault(&credentials.DetectOptions{
				CredentialsFile: embedder.credentialsFile,
				Scopes: []string{
					"https://www.googleapis.com/auth/cloud-platform",
				},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to load credentials from file: %w", err)
			}
			clientConfig.Credentials = creds
		} else if len(embedder.credentialsJSON) > 0 {
			creds, err := credentials.DetectDefault(&credentials.DetectOptions{
				CredentialsJSON: embedder.credentialsJSON,
				Scopes: []string{
					"https://www.googleapis.com/auth/cloud-platform",
				},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to load credentials from JSON: %w", err)
			}
			clientConfig.Credentials = creds
		}

		// Set project and location if provided
		if embedder.projectID != "" {
			clientConfig.Project = embedder.projectID
			clientConfig.Location = embedder.location
		}

		// Set API key if provided (alternative authentication method)
		if embedder.apiKey != "" {
			clientConfig.APIKey = embedder.apiKey
		}
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	embedder.client = client

	return embedder, nil
}

// DefaultGeminiEmbeddingConfig returns a default configuration for Gemini embedding generation
func DefaultGeminiEmbeddingConfig(model string) EmbeddingConfig {
	if model == "" {
		model = DefaultGeminiEmbeddingModel
	}

	return EmbeddingConfig{
		Model:               model,
		Dimensions:          768, // Default dimensions for Gemini embedding models
		EncodingFormat:      "float",
		SimilarityMetric:    "cosine",
		SimilarityThreshold: 0.0,
	}
}

// Embed generates an embedding using Gemini API with default configuration
func (e *GeminiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return e.EmbedWithConfig(ctx, text, e.config)
}

// EmbedWithConfig generates an embedding using Gemini API with custom configuration
func (e *GeminiEmbedder) EmbedWithConfig(ctx context.Context, text string, config EmbeddingConfig) ([]float32, error) {
	model := config.Model
	if model == "" {
		model = e.model
	}

	// Build the embed content config
	embedConfig := &genai.EmbedContentConfig{}

	// Set task type if configured
	if e.taskType != "" {
		embedConfig.TaskType = e.taskType
	}

	// Set output dimensionality if specified and supported
	// #nosec G115 - dimensions are bounded by embedding model limits (typically < 10000)
	if config.Dimensions > 0 && config.Dimensions <= 32767 {
		dims := int32(config.Dimensions)
		embedConfig.OutputDimensionality = &dims
	}

	e.logger.Debug(ctx, "Generating embedding with Gemini", map[string]interface{}{
		"model":      model,
		"task_type":  e.taskType,
		"dimensions": config.Dimensions,
	})

	// Create content parts for embedding
	contents := []*genai.Content{
		{
			Parts: []*genai.Part{
				{Text: text},
			},
		},
	}

	// Generate embedding
	result, err := e.client.Models.EmbedContent(ctx, model, contents, embedConfig)
	if err != nil {
		e.logger.Error(ctx, "Failed to generate embedding", map[string]interface{}{
			"error": err.Error(),
			"model": model,
		})
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	if result == nil || len(result.Embeddings) == 0 || result.Embeddings[0] == nil || len(result.Embeddings[0].Values) == 0 {
		return nil, errors.New("no embedding data returned from Gemini API")
	}

	e.logger.Debug(ctx, "Successfully generated embedding", map[string]interface{}{
		"model":      model,
		"dimensions": len(result.Embeddings[0].Values),
	})

	return result.Embeddings[0].Values, nil
}

// EmbedBatch generates embeddings for multiple texts using default configuration
func (e *GeminiEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return e.EmbedBatchWithConfig(ctx, texts, e.config)
}

// EmbedBatchWithConfig generates embeddings for multiple texts with custom configuration
func (e *GeminiEmbedder) EmbedBatchWithConfig(ctx context.Context, texts []string, config EmbeddingConfig) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	model := config.Model
	if model == "" {
		model = e.model
	}

	// Build the embed content config
	embedConfig := &genai.EmbedContentConfig{}

	// Set task type if configured
	if e.taskType != "" {
		embedConfig.TaskType = e.taskType
	}

	// Set output dimensionality if specified and supported
	// #nosec G115 - dimensions are bounded by embedding model limits (typically < 10000)
	if config.Dimensions > 0 && config.Dimensions <= 32767 {
		dims := int32(config.Dimensions)
		embedConfig.OutputDimensionality = &dims
	}

	e.logger.Debug(ctx, "Generating batch embeddings with Gemini", map[string]interface{}{
		"model":      model,
		"task_type":  e.taskType,
		"dimensions": config.Dimensions,
		"batch_size": len(texts),
	})

	// Create content parts for batch embedding
	contents := make([]*genai.Content, len(texts))
	for i, text := range texts {
		contents[i] = &genai.Content{
			Parts: []*genai.Part{
				{Text: text},
			},
		}
	}

	// Generate embeddings - EmbedContent handles multiple contents
	result, err := e.client.Models.EmbedContent(ctx, model, contents, embedConfig)
	if err != nil {
		e.logger.Error(ctx, "Failed to generate batch embeddings", map[string]interface{}{
			"error":      err.Error(),
			"model":      model,
			"batch_size": len(texts),
		})
		return nil, fmt.Errorf("failed to generate batch embeddings: %w", err)
	}

	if result == nil || len(result.Embeddings) == 0 {
		return nil, errors.New("no embedding data returned from Gemini API")
	}

	// Extract embeddings
	embeddings := make([][]float32, len(result.Embeddings))
	for i, emb := range result.Embeddings {
		if emb == nil || len(emb.Values) == 0 {
			return nil, fmt.Errorf("empty embedding at index %d", i)
		}
		embeddings[i] = emb.Values
	}

	e.logger.Debug(ctx, "Successfully generated batch embeddings", map[string]interface{}{
		"model":      model,
		"batch_size": len(embeddings),
	})

	return embeddings, nil
}

// CalculateSimilarity calculates the similarity between two embeddings
func (e *GeminiEmbedder) CalculateSimilarity(vec1, vec2 []float32, metric string) (float32, error) {
	if metric == "" {
		metric = e.config.SimilarityMetric
	}
	return CalculateSimilarity(vec1, vec2, metric)
}

// GetConfig returns the current configuration
func (e *GeminiEmbedder) GetConfig() EmbeddingConfig {
	return e.config
}

// GetModel returns the model name being used
func (e *GeminiEmbedder) GetModel() string {
	return e.model
}
