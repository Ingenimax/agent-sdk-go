package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// BedrockConfig contains configuration for AWS Bedrock
// This mirrors the VertexConfig structure for consistency
type BedrockConfig struct {
	Enabled bool
	Region  string

	// Internal fields
	awsConfig aws.Config
	client    *bedrockruntime.Client
	logger    logging.Logger
}

// NewBedrockConfigWithAWSConfig creates a new BedrockConfig from an existing AWS config
// This is the primary way to configure Bedrock - users configure credentials and settings
// through the aws.Config itself using config.LoadDefaultConfig() or other AWS SDK methods
func NewBedrockConfigWithAWSConfig(ctx context.Context, awsConfig aws.Config) (*BedrockConfig, error) {
	if awsConfig.Region == "" {
		return nil, fmt.Errorf("region is required in AWS config")
	}

	// Create Bedrock Runtime client from existing config
	client := bedrockruntime.NewFromConfig(awsConfig)

	bedrockConfig := &BedrockConfig{
		Enabled:   true,
		Region:    awsConfig.Region,
		awsConfig: awsConfig,
		client:    client,
		logger:    logging.New(),
	}

	bedrockConfig.logger.Info(ctx, "Configured AWS Bedrock with existing AWS config", map[string]interface{}{
		"region": awsConfig.Region,
	})

	return bedrockConfig, nil
}

// BedrockRequest represents the request format for AWS Bedrock (uses standard Anthropic format)
type BedrockRequest struct {
	MaxTokens        int            `json:"max_tokens"`
	Messages         []Message      `json:"messages"`
	System           string         `json:"system,omitempty"`
	Tools            []Tool         `json:"tools,omitempty"`
	ToolChoice       interface{}    `json:"tool_choice,omitempty"`
	Temperature      float64        `json:"temperature,omitempty"`
	TopP             float64        `json:"top_p,omitempty"`
	TopK             int            `json:"top_k,omitempty"`
	StopSequences    []string       `json:"stop_sequences,omitempty"`
	AnthropicVersion string         `json:"anthropic_version"`
	Thinking         *ReasoningSpec `json:"thinking,omitempty"` // Extended thinking support for Claude models
}

// TransformRequest converts an Anthropic CompletionRequest to Bedrock format
func (bc *BedrockConfig) TransformRequest(req *CompletionRequest) (*BedrockRequest, error) {
	if !bc.Enabled {
		return nil, fmt.Errorf("bedrock is not enabled")
	}

	bedrockReq := &BedrockRequest{
		MaxTokens:        req.MaxTokens,
		Messages:         req.Messages,
		System:           req.System,
		Tools:            req.Tools,
		ToolChoice:       req.ToolChoice,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		TopK:             req.TopK,
		StopSequences:    req.StopSequences,
		AnthropicVersion: "bedrock-2023-05-31", // Required for Bedrock
		Thinking:         req.Thinking,         // Extended thinking support
	}

	return bedrockReq, nil
}

// InvokeModel invokes a Bedrock model using the AWS SDK (non-streaming)
func (bc *BedrockConfig) InvokeModel(ctx context.Context, modelID string, req *CompletionRequest) (*CompletionResponse, error) {
	if !bc.Enabled {
		return nil, fmt.Errorf("bedrock is not enabled")
	}

	// Transform request to Bedrock format
	bedrockReq, err := bc.TransformRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to transform request: %w", err)
	}

	// Marshal request to JSON
	requestBody, err := json.Marshal(bedrockReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	bc.logger.Debug(ctx, "Invoking Bedrock model", map[string]interface{}{
		"modelID":     modelID,
		"region":      bc.Region,
		"requestSize": len(requestBody),
	})

	// Invoke the model using AWS SDK
	output, err := bc.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelID),
		Body:        requestBody,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	})
	if err != nil {
		bc.logger.Error(ctx, "Failed to invoke Bedrock model", map[string]interface{}{
			"error":   err.Error(),
			"modelID": modelID,
			"region":  bc.Region,
		})
		return nil, fmt.Errorf("failed to invoke Bedrock model: %w", err)
	}

	// Parse response (Bedrock returns standard Anthropic response format)
	var resp CompletionResponse
	if err := json.Unmarshal(output.Body, &resp); err != nil {
		bc.logger.Error(ctx, "Failed to parse Bedrock response", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to parse Bedrock response: %w", err)
	}

	bc.logger.Debug(ctx, "Successfully received response from Bedrock", map[string]interface{}{
		"modelID":      modelID,
		"stopReason":   resp.StopReason,
		"inputTokens":  resp.Usage.InputTokens,
		"outputTokens": resp.Usage.OutputTokens,
	})

	return &resp, nil
}

// InvokeModelStream invokes a Bedrock model with streaming using AWS SDK
func (bc *BedrockConfig) InvokeModelStream(ctx context.Context, modelID string, req *CompletionRequest) (*bedrockruntime.InvokeModelWithResponseStreamOutput, error) {
	if !bc.Enabled {
		return nil, fmt.Errorf("bedrock is not enabled")
	}

	// Transform request to Bedrock format
	bedrockReq, err := bc.TransformRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to transform request: %w", err)
	}

	// Marshal request to JSON
	requestBody, err := json.Marshal(bedrockReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	bc.logger.Debug(ctx, "Invoking Bedrock model with streaming", map[string]interface{}{
		"modelID": modelID,
		"region":  bc.Region,
	})

	// Invoke the model with streaming using AWS SDK
	output, err := bc.client.InvokeModelWithResponseStream(ctx, &bedrockruntime.InvokeModelWithResponseStreamInput{
		ModelId:     aws.String(modelID),
		Body:        requestBody,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	})
	if err != nil {
		bc.logger.Error(ctx, "Failed to invoke Bedrock model with streaming", map[string]interface{}{
			"error":   err.Error(),
			"modelID": modelID,
			"region":  bc.Region,
		})
		return nil, fmt.Errorf("failed to invoke Bedrock model with streaming: %w", err)
	}

	return output, nil
}

