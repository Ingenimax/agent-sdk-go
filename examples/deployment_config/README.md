# Deployment Configuration Example

This example demonstrates how to fetch deployment configurations from the StarOps Config Service using the Agent SDK.

## Overview

The Agent SDK provides a configuration management system that allows agents to fetch their deployment-specific configurations from the StarOps Config Service. This is useful for:

- Loading environment-specific settings (preview, staging, production)
- Managing API keys and credentials
- Configuring agent-specific parameters
- Centralizing configuration management

## Prerequisites

Before running this example, ensure:

1. The StarOps Config Service is deployed and accessible
2. Configuration entries exist for your deployment in the config service
3. Environment variables are set (see below)

## Environment Variables

### Required for Example 1 (Load from Environment)

```bash
export AGENT_DEPLOYMENT_ID="your-deployment-id"
export ENVIRONMENT="preview"  # or "staging", "production", etc.
```

### Optional

```bash
# Override the default config service host
export STAROPS_CONFIG_SERVICE_HOST="http://starops-config-service.starops-config-service.svc.cluster.local:8091"

# For testing with specific deployment
export TEST_DEPLOYMENT_ID="example-agent-deployment-001"
export TEST_ENVIRONMENT="preview"
```

## How It Works

The configuration system works by:

1. **Reading Environment Variables**: The SDK reads `AGENT_DEPLOYMENT_ID` and `ENVIRONMENT` from the environment
2. **Calling Config Service**: Makes an HTTP GET request to `/api/v1/configurations` with query parameters:
   - `instance_id`: The agent deployment ID
   - `environment`: The environment (e.g., "preview", "production")
3. **Parsing Response**: Converts the array of configurations into a `map[string]string`
4. **Returning Config**: Returns a map where keys are configuration keys and values are the resolved values

## Usage Patterns

### Pattern 1: Load from Environment Variables

The simplest approach - reads `AGENT_DEPLOYMENT_ID` and `ENVIRONMENT` automatically:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/agentconfig"

config, err := agentconfig.LoadFromEnvironment(ctx)
if err != nil {
    log.Fatal(err)
}

// Use configuration values
apiKey := config["API_KEY"]
dbHost := config["DATABASE_HOST"]
```

### Pattern 2: Explicit Parameters

For more control, create a client and specify parameters explicitly:

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/agentconfig"

client, err := agentconfig.NewClient()
if err != nil {
    log.Fatal(err)
}

config, err := client.FetchDeploymentConfig(ctx, "agent-deploy-123", "preview")
if err != nil {
    log.Fatal(err)
}

// Use configuration
for key, value := range config {
    fmt.Printf("%s: %s\n", key, value)
}
```

## Creating Configuration Entries

Before running this example, create configuration entries in the StarOps Config Service:

```bash
# Example using the config service API
curl -X POST http://config-service/api/v1/configurations \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "example-agent-deployment-001",
    "environment": "preview",
    "key": "API_KEY",
    "value": {
      "type": "plain",
      "value": "your-api-key-here"
    },
    "description": "API key for external service"
  }'
```

## Running the Example

1. Build the example:

```bash
cd examples/deployment_config
go build -o deployment_config main.go
```

2. Set environment variables:

```bash
export AGENT_DEPLOYMENT_ID="your-deployment-id"
export ENVIRONMENT="preview"
```

3. Run the example:

```bash
./deployment_config
```

## Example Output

```
=== Example 1: Load from Environment ===
Loading configuration from environment
  deployment_id: example-agent-deployment-001
  environment: preview

Configuration loaded:
  API_KEY: sk-1234...xyz789
  DATABASE_HOST: postgres.internal.svc.cluster.local:5432
  MAX_RETRIES: 3

=== Example 2: Custom Client with Explicit Parameters ===
Creating configuration client
  deployment_id: example-agent-deployment-001
  environment: preview

Successfully fetched configuration
  config_count: 3

Configuration fetched:
  API_KEY: sk-1234...xyz789
  DATABASE_HOST: postgres.internal.svc.cluster.local:5432
  MAX_RETRIES: 3

Example usage: API_KEY found with length 48

=== Example 3: Error Handling ===
Testing error handling with invalid parameters
✓ Expected error with empty deployment_id: deploymentID cannot be empty
✓ Expected error with empty environment: environment cannot be empty

=== Configuration Example Complete ===
```

## Configuration Types

The config service supports two types of configuration values:

1. **Plain Text**: Regular configuration values stored as plain text
   - Example: `DATABASE_HOST`, `MAX_RETRIES`

2. **Secrets**: Sensitive values stored securely and resolved at fetch time
   - Example: `API_KEY`, `DATABASE_PASSWORD`
   - The SDK automatically retrieves the resolved secret values

Both types are returned as strings in the configuration map.

## Security Notes

- The `/api/v1/configurations` endpoint is configured to skip authentication for agent deployments
- Sensitive values (secrets) are resolved by the config service before being returned
- Always use environment variables or secure secret management for sensitive data
- Never commit configuration with real credentials to version control

## Troubleshooting

### Error: "STAROPS_CONFIG_SERVICE_HOST is not configured"

Solution: Set the environment variable:
```bash
export STAROPS_CONFIG_SERVICE_HOST="http://starops-config-service.starops-config-service.svc.cluster.local:8091"
```

### Error: "AGENT_DEPLOYMENT_ID environment variable is required"

Solution: Set the deployment ID:
```bash
export AGENT_DEPLOYMENT_ID="your-deployment-id"
```

### Error: Connection refused

- Verify the config service is running
- Check the service host and port are correct
- Ensure network connectivity to the service

### Empty configuration returned

- Verify configuration entries exist in the config service for your deployment ID and environment
- Check that the instance_id and environment match exactly
- Use the config service API to list configurations and verify they exist

## Integration with Agent

You can use the loaded configuration to initialize your agent:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agentconfig"
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
)

// Load configuration
config, err := agentconfig.LoadFromEnvironment(ctx)
if err != nil {
    log.Fatal(err)
}

// Use configuration values
os.Setenv("OPENAI_API_KEY", config["OPENAI_API_KEY"])
os.Setenv("DATABASE_URL", config["DATABASE_URL"])

// Create agent with config-driven settings
agent, err := agent.NewAgent(
    agent.WithName(config["AGENT_NAME"]),
    agent.WithMaxIterations(parseInt(config["MAX_ITERATIONS"])),
    // ... other options
)
```

## Related Documentation

- [Agent SDK Configuration](../../docs/configuration.md)
- [StarOps Config Service API](../../../starops-config-service/README.md)
- [Environment Variables](../../docs/environment_variables.md)

