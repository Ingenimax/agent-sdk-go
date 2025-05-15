package main

import (
	"log"
	"time"

	mcp_golang "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/http"
)

// TimeArgs defines the arguments for the time tool
type TimeArgs struct {
	Format string `json:"format" jsonschema:"description=The time format to use"`
}

type WhatToDrinkArgs struct {
	Objective string `json:"objective" jsonschema:"description=The objective of the drink ('hydrate', 'energize', 'relax', 'focus')" required:"true"`
}

func main() {
	// Create an HTTP transport that listens on /mcp endpoint
	transport := http.NewHTTPTransport("/mcp").WithAddr(":8083")

	// Create a new server with the transport
	server := mcp_golang.NewServer(
		transport,
		mcp_golang.WithName("mcp-golang-stateless-http-example"),
		mcp_golang.WithInstructions("A simple example of a stateless HTTP server using mcp-golang"),
		mcp_golang.WithVersion("0.0.1"),
	)

	// Register a simple tool
	err := server.RegisterTool("time", "Returns the current time in the specified format", func(args TimeArgs) (*mcp_golang.ToolResponse, error) {
		format := args.Format
		return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(time.Now().Format(format))), nil
		// return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("Error calling the tool")), nil
	})
	if err != nil {
		panic(err)
	}

	err = server.RegisterTool("what_to_drink", "Returns a drink based on the objective", func(args WhatToDrinkArgs) (*mcp_golang.ToolResponse, error) {
		objective := args.Objective
		switch objective {
		case "hydrate":
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("water")), nil
		case "energize":
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("coffee")), nil
		case "relax":
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("tea")), nil
		case "focus":
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("coffee")), nil
		default:
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("coffee")), nil
		}
	})
	if err != nil {
		panic(err)
	}

	// Start the server
	log.Println("Starting HTTP server on :8083...")
	if err := server.Serve(); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
