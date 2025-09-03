package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
)

// Define structs that match our JSON schemas for type safety
type PersonAnalysis struct {
	Name         string        `json:"name"`
	Profession   string        `json:"profession"`
	Skills       []string      `json:"skills"`
	Experience   int           `json:"experience_years"`
	Achievements []Achievement `json:"achievements"`
	Summary      string        `json:"summary"`
}

type Achievement struct {
	Title       string `json:"title"`
	Year        int    `json:"year"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
}

type BusinessAnalysis struct {
	CompanyName   string        `json:"company_name"`
	Industry      string        `json:"industry"`
	Founded       int           `json:"founded"`
	Headquarters  string        `json:"headquarters"`
	Employees     EmployeeRange `json:"employees"`
	Revenue       RevenueInfo   `json:"revenue"`
	Products      []Product     `json:"products"`
	KeyMetrics    []string      `json:"key_metrics"`
	Strengths     []string      `json:"strengths"`
	Challenges    []string      `json:"challenges"`
	FutureOutlook string        `json:"future_outlook"`
}

type EmployeeRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type RevenueInfo struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Year     int     `json:"year"`
}

type Product struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	LaunchYear  int    `json:"launch_year"`
	Description string `json:"description"`
}

type RecipeAnalysis struct {
	Name            string       `json:"name"`
	Cuisine         string       `json:"cuisine"`
	Difficulty      string       `json:"difficulty"`
	PrepTime        int          `json:"prep_time_minutes"`
	CookTime        int          `json:"cook_time_minutes"`
	Servings        int          `json:"servings"`
	Ingredients     []Ingredient `json:"ingredients"`
	Instructions    []string     `json:"instructions"`
	NutritionalInfo []string     `json:"nutritional_info"`
	Tags            []string     `json:"tags"`
}

type Ingredient struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
	Notes    string  `json:"notes,omitempty"`
}

func main() {
	fmt.Println("📋 Gemini Structured Output Examples")
	fmt.Println("=====================================")
	fmt.Println()

	// Get API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY environment variable is required")
	}

	ctx := context.Background()

	// Create Gemini client
	client, err := gemini.NewClient(
		apiKey,
		gemini.WithModel(gemini.ModelGemini25Flash),
	)
	if err != nil {
		log.Fatalf("Failed to create Gemini client: %v", err)
	}

	// Example 1: Person Analysis
	fmt.Println("=== Example 1: Person Analysis ===")
	personFormat := structuredoutput.NewResponseFormat(PersonAnalysis{})

	personPrompt := `Analyze this person:
	Marie Curie was a Polish-French physicist and chemist who conducted pioneering research on radioactivity.
	She was the first woman to win a Nobel Prize, the first person to win a Nobel Prize twice, and the only
	person to win a Nobel Prize in two different sciences. She discovered two elements, polonium and radium.
	She founded the Radium Institute in Paris and Warsaw. She died in 1934 from radiation exposure.`

	response, err := client.Generate(ctx, personPrompt,
		gemini.WithResponseFormat(*personFormat),
		gemini.WithSystemMessage("You are an expert biographical analyst. Extract structured information about people."),
	)
	if err != nil {
		log.Fatalf("Failed to analyze person: %v", err)
	}

	// Parse and display the structured response
	var personAnalysis PersonAnalysis
	if err := json.Unmarshal([]byte(response), &personAnalysis); err != nil {
		log.Printf("Failed to parse person analysis: %v", err)
		fmt.Printf("Raw response: %s\n", response)
	} else {
		fmt.Printf("Name: %s\n", personAnalysis.Name)
		fmt.Printf("Profession: %s\n", personAnalysis.Profession)
		fmt.Printf("Experience: %d years\n", personAnalysis.Experience)
		fmt.Printf("Skills: %v\n", personAnalysis.Skills)
		fmt.Printf("Summary: %s\n", personAnalysis.Summary)
		fmt.Printf("Achievements:\n")
		for _, achievement := range personAnalysis.Achievements {
			fmt.Printf("  - %s (%d): %s\n", achievement.Title, achievement.Year, achievement.Description)
		}
	}
	fmt.Println()

	// Example 2: Business Analysis
	fmt.Println("=== Example 2: Business Analysis ===")
	businessFormat := structuredoutput.NewResponseFormat(BusinessAnalysis{})

	businessPrompt := `Analyze this company:
	Tesla, Inc. is an American multinational automotive and clean energy company headquartered in Austin, Texas.
	Tesla designs and manufactures electric vehicles, battery energy storage systems, and solar panels.
	Founded in 2003 by Martin Eberhard and Marc Tarpenning, the company was later led by Elon Musk as CEO.
	Tesla went public in 2010 and has become the world's most valuable automaker. The company has around 140,000
	employees worldwide and reported revenue of approximately $96 billion in 2023. Major products include Model S,
	Model 3, Model X, Model Y vehicles, Powerwall home batteries, and solar roof tiles.`

	response, err = client.Generate(ctx, businessPrompt,
		gemini.WithResponseFormat(*businessFormat),
		gemini.WithSystemMessage("You are a business analyst. Provide comprehensive company analysis."),
	)
	if err != nil {
		log.Fatalf("Failed to analyze business: %v", err)
	}

	var businessAnalysis BusinessAnalysis
	if err := json.Unmarshal([]byte(response), &businessAnalysis); err != nil {
		log.Printf("Failed to parse business analysis: %v", err)
		fmt.Printf("Raw response: %s\n", response)
	} else {
		fmt.Printf("Company: %s\n", businessAnalysis.CompanyName)
		fmt.Printf("Industry: %s\n", businessAnalysis.Industry)
		fmt.Printf("Founded: %d\n", businessAnalysis.Founded)
		fmt.Printf("Headquarters: %s\n", businessAnalysis.Headquarters)
		fmt.Printf("Employees: %d-%d\n", businessAnalysis.Employees.Min, businessAnalysis.Employees.Max)
		fmt.Printf("Revenue: $%.1fB %s (%d)\n", businessAnalysis.Revenue.Amount/1e9, businessAnalysis.Revenue.Currency, businessAnalysis.Revenue.Year)
		fmt.Printf("Products:\n")
		for _, product := range businessAnalysis.Products {
			fmt.Printf("  - %s (%s, %d): %s\n", product.Name, product.Category, product.LaunchYear, product.Description)
		}
		fmt.Printf("Key Metrics: %v\n", businessAnalysis.KeyMetrics)
		fmt.Printf("Strengths: %v\n", businessAnalysis.Strengths)
		fmt.Printf("Challenges: %v\n", businessAnalysis.Challenges)
	}
	fmt.Println()

	// Example 3: Recipe Analysis
	fmt.Println("=== Example 3: Recipe Analysis ===")
	recipeFormat := structuredoutput.NewResponseFormat(RecipeAnalysis{})

	recipePrompt := `Analyze this recipe text and extract structured information:

	Spaghetti Carbonara (Serves 4)

	This classic Italian pasta dish takes about 30 minutes total - 10 minutes prep and 20 minutes cooking.
	It's moderately difficult due to the technique required for the egg mixture.

	Ingredients:
	- 400g spaghetti pasta
	- 150g pancetta or guanciale, diced
	- 3 large eggs plus 1 extra yolk
	- 100g Pecorino Romano cheese, grated
	- 50g Parmigiano-Reggiano, grated
	- Black pepper, freshly ground
	- Salt for pasta water

	Instructions:
	1. Bring a large pot of salted water to boil and cook spaghetti until al dente
	2. Meanwhile, cook pancetta in a large pan until crispy
	3. In a bowl, whisk eggs, egg yolk, both cheeses, and black pepper
	4. Drain pasta, reserving 1 cup pasta water
	5. Add hot pasta to the pancetta pan
	6. Remove from heat and quickly mix in egg mixture, adding pasta water gradually
	7. Toss until creamy sauce forms
	8. Serve immediately with extra cheese and pepper

	This dish is high in protein and carbohydrates. Each serving has approximately 650 calories.`

	response, err = client.Generate(ctx, recipePrompt,
		gemini.WithResponseFormat(*recipeFormat),
		gemini.WithSystemMessage("You are a culinary expert. Extract detailed recipe information."),
	)
	if err != nil {
		log.Fatalf("Failed to analyze recipe: %v", err)
	}

	var recipeAnalysis RecipeAnalysis
	if err := json.Unmarshal([]byte(response), &recipeAnalysis); err != nil {
		log.Printf("Failed to parse recipe analysis: %v", err)
		fmt.Printf("Raw response: %s\n", response)
	} else {
		fmt.Printf("Recipe: %s\n", recipeAnalysis.Name)
		fmt.Printf("Cuisine: %s\n", recipeAnalysis.Cuisine)
		fmt.Printf("Difficulty: %s\n", recipeAnalysis.Difficulty)
		fmt.Printf("Time: %d min prep + %d min cook = %d min total\n",
			recipeAnalysis.PrepTime, recipeAnalysis.CookTime, recipeAnalysis.PrepTime+recipeAnalysis.CookTime)
		fmt.Printf("Servings: %d\n", recipeAnalysis.Servings)
		fmt.Printf("Ingredients:\n")
		for _, ingredient := range recipeAnalysis.Ingredients {
			fmt.Printf("  - %.1f %s %s", ingredient.Quantity, ingredient.Unit, ingredient.Name)
			if ingredient.Notes != "" {
				fmt.Printf(" (%s)", ingredient.Notes)
			}
			fmt.Println()
		}
		fmt.Printf("Instructions: %d steps\n", len(recipeAnalysis.Instructions))
		if len(recipeAnalysis.Tags) > 0 {
			fmt.Printf("Tags: %v\n", recipeAnalysis.Tags)
		}
	}
	fmt.Println()

	// Define data extraction structure
	type DataExtraction struct {
		Entities struct {
			People        []string `json:"people"`
			Places        []string `json:"places"`
			Organizations []string `json:"organizations"`
			Dates         []string `json:"dates"`
			Numbers       []string `json:"numbers"`
		} `json:"entities"`
		Sentiment   string   `json:"sentiment"`
		KeyTopics   []string `json:"key_topics"`
		Summary     string   `json:"summary"`
		ActionItems []struct {
			Task     string `json:"task"`
			Priority string `json:"priority"`
			Assignee string `json:"assignee"`
		} `json:"action_items,omitempty"`
	}

	// Example 4: Multi-field Data Extraction
	fmt.Println("=== Example 4: Multi-field Data Extraction ===")
	extractionFormat := structuredoutput.NewResponseFormat(DataExtraction{})

	extractionPrompt := `Extract structured information from this business meeting transcript:

	Meeting Notes - Q4 Planning Session
	Date: December 15, 2024
	Attendees: Sarah Johnson (CEO), Mike Chen (CTO), Lisa Rodriguez (Marketing Director)

	Sarah opened the meeting discussing the 25% revenue growth this quarter, reaching $2.3M.
	She praised the team's efforts in the New York and London markets. Mike reported that the
	new AI platform will launch in January 2025, requiring 3 additional engineers. Lisa
	mentioned the upcoming campaign for the European market will cost $150K but could bring
	in 500 new customers. The team agreed to hire 2 engineers by January 15th and increase
	the marketing budget by $200K for Q1 2025. Overall, everyone was optimistic about next
	year's projections of $12M annual revenue.`

	response, err = client.Generate(ctx, extractionPrompt,
		gemini.WithResponseFormat(*extractionFormat),
		gemini.WithSystemMessage("You are an expert data extraction specialist. Extract all relevant structured information."),
	)
	if err != nil {
		log.Fatalf("Failed to extract data: %v", err)
	}

	// Parse and display the structured response
	var dataExtraction DataExtraction
	if err := json.Unmarshal([]byte(response), &dataExtraction); err != nil {
		log.Printf("Failed to parse data extraction: %v", err)
		fmt.Printf("Raw response: %s\n", response)
	} else {
		fmt.Printf("Sentiment: %s\n", dataExtraction.Sentiment)
		fmt.Printf("Key Topics: %v\n", dataExtraction.KeyTopics)
		fmt.Printf("Summary: %s\n", dataExtraction.Summary)
		fmt.Printf("Entities:\n")
		fmt.Printf("  - People: %v\n", dataExtraction.Entities.People)
		fmt.Printf("  - Places: %v\n", dataExtraction.Entities.Places)
		fmt.Printf("  - Organizations: %v\n", dataExtraction.Entities.Organizations)
		fmt.Printf("  - Dates: %v\n", dataExtraction.Entities.Dates)
		fmt.Printf("  - Numbers: %v\n", dataExtraction.Entities.Numbers)
		if len(dataExtraction.ActionItems) > 0 {
			fmt.Printf("Action Items:\n")
			for _, item := range dataExtraction.ActionItems {
				fmt.Printf("  - %s (%s priority, assigned to: %s)\n", item.Task, item.Priority, item.Assignee)
			}
		}
	}
	fmt.Println()

	fmt.Println("✅ Structured output examples completed!")
	fmt.Println("\nKey capabilities demonstrated:")
	fmt.Println("- Person biographical analysis")
	fmt.Println("- Business/company analysis")
	fmt.Println("- Recipe and culinary data extraction")
	fmt.Println("- Multi-field entity and data extraction")
	fmt.Println("- Automatic JSON schema generation from Go structs")
	fmt.Println("- Type-safe Go struct unmarshaling")
	fmt.Println("- Clean, maintainable structured output code")
}
