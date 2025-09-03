package azureopenai

import (
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestNewClient(t *testing.T) {
	apiKey := "test-api-key"
	baseURL := "https://test.openai.azure.com"
	deployment := "test-deployment"

	client := NewClient(apiKey, baseURL, deployment)

	if client.apiKey != apiKey {
		t.Errorf("Expected API key %s, got %s", apiKey, client.apiKey)
	}

	if client.baseURL != baseURL {
		t.Errorf("Expected base URL %s, got %s", baseURL, client.baseURL)
	}

	if client.deployment != deployment {
		t.Errorf("Expected deployment %s, got %s", deployment, client.deployment)
	}

	if client.Model != "gpt-4o-mini" {
		t.Errorf("Expected default model gpt-4o-mini, got %s", client.Model)
	}

	if client.apiVersion != "2024-08-01-preview" {
		t.Errorf("Expected default API version 2024-08-01-preview, got %s", client.apiVersion)
	}
}

func TestNewClientWithOptions(t *testing.T) {
	apiKey := "test-api-key"
	baseURL := "https://test.openai.azure.com"
	deployment := "test-deployment"
	model := "gpt-4"
	apiVersion := "2023-12-01-preview"
	region := "eastus"
	resourceName := "test-resource"
	logger := logging.New()

	client := NewClient(
		apiKey,
		baseURL,
		deployment,
		WithModel(model),
		WithAPIVersion(apiVersion),
		WithRegion(region),
		WithResourceName(resourceName),
		WithLogger(logger),
	)

	if client.Model != model {
		t.Errorf("Expected model %s, got %s", model, client.Model)
	}

	if client.apiVersion != apiVersion {
		t.Errorf("Expected API version %s, got %s", apiVersion, client.apiVersion)
	}

	if client.region != region {
		t.Errorf("Expected region %s, got %s", region, client.region)
	}

	if client.resourceName != resourceName {
		t.Errorf("Expected resource name %s, got %s", resourceName, client.resourceName)
	}

	if client.logger != logger {
		t.Errorf("Expected custom logger, got different logger")
	}
}

func TestClientName(t *testing.T) {
	client := NewClient("test-key", "https://test.openai.azure.com", "test-deployment")

	expectedName := "azure-openai"
	if client.Name() != expectedName {
		t.Errorf("Expected name %s, got %s", expectedName, client.Name())
	}
}

func TestClientSupportsStreaming(t *testing.T) {
	client := NewClient("test-key", "https://test.openai.azure.com", "test-deployment")

	if !client.SupportsStreaming() {
		t.Error("Expected client to support streaming")
	}
}

func TestGetModel(t *testing.T) {
	model := "gpt-4"
	client := NewClient(
		"test-key",
		"https://test.openai.azure.com",
		"test-deployment",
		WithModel(model),
	)

	if client.GetModel() != model {
		t.Errorf("Expected model %s, got %s", model, client.GetModel())
	}
}

func TestGetDeployment(t *testing.T) {
	deployment := "test-deployment"
	client := NewClient("test-key", "https://test.openai.azure.com", deployment)

	if client.GetDeployment() != deployment {
		t.Errorf("Expected deployment %s, got %s", deployment, client.GetDeployment())
	}
}

func TestNewClientFromRegion(t *testing.T) {
	apiKey := "test-api-key"
	region := "eastus"
	resourceName := "test-resource"
	deployment := "test-deployment"

	client := NewClientFromRegion(apiKey, region, resourceName, deployment)

	if client.apiKey != apiKey {
		t.Errorf("Expected API key %s, got %s", apiKey, client.apiKey)
	}

	if client.region != region {
		t.Errorf("Expected region %s, got %s", region, client.region)
	}

	if client.resourceName != resourceName {
		t.Errorf("Expected resource name %s, got %s", resourceName, client.resourceName)
	}

	if client.deployment != deployment {
		t.Errorf("Expected deployment %s, got %s", deployment, client.deployment)
	}

	if client.Model != "gpt-4o-mini" {
		t.Errorf("Expected default model gpt-4o-mini, got %s", client.Model)
	}

	if client.apiVersion != "2024-08-01-preview" {
		t.Errorf("Expected default API version 2024-08-01-preview, got %s", client.apiVersion)
	}

	// Check that baseURL was constructed correctly
	expectedBaseURL := "https://test-resource.openai.azure.com"
	if client.baseURL != expectedBaseURL {
		t.Errorf("Expected base URL %s, got %s", expectedBaseURL, client.baseURL)
	}
}

func TestGetRegion(t *testing.T) {
	region := "westus2"
	client := NewClientFromRegion("test-key", region, "test-resource", "test-deployment")

	if client.GetRegion() != region {
		t.Errorf("Expected region %s, got %s", region, client.GetRegion())
	}
}

func TestGetResourceName(t *testing.T) {
	resourceName := "my-openai-resource"
	client := NewClientFromRegion("test-key", "eastus", resourceName, "test-deployment")

	if client.GetResourceName() != resourceName {
		t.Errorf("Expected resource name %s, got %s", resourceName, client.GetResourceName())
	}
}

func TestGetBaseURL(t *testing.T) {
	baseURL := "https://test.openai.azure.com"
	client := NewClient("test-key", baseURL, "test-deployment")

	if client.GetBaseURL() != baseURL {
		t.Errorf("Expected base URL %s, got %s", baseURL, client.GetBaseURL())
	}
}

func TestIsReasoningModel(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"gpt-4", false},
		{"gpt-4o", false},
		{"gpt-4o-mini", false},
		{"o1-preview", true},
		{"o1-mini", true},
		{"o3-mini", true},
		{"gpt-5", true},
		{"gpt-5-mini", true},
		{"claude-3", false},
	}

	for _, test := range tests {
		result := isReasoningModel(test.model)
		if result != test.expected {
			t.Errorf("For model %s, expected %v, got %v", test.model, test.expected, result)
		}
	}
}

func TestGetTemperatureForModel(t *testing.T) {
	tests := []struct {
		model       string
		requestTemp float64
		expected    float64
	}{
		{"gpt-4", 0.7, 0.7},
		{"gpt-4o-mini", 0.5, 0.5},
		{"o1-preview", 0.7, 1.0}, // Reasoning models force temperature to 1.0
		{"o1-mini", 0.3, 1.0},    // Reasoning models force temperature to 1.0
	}

	for _, test := range tests {
		client := NewClient(
			"test-key",
			"https://test.openai.azure.com",
			"test-deployment",
			WithModel(test.model),
		)

		result := client.getTemperatureForModel(test.requestTemp)
		if result != test.expected {
			t.Errorf("For model %s with temp %f, expected %f, got %f",
				test.model, test.requestTemp, test.expected, result)
		}
	}
}

func TestWithTemperatureOption(t *testing.T) {
	temp := 0.8
	options := &interfaces.GenerateOptions{}

	WithTemperature(temp)(options)

	if options.LLMConfig == nil {
		t.Error("Expected LLMConfig to be initialized")
		return
	}

	if options.LLMConfig.Temperature != temp {
		t.Errorf("Expected temperature %f, got %f", temp, options.LLMConfig.Temperature)
	}
}

func TestWithTopPOption(t *testing.T) {
	topP := 0.9
	options := &interfaces.GenerateOptions{}

	WithTopP(topP)(options)

	if options.LLMConfig == nil {
		t.Error("Expected LLMConfig to be initialized")
		return
	}

	if options.LLMConfig.TopP != topP {
		t.Errorf("Expected TopP %f, got %f", topP, options.LLMConfig.TopP)
	}
}

func TestWithFrequencyPenaltyOption(t *testing.T) {
	penalty := 0.5
	options := &interfaces.GenerateOptions{}

	WithFrequencyPenalty(penalty)(options)

	if options.LLMConfig == nil {
		t.Error("Expected LLMConfig to be initialized")
		return
	}

	if options.LLMConfig.FrequencyPenalty != penalty {
		t.Errorf("Expected FrequencyPenalty %f, got %f", penalty, options.LLMConfig.FrequencyPenalty)
	}
}

func TestWithPresencePenaltyOption(t *testing.T) {
	penalty := 0.3
	options := &interfaces.GenerateOptions{}

	WithPresencePenalty(penalty)(options)

	if options.LLMConfig == nil {
		t.Error("Expected LLMConfig to be initialized")
		return
	}

	if options.LLMConfig.PresencePenalty != penalty {
		t.Errorf("Expected PresencePenalty %f, got %f", penalty, options.LLMConfig.PresencePenalty)
	}
}

func TestWithStopSequencesOption(t *testing.T) {
	stopSeqs := []string{"STOP", "END"}
	options := &interfaces.GenerateOptions{}

	WithStopSequences(stopSeqs)(options)

	if options.LLMConfig == nil {
		t.Error("Expected LLMConfig to be initialized")
		return
	}

	if len(options.LLMConfig.StopSequences) != len(stopSeqs) {
		t.Errorf("Expected %d stop sequences, got %d", len(stopSeqs), len(options.LLMConfig.StopSequences))
		return
	}

	for i, seq := range stopSeqs {
		if options.LLMConfig.StopSequences[i] != seq {
			t.Errorf("Expected stop sequence %s at index %d, got %s", seq, i, options.LLMConfig.StopSequences[i])
		}
	}
}

func TestWithSystemMessageOption(t *testing.T) {
	sysMsg := "You are a helpful assistant."
	options := &interfaces.GenerateOptions{}

	WithSystemMessage(sysMsg)(options)

	if options.SystemMessage != sysMsg {
		t.Errorf("Expected system message %s, got %s", sysMsg, options.SystemMessage)
	}
}

func TestWithResponseFormatOption(t *testing.T) {
	format := interfaces.ResponseFormat{
		Name: "test_format",
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"answer": map[string]interface{}{
					"type": "string",
				},
			},
		},
	}
	options := &interfaces.GenerateOptions{}

	WithResponseFormat(format)(options)

	if options.ResponseFormat == nil {
		t.Error("Expected ResponseFormat to be set")
		return
	}

	if options.ResponseFormat.Name != format.Name {
		t.Errorf("Expected format name %s, got %s", format.Name, options.ResponseFormat.Name)
	}
}

func TestWithReasoningOption(t *testing.T) {
	reasoning := "comprehensive"
	options := &interfaces.GenerateOptions{}

	WithReasoning(reasoning)(options)

	if options.LLMConfig == nil {
		t.Error("Expected LLMConfig to be initialized")
		return
	}

	if options.LLMConfig.Reasoning != reasoning {
		t.Errorf("Expected reasoning %s, got %s", reasoning, options.LLMConfig.Reasoning)
	}
}

func TestConvertToOpenAISchema(t *testing.T) {
	client := NewClient("test-key", "https://test.openai.azure.com", "test-deployment")

	params := map[string]interfaces.ParameterSpec{
		"query": {
			Type:        "string",
			Description: "The search query",
			Required:    true,
		},
		"limit": {
			Type:        "integer",
			Description: "Maximum number of results",
			Required:    false,
			Default:     10,
		},
		"categories": {
			Type:        "array",
			Description: "Categories to search in",
			Required:    false,
			Items: &interfaces.ParameterSpec{
				Type: "string",
				Enum: []interface{}{"news", "blogs", "docs"},
			},
		},
	}

	schema := client.convertToOpenAISchema(params)

	// Check basic structure
	if schema["type"] != "object" {
		t.Errorf("Expected type 'object', got %v", schema["type"])
	}

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Error("Expected properties to be a map")
		return
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Error("Expected required to be a string slice")
		return
	}

	// Check query parameter
	queryProp, ok := properties["query"].(map[string]interface{})
	if !ok {
		t.Error("Expected query property to be a map")
		return
	}

	if queryProp["type"] != "string" {
		t.Errorf("Expected query type 'string', got %v", queryProp["type"])
	}

	if queryProp["description"] != "The search query" {
		t.Errorf("Expected query description 'The search query', got %v", queryProp["description"])
	}

	// Check required fields
	if len(required) != 1 || required[0] != "query" {
		t.Errorf("Expected required fields ['query'], got %v", required)
	}

	// Check limit parameter with default
	limitProp, ok := properties["limit"].(map[string]interface{})
	if !ok {
		t.Error("Expected limit property to be a map")
		return
	}

	if limitProp["default"] != 10 {
		t.Errorf("Expected limit default 10, got %v", limitProp["default"])
	}

	// Check categories parameter with items
	categoriesProp, ok := properties["categories"].(map[string]interface{})
	if !ok {
		t.Error("Expected categories property to be a map")
		return
	}

	items, ok := categoriesProp["items"].(map[string]interface{})
	if !ok {
		t.Error("Expected categories items to be a map")
		return
	}

	if items["type"] != "string" {
		t.Errorf("Expected categories items type 'string', got %v", items["type"])
	}

	enum, ok := items["enum"].([]interface{})
	if !ok {
		t.Error("Expected categories items enum to be a slice")
		return
	}

	expectedEnum := []interface{}{"news", "blogs", "docs"}
	if len(enum) != len(expectedEnum) {
		t.Errorf("Expected enum length %d, got %d", len(expectedEnum), len(enum))
		return
	}

	for i, val := range expectedEnum {
		if enum[i] != val {
			t.Errorf("Expected enum value %v at index %d, got %v", val, i, enum[i])
		}
	}
}

// Integration test that would require actual Azure OpenAI credentials
// This is commented out as it requires real API access
/*
func TestGenerateIntegration(t *testing.T) {
	// Skip if no API key is provided
	apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	baseURL := os.Getenv("AZURE_OPENAI_BASE_URL")
	deployment := os.Getenv("AZURE_OPENAI_DEPLOYMENT")

	if apiKey == "" || baseURL == "" || deployment == "" {
		t.Skip("Skipping integration test: AZURE_OPENAI_API_KEY, AZURE_OPENAI_BASE_URL, or AZURE_OPENAI_DEPLOYMENT not set")
	}

	client := NewClient(apiKey, baseURL, deployment)

	ctx := context.Background()
	response, err := client.Generate(ctx, "Say hello in one word")

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if response == "" {
		t.Error("Expected non-empty response")
	}

	t.Logf("Response: %s", response)
}
*/
