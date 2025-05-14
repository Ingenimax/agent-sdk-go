package multitenancy_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
)

func TestMultiTenancy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Create a config manager
	configManager := multitenancy.NewConfigManager()

	// Register two tenants
	err := configManager.RegisterTenant(&multitenancy.TenantConfig{
		OrgID: "org1",
		LLMAPIKeys: map[string]string{
			"openai": "org1-api-key",
		},
	})
	if err != nil {
		t.Fatalf("Failed to register tenant: %v", err)
	}

	err = configManager.RegisterTenant(&multitenancy.TenantConfig{
		OrgID: "org2",
		LLMAPIKeys: map[string]string{
			"openai": "org2-api-key",
		},
	})
	if err != nil {
		t.Fatalf("Failed to register tenant: %v", err)
	}

	// Create contexts for each organization
	ctx1 := multitenancy.WithOrgID(context.Background(), "org1")
	ctx2 := multitenancy.WithOrgID(context.Background(), "org2")

	// Test that we can get the correct API keys for each organization
	apiKey1, err := configManager.GetLLMAPIKey(ctx1, "openai")
	if err != nil {
		t.Fatalf("Failed to get API key: %v", err)
	}
	if apiKey1 != "org1-api-key" {
		t.Errorf("Expected API key 'org1-api-key', got '%s'", apiKey1)
	}

	apiKey2, err := configManager.GetLLMAPIKey(ctx2, "openai")
	if err != nil {
		t.Fatalf("Failed to get API key: %v", err)
	}
	if apiKey2 != "org2-api-key" {
		t.Errorf("Expected API key 'org2-api-key', got '%s'", apiKey2)
	}

	// Create a vector store
	store := weaviate.New(nil)

	// Test that the class names are different for each organization
	weaviateStore := store // No type assertion needed if store is already the correct type

	// Get the method using reflection
	method := reflect.ValueOf(weaviateStore).MethodByName("getClassName")
	if !method.IsValid() {
		t.Fatalf("getClassName method not found")
	}

	// Call the method
	results := method.Call([]reflect.Value{reflect.ValueOf(ctx1)})
	if len(results) != 2 {
		t.Fatalf("Expected 2 return values, got %d", len(results))
	}

	// Check for error
	if !results[1].IsNil() {
		t.Fatalf("Failed to get class name: %v", results[1].Interface())
	}

	className1 := results[0].String()

	// Use reflection to call the unexported method for the second context
	results = method.Call([]reflect.Value{reflect.ValueOf(ctx2)})
	if len(results) != 2 {
		t.Fatalf("Expected 2 return values, got %d", len(results))
	}

	// Check for error
	if !results[1].IsNil() {
		t.Fatalf("Failed to get class name: %v", results[1].Interface())
	}

	className2 := results[0].String()

	if className1 == className2 {
		t.Errorf("Expected different class names, got '%s' for both", className1)
	}
}
