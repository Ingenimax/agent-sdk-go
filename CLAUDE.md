# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Building and Installation
```bash
# Build CLI tool
make build-cli

# Build all binaries (CLI + examples)
make build

# Install CLI to system PATH
make install

# Create release builds for multiple platforms
make release
```

### Testing and Quality
```bash
# Run all tests
make test
# Or: go test ./...

# Run linter
make lint
# Or: golangci-lint run ./...

# Format code
make fmt
# Or: go fmt ./...

# Tidy dependencies
make tidy
# Or: go mod tidy
```

### Development Setup
```bash
# Set up development environment (installs pre-commit hooks)
make dev-setup

# Generate protobuf files
make proto
# Or: ./scripts/generate-proto.sh

# Quick start guide
make quickstart
```

### Running Single Tests
```bash
# Run specific test file
go test ./pkg/agent/agent_test.go

# Run specific test function
go test -run TestSpecificFunction ./pkg/agent

# Run tests with verbose output
go test -v ./pkg/agent
```

## Architecture Overview

This is a Go-based AI Agent SDK with a modular, interface-driven architecture:

### Core Components

**Agent (`pkg/agent/`)**: Central orchestrator that coordinates LLM, memory, and tools
- `agent.go`: Main agent implementation with tool execution and streaming
- `config.go`: YAML-based agent and task configuration loading
- `streaming.go`: Real-time response streaming capabilities

**LLM Providers (`pkg/llm/`)**: Multi-provider LLM support
- `openai/`: OpenAI GPT models with streaming
- `anthropic/`: Claude models with SSE streaming
- `azureopenai/`: Azure OpenAI with deployment-based configuration
- `gemini/`: Google Vertex AI Gemini models with reasoning modes
- `ollama/`: Local LLM server integration
- `vertex/`: Google Vertex AI integration
- `vllm/`: High-performance local inference

**Memory System (`pkg/memory/`)**: Conversation and context management
- `conversation_buffer.go`: Simple conversation history
- `conversation_summary.go`: Summarized conversation memory
- `vector_retriever.go`: Semantic search-based memory
- `redis_memory.go`: Distributed Redis-backed memory

**Tools (`pkg/tools/`)**: Extensible tool system
- `registry.go`: Tool registration and management
- `agent_tool.go`: Sub-agent execution as tools
- Individual tool implementations (websearch, github, calculator, etc.)

**MCP Integration (`pkg/mcp/`)**: Model Context Protocol support
- `mcp.go`: Core MCP server management
- `lazy.go`: Lazy loading of MCP servers
- `tools.go`: MCP tool adaptation layer
- Supports both HTTP and stdio transports

### Key Architectural Patterns

**Interface-Driven Design**: All major components implement interfaces (`pkg/interfaces/`)
- Enables easy swapping of implementations (LLM providers, memory backends, etc.)
- Supports dependency injection and testing

**Multi-Tenancy Support**: Built-in organization isolation
- Context-based organization ID propagation
- Isolated memory and resources per organization

**Execution Planning**: Structured task management
- Planning, approval, and execution phases
- Task delegation and handoff between agents
- Code and LLM orchestration patterns

**Guardrails System**: Comprehensive safety mechanisms
- Content filtering, PII detection, rate limiting
- Tool restrictions and middleware
- Token limits and usage monitoring

**Observability**: Integrated tracing and logging
- OpenTelemetry integration with Langfuse
- LLM middleware for automatic instrumentation
- Session-based tracing for conversation flows

### Configuration Patterns

**YAML-Based Configuration**: Agent and task definitions
- `agents.yaml`: Agent personas (role, goal, backstory)
- `tasks.yaml`: Task definitions with expected outputs
- Template substitution with variables
- Structured output schema definitions

**Environment Configuration**: `.env` file support
- API keys for various providers
- Service endpoints and configuration
- Development vs production settings

**Auto-Configuration**: LLM-powered config generation
- Generate complete agent profiles from system prompts
- Automatic task definition creation
- Exportable YAML configurations

## Important Implementation Details

### Tool System
- Tools implement the `interfaces.Tool` interface
- Lazy loading supported for MCP tools (initialized on first use)
- Sub-agents can be called as tools for complex orchestration
- Tool registry manages availability and discovery

### Memory Management
- Context-based memory with conversation IDs
- Multiple backend options (in-memory, Redis, vector-based)
- Automatic conversation summarization for long contexts
- Vector retrieval for semantic memory search

### Streaming Support
- Real-time response streaming for all major LLM providers
- Server-Sent Events (SSE) for web applications
- Chunked response processing with tool execution

### Error Handling
- Retry mechanisms with exponential backoff
- Circuit breaker patterns for external services
- Graceful degradation when optional services fail

### Testing Strategy
- Unit tests for core components
- Integration tests with external services
- Test data in `testdata/` directories
- Mock implementations for testing

## Project Structure Notes

- `cmd/agent-cli/`: CLI tool for headless usage
- `examples/`: Comprehensive examples for all features
- `docs/`: Detailed documentation for each component
- `pkg/`: Main SDK library code
- `scripts/`: Development and build scripts
- `bin/`: Built binaries (created by make commands)
