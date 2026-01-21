# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

### Building
- `make build-cli` - Build the CLI tool to `bin/agent-cli`
- `make build` - Build CLI and all examples
- `make install` - Install CLI tool to system PATH

### Testing and Quality
- `make test` - Run all tests (`go test ./...`)
- `make lint` - Run linter (`golangci-lint run ./...`)
- `make fmt` - Format code (`go fmt ./...`)
- `make tidy` - Tidy dependencies (`go mod tidy`)

### Development
- `make dev-setup` - Set up development environment
- `make proto` - Generate protobuf files
- `make clean` - Clean build artifacts

## Architecture Overview

This is a Go-based AI agent SDK with a modular architecture:

### Core Package Structure (`pkg/`)
- **agent**: Main agent orchestration and configuration
- **llm**: Multi-provider LLM integrations (OpenAI, Anthropic, Google Vertex AI, Ollama, vLLM)
- **memory**: Conversation memory management (buffer, vector-based)
- **tools**: Extensible tool ecosystem for agent capabilities
- **mcp**: Model Context Protocol server integration (HTTP/stdio)
- **config**: Configuration management with environment variables
- **multitenancy**: Enterprise multi-tenant support
- **guardrails**: Safety mechanisms for AI deployment
- **executionplan**: Planning and execution of complex multi-step tasks
- **vectorstore**: Semantic search and retrieval
- **tracing**: Observability and monitoring

### Key Components
- **Agent Configuration**: YAML-based agent and task definitions
- **Auto-Configuration**: Generate agent configs from system prompts
- **Memory Management**: Persistent conversation tracking with Redis support
- **Tool Registry**: Plugin system for web search, GitHub, and custom operations
- **MCP Integration**: Both eager and lazy initialization patterns
- **Structured Output**: JSON schema-based response formatting

### CLI Tool (`cmd/agent-cli/`)
Headless SDK with interactive chat, task execution, and MCP server management.

## Go Version and Dependencies

- Requires Go 1.24+
- Uses module `github.com/Ingenimax/agent-sdk-go`
- Key dependencies: OpenAI/Anthropic/Google clients, Redis, Weaviate, OpenTelemetry

## Environment Configuration

Uses `.env` files and environment variables for configuration. Key variables:
- `OPENAI_API_KEY`, `OPENAI_MODEL` - OpenAI configuration
- `LOG_LEVEL` - Logging level
- `REDIS_ADDRESS` - Redis for distributed memory

See `env.example` for complete list of configuration options.

## Testing

Run individual tests: `go test ./pkg/[package]`
Run specific test: `go test -run TestName ./pkg/[package]`

Always run linter after code changes: `make lint`

## Microservice UI (pkg/microservice/ui-nextjs)

The SDK includes an embedded Next.js web UI for agent interaction, located at `pkg/microservice/ui-nextjs/`.

### Architecture
- **Framework**: Next.js 15 with App Router
- **Styling**: Tailwind CSS with shadcn/ui components
- **State**: React hooks, no external state management

### Key Components
- `components/chat/chat-area.tsx` - Main chat interface with streaming support
- `components/chat/chat-message.tsx` - Individual message rendering with markdown
- `components/chat/image-lightbox.tsx` - Lightbox for viewing generated images
- `components/screens/` - Various screens (memory, tools, settings, traces)
- `components/layout/` - Sidebar and main layout components

### Performance Considerations

**Message Rendering**: The `ChatMessage` component uses `React.memo` with a custom comparison function to prevent unnecessary re-renders. This is critical when messages contain large base64-encoded images.

```tsx
// ChatMessage is memoized to prevent re-renders on parent state changes
const ChatMessageInner = memo(function ChatMessageInner({ message }) {
  // ... component code
}, (prevProps, nextProps) => {
  // Only re-render if message content or id changes
  return prevProps.message.id === nextProps.message.id &&
         prevProps.message.content === nextProps.message.content;
});
```

**Image Handling**: When image generation tools return base64 data:
1. Large images (>50KB base64) should not be embedded inline in tool responses
2. The backend `imagegen` tool has size limits to prevent token overflow
3. Configure GCS storage for production to get shareable URLs instead of base64
4. Without proper memoization, base64 images cause UI flickering on every keystroke

### Development
```bash
cd pkg/microservice/ui-nextjs
npm install
npm run dev    # Development server on port 3000
npm run build  # Production build
```

### Integration
The UI is embedded in the Go binary via `go:embed` and served by the microservice HTTP server. See `pkg/microservice/http_server.go` for the embedding logic.