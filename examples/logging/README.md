# Logger Package

A flexible and structured logging package for the Agent SDK Go, built on top of [zerolog](https://github.com/rs/zerolog) for high-performance JSON logging.

## Features

- **Structured Logging**: JSON and console output formats
- **Context-Aware**: Automatic extraction of trace IDs and organization IDs from context
- **Multiple Log Levels**: Debug, Info, Warn, Error
- **High Performance**: Built on zerolog for optimal performance
- **Flexible Configuration**: Global configuration options

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "github.com/your-org/agent-sdk-go/pkg/logging"
)

func main() {
    // Create a new logger
    logger := logging.New()

    // Create context with trace information
    ctx := context.WithValue(context.Background(), "trace_id", "abc123")
    ctx = context.WithValue(ctx, "org_id", "org-456")

    // Log messages with structured fields
    logger.Info(ctx, "User login successful", map[string]interface{}{
        "user_id": "user-789",
        "ip_address": "192.168.1.1",
    })

    logger.Error(ctx, "Database connection failed", map[string]interface{}{
        "error": "connection timeout",
        "retry_count": 3,
    })
}
```

### Output Examples

**Console Format (default):**
```
2024-01-15T10:30:45Z INF User login successful trace_id=abc123 org_id=org-456 user_id=user-789 ip_address=192.168.1.1
2024-01-15T10:30:46Z ERR Database connection failed trace_id=abc123 org_id=org-456 error="connection timeout" retry_count=3
```

**JSON Format:**
```json
{"level":"info","time":"2024-01-15T10:30:45Z","trace_id":"abc123","org_id":"org-456","user_id":"user-789","ip_address":"192.168.1.1","message":"User login successful"}
{"level":"error","time":"2024-01-15T10:30:46Z","trace_id":"abc123","org_id":"org-456","error":"connection timeout","retry_count":3,"message":"Database connection failed"}
```

## Configuration

### Enable JSON Output

#### Using Environment Variables (Recommended)

Set one of these environment variables before running your application:

```bash
# Option 1: Using LOG_FORMAT
export LOG_FORMAT=json

# Option 2: Using LOG_JSON
export LOG_JSON=true
# Also accepts: LOG_JSON=1 or LOG_JSON=yes
```

Then create the logger as usual:

```go
import "github.com/your-org/agent-sdk-go/pkg/logging"

// Logger will automatically use JSON format based on environment variable
logger := logging.New()
```

#### Programmatic Configuration

You can also enable JSON output programmatically:

```go
import "github.com/your-org/agent-sdk-go/pkg/logging"

// Enable JSON output globally
logging.SetZeroLogJsonEnabled()

// Create logger with JSON output
logger := logging.New()
```

Note: Programmatic configuration overrides environment variable settings.

### Set Log Level

```go
// Create logger with specific level
logger := &logging.ZeroLogger{}
logger = logging.WithLevel("debug")(logger)
```

## API Reference

### Logger Interface

```go
type Logger interface {
    Info(ctx context.Context, msg string, fields map[string]interface{})
    Warn(ctx context.Context, msg string, fields map[string]interface{})
    Error(ctx context.Context, msg string, fields map[string]interface{})
    Debug(ctx context.Context, msg string, fields map[string]interface{})
}
```

### Global Functions
- `New() Logger` - Creates a new logger instance
- `SetZeroLogJsonEnabled()` - Enables JSON output format
- `WithLevel(level string)` - Returns a function to set log level

### Context Keys

The logger automatically extracts the following keys from the context:

- `trace_id` - For distributed tracing
- `org_id` - For multi-tenant applications

## Advanced Usage

### Custom Fields

```go
logger.Info(ctx, "Processing request", map[string]interface{}{
    "request_id": "req-123",
    "duration_ms": 150,
    "status_code": 200,
    "user_agent": "Mozilla/5.0...",
})
```

### Error Logging with Stack Traces

```go
import "errors"

err := errors.New("something went wrong")
logger.Error(ctx, "Operation failed", map[string]interface{}{
    "error": err.Error(),
    "operation": "user_creation",
    "retry_after": "5s",
})
```

### Debug Logging

```go
logger.Debug(ctx, "Processing step completed", map[string]interface{}{
    "step": "validation",
    "processed_items": 42,
    "memory_usage": "128MB",
})
```

## Best Practices

1. **Always use context**: Pass context to include trace and organization information
2. **Use structured fields**: Include relevant metadata in the fields map
3. **Choose appropriate log levels**:
   - `Debug`: Detailed information for debugging
   - `Info`: General information about application flow
   - `Warn`: Warning messages for potentially harmful situations
   - `Error`: Error events that might still allow the application to continue
4. **Include relevant context**: Add fields that help with debugging and monitoring
5. **Use consistent field names**: Establish naming conventions for your fields

## Performance Considerations

- The logger is built on zerolog, which is designed for high performance
- JSON output is slightly faster than console output
- Avoid expensive operations in log messages (use lazy evaluation if needed)
- Consider log level filtering in production environments

## Integration with Monitoring

The structured JSON output is compatible with log aggregation systems like:

- **ELK Stack** (Elasticsearch, Logstash, Kibana)
- **Fluentd**
- **Prometheus + Grafana**
- **Datadog**
- **New Relic**

Example for log parsing in Elasticsearch:

```json
{
  "mappings": {
    "properties": {
      "level": {"type": "keyword"},
      "time": {"type": "date"},
      "trace_id": {"type": "keyword"},
      "org_id": {"type": "keyword"},
      "message": {"type": "text"},
      "user_id": {"type": "keyword"}
    }
  }
}
```

## Dependencies

- [zerolog](https://github.com/rs/zerolog) - High performance structured logging
- Go 1.19+

## License

This package is part of the Agent SDK Go and follows the same license terms.
