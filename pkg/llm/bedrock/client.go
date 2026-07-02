// Package bedrock implements the agent-sdk-go LLM interface on top of Amazon
// Bedrock's provider-agnostic Converse API (bedrockruntime.Converse). Unlike
// the OpenAI-compatible shim, Converse supports the full Bedrock model catalog
// (Anthropic, Meta, Amazon, Mistral, Cohere, …) with a single request/response
// shape, and it accepts inference-profile IDs (e.g. us.anthropic.claude-...).
//
// Authentication uses a Bedrock bearer API key (the AWS_BEARER_TOKEN_BEDROCK
// mechanism) carried on the aws.Config, so callers pass the key directly
// without wiring AWS SigV4 credentials.
package bedrock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brdoc "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	smithybearer "github.com/aws/smithy-go/auth/bearer"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/retry"
)

const (
	// DefaultRegion is the AWS region used when none is configured.
	DefaultRegion = "us-east-1"

	// DefaultMaxTokens caps the generated response when the caller does not
	// specify a limit. Converse requires a max-tokens value for some providers.
	DefaultMaxTokens = 4096

	// DefaultMaxIterations bounds the tool-calling loop.
	DefaultMaxIterations = 10

	providerName = "bedrock"
)

// Client implements interfaces.LLM for Amazon Bedrock via the Converse API.
type Client struct {
	client        *bedrockruntime.Client
	Model         string
	Region        string
	logger        logging.Logger
	retryExecutor *retry.Executor
}

// Option configures a Client.
type Option func(*clientOptions)

type clientOptions struct {
	model     string
	region    string
	logger    logging.Logger
	retry     *retry.Executor
	awsConfig *aws.Config
}

// WithModel sets the default model (inference-profile ID or model ID).
func WithModel(model string) Option {
	return func(o *clientOptions) { o.model = model }
}

// WithRegion sets the AWS region used to resolve the Bedrock endpoint.
func WithRegion(region string) Option {
	return func(o *clientOptions) {
		if region != "" {
			o.region = region
		}
	}
}

// WithLogger sets the logger.
func WithLogger(logger logging.Logger) Option {
	return func(o *clientOptions) { o.logger = logger }
}

// WithRetry configures a retry policy for Converse calls.
func WithRetry(opts ...retry.Option) Option {
	return func(o *clientOptions) { o.retry = retry.NewExecutor(retry.NewPolicy(opts...)) }
}

// WithAWSConfig lets callers supply a fully-formed aws.Config (credentials,
// endpoint, region). When set it overrides bearer-token/region wiring.
func WithAWSConfig(cfg aws.Config) Option {
	return func(o *clientOptions) { o.awsConfig = &cfg }
}

// NewClient builds a Bedrock Converse client authenticated with a bearer API
// key. Pass WithAWSConfig instead to use standard AWS credentials.
func NewClient(ctx context.Context, apiKey string, options ...Option) (*Client, error) {
	opts := &clientOptions{
		region: DefaultRegion,
		logger: logging.New(),
	}
	for _, option := range options {
		option(opts)
	}

	var awsCfg aws.Config
	if opts.awsConfig != nil {
		awsCfg = *opts.awsConfig
	} else {
		if apiKey == "" {
			return nil, fmt.Errorf("bedrock: api key must not be empty")
		}
		loaded, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(opts.region),
			config.WithBearerAuthTokenProvider(smithybearer.StaticTokenProvider{
				Token: smithybearer.Token{Value: apiKey},
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("bedrock: failed to load AWS config: %w", err)
		}
		awsCfg = loaded
		// bedrockruntime lists SigV4 first and HTTP bearer second, so without a
		// preference the client picks SigV4 and falls back to the default AWS
		// credential chain (which errors when none is configured, e.g. expired
		// SSO). Setting the bearer token alone is not enough; we must also make
		// the bearer scheme win. The SDK only does this automatically when
		// AWS_BEARER_TOKEN_BEDROCK is set in the environment.
		awsCfg.AuthSchemePreference = []string{"httpBearerAuth"}
	}
	if awsCfg.Region == "" {
		awsCfg.Region = opts.region
	}

	c := &Client{
		client:        bedrockruntime.NewFromConfig(awsCfg),
		Model:         opts.model,
		Region:        awsCfg.Region,
		logger:        opts.logger,
		retryExecutor: opts.retry,
	}
	return c, nil
}

// Name returns the provider name.
func (c *Client) Name() string { return providerName }

// SupportsStreaming reports streaming support. The Converse (non-streaming)
// path is implemented; streaming (ConverseStream) is a follow-up.
func (c *Client) SupportsStreaming() bool { return false }

// Generate returns the model's text response for the prompt.
func (c *Client) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	resp, err := c.GenerateDetailed(ctx, prompt, options...)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// GenerateDetailed returns the model response with token usage.
func (c *Client) GenerateDetailed(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	params := defaultParams()
	for _, option := range options {
		option(params)
	}

	messages := c.buildMessages(ctx, prompt, params.Memory)
	input := &bedrockruntime.ConverseInput{
		ModelId:         aws.String(c.Model),
		Messages:        messages,
		System:          systemBlocks(withResponseFormatInstructions(params.SystemMessage, params.ResponseFormat)),
		InferenceConfig: inferenceConfig(params.LLMConfig),
	}

	out, err := c.converse(ctx, input)
	if err != nil {
		return nil, err
	}

	msg, ok := assistantMessage(out)
	if !ok {
		return nil, fmt.Errorf("bedrock: empty response from Converse")
	}
	return &interfaces.LLMResponse{
		Content:    joinText(msg.Content),
		Model:      c.Model,
		StopReason: string(out.StopReason),
		Usage:      tokenUsage(out.Usage),
		Metadata:   map[string]interface{}{"provider": providerName},
	}, nil
}

// GenerateWithTools returns the model's text response, running tools as needed.
func (c *Client) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	resp, err := c.GenerateWithToolsDetailed(ctx, prompt, tools, options...)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// GenerateWithToolsDetailed runs the Converse tool-use loop and returns the
// final response with accumulated token usage.
//
// DisableFinalSummary does not apply here: unlike anthropic/openai, this loop
// has no separate "final summary" call — it returns the last Converse
// response as soon as the model stops requesting tools.
func (c *Client) GenerateWithToolsDetailed(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	params := defaultParams()
	for _, option := range options {
		option(params)
	}

	maxIterations := params.MaxIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}

	messages := c.buildMessages(ctx, prompt, params.Memory)
	system := systemBlocks(withResponseFormatInstructions(params.SystemMessage, params.ResponseFormat))
	inference := inferenceConfig(params.LLMConfig)
	toolConfig := convertTools(tools)

	var totalIn, totalOut int32

	for iteration := 0; iteration < maxIterations; iteration++ {
		input := &bedrockruntime.ConverseInput{
			ModelId:         aws.String(c.Model),
			Messages:        messages,
			System:          system,
			InferenceConfig: inference,
			ToolConfig:      toolConfig,
		}

		out, err := c.converse(ctx, input)
		if err != nil {
			return nil, err
		}
		msg, ok := assistantMessage(out)
		if !ok {
			return nil, fmt.Errorf("bedrock: empty response from Converse")
		}
		if out.Usage != nil {
			// Each Converse call is billed independently for the full
			// (growing) conversation it resends, so summing per-call usage
			// across iterations reflects total tokens billed, not a
			// double-count of a single logical request.
			totalIn += aws.ToInt32(out.Usage.InputTokens)
			totalOut += aws.ToInt32(out.Usage.OutputTokens)
		}

		toolUses := toolUseBlocks(msg.Content)
		if len(toolUses) == 0 || out.StopReason != types.StopReasonToolUse {
			return &interfaces.LLMResponse{
				Content:    joinText(msg.Content),
				Model:      c.Model,
				StopReason: string(out.StopReason),
				Usage: &interfaces.TokenUsage{
					InputTokens:  int(totalIn),
					OutputTokens: int(totalOut),
					TotalTokens:  int(totalIn + totalOut),
				},
				Metadata: map[string]interface{}{"provider": providerName, "iterations": iteration + 1},
			}, nil
		}

		// Append the assistant message (with tool-use blocks), then a user
		// message carrying the tool results, per the Converse protocol.
		messages = append(messages, msg)
		results := c.executeToolsParallel(ctx, toolUses, tools)
		messages = append(messages, types.Message{
			Role:    types.ConversationRoleUser,
			Content: results,
		})
	}

	return nil, fmt.Errorf("bedrock: exceeded max tool iterations (%d)", maxIterations)
}

// converse runs a single Converse call with optional retry.
func (c *Client) converse(ctx context.Context, input *bedrockruntime.ConverseInput) (*bedrockruntime.ConverseOutput, error) {
	var out *bedrockruntime.ConverseOutput
	op := func() error {
		var err error
		out, err = c.client.Converse(ctx, input)
		if err != nil {
			c.logger.Error(ctx, "Bedrock Converse call failed", map[string]interface{}{
				"error": err.Error(),
				"model": c.Model,
			})
			return fmt.Errorf("failed to invoke Bedrock model: %w", err)
		}
		return nil
	}
	if c.retryExecutor != nil {
		if err := c.retryExecutor.Execute(ctx, op); err != nil {
			return nil, err
		}
		return out, nil
	}
	if err := op(); err != nil {
		return nil, err
	}
	return out, nil
}

func defaultParams() *interfaces.GenerateOptions {
	return &interfaces.GenerateOptions{
		LLMConfig: &interfaces.LLMConfig{Temperature: 0.7},
	}
}

// buildMessages replays prior user/assistant text turns from memory (tool-call
// history is omitted to avoid invalid Converse tool sequences) and appends the
// current prompt as a user message.
//
// Converse requires messages to start with a user turn and to strictly
// alternate user/assistant. Since tool-call and empty turns are filtered out,
// naively appending survivors can produce consecutive same-role turns (or a
// leading assistant turn); appendAlternating coalesces those into a single
// turn, and any leading non-user turn is trimmed, to keep the sequence valid.
func (c *Client) buildMessages(ctx context.Context, prompt string, memory interfaces.Memory) []types.Message {
	var messages []types.Message
	if memory != nil {
		history, err := memory.GetMessages(ctx)
		if err != nil {
			c.logger.Warn(ctx, "Bedrock: failed to read memory, continuing without history", map[string]interface{}{
				"error": err.Error(),
			})
		}
		for _, m := range history {
			switch m.Role {
			case interfaces.MessageRoleUser:
				if m.Content != "" {
					messages = appendAlternating(messages, types.ConversationRoleUser, m.Content)
				}
			case interfaces.MessageRoleAssistant:
				if m.Content != "" && len(m.ToolCalls) == 0 {
					messages = appendAlternating(messages, types.ConversationRoleAssistant, m.Content)
				}
			}
		}
	}
	messages = appendAlternating(messages, types.ConversationRoleUser, prompt)

	for len(messages) > 0 && messages[0].Role != types.ConversationRoleUser {
		messages = messages[1:]
	}
	return messages
}

// appendAlternating appends a text turn, merging it into the previous turn
// when both share the same role so the sequence keeps Converse's required
// strict user/assistant alternation instead of producing two consecutive
// same-role messages.
func appendAlternating(messages []types.Message, role types.ConversationRole, text string) []types.Message {
	if len(messages) > 0 && messages[len(messages)-1].Role == role {
		last := messages[len(messages)-1]
		if tb, ok := last.Content[0].(*types.ContentBlockMemberText); ok {
			tb.Value += "\n\n" + text
			return messages
		}
	}
	return append(messages, textMessage(role, text))
}

func textMessage(role types.ConversationRole, text string) types.Message {
	return types.Message{
		Role:    role,
		Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: text}},
	}
}

// withResponseFormatInstructions appends structured-output instructions to
// the system prompt when a ResponseFormat is requested. Converse has no
// native structured-output field, so — like the anthropic client — Bedrock
// gets JSON compliance via prompting rather than a request parameter.
func withResponseFormatInstructions(system string, format *interfaces.ResponseFormat) string {
	if format == nil {
		return system
	}
	instructions := "You must respond with valid JSON that matches the requested schema. " +
		"Return ONLY the raw JSON object, with no markdown formatting, code fences, or wrapper text."
	if schemaJSON, err := json.MarshalIndent(format.Schema, "", "  "); err == nil {
		instructions += "\n\nSchema:\n" + string(schemaJSON)
	}
	if system == "" {
		return instructions
	}
	return system + "\n\n" + instructions
}

func systemBlocks(system string) []types.SystemContentBlock {
	if system == "" {
		return nil
	}
	return []types.SystemContentBlock{&types.SystemContentBlockMemberText{Value: system}}
}

// inferenceConfig maps the subset of LLMConfig that Converse's
// InferenceConfiguration supports. FrequencyPenalty, PresencePenalty, and the
// reasoning fields (Reasoning, EnableReasoning, ReasoningBudget) have no
// Converse equivalent and are intentionally left as silent no-ops; callers
// should not assume they take effect on this provider.
func inferenceConfig(cfg *interfaces.LLMConfig) *types.InferenceConfiguration {
	ic := &types.InferenceConfiguration{MaxTokens: aws.Int32(DefaultMaxTokens)}
	if cfg == nil {
		return ic
	}
	// Temperature always has a resolved value by the time it reaches here
	// (defaultParams seeds 0.7, WithTemperature overrides it), so 0 is a
	// deliberate request for deterministic output, not "unset" — unlike TopP
	// below, which has no seeded default and genuinely means "unset" at 0.
	ic.Temperature = aws.Float32(float32(cfg.Temperature))
	if cfg.TopP != 0 {
		ic.TopP = aws.Float32(float32(cfg.TopP))
	}
	if len(cfg.StopSequences) > 0 {
		ic.StopSequences = cfg.StopSequences
	}
	return ic
}

func assistantMessage(out *bedrockruntime.ConverseOutput) (types.Message, bool) {
	if out == nil {
		return types.Message{}, false
	}
	m, ok := out.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return types.Message{}, false
	}
	return m.Value, true
}

func joinText(blocks []types.ContentBlock) string {
	var out string
	for _, b := range blocks {
		if t, ok := b.(*types.ContentBlockMemberText); ok {
			out += t.Value
		}
	}
	return out
}

func toolUseBlocks(blocks []types.ContentBlock) []types.ToolUseBlock {
	var uses []types.ToolUseBlock
	for _, b := range blocks {
		if u, ok := b.(*types.ContentBlockMemberToolUse); ok {
			uses = append(uses, u.Value)
		}
	}
	return uses
}

func tokenUsage(u *types.TokenUsage) *interfaces.TokenUsage {
	if u == nil {
		return nil
	}
	in := int(aws.ToInt32(u.InputTokens))
	outTok := int(aws.ToInt32(u.OutputTokens))
	total := int(aws.ToInt32(u.TotalTokens))
	if total == 0 {
		total = in + outTok
	}
	return &interfaces.TokenUsage{InputTokens: in, OutputTokens: outTok, TotalTokens: total}
}

// convertTools maps SDK tools to a Converse ToolConfiguration. Returns nil when
// there are no tools so Converse is called without tool config.
func convertTools(tools []interfaces.Tool) *types.ToolConfiguration {
	if len(tools) == 0 {
		return nil
	}
	converted := make([]types.Tool, 0, len(tools))
	for _, tool := range tools {
		properties := map[string]interface{}{}
		var required []string
		for name, param := range tool.Parameters() {
			prop := map[string]interface{}{"type": param.Type}
			if param.Description != "" {
				prop["description"] = param.Description
			}
			if param.Default != nil {
				prop["default"] = param.Default
			}
			if param.Enum != nil {
				prop["enum"] = param.Enum
			}
			if param.Items != nil {
				items := map[string]interface{}{"type": param.Items.Type}
				if param.Items.Enum != nil {
					items["enum"] = param.Items.Enum
				}
				prop["items"] = items
			}
			properties[name] = prop
			if param.Required {
				required = append(required, name)
			}
		}
		schema := map[string]interface{}{
			"type":       "object",
			"properties": properties,
		}
		if len(required) > 0 {
			schema["required"] = required
		}
		converted = append(converted, &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(tool.Name()),
				Description: aws.String(tool.Description()),
				InputSchema: &types.ToolInputSchemaMemberJson{Value: brdoc.NewLazyDocument(schema)},
			},
		})
	}
	return &types.ToolConfiguration{Tools: converted}
}

// executeToolsParallel runs the requested tool uses concurrently and returns
// the tool-result content blocks in request order.
func (c *Client) executeToolsParallel(ctx context.Context, toolUses []types.ToolUseBlock, tools []interfaces.Tool) []types.ContentBlock {
	results := make([]types.ContentBlock, len(toolUses))
	var wg sync.WaitGroup

	for i, use := range toolUses {
		wg.Add(1)
		go func(index int, u types.ToolUseBlock) {
			defer wg.Done()
			results[index] = c.executeTool(ctx, u, tools)
		}(i, use)
	}
	wg.Wait()
	return results
}

func (c *Client) executeTool(ctx context.Context, use types.ToolUseBlock, tools []interfaces.Tool) types.ContentBlock {
	name := aws.ToString(use.Name)
	toolResult := types.ToolResultBlock{ToolUseId: use.ToolUseId}

	var tool interfaces.Tool
	for _, t := range tools {
		if t.Name() == name {
			tool = t
			break
		}
	}
	if tool == nil {
		toolResult.Status = types.ToolResultStatusError
		toolResult.Content = []types.ToolResultContentBlock{
			&types.ToolResultContentBlockMemberText{Value: fmt.Sprintf("Error: tool %q not found", name)},
		}
		return &types.ContentBlockMemberToolResult{Value: toolResult}
	}

	args := toolInputJSON(use.Input)
	c.logger.Info(ctx, "Executing tool", map[string]interface{}{"tool_name": name, "arguments": args})

	content, err := tool.Execute(ctx, args)
	if err != nil {
		c.logger.Error(ctx, "Tool execution failed", map[string]interface{}{"tool_name": name, "error": err.Error()})
		toolResult.Status = types.ToolResultStatusError
		content = fmt.Sprintf("Error executing tool: %v", err)
	}
	if content == "" {
		// Converse rejects tool-result text blocks with empty content; a
		// successful tool can legitimately return "", so substitute a
		// placeholder rather than fail the whole iteration.
		content = "(no output)"
	}
	toolResult.Content = []types.ToolResultContentBlock{
		&types.ToolResultContentBlockMemberText{Value: content},
	}
	return &types.ContentBlockMemberToolResult{Value: toolResult}
}

// toolInputJSON renders the Converse tool-use input document as a JSON string
// for the SDK tool's Execute(args string) contract.
func toolInputJSON(input brdoc.Interface) string {
	if input == nil {
		return "{}"
	}
	var m map[string]interface{}
	if err := input.UnmarshalSmithyDocument(&m); err != nil {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}
