# MCP Tool Output Schemas

## Overview

The MCP Tool Output Schemas feature provides comprehensive validation and type safety for MCP tool responses. It implements a complete JSON Schema validation framework that ensures tool outputs conform to expected structures, enabling better error handling, documentation, and integration reliability.

## Key Features

- **JSON Schema Validation**: Complete JSON Schema Draft 7 support
- **Type Safety**: Strong typing for tool outputs with structured responses
- **Schema Builder**: Fluent API for creating schemas programmatically
- **Pre-built Templates**: Common schema patterns for typical use cases
- **Tool Integration**: Seamless integration with MCP tool definitions
- **Validation Reporting**: Detailed error messages with context
- **Performance Optimization**: Schema compilation and caching

## Architecture

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐
│   Agent     │───▶│ ToolManager  │───▶│ MCP Server  │
│             │    │              │    │             │
│ - CallTool  │    │ - Validation │    │ - Tools     │
│ - Validate  │    │ - Schemas    │    │ - Responses │
│ - Types     │    │ - Errors     │    │ - Metadata  │
└─────────────┘    └──────────────┘    └─────────────┘
       │                    │
       └────────────────────┼─────────────────────
                            ▼
                   ┌─────────────┐
                   │ Validator   │
                   │             │
                   │ - JSON      │
                   │ - Types     │
                   │ - Rules     │
                   └─────────────┘
```

## Usage Examples

### Basic Schema Validation

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/Ingenimax/agent-sdk-go/pkg/mcp"
    "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func main() {
    ctx := context.Background()

    // Build agent with schema-enabled MCP servers
    builder := mcp.NewBuilder().
        AddPreset("filesystem").
        AddPreset("weather-api")

    servers, _, err := builder.Build(ctx)
    if err != nil {
        panic(err)
    }

    // Create tool manager with schema validation
    toolManager := mcp.NewToolManager(servers)
    validator := mcp.NewSchemaValidator()

    // Define expected schema for weather tool
    weatherSchema := mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("temperature", mcp.NewSchemaBuilder().Number().Build()).
        RequiredProperty("conditions", mcp.NewSchemaBuilder().String().Build()).
        OptionalProperty("humidity", mcp.NewSchemaBuilder().Number().Build()).
        OptionalProperty("wind_speed", mcp.NewSchemaBuilder().Number().Build()).
        Build()

    // Execute tool and validate response
    if len(servers) > 0 {
        server := servers[0]

        // Call weather tool
        response, err := server.CallTool(ctx, "get-weather", map[string]interface{}{
            "location": "New York",
        })
        if err != nil {
            log.Printf("Tool execution failed: %v", err)
            return
        }

        // Validate response against schema
        tool := interfaces.MCPTool{
            Name:         "get-weather",
            Description:  "Get current weather",
            OutputSchema: weatherSchema,
        }

        if err := validator.ValidateToolResponse(ctx, tool, response); err != nil {
            log.Printf("Schema validation failed: %v", err)
            return
        }

        fmt.Printf("Weather data is valid: %s\n", response.Content)

        // Access structured content safely
        if response.StructuredContent != nil {
            if weather, ok := response.StructuredContent.(map[string]interface{}); ok {
                temp := weather["temperature"].(float64)
                conditions := weather["conditions"].(string)
                fmt.Printf("Temperature: %.1f°C, Conditions: %s\n", temp, conditions)
            }
        }
    }
}
```

### Advanced Schema Building

```go
func advancedSchemaUsage() {
    validator := mcp.NewSchemaValidator()

    // Complex nested schema for user profile
    userProfileSchema := mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("id", mcp.NewSchemaBuilder().Number().Build()).
        RequiredProperty("name", mcp.NewSchemaBuilder().String().Build()).
        RequiredProperty("email", mcp.NewSchemaBuilder().String().Build()).
        OptionalProperty("profile", mcp.NewSchemaBuilder().
            Object().
            OptionalProperty("age", mcp.NewSchemaBuilder().Number().Build()).
            OptionalProperty("location", mcp.NewSchemaBuilder().
                Object().
                RequiredProperty("city", mcp.NewSchemaBuilder().String().Build()).
                RequiredProperty("country", mcp.NewSchemaBuilder().String().Build()).
                Build()).
            OptionalProperty("tags", mcp.NewSchemaBuilder().
                Array(mcp.NewSchemaBuilder().String().Build()).
                Build()).
            Build()).
        Build()

    // File information schema
    fileInfoSchema := mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("name", mcp.NewSchemaBuilder().String().Build()).
        RequiredProperty("size", mcp.NewSchemaBuilder().Number().Build()).
        RequiredProperty("modified", mcp.NewSchemaBuilder().String().Build()).
        RequiredProperty("type", mcp.NewSchemaBuilder().String().Build()).
        OptionalProperty("permissions", mcp.NewSchemaBuilder().
            Object().
            RequiredProperty("readable", mcp.NewSchemaBuilder().Boolean().Build()).
            RequiredProperty("writable", mcp.NewSchemaBuilder().Boolean().Build()).
            RequiredProperty("executable", mcp.NewSchemaBuilder().Boolean().Build()).
            Build()).
        Build()

    // API response schema with error handling
    apiResponseSchema := mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("success", mcp.NewSchemaBuilder().Boolean().Build()).
        OptionalProperty("data", mcp.NewSchemaBuilder().Object().Build()).
        OptionalProperty("error", mcp.NewSchemaBuilder().
            Object().
            RequiredProperty("code", mcp.NewSchemaBuilder().Number().Build()).
            RequiredProperty("message", mcp.NewSchemaBuilder().String().Build()).
            OptionalProperty("details", mcp.NewSchemaBuilder().String().Build()).
            Build()).
        Build()

    // Validate sample data
    sampleData := map[string]interface{}{
        "id":    123,
        "name":  "John Doe",
        "email": "john@example.com",
        "profile": map[string]interface{}{
            "age": 30,
            "location": map[string]interface{}{
                "city":    "New York",
                "country": "USA",
            },
            "tags": []interface{}{"developer", "go", "mcp"},
        },
    }

    if validator.ValidateAgainstSchema(sampleData, userProfileSchema) {
        fmt.Println("User profile data is valid")
    } else {
        fmt.Println("User profile validation failed")
    }

    fmt.Printf("Created schemas for user profiles, files, and API responses\n")
}
```

### Tool Manager Integration

```go
func toolManagerIntegration(ctx context.Context, servers []interfaces.MCPServer) {
    // Create tool manager with schema support
    toolManager := mcp.NewToolManager(servers)

    // Enhanced tool with output schema
    weatherTool := interfaces.MCPTool{
        Name:        "enhanced-weather",
        Description: "Get weather with validated output",
        Schema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "location": map[string]interface{}{
                    "type":        "string",
                    "description": "City name or coordinates",
                },
            },
            "required": []string{"location"},
        },
        OutputSchema: mcp.NewSchemaBuilder().
            Object().
            RequiredProperty("temperature", mcp.NewSchemaBuilder().Number().Build()).
            RequiredProperty("conditions", mcp.NewSchemaBuilder().String().Build()).
            RequiredProperty("humidity", mcp.NewSchemaBuilder().Number().Build()).
            OptionalProperty("forecast", mcp.NewSchemaBuilder().
                Array(mcp.NewSchemaBuilder().
                    Object().
                    RequiredProperty("day", mcp.NewSchemaBuilder().String().Build()).
                    RequiredProperty("high", mcp.NewSchemaBuilder().Number().Build()).
                    RequiredProperty("low", mcp.NewSchemaBuilder().Number().Build()).
                    Build()).
                Build()).
            Build(),
    }

    // Execute with automatic validation
    result, err := toolManager.ExecuteToolWithValidation(ctx, weatherTool, map[string]interface{}{
        "location": "San Francisco",
    })
    if err != nil {
        log.Printf("Tool execution or validation failed: %v", err)
        return
    }

    fmt.Printf("Validated tool result: %s\n", result.Content)

    // Get structured, type-safe result
    weather := result.StructuredContent.(map[string]interface{})
    temperature := weather["temperature"].(float64)
    conditions := weather["conditions"].(string)

    fmt.Printf("Weather: %.1f°C, %s\n", temperature, conditions)

    // Access optional forecast if available
    if forecast, exists := weather["forecast"]; exists {
        forecastArray := forecast.([]interface{})
        for _, dayForecast := range forecastArray {
            day := dayForecast.(map[string]interface{})
            fmt.Printf("  %s: High %.1f°C, Low %.1f°C\n",
                day["day"].(string),
                day["high"].(float64),
                day["low"].(float64))
        }
    }
}
```

## SchemaValidator API

### Core Methods

```go
type SchemaValidator struct {
    compiledSchemas map[string]*jsonSchema
    mutex          sync.RWMutex
}

// Create new schema validator
func NewSchemaValidator() *SchemaValidator

// Validate tool response against its output schema
func (sv *SchemaValidator) ValidateToolResponse(ctx context.Context, tool interfaces.MCPTool, response *interfaces.MCPToolResponse) error

// Validate data against arbitrary schema
func (sv *SchemaValidator) ValidateAgainstSchema(data interface{}, schema map[string]interface{}) bool

// Get detailed validation errors
func (sv *SchemaValidator) ValidateWithErrors(data interface{}, schema map[string]interface{}) []ValidationError

// Compile schema for better performance
func (sv *SchemaValidator) CompileSchema(name string, schema map[string]interface{}) error
```

### Schema Compilation

```go
// Pre-compile frequently used schemas
func (sv *SchemaValidator) CompileSchema(name string, schema map[string]interface{}) error

// Check if schema is compiled
func (sv *SchemaValidator) IsSchemaCompiled(name string) bool

// Validate using compiled schema
func (sv *SchemaValidator) ValidateWithCompiledSchema(name string, data interface{}) error

// Get compilation statistics
func (sv *SchemaValidator) GetCompilationStats() CompilationStats
```

### Validation Reporting

```go
// Detailed validation with full error context
func (sv *SchemaValidator) ValidateDetailed(data interface{}, schema map[string]interface{}) *ValidationResult

// Get validation summary
func (sv *SchemaValidator) GetValidationSummary(data interface{}, schema map[string]interface{}) ValidationSummary

// Format validation errors for display
func (sv *SchemaValidator) FormatValidationErrors(errors []ValidationError) string
```

## SchemaBuilder API

### Basic Types

```go
type SchemaBuilder struct {
    schema map[string]interface{}
}

// Create new schema builder
func NewSchemaBuilder() *SchemaBuilder

// Basic type builders
func (sb *SchemaBuilder) Object() *SchemaBuilder
func (sb *SchemaBuilder) Array(itemSchema map[string]interface{}) *SchemaBuilder
func (sb *SchemaBuilder) String() *SchemaBuilder
func (sb *SchemaBuilder) Number() *SchemaBuilder
func (sb *SchemaBuilder) Boolean() *SchemaBuilder

// Build final schema
func (sb *SchemaBuilder) Build() map[string]interface{}
```

### Object Properties

```go
// Add properties to object schemas
func (sb *SchemaBuilder) Property(name string, propertySchema map[string]interface{}) *SchemaBuilder

// Convenience methods for required properties
func (sb *SchemaBuilder) RequiredProperty(name string, propertySchema map[string]interface{}) *SchemaBuilder

// Convenience methods for optional properties
func (sb *SchemaBuilder) OptionalProperty(name string, propertySchema map[string]interface{}) *SchemaBuilder

// Set required fields
func (sb *SchemaBuilder) Required(fields ...string) *SchemaBuilder

// Add description
func (sb *SchemaBuilder) Description(desc string) *SchemaBuilder
```

### Advanced Constraints

```go
// String constraints
func (sb *SchemaBuilder) MinLength(min int) *SchemaBuilder
func (sb *SchemaBuilder) MaxLength(max int) *SchemaBuilder
func (sb *SchemaBuilder) Pattern(regex string) *SchemaBuilder
func (sb *SchemaBuilder) Format(format string) *SchemaBuilder // email, uri, date-time, etc.

// Number constraints
func (sb *SchemaBuilder) Minimum(min float64) *SchemaBuilder
func (sb *SchemaBuilder) Maximum(max float64) *SchemaBuilder
func (sb *SchemaBuilder) MultipleOf(multiple float64) *SchemaBuilder

// Array constraints
func (sb *SchemaBuilder) MinItems(min int) *SchemaBuilder
func (sb *SchemaBuilder) MaxItems(max int) *SchemaBuilder
func (sb *SchemaBuilder) UniqueItems(unique bool) *SchemaBuilder

// Enum values
func (sb *SchemaBuilder) Enum(values ...interface{}) *SchemaBuilder

// Default values
func (sb *SchemaBuilder) Default(value interface{}) *SchemaBuilder
```

## ToolManager API

### Enhanced Tool Execution

```go
type ToolManager struct {
    servers   []interfaces.MCPServer
    validator *SchemaValidator
}

// Create tool manager with schema validation
func NewToolManager(servers []interfaces.MCPServer) *ToolManager

// Execute tool with automatic validation
func (tm *ToolManager) ExecuteToolWithValidation(ctx context.Context, tool interfaces.MCPTool, args interface{}) (*interfaces.MCPToolResponse, error)

// Get tools with schema information
func (tm *ToolManager) GetToolsWithSchemas(ctx context.Context) ([]interfaces.MCPTool, error)

// Validate tool arguments before execution
func (tm *ToolManager) ValidateToolArguments(tool interfaces.MCPTool, args interface{}) error
```

### Schema Discovery

```go
// Discover schemas from tool definitions
func (tm *ToolManager) DiscoverToolSchemas(ctx context.Context) (map[string]map[string]interface{}, error)

// Get tools by schema complexity
func (tm *ToolManager) GetToolsBySchemaComplexity(ctx context.Context) (simple, medium, complex []interfaces.MCPTool, err error)

// Find tools with output schemas
func (tm *ToolManager) GetToolsWithOutputSchemas(ctx context.Context) ([]interfaces.MCPTool, error)
```

### Category Organization

```go
// Get tools by category based on schema patterns
func (tm *ToolManager) GetToolsByCategory(ctx context.Context, category string) ([]interfaces.MCPTool, error)

// Categorize tools automatically based on schemas
func (tm *ToolManager) CategorizeToolsBySchema(ctx context.Context) (map[string][]interfaces.MCPTool, error)
```

## Data Structures

### Enhanced MCPTool

```go
type MCPTool struct {
    Name         string                 `json:"name"`
    Description  string                 `json:"description"`
    Schema       interface{}            `json:"inputSchema,omitempty"`   // Input validation
    OutputSchema interface{}            `json:"outputSchema,omitempty"`  // Output validation
    Metadata     map[string]interface{} `json:"metadata,omitempty"`
}
```

### Enhanced MCPToolResponse

```go
type MCPToolResponse struct {
    Content           interface{}            `json:"content,omitempty"`           // Raw content
    StructuredContent interface{}            `json:"structuredContent,omitempty"` // Validated structured content
    IsError           bool                   `json:"isError,omitempty"`
    Metadata          map[string]interface{} `json:"metadata,omitempty"`

    // Validation information
    ValidationResult *ValidationResult `json:"validationResult,omitempty"`
    SchemaVersion    string           `json:"schemaVersion,omitempty"`
}
```

### Validation Structures

```go
type ValidationError struct {
    Path        string      `json:"path"`        // JSON path where error occurred
    Message     string      `json:"message"`     // Human-readable error message
    Value       interface{} `json:"value"`       // The invalid value
    Constraint  string      `json:"constraint"`  // The constraint that was violated
    SchemaPath  string      `json:"schemaPath"`  // Path in the schema
}

type ValidationResult struct {
    Valid  bool              `json:"valid"`
    Errors []ValidationError `json:"errors,omitempty"`
}

type ValidationSummary struct {
    TotalFields    int `json:"totalFields"`
    ValidFields    int `json:"validFields"`
    ErrorCount     int `json:"errorCount"`
    WarningCount   int `json:"warningCount"`
    SchemaVersion  string `json:"schemaVersion"`
}

type CompilationStats struct {
    CompiledCount    int           `json:"compiledCount"`
    AvgCompileTime   time.Duration `json:"avgCompileTime"`
    TotalCompileTime time.Duration `json:"totalCompileTime"`
    CacheHitRate     float64       `json:"cacheHitRate"`
}
```

## Pre-built Schema Templates

### Weather Schema

```go
func WeatherSchema() map[string]interface{} {
    return mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("temperature", mcp.NewSchemaBuilder().Number().Build()).
        RequiredProperty("conditions", mcp.NewSchemaBuilder().String().Build()).
        OptionalProperty("humidity", mcp.NewSchemaBuilder().Number().Minimum(0).Maximum(100).Build()).
        OptionalProperty("wind_speed", mcp.NewSchemaBuilder().Number().Minimum(0).Build()).
        OptionalProperty("visibility", mcp.NewSchemaBuilder().Number().Minimum(0).Build()).
        OptionalProperty("pressure", mcp.NewSchemaBuilder().Number().Minimum(0).Build()).
        Build()
}
```

### File Information Schema

```go
func FileInfoSchema() map[string]interface{} {
    return mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("name", mcp.NewSchemaBuilder().String().MinLength(1).Build()).
        RequiredProperty("size", mcp.NewSchemaBuilder().Number().Minimum(0).Build()).
        RequiredProperty("modified", mcp.NewSchemaBuilder().String().Format("date-time").Build()).
        RequiredProperty("type", mcp.NewSchemaBuilder().String().
            Enum("file", "directory", "symlink").Build()).
        OptionalProperty("permissions", mcp.NewSchemaBuilder().
            Object().
            RequiredProperty("readable", mcp.NewSchemaBuilder().Boolean().Build()).
            RequiredProperty("writable", mcp.NewSchemaBuilder().Boolean().Build()).
            RequiredProperty("executable", mcp.NewSchemaBuilder().Boolean().Build()).
            Build()).
        OptionalProperty("owner", mcp.NewSchemaBuilder().String().Build()).
        OptionalProperty("group", mcp.NewSchemaBuilder().String().Build()).
        Build()
}
```

### API Response Schema

```go
func APIResponseSchema() map[string]interface{} {
    return mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("success", mcp.NewSchemaBuilder().Boolean().Build()).
        RequiredProperty("timestamp", mcp.NewSchemaBuilder().String().Format("date-time").Build()).
        OptionalProperty("data", mcp.NewSchemaBuilder().Object().Build()).
        OptionalProperty("error", mcp.NewSchemaBuilder().
            Object().
            RequiredProperty("code", mcp.NewSchemaBuilder().Number().Build()).
            RequiredProperty("message", mcp.NewSchemaBuilder().String().Build()).
            OptionalProperty("details", mcp.NewSchemaBuilder().String().Build()).
            Build()).
        OptionalProperty("pagination", mcp.NewSchemaBuilder().
            Object().
            RequiredProperty("page", mcp.NewSchemaBuilder().Number().Minimum(1).Build()).
            RequiredProperty("per_page", mcp.NewSchemaBuilder().Number().Minimum(1).Maximum(100).Build()).
            RequiredProperty("total", mcp.NewSchemaBuilder().Number().Minimum(0).Build()).
            Build()).
        Build()
}
```

### Database Record Schema

```go
func DatabaseRecordSchema() map[string]interface{} {
    return mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("id", mcp.NewSchemaBuilder().Number().Build()).
        RequiredProperty("created_at", mcp.NewSchemaBuilder().String().Format("date-time").Build()).
        RequiredProperty("updated_at", mcp.NewSchemaBuilder().String().Format("date-time").Build()).
        OptionalProperty("deleted_at", mcp.NewSchemaBuilder().String().Format("date-time").Build()).
        OptionalProperty("version", mcp.NewSchemaBuilder().Number().Minimum(1).Build()).
        OptionalProperty("metadata", mcp.NewSchemaBuilder().Object().Build()).
        Build()
}
```

## Error Handling

### Schema Validation Errors

```go
func handleSchemaValidationError(err error) {
    if validationErr, ok := err.(*SchemaValidationError); ok {
        fmt.Printf("Schema validation failed:\n")
        for _, fieldErr := range validationErr.Errors {
            fmt.Printf("  Field '%s': %s (got %v)\n",
                fieldErr.Path,
                fieldErr.Message,
                fieldErr.Value)
        }

        // Suggest fixes
        suggestions := validationErr.GetSuggestions()
        if len(suggestions) > 0 {
            fmt.Printf("Suggestions:\n")
            for _, suggestion := range suggestions {
                fmt.Printf("  - %s\n", suggestion)
            }
        }
    }
}

type SchemaValidationError struct {
    ToolName string
    Errors   []ValidationError
}

func (sve *SchemaValidationError) Error() string {
    return fmt.Sprintf("schema validation failed for tool %s: %d errors", sve.ToolName, len(sve.Errors))
}

func (sve *SchemaValidationError) GetSuggestions() []string {
    suggestions := []string{}

    for _, err := range sve.Errors {
        switch err.Constraint {
        case "required":
            suggestions = append(suggestions, fmt.Sprintf("Add required field '%s'", err.Path))
        case "type":
            suggestions = append(suggestions, fmt.Sprintf("Convert '%s' to correct type", err.Path))
        case "format":
            suggestions = append(suggestions, fmt.Sprintf("Fix format for '%s'", err.Path))
        case "enum":
            suggestions = append(suggestions, fmt.Sprintf("Use valid enum value for '%s'", err.Path))
        }
    }

    return suggestions
}
```

### Schema Compilation Errors

```go
func handleSchemaCompilationError(err error) {
    if compileErr, ok := err.(*SchemaCompilationError); ok {
        fmt.Printf("Schema compilation failed: %s\n", compileErr.Message)
        fmt.Printf("Schema path: %s\n", compileErr.SchemaPath)

        if compileErr.Line > 0 {
            fmt.Printf("Line: %d\n", compileErr.Line)
        }

        // Common fixes
        fmt.Printf("Common issues:\n")
        fmt.Printf("- Check JSON syntax\n")
        fmt.Printf("- Verify schema keywords\n")
        fmt.Printf("- Ensure proper nesting\n")
    }
}

type SchemaCompilationError struct {
    SchemaName string
    SchemaPath string
    Line       int
    Message    string
}

func (sce *SchemaCompilationError) Error() string {
    return fmt.Sprintf("schema compilation error: %s", sce.Message)
}
```

## Performance Optimizations

### Schema Caching

```go
type SchemaCache struct {
    cache      map[string]*compiledSchema
    mutex      sync.RWMutex
    maxSize    int
    ttl        time.Duration
}

func NewSchemaCache(maxSize int, ttl time.Duration) *SchemaCache {
    return &SchemaCache{
        cache:   make(map[string]*compiledSchema),
        maxSize: maxSize,
        ttl:     ttl,
    }
}

func (sc *SchemaCache) Get(key string) (*compiledSchema, bool) {
    sc.mutex.RLock()
    defer sc.mutex.RUnlock()

    schema, exists := sc.cache[key]
    if !exists {
        return nil, false
    }

    // Check TTL
    if time.Since(schema.compiledAt) > sc.ttl {
        go sc.evict(key) // Async eviction
        return nil, false
    }

    return schema, true
}
```

### Batch Validation

```go
// Validate multiple responses efficiently
func (sv *SchemaValidator) ValidateMultiple(ctx context.Context, validations []ValidationRequest) []ValidationResult {
    results := make([]ValidationResult, len(validations))

    // Use worker pool for parallel validation
    workerPool := sync.Pool{
        New: func() interface{} {
            return &schemaValidator{}
        },
    }

    var wg sync.WaitGroup
    for i, req := range validations {
        wg.Add(1)
        go func(idx int, request ValidationRequest) {
            defer wg.Done()

            validator := workerPool.Get().(*schemaValidator)
            defer workerPool.Put(validator)

            result := validator.validate(request.Data, request.Schema)
            results[idx] = result
        }(i, req)
    }

    wg.Wait()
    return results
}

type ValidationRequest struct {
    Data   interface{}
    Schema map[string]interface{}
    Name   string
}
```

### Schema Compilation

```go
// Pre-compile schemas for better performance
func (sv *SchemaValidator) PrecompileSchemas(schemas map[string]map[string]interface{}) error {
    compiled := make(map[string]*jsonSchema, len(schemas))

    // Compile in parallel
    var wg sync.WaitGroup
    var mutex sync.Mutex
    errors := []error{}

    for name, schema := range schemas {
        wg.Add(1)
        go func(schemaName string, schemaData map[string]interface{}) {
            defer wg.Done()

            compiledSchema, err := compileSchema(schemaData)
            if err != nil {
                mutex.Lock()
                errors = append(errors, fmt.Errorf("failed to compile schema %s: %w", schemaName, err))
                mutex.Unlock()
                return
            }

            mutex.Lock()
            compiled[schemaName] = compiledSchema
            mutex.Unlock()
        }(name, schema)
    }

    wg.Wait()

    if len(errors) > 0 {
        return fmt.Errorf("schema compilation errors: %v", errors)
    }

    sv.mutex.Lock()
    for name, schema := range compiled {
        sv.compiledSchemas[name] = schema
    }
    sv.mutex.Unlock()

    return nil
}
```

## Integration Examples

### File System Tools

```go
func setupFileSystemSchemas(toolManager *mcp.ToolManager) {
    // File listing schema
    fileListSchema := mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("files", mcp.NewSchemaBuilder().
            Array(FileInfoSchema()).
            Build()).
        RequiredProperty("path", mcp.NewSchemaBuilder().String().Build()).
        RequiredProperty("total_count", mcp.NewSchemaBuilder().Number().Minimum(0).Build()).
        Build()

    // File read schema
    fileContentSchema := mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("content", mcp.NewSchemaBuilder().String().Build()).
        RequiredProperty("encoding", mcp.NewSchemaBuilder().String().
            Enum("utf-8", "base64", "binary").
            Default("utf-8").
            Build()).
        RequiredProperty("size", mcp.NewSchemaBuilder().Number().Minimum(0).Build()).
        OptionalProperty("metadata", FileInfoSchema()).
        Build()

    // Register schemas
    toolManager.RegisterToolSchema("list-files", nil, fileListSchema)
    toolManager.RegisterToolSchema("read-file", nil, fileContentSchema)
}
```

### API Integration Tools

```go
func setupAPISchemas(toolManager *mcp.ToolManager) {
    // HTTP request schema
    httpRequestSchema := mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("url", mcp.NewSchemaBuilder().String().Format("uri").Build()).
        RequiredProperty("method", mcp.NewSchemaBuilder().String().
            Enum("GET", "POST", "PUT", "DELETE", "PATCH").
            Default("GET").
            Build()).
        OptionalProperty("headers", mcp.NewSchemaBuilder().Object().Build()).
        OptionalProperty("body", mcp.NewSchemaBuilder().String().Build()).
        OptionalProperty("timeout", mcp.NewSchemaBuilder().Number().Minimum(1).Maximum(300).Build()).
        Build()

    // HTTP response schema
    httpResponseSchema := mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("status_code", mcp.NewSchemaBuilder().Number().Minimum(100).Maximum(599).Build()).
        RequiredProperty("headers", mcp.NewSchemaBuilder().Object().Build()).
        RequiredProperty("body", mcp.NewSchemaBuilder().String().Build()).
        RequiredProperty("response_time", mcp.NewSchemaBuilder().Number().Minimum(0).Build()).
        OptionalProperty("error", mcp.NewSchemaBuilder().String().Build()).
        Build()

    toolManager.RegisterToolSchema("http-request", httpRequestSchema, httpResponseSchema)
}
```

### Database Tools

```go
func setupDatabaseSchemas(toolManager *mcp.ToolManager) {
    // Query input schema
    queryInputSchema := mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("sql", mcp.NewSchemaBuilder().String().MinLength(1).Build()).
        OptionalProperty("parameters", mcp.NewSchemaBuilder().Array(
            mcp.NewSchemaBuilder().Object().Build(),
        ).Build()).
        OptionalProperty("timeout", mcp.NewSchemaBuilder().Number().Minimum(1).Maximum(300).Build()).
        Build()

    // Query result schema
    queryResultSchema := mcp.NewSchemaBuilder().
        Object().
        RequiredProperty("rows", mcp.NewSchemaBuilder().Array(
            mcp.NewSchemaBuilder().Object().Build(),
        ).Build()).
        RequiredProperty("row_count", mcp.NewSchemaBuilder().Number().Minimum(0).Build()).
        RequiredProperty("columns", mcp.NewSchemaBuilder().Array(
            mcp.NewSchemaBuilder().String().Build(),
        ).Build()).
        RequiredProperty("execution_time", mcp.NewSchemaBuilder().Number().Minimum(0).Build()).
        OptionalProperty("affected_rows", mcp.NewSchemaBuilder().Number().Minimum(0).Build()).
        Build()

    toolManager.RegisterToolSchema("execute-query", queryInputSchema, queryResultSchema)
}
```

## Best Practices

### 1. Schema Design Principles

```go
// Good: Specific, descriptive schemas
weatherSchema := mcp.NewSchemaBuilder().
    Object().
    RequiredProperty("temperature", mcp.NewSchemaBuilder().
        Number().
        Minimum(-100).
        Maximum(100).
        Description("Temperature in Celsius").
        Build()).
    RequiredProperty("conditions", mcp.NewSchemaBuilder().
        String().
        Enum("sunny", "cloudy", "rainy", "snowy", "foggy").
        Description("Current weather conditions").
        Build()).
    Build()

// Avoid: Too generic schemas
badSchema := mcp.NewSchemaBuilder().
    Object().
    OptionalProperty("data", mcp.NewSchemaBuilder().Object().Build()).
    Build()
```

### 2. Error-Tolerant Validation

```go
func validateWithFallback(validator *mcp.SchemaValidator, data interface{}, schema map[string]interface{}) (bool, []string) {
    result := validator.ValidateDetailed(data, schema)

    if result.Valid {
        return true, nil
    }

    // Check if errors are recoverable
    warnings := []string{}
    criticalErrors := []error{}

    for _, err := range result.Errors {
        if isRecoverableError(err) {
            warnings = append(warnings, fmt.Sprintf("Warning: %s", err.Message))
        } else {
            criticalErrors = append(criticalErrors, err)
        }
    }

    // Allow validation to pass with warnings if no critical errors
    return len(criticalErrors) == 0, warnings
}

func isRecoverableError(err ValidationError) bool {
    switch err.Constraint {
    case "format": // Format errors are often recoverable
        return true
    case "additional_properties": // Extra properties can be ignored
        return true
    default:
        return false
    }
}
```

### 3. Schema Versioning

```go
type VersionedSchema struct {
    Version string                 `json:"version"`
    Schema  map[string]interface{} `json:"schema"`
}

func (vs *VersionedSchema) IsCompatible(otherVersion string) bool {
    return compareVersions(vs.Version, otherVersion) >= 0
}

// Schema migration
func migrateSchema(data interface{}, fromVersion, toVersion string) (interface{}, error) {
    migrations := getSchemaeMigrations(fromVersion, toVersion)

    current := data
    for _, migration := range migrations {
        migrated, err := migration.Apply(current)
        if err != nil {
            return nil, fmt.Errorf("migration failed: %w", err)
        }
        current = migrated
    }

    return current, nil
}
```

## Troubleshooting

### Common Validation Issues

1. **Type Mismatches**
   ```go
   // Common issue: Numbers as strings
   data := map[string]interface{}{
       "temperature": "25.5", // String instead of number
   }

   // Solution: Convert types before validation
   if temp, ok := data["temperature"].(string); ok {
       if tempFloat, err := strconv.ParseFloat(temp, 64); err == nil {
           data["temperature"] = tempFloat
       }
   }
   ```

2. **Missing Required Fields**
   ```go
   // Check for required fields before validation
   required := []string{"temperature", "conditions"}
   for _, field := range required {
       if _, exists := data[field]; !exists {
           return fmt.Errorf("missing required field: %s", field)
       }
   }
   ```

3. **Schema Complexity Issues**
   ```go
   // Monitor schema validation performance
   start := time.Now()
   valid := validator.ValidateAgainstSchema(data, schema)
   duration := time.Since(start)

   if duration > 100*time.Millisecond {
       log.Printf("Slow schema validation detected: %v", duration)
       // Consider schema simplification or caching
   }
   ```

### Debugging Validation

```go
// Enable detailed validation logging
validator.SetDebugMode(true)

// Get validation trace
trace := validator.GetValidationTrace(data, schema)
for _, step := range trace.Steps {
    fmt.Printf("Step %d: %s - %s\n", step.Index, step.Path, step.Result)
}

// Analyze schema complexity
complexity := validator.AnalyzeSchemaComplexity(schema)
fmt.Printf("Schema complexity: %d (max recommended: 100)\n", complexity.Score)
if complexity.Score > 100 {
    fmt.Printf("Consider simplifying schema or breaking into smaller schemas\n")
}
```

This comprehensive documentation covers all aspects of the MCP Tool Output Schemas feature, providing developers with robust tools for ensuring type safety and data validation in their MCP applications.