package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WhatToEatArgs defines the input structure for the what_to_eat tool
type WhatToEatArgs struct {
	Objective string `json:"objective" jsonschema:"the meal objective (breakfast, lunch, dinner, snack)"`
}

// WhatToEatResult defines the output structure for the what_to_eat tool
type WhatToEatResult struct {
	Food string `json:"food" jsonschema:"recommended food for the objective"`
}

func main() {
	// Create a new MCP server with implementation details
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-golang-stdio-example",
		Version: "0.0.1",
	}, nil)

	// Add the what_to_eat tool using the generic AddTool function
	// This automatically generates the input and output schemas
	mcp.AddTool(server, &mcp.Tool{
		Name:        "what_to_eat",
		Description: "Returns a list of foods based on the objective",
	}, whatToEatHandler)

	// Run the server on the stdio transport
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// whatToEatHandler handles the what_to_eat tool calls
func whatToEatHandler(ctx context.Context, req *mcp.CallToolRequest, args WhatToEatArgs) (*mcp.CallToolResult, WhatToEatResult, error) {
	var food string
	switch args.Objective {
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

	result := WhatToEatResult{
		Food: food,
	}

	// Return the result with text content
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result.Food},
		},
	}, result, nil
}
