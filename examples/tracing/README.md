# Tracing Example

This example demonstrates how to use the agent SDK with unified tracing capabilities supporting both Langfuse and OpenTelemetry backends.

## Prerequisites

- Go 1.21 or later
- Either Langfuse account and API keys OR OpenTelemetry collector endpoint
- OpenAI API key

## Environment Variables

The example automatically detects which tracing backend to use based on environment variables:

### Option 1: Langfuse Tracing

```bash
# Langfuse configuration (required for Langfuse)
export LANGFUSE_SECRET_KEY="your-langfuse-secret-key"
export LANGFUSE_PUBLIC_KEY="your-langfuse-public-key"
export LANGFUSE_HOST="https://cloud.langfuse.com"  # optional
export LANGFUSE_ENVIRONMENT="development"  # optional

# OpenAI configuration
export OPENAI_API_KEY="your-openai-api-key"
```

### Option 2: OpenTelemetry Tracing

```bash
# OpenTelemetry configuration (required for OTEL)
export OTEL_COLLECTOR_ENDPOINT="localhost:4317"  # or your OTEL endpoint
export SERVICE_NAME="agent-sdk-example"  # optional

# OpenAI configuration
export OPENAI_API_KEY="your-openai-api-key"
```

### Optional Tools Configuration

```bash
# Google Search (optional)
export GOOGLE_API_KEY="your-google-api-key"
export GOOGLE_SEARCH_ENGINE_ID="your-search-engine-id"
```

## Running the Example

### With Langfuse Tracing

1. Set Langfuse environment variables (see above)
2. Run the example:
```bash
go run cmd/examples/tracing/main.go
```

### With OpenTelemetry Tracing

1. Start the OpenTelemetry collector (if running locally):
```bash
docker run -v $(pwd)/otel-collector-config.yaml:/etc/otel-collector-config.yaml \
    -p 4317:4317 \
    -p 4318:4318 \
    otel/opentelemetry-collector:latest \
    --config /etc/otel-collector-config.yaml
```

2. Set OpenTelemetry environment variables (see above)
3. Run the example:
```bash
go run cmd/examples/tracing/main.go
```

### Interactive Usage

3. Interact with the agent by entering queries. Type 'exit' to quit.

## Code Structure

The example consists of two main files:

- `main.go`: Contains the main program that sets up tracing and runs an interactive agent
- `production_config.go`: Contains production-ready configuration functions that can be reused in other applications

### Using in Your Application

To use the tracing configuration in your own application:

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/cmd/examples/tracing"
)

func main() {
    ctx := context.Background()

    // Create a traced agent
    agent, err := tracing.CreateTracedAgent(ctx)
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    // Use the agent
    response, err := agent.Run(ctx, "Your query here")
    if err != nil {
        log.Printf("Error: %v", err)
        return
    }

    fmt.Println(response)
}
```

## Tracing Features

The example demonstrates:

1. **Langfuse Integration**
   - Tracks LLM calls and responses
   - Monitors performance metrics
   - Provides a web interface for visualization

2. **OpenTelemetry Integration**
   - Distributed tracing
   - Metrics collection
   - Integration with various observability platforms

3. **Middleware Pattern**
   - LLM tracing middleware
   - Memory tracing middleware
   - Extensible for additional tracing needs

## Viewing Traces

1. **Langfuse Dashboard**
   - Visit https://cloud.langfuse.com
   - Navigate to your project
   - View traces, metrics, and logs

2. **OpenTelemetry**
   - Use tools like Jaeger, Zipkin, or your preferred observability platform
   - Connect to your OpenTelemetry collector endpoint
   - View distributed traces and metrics
