package main

import (
	"context"
	"log"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Create a new MCP server
	mcpServer := server.NewMCPServer(
		"mcp-golang-stateless-http-example",
		"0.0.1",
		server.WithInstructions("A simple example of a stateless HTTP server using mcp-golang"),
	)

	// Register the time tool
	timeTool := mcp.NewTool("time",
		mcp.WithDescription("Returns the current time in the specified format"),
		mcp.WithString("format"),
	)

	mcpServer.AddTool(timeTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get format parameter
		format := request.GetString("format", time.RFC3339)

		return mcp.NewToolResultText(time.Now().Format(format)), nil
	})

	// Register the what_to_drink tool
	drinkTool := mcp.NewTool("what_to_drink",
		mcp.WithDescription("Returns a drink based on the objective"),
		mcp.WithString("objective", mcp.Required()),
	)

	mcpServer.AddTool(drinkTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get objective parameter
		objective := request.GetString("objective", "energize")

		var drink string
		switch objective {
		case "hydrate":
			drink = "water"
		case "energize":
			drink = "coffee"
		case "relax":
			drink = "tea"
		case "focus":
			drink = "coffee"
		default:
			drink = "coffee"
		}

		return mcp.NewToolResultText(drink), nil
	})

	// Create an SSE server for HTTP transport
	sseServer := server.NewSSEServer(
		mcpServer,
		server.WithBaseURL("http://localhost:8083"),
		server.WithStaticBasePath("/mcp"),
	)

	// Start the server
	log.Println("Starting HTTP server on :8083...")
	if err := sseServer.Start(":8083"); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
