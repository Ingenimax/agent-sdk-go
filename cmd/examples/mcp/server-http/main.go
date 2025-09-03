package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TimeArgs defines the input structure for the time tool
type TimeArgs struct {
	Format string `json:"format,omitempty" jsonschema:"the time format to use (default: RFC3339)"`
}

// TimeResult defines the output structure for the time tool
type TimeResult struct {
	CurrentTime string `json:"current_time" jsonschema:"the current time in the specified format"`
}

// DrinkArgs defines the input structure for the what_to_drink tool
type DrinkArgs struct {
	Objective string `json:"objective" jsonschema:"the objective for the drink (hydrate, energize, relax, focus)"`
}

// DrinkResult defines the output structure for the what_to_drink tool
type DrinkResult struct {
	Drink string `json:"drink" jsonschema:"recommended drink for the objective"`
}

func main() {
	// Create a new MCP server with implementation details
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-golang-http-example",
		Version: "0.0.1",
	}, nil)

	// Add the time tool using the generic AddTool function
	mcp.AddTool(server, &mcp.Tool{
		Name:        "time",
		Description: "Returns the current time in the specified format",
	}, timeHandler)

	// Add the what_to_drink tool using the generic AddTool function
	mcp.AddTool(server, &mcp.Tool{
		Name:        "what_to_drink",
		Description: "Returns a drink based on the objective",
	}, drinkHandler)

	// Create SSE handler for HTTP transport
	sseHandler := mcp.NewSSEHandler(func(request *http.Request) *mcp.Server {
		return server
	})

	// Set up HTTP routes
	http.Handle("/mcp", sseHandler)

	// Optional: Add a simple health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Start the HTTP server
	log.Println("Starting HTTP server on :8083...")
	log.Println("MCP endpoint available at: http://localhost:8083/mcp")
	log.Println("Health check available at: http://localhost:8083/health")

	if err := http.ListenAndServe(":8083", nil); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

// timeHandler handles the time tool calls
func timeHandler(ctx context.Context, req *mcp.CallToolRequest, args TimeArgs) (*mcp.CallToolResult, TimeResult, error) {
	format := args.Format
	if format == "" {
		format = time.RFC3339
	}

	currentTime := time.Now().Format(format)
	result := TimeResult{
		CurrentTime: currentTime,
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: currentTime},
		},
	}, result, nil
}

// drinkHandler handles the what_to_drink tool calls
func drinkHandler(ctx context.Context, req *mcp.CallToolRequest, args DrinkArgs) (*mcp.CallToolResult, DrinkResult, error) {
	var drink string
	switch args.Objective {
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

	result := DrinkResult{
		Drink: drink,
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: drink},
		},
	}, result, nil
}
