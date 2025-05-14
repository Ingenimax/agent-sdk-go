# OpenAI Retry Example

This example demonstrates how to use the retry mechanism with the OpenAI client in the agent-sdk-go.

## Features Demonstrated

1. Configuring retry policy with custom parameters:
   - Maximum attempts
   - Initial retry interval
   - Backoff coefficient
   - Maximum interval

2. Using retry with different OpenAI operations:
   - Text generation
   - Chat completion
   - Different retry configurations

## Running the Example

1. Set your OpenAI API key:
```bash
export OPENAI_API_KEY=your-api-key
```

2. Run the example:
```bash
go run main.go
```

## Example Output

The example will show:
- Debug logs for each retry attempt
- Backoff intervals between retries
- Success/failure messages
- Final responses from OpenAI

## Retry Configuration Options

The example demonstrates two different retry configurations:

1. Default configuration:
```go
openai.WithRetry(
    retry.WithMaxAttempts(3),
    retry.WithInitialInterval(time.Second),
    retry.WithBackoffCoefficient(2.0),
    retry.WithMaximumInterval(time.Second*30),
)
```

2. Aggressive configuration (for demonstration):
```go
openai.WithRetry(
    retry.WithMaxAttempts(5),
    retry.WithInitialInterval(time.Millisecond*500),
    retry.WithBackoffCoefficient(1.5),
    retry.WithMaximumInterval(time.Second*5),
)
```

## Understanding the Retry Mechanism

The retry mechanism provides:
- Exponential backoff between retries
- Configurable maximum attempts
- Context-aware cancellation
- Detailed logging of retry attempts
- Flexible configuration options

The debug logs will show:
- Each retry attempt
- Current and next retry intervals
- Success/failure of each attempt
- Final outcome of the operation
