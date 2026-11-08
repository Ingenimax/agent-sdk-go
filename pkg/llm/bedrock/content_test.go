package bedrock

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

func TestBuildMessagesUsesPartsWithoutMemory(t *testing.T) {
	parts := []*interfaces.Part{
		interfaces.NewPartFromBytes([]byte{1, 2}, "image/png"),
		interfaces.NewPartFromText("describe"),
	}
	messages := newTestClient().buildMessages(context.Background(), "describe", nil, parts)
	if len(messages) != 1 || len(messages[0].Content) != 2 {
		t.Fatalf("messages = %+v, want one turn with an image and text", messages)
	}
	img, ok := messages[0].Content[0].(*types.ContentBlockMemberImage)
	if !ok {
		t.Fatalf("block 0 = %T, want an image", messages[0].Content[0])
	}
	if img.Value.Format != types.ImageFormatPng {
		t.Errorf("format = %q, want png", img.Value.Format)
	}
	if src, ok := img.Value.Source.(*types.ImageSourceMemberBytes); !ok || len(src.Value) != 2 {
		t.Errorf("source = %+v, want the image bytes", img.Value.Source)
	}
	if text, ok := messages[0].Content[1].(*types.ContentBlockMemberText); !ok || text.Value != "describe" {
		t.Errorf("block 1 = %+v, want the text", messages[0].Content[1])
	}
}

func TestBuildMessagesKeepsPartsFromMemory(t *testing.T) {
	mem := stubMemory{messages: []interfaces.Message{{
		Role:    interfaces.MessageRoleUser,
		Content: "describe",
		Parts: []*interfaces.Part{
			interfaces.NewPartFromBytes([]byte{1, 2}, "image/png"),
			interfaces.NewPartFromText("describe"),
		},
	}}}
	messages := newTestClient().buildMessages(context.Background(), "describe", mem, nil)
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1 (coalesced)", len(messages))
	}
	var images int
	for _, block := range messages[0].Content {
		if _, ok := block.(*types.ContentBlockMemberImage); ok {
			images++
		}
	}
	if images != 1 {
		t.Errorf("turn has %d images, want 1", images)
	}
}

func TestCheckContentRefusesWhatConverseCannotTake(t *testing.T) {
	for _, part := range []*interfaces.Part{
		interfaces.NewPartFromURI("https://example.com/a.png", "image/png"),
		interfaces.NewPartFromBytes([]byte{1}, "image/bmp"),
		interfaces.NewPartFromBytes([]byte{1}, "application/zip"),
	} {
		params := &interfaces.GenerateOptions{}
		interfaces.WithParts(part)(params)
		if err := checkContent(context.Background(), params); !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("checkContent(%s) = %v, want ErrContentUnsupported", part.MIMEType(), err)
		}
	}
}

func TestCheckContentAcceptsConverseMedia(t *testing.T) {
	pdf := interfaces.NewPartFromBytes([]byte{1}, "application/pdf")
	pdf.InlineData.DisplayName = "report.pdf"
	for _, part := range []*interfaces.Part{
		interfaces.NewPartFromBytes([]byte{1}, "image/webp"),
		interfaces.NewPartFromURI("s3://bucket/a.png", "image/png"),
		pdf,
		interfaces.NewPartFromBytes([]byte{1}, "video/mp4"),
		interfaces.NewPartFromBytes([]byte{1}, "audio/mpeg"),
	} {
		params := &interfaces.GenerateOptions{}
		interfaces.WithParts(part, interfaces.NewPartFromText("describe"))(params)
		if err := checkContent(context.Background(), params); err != nil {
			t.Errorf("checkContent(%s) = %v", part.MIMEType(), err)
		}
	}
}

func TestCheckContentRefusesDocumentWithoutName(t *testing.T) {
	for _, name := range []string{"", "..."} {
		pdf := interfaces.NewPartFromBytes([]byte{1}, "application/pdf")
		pdf.InlineData.DisplayName = name
		params := &interfaces.GenerateOptions{}
		interfaces.WithParts(pdf, interfaces.NewPartFromText("summarize"))(params)
		if err := checkContent(context.Background(), params); !errors.Is(err, errDocumentName) {
			t.Errorf("checkContent(name %q) = %v, want errDocumentName", name, err)
		}
	}
}

func TestDocumentName(t *testing.T) {
	cases := map[string]string{
		"report.pdf":             "report pdf",
		"Q3  results (final)":    "Q3 results (final)",
		"[draft]-notes_v2.docx":  "[draft]-notes v2 docx",
		"résumé.pdf":             "r sum pdf",
		strings.Repeat("a", 250): strings.Repeat("a", 200),
	}
	for in, want := range cases {
		part := interfaces.NewPartFromBytes([]byte{1}, "application/pdf")
		part.InlineData.DisplayName = in
		if got := documentName(part); got != want {
			t.Errorf("documentName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPartBlocksBuildsEachKind(t *testing.T) {
	pdf := interfaces.NewPartFromURI("s3://bucket/report.pdf", "application/pdf")
	pdf.FileData.DisplayName = "report.pdf"
	blocks := partBlocks([]*interfaces.Part{
		pdf,
		interfaces.NewPartFromBytes([]byte{1}, "video/quicktime"),
		interfaces.NewPartFromBytes([]byte{1}, "audio/wav"),
		interfaces.NewPartFromURI("s3://bucket/a.jpg", "image/jpeg"),
	})
	if len(blocks) != 4 {
		t.Fatalf("got %d blocks, want 4", len(blocks))
	}

	doc, ok := blocks[0].(*types.ContentBlockMemberDocument)
	if !ok {
		t.Fatalf("block 0 = %T, want a document", blocks[0])
	}
	src, ok := doc.Value.Source.(*types.DocumentSourceMemberS3Location)
	if !ok || *src.Value.Uri != "s3://bucket/report.pdf" {
		t.Errorf("document source = %+v, want the S3 location", doc.Value.Source)
	}
	if doc.Value.Format != types.DocumentFormatPdf || *doc.Value.Name != "report pdf" {
		t.Errorf("document = %+v, want a pdf named %q", doc.Value, "report pdf")
	}

	if video, ok := blocks[1].(*types.ContentBlockMemberVideo); !ok || video.Value.Format != types.VideoFormatMov {
		t.Errorf("block 1 = %+v, want a mov video", blocks[1])
	}
	if audio, ok := blocks[2].(*types.ContentBlockMemberAudio); !ok || audio.Value.Format != types.AudioFormatWav {
		t.Errorf("block 2 = %+v, want a wav audio", blocks[2])
	}
	img, ok := blocks[3].(*types.ContentBlockMemberImage)
	if !ok {
		t.Fatalf("block 3 = %T, want an image", blocks[3])
	}
	if _, ok := img.Value.Source.(*types.ImageSourceMemberS3Location); !ok {
		t.Errorf("image source = %T, want an S3 location", img.Value.Source)
	}
}

func TestCheckContentRefusesDuplicateDocumentNames(t *testing.T) {
	first := interfaces.NewPartFromBytes([]byte{1}, "application/pdf")
	first.InlineData.DisplayName = "a.pdf"
	second := interfaces.NewPartFromBytes([]byte{2}, "application/pdf")
	second.InlineData.DisplayName = "a pdf"
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(first, second, interfaces.NewPartFromText("compare"))(params)
	if err := checkContent(context.Background(), params); err == nil || !strings.Contains(err.Error(), "unique") {
		t.Errorf("checkContent() = %v, want a duplicate name error", err)
	}
}

func TestCheckContentRefusesDocumentWithoutText(t *testing.T) {
	pdf := interfaces.NewPartFromBytes([]byte{1}, "application/pdf")
	pdf.InlineData.DisplayName = "report"
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(pdf)(params)
	if err := checkContent(context.Background(), params); err == nil || !strings.Contains(err.Error(), "text part") {
		t.Errorf("checkContent() = %v, want a missing text error", err)
	}

	mem := stubMemory{messages: []interfaces.Message{{Role: interfaces.MessageRoleUser, Parts: []*interfaces.Part{pdf}}}}
	if err := checkContent(context.Background(), &interfaces.GenerateOptions{Memory: mem}); err == nil {
		t.Error("checkContent() with a memory document and no text = nil, want an error")
	}
}
