package main

import (
	"context"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Create a new MCP server
	mcpServer := server.NewMCPServer(
		"mcp-golang-stdio-example",
		"0.0.1",
		server.WithInstructions("A simple example of a stdio server using mcp-go"),
	)

	// Register the what_to_eat tool
	whatToEatTool := mcp.NewTool("what_to_eat",
		mcp.WithDescription("Returns a list of foods based on the objective"),
		mcp.WithString("objective", mcp.Required()),
	)

	mcpServer.AddTool(whatToEatTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get objective parameter
		objective := request.GetString("objective", "lunch")

		var food string
		switch objective {
		case "breakfast":
			food = "bread, eggs, coffee"
		case "lunch":
			food = "pasta, salad, water"
		case "dinner":
			food = "pizza, salad, water"
		case "snack":
			food = "apple, almonds, water"
		default:
			food = "pasta, salad, water"
		}

		return mcp.NewToolResultText(food), nil
	})

	// Create context for stdio server
	ctx := context.Background()

	// Create stdio server
	stdioServer := server.NewStdioServer(mcpServer)

	// Serve the server using stdio
	if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil {
		panic(err)
	}
}
