# Image Generation

This document explains how to use image generation capabilities in the Agent SDK using Gemini 2.5 Flash Image model.

## Overview

The Agent SDK provides image generation support through:

1. **ImageGenerator Interface** - An optional interface that LLM providers can implement for native image generation
2. **Image Generation Tool** - A tool wrapper that agents can use to generate images
3. **Pluggable Image Storage** - Storage backends for persisting generated images (local, GCS)
4. **Memory Integration** - Automatic tracking of generated images in conversation memory

## Supported Models

Currently supported models for image generation:

| Provider | Model | Notes |
|----------|-------|-------|
| Gemini | `gemini-2.5-flash-image` | Native text-to-image generation |

## Architecture

```
                    ┌─────────────────────────────────────┐
                    │         Agent / User Request        │
                    └─────────────────┬───────────────────┘
                                      │
                                      ▼
                    ┌─────────────────────────────────────┐
                    │        ImageGenerationTool          │
                    │   (Tool wrapper for agents)         │
                    └─────────────────┬───────────────────┘
                                      │
                                      ▼
                    ┌─────────────────────────────────────┐
                    │       ImageGenerator Interface      │
                    │   (Implemented by Gemini Client)    │
                    └─────────────────┬───────────────────┘
                                      │
                                      ▼
                    ┌─────────────────────────────────────┐
                    │         Gemini API                  │
                    │   generateContent with IMAGE        │
                    │   responseModalities                │
                    └─────────────────┬───────────────────┘
                                      │
                                      ▼ (base64 image data)
                    ┌─────────────────────────────────────┐
                    │       ImageStorage Interface        │
                    │   (Pluggable storage backend)       │
                    └─────────────────┬───────────────────┘
                                      │
              ┌───────────────────────┴───────────────────────┐
              ▼                                               ▼
    ┌─────────────────┐                             ┌─────────────────┐
    │  Local Storage  │                             │   GCS Storage   │
    │  (Filesystem)   │                             │ (Cloud Storage) │
    └────────┬────────┘                             └────────┬────────┘
             │                                               │
             └───────────────────────┬───────────────────────┘
                                     │
                                     ▼
                    ┌─────────────────────────────────────┐
                    │         URL returned                │
                    │   Stored in Memory for reference    │
                    └─────────────────────────────────────┘
```

## Quick Start

### Direct LLM Usage

Generate images directly using the Gemini client:

```go
import (
    "context"
    "os"

    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
)

func main() {
    ctx := context.Background()

    // Create Gemini client with image generation model
    client, err := gemini.NewClient(ctx,
        gemini.WithAPIKey(os.Getenv("GEMINI_API_KEY")),
        gemini.WithModel(gemini.ModelGemini25FlashImage),
    )
    if err != nil {
        panic(err)
    }

    // Check if client supports image generation
    if !client.SupportsImageGeneration() {
        panic("model does not support image generation")
    }

    // Generate an image
    response, err := client.GenerateImage(ctx, interfaces.ImageGenerationRequest{
        Prompt: "A futuristic city skyline at sunset with flying cars",
        Options: &interfaces.ImageGenerationOptions{
            AspectRatio:  "16:9",
            OutputFormat: "png",
        },
    })
    if err != nil {
        panic(err)
    }

    // Access the generated image
    for _, img := range response.Images {
        fmt.Printf("Generated image: %s (%d bytes)\n", img.MimeType, len(img.Data))
        // img.Data contains raw bytes
        // img.Base64 contains base64-encoded data
    }
}
```

### Agent with Image Generation Tool

Use image generation as a tool within an agent:

```go
import (
    "context"
    "os"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
    "github.com/Ingenimax/agent-sdk-go/pkg/storage/local"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/imagegen"
)

func main() {
    ctx := context.Background()

    // Create Gemini client for image generation
    imageClient, _ := gemini.NewClient(ctx,
        gemini.WithAPIKey(os.Getenv("GEMINI_API_KEY")),
        gemini.WithModel(gemini.ModelGemini25FlashImage),
    )

    // Create storage backend
    storage := local.NewStorage(
        local.WithPath("/var/images"),
        local.WithBaseURL("https://myapp.com/images"),
    )

    // Create image generation tool
    imgTool := imagegen.New(imageClient, storage)

    // Create a separate LLM for the agent (text generation)
    textClient, _ := gemini.NewClient(ctx,
        gemini.WithAPIKey(os.Getenv("GEMINI_API_KEY")),
        gemini.WithModel(gemini.ModelGemini25Flash),
    )

    // Create agent with image generation tool
    ag, _ := agent.NewAgent(
        agent.WithLLM(textClient),
        agent.WithMemory(memory.NewConversationBuffer()),
        agent.WithTools(imgTool),
        agent.WithSystemPrompt("You are a helpful assistant that can generate images."),
    )

    // Agent can now generate images
    response, _ := ag.Run(ctx, "Create an image of a robot playing chess in a park")
    fmt.Println(response)
    // Response will include the URL to the generated image
}
```

## Core Types

### ImageGenerationRequest

Request structure for image generation:

```go
type ImageGenerationRequest struct {
    // Prompt is the text description of the image to generate (required)
    Prompt string

    // ReferenceImage is an optional input image for image-to-image generation
    ReferenceImage *ImageData

    // Options contains generation configuration
    Options *ImageGenerationOptions
}
```

### ImageGenerationOptions

Configuration options for image generation:

```go
type ImageGenerationOptions struct {
    // NumberOfImages specifies how many images to generate (default: 1)
    NumberOfImages int

    // AspectRatio controls the image dimensions
    // Supported values: "1:1", "16:9", "9:16", "4:3", "3:4"
    AspectRatio string

    // OutputFormat specifies the desired output format
    // Supported values: "png", "jpeg"
    OutputFormat string

    // SafetyFilterLevel controls content filtering
    // Supported values: "none", "low", "medium", "high"
    SafetyFilterLevel string
}
```

### ImageGenerationResponse

Response from image generation:

```go
type ImageGenerationResponse struct {
    // Images contains the generated images
    Images []GeneratedImage

    // Usage contains token/cost information if available
    Usage *ImageUsage

    // Metadata contains provider-specific information
    Metadata map[string]interface{}
}
```

### GeneratedImage

Individual generated image:

```go
type GeneratedImage struct {
    // Data contains the raw image bytes
    Data []byte

    // Base64 contains the base64-encoded image data
    Base64 string

    // MimeType is the MIME type of the image (e.g., "image/png")
    MimeType string

    // URL is the storage URL (populated after upload to storage)
    URL string

    // RevisedPrompt is the prompt actually used by the model
    RevisedPrompt string

    // FinishReason indicates why generation stopped
    FinishReason string
}
```

## ImageGenerator Interface

LLM providers that support image generation implement the `ImageGenerator` interface:

```go
type ImageGenerator interface {
    // GenerateImage generates one or more images from a text prompt
    GenerateImage(ctx context.Context, request ImageGenerationRequest) (*ImageGenerationResponse, error)

    // SupportsImageGeneration returns true if this LLM supports image generation
    SupportsImageGeneration() bool

    // SupportedImageFormats returns the output formats supported
    SupportedImageFormats() []string
}
```

### Checking for Image Generation Support

Use type assertion to check if an LLM supports image generation:

```go
if imgGen, ok := llm.(interfaces.ImageGenerator); ok {
    if imgGen.SupportsImageGeneration() {
        // LLM supports image generation
        response, err := imgGen.GenerateImage(ctx, request)
    }
}
```

## Image Storage

The SDK provides a pluggable storage interface for persisting generated images.

### ImageStorage Interface

```go
type ImageStorage interface {
    // Store saves an image and returns an accessible URL
    Store(ctx context.Context, image *GeneratedImage, metadata StorageMetadata) (string, error)

    // Delete removes an image by URL
    Delete(ctx context.Context, url string) error

    // Get retrieves image data by URL (optional, for some backends)
    Get(ctx context.Context, url string) ([]byte, error)

    // Name returns the storage backend name
    Name() string
}

type StorageMetadata struct {
    OrgID     string            // Organization ID for multi-tenancy
    ThreadID  string            // Conversation thread ID
    MessageID string            // Message ID
    Prompt    string            // Original prompt
    Tags      map[string]string // Custom tags
}
```

### Local Storage

Store images on the local filesystem:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/storage/local"

// Create local storage
storage := local.NewStorage(
    local.WithPath("/var/images"),              // Storage directory
    local.WithBaseURL("https://myapp.com/images"), // Optional URL prefix
)

// Files are stored as: /var/images/{orgID}/{threadID}/{timestamp}_{hash}.png
// Returns URL: https://myapp.com/images/{orgID}/{threadID}/{timestamp}_{hash}.png
```

### GCP Cloud Storage

Store images in Google Cloud Storage:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/storage/gcs"

// Create GCS storage
storage, err := gcs.NewStorage(
    gcs.WithBucket("my-bucket"),
    gcs.WithPrefix("generated-images/"),
    gcs.WithCredentialsFile("/path/to/credentials.json"), // Optional
    gcs.WithSignedURLExpiration(24 * time.Hour),          // For presigned URLs
)

// Files are stored as: gs://my-bucket/generated-images/{orgID}/{threadID}/{timestamp}_{hash}.png
// Returns signed URL or public URL based on configuration
```

## Image Generation Tool

The `imagegen` tool wraps image generation for use with agents.

### Tool Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `prompt` | string | Yes | - | Text description of the image to generate |
| `aspect_ratio` | string | No | `1:1` | Image aspect ratio (`1:1`, `16:9`, `9:16`, `4:3`, `3:4`) |
| `output_format` | string | No | `png` | Output format (`png`, `jpeg`) |

### Creating the Tool

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/imagegen"
    "github.com/Ingenimax/agent-sdk-go/pkg/storage/local"
)

// Create storage backend
storage := local.NewStorage(local.WithPath("/var/images"))

// Create image generation tool
imgTool := imagegen.New(
    geminiClient,  // Must implement ImageGenerator
    storage,       // Storage backend
    imagegen.WithMaxPromptLength(2000), // Optional: limit prompt length
)
```

### Tool Response

The tool returns a structured response containing:
- Number of images generated
- Image format and size
- Storage URL for each image
- Usage information (tokens consumed)

## Memory Integration

Generated images are automatically tracked in conversation memory via `ImageReference`:

```go
type ImageReference struct {
    URL       string    // Storage URL of the generated image
    MimeType  string    // Image MIME type
    Prompt    string    // Original prompt used
    CreatedAt time.Time // Generation timestamp
}
```

### Accessing Image References

Image references are stored in message metadata:

```go
// After agent generates an image, the message metadata contains:
msg.Metadata["image_references"] = []ImageReference{
    {
        URL:       "https://storage.example.com/images/abc123.png",
        MimeType:  "image/png",
        Prompt:    "A robot playing chess",
        CreatedAt: time.Now(),
    },
}
```

This enables:
- Agents to reference previously generated images
- Conversation history to include image context
- URLs to persist across sessions

## Configuration

### YAML Configuration

The recommended way to configure image generation is through YAML agent configuration files. This provides a declarative, environment-aware setup.

#### Basic YAML Configuration

```yaml
# agents.yaml
creative_agent:
  role: "Creative Assistant"
  goal: "Help users create visual content"
  backstory: "You are a creative assistant that can generate images"

  # Primary LLM for text generation
  llm_provider:
    provider: "${LLM_PROVIDER:-gemini}"
    model: "${GEMINI_MODEL:-gemini-2.5-flash}"
    config:
      api_key: "${GEMINI_API_KEY}"
      temperature: 0.7

  # Image Generation Configuration
  image_generation:
    enabled: true
    provider: "gemini"
    model: "${GEMINI_IMAGE_MODEL:-gemini-2.5-flash-image}"
    config:
      api_key: "${GEMINI_API_KEY}"
      default_aspect_ratio: "1:1"
      default_output_format: "png"
      safety_filter_level: "medium"
      max_prompt_length: 2000

    # Image Storage Configuration
    storage:
      type: "${IMAGE_STORAGE_TYPE:-local}"

      # Local Storage (development)
      local:
        path: "${IMAGE_STORAGE_LOCAL_PATH:-/tmp/generated_images}"
        base_url: "${IMAGE_STORAGE_LOCAL_BASE_URL}"

      # GCS Storage (production - GCP)
      gcs:
        bucket: "${IMAGE_STORAGE_GCS_BUCKET}"
        prefix: "${IMAGE_STORAGE_GCS_PREFIX:-generated-images/}"
        credentials_file: "${GOOGLE_APPLICATION_CREDENTIALS}"
        signed_url_expiration: "${IMAGE_URL_EXPIRATION:-24h}"

  # Tools to include
  tools:
    - generate_image  # Automatically configured from image_generation section
```

#### Production Configuration Example

```yaml
# production-agents.yaml
starops_creative_agent:
  role: "Creative Platform Engineer"
  goal: "Generate visual documentation and diagrams for infrastructure"

  llm_provider:
    provider: "gemini"
    model: "gemini-2.5-flash"
    config:
      api_key: "${GEMINI_API_KEY}"
      project: "${GOOGLE_CLOUD_PROJECT}"
      location: "${GOOGLE_CLOUD_LOCATION:-us-central1}"

  image_generation:
    enabled: true
    provider: "gemini"
    model: "gemini-2.5-flash-image"
    config:
      api_key: "${GEMINI_API_KEY}"
      project: "${GOOGLE_CLOUD_PROJECT}"
      location: "${GOOGLE_CLOUD_LOCATION:-us-central1}"
      default_aspect_ratio: "16:9"
      default_output_format: "png"

    storage:
      type: "gcs"
      gcs:
        bucket: "${IMAGE_STORAGE_GCS_BUCKET}"
        prefix: "starops/generated-images/"
        signed_url_expiration: "24h"

  llm_config:
    temperature: 0.7
    enable_reasoning: true

  memory:
    type: "redis"
    config:
      address: "${REDIS_ADDRESS}"
      key_prefix: "starops-creative:"
```

#### Using YAML Configuration in Code

```go
package main

import (
    "context"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
)

func main() {
    ctx := context.Background()

    // Load agent configuration from YAML
    configs, err := agent.LoadAgentConfigsFromFile("agents.yaml")
    if err != nil {
        log.Fatal(err)
    }

    // Create agent with YAML config
    // Image generation tool is automatically configured
    creativeAgent, err := agent.NewAgentFromConfig(
        "creative_agent",
        configs,
        nil, // environment variables loaded automatically
    )
    if err != nil {
        log.Fatal(err)
    }

    // Agent can now generate images using the configured tool
    response, err := creativeAgent.Run(ctx, "Create an image of a cloud architecture diagram")
    if err != nil {
        log.Fatal(err)
    }

    log.Println(response)
}
```

#### Environment-Specific Configuration

Use different YAML files for different environments:

```yaml
# development.yaml
my_agent:
  image_generation:
    enabled: true
    provider: "gemini"
    model: "gemini-2.5-flash-image"
    config:
      api_key: "${GEMINI_API_KEY}"
    storage:
      type: "local"
      local:
        path: "/tmp/dev_images"
        base_url: "http://localhost:8080/images"

# production.yaml
my_agent:
  image_generation:
    enabled: true
    provider: "gemini"
    model: "gemini-2.5-flash-image"
    config:
      api_key: "${GEMINI_API_KEY}"
      project: "${GOOGLE_CLOUD_PROJECT}"
    storage:
      type: "gcs"
      gcs:
        bucket: "prod-generated-images"
        prefix: "v1/"
        signed_url_expiration: "1h"
```

#### YAML Configuration Schema

```yaml
image_generation:
  enabled: boolean          # Enable/disable image generation (default: false)
  provider: string          # Provider name: "gemini" (required if enabled)
  model: string             # Model identifier (default: gemini-2.5-flash-image)

  config:                   # Provider-specific configuration
    api_key: string         # API key (use env var)
    project: string         # GCP project (for Vertex AI)
    location: string        # GCP location (for Vertex AI)
    default_aspect_ratio: string    # Default aspect ratio (1:1, 16:9, etc.)
    default_output_format: string   # Default format (png, jpeg)
    safety_filter_level: string     # Safety level (none, low, medium, high)
    max_prompt_length: integer      # Max prompt characters (default: 2000)

  storage:                  # Image storage configuration
    type: string            # Storage type: local, gcs

    local:                  # Local storage options
      path: string          # Directory path
      base_url: string      # URL prefix for serving

    gcs:                    # GCS storage options
      bucket: string        # Bucket name
      prefix: string        # Path prefix
      credentials_file: string      # Service account JSON path
      signed_url_expiration: string # URL expiration duration
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GEMINI_API_KEY` | Gemini API key | Required |
| `GEMINI_IMAGE_MODEL` | Model for image generation | `gemini-2.5-flash-image` |
| `IMAGE_STORAGE_TYPE` | Storage backend type (`local`, `gcs`) | `local` |
| `IMAGE_STORAGE_LOCAL_PATH` | Local storage path | `/tmp/generated_images` |
| `IMAGE_STORAGE_LOCAL_BASE_URL` | URL prefix for local files | (empty) |
| `IMAGE_STORAGE_GCS_BUCKET` | GCS bucket name | - |
| `IMAGE_STORAGE_GCS_PREFIX` | GCS path prefix | `generated-images/` |
| `IMAGE_URL_EXPIRATION` | Signed URL expiration | `24h` |

### Storage Configuration

Use `StorageConfig` for programmatic configuration:

```go
type StorageConfig struct {
    Type string // "local", "gcs"

    // Local storage
    LocalPath string
    BaseURL   string

    // GCS storage
    GCSBucket string
    GCSPrefix string
}
```

## Error Handling

The SDK defines specific errors for image generation:

```go
var (
    // ErrImageGenerationNotSupported - model doesn't support image generation
    ErrImageGenerationNotSupported = errors.New("image generation not supported by this model")

    // ErrContentBlocked - content was blocked by safety filters
    ErrContentBlocked = errors.New("content blocked by safety filters")

    // ErrRateLimitExceeded - rate limiting triggered
    ErrRateLimitExceeded = errors.New("rate limit exceeded")

    // ErrInvalidPrompt - invalid or empty prompt
    ErrInvalidPrompt = errors.New("invalid or empty prompt")

    // ErrStorageUploadFailed - failed to upload to storage
    ErrStorageUploadFailed = errors.New("failed to upload image to storage")
)
```

### Error Handling Example

```go
response, err := client.GenerateImage(ctx, request)
if err != nil {
    switch {
    case errors.Is(err, interfaces.ErrContentBlocked):
        log.Println("Content was blocked by safety filters")
    case errors.Is(err, interfaces.ErrRateLimitExceeded):
        log.Println("Rate limit exceeded, please retry later")
    case errors.Is(err, interfaces.ErrInvalidPrompt):
        log.Println("Invalid prompt provided")
    default:
        log.Printf("Image generation failed: %v", err)
    }
    return
}
```

## Security Considerations

1. **Presigned URLs**: Use expiring URLs for cloud storage (default: 24 hours)
2. **Content Safety**: Gemini's safety filters are applied by default
3. **Storage Permissions**: Ensure proper IAM roles for GCS
4. **No Secrets in URLs**: API keys are never embedded in image URLs
5. **Input Validation**: Prompts are validated for length and content

## Best Practices

1. **Use Separate Models**: Use a text model for the agent and an image model for generation
2. **Configure Storage Properly**: Use cloud storage for production, local for development
3. **Set Appropriate Expiration**: Balance between security and usability for presigned URLs
4. **Handle Errors Gracefully**: Always check for rate limits and content blocks
5. **Monitor Usage**: Track token consumption for cost management

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
    "github.com/Ingenimax/agent-sdk-go/pkg/memory"
    "github.com/Ingenimax/agent-sdk-go/pkg/storage/gcs"
    "github.com/Ingenimax/agent-sdk-go/pkg/tools/imagegen"
)

func main() {
    ctx := context.Background()

    // Create Gemini client for image generation
    imageClient, err := gemini.NewClient(ctx,
        gemini.WithAPIKey(os.Getenv("GEMINI_API_KEY")),
        gemini.WithModel(gemini.ModelGemini25FlashImage),
    )
    if err != nil {
        log.Fatalf("Failed to create image client: %v", err)
    }

    // Create GCS storage for production
    storage, err := gcs.NewStorage(
        gcs.WithBucket(os.Getenv("IMAGE_STORAGE_GCS_BUCKET")),
        gcs.WithPrefix("generated-images/"),
    )
    if err != nil {
        log.Fatalf("Failed to create storage: %v", err)
    }

    // Create image generation tool
    imgTool := imagegen.New(imageClient, storage)

    // Create text LLM for the agent
    textClient, err := gemini.NewClient(ctx,
        gemini.WithAPIKey(os.Getenv("GEMINI_API_KEY")),
        gemini.WithModel(gemini.ModelGemini25Flash),
    )
    if err != nil {
        log.Fatalf("Failed to create text client: %v", err)
    }

    // Create memory
    mem := memory.NewConversationBuffer()

    // Create agent
    ag, err := agent.NewAgent(
        agent.WithLLM(textClient),
        agent.WithMemory(mem),
        agent.WithTools(imgTool),
        agent.WithSystemPrompt(`You are a creative assistant that can generate images.
When the user asks for an image, use the generate_image tool.
Be descriptive in your prompts to get the best results.`),
    )
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Run conversation
    queries := []string{
        "Create an image of a serene Japanese garden with a koi pond at sunset",
        "Now make a similar image but in winter with snow",
    }

    for _, query := range queries {
        fmt.Printf("\nUser: %s\n", query)
        response, err := ag.Run(ctx, query)
        if err != nil {
            log.Printf("Error: %v", err)
            continue
        }
        fmt.Printf("Assistant: %s\n", response)
    }
}
```

## See Also

- [LLM Configuration](llm.md) - Configuring LLM providers
- [Tools](tools.md) - Creating and using tools
- [Memory](memory.md) - Conversation memory management
- [Gemini API](gemini-api.md) - Gemini-specific configuration
