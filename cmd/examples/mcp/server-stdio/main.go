package main

import (
	mcp_golang "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

type WhatToEatArgs struct {
	Objective string `json:"objective" jsonschema:"description=The objective of the food ('breakfast', 'lunch', 'dinner', 'snack')" required:"true"`
}

func main() {
	done := make(chan struct{})

	server := mcp_golang.NewServer(stdio.NewStdioServerTransport())

	// Register a simple dice roller tool
	err := server.RegisterTool("what_to_eat", "Returns a list of foods based on the objective", func(args WhatToEatArgs) (*mcp_golang.ToolResponse, error) {
		objective := args.Objective
		switch objective {
		case "breakfast":
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("bread, eggs, coffee")), nil
		case "lunch":
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("pasta, salad, water")), nil
		case "dinner":
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("pizza, salad, water")), nil
		case "snack":
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("apple, almonds, water")), nil
		default:
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent("pasta, salad, water")), nil
		}
	})

	if err != nil {
		panic(err)
	}

	err = server.Serve()
	if err != nil {
		panic(err)
	}

	<-done
}
