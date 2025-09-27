package main

import (
	"context"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// Define custom types for context keys
type contextKey string

const (
	traceIDKey contextKey = "trace_id"
	orgIDKey   contextKey = "org_id"
)

func main() {
	// Create a logger
	logger := logging.New()

	// Create context
	ctx := context.WithValue(context.Background(), traceIDKey, "trace-001")
	ctx = context.WithValue(ctx, orgIDKey, "org-001")

	fmt.Println("Console Mode (Default):")
	fmt.Println("----------------------")

	// Simple logging
	logger.Info(ctx, "Hello World", map[string]interface{}{
		"message": "This is a simple log message",
	})

	logger.Error(ctx, "Something went wrong", map[string]interface{}{
		"error": "example error",
	})

	fmt.Println()
	fmt.Println("JSON Mode:")
	fmt.Println("----------")

	// Enable JSON logging
	logging.SetZeroLogJsonEnabled()

	// Create new logger
	jsonLogger := logging.New()

	// Same context
	ctx2 := context.WithValue(context.Background(), traceIDKey, "trace-002")
	ctx2 = context.WithValue(ctx2, orgIDKey, "org-002")

	// Same logging but in JSON format
	jsonLogger.Info(ctx2, "Hello World", map[string]interface{}{
		"message": "This is a simple log message",
	})

	jsonLogger.Error(ctx2, "Something went wrong", map[string]interface{}{
		"error": "example error",
	})
}
