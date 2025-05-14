# Graph Workflow Example

This example demonstrates a graph-based workflow for generating Terraform modules using the agent-sdk-go framework. The workflow implements a multi-agent system that plans, generates, and reviews Terraform configurations.

## Overview

The workflow consists of three main agents connected in a graph structure:

1. **Planner Agent**: Analyzes requirements and creates a plan for the Terraform module
2. **Generator Agent**: Creates Terraform files based on the plan
3. **Reviewer Agent**: Reviews the generated files and ensures they meet quality standards

## Workflow Structure

```
[Planner] --> [Generator] --> [Reviewer]
                ^              |
                |              |
                +--------------+
```

### Benefits of Using Graphs

The graph-based workflow architecture offers several key advantages:

1. **Flexible Flow Control**
   - Dynamic routing between agents based on conditions
   - Support for parallel processing when needed
   - Easy addition of new nodes and connections

2. **Improved Error Handling**
   - Built-in retry mechanisms through graph cycles
   - Clear error propagation paths
   - Ability to implement fallback strategies

3. **Better State Management**
   - Clear visualization of workflow state
   - Predictable data flow between agents
   - Easy tracking of progress and dependencies

4. **Scalability**
   - Simple addition of new agents or processing steps
   - Modular design for easy maintenance
   - Support for complex multi-agent interactions

5. **Monitoring and Debugging**
   - Clear visualization of the entire workflow
   - Easy identification of bottlenecks
   - Simple tracking of agent interactions

### Node Types

- **Planner Node**: Creates a structured plan for the Terraform module
- **Generator Node**: Generates Terraform files based on the plan
- **Reviewer Node**: Reviews and validates the generated files

### Edge Conditions

- Planner → Generator: Direct flow of module plan
- Generator → Reviewer: Direct flow of generated files
- Reviewer → Generator: Conditional flow based on review results
- Reviewer → End: Conditional flow when review is approved

## Components

### 1. Planner Agent
- Analyzes requirements
- Creates a structured plan including:
  - Module name
  - Required resources
  - Variables
  - Outputs

### 2. Generator Agent
- Creates four essential Terraform files:
  - `main.tf`: Resource definitions
  - `variables.tf`: Variable definitions
  - `outputs.tf`: Output definitions
  - `versions.tf`: Provider and version configurations

### 3. Reviewer Agent
- Validates Terraform configurations
- Checks for:
  - Syntax correctness
  - Best practices
  - Security considerations
  - Documentation quality
  - Resource naming conventions

## Usage

1. Set the required environment variables:
   ```bash
   export OPENAI_API_KEY=your_api_key
   ```

2. Run the example:
   ```bash
   go run main.go
   ```

## Retry Mechanism

The workflow includes a retry mechanism that:
- Allows up to 10 retries for file generation
- Returns to the generator if the review is not approved
- Ends the workflow if maximum retries are reached

## Output Types

### TerraformPlan
```go
type TerraformPlan struct {
    ModuleName string   `json:"module_name"`
    Resources  []string `json:"resources"`
    Variables  []string `json:"variables"`
    Outputs    []string `json:"outputs"`
}
```

### FileGenerationPlan
```go
type FileGenerationPlan struct {
    Files []FileSpec `json:"files"`
}
```

### ReviewResult
```go
type ReviewResult struct {
    Approved          bool     `json:"approved"`
    Comments          []string `json:"comments"`
    Suggestions       []string `json:"suggestions"`
    ValidationResults string   `json:"validation_results"`
}
```

## Tools

The workflow uses several tools:
- `FileWriterTool`: Writes generated files to disk
- `TerraformValidateTool`: Validates Terraform configurations

## Best Practices

1. Always validate generated Terraform configurations
2. Follow security best practices in resource definitions
3. Include proper documentation and comments
4. Use consistent naming conventions
5. Implement proper error handling and retry mechanisms
