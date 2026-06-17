# HTTP MCP Test Agent

Minimal agent that connects to a single MCP server over HTTP (with token in the URL) for testing.

## Prerequisites

- `OPEN_API_URL`
- `OPENAI_API_KEY` set in the environment
- MCP server reachable at `http://localhost:8000/mcp` (e.g. on same host or Docker network)

## Run

```bash
cd examples/mcp/http-test-agent
go run .
```

With a custom query:

```bash
go run . "Your question here"
```

## Configuration

The MCP server URL (including token) is set in `main.go` as `mcpServerURL`. Change it or load from env if needed:

```go
mcpURL := os.Getenv("MCP_SERVER_URL")
if mcpURL == "" {
    mcpURL = mcpServerURL
}
agent.WithMCPURLs(mcpURL),
```
