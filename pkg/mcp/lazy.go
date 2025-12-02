package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

const (
	// Constants for server readiness retry logic
	defaultMaxRetryAttempts = 5
	defaultRetryInterval    = 3 * time.Second
)

// LazyMCPServerCache manages shared MCP server instances
type LazyMCPServerCache struct {
	servers        map[string]interfaces.MCPServer
	serverMetadata map[string]*interfaces.MCPServerInfo
	mu             sync.RWMutex
	logger         logging.Logger
}

// Global server cache to share instances across tools
var globalServerCache = &LazyMCPServerCache{
	servers:        make(map[string]interfaces.MCPServer),
	serverMetadata: make(map[string]*interfaces.MCPServerInfo),
	logger:         logging.New(),
}

// getOrCreateServer gets an existing server or creates a new one
func (cache *LazyMCPServerCache) getOrCreateServer(ctx context.Context, config LazyMCPServerConfig) (interfaces.MCPServer, error) {
	serverKey := fmt.Sprintf("%s:%s:%v", config.Type, config.Name, config.Command)

	// Try to get existing server (read lock)
	cache.mu.RLock()
	if server, exists := cache.servers[serverKey]; exists {
		cache.mu.RUnlock()
		return server, nil
	}
	cache.mu.RUnlock()

	// Create new server (write lock)
	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Double-check in case another goroutine created it
	if server, exists := cache.servers[serverKey]; exists {
		return server, nil
	}

	cache.logger.Info(ctx, "Initializing MCP server on demand", map[string]interface{}{
		"server_name": config.Name,
		"server_type": config.Type,
		"command":     config.Command,
		"args":        config.Args,
		"env_count":   len(config.Env),
	})

	var server interfaces.MCPServer
	var err error

	switch config.Type {
	case "stdio":
		server, err = NewStdioServer(ctx, StdioServerConfig{
			Command: config.Command,
			Args:    config.Args,
			Env:     config.Env,
		})
	case "http":
		server, err = NewHTTPServer(ctx, HTTPServerConfig{
			BaseURL: config.URL,
		})
	default:
		return nil, fmt.Errorf("unsupported MCP server type: %s", config.Type)
	}

	if err != nil {
		cache.logger.Error(ctx, "Failed to initialize MCP server", map[string]interface{}{
			"server_name": config.Name,
			"error":       err.Error(),
		})
		return nil, fmt.Errorf("failed to initialize MCP server '%s': %v", config.Name, err)
	}

	cache.servers[serverKey] = server

	// Capture server metadata if available
	if serverInfo, err := server.GetServerInfo(); err == nil && serverInfo != nil {
		cache.serverMetadata[serverKey] = serverInfo
		cache.logger.Info(ctx, "MCP server initialized successfully with metadata", map[string]interface{}{
			"server_name":        config.Name,
			"discovered_name":    serverInfo.Name,
			"discovered_title":   serverInfo.Title,
			"discovered_version": serverInfo.Version,
		})
	} else {
		cache.logger.Info(ctx, "MCP server initialized successfully", map[string]interface{}{
			"server_name": config.Name,
		})
	}

	// Wait for MCP server to be ready with retries
	cache.logger.Info(ctx, "Waiting for MCP server to be ready", map[string]interface{}{
		"server_name":    config.Name,
		"max_retries":    defaultMaxRetryAttempts,
		"retry_interval": defaultRetryInterval.String(),
	})

	for attempt := 1; attempt <= defaultMaxRetryAttempts; attempt++ {
		// Try to list tools to check if server is ready
		_, err := server.ListTools(ctx)
		if err == nil {
			cache.logger.Info(ctx, "MCP server is ready", map[string]interface{}{
				"server_name": config.Name,
				"attempt":     attempt,
			})
			break
		}

		if attempt < defaultMaxRetryAttempts {
			cache.logger.Debug(ctx, "MCP server not ready, retrying", map[string]interface{}{
				"server_name": config.Name,
				"attempt":     attempt,
				"error":       err.Error(),
			})
			time.Sleep(defaultRetryInterval)
		} else {
			cache.logger.Warn(ctx, "MCP server may not be fully ready after retries", map[string]interface{}{
				"server_name": config.Name,
				"attempts":    attempt,
				"last_error":  err.Error(),
			})
		}
	}

	return server, nil
}

// LazyMCPServerConfig holds configuration for creating an MCP server on demand
type LazyMCPServerConfig struct {
	Name    string
	Type    string // "stdio" or "http"
	Command string
	Args    []string
	Env     []string
	URL     string
}

// LazyMCPTool is a tool that initializes its MCP server on first use
type LazyMCPTool struct {
	name         string
	description  string
	schema       interface{} // Will be discovered dynamically
	schemaLoaded bool        // Track if schema has been loaded
	serverConfig LazyMCPServerConfig
	serverInfo   *interfaces.MCPServerInfo // Discovered server metadata
	logger       logging.Logger
	mu           sync.RWMutex // Protect schema loading
}

// NewLazyMCPTool creates a new lazy MCP tool
func NewLazyMCPTool(name, description string, schema interface{}, config LazyMCPServerConfig) interfaces.Tool {
	return &LazyMCPTool{
		name:         name,
		description:  description,
		schema:       nil, // Will be discovered dynamically
		schemaLoaded: false,
		serverConfig: config,
		logger:       logging.New(),
	}
}

// Name returns the name of the tool
func (t *LazyMCPTool) Name() string {
	return t.name
}

// DisplayName implements interfaces.ToolWithDisplayName.DisplayName
func (t *LazyMCPTool) DisplayName() string {
	return t.name
}

// Description returns a description of what the tool does
func (t *LazyMCPTool) Description() string {
	if t.description != "" {
		return t.description
	}

	// Fallback to server context if no tool-specific description
	if t.serverInfo != nil && t.serverInfo.Title != "" {
		return fmt.Sprintf("%s (from %s)", t.name, t.serverInfo.Title)
	}

	return fmt.Sprintf("%s (MCP tool)", t.name)
}

// Internal implements interfaces.InternalTool.Internal
func (t *LazyMCPTool) Internal() bool {
	return false
}

// getServer gets the MCP server, initializing it if necessary
func (t *LazyMCPTool) getServer(ctx context.Context) (interfaces.MCPServer, error) {
	server, err := globalServerCache.getOrCreateServer(ctx, t.serverConfig)
	if err != nil {
		return nil, err
	}

	// Load server metadata if not already loaded
	if t.serverInfo == nil {
		serverKey := fmt.Sprintf("%s:%s:%v", t.serverConfig.Type, t.serverConfig.Name, t.serverConfig.Command)
		globalServerCache.mu.RLock()
		if metadata, exists := globalServerCache.serverMetadata[serverKey]; exists {
			t.serverInfo = metadata
		}
		globalServerCache.mu.RUnlock()
	}

	return server, nil
}

// discoverSchema discovers the tool's schema from the MCP server
func (t *LazyMCPTool) discoverSchema(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if schema is already loaded
	if t.schemaLoaded {
		return nil
	}

	// Get the server
	server, err := t.getServer(ctx)
	if err != nil {
		return fmt.Errorf("failed to get MCP server: %w", err)
	}

	// List tools from the server to find our tool's schema
	tools, err := server.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tools from MCP server: %w", err)
	}

	// Find our tool in the list
	for _, tool := range tools {
		if tool.Name == t.name {
			t.schema = tool.Schema
			t.schemaLoaded = true
			t.logger.Debug(ctx, "Discovered schema for MCP tool", map[string]interface{}{
				"tool_name": t.name,
				"schema":    tool.Schema,
			})
			return nil
		}
	}

	// Tool not found - this is unexpected but not fatal
	t.logger.Warn(ctx, "Tool not found in MCP server tool list", map[string]interface{}{
		"tool_name": t.name,
	})
	t.schemaLoaded = true // Mark as loaded to avoid repeated attempts
	return nil
}

// Run executes the tool with the given input
func (t *LazyMCPTool) Run(ctx context.Context, input string) (string, error) {
	// Get server (initialize on demand)
	server, err := t.getServer(ctx)
	if err != nil {
		return "", err
	}

	// Parse the input as JSON to get the arguments
	var args map[string]interface{}
	if input != "" {
		if err := json.Unmarshal([]byte(input), &args); err != nil {
			return "", fmt.Errorf("failed to parse input as JSON: %w", err)
		}
	}

	// Log the tool call for debugging
	t.logger.Debug(ctx, "Calling MCP tool", map[string]interface{}{
		"tool_name":   t.name,
		"args":        args,
		"server_name": t.serverConfig.Name,
		"server_type": t.serverConfig.Type,
	})

	// Call the tool on the MCP server
	resp, err := server.CallTool(ctx, t.name, args)
	if err != nil {
		t.logger.Error(ctx, "MCP tool call failed", map[string]interface{}{
			"tool_name": t.name,
			"error":     err.Error(),
		})
		return "", fmt.Errorf("MCP server call failed: %v", err)
	}

	// Log the response for debugging
	t.logger.Debug(ctx, "Received MCP tool response", map[string]interface{}{
		"tool_name": t.name,
		"is_error":  resp.IsError,
		"content":   resp.Content,
	})

	// Handle error response
	if resp.IsError {
		// Better error content handling
		var errorMsg string
		switch content := resp.Content.(type) {
		case string:
			errorMsg = content
		case []byte:
			errorMsg = string(content)
		case map[string]interface{}:
			if msg, ok := content["message"].(string); ok {
				errorMsg = msg
			} else if bytes, err := json.Marshal(content); err == nil {
				errorMsg = string(bytes)
			} else {
				errorMsg = fmt.Sprintf("%v", content)
			}
		default:
			if bytes, err := json.Marshal(content); err == nil {
				errorMsg = string(bytes)
			} else {
				errorMsg = fmt.Sprintf("%v", content)
			}
		}
		return "", fmt.Errorf("MCP tool error: %s", errorMsg)
	}

	// Convert successful response to string
	result := extractTextFromMCPContent(resp.Content)
	return result, nil
}

// extractTextFromMCPContent extracts text from various MCP content formats
func extractTextFromMCPContent(content interface{}) string {
	switch c := content.(type) {
	case string:
		return c
	case []byte:
		return string(c)
	case []mcp.Content:
		// Handle official MCP SDK Content array
		var result string
		for i, item := range c {
			if i > 0 {
				result += "\n"
			}
			// Extract text from MCP Content based on its type
			switch contentItem := item.(type) {
			case *mcp.TextContent:
				if contentItem.Text != "" {
					result += contentItem.Text
				} else {
					// If no text, try to extract from other fields
					result += fmt.Sprintf("%+v", contentItem)
				}
			default:
				// For other content types, try JSON marshaling
				if bytes, err := json.Marshal(item); err == nil {
					result += string(bytes)
				} else {
					result += fmt.Sprintf("%v", item)
				}
			}
		}
		return result
	case []interface{}:
		// Handle array of content items (common in MCP responses)
		var result string
		for i, item := range c {
			if i > 0 {
				result += "\n"
			}
			result += extractTextFromMCPContent(item)
		}
		return result
	case map[string]interface{}:
		// Handle structured content objects
		if text, ok := c["text"].(string); ok {
			return text
		}
		if content, ok := c["content"].(string); ok {
			return content
		}
		if message, ok := c["message"].(string); ok {
			return message
		}
		// If it's a structured object, try to JSON marshal it
		if bytes, err := json.Marshal(c); err == nil {
			return string(bytes)
		}
		return fmt.Sprintf("%v", c)
	default:
		// For any other type, try JSON marshaling first
		if bytes, err := json.Marshal(content); err == nil {
			return string(bytes)
		}
		// Fall back to string representation
		return fmt.Sprintf("%v", content)
	}
}

// Parameters returns the parameters that the tool accepts
func (t *LazyMCPTool) Parameters() map[string]interfaces.ParameterSpec {
	// Try to discover schema if not loaded yet
	ctx := context.Background() // Use background context for schema discovery
	if !t.schemaLoaded {
		if err := t.discoverSchema(ctx); err != nil {
			t.logger.Warn(ctx, "Failed to discover schema for tool", map[string]interface{}{
				"tool_name": t.name,
				"error":     err.Error(),
			})
			// Return empty params if schema discovery fails
			return make(map[string]interfaces.ParameterSpec)
		}
	}

	// Convert the schema to a map of ParameterSpec
	params := make(map[string]interfaces.ParameterSpec)

	var schemaMap map[string]interface{}

	// Handle different schema formats
	switch schema := t.schema.(type) {
	case map[string]interface{}:
		schemaMap = schema
	case string:
		// Parse JSON string schema
		if err := json.Unmarshal([]byte(schema), &schemaMap); err != nil {
			t.logger.Warn(ctx, "Failed to parse schema JSON string", map[string]interface{}{
				"tool_name": t.name,
				"error":     err.Error(),
			})
			return params
		}
	default:
		// Try to marshal and unmarshal to convert any type to map
		if schemaBytes, err := json.Marshal(t.schema); err == nil {
			if err := json.Unmarshal(schemaBytes, &schemaMap); err != nil {
				t.logger.Warn(ctx, "Failed to unmarshal schema after marshaling", map[string]interface{}{
					"tool_name": t.name,
					"error":     err.Error(),
				})
				return params
			}
		} else {
			t.logger.Warn(ctx, "Schema cannot be marshaled to JSON", map[string]interface{}{
				"tool_name":   t.name,
				"schema_type": fmt.Sprintf("%T", t.schema),
			})
			return params
		}
	}

	if properties, ok := schemaMap["properties"].(map[string]interface{}); ok {
		for name, prop := range properties {
			if propMap, ok := prop.(map[string]interface{}); ok {
				// Handle type extraction - support complex types like anyOf
				var paramType string
				if typeVal, ok := propMap["type"]; ok && typeVal != nil {
					paramType = fmt.Sprintf("%v", typeVal)
				} else if anyOf, ok := propMap["anyOf"].([]interface{}); ok && len(anyOf) > 0 {
					// For anyOf types, use the first non-null type
					for _, typeOption := range anyOf {
						if typeMap, ok := typeOption.(map[string]interface{}); ok {
							if t, ok := typeMap["type"].(string); ok && t != "null" {
								paramType = t
								break
							}
						}
					}
					if paramType == "" {
						paramType = "string" // fallback
					}
				} else {
					paramType = "string" // fallback for unknown types
				}

				paramSpec := interfaces.ParameterSpec{
					Type:        paramType,
					Description: fmt.Sprintf("%v", propMap["description"]),
				}

				// Handle array items
				if paramType == "array" {
					if items, ok := propMap["items"]; ok {
						if itemsMap, ok := items.(map[string]interface{}); ok {
							if itemType, ok := itemsMap["type"].(string); ok {
								paramSpec.Items = &interfaces.ParameterSpec{
									Type: itemType,
								}
								// Handle enum for items if present
								if enum, ok := itemsMap["enum"]; ok {
									if enumSlice, ok := enum.([]interface{}); ok {
										paramSpec.Items.Enum = enumSlice
									}
								}
							}
						}
					}
				}

				// Handle enum values
				if enum, ok := propMap["enum"]; ok {
					if enumSlice, ok := enum.([]interface{}); ok {
						paramSpec.Enum = enumSlice
					}
				}

				// Handle default values
				if defaultVal, ok := propMap["default"]; ok {
					paramSpec.Default = defaultVal
				}

				// Check if the parameter is required
				if required, ok := schemaMap["required"].([]interface{}); ok {
					for _, req := range required {
						if req == name {
							paramSpec.Required = true
							break
						}
					}
				}

				params[name] = paramSpec
			}
		}
	}

	return params
}

// Execute executes the tool with the given arguments
func (t *LazyMCPTool) Execute(ctx context.Context, args string) (string, error) {
	// This is the same as Run for LazyMCPTool
	return t.Run(ctx, args)
}

// GetOrCreateServerFromCache provides public access to the global server cache
func GetOrCreateServerFromCache(ctx context.Context, config LazyMCPServerConfig) (interfaces.MCPServer, error) {
	return globalServerCache.getOrCreateServer(ctx, config)
}

// GetServerMetadataFromCache gets server metadata from the global cache
func GetServerMetadataFromCache(config LazyMCPServerConfig) *interfaces.MCPServerInfo {
	serverKey := fmt.Sprintf("%s:%s:%v", config.Type, config.Name, config.Command)
	globalServerCache.mu.RLock()
	defer globalServerCache.mu.RUnlock()
	return globalServerCache.serverMetadata[serverKey]
}
