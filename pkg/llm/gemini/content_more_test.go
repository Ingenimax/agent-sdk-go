package gemini

import (
	"context"
	"errors"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// errorMemory fails every GetMessages call.
type errorMemory struct{}

func (errorMemory) AddMessage(context.Context, interfaces.Message) error { return nil }
func (errorMemory) GetMessages(context.Context, ...interfaces.GetMessagesOption) ([]interfaces.Message, error) {
	return nil, errors.New("memory down")
}
func (errorMemory) Clear(context.Context) error { return nil }

func TestUserContentWithoutPartsIsTheText(t *testing.T) {
	content := userContent("hello", nil)
	if content.Role != "user" || len(content.Parts) != 1 || content.Parts[0].Text != "hello" {
		t.Errorf("userContent() = %+v, want one user text part", content)
	}
}

func TestUserContentKeepsEmptyTextAndFileFields(t *testing.T) {
	content := userContent("ignored", []*interfaces.Part{
		interfaces.NewPartFromText(""),
		interfaces.NewPartFromURI("https://example.com/a.mp4", "video/mp4"),
		interfaces.NewPartFromBytes([]byte{1, 2, 3}, "audio/wav"),
	})
	if content.Role != "user" || len(content.Parts) != 3 {
		t.Fatalf("userContent() = %+v, want three user parts", content)
	}
	if p := content.Parts[0]; p.Text != "" || p.InlineData != nil || p.FileData != nil {
		t.Errorf("part 0 = %+v, want an empty text part", p)
	}
	if file := content.Parts[1].FileData; file == nil || file.FileURI != "https://example.com/a.mp4" || file.MIMEType != "video/mp4" {
		t.Errorf("part 1 = %+v, want the video URI", content.Parts[1])
	}
	if blob := content.Parts[2].InlineData; blob == nil || blob.MIMEType != "audio/wav" || len(blob.Data) != 3 {
		t.Errorf("part 2 = %+v, want the inline audio", content.Parts[2])
	}
}

func TestBuildContentsWithoutMemoryOrParts(t *testing.T) {
	contents := newMessageHistoryBuilder(logging.New()).buildContents(context.Background(), "hi", &interfaces.GenerateOptions{})
	if len(contents) != 1 || contents[0].Parts[0].Text != "hi" {
		t.Errorf("contents = %+v, want the prompt", contents)
	}
}

func TestBuildContentsMemoryErrorGivesNoContents(t *testing.T) {
	params := &interfaces.GenerateOptions{Memory: errorMemory{}}
	interfaces.WithParts(interfaces.NewPartFromText("ignored"))(params)
	contents := newMessageHistoryBuilder(logging.New()).buildContents(context.Background(), "hi", params)
	if len(contents) != 0 {
		t.Errorf("contents = %+v, want none when memory fails", contents)
	}
}

func TestBuildContentsFromMemoryKeepsUserParts(t *testing.T) {
	mem := &mockMemory{messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "describe", Parts: []*interfaces.Part{
			interfaces.NewPartFromURI("gs://bucket/a.png", "image/png"),
			interfaces.NewPartFromText("describe"),
		}},
		{Role: interfaces.MessageRoleAssistant, Content: "a cat"},
		{Role: interfaces.MessageRoleUser, Content: "what color?"},
	}}
	params := &interfaces.GenerateOptions{Memory: mem}
	// Parts set with WithParts are ignored when memory is set.
	interfaces.WithParts(interfaces.NewPartFromText("ignored"))(params)
	contents := newMessageHistoryBuilder(logging.New()).buildContents(context.Background(), "what color?", params)
	if len(contents) != 3 {
		t.Fatalf("got %d contents, want 3", len(contents))
	}
	if file := contents[0].Parts[0].FileData; file == nil || file.FileURI != "gs://bucket/a.png" {
		t.Errorf("content 0 = %+v, want the image URI first", contents[0])
	}
	if contents[1].Role != "model" || contents[1].Parts[0].Text != "a cat" {
		t.Errorf("content 1 = %+v, want the model reply", contents[1])
	}
	if contents[2].Role != "user" || len(contents[2].Parts) != 1 || contents[2].Parts[0].Text != "what color?" {
		t.Errorf("content 2 = %+v, want the text-only user turn", contents[2])
	}
}

func TestConvertMemoryMessageCases(t *testing.T) {
	builder := newMessageHistoryBuilder(logging.New())

	if got := builder.convertMemoryMessage(interfaces.Message{Role: interfaces.MessageRoleAssistant}); got != nil {
		t.Errorf("empty assistant message = %+v, want nil", got)
	}
	if got := builder.convertMemoryMessage(interfaces.Message{Role: interfaces.MessageRoleTool, Content: "r"}); got != nil {
		t.Errorf("tool message without a call id = %+v, want nil", got)
	}
	if got := builder.convertMemoryMessage(interfaces.Message{Role: "other", Content: "x"}); got != nil {
		t.Errorf("unknown role = %+v, want nil", got)
	}

	tool := builder.convertMemoryMessage(interfaces.Message{Role: interfaces.MessageRoleTool, Content: "r", ToolCallID: "c1"})
	if tool == nil || tool.Role != "user" || tool.Parts[0].FunctionResponse == nil {
		t.Fatalf("tool message = %+v, want a user function response", tool)
	}
	if resp := tool.Parts[0].FunctionResponse; resp.Name != "unknown" || resp.Response["result"] != "r" {
		t.Errorf("function response = %+v, want name unknown and the result", resp)
	}

	system := builder.convertMemoryMessage(interfaces.Message{Role: interfaces.MessageRoleSystem, Content: "summary"})
	if system == nil || system.Role != "user" || system.Parts[0].Text != "System: summary" {
		t.Errorf("system message = %+v, want a user turn prefixed with System:", system)
	}

	call := builder.convertMemoryMessage(interfaces.Message{
		Role:      interfaces.MessageRoleAssistant,
		ToolCalls: []interfaces.ToolCall{{Name: "search", Arguments: "not json"}},
	})
	if call == nil || len(call.Parts) != 1 {
		t.Fatalf("tool call message = %+v, want one function call part and no text", call)
	}
	if fc := call.Parts[0].FunctionCall; fc == nil || fc.Name != "search" || fc.Args == nil || len(fc.Args) != 0 {
		t.Errorf("function call = %+v, want empty args for bad JSON", call.Parts[0].FunctionCall)
	}

	userWithParts := builder.convertMemoryMessage(interfaces.Message{
		Role:    interfaces.MessageRoleUser,
		Content: "ignored",
		Parts:   []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1}, "application/pdf")},
	})
	if len(userWithParts.Parts) != 1 || userWithParts.Parts[0].InlineData == nil {
		t.Errorf("user message = %+v, want the parts in place of the content", userWithParts)
	}
}
