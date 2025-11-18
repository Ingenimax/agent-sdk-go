package agentconfig

import (
	"context"
	"fmt"
	"os"
)

// LoadFromEnvironment loads configuration from environment variables
// Reads AGENT_DEPLOYMENT_ID and ENVIRONMENT from environment variables
// and fetches the configuration from the StarOps config service
func LoadFromEnvironment(ctx context.Context) (map[string]string, error) {
	deploymentID := os.Getenv("AGENT_DEPLOYMENT_ID")
	if deploymentID == "" {
		return nil, fmt.Errorf("AGENT_DEPLOYMENT_ID environment variable is required")
	}

	environment := os.Getenv("ENVIRONMENT")
	if environment == "" {
		return nil, fmt.Errorf("ENVIRONMENT environment variable is required")
	}

	client, err := NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create config client: %w", err)
	}

	return client.FetchDeploymentConfig(ctx, deploymentID, environment)
}

