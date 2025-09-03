package shared

import (
	"fmt"
	"os"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

// CreateLLM creates an LLM client based on environment variables
// It checks for LLM_PROVIDER to determine which provider to use (openai, anthropic, gemini)
// If not set, it tries to detect based on available API keys
func CreateLLM() (interfaces.LLM, error) {
	provider := strings.ToLower(os.Getenv("LLM_PROVIDER"))

	// If no provider specified, try to detect based on available API keys
	if provider == "" {
		if os.Getenv("OPENAI_API_KEY") != "" {
			provider = "openai"
		} else if os.Getenv("ANTHROPIC_API_KEY") != "" {
			provider = "anthropic"
		} else if os.Getenv("GEMINI_API_KEY") != "" {
			provider = "gemini"
		} else {
			return nil, fmt.Errorf("no LLM provider specified and no API keys found. Set LLM_PROVIDER or provide an API key (OPENAI_API_KEY, ANTHROPIC_API_KEY, or GEMINI_API_KEY)")
		}
	}

	switch provider {
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY environment variable is required for OpenAI provider")
		}
		model := os.Getenv("OPENAI_MODEL")
		if model == "" {
			model = "gpt-4o" // Default model
		}
		return openai.NewClient(apiKey, openai.WithModel(model)), nil

	case "anthropic":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is required for Anthropic provider")
		}
		model := os.Getenv("ANTHROPIC_MODEL")
		if model == "" {
			model = anthropic.Claude37Sonnet // Default model
		}
		return anthropic.NewClient(apiKey, anthropic.WithModel(model)), nil

	case "gemini":
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY environment variable is required for Gemini provider")
		}
		model := os.Getenv("GEMINI_MODEL")
		if model == "" {
			model = gemini.ModelGemini15Flash // Default model
		}
		client, err := gemini.NewClient(apiKey, gemini.WithModel(model))
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini client: %w", err)
		}
		return client, nil

	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s (supported: openai, anthropic, gemini)", provider)
	}
}

// GetProviderInfo returns information about the current LLM provider
func GetProviderInfo() string {
	provider := strings.ToLower(os.Getenv("LLM_PROVIDER"))

	// Auto-detect if not specified
	if provider == "" {
		if os.Getenv("OPENAI_API_KEY") != "" {
			provider = "openai"
		} else if os.Getenv("ANTHROPIC_API_KEY") != "" {
			provider = "anthropic"
		} else if os.Getenv("GEMINI_API_KEY") != "" {
			provider = "gemini"
		} else {
			return "No LLM provider configured"
		}
	}

	switch provider {
	case "openai":
		model := os.Getenv("OPENAI_MODEL")
		if model == "" {
			model = "gpt-4o"
		}
		return fmt.Sprintf("OpenAI (%s)", model)

	case "anthropic":
		model := os.Getenv("ANTHROPIC_MODEL")
		if model == "" {
			model = anthropic.Claude37Sonnet
		}
		return fmt.Sprintf("Anthropic (%s)", model)

	case "gemini":
		model := os.Getenv("GEMINI_MODEL")
		if model == "" {
			model = gemini.ModelGemini15Flash
		}
		return fmt.Sprintf("Gemini (%s)", model)

	default:
		return fmt.Sprintf("Unknown provider: %s", provider)
	}
}
