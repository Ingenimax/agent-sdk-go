package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config represents the global configuration for the Agent SDK
type Config struct {
	// LLM configuration
	LLM struct {
		// OpenAI configuration
		OpenAI struct {
			APIKey         string
			Model          string
			Temperature    float64
			BaseURL        string
			Timeout        time.Duration
			EmbeddingModel string
		}

		// Anthropic configuration
		Anthropic struct {
			APIKey      string
			Model       string
			Temperature float64
			BaseURL     string
			Timeout     time.Duration
		}

		// Azure OpenAI configuration
		AzureOpenAI struct {
			APIKey       string
			Temperature  float64
			BaseURL      string
			Region       string
			ResourceName string
			Deployment   string
			APIVersion   string
			Timeout      time.Duration
		}
	}

	// Memory configuration
	Memory struct {
		// Redis configuration
		Redis struct {
			URL      string
			Password string
			DB       int
		}
	}

	// VectorStore configuration
	VectorStore struct {
		// Weaviate configuration
		Weaviate struct {
			URL       string
			APIKey    string
			Scheme    string
			Host      string
			ClassName string
		}
	}

	// DataStore configuration
	DataStore struct {
		// Supabase configuration
		Supabase struct {
			URL    string
			APIKey string
			Table  string
		}
	}

	// Tools configuration
	Tools struct {
		// Web search configuration
		WebSearch struct {
			GoogleAPIKey         string
			GoogleSearchEngineID string
		}
		// GitHub configuration
		GitHub struct {
			Token string
		}

		Tavily struct {
			TavilyAPIKey string
		}
	}

	// Tracing configuration
	Tracing struct {
		// Langfuse configuration
		Langfuse struct {
			Enabled     bool
			SecretKey   string
			PublicKey   string
			Host        string
			Environment string
		}

		// OpenTelemetry configuration
		OpenTelemetry struct {
			Enabled           bool
			ServiceName       string
			CollectorEndpoint string
		}
	}

	// Multitenancy configuration
	Multitenancy struct {
		Enabled      bool
		DefaultOrgID string
	}

	// Guardrails configuration
	Guardrails struct {
		Enabled    bool
		ConfigPath string
	}
}

// OpenAIConfig contains OpenAI-specific configuration
type OpenAIConfig struct {
	APIKey      string
	Model       string
	Temperature float64
	BaseURL     string
	Timeout     time.Duration
}

// AnthropicConfig contains Anthropic-specific configuration
type AnthropicConfig struct {
	APIKey      string
	Model       string
	Temperature float64
	BaseURL     string
	Timeout     time.Duration
}

// AzureOpenAIConfig contains Azure OpenAI-specific configuration
type AzureOpenAIConfig struct {
	APIKey       string
	Temperature  float64
	BaseURL      string
	Region       string
	ResourceName string
	Deployment   string
	APIVersion   string
	Timeout      time.Duration
}

// LoadDotEnv loads environment variables from a .env file.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Split at first '='
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// remove surrounding quotes if present
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		_ = os.Setenv(key, val)
	}
	return scanner.Err()
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	config := &Config{}

	// LLM configuration
	initLLMConfig(config)

	// Memory configuration
	config.Memory.Redis.URL = getEnv("REDIS_URL", "localhost:6379")
	config.Memory.Redis.Password = getEnv("REDIS_PASSWORD", "")
	config.Memory.Redis.DB = getEnvInt("REDIS_DB", 0)

	// VectorStore configuration
	config.VectorStore.Weaviate.URL = getEnv("WEAVIATE_URL", "")
	config.VectorStore.Weaviate.APIKey = getEnv("WEAVIATE_API_KEY", "")
	config.VectorStore.Weaviate.Scheme = getEnv("WEAVIATE_SCHEME", "https")
	config.VectorStore.Weaviate.Host = getEnv("WEAVIATE_HOST", "localhost:8080")
	config.VectorStore.Weaviate.ClassName = getEnv("WEAVIATE_CLASS_NAME", "Document")

	// DataStore configuration
	config.DataStore.Supabase.URL = getEnv("SUPABASE_URL", "")
	config.DataStore.Supabase.APIKey = getEnv("SUPABASE_API_KEY", "")
	config.DataStore.Supabase.Table = getEnv("SUPABASE_TABLE", "documents")

	// Tools configuration
	config.Tools.WebSearch.GoogleAPIKey = getEnv("GOOGLE_API_KEY", "")
	config.Tools.WebSearch.GoogleSearchEngineID = getEnv("GOOGLE_SEARCH_ENGINE_ID", "")

	config.Tools.GitHub.Token = getEnv("GITHUB_TOKEN", "")

	config.Tools.Tavily.TavilyAPIKey = getEnv("TAVILY_API_KEY", "")

	// Tracing configuration
	config.Tracing.Langfuse.Enabled = getEnvBool("LANGFUSE_ENABLED", false)
	config.Tracing.Langfuse.SecretKey = getEnv("LANGFUSE_SECRET_KEY", "")
	config.Tracing.Langfuse.PublicKey = getEnv("LANGFUSE_PUBLIC_KEY", "")
	config.Tracing.Langfuse.Host = getEnv("LANGFUSE_HOST", "https://cloud.langfuse.com")
	config.Tracing.Langfuse.Environment = getEnv("LANGFUSE_ENVIRONMENT", "development")

	config.Tracing.OpenTelemetry.Enabled = getEnvBool("OTEL_ENABLED", false)
	config.Tracing.OpenTelemetry.ServiceName = getEnv("OTEL_SERVICE_NAME", "agent-sdk")
	config.Tracing.OpenTelemetry.CollectorEndpoint = getEnv("OTEL_COLLECTOR_ENDPOINT", "localhost:4317")

	// Multitenancy configuration
	config.Multitenancy.Enabled = getEnvBool("MULTITENANCY_ENABLED", false)
	config.Multitenancy.DefaultOrgID = getEnv("DEFAULT_ORG_ID", "default")

	// Guardrails configuration
	config.Guardrails.Enabled = getEnvBool("GUARDRAILS_ENABLED", false)
	config.Guardrails.ConfigPath = getEnv("GUARDRAILS_CONFIG_PATH", "")

	return config
}

// initLLMConfig initializes LLM configuration with defaults
func initLLMConfig(config *Config) {
	// OpenAI defaults
	config.LLM.OpenAI.APIKey = getEnvString("OPENAI_API_KEY", "")
	config.LLM.OpenAI.Model = getEnvString("OPENAI_MODEL", "gpt-4o-mini")
	config.LLM.OpenAI.Temperature = getEnvFloat("OPENAI_TEMPERATURE", 0.7)
	config.LLM.OpenAI.BaseURL = getEnvString("OPENAI_BASE_URL", "")
	config.LLM.OpenAI.Timeout = time.Duration(getEnvInt("OPENAI_TIMEOUT", 60)) * time.Second

	// Anthropic defaults
	config.LLM.Anthropic.APIKey = getEnvString("ANTHROPIC_API_KEY", "")
	config.LLM.Anthropic.Model = getEnvString("ANTHROPIC_MODEL", "claude-3-7-sonnet-20240307")
	config.LLM.Anthropic.Temperature = getEnvFloat("ANTHROPIC_TEMPERATURE", 0.7)
	config.LLM.Anthropic.BaseURL = getEnvString("ANTHROPIC_BASE_URL", "")
	config.LLM.Anthropic.Timeout = time.Duration(getEnvInt("ANTHROPIC_TIMEOUT", 60)) * time.Second

	// Azure OpenAI defaults
	config.LLM.AzureOpenAI.APIKey = getEnvString("AZURE_OPENAI_API_KEY", "")
	config.LLM.AzureOpenAI.Temperature = getEnvFloat("AZURE_OPENAI_TEMPERATURE", 0.7)
	config.LLM.AzureOpenAI.BaseURL = getEnvString("AZURE_OPENAI_BASE_URL", "")
	config.LLM.AzureOpenAI.Region = getEnvString("AZURE_OPENAI_REGION", "")
	config.LLM.AzureOpenAI.ResourceName = getEnvString("AZURE_OPENAI_RESOURCE_NAME", "")
	config.LLM.AzureOpenAI.Deployment = getEnvString("AZURE_OPENAI_DEPLOYMENT", "")
	config.LLM.AzureOpenAI.APIVersion = getEnvString("AZURE_OPENAI_API_VERSION", "2024-08-01-preview")
	config.LLM.AzureOpenAI.Timeout = time.Duration(getEnvInt("AZURE_OPENAI_TIMEOUT", 60)) * time.Second
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getEnvBool gets a boolean environment variable or returns a default value
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return boolValue
}

// getEnvInt gets an integer environment variable or returns a default value
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return intValue
}

// getEnvFloat gets a float environment variable or returns a default value
func getEnvFloat(key string, defaultValue float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	floatValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultValue
	}
	return floatValue
}

// getEnvString gets a string environment variable or returns a default value
func getEnvString(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// Global instance of the configuration
var globalConfig *Config

// Initialize the global configuration
func init() {
	if err := LoadDotEnv(".env"); err != nil {
		panic(err)
	}
	globalConfig = LoadFromEnv()
}

// Get returns the global configuration
func Get() *Config {
	return globalConfig
}

// Reload reloads the configuration from environment variables
func Reload() *Config {
	globalConfig = LoadFromEnv()
	return globalConfig
}
