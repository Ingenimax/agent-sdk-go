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
	toolsregistry "github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/github"
)

type GitHubResponse struct {
	UserResponse string   `json:"user_response" description:"The response to the user's query"`
	FileNames    []string `json:"file_names" description:"List of file names found in the repository"`
}

func main() {
	// Create a logger
	logger := logging.New()

	// Get configuration
	cfg := config.Get()

	// Create a new agent with OpenAI
	openaiClient := openai.NewClient(cfg.LLM.OpenAI.APIKey,
		openai.WithLogger(logger))

	// Create tools registry
	toolRegistry := toolsregistry.NewRegistry()

	// Add GitHub content extractor tool
	githubTool, err := github.NewGitHubContentExtractorTool(cfg.Tools.GitHub.Token)
	if err != nil {
		logger.Error(context.Background(), "Failed to create GitHub tool", map[string]interface{}{"error": err.Error()})
		return
	}
	toolRegistry.Register(githubTool)

	responseFormat := structuredoutput.NewResponseFormat(GitHubResponse{})

	// Create the agent with the tools
	agent, err := agent.NewAgent(
		agent.WithLLM(openaiClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(toolRegistry.List()...),
		agent.WithRequirePlanApproval(false),
		agent.WithSystemPrompt(`
		You are a helpful AI assistant that can help users extract and analyze content from GitHub repositories.
		When users ask about repository contents, use the GitHub content extractor tool to find relevant information.
		You should always use the GitHub content extractor tool to find relevant information.
		
		When searching for Terraform files, use these patterns: ['.tf', '.tfvars', '.hcl']
		`),
		agent.WithName("GitHubAssistant"),
		agent.WithResponseFormat(*responseFormat),
	)
	if err != nil {
		logger.Error(context.Background(), "Failed to create agent", map[string]interface{}{"error": err.Error()})
		return
	}

	// Log created agent
	logger.Info(context.Background(), "Created agent with tools", map[string]interface{}{"tools": toolRegistry.List()})

	// Create a context with organization ID and conversation ID
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "default-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "conversation-123")

	// Example queries to test the agent with different parameters
	queries := []string{
		// "List all Terraform-related files in the repository https://github.com/terraform-aws-modules/terraform-aws-vpc",
		// "Find the first 5 Terraform files in https://github.com/terraform-aws-modules/terraform-aws-vpc",
		// "List Terraform files up to 2 directory levels deep in https://github.com/terraform-aws-modules/terraform-aws-vpc",
		// "Find Terraform files smaller than 1MB in https://github.com/terraform-aws-modules/terraform-aws-vpc",
		"Extract specific files main.tf and variables.tf from https://github.com/terraform-aws-modules/terraform-aws-vpc",
	}

	for _, query := range queries {
		fmt.Printf("\nQuery: %s\n", query)
		response, err := agent.Run(ctx, query)
		if err != nil {
			logger.Error(ctx, "Failed to run agent", map[string]interface{}{"error": err.Error()})
			continue
		}

		var githubResponse GitHubResponse
		err = json.Unmarshal([]byte(response), &githubResponse)
		if err != nil {
			logger.Error(ctx, "Failed to unmarshal response", map[string]interface{}{
				"error":    err.Error(),
				"response": response,
			})
			continue
		}

		// Print markdown response with line breaks and formatting
		lines := strings.Split(githubResponse.UserResponse, "\n")
		for _, line := range lines {
			fmt.Printf("Response: %s\n", strings.TrimSpace(line))
		}
		fmt.Printf("\nFound %d files:\n", len(githubResponse.FileNames))
		for _, fileName := range githubResponse.FileNames {
			fmt.Printf("- %s\n", fileName)
		}
		fmt.Println("\n" + strings.Repeat("-", 80))
	}
}
