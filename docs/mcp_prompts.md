# MCP Prompts Support

## Overview

The MCP Prompts feature enables agents to discover, execute, and manage dynamic prompt templates from MCP servers. Prompts are reusable templates that can be parameterized with variables, allowing for sophisticated prompt engineering and dynamic content generation workflows.

## Key Features

- **Prompt Discovery**: List all available prompts from MCP servers
- **Template Execution**: Execute prompts with variable substitution
- **Variable Validation**: Validate required and optional parameters
- **Category Organization**: Organize prompts by category and use case
- **Template Engine**: Full Go template support with custom functions
- **Smart Suggestions**: Intelligent variable suggestions and auto-completion

## Architecture

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐
│   Agent     │───▶│ PromptMgr    │───▶│ MCP Server  │
│             │    │              │    │             │
│ - ListPrmpt │    │ - Templating │    │ - Templates │
│ - GetPrompt │    │ - Validation │    │ - Variables │
│ - Execute   │    │ - Caching    │    │ - Metadata  │
└─────────────┘    └──────────────┘    └─────────────┘
```

## Usage Examples

### Basic Prompt Execution

```go
package main

import (
    "context"
    "fmt"
    "github.com/Ingenimax/agent-sdk-go/pkg/mcp"
)

func main() {
    ctx := context.Background()

    // Build agent with prompt-enabled MCP servers
    builder := mcp.NewBuilder().
        AddPreset("github").        // GitHub prompts
        AddPreset("writing")        // Writing prompts

    servers, _, err := builder.Build(ctx)
    if err != nil {
        panic(err)
    }

    // Create prompt manager
    promptManager := mcp.NewPromptManager(servers)

    // List available prompts
    prompts, err := promptManager.GetAllPrompts(ctx)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Found %d prompts:\n", len(prompts))
    for _, prompt := range prompts {
        fmt.Printf("- %s: %s\n", prompt.Name, prompt.Description)
        for _, arg := range prompt.Arguments {
            required := ""
            if arg.Required {
                required = " (required)"
            }
            fmt.Printf("  • %s: %s%s\n", arg.Name, arg.Description, required)
        }
    }

    // Execute a prompt with variables
    variables := map[string]interface{}{
        "repository": "golang/go",
        "issue_type": "bug",
        "priority":   "high",
    }

    result, err := promptManager.ExecutePrompt(ctx, "create-github-issue", variables)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Generated prompt:\n%s\n", result.Prompt)

    // Access individual messages if multi-turn
    for i, message := range result.Messages {
        fmt.Printf("Message %d (%s): %s\n", i+1, message.Role, message.Content)
    }
}
```

### Advanced Template Execution

```go
func advancedPromptUsage(ctx context.Context, manager *mcp.PromptManager) {
    // Execute prompt with template validation
    variables := map[string]interface{}{
        "customer_name":    "John Doe",
        "order_id":        "ORD-12345",
        "delivery_date":   "2024-01-15",
        "items":           []string{"Widget A", "Widget B", "Widget C"},
    }

    // Validate variables before execution
    prompt, err := manager.GetPromptByName(ctx, "customer-notification")
    if err != nil {
        panic(err)
    }

    if err := manager.ValidatePromptVariables(prompt, variables); err != nil {
        fmt.Printf("Variable validation failed: %v\n", err)

        // Get suggested variables
        suggestions := manager.SuggestPromptVariables(prompt, variables)
        fmt.Printf("Suggestions: %v\n", suggestions)
        return
    }

    // Execute with validated variables
    result, err := manager.ExecutePromptTemplate(ctx, prompt, variables)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Executed prompt: %s\n", result.Prompt)
}
```

### Prompt Categories and Organization

```go
func organizedPromptUsage(ctx context.Context, manager *mcp.PromptManager) {
    // Get prompts by category
    writingPrompts, err := manager.GetPromptsByCategory(ctx, "writing")
    if err != nil {
        panic(err)
    }

    codePrompts, err := manager.GetPromptsByCategory(ctx, "code-generation")
    if err != nil {
        panic(err)
    }

    analysisPrompts, err := manager.GetPromptsByCategory(ctx, "data-analysis")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Available prompt categories:\n")
    fmt.Printf("- Writing: %d prompts\n", len(writingPrompts))
    fmt.Printf("- Code Generation: %d prompts\n", len(codePrompts))
    fmt.Printf("- Data Analysis: %d prompts\n", len(analysisPrompts))

    // Search prompts by keyword
    searchResults, err := manager.SearchPrompts(ctx, "email")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Found %d prompts matching 'email':\n", len(searchResults))
    for _, prompt := range searchResults {
        fmt.Printf("- %s: %s\n", prompt.Name, prompt.Description)
    }
}
```

## PromptManager API

### Core Methods

```go
type PromptManager struct {
    servers []interfaces.MCPServer
    cache   *promptCache
}

// Create new prompt manager
func NewPromptManager(servers []interfaces.MCPServer) *PromptManager

// List all prompts across all servers
func (pm *PromptManager) GetAllPrompts(ctx context.Context) ([]interfaces.MCPPrompt, error)

// Get specific prompt by name
func (pm *PromptManager) GetPromptByName(ctx context.Context, name string) (interfaces.MCPPrompt, error)

// Execute prompt with variables
func (pm *PromptManager) ExecutePrompt(ctx context.Context, name string, variables map[string]interface{}) (*interfaces.MCPPromptResult, error)

// Execute prompt template directly
func (pm *PromptManager) ExecutePromptTemplate(ctx context.Context, prompt interfaces.MCPPrompt, variables map[string]interface{}) (*interfaces.MCPPromptResult, error)
```

### Validation Methods

```go
// Validate prompt variables
func (pm *PromptManager) ValidatePromptVariables(prompt interfaces.MCPPrompt, variables map[string]interface{}) error

// Get suggestions for missing variables
func (pm *PromptManager) SuggestPromptVariables(prompt interfaces.MCPPrompt, variables map[string]interface{}) map[string]interface{}

// Check required variables
func (pm *PromptManager) GetRequiredVariables(prompt interfaces.MCPPrompt) []string

// Get optional variables
func (pm *PromptManager) GetOptionalVariables(prompt interfaces.MCPPrompt) []string
```

### Organization Methods

```go
// Get prompts by category
func (pm *PromptManager) GetPromptsByCategory(ctx context.Context, category string) ([]interfaces.MCPPrompt, error)

// Search prompts by keyword
func (pm *PromptManager) SearchPrompts(ctx context.Context, query string) ([]interfaces.MCPPrompt, error)

// Get prompts by server
func (pm *PromptManager) GetPromptsByServer(ctx context.Context, serverName string) ([]interfaces.MCPPrompt, error)

// Get all categories
func (pm *PromptManager) GetCategories(ctx context.Context) ([]string, error)
```

### Template Methods

```go
// Build variables from template analysis
func (pm *PromptManager) BuildVariablesFromTemplate(templateStr string, data interface{}) (map[string]interface{}, error)

// Parse template and extract variable names
func (pm *PromptManager) ExtractTemplateVariables(templateStr string) ([]string, error)

// Validate template syntax
func (pm *PromptManager) ValidateTemplate(templateStr string) error

// Preview prompt execution without calling server
func (pm *PromptManager) PreviewPrompt(prompt interfaces.MCPPrompt, variables map[string]interface{}) (string, error)
```

## Data Structures

### MCPPrompt

```go
type MCPPrompt struct {
    Name        string                `json:"name"`
    Description string                `json:"description"`
    Arguments   []MCPPromptArgument   `json:"arguments"`
    Metadata    map[string]string     `json:"metadata"`
}
```

### MCPPromptArgument

```go
type MCPPromptArgument struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    Required    bool   `json:"required"`
    Type        string `json:"type,omitempty"`        // string, number, boolean, array, object
    Default     interface{} `json:"default,omitempty"` // Default value
    Enum        []interface{} `json:"enum,omitempty"`  // Allowed values
}
```

### MCPPromptResult

```go
type MCPPromptResult struct {
    Variables map[string]interface{} `json:"variables"`
    Messages  []MCPPromptMessage     `json:"messages"`
    Prompt    string                 `json:"prompt"`    // Combined prompt for backward compatibility
    Metadata  map[string]string      `json:"metadata"`
}
```

### MCPPromptMessage

```go
type MCPPromptMessage struct {
    Role    string `json:"role"`    // "system", "user", "assistant"
    Content string `json:"content"`
}
```

## Template Engine

### Built-in Functions

The prompt template engine includes useful functions for dynamic content generation:

```go
// String functions
{{ .name | upper }}              // Convert to uppercase
{{ .name | lower }}              // Convert to lowercase
{{ .name | title }}              // Title case
{{ .text | truncate 100 }}       // Truncate to 100 chars
{{ .text | wordwrap 80 }}        // Word wrap at 80 chars

// Date functions
{{ now | date "2006-01-02" }}    // Current date
{{ .timestamp | dateAdd "24h" }} // Add 24 hours
{{ .date | dateSub "1d" }}       // Subtract 1 day

// Number functions
{{ .price | printf "%.2f" }}     // Format as currency
{{ .count | add 1 }}             // Add numbers
{{ .total | mul 0.1 }}           // Multiply

// Array functions
{{ .items | join ", " }}         // Join array elements
{{ .tags | slice 0 3 }}          // Get first 3 elements
{{ .list | length }}             // Get array length

// Conditional functions
{{ if .urgent }}URGENT: {{ end }}{{ .message }}
{{ .status | default "pending" }} // Default value if empty

// Custom MCP functions
{{ .repo | githubLink }}         // Generate GitHub URL
{{ .data | jsonPretty }}         // Pretty-print JSON
{{ .code | highlight "go" }}     // Syntax highlighting
```

### Template Examples

#### Email Generation

```go
// Template: "email-notification"
variables := map[string]interface{}{
    "recipient":    "john@example.com",
    "subject":      "Order Confirmation",
    "order_id":     "ORD-12345",
    "items":        []string{"Product A", "Product B"},
    "total":        99.99,
    "delivery_date": "2024-01-15",
}

// Template content:
/*
Subject: {{ .subject }}
To: {{ .recipient }}

Dear {{ .recipient | emailName }},

Your order {{ .order_id }} has been confirmed!

Items ordered:
{{- range .items }}
- {{ . }}
{{- end }}

Total: ${{ .total | printf "%.2f" }}
Expected delivery: {{ .delivery_date | date "January 2, 2006" }}

Thank you for your business!
*/
```

#### Code Generation

```go
// Template: "go-struct"
variables := map[string]interface{}{
    "package_name": "models",
    "struct_name":  "User",
    "fields": []map[string]interface{}{
        {"name": "ID", "type": "int64", "json": "id"},
        {"name": "Name", "type": "string", "json": "name"},
        {"name": "Email", "type": "string", "json": "email"},
    },
}

// Template content:
/*
package {{ .package_name }}

type {{ .struct_name }} struct {
{{- range .fields }}
    {{ .name }} {{ .type }} `json:"{{ .json }}"`
{{- end }}
}
*/
```

#### Multi-turn Conversation

```go
// Template: "technical-interview"
variables := map[string]interface{}{
    "candidate_name": "Alice Johnson",
    "position":       "Senior Go Developer",
    "experience":     "5 years",
    "technologies":   []string{"Go", "Kubernetes", "PostgreSQL"},
}

// Returns multiple messages for conversation flow
```

## Error Handling

### Validation Errors

```go
type PromptValidationError struct {
    PromptName string
    Variable   string
    Message    string
}

func (e *PromptValidationError) Error() string {
    return fmt.Sprintf("prompt validation error in %s.%s: %s",
        e.PromptName, e.Variable, e.Message)
}

// Handle validation errors
result, err := manager.ExecutePrompt(ctx, "email-template", variables)
if err != nil {
    if validErr, ok := err.(*PromptValidationError); ok {
        fmt.Printf("Missing variable: %s\n", validErr.Variable)

        // Get suggestions
        suggestions := manager.SuggestPromptVariables(prompt, variables)
        if suggestion, exists := suggestions[validErr.Variable]; exists {
            fmt.Printf("Suggested value: %v\n", suggestion)
        }
        return
    }
}
```

### Template Execution Errors

```go
type TemplateExecutionError struct {
    PromptName string
    Template   string
    Line       int
    Message    string
}

func handleTemplateError(err error) {
    if templateErr, ok := err.(*TemplateExecutionError); ok {
        fmt.Printf("Template error in %s at line %d: %s\n",
            templateErr.PromptName,
            templateErr.Line,
            templateErr.Message)
    }
}
```

## Performance Optimizations

### Caching

```go
// Enable prompt caching
manager := mcp.NewPromptManager(servers)
manager.EnableCaching(10 * time.Minute) // Cache for 10 minutes

// Cache management
manager.ClearCache()
manager.RefreshCache(ctx)
stats := manager.GetCacheStats()

// Pre-load frequently used prompts
frequentPrompts := []string{"email-template", "code-review", "summary"}
manager.PreloadPrompts(ctx, frequentPrompts)
```

### Template Compilation

```go
// Pre-compile templates for better performance
manager.PrecompileTemplates(ctx)

// Check if template is compiled
isCompiled := manager.IsTemplateCompiled("email-template")

// Get compilation stats
stats := manager.GetCompilationStats()
fmt.Printf("Compiled templates: %d\n", stats.CompiledCount)
fmt.Printf("Average compile time: %v\n", stats.AvgCompileTime)
```

### Batch Operations

```go
// Execute multiple prompts efficiently
promptRequests := []PromptRequest{
    {Name: "email-template", Variables: emailVars},
    {Name: "sms-template", Variables: smsVars},
    {Name: "push-notification", Variables: pushVars},
}

results, err := manager.ExecuteMultiplePrompts(ctx, promptRequests)
if err != nil {
    panic(err)
}

for name, result := range results {
    fmt.Printf("Prompt %s: %s\n", name, result.Prompt)
}
```

## Security Considerations

### Variable Sanitization

```go
// Automatic sanitization of user input
variables := map[string]interface{}{
    "user_input": "<script>alert('xss')</script>", // Will be escaped
    "html_content": template.HTML("<b>safe</b>"),  // Explicitly marked as safe
}

// Configure sanitization rules
config := &PromptManagerConfig{
    SanitizeInput:     true,
    AllowedHTMLTags:   []string{"b", "i", "em", "strong"},
    EscapeJavaScript:  true,
    MaxVariableLength: 10000,
}

manager := NewPromptManagerWithConfig(servers, config)
```

### Template Security

```go
// Disable dangerous template functions
manager.DisableFunctions([]string{"exec", "system", "env"})

// Restrict template complexity
manager.SetMaxTemplateDepth(10)
manager.SetMaxLoopIterations(1000)
manager.SetTemplateTimeout(5 * time.Second)

// Validate templates before execution
if err := manager.ValidateTemplate(templateStr); err != nil {
    log.Printf("Unsafe template detected: %v", err)
    return
}
```

## Integration Examples

### Email Campaign System

```go
func sendEmailCampaign(ctx context.Context, manager *mcp.PromptManager) {
    // Get email template
    template, err := manager.GetPromptByName(ctx, "marketing-email")
    if err != nil {
        panic(err)
    }

    // Customer list
    customers := []Customer{
        {Name: "John Doe", Email: "john@example.com", Segment: "premium"},
        {Name: "Jane Smith", Email: "jane@example.com", Segment: "standard"},
    }

    // Generate personalized emails
    for _, customer := range customers {
        variables := map[string]interface{}{
            "name":         customer.Name,
            "email":        customer.Email,
            "segment":      customer.Segment,
            "offers":       getOffersForSegment(customer.Segment),
            "unsubscribe":  generateUnsubscribeLink(customer.Email),
        }

        result, err := manager.ExecutePromptTemplate(ctx, template, variables)
        if err != nil {
            log.Printf("Failed to generate email for %s: %v", customer.Name, err)
            continue
        }

        // Send email
        sendEmail(customer.Email, result.Prompt)
    }
}
```

### Code Generation Workflow

```go
func generateAPICode(ctx context.Context, manager *mcp.PromptManager) {
    // API specification
    apiSpec := APISpec{
        Name:      "UserService",
        Package:   "services",
        Endpoints: []Endpoint{
            {Method: "GET", Path: "/users", Handler: "ListUsers"},
            {Method: "POST", Path: "/users", Handler: "CreateUser"},
        },
    }

    // Generate different code components
    templates := []string{
        "go-api-handler",
        "go-api-routes",
        "go-api-models",
        "go-api-tests",
    }

    for _, templateName := range templates {
        variables := map[string]interface{}{
            "spec":      apiSpec,
            "timestamp": time.Now(),
            "author":    "Code Generator",
        }

        result, err := manager.ExecutePrompt(ctx, templateName, variables)
        if err != nil {
            log.Printf("Failed to generate %s: %v", templateName, err)
            continue
        }

        // Write generated code to file
        filename := fmt.Sprintf("%s.go", strings.ReplaceAll(templateName, "-", "_"))
        writeFile(filename, result.Prompt)
    }
}
```

### Documentation Generation

```go
func generateDocumentation(ctx context.Context, manager *mcp.PromptManager) {
    // API documentation
    result, err := manager.ExecutePrompt(ctx, "api-documentation", map[string]interface{}{
        "service_name": "UserAPI",
        "version":      "v1.0.0",
        "endpoints":    getAPIEndpoints(),
        "examples":     getAPIExamples(),
    })
    if err != nil {
        panic(err)
    }

    writeFile("api-docs.md", result.Prompt)

    // README generation
    readme, err := manager.ExecutePrompt(ctx, "readme-template", map[string]interface{}{
        "project_name":   "MyProject",
        "description":    "A powerful API service",
        "features":       getProjectFeatures(),
        "installation":   getInstallationSteps(),
    })
    if err != nil {
        panic(err)
    }

    writeFile("README.md", readme.Prompt)
}
```

## Best Practices

### 1. Prompt Organization

```go
// Organize prompts by domain
domains := map[string][]string{
    "email":        {"welcome-email", "notification-email", "reminder-email"},
    "code":         {"go-struct", "api-handler", "test-case"},
    "documentation": {"api-docs", "readme", "changelog"},
    "content":      {"blog-post", "social-media", "press-release"},
}

// Use consistent naming conventions
// Format: {domain}-{purpose}-{variant}
// Examples:
// - "email-welcome-new-user"
// - "code-api-crud-handler"
// - "docs-api-endpoint-reference"
```

### 2. Variable Validation

```go
func validatePromptExecution(ctx context.Context, manager *mcp.PromptManager, name string, variables map[string]interface{}) error {
    // Get prompt definition
    prompt, err := manager.GetPromptByName(ctx, name)
    if err != nil {
        return err
    }

    // Validate all required variables are present
    for _, arg := range prompt.Arguments {
        if arg.Required {
            if _, exists := variables[arg.Name]; !exists {
                return fmt.Errorf("missing required variable: %s", arg.Name)
            }
        }
    }

    // Type validation
    for _, arg := range prompt.Arguments {
        if value, exists := variables[arg.Name]; exists {
            if !validateType(value, arg.Type) {
                return fmt.Errorf("invalid type for %s: expected %s", arg.Name, arg.Type)
            }
        }
    }

    // Enum validation
    for _, arg := range prompt.Arguments {
        if len(arg.Enum) > 0 {
            if value, exists := variables[arg.Name]; exists {
                if !contains(arg.Enum, value) {
                    return fmt.Errorf("invalid value for %s: must be one of %v", arg.Name, arg.Enum)
                }
            }
        }
    }

    return nil
}
```

### 3. Error Recovery

```go
func robustPromptExecution(ctx context.Context, manager *mcp.PromptManager, name string, variables map[string]interface{}) (*interfaces.MCPPromptResult, error) {
    // Try execution with validation
    result, err := manager.ExecutePrompt(ctx, name, variables)
    if err == nil {
        return result, nil
    }

    // Handle validation errors
    if validErr, ok := err.(*PromptValidationError); ok {
        // Try to auto-fix with suggestions
        suggestions := manager.SuggestPromptVariables(prompt, variables)
        if suggestion, exists := suggestions[validErr.Variable]; exists {
            variables[validErr.Variable] = suggestion
            return manager.ExecutePrompt(ctx, name, variables)
        }
    }

    // Handle template errors
    if templateErr, ok := err.(*TemplateExecutionError); ok {
        log.Printf("Template execution failed: %v", templateErr)
        // Could fallback to a simpler template
        return manager.ExecutePrompt(ctx, name+"-simple", variables)
    }

    return nil, err
}
```

## Troubleshooting

### Common Issues

1. **Template Compilation Errors**
   ```go
   if err := manager.ValidateTemplate(templateStr); err != nil {
       log.Printf("Template syntax error: %v", err)
       // Check for common issues:
       // - Unclosed template blocks {{ ... }}
       // - Invalid function names
       // - Missing variable references
   }
   ```

2. **Variable Type Mismatches**
   ```go
   // Ensure correct types
   variables := map[string]interface{}{
       "count":     123,           // int
       "price":     99.99,         // float64
       "enabled":   true,          // bool
       "items":     []string{...}, // slice
       "metadata":  map[string]interface{}{...}, // map
   }
   ```

3. **Performance Issues**
   ```go
   // Enable caching for frequently used prompts
   manager.EnableCaching(15 * time.Minute)

   // Pre-compile templates
   manager.PrecompileTemplates(ctx)

   // Monitor performance
   stats := manager.GetPerformanceStats()
   if stats.AvgExecutionTime > 1*time.Second {
       log.Printf("Slow prompt execution detected: %v", stats.AvgExecutionTime)
   }
   ```

### Debugging

```go
// Enable debug mode
manager.SetDebugMode(true)

// Trace template execution
manager.EnableTracing(true)
trace := manager.GetExecutionTrace(name)
for _, step := range trace.Steps {
    fmt.Printf("Step %d: %s (%v)\n", step.Index, step.Description, step.Duration)
}

// Monitor prompt usage
usage := manager.GetUsageStats()
fmt.Printf("Most used prompts: %v\n", usage.TopPrompts)
fmt.Printf("Error rate: %.2f%%\n", usage.ErrorRate)
```

This comprehensive documentation covers all aspects of the MCP Prompts feature, providing developers with everything needed to effectively use dynamic prompt templates in their MCP-enabled applications.