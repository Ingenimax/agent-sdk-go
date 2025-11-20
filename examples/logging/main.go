package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// Define custom types for context keys
type contextKey string

const (
	traceIDKey contextKey = "trace_id"
	orgIDKey   contextKey = "org_id"
)

func main() {
	// Create context with trace and org IDs
	ctx := context.WithValue(context.Background(), traceIDKey, "trace-001")
	ctx = context.WithValue(ctx, orgIDKey, "org-001")

	// Check if JSON logging is enabled via environment variable
	logFormat := os.Getenv("LOG_FORMAT")
	logJSON := os.Getenv("LOG_JSON")

	fmt.Println("Logging Example")
	fmt.Println("===============")
	fmt.Println()

	if logFormat == "json" || logJSON == "true" || logJSON == "1" || logJSON == "yes" {
		fmt.Println("JSON logging is enabled via environment variable")
		fmt.Println("LOG_FORMAT:", logFormat)
		fmt.Println("LOG_JSON:", logJSON)
	} else {
		fmt.Println("Console logging is enabled (default)")
		fmt.Println("To enable JSON logging, set:")
		fmt.Println("  LOG_FORMAT=json")
		fmt.Println("  or LOG_JSON=true")
	}
	fmt.Println()

	// Create a logger (will automatically use JSON format if env var is set)
	logger := logging.New()

	// Log messages at different levels
	logger.Debug(ctx, "Debug message", map[string]interface{}{
		"component": "example",
		"detail":    "This is a debug level message",
	})

	logger.Info(ctx, "Application started", map[string]interface{}{
		"version": "1.0.0",
		"mode":    "example",
	})

	logger.Warn(ctx, "Warning message", map[string]interface{}{
		"warning": "This is a warning",
		"code":    "WARN001",
	})

	logger.Error(ctx, "Error occurred", map[string]interface{}{
		"error":      "example error",
		"error_code": "ERR001",
		"retryable":  true,
	})

	fmt.Println()
	fmt.Println("---")
	fmt.Println()
	fmt.Println("Programmatic JSON Mode Demo:")
	fmt.Println("-----------------------------")
	fmt.Println()

	// You can also enable JSON logging programmatically
	// This will override any environment variable settings
	logging.SetZeroLogJsonEnabled()

	// Create new logger with JSON format
	jsonLogger := logging.New()

	ctx2 := context.WithValue(context.Background(), traceIDKey, "trace-002")
	ctx2 = context.WithValue(ctx2, orgIDKey, "org-002")

	jsonLogger.Info(ctx2, "Programmatically enabled JSON logging", map[string]interface{}{
		"method": "SetZeroLogJsonEnabled()",
		"note":   "This overrides environment variables",
	})
}
