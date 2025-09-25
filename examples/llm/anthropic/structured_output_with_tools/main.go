package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
)

// MockWeatherTool provides mock weather data for testing
type MockWeatherTool struct{}

// NewMockWeatherTool creates a new mock weather tool
func NewMockWeatherTool() *MockWeatherTool {
	return &MockWeatherTool{}
}

// Name returns the tool name
func (t *MockWeatherTool) Name() string {
	return "get_weather"
}

// Description returns the tool description
func (t *MockWeatherTool) Description() string {
	return "Get current weather conditions for a specified location"
}

// Parameters returns the tool parameters
func (t *MockWeatherTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"location": {
			Type:        "string",
			Description: "The city and state/country for weather data (e.g., 'New York, NY' or 'London, UK')",
			Required:    true,
		},
		"units": {
			Type:        "string",
			Description: "Temperature units: 'celsius' or 'fahrenheit'",
			Required:    false,
			Default:     "celsius",
			Enum:        []interface{}{"celsius", "fahrenheit"},
		},
	}
}

// Run executes the tool with the given input
func (t *MockWeatherTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute runs the mock weather tool
func (t *MockWeatherTool) Execute(ctx context.Context, params string) (string, error) {
	var request struct {
		Location string `json:"location"`
		Units    string `json:"units"`
	}

	if err := json.Unmarshal([]byte(params), &request); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if request.Location == "" {
		return "", fmt.Errorf("location parameter is required")
	}

	if request.Units == "" {
		request.Units = "celsius"
	}

	// Generate mock weather data based on location
	weather := generateMockWeather(request.Location, request.Units)

	response, err := json.MarshalIndent(weather, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal weather data: %w", err)
	}

	return string(response), nil
}

// StockPriceTool provides mock stock price data
type StockPriceTool struct{}

// NewStockPriceTool creates a new mock stock price tool
func NewStockPriceTool() *StockPriceTool {
	return &StockPriceTool{}
}

// Name returns the tool name
func (t *StockPriceTool) Name() string {
	return "get_stock_price"
}

// Description returns the tool description
func (t *StockPriceTool) Description() string {
	return "Get current stock price and basic financial data for a company"
}

// Parameters returns the tool parameters
func (t *StockPriceTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"symbol": {
			Type:        "string",
			Description: "Stock ticker symbol (e.g., 'AAPL', 'TSLA', 'GOOGL')",
			Required:    true,
		},
		"include_metrics": {
			Type:        "boolean",
			Description: "Include additional financial metrics",
			Required:    false,
			Default:     false,
		},
	}
}

// Run executes the tool with the given input
func (t *StockPriceTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

// Execute runs the mock stock price tool
func (t *StockPriceTool) Execute(ctx context.Context, params string) (string, error) {
	var request struct {
		Symbol         string `json:"symbol"`
		IncludeMetrics bool   `json:"include_metrics"`
	}

	if err := json.Unmarshal([]byte(params), &request); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if request.Symbol == "" {
		return "", fmt.Errorf("symbol parameter is required")
	}

	// Generate mock stock data
	stockData := generateMockStockData(strings.ToUpper(request.Symbol), request.IncludeMetrics)

	response, err := json.MarshalIndent(stockData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal stock data: %w", err)
	}

	return string(response), nil
}

// extractJSONFromResponse cleans Claude's response to extract pure JSON
func extractJSONFromResponse(response string) string {
	// Remove markdown code blocks
	if strings.Contains(response, "```json") {
		start := strings.Index(response, "```json") + 7
		end := strings.LastIndex(response, "```")
		if start < end && end > start {
			return strings.TrimSpace(response[start:end])
		}
	}

	// Find first { and last } to extract JSON object
	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")
	if startIdx != -1 && endIdx != -1 && startIdx < endIdx {
		return strings.TrimSpace(response[startIdx:endIdx+1])
	}

	return response
}

// generateMockWeather creates realistic mock weather data
func generateMockWeather(location, units string) map[string]interface{} {
	location = strings.ToLower(location)

	// Base weather data that varies by location
	var temp, feelsLike, windSpeed, pressure float64
	var humidity, uvIndex int
	var condition string
	var alerts []string

	// Generate location-specific weather
	switch {
	case strings.Contains(location, "new york") || strings.Contains(location, "nyc"):
		if units == "fahrenheit" {
			temp, feelsLike = 68.0, 72.0
		} else {
			temp, feelsLike = 20.0, 22.2
		}
		humidity = 65
		windSpeed = 12.5
		pressure = 1013.2
		uvIndex = 6
		condition = "Partly Cloudy"

	case strings.Contains(location, "london"):
		if units == "fahrenheit" {
			temp, feelsLike = 59.0, 55.0
		} else {
			temp, feelsLike = 15.0, 12.8
		}
		humidity = 78
		windSpeed = 8.2
		pressure = 1008.5
		uvIndex = 3
		condition = "Light Rain"
		alerts = []string{"Light rain expected throughout the day"}

	default:
		// Default weather for unknown locations
		if units == "fahrenheit" {
			temp, feelsLike = 72.0, 75.0
		} else {
			temp, feelsLike = 22.2, 23.9
		}
		humidity = 60
		windSpeed = 10.0
		pressure = 1013.25
		uvIndex = 5
		condition = "Partly Cloudy"
	}

	weather := map[string]interface{}{
		"location": strings.Title(location),
		"current_conditions": map[string]interface{}{
			"temperature":    temp,
			"feels_like":     feelsLike,
			"humidity":       humidity,
			"wind_speed":     windSpeed,
			"pressure":       pressure,
			"uv_index":       uvIndex,
			"condition":      condition,
			"visibility":     16.1, // km
			"units":          units,
		},
		"alerts": alerts,
		"forecast": map[string]interface{}{
			"today":    "Current conditions expected to continue",
			"tomorrow": "Similar conditions with slight temperature variation",
		},
		"timestamp": "2024-01-15T14:30:00Z",
		"data_source": "Mock Weather Service",
	}

	return weather
}

// generateMockStockData creates realistic mock stock data
func generateMockStockData(symbol string, includeMetrics bool) map[string]interface{} {
	var price, change, volume float64
	var marketCap, revenue, netIncome float64
	var pe, eps, bookValue float64
	var companyName string

	// Generate symbol-specific data
	switch symbol {
	case "AAPL":
		companyName = "Apple Inc."
		price = 150.25
		change = 2.15
		volume = 52500000
		if includeMetrics {
			marketCap = 2400.5
			revenue = 394.3
			netIncome = 99.8
			pe = 24.8
			eps = 6.05
			bookValue = 4.25
		}

	case "TSLA":
		companyName = "Tesla Inc."
		price = 238.45
		change = -5.67
		volume = 85200000
		if includeMetrics {
			marketCap = 758.2
			revenue = 96.8
			netIncome = 15.0
			pe = 50.4
			eps = 4.73
			bookValue = 26.12
		}

	default:
		companyName = fmt.Sprintf("Unknown Company (%s)", symbol)
		price = 100.00
		change = 0.00
		volume = 1000000
		if includeMetrics {
			marketCap = 50.0
			revenue = 10.0
			netIncome = 1.0
			pe = 20.0
			eps = 5.00
			bookValue = 15.00
		}
	}

	stockData := map[string]interface{}{
		"symbol":       symbol,
		"company_name": companyName,
		"current_price": price,
		"change":       change,
		"change_percent": (change / (price - change)) * 100,
		"volume":       volume,
		"currency":     "USD",
		"market_status": "OPEN",
		"timestamp":    "2024-01-15T21:30:00Z",
		"data_source":  "Mock Stock Service",
	}

	if includeMetrics {
		stockData["financial_metrics"] = map[string]interface{}{
			"market_cap_billions":      marketCap,
			"revenue_billions":         revenue,
			"net_income_billions":      netIncome,
			"pe_ratio":                pe,
			"earnings_per_share":       eps,
			"book_value_per_share":     bookValue,
			"price_to_book":           price / bookValue,
			"shares_outstanding_billions": marketCap / price,
		}
	}

	return stockData
}

// SimpleAnalysis represents a simple structured analysis
type SimpleAnalysis struct {
	CompanyName   string              `json:"company_name" description:"Name of the company"`
	CurrentPrice  float64             `json:"current_price" description:"Current stock price"`
	Calculations  []CalculationResult `json:"calculations" description:"Calculations performed"`
	Recommendation string             `json:"recommendation" description:"Investment recommendation"`
	DataSources   []string            `json:"data_sources" description:"Data sources used"`
}

// CalculationResult represents a calculation result
type CalculationResult struct {
	Name        string  `json:"name" description:"Name of the calculation"`
	Formula     string  `json:"formula" description:"Formula used"`
	Result      float64 `json:"result" description:"Calculated result"`
	Description string  `json:"description" description:"What this means"`
}

// runSimpleTest runs a simple structured output + tools test
func runSimpleTest() {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("ANTHROPIC_API_KEY environment variable is required")
		return
	}

	// Create context
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "test-org")

	fmt.Println("Simple Structured Output + Tools Test")
	fmt.Println("=====================================")

	// Create Anthropic client
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(anthropic.ClaudeSonnet4),
	)

	// Create tools
	calc := calculator.New()
	stockTool := NewStockPriceTool()

	// Create response format
	responseFormat := structuredoutput.NewResponseFormat(SimpleAnalysis{})

	// Simple request with structured output + tools
	response, err := client.GenerateWithTools(
		ctx,
		`Analyze Apple (AAPL) stock. Get the current price, then calculate P/E ratio if price is $150 and EPS is $6. Provide your analysis in the required JSON format.`,
		[]interfaces.Tool{stockTool, calc},
		anthropic.WithResponseFormat(*responseFormat),
		anthropic.WithSystemMessage(`You are a financial analyst. Use tools to get data and perform calculations.
		Always respond with valid JSON matching the specified schema. Do not add any text before or after the JSON.`),
		anthropic.WithTemperature(0.1),
	)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Raw Response:\n%s\n\n", response)

	// Clean and parse JSON
	cleanJSON := extractJSONFromResponse(response)
	fmt.Printf("Cleaned JSON:\n%s\n\n", cleanJSON)

	var analysis SimpleAnalysis
	if err := json.Unmarshal([]byte(cleanJSON), &analysis); err != nil {
		fmt.Printf("JSON Parse Error: %v\n", err)
		return
	}

	// Display results
	fmt.Printf("Parsed Successfully!\n")
	fmt.Printf("Company: %s\n", analysis.CompanyName)
	fmt.Printf("Price: $%.2f\n", analysis.CurrentPrice)
	fmt.Printf("Recommendation: %s\n", analysis.Recommendation)
	fmt.Printf("Calculations: %d performed\n", len(analysis.Calculations))
	for _, calc := range analysis.Calculations {
		fmt.Printf("  - %s: %.2f\n", calc.Name, calc.Result)
	}
}

// FinancialAnalysis represents a structured financial analysis with calculations
type FinancialAnalysis struct {
	CompanyName        string              `json:"company_name" description:"Name of the company being analyzed"`
	CurrentStockPrice  float64             `json:"current_stock_price" description:"Current stock price in USD"`
	MarketCap          float64             `json:"market_cap" description:"Market capitalization in billions USD"`
	PERatio            float64             `json:"pe_ratio" description:"Price-to-earnings ratio"`
	ValuationMetrics   ValuationMetrics    `json:"valuation_metrics" description:"Detailed valuation calculations"`
	InvestmentDecision InvestmentDecision  `json:"investment_decision" description:"Investment recommendation"`
	RiskAssessment     RiskAssessment      `json:"risk_assessment" description:"Risk analysis"`
	DataSources        []string            `json:"data_sources" description:"Sources of information used"`
	CalculationSteps   []CalculationStep   `json:"calculation_steps" description:"Step-by-step calculations performed"`
}

// ValuationMetrics represents detailed valuation calculations
type ValuationMetrics struct {
	Revenue            float64 `json:"revenue" description:"Annual revenue in billions USD"`
	NetIncome          float64 `json:"net_income" description:"Net income in billions USD"`
	SharesOutstanding  float64 `json:"shares_outstanding" description:"Number of shares outstanding in billions"`
	EarningsPerShare   float64 `json:"earnings_per_share" description:"Earnings per share in USD"`
	BookValue          float64 `json:"book_value" description:"Book value per share in USD"`
	PriceToBook        float64 `json:"price_to_book" description:"Price-to-book ratio"`
	DebtToEquity       float64 `json:"debt_to_equity" description:"Debt-to-equity ratio"`
	ReturnOnEquity     float64 `json:"return_on_equity" description:"Return on equity percentage"`
}

// InvestmentDecision represents the investment recommendation
type InvestmentDecision struct {
	Recommendation string  `json:"recommendation" description:"Investment recommendation (BUY, HOLD, SELL)"`
	TargetPrice    float64 `json:"target_price" description:"Target price in USD"`
	Confidence     float64 `json:"confidence" description:"Confidence level from 0.0 to 1.0"`
	TimeHorizon    string  `json:"time_horizon" description:"Investment time horizon"`
	Reasoning      string  `json:"reasoning" description:"Reasoning behind the recommendation"`
}

// RiskAssessment represents risk analysis
type RiskAssessment struct {
	RiskLevel      string   `json:"risk_level" description:"Overall risk level (LOW, MEDIUM, HIGH)"`
	RiskScore      float64  `json:"risk_score" description:"Risk score from 1.0 to 10.0"`
	KeyRisks       []string `json:"key_risks" description:"Primary risk factors"`
	Volatility     float64  `json:"volatility" description:"Stock volatility percentage"`
	Beta           float64  `json:"beta" description:"Beta coefficient"`
	MaxDrawdown    float64  `json:"max_drawdown" description:"Maximum drawdown percentage"`
}

// CalculationStep represents a single calculation step
type CalculationStep struct {
	Description string  `json:"description" description:"Description of the calculation"`
	Formula     string  `json:"formula" description:"Mathematical formula used"`
	Input       string  `json:"input" description:"Input values"`
	Result      float64 `json:"result" description:"Calculation result"`
	Unit        string  `json:"unit" description:"Unit of measurement"`
}

// WeatherAnalysis represents structured weather analysis with tool usage
type WeatherAnalysis struct {
	Location           string                 `json:"location" description:"Location analyzed"`
	CurrentConditions  CurrentWeatherData     `json:"current_conditions" description:"Current weather conditions"`
	WeatherAlert       WeatherAlert           `json:"weather_alert" description:"Weather alert information"`
	ActivitySuggestion ActivitySuggestion     `json:"activity_suggestion" description:"Activity recommendations"`
	CalculatedMetrics  []WeatherCalculation   `json:"calculated_metrics" description:"Weather calculations performed"`
	DataSources        []string               `json:"data_sources" description:"Weather data sources used"`
}

// CurrentWeatherData represents current weather information
type CurrentWeatherData struct {
	Temperature     float64 `json:"temperature" description:"Temperature in Celsius"`
	FeelsLike       float64 `json:"feels_like" description:"Feels like temperature in Celsius"`
	Humidity        int     `json:"humidity" description:"Humidity percentage"`
	WindSpeed       float64 `json:"wind_speed" description:"Wind speed in km/h"`
	Visibility      float64 `json:"visibility" description:"Visibility in kilometers"`
	UVIndex         int     `json:"uv_index" description:"UV index (0-11)"`
	Pressure        float64 `json:"pressure" description:"Atmospheric pressure in hPa"`
	Condition       string  `json:"condition" description:"Weather condition description"`
}

// WeatherAlert represents weather alerts and warnings
type WeatherAlert struct {
	HasAlert    bool     `json:"has_alert" description:"Whether there are active alerts"`
	AlertType   string   `json:"alert_type" description:"Type of weather alert"`
	Severity    string   `json:"severity" description:"Alert severity level"`
	Description string   `json:"description" description:"Alert description"`
	Precautions []string `json:"precautions" description:"Recommended precautions"`
}

// ActivitySuggestion represents activity recommendations
type ActivitySuggestion struct {
	IndoorActivities  []string `json:"indoor_activities" description:"Recommended indoor activities"`
	OutdoorActivities []string `json:"outdoor_activities" description:"Recommended outdoor activities"`
	ClothingSuggestion string  `json:"clothing_suggestion" description:"Clothing recommendations"`
	TravelAdvisory    string   `json:"travel_advisory" description:"Travel recommendations"`
}

// WeatherCalculation represents weather-related calculations
type WeatherCalculation struct {
	MetricName  string  `json:"metric_name" description:"Name of the calculated metric"`
	Formula     string  `json:"formula" description:"Formula or method used"`
	Input       string  `json:"input" description:"Input parameters"`
	Result      float64 `json:"result" description:"Calculated result"`
	Unit        string  `json:"unit" description:"Unit of measurement"`
	Explanation string  `json:"explanation" description:"Explanation of what this metric means"`
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
	ctx = multitenancy.WithOrgID(ctx, "structured-tools-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "structured-tools-demo")

	fmt.Println("Anthropic Structured Output + Tools Examples")
	fmt.Println("============================================")
	fmt.Println()

	// Example 1: Financial Analysis with Calculator and Stock Price Tools
	fmt.Println("Example 1: Financial Analysis with Calculator and Stock Price Tools")
	fmt.Println("-------------------------------------------------------------------")
	financialAnalysisExample(ctx, apiKey)
	fmt.Println()

	// Example 2: Weather Analysis with Mock Weather Tool
	fmt.Println("Example 2: Weather Analysis with Mock Weather Tool")
	fmt.Println("--------------------------------------------------")
	weatherAnalysisExample(ctx, apiKey)
	fmt.Println()

	// Example 3: Simple Structured Output + Tools Test
	fmt.Println("Example 3: Simple Structured Output + Tools Test")
	fmt.Println("------------------------------------------------")
	runSimpleTest()
	fmt.Println()

	// Example 4: Agent with Structured Output and Tools
	fmt.Println("Example 4: Agent with Structured Output and Tools")
	fmt.Println("-------------------------------------------------")
	agentWithToolsExample(ctx, apiKey)
}

func financialAnalysisExample(ctx context.Context, apiKey string) {
	// Create Anthropic client
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(anthropic.ClaudeSonnet4),
	)

	// Create tools
	calc := calculator.New()
	stockTool := NewStockPriceTool()

	// Create response format for financial analysis
	responseFormat := structuredoutput.NewResponseFormat(FinancialAnalysis{})

	// Generate structured financial analysis with tools
	response, err := client.GenerateWithTools(
		ctx,
		`Analyze Apple Inc. (AAPL) stock for investment. First get the current stock price and financial metrics, then use the calculator to perform key financial calculations:
		- Get current AAPL stock data with financial metrics
		- Calculate market cap using current price and shares outstanding
		- Calculate P/E ratio using current price and EPS
		- Calculate price-to-book ratio
		- Calculate ROE percentage

		Provide a comprehensive investment analysis with structured output including all calculation steps.`,
		[]interfaces.Tool{stockTool, calc},
		anthropic.WithResponseFormat(*responseFormat),
		anthropic.WithSystemMessage(`You are a financial analyst who provides detailed investment analysis.
		Use the calculator tool to perform accurate calculations and include all calculation steps in your analysis.
		Always structure your response according to the specified JSON schema.`),
		anthropic.WithTemperature(0.2),
	)
	if err != nil {
		fmt.Printf("Error generating financial analysis: %v\n", err)
		return
	}

	// Parse the JSON response
	var analysis FinancialAnalysis
	if err := json.Unmarshal([]byte(response), &analysis); err != nil {
		fmt.Printf("Error parsing financial analysis: %v\n", err)
		fmt.Printf("Raw response: %s\n", response)
		return
	}

	// Display the structured analysis
	fmt.Printf("Company: %s\n", analysis.CompanyName)
	fmt.Printf("Current Stock Price: $%.2f\n", analysis.CurrentStockPrice)
	fmt.Printf("Market Cap: $%.1fB\n", analysis.MarketCap)
	fmt.Printf("P/E Ratio: %.2f\n", analysis.PERatio)

	fmt.Printf("\nValuation Metrics:\n")
	fmt.Printf("  Revenue: $%.1fB\n", analysis.ValuationMetrics.Revenue)
	fmt.Printf("  Net Income: $%.1fB\n", analysis.ValuationMetrics.NetIncome)
	fmt.Printf("  EPS: $%.2f\n", analysis.ValuationMetrics.EarningsPerShare)
	fmt.Printf("  P/B Ratio: %.2f\n", analysis.ValuationMetrics.PriceToBook)
	fmt.Printf("  ROE: %.1f%%\n", analysis.ValuationMetrics.ReturnOnEquity)

	fmt.Printf("\nInvestment Decision:\n")
	fmt.Printf("  Recommendation: %s\n", analysis.InvestmentDecision.Recommendation)
	fmt.Printf("  Target Price: $%.2f\n", analysis.InvestmentDecision.TargetPrice)
	fmt.Printf("  Confidence: %.1f%%\n", analysis.InvestmentDecision.Confidence*100)
	fmt.Printf("  Reasoning: %s\n", analysis.InvestmentDecision.Reasoning)

	fmt.Printf("\nRisk Assessment:\n")
	fmt.Printf("  Risk Level: %s\n", analysis.RiskAssessment.RiskLevel)
	fmt.Printf("  Risk Score: %.1f/10\n", analysis.RiskAssessment.RiskScore)
	fmt.Printf("  Beta: %.2f\n", analysis.RiskAssessment.Beta)
	fmt.Printf("  Key Risks: %v\n", analysis.RiskAssessment.KeyRisks)

	fmt.Printf("\nCalculation Steps Performed:\n")
	for i, step := range analysis.CalculationSteps {
		fmt.Printf("  %d. %s\n", i+1, step.Description)
		fmt.Printf("     Formula: %s\n", step.Formula)
		fmt.Printf("     Result: %.2f %s\n", step.Result, step.Unit)
	}

	fmt.Printf("\nData Sources: %v\n", analysis.DataSources)
}

func weatherAnalysisExample(ctx context.Context, apiKey string) {
	// Create Anthropic client
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(anthropic.ClaudeSonnet4),
	)

	// Create tools
	weatherTool := NewMockWeatherTool()
	calc := calculator.New()

	// Create response format for weather analysis
	responseFormat := structuredoutput.NewResponseFormat(WeatherAnalysis{})

	// Generate structured weather analysis with tools
	response, err := client.GenerateWithTools(
		ctx,
		`Analyze current weather conditions in New York City. Use the weather tool to get current conditions and calculator for weather-related calculations:
		- Get current NYC weather conditions in Celsius
		- Calculate heat index or wind chill if applicable
		- Calculate visibility in miles from kilometers
		- Calculate wind speed in mph from km/h
		- Provide activity recommendations based on conditions

		Structure your analysis with detailed weather metrics and calculations.`,
		[]interfaces.Tool{weatherTool, calc},
		anthropic.WithResponseFormat(*responseFormat),
		anthropic.WithSystemMessage(`You are a meteorologist who provides comprehensive weather analysis.
		Use the weather tool to get current weather data and calculator for weather-related computations.
		Structure your response according to the specified JSON schema with detailed weather information.`),
		anthropic.WithTemperature(0.3),
	)
	if err != nil {
		fmt.Printf("Error generating weather analysis: %v\n", err)
		return
	}

	// Parse the JSON response
	var weather WeatherAnalysis
	if err := json.Unmarshal([]byte(response), &weather); err != nil {
		fmt.Printf("Error parsing weather analysis: %v\n", err)
		fmt.Printf("Raw response: %s\n", response)
		return
	}

	// Display the structured weather analysis
	fmt.Printf("Location: %s\n", weather.Location)

	fmt.Printf("\nCurrent Conditions:\n")
	fmt.Printf("  Temperature: %.1f°C (feels like %.1f°C)\n",
		weather.CurrentConditions.Temperature, weather.CurrentConditions.FeelsLike)
	fmt.Printf("  Condition: %s\n", weather.CurrentConditions.Condition)
	fmt.Printf("  Humidity: %d%%\n", weather.CurrentConditions.Humidity)
	fmt.Printf("  Wind Speed: %.1f km/h\n", weather.CurrentConditions.WindSpeed)
	fmt.Printf("  Visibility: %.1f km\n", weather.CurrentConditions.Visibility)
	fmt.Printf("  UV Index: %d\n", weather.CurrentConditions.UVIndex)
	fmt.Printf("  Pressure: %.1f hPa\n", weather.CurrentConditions.Pressure)

	fmt.Printf("\nWeather Alerts:\n")
	if weather.WeatherAlert.HasAlert {
		fmt.Printf("  Alert Type: %s\n", weather.WeatherAlert.AlertType)
		fmt.Printf("  Severity: %s\n", weather.WeatherAlert.Severity)
		fmt.Printf("  Description: %s\n", weather.WeatherAlert.Description)
		fmt.Printf("  Precautions: %v\n", weather.WeatherAlert.Precautions)
	} else {
		fmt.Printf("  No active weather alerts\n")
	}

	fmt.Printf("\nActivity Suggestions:\n")
	fmt.Printf("  Indoor Activities: %v\n", weather.ActivitySuggestion.IndoorActivities)
	fmt.Printf("  Outdoor Activities: %v\n", weather.ActivitySuggestion.OutdoorActivities)
	fmt.Printf("  Clothing: %s\n", weather.ActivitySuggestion.ClothingSuggestion)
	fmt.Printf("  Travel: %s\n", weather.ActivitySuggestion.TravelAdvisory)

	fmt.Printf("\nCalculated Metrics:\n")
	for _, calc := range weather.CalculatedMetrics {
		fmt.Printf("  %s: %.2f %s\n", calc.MetricName, calc.Result, calc.Unit)
		fmt.Printf("    Formula: %s\n", calc.Formula)
		fmt.Printf("    Explanation: %s\n", calc.Explanation)
	}

	fmt.Printf("\nData Sources: %v\n", weather.DataSources)
}

func agentWithToolsExample(ctx context.Context, apiKey string) {
	// Create Anthropic client
	client := anthropic.NewClient(
		apiKey,
		anthropic.WithModel(anthropic.ClaudeSonnet4),
	)

	// Create response format for financial analysis (reusing the struct)
	responseFormat := structuredoutput.NewResponseFormat(FinancialAnalysis{})

	// Create tools
	calc := calculator.New()
	stockTool := NewStockPriceTool()

	// Create an agent with structured output and tools
	agentInstance, err := agent.NewAgent(
		agent.WithLLM(client),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(calc, stockTool),
		agent.WithSystemPrompt(`You are a financial analyst AI assistant that provides comprehensive investment analysis.
		You have access to calculator and stock price tools to gather information and perform calculations.
		Always structure your responses according to the specified JSON schema.
		Use tools when you need to get stock data or perform calculations.`),
		agent.WithResponseFormat(*responseFormat),
	)
	if err != nil {
		fmt.Printf("Failed to create agent: %v\n", err)
		return
	}

	// Run the agent with a complex financial analysis request
	response, err := agentInstance.Run(ctx, `Analyze Tesla (TSLA) stock for investment. Get current Tesla stock price and financial metrics, then calculate key ratios like P/E, market cap, and provide a complete investment recommendation.`)
	if err != nil {
		fmt.Printf("Failed to run agent: %v\n", err)
		return
	}

	// Parse the JSON response
	var analysis FinancialAnalysis
	if err := json.Unmarshal([]byte(response), &analysis); err != nil {
		fmt.Printf("Error parsing agent response: %v\n", err)
		fmt.Printf("Raw response: %s\n", response)
		return
	}

	// Display the structured analysis from agent
	fmt.Printf("Agent Analysis for %s\n", analysis.CompanyName)
	fmt.Printf("Current Price: $%.2f\n", analysis.CurrentStockPrice)
	fmt.Printf("Market Cap: $%.1fB\n", analysis.MarketCap)
	fmt.Printf("P/E Ratio: %.2f\n", analysis.PERatio)
	fmt.Printf("\nRecommendation: %s\n", analysis.InvestmentDecision.Recommendation)
	fmt.Printf("Target Price: $%.2f\n", analysis.InvestmentDecision.TargetPrice)
	fmt.Printf("Confidence: %.0f%%\n", analysis.InvestmentDecision.Confidence*100)
	fmt.Printf("Risk Level: %s (%.1f/10)\n", analysis.RiskAssessment.RiskLevel, analysis.RiskAssessment.RiskScore)

	fmt.Printf("\nTools Used - Calculations Performed:\n")
	for _, step := range analysis.CalculationSteps {
		fmt.Printf("  - %s: %.2f %s\n", step.Description, step.Result, step.Unit)
	}
}