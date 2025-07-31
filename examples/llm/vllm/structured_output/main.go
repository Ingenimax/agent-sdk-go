package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm/vllm"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
)

// Person represents a person with biographical information
type Person struct {
	Name        string    `json:"name" description:"The person's full name"`
	Profession  string    `json:"profession" description:"Their primary occupation"`
	Description string    `json:"description" description:"A brief biography"`
	BirthDate   string    `json:"birth_date,omitempty" description:"Date of birth"`
	Companies   []Company `json:"companies,omitempty" description:"Companies they've worked for"`
}

// Company represents a company
type Company struct {
	Name        string `json:"name" description:"Company name"`
	Country     string `json:"country" description:"Country where company is headquartered"`
	Description string `json:"description" description:"Brief description of the company"`
}

// WeatherInfo represents weather information
type WeatherInfo struct {
	Location    string  `json:"location" description:"City or location name"`
	Temperature float64 `json:"temperature" description:"Temperature in Celsius"`
	Humidity    int     `json:"humidity" description:"Humidity percentage"`
	Condition   string  `json:"condition" description:"Weather condition (sunny, rainy, etc.)"`
	WindSpeed   float64 `json:"wind_speed,omitempty" description:"Wind speed in km/h"`
	Pressure    float64 `json:"pressure,omitempty" description:"Atmospheric pressure in hPa"`
}

// ProgrammingTask represents a programming task with analysis
type ProgrammingTask struct {
	TaskName      string   `json:"task_name" description:"Name of the programming task"`
	Difficulty    string   `json:"difficulty" description:"Difficulty level (easy, medium, hard)"`
	EstimatedTime string   `json:"estimated_time" description:"Estimated time to complete"`
	Technologies  []string `json:"technologies" description:"Required technologies or languages"`
	KeyConcepts   []string `json:"key_concepts" description:"Key programming concepts involved"`
	CodeExample   string   `json:"code_example,omitempty" description:"Brief code example"`
	Explanation   string   `json:"explanation" description:"Explanation of the task"`
	BestPractices []string `json:"best_practices,omitempty" description:"Best practices for this task"`
}

func main() {
	// Create a logger
	logger := logging.New()
	ctx := context.Background()

	// Get vLLM base URL from environment or use default
	baseURL := os.Getenv("VLLM_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}

	// Get model from environment or use default
	model := os.Getenv("VLLM_MODEL")
	if model == "" {
		model = "llama-2-7b"
	}

	// Create vLLM client
	client := vllm.NewClient(
		vllm.WithModel(model),
		vllm.WithLogger(logger),
		vllm.WithBaseURL(baseURL),
	)

	fmt.Println("=== vLLM Structured Output Examples ===")

	// Example 1: Person Information
	fmt.Println("1. Person Information Example")
	personFormat := structuredoutput.NewResponseFormat(Person{})

	response, err := client.Generate(
		ctx,
		"Tell me about Albert Einstein",
		vllm.WithResponseFormat(*personFormat),
		vllm.WithTemperature(0.3),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate person info", map[string]interface{}{"error": err.Error()})
	} else {
		var person Person
		if err := json.Unmarshal([]byte(response), &person); err != nil {
			logger.Error(ctx, "Failed to unmarshal person", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Name: %s\n", person.Name)
			fmt.Printf("Profession: %s\n", person.Profession)
			fmt.Printf("Description: %s\n", person.Description)
			if person.BirthDate != "" {
				fmt.Printf("Birth Date: %s\n", person.BirthDate)
			}
			if len(person.Companies) > 0 {
				fmt.Println("Companies:")
				for _, company := range person.Companies {
					fmt.Printf("  - %s (%s): %s\n", company.Name, company.Country, company.Description)
				}
			}
		}
	}
	fmt.Println()

	// Example 2: Weather Information
	fmt.Println("2. Weather Information Example")
	weatherFormat := structuredoutput.NewResponseFormat(WeatherInfo{})

	response, err = client.Generate(
		ctx,
		"Describe the weather in Tokyo during spring",
		vllm.WithResponseFormat(*weatherFormat),
		vllm.WithTemperature(0.5),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate weather info", map[string]interface{}{"error": err.Error()})
	} else {
		var weather WeatherInfo
		if err := json.Unmarshal([]byte(response), &weather); err != nil {
			logger.Error(ctx, "Failed to unmarshal weather", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Location: %s\n", weather.Location)
			fmt.Printf("Temperature: %.1f°C\n", weather.Temperature)
			fmt.Printf("Humidity: %d%%\n", weather.Humidity)
			fmt.Printf("Condition: %s\n", weather.Condition)
			if weather.WindSpeed > 0 {
				fmt.Printf("Wind Speed: %.1f km/h\n", weather.WindSpeed)
			}
			if weather.Pressure > 0 {
				fmt.Printf("Pressure: %.1f hPa\n", weather.Pressure)
			}
		}
	}
	fmt.Println()

	// Example 3: Programming Task Analysis
	fmt.Println("3. Programming Task Analysis Example")
	taskFormat := structuredoutput.NewResponseFormat(ProgrammingTask{})

	response, err = client.Generate(
		ctx,
		"Analyze the task of implementing a binary search tree in Go",
		vllm.WithResponseFormat(*taskFormat),
		vllm.WithTemperature(0.4),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate task analysis", map[string]interface{}{"error": err.Error()})
	} else {
		var task ProgrammingTask
		if err := json.Unmarshal([]byte(response), &task); err != nil {
			logger.Error(ctx, "Failed to unmarshal task", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Task: %s\n", task.TaskName)
			fmt.Printf("Difficulty: %s\n", task.Difficulty)
			fmt.Printf("Estimated Time: %s\n", task.EstimatedTime)
			fmt.Printf("Technologies: %v\n", task.Technologies)
			fmt.Printf("Key Concepts: %v\n", task.KeyConcepts)
			fmt.Printf("Explanation: %s\n", task.Explanation)
			if len(task.BestPractices) > 0 {
				fmt.Println("Best Practices:")
				for _, practice := range task.BestPractices {
					fmt.Printf("  - %s\n", practice)
				}
			}
			if task.CodeExample != "" {
				fmt.Printf("Code Example:\n%s\n", task.CodeExample)
			}
		}
	}
	fmt.Println()

	// Example 4: Agent Integration with Structured Output
	fmt.Println("4. Agent Integration Example")

	// Create a simple agent-like prompt with structured output
	agentPrompt := `You are a helpful assistant that provides structured information about programming languages.

Please analyze the Go programming language and provide information in the requested format.`

	response, err = client.Generate(
		ctx,
		agentPrompt,
		vllm.WithResponseFormat(*personFormat), // Reusing Person format for language analysis
		vllm.WithTemperature(0.3),
		vllm.WithSystemMessage("You are a programming language expert who provides detailed analysis."),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate agent response", map[string]interface{}{"error": err.Error()})
	} else {
		var languageInfo Person
		if err := json.Unmarshal([]byte(response), &languageInfo); err != nil {
			logger.Error(ctx, "Failed to unmarshal language info", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Language Name: %s\n", languageInfo.Name)
			fmt.Printf("Type: %s\n", languageInfo.Profession)
			fmt.Printf("Description: %s\n", languageInfo.Description)
		}
	}
	fmt.Println()

	// Example 5: Error Handling with Invalid JSON
	fmt.Println("5. Error Handling Example")

	// Try to parse a response that might not be valid JSON
	response, err = client.Generate(
		ctx,
		"Write a simple story about a cat",
		vllm.WithTemperature(0.8),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate story", map[string]interface{}{"error": err.Error()})
	} else {
		// Try to parse as JSON even though it's not structured
		var person Person
		if err := json.Unmarshal([]byte(response), &person); err != nil {
			fmt.Printf("Expected: Story is not in JSON format\n")
			fmt.Printf("Actual: %s\n", response)
		} else {
			fmt.Printf("Unexpected: Story was parsed as JSON\n")
		}
	}
	fmt.Println()

	// Example 6: Complex Nested Structure
	fmt.Println("6. Complex Nested Structure Example")

	// Create a more complex structure
	type Project struct {
		Name         string   `json:"name" description:"Project name"`
		Description  string   `json:"description" description:"Project description"`
		Team         []Person `json:"team" description:"Team members"`
		Technologies []string `json:"technologies" description:"Technologies used"`
		Status       string   `json:"status" description:"Project status"`
	}

	projectFormat := structuredoutput.NewResponseFormat(Project{})

	response, err = client.Generate(
		ctx,
		"Describe a software project for building a web application",
		vllm.WithResponseFormat(*projectFormat),
		vllm.WithTemperature(0.4),
	)
	if err != nil {
		logger.Error(ctx, "Failed to generate project info", map[string]interface{}{"error": err.Error()})
	} else {
		var project Project
		if err := json.Unmarshal([]byte(response), &project); err != nil {
			logger.Error(ctx, "Failed to unmarshal project", map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("Project: %s\n", project.Name)
			fmt.Printf("Description: %s\n", project.Description)
			fmt.Printf("Status: %s\n", project.Status)
			fmt.Printf("Technologies: %v\n", project.Technologies)
			if len(project.Team) > 0 {
				fmt.Println("Team:")
				for _, member := range project.Team {
					fmt.Printf("  - %s (%s)\n", member.Name, member.Profession)
				}
			}
		}
	}
	fmt.Println()

	fmt.Println("=== vLLM Structured Output Examples Completed ===")
}
