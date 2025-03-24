package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
)

type Person struct {
	Name        string `json:"name" description:"The person's full name"`
	Profession  string `json:"profession" description:"Their primary occupation"`
	Description string `json:"description" description:"A brief biography"`
	BirthDate   string `json:"birth_date,omitempty" description:"Date of birth in DD/MM/YYYY format"`
	Nationality string `json:"nationality,omitempty" description:"The person's nationality"`
	BirthPlace  string `json:"birth_place,omitempty" description:"The person's birth place"`
	DeathDate   string `json:"death_date,omitempty" description:"Date of death in YYYY-MM-DD format"`
	DeathPlace  string `json:"death_place,omitempty" description:"Place of death"`
}

func main() {
	// Create a logger
	logger := logging.New()

	// Get configuration
	cfg := config.Get()

	// Create an OpenAI client with JSON response format
	openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey,
		openai.WithLogger(logger),
		openai.WithModel("gpt-4o-mini"), // Using a model that supports JSON response format
	)

	responseFormat := structuredoutput.NewResponseFormat(Person{})

	// Create a new agent with JSON response format
	agent, err := agent.NewAgent(
		agent.WithLLM(openaiClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt(`You are an AI assistent that provides information about a person.`),
		agent.WithName("StructuredResponseAgent"),
		// Set the response format to JSON
		agent.WithResponseFormat(*responseFormat),
	)
	if err != nil {
		logger.Error(context.Background(), "Failed to create agent", map[string]interface{}{"error": err.Error()})
		return
	}

	// Create a context with organization ID
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "structured-response-demo")

	// Example queries that should return structured JSON
	queries := []string{
		"Tell me about Albert Einstein",
		"Tell me about Steve Jobs",
		"Tell me about Andrew Ng",
	}

	// Run the agent for each query
	for _, query := range queries {
		fmt.Printf("\n\nQuery: %s\n", query)
		fmt.Println("----------------------------------------")

		response, err := agent.Run(ctx, query)
		if err != nil {
			logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
			continue
		}

		// unmarshal the response
		var responseType Person
		err = json.Unmarshal([]byte(response), &responseType)
		if err != nil {
			logger.Error(ctx, "Failed to unmarshal response", map[string]interface{}{"error": err.Error()})
			continue
		}

		fmt.Printf("Name: %s\n", responseType.Name)
		fmt.Printf("Profession: %s\n", responseType.Profession)
		fmt.Printf("Description: %s\n", responseType.Description)
		fmt.Printf("BirthDate: %s\n", responseType.BirthDate)
		fmt.Printf("BirthPlace: %s\n", responseType.BirthPlace)
		fmt.Printf("DeathDate: %s\n", responseType.DeathDate)
		fmt.Printf("DeathPlace: %s\n", responseType.DeathPlace)
		fmt.Printf("Nationality: %s\n", responseType.Nationality)
	}
}
