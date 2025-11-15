# MCP Resources Support

## Overview

The MCP Resources feature enables agents to discover, access, and monitor external data sources through the Model Context Protocol. Resources can include files, database records, API responses, real-time data streams, and any other structured or unstructured data that MCP servers expose.

## Key Features

- **Resource Discovery**: List all available resources from MCP servers
- **Content Retrieval**: Get resource content with proper MIME type handling
- **Real-time Monitoring**: Watch for resource changes and updates
- **Metadata Support**: Access resource annotations and metadata
- **Streaming Support**: Handle large resources efficiently

## Architecture

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐
│   Agent     │───▶│ ResourceMgr  │───▶│ MCP Server  │
│             │    │              │    │             │
│ - ListRes   │    │ - Caching    │    │ - Files     │
│ - GetRes    │    │ - Watching   │    │ - APIs      │
│ - WatchRes  │    │ - Metadata   │    │ - DBs       │
└─────────────┘    └──────────────┘    └─────────────┘
```

## Usage Examples

### Basic Resource Access

```go
package main

import (
    "context"
    "fmt"
    "github.com/Ingenimax/agent-sdk-go/pkg/mcp"
)

func main() {
    ctx := context.Background()

    // Build agent with resource-enabled MCP servers
    builder := mcp.NewBuilder().
        AddPreset("filesystem").    // File system resources
        AddPreset("postgres")       // Database resources

    servers, _, err := builder.Build(ctx)
    if err != nil {
        panic(err)
    }

    // Create resource manager
    resourceManager := mcp.NewResourceManager(servers)

    // List all available resources
    resources, err := resourceManager.ListAllResources(ctx)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Found %d resources:\n", len(resources))
    for _, resource := range resources {
        fmt.Printf("- %s (%s): %s\n",
            resource.Name,
            resource.MimeType,
            resource.Description)
    }

    // Get specific resource content
    if len(resources) > 0 {
        content, err := resourceManager.GetResourceContent(ctx, resources[0].URI)
        if err != nil {
            panic(err)
        }

        fmt.Printf("Resource content: %s\n", content.Text)
        fmt.Printf("MIME type: %s\n", content.MimeType)
        fmt.Printf("Size: %d bytes\n", len(content.Text)+len(content.Blob))
    }
}
```

### Resource Watching and Updates

```go
func watchResourceChanges(ctx context.Context, server mcp.MCPServer) {
    // Watch for changes to a specific resource
    updates, err := server.WatchResource(ctx, "file:///path/to/config.json")
    if err != nil {
        panic(err)
    }

    // Process updates in real-time
    for update := range updates {
        switch update.Type {
        case interfaces.MCPResourceUpdateTypeChanged:
            fmt.Printf("Resource updated: %s\n", update.URI)
            fmt.Printf("New content: %s\n", update.Content.Text)

        case interfaces.MCPResourceUpdateTypeError:
            fmt.Printf("Resource error: %s - %v\n", update.URI, update.Error)

        case interfaces.MCPResourceUpdateTypeDeleted:
            fmt.Printf("Resource deleted: %s\n", update.URI)
        }
    }
}
```

### Advanced Resource Filtering

```go
func advancedResourceUsage(ctx context.Context, manager *mcp.ResourceManager) {
    // Filter resources by type
    fileResources, err := manager.GetResourcesByMimeType(ctx, "text/plain")
    if err != nil {
        panic(err)
    }

    // Filter resources by metadata
    recentResources, err := manager.GetResourcesByMetadata(ctx, map[string]string{
        "category": "recent",
        "priority": "high",
    })
    if err != nil {
        panic(err)
    }

    // Get resource with streaming support for large files
    stream, err := manager.GetResourceStream(ctx, "file:///large/dataset.csv")
    if err != nil {
        panic(err)
    }
    defer stream.Close()

    // Process streaming data
    buffer := make([]byte, 1024)
    for {
        n, err := stream.Read(buffer)
        if err != nil {
            break
        }
        // Process chunk
        processChunk(buffer[:n])
    }
}
```

## Resource Types

### File System Resources
- **URI Format**: `file:///path/to/file.ext`
- **Content**: File contents with appropriate MIME type
- **Metadata**: File size, modification time, permissions

### Database Resources
- **URI Format**: `db://table/record/id` or `sql://query/hash`
- **Content**: JSON representation of data
- **Metadata**: Schema info, relationships, constraints

### API Resources
- **URI Format**: `api://service/endpoint?params`
- **Content**: API response data
- **Metadata**: Headers, status codes, rate limits

### Real-time Data
- **URI Format**: `stream://source/channel`
- **Content**: Live data feed
- **Metadata**: Update frequency, source reliability

## ResourceManager API

### Core Methods

```go
type ResourceManager struct {
    servers []interfaces.MCPServer
}

// Create new resource manager
func NewResourceManager(servers []interfaces.MCPServer) *ResourceManager

// List all resources across all servers
func (rm *ResourceManager) ListAllResources(ctx context.Context) ([]interfaces.MCPResource, error)

// Get resource content by URI
func (rm *ResourceManager) GetResourceContent(ctx context.Context, uri string) (*interfaces.MCPResourceContent, error)

// Watch resource for changes
func (rm *ResourceManager) WatchResourceChanges(ctx context.Context, uri string) (<-chan interfaces.MCPResourceUpdate, error)

// Get resources by server
func (rm *ResourceManager) GetResourcesByServer(ctx context.Context, serverName string) ([]interfaces.MCPResource, error)
```

### Filtering Methods

```go
// Filter by MIME type
func (rm *ResourceManager) GetResourcesByMimeType(ctx context.Context, mimeType string) ([]interfaces.MCPResource, error)

// Filter by metadata
func (rm *ResourceManager) GetResourcesByMetadata(ctx context.Context, metadata map[string]string) ([]interfaces.MCPResource, error)

// Search resources by name or description
func (rm *ResourceManager) SearchResources(ctx context.Context, query string) ([]interfaces.MCPResource, error)

// Get recently modified resources
func (rm *ResourceManager) GetRecentResources(ctx context.Context, since time.Time) ([]interfaces.MCPResource, error)
```

### Streaming Methods

```go
// Get resource as stream for large content
func (rm *ResourceManager) GetResourceStream(ctx context.Context, uri string) (io.ReadCloser, error)

// Watch multiple resources efficiently
func (rm *ResourceManager) WatchMultipleResources(ctx context.Context, uris []string) (<-chan interfaces.MCPResourceUpdate, error)
```

## Data Structures

### MCPResource

```go
type MCPResource struct {
    URI         string            `json:"uri"`
    Name        string            `json:"name"`
    Description string            `json:"description"`
    MimeType    string            `json:"mimeType"`
    Metadata    map[string]string `json:"metadata"`
}
```

### MCPResourceContent

```go
type MCPResourceContent struct {
    URI      string            `json:"uri"`
    MimeType string            `json:"mimeType"`
    Text     string            `json:"text,omitempty"`
    Blob     []byte            `json:"blob,omitempty"`
    Reader   io.Reader         `json:"-"`
    Metadata map[string]string `json:"metadata"`
}
```

### MCPResourceUpdate

```go
type MCPResourceUpdate struct {
    URI       string                    `json:"uri"`
    Type      MCPResourceUpdateType     `json:"type"`
    Content   *MCPResourceContent       `json:"content,omitempty"`
    Timestamp time.Time                 `json:"timestamp"`
    Error     error                     `json:"error,omitempty"`
}

type MCPResourceUpdateType string

const (
    MCPResourceUpdateTypeChanged MCPResourceUpdateType = "changed"
    MCPResourceUpdateTypeDeleted MCPResourceUpdateType = "deleted"
    MCPResourceUpdateTypeError   MCPResourceUpdateType = "error"
)
```

## Error Handling

```go
// Resource-specific errors
type ResourceError struct {
    URI       string
    Operation string
    Cause     error
}

func (e *ResourceError) Error() string {
    return fmt.Sprintf("resource error: %s during %s: %v", e.URI, e.Operation, e.Cause)
}

// Handle different error types
func handleResourceError(err error) {
    switch e := err.(type) {
    case *ResourceError:
        if e.URI == "" {
            // Server-level error
            log.Printf("Server error: %v", e.Cause)
        } else {
            // Resource-specific error
            log.Printf("Resource %s error: %v", e.URI, e.Cause)
        }

    case *mcp.MCPError:
        if e.Retryable {
            // Retry the operation
            time.Sleep(e.RetryDelay)
            // ... retry logic
        }

    default:
        log.Printf("Unknown error: %v", err)
    }
}
```

## Performance Optimizations

### Caching

```go
// Enable resource caching
manager := mcp.NewResourceManager(servers)
manager.EnableCaching(5*time.Minute) // Cache for 5 minutes

// Cache-specific operations
manager.ClearCache()
manager.RefreshCache(ctx)
stats := manager.GetCacheStats()
```

### Batching

```go
// Get multiple resources efficiently
uris := []string{
    "file:///config/app.json",
    "db://users/profile/123",
    "api://weather/current",
}

contents, err := manager.GetMultipleResources(ctx, uris)
if err != nil {
    panic(err)
}

for uri, content := range contents {
    fmt.Printf("Resource %s: %d bytes\n", uri, len(content.Text))
}
```

### Connection Pooling

```go
// Configure connection pooling for resource access
config := &mcp.ResourceManagerConfig{
    MaxConcurrentRequests: 10,
    RequestTimeout:        30 * time.Second,
    EnableConnectionPool:  true,
    PoolSize:             5,
}

manager := mcp.NewResourceManagerWithConfig(servers, config)
```

## Security Considerations

### Access Control

Resources respect OAuth scopes and MCP server permissions:

```go
// OAuth scopes for resource access
oauth := &mcp.OAuth2Config{
    Scopes: []string{
        "mcp:resources:read",     // Read resource content
        "mcp:resources:list",     // List available resources
        "mcp:resources:watch",    // Subscribe to resource updates
    },
}
```

### Data Privacy

- **Encryption**: All resource data is transmitted over HTTPS
- **Tokenization**: Sensitive data can be tokenized by MCP servers
- **Audit Logs**: All resource access is logged for compliance

### Rate Limiting

```go
// Handle rate limiting gracefully
manager.SetRateLimit(100, time.Minute) // 100 requests per minute

// Automatic backoff on rate limit errors
manager.EnableRateLimitBackoff(true)
```

## Best Practices

### 1. Resource URI Design

```go
// Good URI patterns
"file:///documents/report.pdf"           // File system
"db://inventory/products?category=tech"  // Database query
"api://slack/channels/general/messages"  // API endpoint
"stream://metrics/cpu-usage"             // Real-time data

// Avoid
"resource://unclear-identifier"          // Too generic
"data"                                   // No scheme
```

### 2. Efficient Resource Watching

```go
// Watch multiple related resources together
uris := []string{
    "file:///config/app.json",
    "file:///config/database.json",
    "file:///config/features.json",
}

updates, err := manager.WatchMultipleResources(ctx, uris)
if err != nil {
    panic(err)
}

// Use select to handle updates and cancellation
for {
    select {
    case update := <-updates:
        handleResourceUpdate(update)
    case <-ctx.Done():
        return
    }
}
```

### 3. Error Recovery

```go
func robustResourceAccess(ctx context.Context, manager *mcp.ResourceManager, uri string) (*interfaces.MCPResourceContent, error) {
    maxRetries := 3
    backoff := time.Second

    for i := 0; i < maxRetries; i++ {
        content, err := manager.GetResourceContent(ctx, uri)
        if err == nil {
            return content, nil
        }

        // Check if error is retryable
        if mcpErr, ok := err.(*mcp.MCPError); ok && mcpErr.Retryable {
            time.Sleep(backoff)
            backoff *= 2
            continue
        }

        // Non-retryable error
        return nil, err
    }

    return nil, fmt.Errorf("failed to get resource after %d retries", maxRetries)
}
```

## Integration Examples

### With File System MCP Server

```go
// Monitor configuration files
builder := mcp.NewBuilder().AddPreset("filesystem")
servers, _, _ := builder.Build(ctx)
manager := mcp.NewResourceManager(servers)

// Watch config directory
updates, _ := manager.WatchResourceChanges(ctx, "file:///app/config/")
for update := range updates {
    if strings.HasSuffix(update.URI, ".json") {
        reloadConfiguration(update.Content.Text)
    }
}
```

### With Database MCP Server

```go
// Access database records as resources
builder := mcp.NewBuilder().AddPreset("postgres")
servers, _, _ := builder.Build(ctx)
manager := mcp.NewResourceManager(servers)

// Get user profile
profile, _ := manager.GetResourceContent(ctx, "db://users/profiles/123")
var user User
json.Unmarshal([]byte(profile.Text), &user)
```

### With API MCP Server

```go
// Access external APIs as resources
builder := mcp.NewBuilder().
    AddHTTPServerWithAuth("weather-api", "https://api.weather.com/mcp", "api-key")

servers, _, _ := builder.Build(ctx)
manager := mcp.NewResourceManager(servers)

// Get current weather
weather, _ := manager.GetResourceContent(ctx, "api://weather/current?location=NYC")
fmt.Printf("Current weather: %s\n", weather.Text)
```

## Troubleshooting

### Common Issues

1. **Resource Not Found**
   ```go
   content, err := manager.GetResourceContent(ctx, uri)
   if err != nil {
       if strings.Contains(err.Error(), "not found") {
           log.Printf("Resource %s does not exist", uri)
           return nil
       }
   }
   ```

2. **Permission Denied**
   ```go
   if strings.Contains(err.Error(), "permission denied") {
       log.Printf("Insufficient permissions for resource %s", uri)
       // Check OAuth scopes or MCP server permissions
   }
   ```

3. **Resource Too Large**
   ```go
   // Use streaming for large resources
   stream, err := manager.GetResourceStream(ctx, uri)
   if err != nil {
       return err
   }
   defer stream.Close()
   ```

### Debugging

```go
// Enable debug logging
manager.SetLogLevel("debug")

// Monitor resource access metrics
stats := manager.GetStats()
fmt.Printf("Resources accessed: %d\n", stats.TotalRequests)
fmt.Printf("Cache hit rate: %.2f%%\n", stats.CacheHitRate)
fmt.Printf("Average response time: %v\n", stats.AvgResponseTime)
```

## Migration Guide

### From Manual Resource Access

```go
// Before: Manual HTTP requests
resp, _ := http.Get("https://api.example.com/data")
data, _ := ioutil.ReadAll(resp.Body)

// After: MCP Resources
content, _ := manager.GetResourceContent(ctx, "api://example/data")
data := content.Text
```

### From File System Operations

```go
// Before: Direct file access
data, _ := os.ReadFile("/path/to/file.json")

// After: MCP Resources
content, _ := manager.GetResourceContent(ctx, "file:///path/to/file.json")
data := []byte(content.Text)
```

This comprehensive documentation covers all aspects of the MCP Resources feature, from basic usage to advanced patterns and troubleshooting. The feature enables seamless integration with various data sources while maintaining security, performance, and ease of use.