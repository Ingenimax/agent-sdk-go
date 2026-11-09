package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// envKeys lists every variable that LoadFromEnv reads.
var envKeys = []string{
	"OPENAI_API_KEY", "OPENAI_MODEL", "OPENAI_TEMPERATURE", "OPENAI_BASE_URL", "OPENAI_TIMEOUT",
	"ANTHROPIC_API_KEY", "ANTHROPIC_MODEL", "ANTHROPIC_TEMPERATURE", "ANTHROPIC_BASE_URL", "ANTHROPIC_TIMEOUT",
	"AZURE_OPENAI_API_KEY", "AZURE_OPENAI_TEMPERATURE", "AZURE_OPENAI_BASE_URL", "AZURE_OPENAI_REGION",
	"AZURE_OPENAI_RESOURCE_NAME", "AZURE_OPENAI_DEPLOYMENT", "AZURE_OPENAI_API_VERSION", "AZURE_OPENAI_TIMEOUT",
	"REDIS_URL", "REDIS_PASSWORD", "REDIS_DB",
	"WEAVIATE_URL", "WEAVIATE_API_KEY", "WEAVIATE_SCHEME", "WEAVIATE_HOST", "WEAVIATE_CLASS_NAME",
	"SUPABASE_URL", "SUPABASE_API_KEY", "SUPABASE_TABLE",
	"GOOGLE_API_KEY", "GOOGLE_SEARCH_ENGINE_ID", "GITHUB_TOKEN",
	"LANGFUSE_ENABLED", "LANGFUSE_SECRET_KEY", "LANGFUSE_PUBLIC_KEY", "LANGFUSE_HOST",
	"LANGFUSE_ENVIRONMENT", "LANGFUSE_INCLUDE_CONTENT",
	"OTEL_ENABLED", "OTEL_SERVICE_NAME", "OTEL_COLLECTOR_ENDPOINT",
	"MULTITENANCY_ENABLED", "DEFAULT_ORG_ID",
	"GUARDRAILS_ENABLED", "GUARDRAILS_CONFIG_PATH",
	"STAROPS_CONFIG_SERVICE_HOST",
}

// clearEnv unsets every config variable for the duration of the test.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range envKeys {
		// t.Setenv registers the restore; Unsetenv then removes the value.
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadFromEnvDefaults(t *testing.T) {
	clearEnv(t)

	c := LoadFromEnv()

	assert.Equal(t, "", c.LLM.OpenAI.APIKey)
	assert.Equal(t, "gpt-4o-mini", c.LLM.OpenAI.Model)
	assert.Equal(t, 0.7, c.LLM.OpenAI.Temperature)
	assert.Equal(t, 60*time.Second, c.LLM.OpenAI.Timeout)
	assert.Equal(t, "claude-3-7-sonnet-20240307", c.LLM.Anthropic.Model)
	assert.Equal(t, 0.7, c.LLM.Anthropic.Temperature)
	assert.Equal(t, 60*time.Second, c.LLM.Anthropic.Timeout)
	assert.Equal(t, "2024-08-01-preview", c.LLM.AzureOpenAI.APIVersion)
	assert.Equal(t, 60*time.Second, c.LLM.AzureOpenAI.Timeout)

	assert.Equal(t, "localhost:6379", c.Memory.Redis.URL)
	assert.Equal(t, 0, c.Memory.Redis.DB)
	assert.Equal(t, "https", c.VectorStore.Weaviate.Scheme)
	assert.Equal(t, "localhost:8080", c.VectorStore.Weaviate.Host)
	assert.Equal(t, "Document", c.VectorStore.Weaviate.ClassName)
	assert.Equal(t, "documents", c.DataStore.Supabase.Table)

	assert.False(t, c.Tracing.Langfuse.Enabled)
	assert.Equal(t, "https://cloud.langfuse.com", c.Tracing.Langfuse.Host)
	assert.Equal(t, "development", c.Tracing.Langfuse.Environment)
	assert.False(t, c.Tracing.OpenTelemetry.Enabled)
	assert.Equal(t, "agent-sdk", c.Tracing.OpenTelemetry.ServiceName)
	assert.Equal(t, "localhost:4317", c.Tracing.OpenTelemetry.CollectorEndpoint)

	assert.False(t, c.Multitenancy.Enabled)
	assert.Equal(t, "default", c.Multitenancy.DefaultOrgID)
	assert.False(t, c.Guardrails.Enabled)
	assert.Equal(t,
		"http://starops-config-service-service.starops-config-service.svc.cluster.local:8080",
		c.ConfigService.Host)
}

func TestLoadFromEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "gpt-x")
	t.Setenv("OPENAI_TEMPERATURE", "0.2")
	t.Setenv("OPENAI_TIMEOUT", "5")
	t.Setenv("ANTHROPIC_MODEL", "claude-x")
	t.Setenv("ANTHROPIC_TIMEOUT", "7")
	t.Setenv("AZURE_OPENAI_DEPLOYMENT", "dep")
	t.Setenv("AZURE_OPENAI_TEMPERATURE", "1.5")
	t.Setenv("REDIS_URL", "redis:1")
	t.Setenv("REDIS_DB", "4")
	t.Setenv("WEAVIATE_CLASS_NAME", "Doc2")
	t.Setenv("SUPABASE_TABLE", "t")
	t.Setenv("GITHUB_TOKEN", "gh")
	t.Setenv("LANGFUSE_ENABLED", "true")
	t.Setenv("LANGFUSE_INCLUDE_CONTENT", "1")
	t.Setenv("OTEL_ENABLED", "TRUE")
	t.Setenv("MULTITENANCY_ENABLED", "t")
	t.Setenv("DEFAULT_ORG_ID", "acme")
	t.Setenv("GUARDRAILS_ENABLED", "true")
	t.Setenv("GUARDRAILS_CONFIG_PATH", "/etc/g.yaml")
	t.Setenv("STAROPS_CONFIG_SERVICE_HOST", "http://cfg")

	c := LoadFromEnv()

	assert.Equal(t, "sk-test", c.LLM.OpenAI.APIKey)
	assert.Equal(t, "gpt-x", c.LLM.OpenAI.Model)
	assert.Equal(t, 0.2, c.LLM.OpenAI.Temperature)
	assert.Equal(t, 5*time.Second, c.LLM.OpenAI.Timeout)
	assert.Equal(t, "claude-x", c.LLM.Anthropic.Model)
	assert.Equal(t, 7*time.Second, c.LLM.Anthropic.Timeout)
	assert.Equal(t, "dep", c.LLM.AzureOpenAI.Deployment)
	assert.Equal(t, 1.5, c.LLM.AzureOpenAI.Temperature)
	assert.Equal(t, "redis:1", c.Memory.Redis.URL)
	assert.Equal(t, 4, c.Memory.Redis.DB)
	assert.Equal(t, "Doc2", c.VectorStore.Weaviate.ClassName)
	assert.Equal(t, "t", c.DataStore.Supabase.Table)
	assert.Equal(t, "gh", c.Tools.GitHub.Token)
	assert.True(t, c.Tracing.Langfuse.Enabled)
	assert.True(t, c.Tracing.Langfuse.IncludeContent)
	assert.True(t, c.Tracing.OpenTelemetry.Enabled)
	assert.True(t, c.Multitenancy.Enabled)
	assert.Equal(t, "acme", c.Multitenancy.DefaultOrgID)
	assert.True(t, c.Guardrails.Enabled)
	assert.Equal(t, "/etc/g.yaml", c.Guardrails.ConfigPath)
	assert.Equal(t, "http://cfg", c.ConfigService.Host)
}

func TestLoadFromEnvInvalidValuesFallBackToDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("OPENAI_TEMPERATURE", "hot")
	t.Setenv("OPENAI_TIMEOUT", "soon")
	t.Setenv("REDIS_DB", "one")
	t.Setenv("LANGFUSE_ENABLED", "maybe")
	t.Setenv("OTEL_ENABLED", "yes")

	c := LoadFromEnv()

	assert.Equal(t, 0.7, c.LLM.OpenAI.Temperature)
	assert.Equal(t, 60*time.Second, c.LLM.OpenAI.Timeout)
	assert.Equal(t, 0, c.Memory.Redis.DB)
	assert.False(t, c.Tracing.Langfuse.Enabled)
	assert.False(t, c.Tracing.OpenTelemetry.Enabled)
}

func TestGetAndReload(t *testing.T) {
	orig := globalConfig
	t.Cleanup(func() { globalConfig = orig })

	clearEnv(t)
	t.Setenv("OPENAI_MODEL", "first")
	first := Reload()
	assert.Same(t, first, Get())
	assert.Equal(t, "first", Get().LLM.OpenAI.Model)

	t.Setenv("OPENAI_MODEL", "second")
	assert.Equal(t, "first", Get().LLM.OpenAI.Model, "Get must not reread the environment")

	second := Reload()
	assert.NotSame(t, first, second)
	assert.Same(t, second, Get())
	assert.Equal(t, "second", Get().LLM.OpenAI.Model)
}
