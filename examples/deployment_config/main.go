package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agentconfig"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func main() {
	// Create a logger
	logger := logging.New()
	ctx := context.Background()

	logger.Info(ctx, "Deployment Configuration Example", nil)
	logger.Info(ctx, "This example demonstrates how to fetch deployment configurations from StarOps Config Service", nil)

	// Example 1: Load configuration from environment variables
	// This requires AGENT_DEPLOYMENT_ID and ENVIRONMENT to be set
	fmt.Println("\n=== Example 1: Load from Environment ===")

	deploymentID := os.Getenv("AGENT_DEPLOYMENT_ID")
	environment := os.Getenv("ENVIRONMENT")

	if deploymentID != "" && environment != "" {
		logger.Info(ctx, "Loading configuration from environment", map[string]interface{}{
			"deployment_id": deploymentID,
			"environment":   environment,
		})

		config, err := agentconfig.LoadFromEnvironment(ctx)
		if err != nil {
			logger.Error(ctx, "Failed to load deployment config", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			logger.Info(ctx, "Successfully loaded configuration", map[string]interface{}{
				"config_count": len(config),
			})

			// Display the configuration
			fmt.Println("\nConfiguration loaded:")
			for key, value := range config {
				// Mask sensitive values (secrets) for display
				displayValue := value
				if len(value) > 20 {
					displayValue = value[:10] + "..." + value[len(value)-10:]
				}
				fmt.Printf("  %s: %s\n", key, displayValue)
			}
		}
	} else {
		logger.Info(ctx, "Skipping Example 1: AGENT_DEPLOYMENT_ID or ENVIRONMENT not set", nil)
		fmt.Println("Set AGENT_DEPLOYMENT_ID and ENVIRONMENT to use LoadDeploymentConfig()")
	}

	// Example 2: Using a custom client with explicit parameters
	fmt.Println("\n=== Example 2: Custom Client with Explicit Parameters ===")

	// Override with explicit values for testing
	testDeploymentID := os.Getenv("AGENT_DEPLOYMENT_ID")
	testEnvironment := os.Getenv("ENVIRONMENT")

	if testDeploymentID == "" {
		testDeploymentID = "example-agent-deployment-001"
	}
	if testEnvironment == "" {
		testEnvironment = "preview"
	}

	logger.Info(ctx, "Creating configuration client", map[string]interface{}{
		"deployment_id": testDeploymentID,
		"environment":   testEnvironment,
	})

	client, err := agentconfig.NewClient()
	if err != nil {
		logger.Error(ctx, "Failed to create config client", map[string]interface{}{
			"error": err.Error(),
		})
		fmt.Printf("Error: %v\n", err)
		return
	}

	config, err := client.FetchDeploymentConfig(ctx, testDeploymentID, testEnvironment)
	if err != nil {
		logger.Error(ctx, "Failed to fetch deployment config", map[string]interface{}{
			"error":         err.Error(),
			"deployment_id": testDeploymentID,
			"environment":   testEnvironment,
		})
		fmt.Printf("Error: %v\n", err)

		// Print troubleshooting information
		fmt.Println("\nTroubleshooting:")
		fmt.Println("1. Ensure STAROPS_CONFIG_SERVICE_HOST is set correctly")
		fmt.Println("2. Verify the config service is running and accessible")
		fmt.Println("3. Check that the deployment_id and environment exist in the config service")
		fmt.Printf("4. Current host: %s\n", os.Getenv("STAROPS_CONFIG_SERVICE_HOST"))
	} else {
		logger.Info(ctx, "Successfully fetched configuration", map[string]interface{}{
			"config_count":  len(config),
			"deployment_id": testDeploymentID,
			"environment":   testEnvironment,
		})

		// Display the configuration
		fmt.Println("\nConfiguration fetched:")
		if len(config) == 0 {
			fmt.Println("  (No configurations found - this is normal if none exist for this deployment)")
		}
		for key, value := range config {
			// Mask sensitive values for display
			displayValue := value
			if len(value) > 20 {
				displayValue = value[:10] + "..." + value[len(value)-10:]
			}
			fmt.Printf("  %s: %s\n", key, displayValue)
		}

		// Example: Using configuration values
		if apiKey, exists := config["API_KEY"]; exists {
			logger.Info(ctx, "Found API_KEY in configuration", map[string]interface{}{
				"key_length": len(apiKey),
			})
			fmt.Printf("\nExample usage: API_KEY found with length %d\n", len(apiKey))
		}
	}

	// Example 3: Demonstrating error handling
	fmt.Println("\n=== Example 3: Error Handling ===")

	logger.Info(ctx, "Testing error handling with invalid parameters", nil)

	_, err = client.FetchDeploymentConfig(ctx, "", "preview")
	if err != nil {
		fmt.Printf("✓ Expected error with empty deployment_id: %v\n", err)
	}

	_, err = client.FetchDeploymentConfig(ctx, "test", "")
	if err != nil {
		fmt.Printf("✓ Expected error with empty environment: %v\n", err)
	}

	fmt.Println("\n=== Configuration Example Complete ===")
}
