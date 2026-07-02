package bedrock

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestJoinText(t *testing.T) {
	blocks := []types.ContentBlock{
		&types.ContentBlockMemberText{Value: "Hello "},
		&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{Name: aws.String("x")}},
		&types.ContentBlockMemberText{Value: "world"},
	}
	if got := joinText(blocks); got != "Hello world" {
		t.Fatalf("joinText = %q, want %q", got, "Hello world")
	}
}

func TestToolUseBlocks(t *testing.T) {
	blocks := []types.ContentBlock{
		&types.ContentBlockMemberText{Value: "thinking"},
		&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{Name: aws.String("a"), ToolUseId: aws.String("1")}},
		&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{Name: aws.String("b"), ToolUseId: aws.String("2")}},
	}
	uses := toolUseBlocks(blocks)
	if len(uses) != 2 {
		t.Fatalf("toolUseBlocks len = %d, want 2", len(uses))
	}
	if aws.ToString(uses[0].Name) != "a" || aws.ToString(uses[1].Name) != "b" {
		t.Fatalf("unexpected tool uses: %+v", uses)
	}
}

func TestSystemBlocks(t *testing.T) {
	if systemBlocks("") != nil {
		t.Fatal("empty system should yield nil")
	}
	got := systemBlocks("be nice")
	if len(got) != 1 {
		t.Fatalf("systemBlocks len = %d, want 1", len(got))
	}
	if tb, ok := got[0].(*types.SystemContentBlockMemberText); !ok || tb.Value != "be nice" {
		t.Fatalf("unexpected system block: %+v", got[0])
	}
}

func TestInferenceConfig(t *testing.T) {
	ic := inferenceConfig(&interfaces.LLMConfig{Temperature: 0.5, TopP: 0.9, StopSequences: []string{"x"}})
	if aws.ToInt32(ic.MaxTokens) != DefaultMaxTokens {
		t.Fatalf("MaxTokens = %d, want %d", aws.ToInt32(ic.MaxTokens), DefaultMaxTokens)
	}
	if aws.ToFloat32(ic.Temperature) != 0.5 {
		t.Fatalf("Temperature = %v, want 0.5", aws.ToFloat32(ic.Temperature))
	}
	if aws.ToFloat32(ic.TopP) != 0.9 {
		t.Fatalf("TopP = %v, want 0.9", aws.ToFloat32(ic.TopP))
	}
	if len(ic.StopSequences) != 1 || ic.StopSequences[0] != "x" {
		t.Fatalf("StopSequences = %v", ic.StopSequences)
	}
}

func TestInferenceConfigTemperatureZeroIsExplicit(t *testing.T) {
	// A caller requesting WithTemperature(0) for deterministic output must
	// have that value passed through, not treated as "unset".
	ic := inferenceConfig(&interfaces.LLMConfig{Temperature: 0})
	if ic.Temperature == nil {
		t.Fatal("Temperature = nil, want explicit 0")
	}
	if aws.ToFloat32(ic.Temperature) != 0 {
		t.Fatalf("Temperature = %v, want 0", aws.ToFloat32(ic.Temperature))
	}
}

func TestTokenUsageFallbackTotal(t *testing.T) {
	u := tokenUsage(&types.TokenUsage{InputTokens: aws.Int32(3), OutputTokens: aws.Int32(4)})
	if u.TotalTokens != 7 {
		t.Fatalf("TotalTokens = %d, want 7 (fallback)", u.TotalTokens)
	}
}

func TestConvertToolsNilWhenEmpty(t *testing.T) {
	if convertTools(nil) != nil {
		t.Fatal("convertTools(nil) should be nil")
	}
}

func TestConvertToolsBuildsSpec(t *testing.T) {
	cfg := convertTools([]interfaces.Tool{stubTool{}})
	if cfg == nil || len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %+v", cfg)
	}
	spec, ok := cfg.Tools[0].(*types.ToolMemberToolSpec)
	if !ok {
		t.Fatalf("unexpected tool type %T", cfg.Tools[0])
	}
	if aws.ToString(spec.Value.Name) != "search" {
		t.Fatalf("tool name = %q, want search", aws.ToString(spec.Value.Name))
	}
	if _, ok := spec.Value.InputSchema.(*types.ToolInputSchemaMemberJson); !ok {
		t.Fatalf("unexpected input schema type %T", spec.Value.InputSchema)
	}
}

// stubMemory returns a fixed message history for buildMessages tests.
type stubMemory struct{ messages []interfaces.Message }

func (m stubMemory) AddMessage(context.Context, interfaces.Message) error { return nil }
func (m stubMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return m.messages, nil
}
func (m stubMemory) Clear(context.Context) error { return nil }

func newTestClient() *Client {
	return &Client{logger: logging.New()}
}

// textOf returns the joined text content of a Converse message, panicking if
// it isn't a single text block (the shape textMessage/appendAlternating produce).
func textOf(t *testing.T, m types.Message) string {
	t.Helper()
	tb, ok := m.Content[0].(*types.ContentBlockMemberText)
	if !ok {
		t.Fatalf("message content is not text: %T", m.Content[0])
	}
	return tb.Value
}

func TestBuildMessagesAlwaysStartsWithUser(t *testing.T) {
	c := newTestClient()
	mem := stubMemory{messages: []interfaces.Message{
		{Role: interfaces.MessageRoleAssistant, Content: "orphaned assistant turn"},
		{Role: interfaces.MessageRoleUser, Content: "hi"},
	}}
	messages := c.buildMessages(context.Background(), "follow up", mem)
	if messages[0].Role != types.ConversationRoleUser {
		t.Fatalf("first message role = %v, want user", messages[0].Role)
	}
}

func TestBuildMessagesCoalescesConsecutiveSameRoleTurns(t *testing.T) {
	c := newTestClient()
	// The assistant turn with tool calls is filtered out, which would
	// otherwise leave two consecutive user turns ("first", "follow up").
	mem := stubMemory{messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "first"},
		{Role: interfaces.MessageRoleAssistant, Content: "using a tool", ToolCalls: []interfaces.ToolCall{{Name: "search"}}},
	}}
	messages := c.buildMessages(context.Background(), "follow up", mem)

	for i := 1; i < len(messages); i++ {
		if messages[i].Role == messages[i-1].Role {
			t.Fatalf("messages[%d] and [%d] share role %v; alternation violated: %+v", i-1, i, messages[i].Role, messages)
		}
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1 (coalesced)", len(messages))
	}
	if got := textOf(t, messages[0]); got != "first\n\nfollow up" {
		t.Fatalf("coalesced content = %q", got)
	}
}

func TestBuildMessagesSkipsEmptyTurns(t *testing.T) {
	c := newTestClient()
	mem := stubMemory{messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: ""},
		{Role: interfaces.MessageRoleAssistant, Content: ""},
		{Role: interfaces.MessageRoleUser, Content: "hi"},
		{Role: interfaces.MessageRoleAssistant, Content: "hello"},
	}}
	messages := c.buildMessages(context.Background(), "how are you", mem)
	if len(messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3, got %+v", len(messages), messages)
	}
	if messages[0].Role != types.ConversationRoleUser || textOf(t, messages[0]) != "hi" {
		t.Fatalf("messages[0] = %+v", messages[0])
	}
	if messages[1].Role != types.ConversationRoleAssistant || textOf(t, messages[1]) != "hello" {
		t.Fatalf("messages[1] = %+v", messages[1])
	}
	if messages[2].Role != types.ConversationRoleUser || textOf(t, messages[2]) != "how are you" {
		t.Fatalf("messages[2] = %+v", messages[2])
	}
}

func TestExecuteToolEmptyOutputGetsPlaceholder(t *testing.T) {
	c := newTestClient()
	use := types.ToolUseBlock{Name: aws.String("search"), ToolUseId: aws.String("1")}
	block := c.executeTool(context.Background(), use, []interfaces.Tool{stubTool{}})

	tr, ok := block.(*types.ContentBlockMemberToolResult)
	if !ok {
		t.Fatalf("unexpected block type %T", block)
	}
	content, ok := tr.Value.Content[0].(*types.ToolResultContentBlockMemberText)
	if !ok {
		t.Fatalf("unexpected content type %T", tr.Value.Content[0])
	}
	if content.Value != "(no output)" {
		t.Fatalf("content = %q, want placeholder", content.Value)
	}
}

func TestWithResponseFormatInstructionsNilIsNoop(t *testing.T) {
	if got := withResponseFormatInstructions("be nice", nil); got != "be nice" {
		t.Fatalf("got %q, want unchanged system message", got)
	}
}

func TestWithResponseFormatInstructionsIncludesSchema(t *testing.T) {
	format := &interfaces.ResponseFormat{Schema: interfaces.JSONSchema{"type": "object"}}
	got := withResponseFormatInstructions("be nice", format)
	if !strings.HasPrefix(got, "be nice\n\n") {
		t.Fatalf("expected original system message preserved, got %q", got)
	}
	if !strings.Contains(got, "valid JSON") || !strings.Contains(got, `"type": "object"`) {
		t.Fatalf("expected schema instructions in %q", got)
	}
}

type stubTool struct{}

func (stubTool) Name() string                                        { return "search" }
func (stubTool) Description() string                                 { return "search the web" }
func (stubTool) Run(_ context.Context, _ string) (string, error)     { return "", nil }
func (stubTool) Execute(_ context.Context, _ string) (string, error) { return "", nil }
func (stubTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"q": {Type: "string", Description: "query", Required: true},
	}
}
