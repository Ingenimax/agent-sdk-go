# Google Custom Search Engine (CSE) MCP Example

This example demonstrates how to use the [mcp-google-cse](https://github.com/Richard-Weiss/mcp-google-cse) MCP server with agent-sdk-go to create an agent that can search the web using Google Custom Search.

## Prerequisites

1. **Python and uvx**: Install uv (includes uvx) for running the MCP server:
   ```bash
   pip install uv
   ```

2. **Google Custom Search API**:
   - Get a Google Custom Search API key from [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
   - Create a Custom Search Engine at [Google CSE](https://programmablesearchengine.google.com/)
   - Note your Engine ID

3. **OpenAI API Key**: Required for the LLM client

## Environment Variables

Set the following environment variables:

```bash
export OPENAI_API_KEY="your-openai-api-key"
export API_KEY="your-google-api-key"
export ENGINE_ID="your-search-engine-id"
```

Or create a `.env` file:

```bash
OPENAI_API_KEY=your-openai-api-key
API_KEY=your-google-api-key
ENGINE_ID=your-search-engine-id
```

## Running the Example

```bash
cd examples/mcp/google-cse
go run main.go
```

## What It Does

The example creates an agent that:
1. Uses OpenAI's GPT-4o-mini as the LLM
2. Integrates with the mcp-google-cse MCP server using lazy initialization
3. Can perform Google searches using the `google_search` tool
4. Finds the best websites for learning AI in 2025

## How It Works

The agent uses the **Lazy MCP Configuration** approach:
- The MCP server is not started until the first tool call
- The `uvx mcp-google-cse` command is run as a stdio process
- Environment variables (API_KEY and ENGINE_ID) are passed to the MCP server
- The agent has access to the `google_search` tool with the proper schema

## Quota Limits

The free quota for Google Custom Search is **100 searches per day**. Each tool call counts as one search.

## Customization

You can modify the query in `main.go`:

```go
query := "What are the best websites for learning AI in 2025?"
```

Or you can extend the example to accept user input for interactive searching.

## Optional Configuration

The mcp-google-cse server supports additional environment variables:
- `COUNTRY_REGION`: Filter results by country
- `GEOLOCATION`: Set user location (default: "us")
- `RESULT_LANGUAGE`: Search result language (default: "lang_en")
- `RESULT_NUM`: Number of results to return, 1-10 (default: 10)

Add these to the `Env` slice in the LazyMCPConfig if needed.
