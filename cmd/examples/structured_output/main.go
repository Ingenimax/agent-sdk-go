package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
)

type BirthData struct {
	BirthDate  string `json:"birth_date,omitempty" description:"Date of birth in DD/MM/YYYY format"`
	BirthPlace string `json:"birth_place,omitempty" description:"The person's birth place"`
}

type Company struct {
	Name        string `json:"name,omitempty" description:"The name of the company"`
	Country     string `json:"country,omitempty" description:"The country where the company is headquartered"`
	Description string `json:"description,omitempty" description:"A brief description of the company"`
}

type WorkInformation struct {
	Companies []Company `json:"companies,omitempty" description:"The list of companies the person has worked for"`
	Positions []string  `json:"positions,omitempty" description:"The list of positions the person has held"`
	Countries []string  `json:"countries,omitempty" description:"The list of countries the person has worked in"`
}

type Person struct {
	Name                   string          `json:"name" description:"The person's full name"`
	Profession             string          `json:"profession" description:"Their primary occupation"`
	Description            string          `json:"description" description:"A brief biography"`
	Nationality            string          `json:"nationality,omitempty" description:"The person's nationality"`
	DeathDate              string          `json:"death_date,omitempty" description:"Date of death in YYYY-MM-DD format"`
	DeathPlace             string          `json:"death_place,omitempty" description:"Place of death"`
	BirthData              BirthData       `json:"birth_data,omitempty" description:"The person's birth data"`
	WorkInfo               WorkInformation `json:"work_info,omitempty" description:"The person's work information"`
	PersonInformationFound bool            `json:"person_information_found,omitempty" description:"Whether the person's information was found"`
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
		agent.WithSystemPrompt(`
			You are an AI assistant that provides accurate biographical information about people.
            
            Guidelines:
            - Provide factual, verifiable information only
            - If a field's information is unknown, use null instead of making assumptions
            - For living persons, leave death-related fields as null
            - Keep descriptions concise but informative
            - Focus on the person's most significant achievements and contributions
			- Provide the list of companies the person has worked for, with the country where the company is headquartered and a description of the company.
            
            If the person is not a real historical or contemporary figure, or if you're unsure about their existence, return all fields as null.
		`),
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
		"Tell me about a person that does not exist",
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

		if responseType.PersonInformationFound {
			// Print basic information
			fmt.Printf("\nBasic Information:\n")
			fmt.Printf("================\n")
			fmt.Printf("Name: %s\n", responseType.Name)
			fmt.Printf("Profession: %s\n", responseType.Profession)
			fmt.Printf("Nationality: %s\n", responseType.Nationality)

			// Print biographical details
			fmt.Printf("\nBiographical Details:\n")
			fmt.Printf("===================\n")
			fmt.Printf("Description: %s\n", responseType.Description)
			fmt.Printf("Birth: %s, %s\n", responseType.BirthData.BirthDate, responseType.BirthData.BirthPlace)
			if responseType.DeathDate != "" {
				fmt.Printf("Death: %s, %s\n", responseType.DeathDate, responseType.DeathPlace)
			}

			// Print work history
			fmt.Printf("\nWork History:\n")
			fmt.Printf("============\n")
			if len(responseType.WorkInfo.Positions) > 0 {
				fmt.Printf("Positions held: %s\n", strings.Join(responseType.WorkInfo.Positions, ", "))
			}

			// Print company details
			if len(responseType.WorkInfo.Companies) > 0 {
				fmt.Printf("\nCompany Details:\n")
				fmt.Printf("===============\n")
				for _, company := range responseType.WorkInfo.Companies {
					fmt.Printf("• %s (%s)\n", company.Name, company.Country)
					fmt.Printf("  %s\n\n", company.Description)
				}
			}
		} else {
			fmt.Printf("No information found for %s\n", query)
		}
	}
}
