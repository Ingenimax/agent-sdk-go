package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/ollama"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
)

// Person represents a structured response for biographical information
type Person struct {
	Name        string    `json:"name" description:"The person's full name"`
	Profession  string    `json:"profession" description:"Their primary occupation"`
	Description string    `json:"description" description:"A brief biography"`
	BirthDate   string    `json:"birth_date,omitempty" description:"Date of birth"`
	DeathDate   string    `json:"death_date,omitempty" description:"Date of death if applicable"`
	Companies   []Company `json:"companies,omitempty" description:"Companies they've worked for"`
	Hobbies     []string  `json:"hobbies,omitempty" description:"Their hobbies and interests"`
}

// Company represents a company where a person has worked
type Company struct {
	Name        string `json:"name" description:"Company name"`
	Country     string `json:"country" description:"Country where company is headquartered"`
	Description string `json:"description" description:"Brief description of the company"`
}

// WeatherInfo represents structured weather information
type WeatherInfo struct {
	Location    string  `json:"location" description:"City and country"`
	Temperature float64 `json:"temperature" description:"Temperature in Celsius"`
	Condition   string  `json:"condition" description:"Weather condition (sunny, rainy, etc.)"`
	Humidity    int     `json:"humidity" description:"Humidity percentage"`
	WindSpeed   float64 `json:"wind_speed" description:"Wind speed in km/h"`
}

// ProgrammingTask represents a structured programming task response
type ProgrammingTask struct {
	Language    string   `json:"language" description:"Programming language"`
	Task        string   `json:"task" description:"Description of the task"`
	Code        string   `json:"code" description:"The actual code implementation"`
	Explanation string   `json:"explanation" description:"Explanation of the code"`
	Complexity  string   `json:"complexity" description:"Time complexity (O(n), O(1), etc.)"`
	Tags        []string `json:"tags" description:"Programming concepts used"`
}

func main() {
	// Create a logger
	logger := logging.New()
	ctx := context.Background()

	// Get Ollama configuration from environment
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen3:0.6b"
	}

	// Create Ollama LLM client
	ollamaClient := ollama.NewClient(
		ollama.WithModel(model),
		ollama.WithLogger(logger),
		ollama.WithBaseURL(baseURL),
		ollama.WithRetry(
			retry.WithMaxAttempts(3),
			retry.WithInitialInterval(1),
			retry.WithBackoffCoefficient(2.0),
			retry.WithMaximumInterval(30),
		),
	)

	// Example 1: Person Information
	fmt.Println("1. Person Information Example")
	personFormat := structuredoutput.NewResponseFormat(Person{})

	personResponse, err := ollamaClient.Generate(
		ctx,
		"Tell me about Albert Einstein",
		ollama.WithResponseFormat(*personFormat),
		ollama.WithTemperature(0.7),
		ollama.WithSystemMessage("You are a helpful assistant that provides accurate biographical information."),
	)

	if err != nil {
		logger.Error(ctx, "Failed to generate person info", map[string]interface{}{"error": err.Error()})
	} else {
		var person Person
		if err := json.Unmarshal([]byte(personResponse), &person); err != nil {
			fmt.Printf("Failed to unmarshal person response: %v\n", err)
		} else {
			fmt.Printf("Name: %s\nProfession: %s\nDescription: %s\n", person.Name, person.Profession, person.Description)
			if len(person.Companies) > 0 {
				fmt.Printf("Companies: %d companies listed\n", len(person.Companies))
			}
			if len(person.Hobbies) > 0 {
				fmt.Printf("Hobbies: %v\n", person.Hobbies)
			}
		}
		fmt.Println()
	}

	// Example 2: Weather Information
	fmt.Println("2. Weather Information Example")
	weatherFormat := structuredoutput.NewResponseFormat(WeatherInfo{})

	weatherResponse, err := ollamaClient.Generate(
		ctx,
		"Provide current weather information for Tokyo, Japan",
		ollama.WithResponseFormat(*weatherFormat),
		ollama.WithTemperature(0.5),
		ollama.WithSystemMessage("You are a weather assistant. Provide realistic weather data."),
	)

	if err != nil {
		logger.Error(ctx, "Failed to generate weather info", map[string]interface{}{"error": err.Error()})
	} else {
		var weather WeatherInfo
		if err := json.Unmarshal([]byte(weatherResponse), &weather); err != nil {
			fmt.Printf("Failed to unmarshal weather response: %v\n", err)
		} else {
			fmt.Printf("Location: %s\nTemperature: %.1f°C\nCondition: %s\nHumidity: %d%%\nWind Speed: %.1f km/h\n",
				weather.Location, weather.Temperature, weather.Condition, weather.Humidity, weather.WindSpeed)
		}
		fmt.Println()
	}

	// Example 3: Programming Task
	fmt.Println("3. Programming Task Example")
	programmingFormat := structuredoutput.NewResponseFormat(ProgrammingTask{})

	programmingResponse, err := ollamaClient.Generate(
		ctx,
		"Write a function to calculate the factorial of a number in Go",
		ollama.WithResponseFormat(*programmingFormat),
		ollama.WithTemperature(0.3),
		ollama.WithSystemMessage("You are a programming expert. Provide clear, well-documented code."),
	)

	if err != nil {
		logger.Error(ctx, "Failed to generate programming task", map[string]interface{}{"error": err.Error()})
	} else {
		var task ProgrammingTask
		if err := json.Unmarshal([]byte(programmingResponse), &task); err != nil {
			fmt.Printf("Failed to unmarshal programming response: %v\n", err)
		} else {
			fmt.Printf("Language: %s\nTask: %s\nComplexity: %s\nTags: %v\n",
				task.Language, task.Task, task.Complexity, task.Tags)
			fmt.Printf("Code:\n%s\n", task.Code)
			fmt.Printf("Explanation: %s\n", task.Explanation)
		}
		fmt.Println()
	}

	// Example 4: Agent with Structured Output
	fmt.Println("4. Agent with Structured Output Example")

	// Create an agent with structured output
	agent, err := agent.NewAgent(
		agent.WithLLM(ollamaClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a helpful AI assistant that provides structured information."),
		agent.WithName("StructuredOllamaAgent"),
		agent.WithResponseFormat(*personFormat), // Use person format for the agent
	)
	if err != nil {
		logger.Error(ctx, "Failed to create agent", map[string]interface{}{"error": err.Error()})
		return
	}

	// Create context with organization and conversation IDs
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "structured-ollama-demo")

	// Run agent with structured output
	agentResponse, err := agent.Run(ctx, "Tell me about Marie Curie")
	if err != nil {
		logger.Error(ctx, "Agent run failed", map[string]interface{}{"error": err.Error()})
	} else {
		var agentPerson Person
		if err := json.Unmarshal([]byte(agentResponse), &agentPerson); err != nil {
			fmt.Printf("Failed to unmarshal agent response: %v\n", err)
		} else {
			fmt.Printf("Agent Response - Name: %s\nProfession: %s\nDescription: %s\n",
				agentPerson.Name, agentPerson.Profession, agentPerson.Description)
		}
	}

	fmt.Println("\n=== Structured Output Examples Completed ===")
}
