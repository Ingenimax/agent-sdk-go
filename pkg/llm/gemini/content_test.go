package gemini

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

func TestUserContentSendsPartsInOrder(t *testing.T) {
	content := userContent("ignored", []*interfaces.Part{
		interfaces.NewPartFromText("describe"),
		interfaces.NewPartFromBytes([]byte{1, 2}, "application/pdf"),
		interfaces.NewPartFromURI("gs://bucket/a.jpg", "image/jpeg"),
	})
	if len(content.Parts) != 3 {
		t.Fatalf("got %d parts, want 3", len(content.Parts))
	}
	if content.Parts[0].Text != "describe" {
		t.Errorf("part 0 = %+v, want the text", content.Parts[0])
	}
	if blob := content.Parts[1].InlineData; blob == nil || blob.MIMEType != "application/pdf" || len(blob.Data) != 2 {
		t.Errorf("part 1 = %+v, want the inline PDF", content.Parts[1])
	}
	if file := content.Parts[2].FileData; file == nil || file.FileURI != "gs://bucket/a.jpg" || file.MIMEType != "image/jpeg" {
		t.Errorf("part 2 = %+v, want the file URI", content.Parts[2])
	}
}

func TestBuildContentsUsesPartsWithoutMemory(t *testing.T) {
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "image/png"), interfaces.NewPartFromText("what is this?"))(params)
	contents := newMessageHistoryBuilder(logging.New()).buildContents(context.Background(), "what is this?", params)
	if len(contents) != 1 || len(contents[0].Parts) != 2 || contents[0].Parts[0].InlineData == nil {
		t.Fatalf("contents = %+v, want one user turn with the image then the text", contents)
	}
}
