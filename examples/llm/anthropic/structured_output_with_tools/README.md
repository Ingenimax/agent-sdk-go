# Anthropic Structured Output with Tools Example

This example demonstrates how to combine Anthropic Claude's structured output capabilities with tool usage, showing how to get structured JSON responses while leveraging tools for data gathering and calculations.

## Overview

This example showcases the powerful combination of:
- **Structured Output**: Ensuring responses follow a specific JSON schema
- **Tool Integration**: Using tools to gather data and perform calculations
- **Multi-step Workflows**: Tools → Calculations → Structured Results

## Features

- **Mock Tools**: Custom weather and stock price tools with realistic data
- **Calculator Integration**: Mathematical calculations within structured workflows
- **Complex JSON Schemas**: Multi-level nested structures for comprehensive data
- **Agent Framework**: Full agent integration with tools and structured output
- **Real-world Use Cases**: Financial analysis and weather reporting scenarios

## What This Example Does

The example demonstrates three scenarios:

1. **Financial Analysis with Tools**:
   - Fetches stock data using mock stock price tool
   - Performs financial calculations using calculator
   - Returns structured investment analysis with all calculation steps

2. **Weather Analysis with Tools**:
   - Gets weather data using mock weather tool
   - Performs weather-related calculations (unit conversions, heat index)
   - Returns structured weather analysis with recommendations

3. **Agent Integration**:
   - Shows how agents can use tools while maintaining structured output
   - Demonstrates complex workflows with multiple tool calls

## Prerequisites

- Go 1.19 or later
- An Anthropic API key

## Setup

1. Set your Anthropic API key:
```bash
export ANTHROPIC_API_KEY=your_api_key_here
```

2. Run the example:
```bash
cd examples/llm/anthropic/structured_output_with_tools
go run *.go
```

## Mock Tools Included

### Weather Tool (`get_weather`)
Provides realistic weather data for various locations:
```json
{
  "location": "New York, NY",
  "current_conditions": {
    "temperature": 20.0,
    "feels_like": 22.2,
    "humidity": 65,
    "wind_speed": 12.5,
    "condition": "Partly Cloudy"
  },
  "alerts": [],
  "forecast": {...},
  "air_quality": {...}
}
```

### Stock Price Tool (`get_stock_price`)
Provides stock data for major companies (AAPL, TSLA, GOOGL, MSFT, NVDA):
```json
{
  "symbol": "AAPL",
  "company_name": "Apple Inc.",
  "current_price": 150.25,
  "financial_metrics": {
    "market_cap_billions": 2400.5,
    "pe_ratio": 24.8,
    "earnings_per_share": 6.05,
    "book_value_per_share": 4.25
  }
}
```

## Structured Output Examples

### Financial Analysis Structure
```go
type FinancialAnalysis struct {
    CompanyName        string              `json:"company_name"`
    CurrentStockPrice  float64             `json:"current_stock_price"`
    ValuationMetrics   ValuationMetrics    `json:"valuation_metrics"`
    InvestmentDecision InvestmentDecision  `json:"investment_decision"`
    RiskAssessment     RiskAssessment      `json:"risk_assessment"`
    CalculationSteps   []CalculationStep   `json:"calculation_steps"`
}
```

### Weather Analysis Structure
```go
type WeatherAnalysis struct {
    Location           string                 `json:"location"`
    CurrentConditions  CurrentWeatherData     `json:"current_conditions"`
    WeatherAlert       WeatherAlert           `json:"weather_alert"`
    ActivitySuggestion ActivitySuggestion     `json:"activity_suggestion"`
    CalculatedMetrics  []WeatherCalculation   `json:"calculated_metrics"`
}
```

## How It Works

### 1. Tool Execution + Structured Output
```go
response, err := client.GenerateWithTools(
    ctx,
    "Analyze AAPL stock. Get current data and calculate P/E ratio.",
    []interfaces.Tool{stockTool, calc},
    anthropic.WithResponseFormat(*responseFormat),
)
```

### 2. Agent with Tools and Structured Output
```go
agent, err := agent.NewAgent(
    agent.WithLLM(client),
    agent.WithTools([]interfaces.Tool{calc, stockTool}),
    agent.WithResponseFormat(*responseFormat),
)
```

### 3. Multi-Step Workflow
1. **Tool Call**: Get stock data from mock API
2. **Calculation**: Use calculator for financial ratios
3. **Structured Response**: Format everything as specified JSON schema

## Sample Output

### Financial Analysis Output
```json
{
  "company_name": "Apple Inc.",
  "current_stock_price": 150.25,
  "market_cap": 2400.5,
  "valuation_metrics": {
    "pe_ratio": 24.8,
    "earnings_per_share": 6.05,
    "price_to_book": 35.35,
    "return_on_equity": 153.5
  },
  "investment_decision": {
    "recommendation": "BUY",
    "target_price": 165.0,
    "confidence": 0.85,
    "reasoning": "Strong fundamentals and growth prospects"
  },
  "calculation_steps": [
    {
      "description": "Calculate P/E Ratio",
      "formula": "Price / Earnings Per Share",
      "result": 24.8,
      "unit": "ratio"
    }
  ]
}
```

## Key Benefits

### 1. **Data Reliability**
- Tools provide real data instead of AI hallucinations
- Calculator ensures accurate mathematical computations
- Mock tools provide consistent, testable data

### 2. **Structured Consistency**
- JSON schema validation ensures proper formatting
- Nested structures handle complex data relationships
- Type safety prevents runtime errors

### 3. **Workflow Integration**
- Tools → Calculations → Structured Output
- Agent framework handles complex multi-step processes
- Memory management for conversation context

## Best Practices Demonstrated

1. **Tool Selection**: Choose appropriate tools for data gathering vs computation
2. **Schema Design**: Create nested structures for complex data
3. **Error Handling**: Comprehensive error checking and validation
4. **Temperature Control**: Lower temperatures for factual, structured responses
5. **System Prompts**: Clear instructions for tool usage and structure compliance

## Extending the Example

### Add New Tools
```go
type MyCustomTool struct{}

func (t *MyCustomTool) Name() string { return "my_tool" }
func (t *MyCustomTool) Description() string { return "Does something useful" }
func (t *MyCustomTool) Parameters() map[string]interfaces.ParameterSpec { /* ... */ }
func (t *MyCustomTool) Execute(ctx context.Context, params string) (string, error) { /* ... */ }
```

### Create New Schemas
```go
type MyAnalysis struct {
    Summary     string            `json:"summary"`
    Details     MyDetails         `json:"details"`
    Calculations []CalculationStep `json:"calculations"`
}
```

## Use Cases

- **Financial Analysis**: Stock screening, portfolio analysis, risk assessment
- **Weather Reporting**: Meteorological analysis, activity planning, alerts
- **Research Analysis**: Data gathering + structured reporting
- **Business Intelligence**: KPI calculation with structured dashboards
- **Scientific Computing**: Data collection + analysis + structured results

This example provides a solid foundation for building applications that need both dynamic data gathering and predictable output formats.