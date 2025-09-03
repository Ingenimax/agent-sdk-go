# Ollama Structured Output Examples

This example demonstrates how to use structured output with Ollama, showing both the manual schema approach and the automatic struct-based approach.

## Features

- **Automatic Schema Generation**: Use `structuredoutput.NewResponseFormat()` to automatically generate JSON schemas from Go structs
- **Type Safety**: Direct unmarshaling into Go structs
- **Field Descriptions**: Use struct tags to provide field descriptions
- **Required/Optional Fields**: Use `omitempty` tag for optional fields
- **Nested Structures**: Support for complex nested objects and arrays

## Usage

### Prerequisites

1. **Install Ollama**: Follow the installation instructions at [ollama.ai](https://ollama.ai)
2. **Start Ollama Server**: Run `ollama serve` to start the server
3. **Pull a Model**: Run `ollama pull qwen3:0.6b` to download a model

### Running the Example

```bash
# Run the structured output examples
go run examples/llm/ollama/structured_output/main.go
```

## Approaches

### 1. Automatic Schema Generation (Recommended)

This approach uses the `structuredoutput` package to automatically generate JSON schemas from Go structs:

```go
// Define your response structure
type Person struct {
    Name        string   `json:"name" description:"The person's full name"`
    Profession  string   `json:"profession" description:"Their primary occupation"`
    Description string   `json:"description" description:"A brief biography"`
    BirthDate   string   `json:"birth_date,omitempty" description:"Date of birth"`
    Companies   []Company `json:"companies,omitempty" description:"Companies they've worked for"`
}

type Company struct {
    Name        string `json:"name" description:"Company name"`
    Country     string `json:"country" description:"Country where company is headquartered"`
    Description string `json:"description" description:"Brief description of the company"`
}

// Create response format automatically
personFormat := structuredoutput.NewResponseFormat(Person{})

// Generate structured response
response, err := ollamaClient.Generate(
    ctx,
    "Tell me about Albert Einstein",
    ollama.WithResponseFormat(*personFormat),
)

// Unmarshal directly into struct
var person Person
json.Unmarshal([]byte(response), &person)
```

### 2. Manual Schema Definition (Legacy)

This approach manually defines the JSON schema:

```go
// Define your response structure
type Person struct {
    Name        string `json:"name"`
    Profession  string `json:"profession"`
    Description string `json:"description"`
}

// Manually create JSON schema
personFormat := interfaces.ResponseFormat{
    Type:   interfaces.ResponseFormatJSON,
    Name:   "Person",
    Schema: interfaces.JSONSchema{
        "type": "object",
        "properties": map[string]interface{}{
            "name": map[string]interface{}{
                "type":        "string",
                "description": "The person's full name",
            },
            "profession": map[string]interface{}{
                "type":        "string",
                "description": "Their primary occupation",
            },
            "description": map[string]interface{}{
                "type":        "string",
                "description": "A brief biography",
            },
        },
        "required": []string{"name", "profession", "description"},
    },
}
```

## Examples Included

1. **Person Information**: Extract biographical data with nested company information
2. **Weather Information**: Get structured weather data with temperature, humidity, etc.
3. **Programming Tasks**: Generate code with explanations and complexity analysis
4. **Agent Integration**: Use structured output with the agent framework

## Benefits of Automatic Schema Generation

- **Less Code**: No need to manually define JSON schemas
- **Type Safety**: Compile-time validation of struct definitions
- **Maintainability**: Changes to structs automatically update schemas
- **Consistency**: Same approach used across all LLM providers
- **Documentation**: Field descriptions from struct tags

## Struct Tag Features

- `json:"field_name"`: JSON field name
- `description:"field description"`: Field description for the LLM
- `omitempty`: Makes the field optional
- Nested structs and arrays are automatically handled

## Running with Different Models

```bash
# Use a specific model
export OLLAMA_MODEL=qwen3:0.6b
go run examples/llm/ollama/structured_output/main.go

# Use a different model
export OLLAMA_MODEL=llama2
go run examples/llm/ollama/structured_output/main.go
```
