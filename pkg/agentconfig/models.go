package agentconfig

import (
	"time"

	"github.com/google/uuid"
)

// ConfigurationValueType represents the type of value stored
type ConfigurationValueType string

const (
	ConfigurationValueTypePlain  ConfigurationValueType = "plain"
	ConfigurationValueTypeSecret ConfigurationValueType = "secret"
)

// SecretRef represents a reference to a secret in the secret manager
type SecretRef struct {
	ProviderID string  `json:"provider_id"`
	Key        string  `json:"key"`
	Instance   *string `json:"instance,omitempty"`
}

// ResolvedConfigurationValue represents a configuration value with the actual resolved content
type ResolvedConfigurationValue struct {
	Type         ConfigurationValueType `json:"type"`
	Value        string                 `json:"value"`
	SecretRef    *SecretRef             `json:"secret_ref,omitempty"`
	StoreInVault bool                   `json:"store_in_vault,omitempty"`
}

// ConfigurationResponse represents the response when returning a configuration with resolved secrets
type ConfigurationResponse struct {
	ID          uuid.UUID                  `json:"id"`
	OrgID       string                     `json:"org_id"`
	UserID      uuid.UUID                  `json:"user_id"`
	InstanceID  string                     `json:"instance_id"`
	Environment string                     `json:"environment"`
	Key         string                     `json:"key"`
	Value       ResolvedConfigurationValue `json:"value"`
	Description *string                    `json:"description,omitempty"`
	CreatedBy   *string                    `json:"created_by,omitempty"`
	UpdatedBy   *string                    `json:"updated_by,omitempty"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
}
