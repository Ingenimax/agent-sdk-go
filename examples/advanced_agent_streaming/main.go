package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/examples/microservices/shared"
	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// ANSI color codes for minimal terminal output
const (
	ColorReset = "\033[0m"
	ColorGray  = "\033[90m" // Dark gray for thinking only
)

// MockDataTool provides sample market data for testing
type MockDataTool struct{}

func (t *MockDataTool) Name() string {
	return "market_data_lookup"
}

func (t *MockDataTool) DisplayName() string {
	return "Market Data Lookup"
}

func (t *MockDataTool) Description() string {
	return "Lookup specific market data and statistics for analysis"
}

func (t *MockDataTool) Internal() bool {
	return false
}

func (t *MockDataTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: "The type of market data to lookup (e.g., 'growth_rates', 'market_size', 'segments', 'regions')",
			Required:    true,
		},
	}
}

func (t *MockDataTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *MockDataTool) Execute(ctx context.Context, args string) (string, error) {
	// Parse query from args (simple extraction for demo)
	query := "market_size" // default
	if args != "" {
		// Simple extraction - look for known query types
		if contains(args, "growth") {
			query = "growth_rates"
		} else if contains(args, "segment") {
			query = "segments"
		} else if contains(args, "region") {
			query = "regions"
		}
	}

	// Mock responses based on query type
	mockData := map[string]string{
		"growth_rates": "Market Growth Rates:\n- E-commerce: 16.5% YoY\n- Mobile commerce: 23.2% YoY\n- B2B e-commerce: 18.7% YoY\n- Social commerce: 34.1% YoY",
		"market_size":  "Market Size Data:\n- Global e-commerce: $6.2T (2024)\n- Projected Q1 2025: $6.8T\n- Mobile share: 72%\n- Desktop share: 28%",
		"segments":     "Top Market Segments:\n1. Fashion & Apparel: 23%\n2. Electronics: 18%\n3. Food & Beverages: 14%\n4. Health & Beauty: 12%\n5. Home & Garden: 9%",
		"regions":      "Regional Performance:\n- Asia-Pacific: 28% global share\n- North America: 24% global share\n- Europe: 21% global share\n- Latin America: 15% global share\n- Others: 12% global share",
	}

	result, exists := mockData[query]
	if !exists {
		result = mockData["market_size"] // fallback
	}

	return fmt.Sprintf("Market Data Results:\n%s\n\nQuery processed successfully", result), nil
}

// TrendAnalysisTool provides trend analysis with mock data
type TrendAnalysisTool struct{}

func (t *TrendAnalysisTool) Name() string {
	return "trend_analysis"
}

// DisplayName implements interfaces.ToolWithDisplayName.DisplayName
func (t *TrendAnalysisTool) DisplayName() string {
	return "Trend Analysis"
}

func (t *TrendAnalysisTool) Description() string {
	return "Analyze market trends and provide forecasting insights"
}

func (t *TrendAnalysisTool) Internal() bool {
	return false
}

func (t *TrendAnalysisTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"category": {
			Type:        "string",
			Description: "The category to analyze trends for (e.g., 'mobile', 'social', 'global')",
			Required:    true,
		},
	}
}

func (t *TrendAnalysisTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *TrendAnalysisTool) Execute(ctx context.Context, args string) (string, error) {
	// Parse category from args
	category := "global" // default
	if args != "" {
		if contains(args, "mobile") {
			category = "mobile"
		} else if contains(args, "social") {
			category = "social"
		} else if contains(args, "b2b") {
			category = "b2b"
		}
	}

	// Mock trend analysis based on category
	trendData := map[string]string{
		"global": "Global E-commerce Trends:\n- Growth accelerating in Q4 2024\n- Mobile-first shopping dominance\n- Cross-border commerce expansion\n- Q1 2025 outlook: Strong growth expected",
		"mobile": "Mobile Commerce Trends:\n- Mobile accounts for 72% of transactions\n- Mobile payments up 25% YoY\n- Social shopping integration growing\n- Fast checkout adoption increasing",
		"social": "Social Commerce Trends:\n- Instagram Shopping leads growth\n- TikTok commerce emerging rapidly\n- Influencer partnerships crucial\n- Gen Z driving social purchases",
		"b2b":    "B2B E-commerce Trends:\n- Digital transformation accelerating\n- Self-service portals in demand\n- Data-driven purchasing decisions\n- Subscription models growing",
	}

	result, exists := trendData[category]
	if !exists {
		result = trendData["global"] // fallback
	}

	return fmt.Sprintf("Trend Analysis Results:\n%s\n\nAnalysis completed for category: %s", result, category), nil
}

// Helper function to check if string contains substring
func contains(str, substr string) bool {
	return len(str) >= len(substr) && (str == substr || (len(str) > len(substr) &&
		(str[:len(substr)] == substr || str[len(str)-len(substr):] == substr ||
			indexOf(str, substr) >= 0)))
}

// Simple indexOf implementation
func indexOf(str, substr string) int {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// StreamingMetrics tracks performance metrics during streaming
type StreamingMetrics struct {
	StartTime      time.Time
	EventCount     int
	ContentLength  int
	ThinkingEvents int
	ToolCalls      int
	SubagentCalls  int
	ErrorCount     int
	LastEventTime  time.Time
}

// UpdateMetrics updates streaming metrics for an event
func (m *StreamingMetrics) UpdateMetrics(event interfaces.AgentStreamEvent) {
	m.EventCount++
	m.LastEventTime = time.Now()
	m.ContentLength += len(event.Content)

	switch event.Type {
	case interfaces.AgentEventThinking:
		m.ThinkingEvents++
	case interfaces.AgentEventToolCall:
		m.ToolCalls++
	case interfaces.AgentEventError:
		m.ErrorCount++
	}
}

// PrintMetrics displays final streaming metrics
func (m *StreamingMetrics) PrintMetrics() {
	duration := m.LastEventTime.Sub(m.StartTime)
	eventsPerSecond := float64(m.EventCount) / duration.Seconds()

	fmt.Printf("\nSTREAMING PERFORMANCE METRICS\n")
	fmt.Printf("├─ Duration: %v\n", duration)
	fmt.Printf("├─ Total Events: %d\n", m.EventCount)
	fmt.Printf("├─ Events/Second: %.1f\n", eventsPerSecond)
	fmt.Printf("├─ Content Length: %d chars\n", m.ContentLength)
	fmt.Printf("├─ Thinking Events: %d\n", m.ThinkingEvents)
	fmt.Printf("├─ Tool Calls: %d\n", m.ToolCalls)
	fmt.Printf("├─ Subagent Calls: %d\n", m.SubagentCalls)
	fmt.Printf("└─ Errors: %d\n", m.ErrorCount)
}

// This example demonstrates advanced agent streaming with 1 main agent, 2 subagents, and 2 tools
//
// Architecture:
// - 1 Main Agent: Project Manager (orchestrates the entire workflow)
// - 2 Subagents: Research Assistant & Data Analyst
// - 2 Tools: Market Data Lookup & Trend Analysis
//
// Required environment variables:
// - ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY (depending on provider)
// - LLM_PROVIDER: "anthropic", "openai", or "gemini" (optional, auto-detects based on API keys)

func main() {
	fmt.Printf("Advanced Agent Streaming: 1 Agent + 2 Subagents + 2 Tools\n")
	fmt.Printf("═══════════════════════════════════════════════════════════\n")
	fmt.Println()

	// Display LLM provider information
	fmt.Printf("Using LLM: %s\n", shared.GetProviderInfo())
	fmt.Println()

	ctx := context.Background()

	// Add required context for multi-tenancy and memory
	ctx = multitenancy.WithOrgID(ctx, "advanced-streaming-demo")
	ctx = memory.WithConversationID(ctx, "project-session")

	// Single comprehensive example
	fmt.Printf("Project Analysis with Agent Orchestration\n")
	fmt.Printf("─────────────────────────────────────────────────\n")
	if err := projectAnalysisDemo(ctx); err != nil {
		log.Printf("Example failed: %v", err)
	}
	fmt.Println()

	fmt.Printf("Advanced streaming example completed!\n")
}

// projectAnalysisDemo demonstrates 1 main agent coordinating 2 subagents with 2 tools
func projectAnalysisDemo(ctx context.Context) error {
	llm, err := shared.CreateLLM()
	if err != nil {
		return fmt.Errorf("failed to create LLM: %w", err)
	}

	// Create memory store for conversation history
	memoryStore := memory.NewConversationBuffer()

	// Create 2 tools
	tools := []interfaces.Tool{
		&MockDataTool{},      // Tool 1: Market Data Lookup
		&TrendAnalysisTool{}, // Tool 2: Trend Analysis
	}

	// Create 2 subagents (they represent the architecture but coordination happens through the main agent)
	_, err = createResearchAssistant(llm)
	if err != nil {
		return fmt.Errorf("failed to create research assistant: %w", err)
	}

	_, err = createDataAnalyst(llm)
	if err != nil {
		return fmt.Errorf("failed to create data analyst: %w", err)
	}

	// Display subagent capabilities (they are coordinated through the main agent's prompt)
	fmt.Printf("   🤖 Subagent 1: ResearchAssistant (coordinated by main agent)\n")
	fmt.Printf("   🤖 Subagent 2: DataAnalyst (coordinated by main agent)\n")

	// Create main agent (Project Manager) with tools
	projectManager, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(memoryStore),
		agent.WithTools(tools...),
		agent.WithSystemPrompt(`You are a Project Manager coordinating a market analysis project.

You have access to:
- Market data lookup tool for specific statistics and analysis
- Trend analysis tool for market forecasting and insights

You coordinate two specialized subagents:
1. Research Assistant: Gathers and organizes information
2. Data Analyst: Performs statistical analysis and projections

EXECUTION PROTOCOL:
1. Think through the approach systematically and show your reasoning process
2. IMMEDIATELY start executing your plan - use tools and gather data right away
3. Use trend analysis tool FIRST to get current market trends
4. Use market data lookup tool for statistical analysis and data points
5. Act as both Research Assistant and Data Analyst in your analysis
6. Synthesize findings into comprehensive reports with concrete numbers
7. Provide specific, actionable recommendations

IMPORTANT: After thinking, you MUST take action! Start by using the web search tool to gather current e-commerce market data. Don't just plan - execute your plan immediately.`),
		agent.WithLLMConfig(interfaces.LLMConfig{
			EnableReasoning: true,
			ReasoningBudget: 1024, // Minimum token budget for thinking (required by Anthropic)
		}),
		agent.WithName("ProjectManager"),
		agent.WithRequirePlanApproval(false), // Auto-execute tools without user approval
	)
	if err != nil {
		return fmt.Errorf("failed to create project manager: %w", err)
	}

	// Check if main agent supports streaming
	streamingAgent, ok := any(projectManager).(interfaces.StreamingAgent)
	if !ok {
		return fmt.Errorf("project manager does not support streaming")
	}

	// Configure advanced streaming
	streamConfig := interfaces.StreamConfig{
		BufferSize:          500,
		IncludeThinking:     true,
		IncludeToolProgress: true,
	}

	fmt.Printf("Starting project analysis with agent coordination...\n")
	fmt.Printf("Project: E-commerce market growth analysis for Q1 2025 business planning\n\n")

	// Display architecture (subagents already displayed above)
	fmt.Printf("Architecture:\n")
	fmt.Printf("   📊 Main Agent: Project Manager (coordinates workflow)\n")
	fmt.Printf("   🔧 Tool 1: Market Data Lookup (statistics and analysis)\n")
	fmt.Printf("   🔧 Tool 2: Trend Analysis (forecasting and insights)\n\n")

	// Start the coordinated analysis
	eventChan, err := streamingAgent.RunStream(ctx, `Execute a comprehensive e-commerce market analysis for Q1 2025 business planning:

PROJECT SCOPE:
- Analyze current e-commerce market trends and growth patterns
- Calculate market projections for Q1 2025
- Identify opportunities and risks for business expansion
- Provide actionable recommendations with supporting data

IMMEDIATE ACTION PLAN:
1. START NOW: Use trend analysis tool to find current e-commerce market trends
2. ANALYZE: Get specific growth rates and market size using market data lookup tool
3. PROJECT: Use market data to model Q1 2025 market projections
4. SYNTHESIZE: Combine all findings into a comprehensive business report
5. RECOMMEND: Provide specific actions with supporting calculations

Begin immediately by analyzing current e-commerce market trends and gathering statistics. Don't just plan - execute each step and use your tools actively throughout the analysis.`)
	if err != nil {
		return fmt.Errorf("failed to start project analysis: %w", err)
	}

	// Process streaming with enhanced visualization
	return processAdvancedAgentStream(eventChan, streamConfig, "Project Manager")
}

// processAdvancedAgentStream processes streaming events with advanced visualization and metrics
func processAdvancedAgentStream(eventChan <-chan interfaces.AgentStreamEvent, config interfaces.StreamConfig, agentName string) error {
	metrics := &StreamingMetrics{
		StartTime: time.Now(),
	}

	var inThinkingMode bool
	var currentToolCall string
	var thinkingBlockCount int
	var thinkingContent strings.Builder

	fmt.Printf("Streaming started for %s with buffer size %d\n", agentName, config.BufferSize)
	fmt.Printf("%s\n", strings.Repeat("═", 80))

	for event := range eventChan {
		metrics.UpdateMetrics(event)

		switch event.Type {
		case interfaces.AgentEventContent:
			// TODO: Remove debug logs after investigating streaming issues
			if inThinkingMode && event.Content != "" {
				// End thinking mode when we get content after thinking
				fmt.Printf("\n%s\n", strings.Repeat("─", 60))
				inThinkingMode = false
				fmt.Printf("Final response:\n")
			}
			if !inThinkingMode {
				fmt.Printf("%s", event.Content)
			}

		case interfaces.AgentEventThinking:
			if !inThinkingMode {
				inThinkingMode = true
				thinkingBlockCount++
				thinkingContent.Reset()
				fmt.Printf("\nTHINKING BLOCK #%d\n", thinkingBlockCount)
				fmt.Printf("%s\n", strings.Repeat("─", 60))
			}
			thinkingContent.WriteString(event.ThinkingStep)
			fmt.Printf("%s%s%s", ColorGray, event.ThinkingStep, ColorReset)

		case interfaces.AgentEventToolCall:
			// TODO: Remove debug logs after investigating streaming issues
			if inThinkingMode {
				// End thinking mode when we get a tool call
				fmt.Printf("\n%s\n", strings.Repeat("─", 60))
				inThinkingMode = false
			}
			if event.ToolCall != nil {
				currentToolCall = event.ToolCall.Name
				fmt.Printf("\nTOOL EXECUTION: %s\n", event.ToolCall.Name)
				fmt.Printf("├─ Arguments: %s\n", event.ToolCall.Arguments)
				fmt.Printf("├─ Status: %s\n", event.ToolCall.Status)
				if event.ToolCall.Status == "executing" {
					fmt.Printf("└─ Executing...\n")
				}
			}

		case interfaces.AgentEventToolResult:
			if event.ToolCall != nil {
				fmt.Printf("├─ Result: %s\n", event.ToolCall.Result)
				fmt.Printf("└─ Duration: %v\n", event.Metadata["execution_time"])
				currentToolCall = ""
			}

		case interfaces.AgentEventError:
			fmt.Printf("\nERROR\n")
			fmt.Printf("└─ %v\n", event.Error)
			return event.Error

		case interfaces.AgentEventComplete:
			if inThinkingMode {
				fmt.Printf("\n%s\n", strings.Repeat("─", 60))
				inThinkingMode = false
			}
			fmt.Printf("\nSTREAMING COMPLETED\n")
		default:
			fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		}

		// Show progress indicators for long-running operations
		if currentToolCall != "" && event.Type == interfaces.AgentEventContent {
			fmt.Printf("%s processing...\r", currentToolCall)
		}
	}

	// Display final metrics
	metrics.PrintMetrics()
	return nil
}

// Helper functions for creating the 2 subagents

func createResearchAssistant(llm interfaces.LLM) (*agent.Agent, error) {
	tools := []interfaces.Tool{
		&TrendAnalysisTool{}, // Research assistant has access to trend analysis
	}

	return agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithTools(tools...),
		agent.WithSystemPrompt(`You are a Research Assistant specialized in gathering and organizing information.

Your capabilities:
- Web search tool for current market data and trends
- Information synthesis and organization
- Source verification and citation

Your role:
1. Gather comprehensive, current information from reliable sources
2. Organize findings in clear, structured formats
3. Verify data accuracy and provide source citations
4. Identify key trends and patterns in collected data
5. Prepare organized data for analysis by other specialists

Always provide well-sourced, current information.`),
		agent.WithName("ResearchAssistant"),
		agent.WithRequirePlanApproval(false), // Auto-execute tools without user approval
	)
}

func createDataAnalyst(llm interfaces.LLM) (*agent.Agent, error) {
	tools := []interfaces.Tool{
		&MockDataTool{}, // Data analyst has access to market data lookup
	}

	return agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithTools(tools...),
		agent.WithSystemPrompt(`You are a Data Analyst specialized in statistical analysis and projections.

Your capabilities:
- Calculator tool for numerical computations
- Statistical analysis and modeling
- Trend analysis and forecasting

Your role:
1. Perform statistical analysis on provided data
2. Calculate growth rates, projections, and trends
3. Identify patterns and correlations in datasets
4. Create financial models and forecasts
5. Generate data-driven insights and recommendations

Use rigorous analytical methods and show your calculations clearly.`),
		agent.WithName("DataAnalyst"),
	)
}

// Utility functions
