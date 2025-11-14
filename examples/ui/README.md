# Agent UI Examples

This directory contains examples of using the Agent SDK with the built-in web UI interface.

## Examples

### 1. Basic Agent with UI (`basic_agent_with_ui.go`)

A simple example showing how to create an agent with the web UI enabled.

```bash
export OPENAI_API_KEY="your-api-key"
go run basic_agent_with_ui.go
```

Then open http://localhost:8080 in your browser.

### 2. Advanced Agent with UI (`advanced_agent_with_ui.go`)

An advanced example featuring:
- Claude (Anthropic) LLM integration
- Custom UI configuration
- Dark theme
- Graceful shutdown handling

```bash
export ANTHROPIC_API_KEY="your-api-key"
# Or use OpenAI as fallback:
# export OPENAI_API_KEY="your-api-key"

# Optional: Enable development mode
export DEV_MODE=true

# Optional: Custom port
export PORT=3000

go run advanced_agent_with_ui.go
```

### 3. Agent with Redis Memory (`agent_with_redis_memory.go`)

An agent with Redis-backed persistent memory:
- Redis memory storage (persists across restarts)
- Conversation history browser
- Memory search functionality
- 24-hour conversation retention
- Falls back to local storage if Redis unavailable

```bash
export OPENAI_API_KEY="your-api-key"

# Start Redis (if not already running)
docker run -d -p 6379:6379 redis:alpine

# Optional: Custom Redis address
export REDIS_ADDR="localhost:6379"

go run agent_with_redis_memory.go
```

### 4. Agent with Buffer Memory (`agent_with_buffer_memory.go`)

An agent with conversation buffer memory (no Redis required):
- Remembers last 10 messages
- No external dependencies
- Perfect for development/testing
- Shows agent's actual memory in UI

```bash
export OPENAI_API_KEY="your-api-key"
go run agent_with_buffer_memory.go
```

## Features

The UI provides:

- **Chat Interface**: Real-time streaming chat with the agent
- **Agent Info Panel**: View agent details, model, tools, and system prompt
- **Sub-Agents**: Manage and delegate to specialized sub-agents
- **Memory Browser**: View and search conversation history
- **Settings**: Configure theme and preferences
- **API Access**: Full REST API for programmatic access

## API Endpoints

When the UI server is running, the following endpoints are available:

- `GET /` - Web UI interface
- `GET /health` - Health check
- `POST /api/v1/agent/run` - Non-streaming chat
- `POST /api/v1/agent/stream` - SSE streaming chat
- `GET /api/v1/agent/metadata` - Agent information
- `GET /api/v1/agent/config` - Detailed configuration
- `GET /api/v1/agent/subagents` - List sub-agents
- `POST /api/v1/agent/delegate` - Delegate to sub-agent
- `GET /api/v1/memory` - Browse memory
- `GET /api/v1/memory/search` - Search memory
- `GET /api/v1/tools` - List available tools
- `WS /ws/chat` - WebSocket for real-time chat

## Building the UI

The UI is automatically embedded in the Go binary. To update the UI:

1. Navigate to the UI directory:
```bash
cd pkg/microservice/ui
```

2. Install dependencies:
```bash
npm install
```

3. Build the UI:
```bash
npm run build
```

The built files will be embedded in the Go binary on the next `go build`.

## Customization

You can customize the UI by modifying the `UIConfig`:

```go
uiConfig := &microservice.UIConfig{
    Enabled:     true,
    DefaultPath: "/",
    DevMode:     false,
    Theme:       "dark", // or "light"
    Features: microservice.UIFeatures{
        Chat:      true,
        Memory:    true,
        AgentInfo: true,
        Settings:  true,
    },
}
```

## Environment Variables

- `OPENAI_API_KEY` - OpenAI API key
- `ANTHROPIC_API_KEY` - Anthropic (Claude) API key
- `PORT` - Server port (default: 8080)
- `DEV_MODE` - Enable development mode (hot reload)
