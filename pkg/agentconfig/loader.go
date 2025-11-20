package agentconfig

import (
	"context"
	"fmt"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
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

// LoadAndMergeWithViper loads dynamic configuration from config service and merges it with viper
// This handles the complete flow of:
// 1. Reading AGENT_DEPLOYMENT_ID, ENVIRONMENT, and STAROPS_CONFIG_SERVICE_HOST from viper
// 2. Setting them in OS environment for SDK access
// 3. Reloading SDK config
// 4. Loading dynamic config from service
// 5. Merging dynamic config into viper (local env vars take priority)
//
// Returns the number of configs loaded and merged, or an error
func LoadAndMergeWithViper(ctx context.Context, v *viper.Viper) (configsLoaded int, configsMerged int, err error) {
	// Step 1: Check for required AGENT_DEPLOYMENT_ID
	deploymentID := v.GetString("AGENT_DEPLOYMENT_ID")
	if deploymentID == "" {
		return 0, 0, fmt.Errorf("AGENT_DEPLOYMENT_ID is required")
	}

	environment := v.GetString("ENVIRONMENT")
	configServiceHost := v.GetString("STAROPS_CONFIG_SERVICE_HOST")

	log.Info().
		Str("agent_deployment_id", deploymentID).
		Str("environment", environment).
		Msg("Starting dynamic config loading from config service")

	// Step 2: Ensure required env vars are set in OS environment so SDK can read them
	if os.Getenv("AGENT_DEPLOYMENT_ID") == "" {
		if err := os.Setenv("AGENT_DEPLOYMENT_ID", deploymentID); err != nil {
			log.Warn().Err(err).Msg("Failed to set AGENT_DEPLOYMENT_ID in environment")
		}
	}
	if environment != "" && os.Getenv("ENVIRONMENT") == "" {
		if err := os.Setenv("ENVIRONMENT", environment); err != nil {
			log.Warn().Err(err).Msg("Failed to set ENVIRONMENT in environment")
		}
	}
	if configServiceHost != "" && os.Getenv("STAROPS_CONFIG_SERVICE_HOST") == "" {
		if err := os.Setenv("STAROPS_CONFIG_SERVICE_HOST", configServiceHost); err != nil {
			log.Warn().Err(err).Msg("Failed to set STAROPS_CONFIG_SERVICE_HOST in environment")
		}
	}

	log.Info().
		Str("config_service_host", configServiceHost).
		Msg("Config service host configured")

	// Step 3: Reload SDK config after setting environment variables
	config.Reload()

	// Step 4: Load dynamic config from service
	dynamicConfig, err := LoadFromEnvironment(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load dynamic config from config service")
		return 0, 0, fmt.Errorf("failed to load dynamic config: %w", err)
	}

	log.Info().
		Int("config_count", len(dynamicConfig)).
		Msg("Successfully loaded dynamic config from config service")

	// Step 5: Merge dynamic configs into viper (local env vars take priority)
	configsMerged = 0
	configsSkipped := 0
	for key, value := range dynamicConfig {
		if v.GetString(key) == "" {
			v.Set(key, value)
			configsMerged++
			// Log important secrets/keys being loaded (without showing values)
			if key == "LANGFUSE_SECRET_KEY" || key == "LANGFUSE_PUBLIC_KEY" {
				log.Debug().Str("key", key).Bool("has_value", value != "").Msg("Loaded config key")
			}
		} else {
			configsSkipped++
			log.Debug().Str("key", key).Msg("Skipped config (already set locally)")
		}
	}

	log.Info().
		Int("configs_loaded", len(dynamicConfig)).
		Int("configs_merged", configsMerged).
		Int("configs_skipped", configsSkipped).
		Msg("Dynamic config merge completed")

	return len(dynamicConfig), configsMerged, nil
}
