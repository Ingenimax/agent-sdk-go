# Anthropic on Vertex AI Example

This example demonstrates how to use Anthropic Claude models through Google Cloud Vertex AI using the agent-sdk-go.

## Prerequisites

1. **Google Cloud Project** with Vertex AI API enabled
2. **Authentication** set up (one of the following):
   - Application Default Credentials: `gcloud auth application-default login`
   - Service Account Key: Set `GOOGLE_APPLICATION_CREDENTIALS` environment variable

## Environment Variables

### Required
- `GOOGLE_CLOUD_PROJECT`: Your Google Cloud project ID
- `GOOGLE_CLOUD_REGION`: Region where Vertex AI is available (default: us-central1)

### Optional
- `GOOGLE_APPLICATION_CREDENTIALS`: Path to service account key file (if not using ADC)

## Supported Regions

Anthropic models on Vertex AI are available in these regions:
- us-central1
- us-east5
- europe-west1
- europe-west4
- asia-southeast1
- asia-northeast3

## Setup

1. Set up Google Cloud authentication:
```bash
# Option 1: Application Default Credentials
gcloud auth application-default login

# Option 2: Service Account (set environment variable)
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account.json"
```

2. Set environment variables:
```bash
export GOOGLE_CLOUD_PROJECT="your-project-id"
export GOOGLE_CLOUD_REGION="us-central1"
```

3. Run the example:
```bash
go run main.go
```

## Example Features

This example demonstrates:

1. **Basic Generation** with Application Default Credentials
2. **Explicit Credentials** usage (if service account is provided)
3. **Tool Calling** with calculator tool on Vertex AI
4. **Streaming** responses from Vertex AI

## Model Names

When using Vertex AI, use the Vertex-specific model names with version suffixes:
- `claude-3-5-haiku@20241022`
- `claude-3-5-sonnet-v2@20241022`
- `claude-3-opus@20240229`

## Code Examples

### Basic Usage
```go
client := anthropic.NewClient(
    "", // No API key needed for Vertex AI
    anthropic.WithModel("claude-3-5-sonnet-v2@20241022"),
    anthropic.WithVertexAI(region, projectID),
)

response, err := client.Generate(ctx, "Your prompt here")
```

### With Explicit Credentials
```go
client := anthropic.NewClient(
    "",
    anthropic.WithModel("claude-3-5-haiku@20241022"),
    anthropic.WithVertexAICredentials(region, projectID, credentialsPath),
)
```

### Tool Calling
```go
calculatorTool := calculator.New()
response, err := client.GenerateWithTools(
    ctx,
    "Calculate 15 * 23 + 45",
    []interfaces.Tool{calculatorTool},
)
```

### Streaming
```go
eventChan, err := client.GenerateStream(ctx, "Tell me a story")
for event := range eventChan {
    if event.Type == "content_delta" {
        fmt.Print(event.Content)
    }
}
```

## Differences from Direct Anthropic API

| Feature | Direct Anthropic API | Vertex AI |
|---------|---------------------|-----------|
| **Authentication** | API Key | Google Cloud credentials |
| **Endpoint** | `api.anthropic.com` | `{region}-aiplatform.googleapis.com` |
| **Model names** | `claude-3-5-sonnet-latest` | `claude-3-5-sonnet-v2@20241022` |
| **API version** | Header: `Anthropic-Version` | Body: `anthropic_version: vertex-2023-10-16` |

## Troubleshooting

### Authentication Issues
```
Error: failed to find default credentials
```
- Run `gcloud auth application-default login`
- Or set `GOOGLE_APPLICATION_CREDENTIALS` to your service account key

### Region Issues
```
Warning: Region 'xxx' may not support Anthropic models
```
- Use a supported region from the list above
- Check that Vertex AI API is enabled in your project

### Project Issues
```
Error: failed to create Vertex AI request
```
- Ensure `GOOGLE_CLOUD_PROJECT` is set correctly
- Verify the project has Vertex AI API enabled
- Check that you have permissions to access Vertex AI

### Model Issues
```
Error from Anthropic API: model not found
```
- Use the correct Vertex AI model names with version suffixes
- Ensure the model is available in your selected region

## Cost Considerations

- Vertex AI pricing may differ from direct Anthropic API pricing
- Check Google Cloud Vertex AI pricing for current rates
- Monitor usage through Google Cloud Console

## Links

- [Vertex AI Anthropic Documentation](https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/use-claude)
- [Google Cloud Authentication](https://cloud.google.com/docs/authentication)
- [Vertex AI Regions](https://cloud.google.com/vertex-ai/docs/general/locations)
