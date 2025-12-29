# Anthropic on AWS Bedrock Example

This example demonstrates how to use Anthropic Claude models through AWS Bedrock using the agent-sdk-go.

## Prerequisites

1. **AWS Account** with Amazon Bedrock access
2. **Model Access** enabled in AWS Bedrock console for Anthropic Claude models
3. **Authentication** set up (one of the following):
   - AWS CLI configured: `aws configure`
   - Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
   - IAM role (when running on EC2/ECS/Lambda)

## Environment Variables

### Option 1: Environment Variables (Development)
```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_SESSION_TOKEN=your_session_token  # Optional, for temporary credentials
export AWS_REGION=us-east-1
```

### Option 2: AWS Configuration Files
```bash
# ~/.aws/credentials
[default]
aws_access_key_id = your_access_key
aws_secret_access_key = your_secret_key

# ~/.aws/config
[default]
region = us-east-1
```

### Option 3: IAM Role (Production)
When running on AWS infrastructure (EC2, ECS, Lambda, EKS), credentials are automatically obtained from the IAM role.

## Supported Regions

Anthropic models on Bedrock are available in these regions:
- **US**: us-east-1, us-west-2
- **Europe**: eu-central-1, eu-west-1, eu-west-2, eu-west-3
- **Asia Pacific**: ap-south-1, ap-southeast-1, ap-southeast-2, ap-northeast-1
- **Americas**: ca-central-1, sa-east-1

## Setup

1. Configure AWS credentials:
```bash
# Option 1: AWS CLI
aws configure

# Option 2: Environment variables
export AWS_ACCESS_KEY_ID="your_access_key"
export AWS_SECRET_ACCESS_KEY="your_secret_key"
export AWS_REGION="us-east-1"
```

2. Enable model access in AWS Bedrock console:
   - Navigate to AWS Bedrock console
   - Go to Model access
   - Request access for Anthropic Claude models

3. Run the example:
```bash
cd examples/llm/anthropic/bedrock
go run main.go
```

## Example Features

This example demonstrates:

1. **Basic Generation** with AWS default credential chain
2. **Custom AWS Config** with retry policies and settings
3. **Automatic Model Conversion** from standard Anthropic names to Bedrock format
4. **Streaming** responses from Bedrock
5. **Extended Thinking** with advanced models (Opus 4.5, Sonnet 4.5)

## Model Names

### Latest Models (Recommended)
- `anthropic.BedrockClaude35Sonnet` - Best balance of intelligence and speed
- `anthropic.BedrockClaude35Haiku` - Fast and cost-effective
- `anthropic.BedrockClaude3Opus` - Highest capability

### Advanced Models with Extended Thinking
- `anthropic.BedrockClaude37Sonnet` - Extended thinking support
- `anthropic.BedrockClaudeSonnet4` - Latest Sonnet with thinking
- `anthropic.BedrockClaudeSonnet45` - Latest Sonnet 4.5
- `anthropic.BedrockClaudeOpus4` - Latest Opus with thinking
- `anthropic.BedrockClaudeOpus41` - Latest Opus 4.1
- `anthropic.BedrockClaudeOpus45` - Latest Opus 4.5

## Code Examples

### Basic Usage
```go
awsConfig, err := config.LoadDefaultConfig(ctx,
    config.WithRegion("us-east-1"),
)
if err != nil {
    log.Fatal(err)
}

client := anthropic.NewClient(
    "", // No Anthropic API key needed for Bedrock
    anthropic.WithModel(anthropic.BedrockClaude35Sonnet),
    anthropic.WithBedrockAWSConfig(awsConfig),
)

response, err := client.Generate(ctx, "Your prompt here")
```

### With Custom AWS Config
```go
awsConfig, err := config.LoadDefaultConfig(ctx,
    config.WithRegion("us-east-1"),
    config.WithRetryMaxAttempts(5),
    // Add any other AWS config options
)
if err != nil {
    log.Fatal(err)
}

client := anthropic.NewClient(
    "",
    anthropic.WithModel(anthropic.BedrockClaude35Haiku),
    anthropic.WithBedrockAWSConfig(awsConfig),
)
```

### Automatic Model Conversion
```go
// SDK automatically converts standard Anthropic model names to Bedrock format
client := anthropic.NewClient(
    "",
    anthropic.WithModel(anthropic.Claude35Sonnet), // Auto-converts to Bedrock format
    anthropic.WithBedrockAWSConfig(awsConfig),
)
```

### Streaming
```go
eventsChan, err := client.GenerateStream(ctx, "Explain cloud computing")
if err != nil {
    log.Fatalf("Error: %v\n", err)
}

for event := range eventsChan {
    switch event.Type {
    case interfaces.StreamEventContentDelta:
        fmt.Print(event.Content)
    case interfaces.StreamEventMessageStop:
        fmt.Println("\nDone!")
    case interfaces.StreamEventError:
        log.Printf("Error: %v\n", event.Error)
    }
}
```

### Extended Thinking
```go
// Use a model that supports extended thinking
client := anthropic.NewClient(
    "",
    anthropic.WithModel(anthropic.BedrockClaudeOpus45),
    anthropic.WithBedrockAWSConfig(awsConfig),
)

eventsChan, err := client.GenerateStream(ctx, "Solve this logic puzzle...")
if err != nil {
    log.Fatalf("Error: %v\n", err)
}

var thinkingContent string
for event := range eventsChan {
    switch event.Type {
    case interfaces.StreamEventThinking:
        // Extended thinking shows the model's reasoning process
        fmt.Print(event.Content)
        thinkingContent += event.Content
    case interfaces.StreamEventContentDelta:
        fmt.Print(event.Content)
    case interfaces.StreamEventMessageStop:
        fmt.Printf("\nThinking tokens: %d characters\n", len(thinkingContent))
    }
}
```

## Differences from Direct Anthropic API

| Feature | Direct Anthropic API | AWS Bedrock |
|---------|---------------------|-------------|
| **Authentication** | API Key | AWS credentials (IAM) |
| **Endpoint** | `api.anthropic.com` | `bedrock-runtime.{region}.amazonaws.com` |
| **Model names** | `claude-3-5-sonnet-latest` | `anthropic.claude-3-5-sonnet-20241022-v2:0` |
| **API version** | Header: `anthropic-version` | Body: `anthropic_version: bedrock-2023-05-31` |
| **Request signing** | API Key header | AWS SigV4 signing |

## Implementation Details

This implementation uses the **official AWS SDK for Go v2**:
- ✅ Native `bedrockruntime.Client` for all API calls
- ✅ Automatic AWS SigV4 request signing
- ✅ Full support for AWS credential chain
- ✅ Streaming support via `InvokeModelWithResponseStream`
- ✅ Consistent with AWS best practices

## Troubleshooting

### Authentication Issues
```
Error: The security token included in the request is invalid
```
- Check your AWS credentials are valid and not expired
- Verify environment variables are set correctly
- Try running `aws sts get-caller-identity` to test credentials

### Permission Issues
```
Error: User is not authorized to perform: bedrock:InvokeModel
```
Add this IAM policy to your user/role:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "bedrock:InvokeModel",
        "bedrock:InvokeModelWithResponseStream"
      ],
      "Resource": [
        "arn:aws:bedrock:*:*:inference-profile/*anthropic.claude-*",
        "arn:aws:bedrock:*::foundation-model/*anthropic.claude-*",
        "arn:aws:bedrock:*::foundation-model/anthropic.claude-*"
      ]
    }
  ]
}
```

### Region Issues
```
Error: The requested model is not available in this region
```
- Check the model is enabled in your AWS Bedrock console
- Verify the region supports your chosen model
- Use a supported region from the list above

### Model Access Issues
```
Error: Model use case details have not been submitted for this account
```
- Navigate to AWS Bedrock console
- Go to "Model access" section
- Request access for the specific Claude model
- Wait for approval (usually instant for most models)

## Cost Considerations

- **Billing through AWS**: All charges appear on your AWS bill
- **Regional pricing**: Pricing may vary by AWS region
- **No Anthropic API costs**: You only pay AWS Bedrock pricing
- **Monitor usage**: Use AWS Cost Explorer to track Bedrock costs

## Links

- [AWS Bedrock Documentation](https://docs.aws.amazon.com/bedrock/)
- [Anthropic on Bedrock Guide](https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages.html)
- [AWS Bedrock Pricing](https://aws.amazon.com/bedrock/pricing/)
- [AWS SDK for Go v2](https://aws.github.io/aws-sdk-go-v2/docs/)
- [agent-sdk-go Repository](https://github.com/Ingenimax/agent-sdk-go)
