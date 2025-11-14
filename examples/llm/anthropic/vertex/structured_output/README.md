# Anthropic Vertex AI Structured Output Example

This example demonstrates how to use structured output with Anthropic Claude models through Google Cloud Vertex AI, generating responses in specific JSON formats for enterprise use cases.

## Overview

This example shows how to integrate Anthropic's Claude models with Google Cloud Vertex AI for structured output generation. It's particularly useful for enterprise environments that prefer Google Cloud's infrastructure and need predictable, parseable responses from Claude.

## Features

- **Vertex AI Integration**: Uses Google Cloud's Vertex AI service to access Claude models
- **Enterprise Schemas**: Complex structured data formats for business use cases
- **Advanced Use Cases**: Research analysis, company evaluation, technical specifications
- **Agent Framework**: Integration with the agent framework using Vertex AI
- **Production Ready**: Error handling and validation for enterprise deployment

## What This Example Does

The example demonstrates four enterprise-focused use cases:

1. **Research Paper Analysis**: Structured analysis of academic papers with methodology, findings, and significance
2. **Company Analysis**: Comprehensive business analysis with financials, competitors, and growth prospects
3. **Technical Specifications**: Detailed technical documentation with requirements, performance metrics, and compatibility
4. **Agent Integration**: Using structured output within an agent framework powered by Vertex AI

## Prerequisites

- Go 1.19 or later
- Google Cloud Project with Vertex AI API enabled
- Google Cloud credentials configured
- Access to Anthropic models through Vertex AI

## Setup

1. **Set up Google Cloud Project**:
```bash
export GOOGLE_CLOUD_PROJECT=your-project-id
export VERTEX_AI_REGION=us-east5  # Optional, defaults to us-east5
# Alternative: export GOOGLE_CLOUD_REGION=us-east5
```

2. **Authenticate with Google Cloud**:
```bash
# Option 1: Use Application Default Credentials (recommended for development)
gcloud auth application-default login

# Option 2: Use Service Account Key (recommended for production)
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json
```

3. **Enable Vertex AI API**:
```bash
gcloud services enable aiplatform.googleapis.com
```

4. **Run the example**:
```bash
cd examples/llm/anthropic/vertex/structured_output
go run main.go
```

## How It Works

### 1. Configure Vertex AI Client

```go
client := anthropic.NewClient(
    "", // No API key needed for Vertex AI
    anthropic.WithModel("claude-sonnet-4@20250514"),
    anthropic.WithVertexAI(region, projectID),
)
```

### 2. Define Multi-Level Enterprise Data Structures

```go
// Nested financial metrics structure
type FinancialMetrics struct {
    Revenue          float64 `json:"revenue" description:"Annual revenue in billions USD"`
    NetIncome        float64 `json:"net_income" description:"Net income in billions USD"`
    GrossMargin      float64 `json:"gross_margin" description:"Gross margin percentage"`
    OperatingMargin  float64 `json:"operating_margin" description:"Operating margin percentage"`
    DebtToEquity     float64 `json:"debt_to_equity" description:"Debt to equity ratio"`
    ReturnOnEquity   float64 `json:"return_on_equity" description:"Return on equity percentage"`
    FreeCashFlow     float64 `json:"free_cash_flow" description:"Free cash flow in billions USD"`
}

// Main company analysis structure with nested objects
type CompanyAnalysis struct {
    CompanyName      string           `json:"company_name" description:"Name of the company"`
    Industry         string           `json:"industry" description:"Industry sector"`
    MarketCap        float64          `json:"market_cap" description:"Market capitalization in billions USD"`
    Employees        int              `json:"employees" description:"Number of employees"`
    BusinessModel    string           `json:"business_model" description:"Primary business model"`
    FinancialMetrics FinancialMetrics `json:"financial_metrics" description:"Detailed financial metrics"`
    MarketPosition   MarketPosition   `json:"market_position" description:"Market position analysis"`
    Innovation       Innovation       `json:"innovation" description:"Innovation and R&D information"`
    // ... other fields
}
```

### 3. Generate Structured Enterprise Data

```go
responseFormat := structuredoutput.NewResponseFormat(CompanyAnalysis{})

response, err := client.Generate(
    ctx,
    "Provide a comprehensive analysis of NVIDIA Corporation",
    anthropic.WithResponseFormat(*responseFormat),
    anthropic.WithSystemMessage("You are a financial analyst specializing in technology companies."),
)
```

## Enterprise Use Cases

### Research Paper Analysis
- Academic paper evaluation
- Literature review automation
- Research trend analysis
- Citation and impact assessment

### Company Analysis
- Financial due diligence
- Market research
- Competitive intelligence
- Investment analysis
- ESG scoring

### Technical Specifications
- Product documentation
- System architecture analysis
- Compatibility assessment
- Performance benchmarking

## Authentication Options

### Development Environment
```bash
gcloud auth application-default login
export VERTEX_AI_REGION=us-east5
export GOOGLE_CLOUD_PROJECT=your-project-id
```

### Production Environment (Service Account)
```bash
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json
export VERTEX_AI_REGION=us-east5
export GOOGLE_CLOUD_PROJECT=your-project-id
```

### Explicit Credentials in Code
```go
client := anthropic.NewClient(
    "",
    anthropic.WithModel("claude-sonnet-4@20250514"),
    anthropic.WithVertexAICredentials(region, projectID, "/path/to/credentials.json"),
)
```

## Error Handling

The example includes comprehensive error handling for:

- Missing Google Cloud project configuration
- Authentication failures
- Vertex AI API errors
- JSON parsing errors
- Network connectivity issues

## Best Practices Implemented

1. **Response Prefilling**: Uses `{` prefill for consistent JSON output
2. **Schema Examples**: Automatically generates examples from struct definitions
3. **Temperature Control**: Lower temperatures for factual, structured responses
4. **System Messages**: Specialized prompts for different domains (financial, technical, academic)
5. **Validation**: JSON schema validation and error reporting

## Supported Claude Models

When using Vertex AI, use the `@` format for model names:

- `claude-sonnet-4@20250514` (Sonnet 4)
- `claude-opus-4@20250514` (Opus 4)
- `claude-opus-4-1@20250805` (Opus 4.1)
- `claude-3-7-sonnet@20250219` (Sonnet 3.7)

## Output Examples

### Multi-Level Company Analysis Output
```json
{
  "company_name": "NVIDIA Corporation",
  "industry": "Semiconductors",
  "founded": 1993,
  "headquarters": "Santa Clara, California, USA",
  "market_cap": 1800.5,
  "employees": 26196,
  "business_model": "Design and manufacture of GPUs and AI chips",
  "key_products": ["GeForce GPUs", "Quadro", "Tesla", "Jetson", "DRIVE"],
  "competitors": ["AMD", "Intel", "Qualcomm"],
  "financial_health": "Excellent with strong revenue growth and profitability",
  "growth_prospects": "Very strong driven by AI and data center demand",
  "esg_score": 7.8,
  "financial_metrics": {
    "revenue": 79.8,
    "net_income": 29.8,
    "gross_margin": 73.2,
    "operating_margin": 32.5,
    "debt_to_equity": 0.24,
    "return_on_equity": 115.2,
    "free_cash_flow": 26.9
  },
  "market_position": {
    "market_share": 88.0,
    "market_size": 150.0,
    "growth_rate": 35.5,
    "competitor_rank": 1,
    "geographic_reach": ["North America", "Europe", "Asia Pacific"],
    "customer_segments": ["Gaming", "Data Center", "Automotive", "Professional"]
  },
  "innovation": {
    "r_and_d_spending": 7.3,
    "patent_count": 11000,
    "recent_innovations": ["H100 GPU", "Grace CPU", "Omniverse Platform"],
    "technology_focus": ["AI/ML", "Ray Tracing", "Autonomous Driving"],
    "partnerships_count": 150
  }
}
```

## Deployment Considerations

- **IAM Permissions**: Ensure service account has `aiplatform.user` role
- **Regional Availability**: Claude models are available in us-east5 region
- **Rate Limiting**: Implement proper rate limiting for production use
- **Cost Management**: Monitor Vertex AI usage and costs
- **Monitoring**: Set up Cloud Monitoring for API usage tracking

## Troubleshooting

**Authentication Issues**:
- Verify `GOOGLE_CLOUD_PROJECT` is set
- Ensure `VERTEX_AI_REGION=us-east5` is configured
- Check `gcloud auth application-default login` status
- Validate service account permissions

**API Errors**:
- Ensure Vertex AI API is enabled
- Check model availability in your region
- Verify quota limits

**JSON Parsing Errors**:
- Review system message prompts
- Check temperature settings (lower is better for structured output)
- Validate schema definitions
