# Environment Variables

This document lists all environment variables used by the Agent SDK.

## LLM Configuration

### OpenAI

- `OPENAI_API_KEY`: API key for OpenAI
- `OPENAI_MODEL`: Model to use (default: "gpt-4o-mini")
- `OPENAI_TEMPERATURE`: Temperature for generation (default: 0.7)
- `OPENAI_MAX_TOKENS`: Maximum tokens to generate (default: 2048)
- `OPENAI_BASE_URL`: Base URL for API calls (default: "https://api.openai.com/v1")
- `OPENAI_TIMEOUT_SECONDS`: Timeout in seconds (default: 60)

### Anthropic

- `ANTHROPIC_API_KEY`: API key for Anthropic
- `ANTHROPIC_MODEL`: Model to use (default: "claude-3-haiku-20240307")
- `ANTHROPIC_TEMPERATURE`: Temperature for generation (default: 0.7)
- `ANTHROPIC_MAX_TOKENS`: Maximum tokens to generate (default: 2048)
- `ANTHROPIC_BASE_URL`: Base URL for API calls (default: "https://api.anthropic.com")
- `ANTHROPIC_TIMEOUT_SECONDS`: Timeout in seconds (default: 60)

### Azure OpenAI

- `AZURE_OPENAI_API_KEY`: API key for Azure OpenAI
- `AZURE_OPENAI_ENDPOINT`: Endpoint for Azure OpenAI
- `AZURE_OPENAI_DEPLOYMENT`: Deployment name
- `AZURE_OPENAI_API_VERSION`: API version (default: "2023-05-15")
- `AZURE_OPENAI_TEMPERATURE`: Temperature for generation (default: 0.7)
- `AZURE_OPENAI_MAX_TOKENS`: Maximum tokens to generate (default: 2048)
- `AZURE_OPENAI_TIMEOUT_SECONDS`: Timeout in seconds (default: 60)

## Memory Configuration

### Redis

- `REDIS_URL`: Redis URL (default: "localhost:6379")
- `REDIS_PASSWORD`: Redis password
- `REDIS_DB`: Redis database number (default: 0)

## VectorStore Configuration

### Weaviate

- `WEAVIATE_URL`: Weaviate URL
- `WEAVIATE_API_KEY`: Weaviate API key
- `WEAVIATE_SCHEME`: Weaviate scheme (default: "http")
- `WEAVIATE_HOST`: Weaviate host (default: "localhost:8080")
- `WEAVIATE_CLASS_NAME`: Weaviate class name (default: "Document")

### Pinecone

- `PINECONE_API_KEY`: Pinecone API key
- `PINECONE_ENVIRONMENT`: Pinecone environment
- `PINECONE_INDEX`: Pinecone index name

## DataStore Configuration

### Supabase

- `SUPABASE_URL`: Supabase URL
- `SUPABASE_API_KEY`: Supabase API key
- `SUPABASE_TABLE`: Supabase table name (default: "documents")

## Tools Configuration

### Web Search

- `GOOGLE_API_KEY`: Google API key for web search
- `GOOGLE_SEARCH_ENGINE_ID`: Google Search Engine ID

### AWS

- `AWS_ACCESS_KEY_ID`: AWS access key ID
- `AWS_SECRET_ACCESS_KEY`: AWS secret access key
- `AWS_REGION`: AWS region (default: "us-east-1")

### Kubernetes

- `KUBECONFIG`: Path to kubeconfig file
- `KUBE_CONTEXT`: Kubernetes context to use

## Tracing Configuration

### Langfuse

- `LANGFUSE_ENABLED`: Enable Langfuse tracing (default: false)
- `LANGFUSE_SECRET_KEY`: Langfuse secret key
- `LANGFUSE_PUBLIC_KEY`: Langfuse public key
- `LANGFUSE_HOST`: Langfuse host (default: "https://cloud.langfuse.com")
- `LANGFUSE_ENVIRONMENT`: Environment name (default: "development")

### OpenTelemetry

- `OTEL_ENABLED`: Enable OpenTelemetry tracing (default: false)
- `OTEL_SERVICE_NAME`: Service name (default: "agent-sdk")
- `OTEL_COLLECTOR_ENDPOINT`: Collector endpoint (default: "localhost:4317")

## Multitenancy Configuration

- `MULTITENANCY_ENABLED`: Enable multitenancy (default: false)
- `DEFAULT_ORG_ID`: Default organization ID (default: "default")

## Guardrails Configuration

- `GUARDRAILS_ENABLED`: Enable guardrails (default: false)
- `GUARDRAILS_CONFIG_PATH`: Path to guardrails configuration file 