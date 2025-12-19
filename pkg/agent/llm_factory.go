package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/azureopenai"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/deepseek"
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
	case "deepseek":
		return createDeepSeekClient(config)
	case "gemini":
		return createGeminiClient(config)
	case "ollama":
		return createOllamaClient(config)
	case "vllm":
		return createVllmClient(config)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s (supported: anthropic, openai, azureopenai, deepseek, gemini, ollama, vllm)", provider)
	}
}

// parseGoogleCredentials parses Google Application Credentials from multiple formats:
// - File path: reads the file content
// - Base64 encoded: decodes the base64 string
// - Raw JSON: uses the content directly
func parseGoogleCredentials(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("empty credentials input")
	}

	// 1. Check if it's a file path
	if _, err := os.Stat(input); err == nil {
		data, err := os.ReadFile(input)
		if err != nil {
			return "", fmt.Errorf("failed to read credentials file: %w", err)
		}
		// Validate it's valid JSON
		if !json.Valid(data) {
			return "", fmt.Errorf("credentials file does not contain valid JSON")
		}
		return string(data), nil
	}

	// 2. Check if it's base64 encoded
	if decoded, err := base64.StdEncoding.DecodeString(input); err == nil {
		// Validate it's valid JSON
		if json.Valid(decoded) {
			return string(decoded), nil
		}
	}

	// 3. Treat as raw JSON content
	if json.Valid([]byte(input)) {
		return input, nil
	}

	return "", fmt.Errorf("invalid credentials format: not a valid file path, base64-encoded JSON, or raw JSON content")
}

// createAnthropicClient creates an Anthropic LLM client
func createAnthropicClient(config *LLMProviderYAML) (interfaces.LLM, error) {
	var options []anthropic.Option
	var apiKey string

	// Check for Vertex AI configuration first (preferred method)
	vertexProject := getConfigString(config.Config, "vertex_ai_project")
	if vertexProject == "" {
		vertexProject = GetEnvValue("VERTEX_AI_PROJECT")
	}

	// Use Vertex AI if configured
	if vertexProject != "" {
		// Using Vertex AI as primary authentication method
		fmt.Printf("DEBUG: Using Vertex AI - Project: %s\n", vertexProject)

		// Check for both vertex_ai_region and vertex_ai_location for backward compatibility
		location := getConfigString(config.Config, "vertex_ai_region")
		if location == "" {
			location = getConfigString(config.Config, "vertex_ai_location")
		}
		if location == "" {
			location = GetEnvValue("VERTEX_AI_REGION")
		}
		if location == "" {
			location = GetEnvValue("VERTEX_AI_LOCATION")
		}
		if location == "" {
			location = "us-central1" // Default location
		}
		fmt.Printf("DEBUG: Vertex AI Location: %s\n", location)

		// Check if explicit credentials are provided
		if creds := getConfigString(config.Config, "google_application_credentials"); creds != "" {
			// Parse credentials - could be file path, base64, or raw JSON
			credContent, err := parseGoogleCredentials(creds)
			if err != nil {
				return nil, fmt.Errorf("failed to parse google_application_credentials: %w", err)
			}
			options = append(options, anthropic.WithGoogleApplicationCredentials(location, vertexProject, credContent))
			fmt.Printf("DEBUG: Using explicit Google Application Credentials\n")
		} else {
			// Use default ADC
			options = append(options, anthropic.WithVertexAI(location, vertexProject))
			fmt.Printf("DEBUG: Using default Application Default Credentials (ADC)\n")
		}

		// Use placeholder API key for Vertex AI
		apiKey = "vertex-ai"
	} else {
		// Fallback to Anthropic API with API key
		apiKey = getConfigString(config.Config, "api_key")
		if apiKey == "" {
			apiKey = GetEnvValue("ANTHROPIC_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("api_key is required for Anthropic provider (set ANTHROPIC_API_KEY) or configure Vertex AI (set VERTEX_AI_PROJECT and VERTEX_AI_REGION)")
		}
		fmt.Printf("DEBUG: Using Anthropic API with key: %s...\n", apiKey[:min(10, len(apiKey))])
	}

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

// createDeepSeekClient creates a DeepSeek LLM client
func createDeepSeekClient(config *LLMProviderYAML) (interfaces.LLM, error) {
	var options []deepseek.Option

	// Get API key from config or environment
	apiKey := getConfigString(config.Config, "api_key")
	if apiKey == "" {
		// Fallback to DEEPSEEK_API_KEY environment variable
		apiKey = GetEnvValue("DEEPSEEK_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("api_key is required for DeepSeek provider (set DEEPSEEK_API_KEY or provide in config)")
	}

	// Set model - use config model or fallback to DEEPSEEK_MODEL env var
	model := ExpandEnv(config.Model)
	if model == "" {
		model = getConfigString(config.Config, "model")
	}
	if model == "" {
		model = GetEnvValue("DEEPSEEK_MODEL")
	}
	if model != "" {
		options = append(options, deepseek.WithModel(model))
	}

	// Set base URL if provided (for custom endpoints)
	if baseURL := getConfigString(config.Config, "base_url"); baseURL != "" {
		options = append(options, deepseek.WithBaseURL(baseURL))
	}

	return deepseek.NewClient(apiKey, options...), nil
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