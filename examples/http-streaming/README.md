# HTTP/SSE Streaming Example

This example demonstrates the HTTP/SSE (Server-Sent Events) streaming capabilities of the Agent SDK, providing browser-friendly streaming interfaces.

## Overview

The HTTP/SSE server provides REST API endpoints that complement the gRPC streaming interface:
- **gRPC Streaming**: For efficient agent-to-agent communication
- **HTTP/SSE Streaming**: For browser and web client access

## Features

### HTTP Endpoints
- `GET /health` - Health check endpoint
- `GET /api/v1/agent/metadata` - Agent metadata and capabilities
- `POST /api/v1/agent/run` - Non-streaming agent execution
- `POST /api/v1/agent/stream` - SSE streaming endpoint

### Browser Demo
- Interactive web interface at `http://localhost:8080`
- Real-time streaming visualization
- Event log for debugging
- Configuration options for org ID and conversation ID

### CORS Support
- Cross-origin requests enabled for browser access
- Proper SSE headers for streaming
- Works with modern browsers supporting EventSource

## Prerequisites

Install dependencies:
```bash
go mod tidy
```

Set up API keys for your chosen provider:

### For Anthropic:
```bash
export ANTHROPIC_API_KEY=your_anthropic_key
export LLM_PROVIDER=anthropic
```

### For OpenAI:
```bash
export OPENAI_API_KEY=your_openai_key
export LLM_PROVIDER=openai
```

## Running the Example

### Start the Server
```bash
go run main.go
```

The server will start on port 8080 and display:
```
🌐 Agent SDK HTTP/SSE Streaming Server
======================================

Configuration:
  - LLM Provider: anthropic
  - Port: 8080

🚀 Starting HTTP/SSE server...
📡 Browser demo: http://localhost:8080
🔗 Health check: http://localhost:8080/health
📋 Agent metadata: http://localhost:8080/api/v1/agent/metadata
🎯 Streaming endpoint: http://localhost:8080/api/v1/agent/stream
```

### Access the Browser Demo
Open your browser and navigate to:
```
http://localhost:8080
```

## Using the Browser Demo

### Basic Usage
1. **Enter a prompt**: Type your question in the text area
2. **Configure options** (optional): Set organization ID or conversation ID
3. **Start streaming**: Click "Start Streaming" to begin
4. **Watch real-time responses**: See content, thinking, and tool calls stream in real-time

### Advanced Features
- **Event Log**: Monitor all streaming events and their data
- **Stop/Resume**: Control streaming flow
- **Clear**: Reset the interface
- **Status Indicators**: Visual feedback on connection status

### Example Prompts
Try these prompts to see different streaming features:

**Basic Content Streaming:**
```
Explain how photosynthesis works
```

**With Thinking (Anthropic):**
```
Think through this step by step: If I have 20 apples and give away 3/4 of them, then buy twice as many as I gave away, how many apples do I have?
```

**With Tool Usage:**
```
Calculate the compound interest for $5000 at 3.5% annual rate for 8 years
```

**Complex Reasoning (OpenAI o1):**
```
Using step-by-step reasoning, solve this logic puzzle: Three friends have different colored cars...
```

## API Reference

### POST /api/v1/agent/stream

**Request Body:**
```json
{
  "input": "Your prompt here",
  "org_id": "optional-org-id",
  "conversation_id": "optional-conversation-id",
  "max_iterations": 5
}
```

**SSE Response Events:**
- `connected` - Initial connection established
- `content` - Text content chunks
- `thinking` - Reasoning process (Anthropic/o1)
- `tool_call` - Tool execution started
- `tool_result` - Tool execution completed
- `error` - Error occurred
- `complete` - Streaming finished
- `done` - Stream ended

**Event Data Format:**
```json
{
  "type": "content",
  "content": "Hello, world!",
  "thinking_step": "I need to...",
  "tool_call": {
    "id": "call_123",
    "name": "calculator",
    "arguments": "{\"a\": 5, \"b\": 3}",
    "result": "8",
    "status": "completed"
  },
  "error": "Error message if any",
  "metadata": {},
  "is_final": false,
  "timestamp": 1673123456789
}
```

## JavaScript Client Example

```javascript
// Create streaming request
const requestData = {
    input: "Explain quantum computing",
    org_id: "my-org",
    conversation_id: "conv-123"
};

// Start SSE connection
fetch('/api/v1/agent/stream', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(requestData)
}).then(response => {
    const reader = response.body.getReader();
    const decoder = new TextDecoder();

    function readStream() {
        return reader.read().then(({ done, value }) => {
            if (done) return;

            const chunk = decoder.decode(value);
            const lines = chunk.split('\n');

            for (const line of lines) {
                if (line.startsWith('data: ')) {
                    const data = JSON.parse(line.substring(6));
                    handleStreamEvent(data);
                }
            }

            return readStream();
        });
    }

    return readStream();
});

function handleStreamEvent(data) {
    switch (data.type) {
        case 'content':
            document.getElementById('response').textContent += data.content;
            break;
        case 'thinking':
            console.log('Thinking:', data.thinking_step);
            break;
        case 'tool_call':
            console.log('Tool call:', data.tool_call);
            break;
        case 'complete':
            console.log('Stream completed');
            break;
    }
}
```

## cURL Examples

### Health Check
```bash
curl http://localhost:8080/health
```

### Agent Metadata
```bash
curl http://localhost:8080/api/v1/agent/metadata
```

### Non-Streaming Request
```bash
curl -X POST http://localhost:8080/api/v1/agent/run \
  -H "Content-Type: application/json" \
  -d '{"input": "What is 2+2?"}'
```

### Streaming Request
```bash
curl -X POST http://localhost:8080/api/v1/agent/stream \
  -H "Content-Type: application/json" \
  -d '{"input": "Explain machine learning"}' \
  --no-buffer
```

## Architecture

```
Browser/Web Client
       ↓ HTTP/SSE
   HTTP Server
       ↓
   Agent.RunStream()
       ↓
   LLM.GenerateStream()
       ↓
   Real-time Events
```

## Performance Considerations

- **Streaming Buffers**: Configurable for different throughput needs
- **Connection Limits**: Server can handle multiple concurrent streams
- **Graceful Shutdown**: Proper cleanup of active streams
- **Error Recovery**: Robust error handling and client reconnection

## Browser Compatibility

- **Chrome/Edge**: Full support
- **Firefox**: Full support
- **Safari**: Full support
- **Mobile browsers**: Generally supported

## Production Deployment

### Environment Variables
```bash
export ANTHROPIC_API_KEY=your_key
export LLM_PROVIDER=anthropic
export PORT=8080
export CORS_ORIGINS=https://yourdomain.com
```

### Docker Support
```dockerfile
FROM golang:1.21-alpine
WORKDIR /app
COPY . .
RUN go build -o server main.go
EXPOSE 8080
CMD ["./server"]
```

### Reverse Proxy (nginx)
```nginx
location /api/ {
    proxy_pass http://localhost:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_cache_bypass $http_upgrade;
    proxy_buffering off;
}
```

## Troubleshooting

### Common Issues

**Server not starting:**
- Check if port 8080 is available
- Verify API keys are set correctly

**Browser can't connect:**
- Check CORS configuration
- Verify server is running on correct port

**Streaming stops unexpectedly:**
- Check network connectivity
- Monitor server logs for errors

**Events not appearing:**
- Verify JSON parsing in client
- Check browser developer console for errors

### Debug Mode
Set environment variable for verbose logging:
```bash
export DEBUG=true
go run main.go
```

## Next Steps

- Integrate with your web application
- Add authentication/authorization
- Implement custom event filtering
- Scale with load balancers
- Add monitoring and metrics
