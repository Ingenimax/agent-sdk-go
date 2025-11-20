package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/azureopenai"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/ollama"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/vllm"
)

// createLLMFromConfig creates an LLM client from YAML configuration
func createLLMFromConfig(config *LLMProviderYAML) (interfaces.LLM, error) {
	if config == nil || config.Provider == "" {
		return nil, fmt.Errorf("LLM provider configuration is required")
	}

	provider := strings.ToLower(config.Provider)
	fmt.Printf("DEBUG: Creating LLM from config - Provider: %s, Model: %s\n", provider, config.Model)

	switch provider {
	case "anthropic":
		return createAnthropicClient(config)
	case "openai":
		return createOpenAIClient(config)
	case "azureopenai", "azure_openai":
		return createAzureOpenAIClient(config)
	case "gemini":
		return createGeminiClient(config)
	case "ollama":
		return createOllamaClient(config)
	case "vllm":
		return createVllmClient(config)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s (supported: anthropic, openai, azureopenai, gemini, ollama, vllm)", provider)
	}
}

// createAnthropicClient creates an Anthropic LLM client
func createAnthropicClient(config *LLMProviderYAML) (interfaces.LLM, error) {
	var options []anthropic.Option

	// Get API key from config or environment
	apiKey := getConfigString(config.Config, "api_key")
	if apiKey == "" {
		// Fallback to ANTHROPIC_API_KEY environment variable
		apiKey = GetEnvValue("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		fmt.Printf("DEBUG: No API key found in config or environment\n")
		return nil, fmt.Errorf("api_key is required for Anthropic provider (set ANTHROPIC_API_KEY or provide in config)")
	}
	fmt.Printf("DEBUG: Found API key: %s...\n", apiKey[:10])

	// Set model - use config model or fallback to ANTHROPIC_MODEL env var
	model := ExpandEnv(config.Model)
	if model == "" {
		model = getConfigString(config.Config, "model")
	}
	if model == "" {
		model = GetEnvValue("ANTHROPIC_MODEL")
	}
	if model != "" {
		options = append(options, anthropic.WithModel(model))
	}

	// Set base URL if provided (for custom endpoints)
	if baseURL := getConfigString(config.Config, "base_url"); baseURL != "" {
		options = append(options, anthropic.WithBaseURL(baseURL))
	}

	// Vertex AI configuration
	if vertexProject := getConfigString(config.Config, "vertex_ai_project"); vertexProject != "" {
		// Check for both vertex_ai_region and vertex_ai_location for backward compatibility
		location := getConfigString(config.Config, "vertex_ai_region")
		if location == "" {
			location = getConfigString(config.Config, "vertex_ai_location")
		}
		if location == "" {
			location = "us-central1" // Default location
		}

		// Check if explicit credentials are provided
		if creds := getConfigString(config.Config, "google_application_credentials"); creds != "" {
			// Use credentials content directly
			options = append(options, anthropic.WithGoogleApplicationCredentials(location, vertexProject, creds))
		} else {
			// Use default ADC
			options = append(options, anthropic.WithVertexAI(location, vertexProject))
		}
	}

	return anthropic.NewClient(apiKey, options...), nil
}

// createOpenAIClient creates an OpenAI LLM client
func createOpenAIClient(config *LLMProviderYAML) (interfaces.LLM, error) {
	var options []openai.Option

	// Get API key from config or environment
	apiKey := getConfigString(config.Config, "api_key")
	if apiKey == "" {
		// Fallback to OPENAI_API_KEY environment variable
		apiKey = GetEnvValue("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("api_key is required for OpenAI provider (set OPENAI_API_KEY or provide in config)")
	}

	// Set model - use config model or fallback to OPENAI_MODEL env var
	model := ExpandEnv(config.Model)
	if model == "" {
		model = getConfigString(config.Config, "model")
	}
	if model == "" {
		model = GetEnvValue("OPENAI_MODEL")
	}
	if model != "" {
		options = append(options, openai.WithModel(model))
	}

	// Set base URL if provided (for custom endpoints)
	if baseURL := getConfigString(config.Config, "base_url"); baseURL != "" {
		options = append(options, openai.WithBaseURL(baseURL))
	}

	return openai.NewClient(apiKey, options...), nil
}

// createAzureOpenAIClient creates an Azure OpenAI LLM client
func createAzureOpenAIClient(config *LLMProviderYAML) (interfaces.LLM, error) {
	var options []azureopenai.Option

	// Get API key from config or environment
	apiKey := getConfigString(config.Config, "api_key")
	if apiKey == "" {
		apiKey = GetEnvValue("AZURE_OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("api_key is required for Azure OpenAI provider (set AZURE_OPENAI_API_KEY or provide in config)")
	}

	// Get required endpoint
	endpoint := getConfigString(config.Config, "endpoint")
	if endpoint == "" {
		endpoint = GetEnvValue("AZURE_OPENAI_ENDPOINT")
	}
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint is required for Azure OpenAI provider (set AZURE_OPENAI_ENDPOINT or provide in config)")
	}

	// Get required deployment name
	deployment := getConfigString(config.Config, "deployment")
	if deployment == "" {
		deployment = ExpandEnv(config.Model)
	}
	if deployment == "" {
		deployment = GetEnvValue("AZURE_OPENAI_DEPLOYMENT")
	}
	if deployment == "" {
		return nil, fmt.Errorf("deployment is required for Azure OpenAI provider (set AZURE_OPENAI_DEPLOYMENT or provide in config)")
	}

	// Get API version
	apiVersion := getConfigString(config.Config, "api_version")
	if apiVersion == "" {
		apiVersion = GetEnvValue("AZURE_OPENAI_API_VERSION")
	}
	if apiVersion == "" {
		apiVersion = "2024-02-01" // Default API version
	}

	options = append(options, azureopenai.WithDeployment(deployment))
	options = append(options, azureopenai.WithAPIVersion(apiVersion))

	return azureopenai.NewClient(apiKey, endpoint, deployment, options...), nil
}

// createGeminiClient creates a Google Gemini LLM client
func createGeminiClient(config *LLMProviderYAML) (interfaces.LLM, error) {
	var options []gemini.Option

	// Get API key from config or environment
	apiKey := getConfigString(config.Config, "api_key")
	if apiKey == "" {
		apiKey = GetEnvValue("GEMINI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("api_key is required for Gemini provider (set GEMINI_API_KEY or provide in config)")
	}

	// Set API key
	options = append(options, gemini.WithAPIKey(apiKey))

	// Set model - use config model or fallback to GEMINI_MODEL env var
	model := ExpandEnv(config.Model)
	if model == "" {
		model = getConfigString(config.Config, "model")
	}
	if model == "" {
		model = GetEnvValue("GEMINI_MODEL")
	}
	if model != "" {
		options = append(options, gemini.WithModel(model))
	}

	// Set project for Vertex AI if provided
	if project := getConfigString(config.Config, "project"); project != "" {
		options = append(options, gemini.WithProjectID(project))
	}

	// Set location for Vertex AI if provided
	if location := getConfigString(config.Config, "location"); location != "" {
		options = append(options, gemini.WithLocation(location))
	}

	// Create context for client initialization
	ctx := context.Background()
	client, err := gemini.NewClient(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return client, nil
}

// createOllamaClient creates an Ollama LLM client
func createOllamaClient(config *LLMProviderYAML) (interfaces.LLM, error) {
	var options []ollama.Option

	// Get base URL from config or environment, default to localhost
	baseURL := getConfigString(config.Config, "base_url")
	if baseURL == "" {
		baseURL = GetEnvValue("OLLAMA_BASE_URL")
	}
	if baseURL == "" {
		baseURL = "http://localhost:11434" // Default Ollama URL
	}

	// Set base URL
	options = append(options, ollama.WithBaseURL(baseURL))

	// Set model - use config model or fallback to OLLAMA_MODEL env var
	model := ExpandEnv(config.Model)
	if model == "" {
		model = getConfigString(config.Config, "model")
	}
	if model == "" {
		model = GetEnvValue("OLLAMA_MODEL")
	}
	if model != "" {
		options = append(options, ollama.WithModel(model))
	}

	return ollama.NewClient(options...), nil
}

// createVllmClient creates a vLLM LLM client
func createVllmClient(config *LLMProviderYAML) (interfaces.LLM, error) {
	var options []vllm.Option

	// Get base URL from config or environment
	baseURL := getConfigString(config.Config, "base_url")
	if baseURL == "" {
		baseURL = GetEnvValue("VLLM_BASE_URL")
	}
	if baseURL == "" {
		return nil, fmt.Errorf("base_url is required for vLLM provider (set VLLM_BASE_URL or provide in config)")
	}

	// Set base URL
	options = append(options, vllm.WithBaseURL(baseURL))

	// Set model - use config model or fallback to VLLM_MODEL env var
	model := ExpandEnv(config.Model)
	if model == "" {
		model = getConfigString(config.Config, "model")
	}
	if model == "" {
		model = GetEnvValue("VLLM_MODEL")
	}
	if model != "" {
		options = append(options, vllm.WithModel(model))
	}

	return vllm.NewClient(options...), nil
}

// Helper function to extract string values from config map
func getConfigString(config map[string]interface{}, key string) string {
	if config == nil {
		return ""
	}
	if value, exists := config[key]; exists {
		if str, ok := value.(string); ok {
			// Expand environment variables using SDK's ExpandEnv that supports .env files
			return ExpandEnv(str)
		}
	}
	return ""
}