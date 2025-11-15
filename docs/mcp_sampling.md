# MCP Sampling Support

## Overview

The MCP Sampling feature provides forward-compatible support for the Model Context Protocol 2025 Sampling specification. It enables agents to request language model completions directly from MCP servers, with sophisticated model selection, parameter control, and content type support for text, image, and audio inputs.

## Key Features

- **MCP 2025 Sampling Specification**: Complete implementation of the latest sampling protocol
- **Model Preferences**: Intelligent model selection based on cost, speed, and intelligence priorities
- **Content Types**: Support for text, image, audio, and multimodal inputs
- **Parameter Control**: Fine-grained control over temperature, tokens, stop sequences
- **Forward Compatibility**: Ready for Go SDK sampling support when available
- **High-Level Utilities**: Convenient methods for common sampling operations

## Architecture

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐
│   Agent     │───▶│ SamplingMgr  │───▶│ MCP Server  │
│             │    │              │    │             │
│ - CreateMsg │    │ - Preferences│    │ - LLM       │
│ - Generate  │    │ - Parameters │    │ - Sampling  │
│ - Analyze   │    │ - Content    │    │ - Models    │
└─────────────┘    └──────────────┘    └─────────────┘
       │                    │
       └────────────────────┼─────────────────
                            ▼
                   ┌─────────────┐
                   │ Model       │
                   │ Selection   │
                   │             │
                   │ - Cost      │
                   │ - Speed     │
                   │ - Quality   │
                   └─────────────┘
```

## Usage Examples

### Basic Text Generation

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/mcp"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func main() {
    ctx := context.Background()

    // Build agent with sampling-capable MCP servers
    builder := mcp.NewBuilder().
        AddHTTPServerWithAuth("openai-mcp", "https://api.openai.com/mcp", "your-api-key").
        AddHTTPServerWithAuth("anthropic-mcp", "https://api.anthropic.com/mcp", "your-api-key")

    servers, _, err := builder.Build(ctx)
    if err != nil {
        // Note: This is forward-compatible - will work when sampling is available
        log.Printf("Sampling demo mode: %v", err)
        servers = []interfaces.MCPServer{} // Empty for demo
    }

    // Create sampling manager
    samplingManager := mcp.NewSamplingManager(servers)

    // Simple text generation
    response, err := samplingManager.CreateTextMessage(ctx, &mcp.SamplingTextOptions{
        Prompt:      "Explain the benefits of the Model Context Protocol in 100 words",
        MaxTokens:   150,
        Temperature: 0.7,
        ModelPreferences: &mcp.ModelPreferences{
            IntelligencePriority: 0.8, // Prioritize quality
            SpeedPriority:       0.6, // Moderate speed importance
            CostPriority:        0.4, // Lower cost importance
        },
    })

    if err != nil {
        log.Printf("Sampling not yet available in Go SDK: %v", err)
        // This will work when the Go MCP SDK adds sampling support
        demonstrateSamplingCapabilities()
        return
    }

    fmt.Printf("Generated response: %s\n", response.Content.Text)
    fmt.Printf("Model used: %s\n", response.Model)
    fmt.Printf("Tokens used: %d\n", response.Usage.TotalTokens)
}
```

### Multi-turn Conversation

```go
func multiTurnConversation(ctx context.Context, samplingManager *mcp.SamplingManager) {
    // Build conversation context
    messages := []mcp.SamplingMessage{
        {
            Role: "system",
            Content: mcp.SamplingContent{
                Type: "text",
                Text: "You are a helpful AI assistant specialized in software development.",
            },
        },
        {
            Role: "user",
            Content: mcp.SamplingContent{
                Type: "text",
                Text: "I'm building a Go application with MCP. What are the best practices?",
            },
        },
    }

    // Create conversation
    response, err := samplingManager.CreateConversation(ctx, &mcp.SamplingConversationOptions{
        Messages:     messages,
        SystemPrompt: "Provide practical, actionable advice with code examples.",
        MaxTokens:    500,
        Temperature:  0.7,
        ModelPreferences: &mcp.ModelPreferences{
            IntelligencePriority: 0.9, // High intelligence for technical advice
            SpeedPriority:       0.5,
            CostPriority:        0.3,
        },
    })

    if err != nil {
        log.Printf("Conversation sampling: %v", err)
        return
    }

    fmt.Printf("AI Response: %s\n", response.Content.Text)

    // Continue conversation
    messages = append(messages, mcp.SamplingMessage{
        Role: "assistant",
        Content: mcp.SamplingContent{
            Type: "text",
            Text: response.Content.Text,
        },
    })

    messages = append(messages, mcp.SamplingMessage{
        Role: "user",
        Content: mcp.SamplingContent{
            Type: "text",
            Text: "Can you show me a specific example of error handling?",
        },
    })

    // Generate follow-up response
    followUp, err := samplingManager.CreateConversation(ctx, &mcp.SamplingConversationOptions{
        Messages:    messages,
        MaxTokens:   300,
        Temperature: 0.6, // Slightly more focused for code examples
    })

    if err != nil {
        log.Printf("Follow-up sampling: %v", err)
        return
    }

    fmt.Printf("Follow-up: %s\n", followUp.Content.Text)
}
```

### Code Generation with Sampling

```go
func codeGenerationSampling(ctx context.Context, samplingManager *mcp.SamplingManager) {
    // Specialized code generation
    response, err := samplingManager.CreateCodeGeneration(ctx, &mcp.SamplingCodeOptions{
        Prompt:       "Create a Go function that validates email addresses with proper error handling",
        Language:     "go",
        Style:        "idiomatic",
        MaxTokens:    400,
        Temperature:  0.3, // Lower temperature for more consistent code
        ModelPreferences: &mcp.ModelPreferences{
            IntelligencePriority: 0.9, // High intelligence for code quality
            SpeedPriority:       0.7, // Fast for development workflow
            CostPriority:        0.4,
            Hints: []mcp.ModelHint{
                {Name: "code-specialized"}, // Prefer code-specialized models
                {Name: "go-expertise"},     // Prefer Go-experienced models
            },
        },
        StopSequences: []string{"```", "\n\n//", "func main"},
    })

    if err != nil {
        log.Printf("Code generation sampling: %v", err)
        return
    }

    fmt.Printf("Generated Code:\n```go\n%s\n```\n", response.Content.Text)

    // Validate code structure
    if response.Metadata != nil {
        if codeMetadata, exists := response.Metadata["code_analysis"]; exists {
            fmt.Printf("Code Analysis: %v\n", codeMetadata)
        }
    }
}
```

### Image Analysis with Sampling

```go
func imageAnalysisSampling(ctx context.Context, samplingManager *mcp.SamplingManager) {
    // Load image data (for demo purposes, using placeholder)
    imageData := []byte("base64-encoded-image-data")

    // Analyze image with text prompt
    response, err := samplingManager.CreateImageAnalysisMessage(ctx, &mcp.SamplingImageOptions{
        ImageData:   imageData,
        ImageType:   "image/jpeg",
        Prompt:      "Describe what you see in this image and identify any text or important details",
        MaxTokens:   300,
        Temperature: 0.5,
        ModelPreferences: &mcp.ModelPreferences{
            IntelligencePriority: 0.8,
            SpeedPriority:       0.6,
            CostPriority:        0.5,
            Hints: []mcp.ModelHint{
                {Name: "vision-capable"},
                {Name: "multimodal"},
            },
        },
    })

    if err != nil {
        log.Printf("Image analysis sampling: %v", err)
        return
    }

    fmt.Printf("Image Analysis: %s\n", response.Content.Text)

    // Check for detected objects/text
    if response.Metadata != nil {
        if objects, exists := response.Metadata["detected_objects"]; exists {
            fmt.Printf("Detected Objects: %v\n", objects)
        }
        if text, exists := response.Metadata["extracted_text"]; exists {
            fmt.Printf("Extracted Text: %v\n", text)
        }
    }
}
```

## SamplingManager API

### Core Methods

```go
type SamplingManager struct {
    servers []interfaces.MCPServer
    logger  logging.Logger
}

// Create new sampling manager
func NewSamplingManager(servers []interfaces.MCPServer) *SamplingManager

// Simple text generation
func (sm *SamplingManager) CreateTextMessage(ctx context.Context, options *SamplingTextOptions) (*MCPSamplingResponse, error)

// Multi-turn conversation
func (sm *SamplingManager) CreateConversation(ctx context.Context, options *SamplingConversationOptions) (*MCPSamplingResponse, error)

// Code generation
func (sm *SamplingManager) CreateCodeGeneration(ctx context.Context, options *SamplingCodeOptions) (*MCPSamplingResponse, error)

// Document summarization
func (sm *SamplingManager) CreateSummary(ctx context.Context, options *SamplingSummaryOptions) (*MCPSamplingResponse, error)
```

### Advanced Methods

```go
// Image analysis
func (sm *SamplingManager) CreateImageAnalysisMessage(ctx context.Context, options *SamplingImageOptions) (*MCPSamplingResponse, error)

// Custom sampling request
func (sm *SamplingManager) CreateCustomMessage(ctx context.Context, request *MCPSamplingRequest) (*MCPSamplingResponse, error)

// Batch sampling for multiple requests
func (sm *SamplingManager) CreateBatchMessages(ctx context.Context, requests []*MCPSamplingRequest) ([]*MCPSamplingResponse, error)

// Streaming sampling for real-time generation
func (sm *SamplingManager) CreateStreamingMessage(ctx context.Context, options *SamplingStreamOptions) (<-chan *MCPSamplingChunk, error)
```

### Model Management

```go
// Get available models from all servers
func (sm *SamplingManager) GetAvailableModels(ctx context.Context) ([]ModelInfo, error)

// Select best model based on preferences
func (sm *SamplingManager) SelectModel(preferences *ModelPreferences, availableModels []ModelInfo) (ModelInfo, error)

// Get model capabilities
func (sm *SamplingManager) GetModelCapabilities(ctx context.Context, modelName string) (*ModelCapabilities, error)
```

## Data Structures

### Sampling Request

```go
type MCPSamplingRequest struct {
    Messages     []SamplingMessage `json:"messages"`
    SystemPrompt string           `json:"systemPrompt,omitempty"`
    MaxTokens    *int             `json:"maxTokens,omitempty"`
    Temperature  *float64         `json:"temperature,omitempty"`

    // Model selection
    ModelPreferences *ModelPreferences `json:"modelPreferences,omitempty"`

    // Control parameters
    StopSequences   []string `json:"stopSequences,omitempty"`
    TopP           *float64 `json:"topP,omitempty"`
    TopK           *int     `json:"topK,omitempty"`
    FrequencyPenalty *float64 `json:"frequencyPenalty,omitempty"`
    PresencePenalty  *float64 `json:"presencePenalty,omitempty"`

    // Context control
    IncludeContext string `json:"includeContext,omitempty"`
}
```

### Sampling Message

```go
type SamplingMessage struct {
    Role    string          `json:"role"`    // "system", "user", "assistant"
    Content SamplingContent `json:"content"`
}

type SamplingContent struct {
    Type     string `json:"type"`               // "text", "image", "audio"
    Text     string `json:"text,omitempty"`
    Data     []byte `json:"data,omitempty"`     // For binary content
    MimeType string `json:"mimeType,omitempty"`

    // Multimodal content
    ImageURL string `json:"imageUrl,omitempty"`
    AudioURL string `json:"audioUrl,omitempty"`
}
```

### Model Preferences

```go
type ModelPreferences struct {
    // Priority weights (0.0 to 1.0)
    CostPriority        float64     `json:"costPriority"`        // Prefer cheaper models
    SpeedPriority       float64     `json:"speedPriority"`       // Prefer faster models
    IntelligencePriority float64    `json:"intelligencePriority"` // Prefer smarter models

    // Model hints for selection
    Hints []ModelHint `json:"hints,omitempty"`

    // Explicit model preferences
    PreferredModels []string `json:"preferredModels,omitempty"`
    AvoidModels     []string `json:"avoidModels,omitempty"`

    // Capability requirements
    RequiredCapabilities []string `json:"requiredCapabilities,omitempty"`
}

type ModelHint struct {
    Name   string `json:"name"`
    Value  string `json:"value,omitempty"`
    Weight float64 `json:"weight,omitempty"`
}
```

### Sampling Response

```go
type MCPSamplingResponse struct {
    Content SamplingContent    `json:"content"`
    Model   string            `json:"model"`
    Usage   TokenUsage        `json:"usage"`

    // Response metadata
    Metadata         map[string]interface{} `json:"metadata,omitempty"`
    FinishReason     string                `json:"finishReason,omitempty"`
    ResponseTime     time.Duration         `json:"responseTime,omitempty"`
    ServerID         string               `json:"serverID,omitempty"`

    // Quality metrics
    Confidence       *float64             `json:"confidence,omitempty"`
    SafetyScore      *float64             `json:"safetyScore,omitempty"`
}

type TokenUsage struct {
    PromptTokens     int `json:"promptTokens"`
    CompletionTokens int `json:"completionTokens"`
    TotalTokens      int `json:"totalTokens"`
}
```

## High-Level Option Types

### Text Generation Options

```go
type SamplingTextOptions struct {
    Prompt      string  `json:"prompt"`
    MaxTokens   int     `json:"maxTokens"`
    Temperature float64 `json:"temperature"`

    ModelPreferences *ModelPreferences `json:"modelPreferences,omitempty"`
    StopSequences    []string         `json:"stopSequences,omitempty"`

    // Context
    SystemPrompt string `json:"systemPrompt,omitempty"`
    Context      string `json:"context,omitempty"`
}
```

### Conversation Options

```go
type SamplingConversationOptions struct {
    Messages     []SamplingMessage `json:"messages"`
    SystemPrompt string           `json:"systemPrompt,omitempty"`
    MaxTokens    int              `json:"maxTokens"`
    Temperature  float64          `json:"temperature"`

    ModelPreferences *ModelPreferences `json:"modelPreferences,omitempty"`

    // Conversation control
    MemoryLength     int    `json:"memoryLength,omitempty"`     // How many messages to remember
    PersonalityPrompt string `json:"personalityPrompt,omitempty"`
}
```

### Code Generation Options

```go
type SamplingCodeOptions struct {
    Prompt      string  `json:"prompt"`
    Language    string  `json:"language"`           // "go", "python", "javascript", etc.
    Style       string  `json:"style,omitempty"`    // "idiomatic", "verbose", "minimal"
    MaxTokens   int     `json:"maxTokens"`
    Temperature float64 `json:"temperature"`

    ModelPreferences *ModelPreferences `json:"modelPreferences,omitempty"`
    StopSequences    []string         `json:"stopSequences,omitempty"`

    // Code-specific options
    IncludeComments bool     `json:"includeComments,omitempty"`
    IncludeTests    bool     `json:"includeTests,omitempty"`
    Framework       string   `json:"framework,omitempty"`
    Dependencies    []string `json:"dependencies,omitempty"`
}
```

### Image Analysis Options

```go
type SamplingImageOptions struct {
    ImageData   []byte  `json:"imageData"`
    ImageType   string  `json:"imageType"`     // "image/jpeg", "image/png", etc.
    Prompt      string  `json:"prompt"`
    MaxTokens   int     `json:"maxTokens"`
    Temperature float64 `json:"temperature"`

    ModelPreferences *ModelPreferences `json:"modelPreferences,omitempty"`

    // Image analysis options
    DetectObjects bool `json:"detectObjects,omitempty"`
    ExtractText   bool `json:"extractText,omitempty"`
    DescribeScene bool `json:"describeScene,omitempty"`
}
```

### Summary Options

```go
type SamplingSummaryOptions struct {
    Text        string  `json:"text"`
    SummaryType string  `json:"summaryType"`    // "brief", "detailed", "bullet-points"
    MaxTokens   int     `json:"maxTokens"`
    Temperature float64 `json:"temperature"`

    ModelPreferences *ModelPreferences `json:"modelPreferences,omitempty"`

    // Summary control
    KeyPoints    int    `json:"keyPoints,omitempty"`     // Number of key points to extract
    Perspective  string `json:"perspective,omitempty"`   // "technical", "business", "general"
    IncludeTopic bool   `json:"includeTopic,omitempty"`  // Include topic classification
}
```

## Model Selection

### Model Information

```go
type ModelInfo struct {
    Name         string            `json:"name"`
    Provider     string            `json:"provider"`
    Capabilities []string          `json:"capabilities"`

    // Performance characteristics
    CostScore       float64 `json:"costScore"`        // Lower is cheaper (0-1)
    SpeedScore      float64 `json:"speedScore"`       // Higher is faster (0-1)
    IntelligenceScore float64 `json:"intelligenceScore"` // Higher is smarter (0-1)

    // Technical specs
    MaxTokens       int      `json:"maxTokens"`
    ContextWindow   int      `json:"contextWindow"`
    SupportedTypes  []string `json:"supportedTypes"`   // "text", "image", "audio"

    // Metadata
    Version     string            `json:"version,omitempty"`
    Description string            `json:"description,omitempty"`
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type ModelCapabilities struct {
    TextGeneration  bool `json:"textGeneration"`
    CodeGeneration  bool `json:"codeGeneration"`
    ImageAnalysis   bool `json:"imageAnalysis"`
    AudioProcessing bool `json:"audioProcessing"`
    Summarization   bool `json:"summarization"`
    Translation     bool `json:"translation"`

    // Advanced capabilities
    FunctionCalling bool `json:"functionCalling"`
    ToolUse        bool `json:"toolUse"`
    Reasoning      bool `json:"reasoning"`
    Mathematics    bool `json:"mathematics"`
}
```

### Model Selection Algorithm

```go
func (sm *SamplingManager) SelectModel(preferences *ModelPreferences, availableModels []ModelInfo) (ModelInfo, error) {
    if len(availableModels) == 0 {
        return ModelInfo{}, fmt.Errorf("no models available")
    }

    // Score each model based on preferences
    type scoredModel struct {
        model ModelInfo
        score float64
    }

    var scored []scoredModel
    for _, model := range availableModels {
        // Check required capabilities
        if !hasRequiredCapabilities(model, preferences.RequiredCapabilities) {
            continue
        }

        // Check avoid list
        if contains(preferences.AvoidModels, model.Name) {
            continue
        }

        // Calculate composite score
        score := calculateModelScore(model, preferences)
        scored = append(scored, scoredModel{model, score})
    }

    if len(scored) == 0 {
        return ModelInfo{}, fmt.Errorf("no suitable models found")
    }

    // Sort by score (descending)
    sort.Slice(scored, func(i, j int) bool {
        return scored[i].score > scored[j].score
    })

    // Apply hints for final selection
    if len(preferences.Hints) > 0 {
        for _, model := range scored {
            if matchesHints(model.model, preferences.Hints) {
                return model.model, nil
            }
        }
    }

    // Return highest scoring model
    return scored[0].model, nil
}

func calculateModelScore(model ModelInfo, preferences *ModelPreferences) float64 {
    costScore := (1.0 - model.CostScore) * preferences.CostPriority
    speedScore := model.SpeedScore * preferences.SpeedPriority
    intelligenceScore := model.IntelligenceScore * preferences.IntelligencePriority

    // Normalize weights
    totalWeight := preferences.CostPriority + preferences.SpeedPriority + preferences.IntelligencePriority
    if totalWeight == 0 {
        totalWeight = 1.0
    }

    return (costScore + speedScore + intelligenceScore) / totalWeight
}
```

## Error Handling

### Sampling-Specific Errors

```go
type SamplingError struct {
    Code        string `json:"code"`
    Message     string `json:"message"`
    ServerID    string `json:"serverID,omitempty"`
    ModelName   string `json:"modelName,omitempty"`
    RetryableAfter *time.Duration `json:"retryableAfter,omitempty"`
}

func (se *SamplingError) Error() string {
    return fmt.Sprintf("sampling error: %s - %s", se.Code, se.Message)
}

func (se *SamplingError) IsRetryable() bool {
    switch se.Code {
    case "rate_limited", "server_overloaded", "temporary_unavailable":
        return true
    case "invalid_request", "unauthorized", "model_not_found":
        return false
    default:
        return false
    }
}

// Handle sampling errors with intelligent retry
func handleSamplingError(err error, options *SamplingTextOptions) (*MCPSamplingResponse, error) {
    samplingErr, ok := err.(*SamplingError)
    if !ok {
        return nil, err
    }

    switch samplingErr.Code {
    case "rate_limited":
        if samplingErr.RetryableAfter != nil {
            time.Sleep(*samplingErr.RetryableAfter)
            // Retry with same options
        }

    case "context_length_exceeded":
        // Reduce max tokens or split request
        if options.MaxTokens > 100 {
            options.MaxTokens = int(float64(options.MaxTokens) * 0.8)
            // Retry with reduced tokens
        }

    case "model_not_available":
        // Try fallback model selection
        if options.ModelPreferences != nil {
            options.ModelPreferences.AvoidModels = append(
                options.ModelPreferences.AvoidModels,
                samplingErr.ModelName)
            // Retry with model excluded
        }

    case "content_filtered":
        // Adjust temperature or rephrase prompt
        if options.Temperature > 0.1 {
            options.Temperature *= 0.8
            // Retry with lower temperature
        }
    }

    return nil, err
}
```

### Fallback Strategies

```go
func (sm *SamplingManager) CreateTextMessageWithFallback(ctx context.Context, options *SamplingTextOptions) (*MCPSamplingResponse, error) {
    // Try primary approach
    response, err := sm.CreateTextMessage(ctx, options)
    if err == nil {
        return response, nil
    }

    // Apply fallback strategies
    fallbackOptions := *options

    // Strategy 1: Reduce complexity
    if fallbackOptions.MaxTokens > 50 {
        fallbackOptions.MaxTokens = min(fallbackOptions.MaxTokens/2, 500)
        if response, err := sm.CreateTextMessage(ctx, &fallbackOptions); err == nil {
            return response, nil
        }
    }

    // Strategy 2: Lower model requirements
    if fallbackOptions.ModelPreferences != nil {
        fallbackOptions.ModelPreferences.IntelligencePriority *= 0.5
        fallbackOptions.ModelPreferences.CostPriority *= 1.5
        if response, err := sm.CreateTextMessage(ctx, &fallbackOptions); err == nil {
            return response, nil
        }
    }

    // Strategy 3: Simplify prompt
    if len(fallbackOptions.Prompt) > 100 {
        fallbackOptions.Prompt = summarizePrompt(fallbackOptions.Prompt)
        if response, err := sm.CreateTextMessage(ctx, &fallbackOptions); err == nil {
            return response, nil
        }
    }

    // All strategies failed
    return nil, fmt.Errorf("all fallback strategies failed: %w", err)
}
```

## Integration Examples

### Chat Application

```go
func buildChatApplication(ctx context.Context) {
    // Setup MCP servers with sampling capability
    builder := mcp.NewBuilder().
        AddHTTPServerWithAuth("openai", "https://api.openai.com/mcp", os.Getenv("OPENAI_KEY")).
        AddHTTPServerWithAuth("anthropic", "https://api.anthropic.com/mcp", os.Getenv("ANTHROPIC_KEY"))

    servers, _, err := builder.Build(ctx)
    if err != nil {
        log.Printf("Chat demo mode: %v", err)
        return
    }

    samplingManager := mcp.NewSamplingManager(servers)
    conversation := []mcp.SamplingMessage{}

    // Chat loop
    for {
        fmt.Print("You: ")
        var userInput string
        fmt.Scanln(&userInput)

        if userInput == "exit" {
            break
        }

        // Add user message
        conversation = append(conversation, mcp.SamplingMessage{
            Role: "user",
            Content: mcp.SamplingContent{
                Type: "text",
                Text: userInput,
            },
        })

        // Generate response
        response, err := samplingManager.CreateConversation(ctx, &mcp.SamplingConversationOptions{
            Messages:    conversation,
            SystemPrompt: "You are a helpful assistant.",
            MaxTokens:   300,
            Temperature: 0.7,
            ModelPreferences: &mcp.ModelPreferences{
                SpeedPriority:       0.8, // Fast for chat
                IntelligencePriority: 0.7,
                CostPriority:        0.6,
            },
        })

        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }

        fmt.Printf("Assistant: %s\n", response.Content.Text)

        // Add assistant response to conversation
        conversation = append(conversation, mcp.SamplingMessage{
            Role: "assistant",
            Content: mcp.SamplingContent{
                Type: "text",
                Text: response.Content.Text,
            },
        })

        // Trim conversation to manage context length
        if len(conversation) > 20 {
            conversation = conversation[2:] // Remove oldest exchange
        }
    }
}
```

### Content Generation Pipeline

```go
func contentGenerationPipeline(ctx context.Context, samplingManager *mcp.SamplingManager) {
    // Step 1: Generate article outline
    outline, err := samplingManager.CreateTextMessage(ctx, &mcp.SamplingTextOptions{
        Prompt: "Create a detailed outline for an article about 'Best Practices for API Design'",
        MaxTokens: 400,
        Temperature: 0.7,
        ModelPreferences: &mcp.ModelPreferences{
            IntelligencePriority: 0.8,
            SpeedPriority:       0.6,
            CostPriority:        0.5,
        },
    })
    if err != nil {
        log.Printf("Outline generation failed: %v", err)
        return
    }

    fmt.Printf("Article Outline:\n%s\n\n", outline.Content.Text)

    // Step 2: Generate introduction
    intro, err := samplingManager.CreateTextMessage(ctx, &mcp.SamplingTextOptions{
        Prompt: fmt.Sprintf("Write an engaging introduction for an article with this outline:\n%s", outline.Content.Text),
        MaxTokens: 300,
        Temperature: 0.6,
        ModelPreferences: &mcp.ModelPreferences{
            IntelligencePriority: 0.8,
            SpeedPriority:       0.7,
            CostPriority:        0.4,
        },
    })
    if err != nil {
        log.Printf("Introduction generation failed: %v", err)
        return
    }

    fmt.Printf("Introduction:\n%s\n\n", intro.Content.Text)

    // Step 3: Generate code examples
    codeExamples, err := samplingManager.CreateCodeGeneration(ctx, &mcp.SamplingCodeOptions{
        Prompt: "Create Go code examples demonstrating RESTful API best practices",
        Language: "go",
        Style: "idiomatic",
        MaxTokens: 500,
        Temperature: 0.3,
        ModelPreferences: &mcp.ModelPreferences{
            IntelligencePriority: 0.9,
            SpeedPriority:       0.5,
            CostPriority:        0.4,
            Hints: []mcp.ModelHint{
                {Name: "code-specialized"},
                {Name: "go-expertise"},
            },
        },
        IncludeComments: true,
        Framework: "gin",
    })
    if err != nil {
        log.Printf("Code generation failed: %v", err)
        return
    }

    fmt.Printf("Code Examples:\n```go\n%s\n```\n\n", codeExamples.Content.Text)

    // Step 4: Generate summary
    fullContent := fmt.Sprintf("%s\n\n%s\n\n%s", outline.Content.Text, intro.Content.Text, codeExamples.Content.Text)
    summary, err := samplingManager.CreateSummary(ctx, &mcp.SamplingSummaryOptions{
        Text: fullContent,
        SummaryType: "bullet-points",
        MaxTokens: 200,
        Temperature: 0.4,
        KeyPoints: 5,
        Perspective: "technical",
    })
    if err != nil {
        log.Printf("Summary generation failed: %v", err)
        return
    }

    fmt.Printf("Article Summary:\n%s\n", summary.Content.Text)
}
```

### Document Analysis System

```go
func documentAnalysisSystem(ctx context.Context, samplingManager *mcp.SamplingManager) {
    // Analyze different types of documents
    documents := []struct {
        name    string
        content string
        docType string
    }{
        {"Technical Spec", "API specification document content...", "technical"},
        {"Legal Contract", "Contract terms and conditions...", "legal"},
        {"Marketing Copy", "Product marketing materials...", "marketing"},
    }

    for _, doc := range documents {
        // Extract key information
        analysis, err := samplingManager.CreateTextMessage(ctx, &mcp.SamplingTextOptions{
            Prompt: fmt.Sprintf("Analyze this %s document and extract key information, main points, and potential issues:\n\n%s", doc.docType, doc.content),
            MaxTokens: 400,
            Temperature: 0.3,
            ModelPreferences: &mcp.ModelPreferences{
                IntelligencePriority: 0.9,
                SpeedPriority:       0.6,
                CostPriority:        0.4,
                Hints: []mcp.ModelHint{
                    {Name: "analysis-focused"},
                    {Name: doc.docType + "-expertise"},
                },
            },
        })

        if err != nil {
            log.Printf("Analysis failed for %s: %v", doc.name, err)
            continue
        }

        fmt.Printf("Analysis of %s:\n%s\n\n", doc.name, analysis.Content.Text)

        // Generate action items
        actions, err := samplingManager.CreateTextMessage(ctx, &mcp.SamplingTextOptions{
            Prompt: fmt.Sprintf("Based on this analysis of a %s document, suggest specific action items:\n\n%s", doc.docType, analysis.Content.Text),
            MaxTokens: 200,
            Temperature: 0.4,
            ModelPreferences: &mcp.ModelPreferences{
                IntelligencePriority: 0.8,
                SpeedPriority:       0.7,
                CostPriority:        0.5,
            },
        })

        if err != nil {
            log.Printf("Action generation failed for %s: %v", doc.name, err)
            continue
        }

        fmt.Printf("Action Items for %s:\n%s\n\n", doc.name, actions.Content.Text)
    }
}
```

## Best Practices

### 1. Model Selection Strategy

```go
// Good: Specific model preferences for different use cases
func getModelPreferencesForUseCase(useCase string) *mcp.ModelPreferences {
    switch useCase {
    case "code-generation":
        return &mcp.ModelPreferences{
            IntelligencePriority: 0.9, // High quality for code
            SpeedPriority:       0.7, // Fast for development
            CostPriority:        0.4,
            Hints: []mcp.ModelHint{
                {Name: "code-specialized"},
                {Name: "technical-writing"},
            },
        }

    case "chat":
        return &mcp.ModelPreferences{
            SpeedPriority:       0.8, // Fast for real-time chat
            IntelligencePriority: 0.7,
            CostPriority:        0.6, // Cost-conscious for volume
        }

    case "analysis":
        return &mcp.ModelPreferences{
            IntelligencePriority: 0.9, // High quality analysis
            SpeedPriority:       0.5,
            CostPriority:        0.3, // Quality over cost
            Hints: []mcp.ModelHint{
                {Name: "reasoning-focused"},
                {Name: "analytical"},
            },
        }

    default:
        return &mcp.ModelPreferences{
            IntelligencePriority: 0.7,
            SpeedPriority:       0.7,
            CostPriority:        0.6,
        }
    }
}
```

### 2. Context Management

```go
func manageConversationContext(messages []mcp.SamplingMessage, maxContextLength int) []mcp.SamplingMessage {
    if len(messages) <= maxContextLength {
        return messages
    }

    // Keep system message and recent messages
    result := []mcp.SamplingMessage{}

    // Always keep system message if it exists
    if len(messages) > 0 && messages[0].Role == "system" {
        result = append(result, messages[0])
        messages = messages[1:]
        maxContextLength--
    }

    // Keep most recent messages
    start := max(0, len(messages)-maxContextLength)
    result = append(result, messages[start:]...)

    return result
}

func estimateTokenCount(text string) int {
    // Simple estimation: ~4 characters per token
    return len(text) / 4
}

func trimToTokenLimit(text string, maxTokens int) string {
    estimatedTokens := estimateTokenCount(text)
    if estimatedTokens <= maxTokens {
        return text
    }

    // Trim to approximate token limit
    targetLength := maxTokens * 4
    if targetLength < len(text) {
        return text[:targetLength] + "..."
    }

    return text
}
```

### 3. Error Recovery and Fallbacks

```go
func robustSamplingExecution(ctx context.Context, sm *mcp.SamplingManager, options *mcp.SamplingTextOptions) (*mcp.MCPSamplingResponse, error) {
    maxRetries := 3
    baseDelay := time.Second

    for attempt := 0; attempt < maxRetries; attempt++ {
        response, err := sm.CreateTextMessage(ctx, options)
        if err == nil {
            return response, nil
        }

        // Check if error is retryable
        if samplingErr, ok := err.(*mcp.SamplingError); ok {
            if !samplingErr.IsRetryable() {
                return nil, err
            }

            // Apply backoff delay
            delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
            if samplingErr.RetryableAfter != nil {
                delay = *samplingErr.RetryableAfter
            }

            time.Sleep(delay)

            // Adjust parameters for retry
            switch samplingErr.Code {
            case "context_length_exceeded":
                options.MaxTokens = int(float64(options.MaxTokens) * 0.8)
            case "rate_limited":
                // Just wait and retry
            case "model_not_available":
                // Let model selection choose a different model
                if options.ModelPreferences != nil {
                    options.ModelPreferences.AvoidModels = append(
                        options.ModelPreferences.AvoidModels,
                        samplingErr.ModelName)
                }
            }

            continue
        }

        // Non-retryable error
        return nil, err
    }

    return nil, fmt.Errorf("sampling failed after %d retries", maxRetries)
}
```

## Troubleshooting

### Common Issues

1. **Context Length Exceeded**
   ```go
   // Check context length before request
   totalTokens := 0
   for _, msg := range messages {
       totalTokens += estimateTokenCount(msg.Content.Text)
   }

   if totalTokens > maxContextLength {
       // Trim or split the request
       messages = trimToTokenLimit(messages, maxContextLength)
   }
   ```

2. **Model Selection Problems**
   ```go
   // Debug model selection
   availableModels, err := sm.GetAvailableModels(ctx)
   if err != nil {
       log.Printf("Failed to get models: %v", err)
   }

   for _, model := range availableModels {
       log.Printf("Available model: %s (cost: %.2f, speed: %.2f, intelligence: %.2f)",
           model.Name, model.CostScore, model.SpeedScore, model.IntelligenceScore)
   }
   ```

3. **Rate Limiting**
   ```go
   // Implement rate limiting client-side
   type RateLimiter struct {
       requests chan time.Time
       limit    int
       window   time.Duration
   }

   func (rl *RateLimiter) Wait(ctx context.Context) error {
       select {
       case rl.requests <- time.Now():
           return nil
       case <-ctx.Done():
           return ctx.Err()
       }
   }
   ```

### Performance Monitoring

```go
// Monitor sampling performance
type SamplingMetrics struct {
    TotalRequests     int64
    SuccessfulRequests int64
    FailedRequests    int64
    AvgResponseTime   time.Duration
    TokensUsed        int64
}

func (sm *SamplingManager) GetMetrics() SamplingMetrics {
    // Return performance metrics
}

// Monitor cost and usage
type CostTracker struct {
    TokenCosts map[string]float64 // Model name -> cost per token
    TotalCost  float64
}

func (ct *CostTracker) TrackUsage(modelName string, tokens int) {
    if costPerToken, exists := ct.TokenCosts[modelName]; exists {
        cost := float64(tokens) * costPerToken
        ct.TotalCost += cost
    }
}
```

This comprehensive documentation covers all aspects of the MCP Sampling feature, preparing the codebase for future Go SDK sampling support while providing powerful utilities for language model integration.