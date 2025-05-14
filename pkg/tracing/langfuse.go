package tracing

import (
	"context"
	"fmt"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/henomis/langfuse-go"
	"github.com/henomis/langfuse-go/model"
)

// LangfuseTracer implements tracing using Langfuse
type LangfuseTracer struct {
	client      *langfuse.Langfuse
	enabled     bool
	environment string
	secretKey   string
	publicKey   string
	host        string
}

// LangfuseConfig contains configuration for Langfuse
type LangfuseConfig struct {
	// Enabled determines whether Langfuse tracing is enabled
	Enabled bool

	// SecretKey is the Langfuse secret key
	SecretKey string

	// PublicKey is the Langfuse public key
	PublicKey string

	// Host is the Langfuse host (optional)
	Host string

	// Environment is the environment name (e.g., "production", "staging")
	Environment string
}

// NewLangfuseTracer creates a new Langfuse tracer
func NewLangfuseTracer(customConfig ...LangfuseConfig) (*LangfuseTracer, error) {
	// Get global configuration
	cfg := config.Get()

	// Use custom config if provided, otherwise use global config
	var tracerConfig LangfuseConfig
	if len(customConfig) > 0 {
		tracerConfig = customConfig[0]
	} else {
		tracerConfig = LangfuseConfig{
			Enabled:     cfg.Tracing.Langfuse.Enabled,
			SecretKey:   cfg.Tracing.Langfuse.SecretKey,
			PublicKey:   cfg.Tracing.Langfuse.PublicKey,
			Host:        cfg.Tracing.Langfuse.Host,
			Environment: cfg.Tracing.Langfuse.Environment,
		}
	}

	if !tracerConfig.Enabled {
		return &LangfuseTracer{
			enabled: false,
		}, nil
	}

	// Create Langfuse client
	client := langfuse.New(context.Background())

	return &LangfuseTracer{
		client:      client,
		enabled:     true,
		environment: tracerConfig.Environment,
		secretKey:   tracerConfig.SecretKey,
		publicKey:   tracerConfig.PublicKey,
		host:        tracerConfig.Host,
	}, nil
}

// TraceGeneration traces an LLM generation
func (t *LangfuseTracer) TraceGeneration(ctx context.Context, modelName string, prompt string, response string, startTime time.Time, endTime time.Time, metadata map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", nil
	}

	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err == nil && orgID != "" {
		metadata["organization_id"] = orgID
	}
	metadata["environment"] = t.environment

	// Convert metadata to model.M
	metadataM := make(model.M)
	for k, v := range metadata {
		metadataM[k] = v
	}

	// Create generation
	generation := &model.Generation{
		Name:      fmt.Sprintf("generation-%d", time.Now().UnixNano()),
		StartTime: &startTime,
		EndTime:   &endTime,
		Model:     modelName,
		Input: []model.M{
			{
				"prompt": prompt,
			},
		},
		Output: model.M{
			"completion": response,
		},
		Metadata: metadataM,
	}

	var id string
	generationID, err := t.client.Generation(generation, &id)
	if err != nil {
		return "", fmt.Errorf("failed to create Langfuse generation: %w", err)
	}

	return generationID.ID, nil
}

// TraceSpan traces a span of execution
func (t *LangfuseTracer) TraceSpan(ctx context.Context, name string, startTime time.Time, endTime time.Time, metadata map[string]interface{}, parentID string) (string, error) {
	if !t.enabled {
		return "", nil
	}

	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err == nil && orgID != "" {
		metadata["organization_id"] = orgID
	}
	metadata["environment"] = t.environment

	// Create span
	span := &model.Span{
		Name:      name,
		StartTime: &startTime,
		EndTime:   &endTime,
		Metadata:  metadata,
	}
	if parentID != "" {
		span.ParentObservationID = parentID
	}

	var id string
	spanID, err := t.client.Span(span, &id)
	if err != nil {
		return "", fmt.Errorf("failed to create Langfuse span: %w", err)
	}

	return spanID.ID, nil
}

// TraceEvent traces an event
func (t *LangfuseTracer) TraceEvent(ctx context.Context, name string, input interface{}, output interface{}, level string, metadata map[string]interface{}, parentID string) (string, error) {
	if !t.enabled {
		return "", nil
	}

	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err == nil && orgID != "" {
		metadata["organization_id"] = orgID
	}
	metadata["environment"] = t.environment

	// Create event
	event := &model.Event{
		Name:     name,
		Input:    input,
		Output:   output,
		Level:    model.ObservationLevel(level),
		Metadata: metadata,
	}
	if parentID != "" {
		event.ParentObservationID = parentID
	}

	var id string
	eventID, err := t.client.Event(event, &id)
	if err != nil {
		return "", fmt.Errorf("failed to create Langfuse event: %w", err)
	}

	return eventID.ID, nil
}

// Flush flushes the Langfuse client
func (t *LangfuseTracer) Flush() error {
	if !t.enabled {
		return nil
	}

	// Flush doesn't return a value
	t.client.Flush(context.Background())
	return nil
}

// LLMMiddleware implements middleware for LLM calls with Langfuse tracing
type LLMMiddleware struct {
	llm    interfaces.LLM
	tracer *LangfuseTracer
}

// NewLLMMiddleware creates a new LLM middleware with Langfuse tracing
func NewLLMMiddleware(llm interfaces.LLM, tracer *LangfuseTracer) *LLMMiddleware {
	return &LLMMiddleware{
		llm:    llm,
		tracer: tracer,
	}
}

// Generate generates text from a prompt with Langfuse tracing
func (m *LLMMiddleware) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	startTime := time.Now()

	// Call the underlying LLM
	response, err := m.llm.Generate(ctx, prompt, options...)

	endTime := time.Now()

	// Extract model from options
	model := "unknown"
	// Create metadata from options
	metadata := map[string]interface{}{
		"options": fmt.Sprintf("%v", options),
	}

	// Trace the generation
	if err == nil {
		_, traceErr := m.tracer.TraceGeneration(ctx, model, prompt, response, startTime, endTime, metadata)
		if traceErr != nil {
			// Log the error but don't fail the request
			fmt.Printf("Failed to trace generation: %v\n", traceErr)
		}
	} else {
		// Trace error
		errorMetadata := map[string]interface{}{
			"options": fmt.Sprintf("%v", options),
			"error":   err.Error(),
		}
		_, traceErr := m.tracer.TraceEvent(ctx, "llm_error", prompt, nil, "error", errorMetadata, "")
		if traceErr != nil {
			// Log the error but don't fail the request
			fmt.Printf("Failed to trace error: %v\n", traceErr)
		}
	}

	return response, err
}

// Name implements interfaces.LLM.Name
func (m *LLMMiddleware) Name() string {
	return m.llm.Name()
}
