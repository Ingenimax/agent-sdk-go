package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
)

// MovieReview represents a structured movie review
type MovieReview struct {
	Title       string   `json:"title" description:"The title of the movie"`
	Director    string   `json:"director" description:"The director of the movie"`
	Year        int      `json:"year" description:"The year the movie was released"`
	Genre       []string `json:"genre" description:"List of genres for the movie"`
	Rating      float64  `json:"rating" description:"Rating out of 10"`
	Summary     string   `json:"summary" description:"A brief summary of the movie plot"`
	Strengths   []string `json:"strengths" description:"Key strengths or positive aspects"`
	Weaknesses  []string `json:"weaknesses" description:"Key weaknesses or areas for improvement"`
	Recommended bool     `json:"recommended" description:"Whether the movie is recommended"`
}

// WeatherReport represents structured weather information
type WeatherReport struct {
	Location    string  `json:"location" description:"The location for the weather report"`
	Temperature float64 `json:"temperature" description:"Temperature in Celsius"`
	Condition   string  `json:"condition" description:"Weather condition (e.g., sunny, cloudy, rainy)"`
	Humidity    int     `json:"humidity" description:"Humidity percentage"`
	WindSpeed   float64 `json:"wind_speed" description:"Wind speed in km/h"`
	Forecast    string  `json:"forecast" description:"Brief forecast for the next few days"`
}

// ProductAnalysis represents a structured product analysis
type ProductAnalysis struct {
	ProductName  string                 `json:"product_name" description:"Name of the product"`
	Category     string                 `json:"category" description:"Product category"`
	Price        float64                `json:"price" description:"Price in USD"`
	Pros         []string               `json:"pros" description:"List of advantages"`
	Cons         []string               `json:"cons" description:"List of disadvantages"`
	Features     map[string]interface{} `json:"features" description:"Key features and specifications"`
	TargetMarket string                 `json:"target_market" description:"Target market for the product"`
	Rating       int                    `json:"rating" description:"Overall rating from 1 to 5"`
}

func main() {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("ANTHROPIC_API_KEY environment variable is required")
		fmt.Println("Please set it with: export ANTHROPIC_API_KEY=your_api_key_here")
		os.Exit(1)
	}

	// Create context
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "example-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "structured-output-demo")

	fmt.Println("Anthropic Structured Output Examples")
	fmt.Println("====================================")
	fmt.Println()

	// Example 1: Movie Review
	fmt.Println("Example 1: Movie Review Structure")
	fmt.Println("---------------------------------")
	movieReviewExample(ctx, apiKey)
	fmt.Println()

	// Example 2: Weather Report
	fmt.Println("Example 2: Weather Report Structure")
	fmt.Println("-----------------------------------")
	weatherReportExample(ctx, apiKey)
	fmt.Println()

	// Example 3: Product Analysis
	fmt.Println("Example 3: Product Analysis Structure")
	fmt.Println("-------------------------------------")
	productAnalysisExample(ctx, apiKey)
	fmt.Println()

	// Example 4: Using with Agent
	fmt.Println("Example 4: Using Structured Output with Agent")
	fmt.Println("---------------------------------------------")
	agentStructuredOutputExample(ctx, apiKey)
}

func movieReviewExample(ctx context.Context, apiKey string) {
	// Create Anthropic client
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(anthropic.ClaudeSonnet4),
	)

	// Create response format for movie review
	responseFormat := structuredoutput.NewResponseFormat(MovieReview{})

	// Generate structured movie review
	response, err := client.Generate(
		ctx,
		"Review the movie 'Inception' (2010) directed by Christopher Nolan",
		anthropic.WithResponseFormat(*responseFormat),
		anthropic.WithSystemMessage("You are a professional movie critic. Provide detailed and insightful reviews."),
		anthropic.WithTemperature(0.3),
	)
	if err != nil {
		fmt.Printf("Error generating movie review: %v\n", err)
		return
	}

	// Parse the JSON response
	var review MovieReview
	if err := json.Unmarshal([]byte(response), &review); err != nil {
		fmt.Printf("Error parsing movie review: %v\n", err)
		return
	}

	// Display the structured review
	fmt.Printf("Movie: %s (%d)\n", review.Title, review.Year)
	fmt.Printf("Director: %s\n", review.Director)
	fmt.Printf("Genres: %v\n", review.Genre)
	fmt.Printf("Rating: %.1f/10\n", review.Rating)
	fmt.Printf("Summary: %s\n", review.Summary)
	fmt.Printf("Strengths: %v\n", review.Strengths)
	fmt.Printf("Weaknesses: %v\n", review.Weaknesses)
	fmt.Printf("Recommended: %v\n", review.Recommended)
}

func weatherReportExample(ctx context.Context, apiKey string) {
	// Create Anthropic client
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(anthropic.Claude35Haiku),
	)

	// Create response format for weather report
	responseFormat := structuredoutput.NewResponseFormat(WeatherReport{})

	// Generate structured weather report
	response, err := client.Generate(
		ctx,
		"Provide a weather report for San Francisco, California for today",
		anthropic.WithResponseFormat(*responseFormat),
		anthropic.WithSystemMessage("You are a weather forecaster. Provide realistic weather information based on typical patterns for the location and season."),
		anthropic.WithTemperature(0.5),
	)
	if err != nil {
		fmt.Printf("Error generating weather report: %v\n", err)
		return
	}

	// Parse the JSON response
	var weather WeatherReport
	if err := json.Unmarshal([]byte(response), &weather); err != nil {
		fmt.Printf("Error parsing weather report: %v\n", err)
		return
	}

	// Display the weather report
	fmt.Printf("Location: %s\n", weather.Location)
	fmt.Printf("Temperature: %.1f°C\n", weather.Temperature)
	fmt.Printf("Condition: %s\n", weather.Condition)
	fmt.Printf("Humidity: %d%%\n", weather.Humidity)
	fmt.Printf("Wind Speed: %.1f km/h\n", weather.WindSpeed)
	fmt.Printf("Forecast: %s\n", weather.Forecast)
}

func productAnalysisExample(ctx context.Context, apiKey string) {
	// Create Anthropic client
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(anthropic.ClaudeSonnet4),
	)

	// Create response format for product analysis
	responseFormat := structuredoutput.NewResponseFormat(ProductAnalysis{})

	// Generate structured product analysis
	response, err := client.Generate(
		ctx,
		"Analyze the Apple AirPods Pro (2nd generation) as a product",
		anthropic.WithResponseFormat(*responseFormat),
		anthropic.WithSystemMessage("You are a product analyst. Provide comprehensive and balanced product analyses."),
		anthropic.WithTemperature(0.4),
	)
	if err != nil {
		fmt.Printf("Error generating product analysis: %v\n", err)
		return
	}

	// Parse the JSON response
	var analysis ProductAnalysis
	if err := json.Unmarshal([]byte(response), &analysis); err != nil {
		fmt.Printf("Error parsing product analysis: %v\n", err)
		return
	}

	// Display the product analysis
	fmt.Printf("Product: %s\n", analysis.ProductName)
	fmt.Printf("Category: %s\n", analysis.Category)
	fmt.Printf("Price: $%.2f\n", analysis.Price)
	fmt.Printf("Rating: %d/5\n", analysis.Rating)
	fmt.Printf("Target Market: %s\n", analysis.TargetMarket)
	fmt.Printf("Pros: %v\n", analysis.Pros)
	fmt.Printf("Cons: %v\n", analysis.Cons)
	fmt.Printf("Features:\n")
	for key, value := range analysis.Features {
		fmt.Printf("  - %s: %v\n", key, value)
	}
}

func agentStructuredOutputExample(ctx context.Context, apiKey string) {
	// Create Anthropic client
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(anthropic.ClaudeSonnet4),
	)

	// Create response format for movie review (reusing the struct)
	responseFormat := structuredoutput.NewResponseFormat(MovieReview{})

	// Create an agent with structured output
	agentInstance, err := agent.NewAgent(
		agent.WithLLM(client),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are a movie expert who provides detailed movie reviews in a structured format."),
		agent.WithResponseFormat(*responseFormat),
	)
	if err != nil {
		fmt.Printf("Failed to create agent: %v\n", err)
		return
	}

	// Run the agent with a movie review request
	response, err := agentInstance.Run(ctx, "Review the movie 'The Matrix' (1999) by the Wachowskis")
	if err != nil {
		fmt.Printf("Failed to run agent: %v\n", err)
		return
	}

	// Parse the JSON response
	var review MovieReview
	if err := json.Unmarshal([]byte(response), &review); err != nil {
		fmt.Printf("Error parsing agent response: %v\n", err)
		return
	}

	// Display the structured review from agent
	fmt.Printf("Movie: %s (%d)\n", review.Title, review.Year)
	fmt.Printf("Directors: %s\n", review.Director)
	fmt.Printf("Genres: %v\n", review.Genre)
	fmt.Printf("Rating: %.1f/10\n", review.Rating)
	fmt.Printf("Summary: %s\n", review.Summary)
	fmt.Printf("Recommended: %v\n", review.Recommended)
}
