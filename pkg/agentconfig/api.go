package agentconfig

import (
	"context"
	"fmt"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
)

// LoadAgentFromRemote is a convenience function that loads and creates an agent from remote config
func LoadAgentFromRemote(ctx context.Context, agentName, environment string, options ...agent.Option) (*agent.Agent, error) {
	config, err := LoadAgentConfig(ctx, agentName, environment,
		WithRemoteOnly(),
		WithCache(5*time.Minute),
		WithEnvOverrides(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load remote config: %w", err)
	}

	return agent.NewAgentFromConfigObject(ctx, config, nil, options...)
}

// LoadAgentFromLocal is a convenience function that loads and creates an agent from local file
func LoadAgentFromLocal(ctx context.Context, agentName, environment string, options ...agent.Option) (*agent.Agent, error) {
	config, err := LoadAgentConfig(ctx, agentName, environment,
		WithLocalOnly(),
		WithEnvOverrides(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load local config: %w", err)
	}

	return agent.NewAgentFromConfigObject(ctx, config, nil, options...)
}

// LoadAgentAuto tries remote first, falls back to local (recommended for most use cases)
func LoadAgentAuto(ctx context.Context, agentName, environment string, options ...agent.Option) (*agent.Agent, error) {
	config, err := LoadAgentConfig(ctx, agentName, environment,
		WithLocalFallback(""), // Auto-detect local file
		WithCache(5*time.Minute),
		WithEnvOverrides(),
		WithVerbose(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from any source: %w", err)
	}

	return agent.NewAgentFromConfigObject(ctx, config, nil, options...)
}

// PreviewAgentConfig returns the resolved configuration without creating an agent
func PreviewAgentConfig(ctx context.Context, agentName, environment string) (*agent.AgentConfig, error) {
	return LoadAgentConfig(ctx, agentName, environment,
		WithLocalFallback(""),
		WithoutCache(), // Don't cache previews
		WithEnvOverrides(),
	)
}

// LoadAgentWithOptions provides full control over loading options
func LoadAgentWithOptions(ctx context.Context, agentName, environment string, loadOptions []LoadOption, agentOptions ...agent.Option) (*agent.Agent, error) {
	config, err := LoadAgentConfig(ctx, agentName, environment, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to load agent config: %w", err)
	}

	return agent.NewAgentFromConfigObject(ctx, config, nil, agentOptions...)
}

// LoadAgentWithVariables loads an agent and applies variable substitutions
func LoadAgentWithVariables(ctx context.Context, agentName, environment string, variables map[string]string, options ...agent.Option) (*agent.Agent, error) {
	config, err := LoadAgentConfig(ctx, agentName, environment,
		WithLocalFallback(""),
		WithCache(5*time.Minute),
		WithEnvOverrides(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return agent.NewAgentFromConfigObject(ctx, config, variables, options...)
}