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

// ResearchPaper represents a structured research paper analysis
type ResearchPaper struct {
	Title        string   `json:"title" description:"The title of the research paper"`
	Authors      []string `json:"authors" description:"List of paper authors"`
	Year         int      `json:"year" description:"Publication year"`
	Journal      string   `json:"journal" description:"Journal or conference name"`
	Abstract     string   `json:"abstract" description:"Brief summary of the paper's abstract"`
	Keywords     []string `json:"keywords" description:"Key terms and concepts"`
	Methodology  string   `json:"methodology" description:"Research methodology used"`
	Findings     []string `json:"findings" description:"Key findings and results"`
	Significance string   `json:"significance" description:"Significance and impact of the work"`
	Rating       float64  `json:"rating" description:"Quality rating from 1.0 to 10.0"`
}

// FinancialMetrics represents detailed financial information
type FinancialMetrics struct {
	Revenue          float64 `json:"revenue" description:"Annual revenue in billions USD"`
	NetIncome        float64 `json:"net_income" description:"Net income in billions USD"`
	GrossMargin      float64 `json:"gross_margin" description:"Gross margin percentage"`
	OperatingMargin  float64 `json:"operating_margin" description:"Operating margin percentage"`
	DebtToEquity     float64 `json:"debt_to_equity" description:"Debt to equity ratio"`
	CurrentRatio     float64 `json:"current_ratio" description:"Current ratio"`
	ReturnOnEquity   float64 `json:"return_on_equity" description:"Return on equity percentage"`
	FreeCashFlow     float64 `json:"free_cash_flow" description:"Free cash flow in billions USD"`
}

// MarketPosition represents market analysis information
type MarketPosition struct {
	MarketShare      float64  `json:"market_share" description:"Market share percentage"`
	MarketSize       float64  `json:"market_size" description:"Total addressable market in billions USD"`
	GrowthRate       float64  `json:"growth_rate" description:"Market growth rate percentage"`
	CompetitorRank   int      `json:"competitor_rank" description:"Ranking among competitors"`
	GeographicReach  []string `json:"geographic_reach" description:"Geographic markets served"`
	CustomerSegments []string `json:"customer_segments" description:"Primary customer segments"`
}

// Innovation represents R&D and innovation metrics
type Innovation struct {
	RAndDSpending    float64  `json:"r_and_d_spending" description:"R&D spending in billions USD"`
	PatentCount      int      `json:"patent_count" description:"Number of patents held"`
	RecentBrevets   []string `json:"recent_innovations" description:"Recent innovations or breakthroughs"`
	TechnologyFocus  []string `json:"technology_focus" description:"Key technology focus areas"`
	PartnershipsCount int     `json:"partnerships_count" description:"Number of strategic partnerships"`
}

// CompanyAnalysis represents structured company analysis
type CompanyAnalysis struct {
	CompanyName      string           `json:"company_name" description:"Name of the company"`
	Industry         string           `json:"industry" description:"Industry sector"`
	Founded          int              `json:"founded" description:"Year company was founded"`
	Headquarters     string           `json:"headquarters" description:"Location of headquarters"`
	MarketCap        float64          `json:"market_cap" description:"Market capitalization in billions USD"`
	Employees        int              `json:"employees" description:"Number of employees"`
	BusinessModel    string           `json:"business_model" description:"Primary business model"`
	KeyProducts      []string         `json:"key_products" description:"Main products or services"`
	Competitors      []string         `json:"competitors" description:"Major competitors"`
	Strengths        []string         `json:"strengths" description:"Company strengths"`
	Challenges       []string         `json:"challenges" description:"Current challenges or risks"`
	FinancialHealth  string           `json:"financial_health" description:"Overall financial health assessment"`
	GrowthProspects  string           `json:"growth_prospects" description:"Future growth outlook"`
	ESGScore         float64          `json:"esg_score" description:"Environmental, Social, Governance score (1-10)"`
	FinancialMetrics FinancialMetrics `json:"financial_metrics" description:"Detailed financial metrics"`
	MarketPosition   MarketPosition   `json:"market_position" description:"Market position analysis"`
	Innovation       Innovation       `json:"innovation" description:"Innovation and R&D information"`
}

// SystemRequirements represents detailed system requirements
type SystemRequirements struct {
	MinCPU           string  `json:"min_cpu" description:"Minimum CPU requirements"`
	MinRAM           string  `json:"min_ram" description:"Minimum RAM requirements"`
	MinStorage       string  `json:"min_storage" description:"Minimum storage requirements"`
	RecommendedCPU   string  `json:"recommended_cpu" description:"Recommended CPU specifications"`
	RecommendedRAM   string  `json:"recommended_ram" description:"Recommended RAM specifications"`
	RecommendedStorage string `json:"recommended_storage" description:"Recommended storage specifications"`
	NetworkBandwidth string  `json:"network_bandwidth" description:"Required network bandwidth"`
	SupportedOS      []string `json:"supported_os" description:"Supported operating systems"`
}

// PerformanceMetrics represents detailed performance information
type PerformanceMetrics struct {
	MaxThroughput    int     `json:"max_throughput" description:"Maximum throughput (requests/second)"`
	AverageLatency   float64 `json:"average_latency" description:"Average response latency in milliseconds"`
	MaxConcurrentUsers int   `json:"max_concurrent_users" description:"Maximum concurrent users supported"`
	MemoryUsage      string  `json:"memory_usage" description:"Typical memory usage"`
	CPUUtilization   float64 `json:"cpu_utilization" description:"Average CPU utilization percentage"`
	StorageIOPS      int     `json:"storage_iops" description:"Storage IOPS requirements"`
	NetworkThroughput string `json:"network_throughput" description:"Network throughput capabilities"`
}

// SecurityConfiguration represents security settings and features
type SecurityConfiguration struct {
	AuthenticationMethods []string `json:"authentication_methods" description:"Supported authentication methods"`
	EncryptionStandards   []string `json:"encryption_standards" description:"Supported encryption standards"`
	ComplianceStandards   []string `json:"compliance_standards" description:"Compliance certifications"`
	AccessControls        []string `json:"access_controls" description:"Access control mechanisms"`
	AuditingFeatures      []string `json:"auditing_features" description:"Auditing and logging features"`
	VulnerabilityScannung bool     `json:"vulnerability_scanning" description:"Built-in vulnerability scanning"`
	PenetrationTested     bool     `json:"penetration_tested" description:"Whether penetration tested"`
}

// TechnicalSpec represents structured technical specifications
type TechnicalSpec struct {
	ProductName     string                  `json:"product_name" description:"Name of the technical product"`
	Category        string                  `json:"category" description:"Product category"`
	Version         string                  `json:"version" description:"Product version"`
	Architecture    string                  `json:"architecture" description:"Technical architecture"`
	Requirements    SystemRequirements      `json:"requirements" description:"System requirements"`
	Features        []string                `json:"features" description:"Key technical features"`
	Performance     PerformanceMetrics      `json:"performance" description:"Performance metrics"`
	Compatibility   []string                `json:"compatibility" description:"Compatibility information"`
	SecurityLevel   string                  `json:"security_level" description:"Security classification level"`
	Security        SecurityConfiguration   `json:"security" description:"Security configuration and features"`
	Documentation   bool                    `json:"documentation" description:"Whether comprehensive documentation exists"`
	SupportLevel    string                  `json:"support_level" description:"Level of support available"`
	Scalability     string                  `json:"scalability" description:"Scalability characteristics"`
}

func main() {
	// Get required environment variables
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		fmt.Println("GOOGLE_CLOUD_PROJECT environment variable is required")
		fmt.Println("Please set it with: export GOOGLE_CLOUD_PROJECT=your-project-id")
		os.Exit(1)
	}

	region := os.Getenv("VERTEX_AI_REGION")
	if region == "" {
		region = os.Getenv("GOOGLE_CLOUD_REGION")
		if region == "" {
			region = "us-east5" // Default region
			fmt.Printf("Using default region: %s\n", region)
		}
	}

	// Check if credentials are set up
	credentialsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if credentialsPath == "" {
		fmt.Println("Warning: GOOGLE_APPLICATION_CREDENTIALS not set")
		fmt.Println("Make sure you have run: gcloud auth application-default login")
		fmt.Println("Or set GOOGLE_APPLICATION_CREDENTIALS to your service account key file")
	}

	// Create context
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "vertex-ai-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "vertex-structured-output-demo")

	fmt.Println("Anthropic Vertex AI Structured Output Examples")
	fmt.Println("===============================================")
	fmt.Printf("Project ID: %s\n", projectID)
	fmt.Printf("Region: %s\n", region)
	fmt.Println()

	// Example 1: Research Paper Analysis
	fmt.Println("Example 1: Research Paper Analysis")
	fmt.Println("----------------------------------")
	researchPaperExample(ctx, projectID, region)
	fmt.Println()

	// Example 2: Company Analysis
	fmt.Println("Example 2: Company Analysis")
	fmt.Println("---------------------------")
	companyAnalysisExample(ctx, projectID, region)
	fmt.Println()

	// Example 3: Technical Specification
	fmt.Println("Example 3: Technical Specification")
	fmt.Println("----------------------------------")
	technicalSpecExample(ctx, projectID, region)
	fmt.Println()

	// Example 4: Using with Agent
	fmt.Println("Example 4: Using Structured Output with Vertex AI Agent")
	fmt.Println("-------------------------------------------------------")
	vertexAgentStructuredOutputExample(ctx, projectID, region)
}

func researchPaperExample(ctx context.Context, projectID, region string) {
	// Create Anthropic client configured for Vertex AI
	client := anthropic.NewClient(
		"", // No API key needed for Vertex AI
		anthropic.WithModel("claude-sonnet-4@20250514"),
		anthropic.WithVertexAI(region, projectID),
	)

	// Create response format for research paper
	responseFormat := structuredoutput.NewResponseFormat(ResearchPaper{})

	// Generate structured research paper analysis
	response, err := client.Generate(
		ctx,
		`Analyze this research paper: "Attention Is All You Need" by Vaswani et al., published in 2017. This paper introduced the Transformer architecture for neural networks.`,
		anthropic.WithResponseFormat(*responseFormat),
		anthropic.WithSystemMessage("You are a research analyst with expertise in machine learning and natural language processing. Provide detailed academic analysis."),
		anthropic.WithTemperature(0.2),
	)
	if err != nil {
		fmt.Printf("Error generating research paper analysis: %v\n", err)
		return
	}

	// Parse the JSON response
	var paper ResearchPaper
	if err := json.Unmarshal([]byte(response), &paper); err != nil {
		fmt.Printf("Error parsing research paper analysis: %v\n", err)
		fmt.Printf("Raw response: %s\n", response)
		return
	}

	// Display the structured analysis
	fmt.Printf("Title: %s\n", paper.Title)
	fmt.Printf("Authors: %v\n", paper.Authors)
	fmt.Printf("Year: %d\n", paper.Year)
	fmt.Printf("Journal: %s\n", paper.Journal)
	fmt.Printf("Rating: %.1f/10\n", paper.Rating)
	fmt.Printf("Keywords: %v\n", paper.Keywords)
	fmt.Printf("Methodology: %s\n", paper.Methodology)
	fmt.Printf("Key Findings: %v\n", paper.Findings)
	fmt.Printf("Significance: %s\n", paper.Significance)
}

func companyAnalysisExample(ctx context.Context, projectID, region string) {
	// Create Anthropic client configured for Vertex AI
	client := anthropic.NewClient(
		"", // No API key needed for Vertex AI
		anthropic.WithModel("claude-sonnet-4@20250514"),
		anthropic.WithVertexAI(region, projectID),
	)

	// Create response format for company analysis
	responseFormat := structuredoutput.NewResponseFormat(CompanyAnalysis{})

	// Generate structured company analysis
	response, err := client.Generate(
		ctx,
		"Provide a comprehensive analysis of NVIDIA Corporation, focusing on their AI and semiconductor business.",
		anthropic.WithResponseFormat(*responseFormat),
		anthropic.WithSystemMessage("You are a financial analyst specializing in technology companies. Provide detailed business analysis with realistic financial estimates."),
		anthropic.WithTemperature(0.3),
	)
	if err != nil {
		fmt.Printf("Error generating company analysis: %v\n", err)
		return
	}

	// Parse the JSON response
	var analysis CompanyAnalysis
	if err := json.Unmarshal([]byte(response), &analysis); err != nil {
		fmt.Printf("Error parsing company analysis: %v\n", err)
		fmt.Printf("Raw response: %s\n", response)
		return
	}

	// Display the company analysis
	fmt.Printf("Company: %s\n", analysis.CompanyName)
	fmt.Printf("Industry: %s\n", analysis.Industry)
	fmt.Printf("Founded: %d\n", analysis.Founded)
	fmt.Printf("Headquarters: %s\n", analysis.Headquarters)
	fmt.Printf("Market Cap: $%.1fB\n", analysis.MarketCap)
	fmt.Printf("Employees: %d\n", analysis.Employees)
	fmt.Printf("Business Model: %s\n", analysis.BusinessModel)
	fmt.Printf("Key Products: %v\n", analysis.KeyProducts)
	fmt.Printf("Competitors: %v\n", analysis.Competitors)
	fmt.Printf("Strengths: %v\n", analysis.Strengths)
	fmt.Printf("Challenges: %v\n", analysis.Challenges)
	fmt.Printf("Financial Health: %s\n", analysis.FinancialHealth)
	fmt.Printf("Growth Prospects: %s\n", analysis.GrowthProspects)
	fmt.Printf("ESG Score: %.1f/10\n", analysis.ESGScore)

	// Display nested financial metrics
	fmt.Printf("\nFinancial Metrics:\n")
	fmt.Printf("  Revenue: $%.1fB\n", analysis.FinancialMetrics.Revenue)
	fmt.Printf("  Net Income: $%.1fB\n", analysis.FinancialMetrics.NetIncome)
	fmt.Printf("  Gross Margin: %.1f%%\n", analysis.FinancialMetrics.GrossMargin)
	fmt.Printf("  Operating Margin: %.1f%%\n", analysis.FinancialMetrics.OperatingMargin)
	fmt.Printf("  ROE: %.1f%%\n", analysis.FinancialMetrics.ReturnOnEquity)
	fmt.Printf("  Free Cash Flow: $%.1fB\n", analysis.FinancialMetrics.FreeCashFlow)

	// Display nested market position
	fmt.Printf("\nMarket Position:\n")
	fmt.Printf("  Market Share: %.1f%%\n", analysis.MarketPosition.MarketShare)
	fmt.Printf("  Market Size: $%.1fB\n", analysis.MarketPosition.MarketSize)
	fmt.Printf("  Growth Rate: %.1f%%\n", analysis.MarketPosition.GrowthRate)
	fmt.Printf("  Competitor Rank: #%d\n", analysis.MarketPosition.CompetitorRank)
	fmt.Printf("  Geographic Reach: %v\n", analysis.MarketPosition.GeographicReach)
	fmt.Printf("  Customer Segments: %v\n", analysis.MarketPosition.CustomerSegments)

	// Display nested innovation metrics
	fmt.Printf("\nInnovation & R&D:\n")
	fmt.Printf("  R&D Spending: $%.1fB\n", analysis.Innovation.RAndDSpending)
	fmt.Printf("  Patent Count: %d\n", analysis.Innovation.PatentCount)
	fmt.Printf("  Recent Innovations: %v\n", analysis.Innovation.RecentBrevets)
	fmt.Printf("  Technology Focus: %v\n", analysis.Innovation.TechnologyFocus)
	fmt.Printf("  Strategic Partnerships: %d\n", analysis.Innovation.PartnershipsCount)
}

func technicalSpecExample(ctx context.Context, projectID, region string) {
	// Create Anthropic client configured for Vertex AI
	client := anthropic.NewClient(
		"", // No API key needed for Vertex AI
		anthropic.WithModel("claude-sonnet-4@20250514"),
		anthropic.WithVertexAI(region, projectID),
	)

	// Create response format for technical specification
	responseFormat := structuredoutput.NewResponseFormat(TechnicalSpec{})

	// Generate structured technical specification
	response, err := client.Generate(
		ctx,
		"Create technical specifications for Kubernetes v1.28, focusing on container orchestration capabilities and system requirements.",
		anthropic.WithResponseFormat(*responseFormat),
		anthropic.WithSystemMessage("You are a technical architect specializing in cloud-native technologies. Provide detailed technical specifications."),
		anthropic.WithTemperature(0.1),
	)
	if err != nil {
		fmt.Printf("Error generating technical specification: %v\n", err)
		return
	}

	// Parse the JSON response
	var spec TechnicalSpec
	if err := json.Unmarshal([]byte(response), &spec); err != nil {
		fmt.Printf("Error parsing technical specification: %v\n", err)
		fmt.Printf("Raw response: %s\n", response)
		return
	}

	// Display the technical specification
	fmt.Printf("Product: %s\n", spec.ProductName)
	fmt.Printf("Category: %s\n", spec.Category)
	fmt.Printf("Version: %s\n", spec.Version)
	fmt.Printf("Architecture: %s\n", spec.Architecture)
	fmt.Printf("Security Level: %s\n", spec.SecurityLevel)
	fmt.Printf("Support Level: %s\n", spec.SupportLevel)
	fmt.Printf("Scalability: %s\n", spec.Scalability)
	fmt.Printf("Documentation Available: %v\n", spec.Documentation)
	fmt.Printf("Key Features: %v\n", spec.Features)
	fmt.Printf("Compatibility: %v\n", spec.Compatibility)

	// Display nested system requirements
	fmt.Printf("\nSystem Requirements:\n")
	fmt.Printf("  Minimum CPU: %s\n", spec.Requirements.MinCPU)
	fmt.Printf("  Minimum RAM: %s\n", spec.Requirements.MinRAM)
	fmt.Printf("  Minimum Storage: %s\n", spec.Requirements.MinStorage)
	fmt.Printf("  Recommended CPU: %s\n", spec.Requirements.RecommendedCPU)
	fmt.Printf("  Recommended RAM: %s\n", spec.Requirements.RecommendedRAM)
	fmt.Printf("  Recommended Storage: %s\n", spec.Requirements.RecommendedStorage)
	fmt.Printf("  Network Bandwidth: %s\n", spec.Requirements.NetworkBandwidth)
	fmt.Printf("  Supported OS: %v\n", spec.Requirements.SupportedOS)

	// Display nested performance metrics
	fmt.Printf("\nPerformance Metrics:\n")
	fmt.Printf("  Max Throughput: %d req/sec\n", spec.Performance.MaxThroughput)
	fmt.Printf("  Average Latency: %.2f ms\n", spec.Performance.AverageLatency)
	fmt.Printf("  Max Concurrent Users: %d\n", spec.Performance.MaxConcurrentUsers)
	fmt.Printf("  Memory Usage: %s\n", spec.Performance.MemoryUsage)
	fmt.Printf("  CPU Utilization: %.1f%%\n", spec.Performance.CPUUtilization)
	fmt.Printf("  Storage IOPS: %d\n", spec.Performance.StorageIOPS)
	fmt.Printf("  Network Throughput: %s\n", spec.Performance.NetworkThroughput)

	// Display nested security configuration
	fmt.Printf("\nSecurity Configuration:\n")
	fmt.Printf("  Authentication Methods: %v\n", spec.Security.AuthenticationMethods)
	fmt.Printf("  Encryption Standards: %v\n", spec.Security.EncryptionStandards)
	fmt.Printf("  Compliance Standards: %v\n", spec.Security.ComplianceStandards)
	fmt.Printf("  Access Controls: %v\n", spec.Security.AccessControls)
	fmt.Printf("  Auditing Features: %v\n", spec.Security.AuditingFeatures)
	fmt.Printf("  Vulnerability Scanning: %v\n", spec.Security.VulnerabilityScannung)
	fmt.Printf("  Penetration Tested: %v\n", spec.Security.PenetrationTested)
}

func vertexAgentStructuredOutputExample(ctx context.Context, projectID, region string) {
	// Create Anthropic client configured for Vertex AI
	client := anthropic.NewClient(
		"", // No API key needed for Vertex AI
		anthropic.WithModel("claude-sonnet-4@20250514"),
		anthropic.WithVertexAI(region, projectID),
	)

	// Create response format for research paper (reusing the struct)
	responseFormat := structuredoutput.NewResponseFormat(ResearchPaper{})

	// Create an agent with structured output
	agentInstance, err := agent.NewAgent(
		agent.WithLLM(client),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You are an AI research expert who analyzes academic papers and provides structured insights."),
		agent.WithResponseFormat(*responseFormat),
	)
	if err != nil {
		fmt.Printf("Failed to create Vertex AI agent: %v\n", err)
		return
	}

	// Run the agent with a research paper analysis request
	response, err := agentInstance.Run(ctx, `Analyze the paper "BERT: Pre-training of Deep Bidirectional Transformers for Language Understanding" by Devlin et al. from 2018.`)
	if err != nil {
		fmt.Printf("Failed to run Vertex AI agent: %v\n", err)
		return
	}

	// Parse the JSON response
	var paper ResearchPaper
	if err := json.Unmarshal([]byte(response), &paper); err != nil {
		fmt.Printf("Error parsing agent response: %v\n", err)
		fmt.Printf("Raw response: %s\n", response)
		return
	}

	// Display the structured analysis from agent
	fmt.Printf("Paper: %s (%d)\n", paper.Title, paper.Year)
	fmt.Printf("Authors: %v\n", paper.Authors)
	fmt.Printf("Journal: %s\n", paper.Journal)
	fmt.Printf("Rating: %.1f/10\n", paper.Rating)
	fmt.Printf("Abstract: %s\n", paper.Abstract)
	fmt.Printf("Methodology: %s\n", paper.Methodology)
	fmt.Printf("Key Findings: %v\n", paper.Findings)
	fmt.Printf("Significance: %s\n", paper.Significance)
}