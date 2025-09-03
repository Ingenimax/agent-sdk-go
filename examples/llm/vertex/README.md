# Vertex AI Example

This example demonstrates how to use the Vertex AI client with Google Cloud's Gemini models.

## Prerequisites

1. **Google Cloud Project**: You need a Google Cloud project with Vertex AI API enabled
2. **Authentication**: Set up authentication using one of these methods:
   - Application Default Credentials (ADC)
   - Service Account Key File

## Setup

### 1. Enable Vertex AI API

```bash
gcloud services enable aiplatform.googleapis.com
```

### 2. Set Environment Variables

```bash
# Required: Your Google Cloud project ID
export GOOGLE_CLOUD_PROJECT="your-project-id"

# Optional: Path to service account key file (if not using ADC)
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/your/service-account.json"
```

### 3. Authentication Options

**Option A: Application Default Credentials (Recommended)**
```bash
gcloud auth application-default login
```

**Option B: Service Account Key File**
1. Create a service account in Google Cloud Console
2. Download the JSON key file
3. Set the `GOOGLE_APPLICATION_CREDENTIALS` environment variable

## Running the Example

```bash
cd examples/llm/vertex
go run main.go
```

## What the Example Demonstrates

### 1. Basic Text Generation
Simple text generation with the Vertex AI client.

### 2. System Messages
Using system messages to guide the model's behavior and role.

### 3. Reasoning Modes
- **None**: Direct, concise answers
- **Minimal**: Brief explanations
- **Comprehensive**: Detailed step-by-step reasoning

### 4. Tool Integration
Demonstrates how to use tools (functions) with the Vertex AI client, including:
- Tool definition with parameters
- Tool execution
- Response handling

### 5. Multiple Models
Shows how to use different Gemini models:
- `gemini-1.5-pro`: Most capable model
- `gemini-1.5-flash`: Faster and more cost-effective

### 6. Parameter Control
Demonstrates various generation parameters:
- **Temperature**: Controls randomness (0.0 to 1.0)
- **Top P**: Nucleus sampling parameter
- **Stop Sequences**: Text sequences that stop generation

## Configuration Options

The client supports various configuration options:

```go
client, err := vertex.NewClient(ctx, projectID,
    vertex.WithModel(vertex.ModelGemini15Pro),     // Model selection
    vertex.WithLocation("us-central1"),            // Regional deployment
    vertex.WithMaxRetries(3),                      // Retry policy
    vertex.WithRetryDelay(time.Second),           // Retry delay
    vertex.WithCredentialsFile("/path/to/key.json"), // Custom credentials
)
```

## Available Models

- `vertex.ModelGemini15Pro`: Gemini 1.5 Pro (most capable)
- `vertex.ModelGemini15Flash`: Gemini 1.5 Flash (faster, cheaper)
- `vertex.ModelGemini20Flash`: Gemini 2.0 Flash (experimental)
- `vertex.ModelGeminiProVision`: Gemini Pro Vision (multimodal)

## Regions

Common Vertex AI regions:
- `us-central1` (default)
- `us-east1`
- `us-west1`
- `europe-west1`
- `asia-southeast1`

## Error Handling

The example includes comprehensive error handling for:
- Authentication failures
- API errors
- Network issues
- Invalid parameters

## Cost Considerations

- Gemini 1.5 Flash is more cost-effective for simple tasks
- Gemini 1.5 Pro offers better performance for complex reasoning
- Consider using appropriate regions to minimize latency and costs

## Troubleshooting

### Common Issues

1. **Authentication Error**
   - Ensure `GOOGLE_CLOUD_PROJECT` is set
   - Verify your authentication method is working
   - Check that Vertex AI API is enabled

2. **Permission Denied**
   - Ensure your account/service account has the `Vertex AI User` role
   - Check project-level permissions

3. **Region Availability**
   - Some models may not be available in all regions
   - Try `us-central1` if you encounter region-specific issues

### Debug Mode

Add verbose logging to debug issues:

```go
import "log/slog"

logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))

client, err := vertex.NewClient(ctx, projectID,
    vertex.WithLogger(logger),
    // ... other options
)
```
