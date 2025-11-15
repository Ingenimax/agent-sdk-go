# MCP Quickstart Examples

This directory contains simple examples to help you get started with Model Context Protocol (MCP) in the agent-sdk-go.

## Prerequisites

1. **OpenAI API Key**: Set your OpenAI API key in the environment:
   ```bash
   export OPENAI_API_KEY="your-openai-api-key-here"
   ```

2. **MCP Servers**: Install the MCP servers you want to use. For these examples, you'll need:
   ```bash
   # Install common MCP servers using npm
   npm install -g @modelcontextprotocol/server-filesystem
   npm install -g @modelcontextprotocol/server-time
   npm install -g @modelcontextprotocol/server-fetch
   ```

3. **Optional Environment Variables** (for preset examples):
   ```bash
   export GITHUB_TOKEN="your-github-token"          # For GitHub operations
   export BRAVE_API_KEY="your-brave-api-key"        # For Brave Search
   export DATABASE_URL="postgresql://..."           # For PostgreSQL operations
   ```

## Examples

### 1. Simple URL-based Configuration (`simple.go`)

The easiest way to add MCP servers to your agent using URL strings.

```bash
go run simple.go
```

**What it demonstrates:**
- Basic MCP server setup using URLs
- File system and time operations
- Simple error handling

**Key features:**
- `stdio://` URLs for local MCP servers
- Automatic tool discovery and execution
- Minimal configuration

### 2. Using Presets (`presets.go`)

Using predefined configurations for common MCP servers.

```bash
go run presets.go
```

**What it demonstrates:**
- Available preset servers
- Environment variable configuration
- User-friendly error messages

**Key features:**
- `agent.WithMCPPresets()` for easy setup
- `mcp.ListPresets()` to see available options
- Structured error handling with `mcp.FormatUserFriendlyError()`

### 3. Advanced Builder Pattern (`builder.go`)

Full control over MCP configuration with retry logic, timeouts, and health checks.

```bash
go run builder.go
```

**What it demonstrates:**
- Builder pattern for advanced configuration
- Retry logic and timeout settings
- Both lazy and eager initialization
- Multiple server types (stdio, HTTP, presets)

**Key features:**
- `mcp.NewBuilder()` for flexible configuration
- Retry and timeout customization
- Health checks and server validation
- Mixed initialization strategies

## Common Use Cases

### File Operations
```go
// Enable file system access
agent.WithMCPPresets("filesystem")

// Agent can now:
// - "List files in /home/user"
// - "Create a file called data.txt with some content"
// - "Read the contents of README.md"
```

### Time and Date
```go
// Enable time operations
agent.WithMCPPresets("time")

// Agent can now:
// - "What's the current time?"
// - "What day of the week is it?"
// - "Convert this timestamp to a readable format"
```

### HTTP Requests
```go
// Enable HTTP requests
agent.WithMCPPresets("fetch")

// Agent can now:
// - "Make a GET request to https://api.github.com"
// - "Check if https://example.com is responding"
// - "Download data from an API endpoint"
```

### Database Operations
```bash
# Set database connection
export DATABASE_URL="postgresql://user:pass@localhost/database"
```

```go
// Enable PostgreSQL access
agent.WithMCPPresets("postgres")

// Agent can now:
// - "Show me all tables in the database"
// - "Query users where age > 25"
// - "Insert a new record into the products table"
```

## Troubleshooting

### Command Not Found
If you get "command not found" errors:
```bash
# Make sure MCP servers are installed
which mcp-filesystem
which mcp-time

# If not found, install them:
npm install -g @modelcontextprotocol/server-filesystem
npm install -g @modelcontextprotocol/server-time
```

### Connection Issues
For HTTP MCP servers:
```bash
# Test the connection manually
curl -v http://localhost:8080/mcp

# Check if the server is running
netstat -an | grep 8080
```

### Environment Variables
```bash
# Check your environment variables
env | grep -E "(OPENAI_API_KEY|GITHUB_TOKEN|DATABASE_URL)"

# Make sure they're set correctly
echo $OPENAI_API_KEY
```

### Debug Mode
Enable debug logging for detailed troubleshooting:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/logging"

logger := logging.New().WithLevel("debug")
// Use logger in your MCP configuration
```

## Next Steps

1. **Explore Advanced Examples**: Check out the `../advanced/` directory for more complex use cases
2. **Read the Full Guide**: See `../../docs/mcp-guide.md` for comprehensive documentation
3. **Create Custom Servers**: Learn how to build your own MCP servers
4. **Community Servers**: Discover community-built MCP servers at https://github.com/modelcontextprotocol

## Support

- **Documentation**: `../../docs/mcp-guide.md`
- **MCP Specification**: https://spec.modelcontextprotocol.io/
- **Community**: https://github.com/modelcontextprotocol
- **Issues**: Report issues in the agent-sdk-go repository