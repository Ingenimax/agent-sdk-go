# Agent SDK Go - Makefile

.PHONY: all build build-cli install clean test lint fmt tidy proto dev-setup release quickstart help

# Default target
all: build

# Build the CLI tool
build-cli:
	@echo "🔨 Building Agent CLI..."
	@cd cmd/agent-cli && go build -o ../../bin/agent-cli .
	@echo "✅ CLI built successfully: agent-cli"

# Build all binaries
build: build-cli
	@echo "🔨 Building all examples..."
	@mkdir -p bin
	@cd examples/simple_agent && go build -o ../../bin/simple-agent .
	@cd examples/agent_config_yaml && go build -o ../../bin/yaml-config .
	@cd examples/mcp/client && go build -o ../../../../bin/mcp-client .
	@echo "✅ All binaries built successfully"

# Install CLI tool to system PATH
install: build-cli
	@echo "📦 Installing agent-cli to /usr/local/bin..."
	@sudo cp bin/agent-cli /usr/local/bin/
	@echo "✅ agent-cli installed successfully"
	@echo "💡 You can now run 'agent-cli' from anywhere"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf bin/
	@echo "✅ Clean complete"

# Run tests
test:
	@echo "🧪 Running tests..."
	@go test ./...
	@echo "✅ Tests complete"

# Run linter
lint:
	@echo "🔍 Running linter..."
	@golangci-lint run ./...
	@echo "✅ Lint complete"

# Format code
fmt:
	@echo "🎨 Formatting code..."
	@go fmt ./...
	@echo "✅ Format complete"

# Tidy dependencies
tidy:
	@echo "📦 Tidying dependencies..."
	@go mod tidy
	@echo "✅ Tidy complete"

# Generate protobuf files
proto:
	@echo "🔧 Generating protobuf files..."
	@./scripts/generate-proto.sh
	@echo "✅ Protobuf generation complete"

# Development setup
dev-setup:
	@echo "🚀 Setting up development environment..."
	@./scripts/dev-env-setup.sh
	@echo "✅ Development setup complete"

# Create release build
release: clean
	@echo "🚀 Creating release build..."
	@mkdir -p bin
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/agent-cli-linux-amd64 cmd/agent-cli/main.go
	@GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/agent-cli-darwin-amd64 cmd/agent-cli/main.go
	@GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/agent-cli-darwin-arm64 cmd/agent-cli/main.go
	@GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/agent-cli-windows-amd64.exe cmd/agent-cli/main.go
	@echo "✅ Release builds created in bin/"

# Quick start - initialize and run a simple example
quickstart: build-cli
	@echo "🚀 Quick start guide:"
	@echo "1. Initialize configuration:"
	@echo "   ./bin/agent-cli init"
	@echo ""
	@echo "2. Set your API key (example for OpenAI):"
	@echo "   export OPENAI_API_KEY=your_api_key_here"
	@echo ""
	@echo "3. Run a simple query:"
	@echo "   ./bin/agent-cli run \"Hello, world!\""
	@echo ""
	@echo "4. Start interactive chat:"
	@echo "   ./bin/agent-cli chat"

# Show help
help:
	@echo "Agent SDK Go - Available Make Targets:"
	@echo ""
	@echo "  build-cli     Build the CLI tool"
	@echo "  build         Build all binaries"
	@echo "  install       Install CLI tool to system PATH"
	@echo "  clean         Clean build artifacts"
	@echo "  test          Run tests"
	@echo "  lint          Run linter"
	@echo "  fmt           Format code"
	@echo "  tidy          Tidy dependencies"
	@echo "  proto         Generate protobuf files"
	@echo "  dev-setup     Set up development environment"
	@echo "  release       Create release builds for multiple platforms"
	@echo "  quickstart    Show quick start guide"
	@echo "  help          Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build-cli"
	@echo "  make install"
	@echo "  make quickstart"
