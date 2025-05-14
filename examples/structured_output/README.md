# Structured Output Example

This example demonstrates how to use the Agent SDK's structured output feature, which allows you to receive responses in predefined structured formats (like JSON) that match your Go structs.

## Features

- Structured JSON responses mapped to Go structs
- JSON schema generation from struct definitions
- Support for field descriptions via struct tags
- Optional fields handling with `omitempty` tag
- Type validation through JSON schema

## Usage

### Prerequisites

Set your OpenAI API key in the environment:
```bash
export OPENAI_API_KEY=your_openai_api_key
```

### Defining a Structured Response

Define your expected response structure using a Go struct with JSON tags:

```go
type Person struct {
    Name        string `json:"name" description:"The person's full name"`
    Profession  string `json:"profession" description:"Their primary occupation"`
    Description string `json:"description" description:"A brief biography"`
    BirthDate   string `json:"birth_date,omitempty" description:"Date of birth"`
}
```

### Creating an Agent with Structured Output

```go
// Create the response format from your struct
responseFormat := structuredoutput.NewResponseFormat(Person{})

// Create the agent with the response format
agent, err := agent.NewAgent(
    agent.WithLLM(openaiClient),
    agent.WithResponseFormat(*responseFormat),
    // ... other options ...
)
```

### Getting Structured Responses

```go
var person Person
response, err := agent.Run(ctx, "Tell me about Albert Einstein")
if err != nil {
    log.Fatal(err)
}

err = json.Unmarshal([]byte(response), &person)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Name: %s\nProfession: %s\n", person.Name, person.Profession)
```

## How It Works

1. The SDK generates a JSON schema from your struct definition
2. This schema is passed to the LLM as part of the response format
3. The LLM formats its response to match the schema exactly
4. The response can be directly unmarshaled into your struct

## Customization

You can customize the structured output by:
- Adding new fields to your struct
- Using `description` tags to guide the LLM
- Making fields optional with `omitempty`
- Creating different structs for different types of responses

## Running the Example

```bash
go run main.go
```

The example will query information about various people and display the structured responses.