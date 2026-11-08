package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	brdoc "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestFormatMapsCoverEveryMIMEType(t *testing.T) {
	images := map[string]types.ImageFormat{
		"image/png":  types.ImageFormatPng,
		"image/jpeg": types.ImageFormatJpeg,
		"image/gif":  types.ImageFormatGif,
		"image/webp": types.ImageFormatWebp,
	}
	documents := map[string]types.DocumentFormat{
		"application/pdf":    types.DocumentFormatPdf,
		"text/csv":           types.DocumentFormatCsv,
		"application/msword": types.DocumentFormatDoc,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": types.DocumentFormatDocx,
		"application/vnd.ms-excel": types.DocumentFormatXls,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": types.DocumentFormatXlsx,
		"text/html":     types.DocumentFormatHtml,
		"text/plain":    types.DocumentFormatTxt,
		"text/markdown": types.DocumentFormatMd,
	}
	videos := map[string]types.VideoFormat{
		"video/x-matroska": types.VideoFormatMkv,
		"video/quicktime":  types.VideoFormatMov,
		"video/mp4":        types.VideoFormatMp4,
		"video/webm":       types.VideoFormatWebm,
		"video/x-flv":      types.VideoFormatFlv,
		"video/mpeg":       types.VideoFormatMpeg,
		"video/x-ms-wmv":   types.VideoFormatWmv,
		"video/3gpp":       types.VideoFormatThreeGp,
	}
	audios := map[string]types.AudioFormat{
		"audio/mpeg":       types.AudioFormatMp3,
		"audio/wav":        types.AudioFormatWav,
		"audio/flac":       types.AudioFormatFlac,
		"audio/ogg":        types.AudioFormatOgg,
		"audio/aac":        types.AudioFormatAac,
		"audio/opus":       types.AudioFormatOpus,
		"audio/mp4":        types.AudioFormatMp4,
		"audio/webm":       types.AudioFormatWebm,
		"audio/x-matroska": types.AudioFormatMka,
		"audio/x-wav":      types.AudioFormatWav,
		"audio/mp3":        types.AudioFormatMp3,
		"audio/x-flac":     types.AudioFormatFlac,
		"audio/x-m4a":      types.AudioFormatM4a,
	}

	if len(imageFormats) != len(images) || len(documentFormats) != len(documents) || len(videoFormats) != len(videos) || len(audioFormats) != len(audios) {
		t.Fatalf("map sizes = %d, %d, %d, %d; want %d, %d, %d, %d",
			len(imageFormats), len(documentFormats), len(videoFormats), len(audioFormats),
			len(images), len(documents), len(videos), len(audios))
	}

	for mime, want := range images {
		part := interfaces.NewPartFromBytes([]byte{1}, mime)
		block, ok := partBlocks([]*interfaces.Part{part})[0].(*types.ContentBlockMemberImage)
		if !ok || block.Value.Format != want {
			t.Errorf("%s: block = %+v, want image format %q", mime, block, want)
		}
		if !supportedPart(part) {
			t.Errorf("supportedPart(%s) = false", mime)
		}
	}
	for mime, want := range documents {
		part := interfaces.NewPartFromBytes([]byte{1}, mime)
		part.InlineData.DisplayName = "doc"
		block, ok := partBlocks([]*interfaces.Part{part})[0].(*types.ContentBlockMemberDocument)
		if !ok || block.Value.Format != want {
			t.Errorf("%s: block = %+v, want document format %q", mime, block, want)
		}
		if !supportedPart(part) {
			t.Errorf("supportedPart(%s) = false", mime)
		}
	}
	for mime, want := range videos {
		part := interfaces.NewPartFromBytes([]byte{1}, mime)
		block, ok := partBlocks([]*interfaces.Part{part})[0].(*types.ContentBlockMemberVideo)
		if !ok || block.Value.Format != want {
			t.Errorf("%s: block = %+v, want video format %q", mime, block, want)
		}
		if !supportedPart(part) {
			t.Errorf("supportedPart(%s) = false", mime)
		}
	}
	for mime, want := range audios {
		part := interfaces.NewPartFromBytes([]byte{1}, mime)
		block, ok := partBlocks([]*interfaces.Part{part})[0].(*types.ContentBlockMemberAudio)
		if !ok || block.Value.Format != want {
			t.Errorf("%s: block = %+v, want audio format %q", mime, block, want)
		}
		if !supportedPart(part) {
			t.Errorf("supportedPart(%s) = false", mime)
		}
	}
}

func TestPartBlocksSourcesForEachKind(t *testing.T) {
	inlineDoc := interfaces.NewPartFromBytes([]byte{7}, "text/csv")
	inlineDoc.InlineData.DisplayName = "data.csv"
	blocks := partBlocks([]*interfaces.Part{
		inlineDoc,
		interfaces.NewPartFromURI("s3://bucket/clip.mp4", "video/mp4"),
		interfaces.NewPartFromURI("s3://bucket/song.mp3", "audio/mpeg"),
		interfaces.NewPartFromBytes([]byte{9}, "application/zip"),
	})
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3 (the zip part is skipped)", len(blocks))
	}

	doc := blocks[0].(*types.ContentBlockMemberDocument)
	if src, ok := doc.Value.Source.(*types.DocumentSourceMemberBytes); !ok || len(src.Value) != 1 || src.Value[0] != 7 {
		t.Errorf("document source = %+v, want the inline bytes", doc.Value.Source)
	}
	if aws.ToString(doc.Value.Name) != "data csv" {
		t.Errorf("document name = %q, want %q", aws.ToString(doc.Value.Name), "data csv")
	}

	video := blocks[1].(*types.ContentBlockMemberVideo)
	if src, ok := video.Value.Source.(*types.VideoSourceMemberS3Location); !ok || aws.ToString(src.Value.Uri) != "s3://bucket/clip.mp4" {
		t.Errorf("video source = %+v, want the S3 location", video.Value.Source)
	}

	audio := blocks[2].(*types.ContentBlockMemberAudio)
	if src, ok := audio.Value.Source.(*types.AudioSourceMemberS3Location); !ok || aws.ToString(src.Value.Uri) != "s3://bucket/song.mp3" {
		t.Errorf("audio source = %+v, want the S3 location", audio.Value.Source)
	}
}

func TestPartBlocksInlineVideoAndAudio(t *testing.T) {
	blocks := partBlocks([]*interfaces.Part{
		interfaces.NewPartFromBytes([]byte{1, 2}, "video/webm"),
		interfaces.NewPartFromBytes([]byte{3}, "audio/flac"),
	})
	if src, ok := blocks[0].(*types.ContentBlockMemberVideo).Value.Source.(*types.VideoSourceMemberBytes); !ok || len(src.Value) != 2 {
		t.Errorf("video source = %+v, want the bytes", blocks[0])
	}
	if src, ok := blocks[1].(*types.ContentBlockMemberAudio).Value.Source.(*types.AudioSourceMemberBytes); !ok || len(src.Value) != 1 {
		t.Errorf("audio source = %+v, want the bytes", blocks[1])
	}
}

func TestPartBlocksEmpty(t *testing.T) {
	if blocks := partBlocks(nil); blocks == nil || len(blocks) != 0 {
		t.Errorf("partBlocks(nil) = %#v, want an empty, non-nil slice", blocks)
	}
}

func TestDocumentNameEdgeCases(t *testing.T) {
	cases := map[string]string{
		"":                                      "",
		"   ":                                   "",
		"...___":                                "",
		"a":                                     "a",
		"  lead and trail  ":                    "lead and trail",
		"tab\tand\nnewline":                     "tab and newline",
		"x(1)[2]-y":                             "x(1)[2]-y",
		"emoji\U0001F600name":                   "emoji name",
		"日本語":                                   "",
		"Report 2024.v2.final.PDF":              "Report 2024 v2 final PDF",
		strings.Repeat("ab ", 70):               strings.TrimSpace(strings.Repeat("ab ", 70)[:200]),
		strings.Repeat("a", 199) + " b":         strings.Repeat("a", 199),
		strings.Repeat("a", 200) + "bbbbbbbbbb": strings.Repeat("a", 200),
	}
	for in, want := range cases {
		part := interfaces.NewPartFromURI("s3://b/k", "application/pdf")
		part.FileData.DisplayName = in
		if got := documentName(part); got != want {
			t.Errorf("documentName(%q) = %q, want %q", in, got, want)
		}
		if got := documentName(part); len(got) > maxDocumentName {
			t.Errorf("documentName(%q) has %d characters, want at most %d", in, len(got), maxDocumentName)
		}
	}
}

func TestDisplayNameReadsInlineAndFileData(t *testing.T) {
	inline := interfaces.NewPartFromBytes([]byte{1}, "application/pdf")
	inline.InlineData.DisplayName = "inline"
	file := interfaces.NewPartFromURI("s3://b/k", "application/pdf")
	file.FileData.DisplayName = "file"
	if displayName(inline) != "inline" || displayName(file) != "file" {
		t.Errorf("displayName = %q, %q; want inline, file", displayName(inline), displayName(file))
	}
}

func TestSupportedPartRules(t *testing.T) {
	cases := []struct {
		part *interfaces.Part
		want bool
	}{
		{interfaces.NewPartFromURI("s3://bucket/a.pdf", "application/pdf"), true},
		{interfaces.NewPartFromURI("https://example.com/a.pdf", "application/pdf"), false},
		{interfaces.NewPartFromURI("gs://bucket/a.png", "image/png"), false},
		{interfaces.NewPartFromURI("s3://bucket/a.bmp", "image/bmp"), false},
		{interfaces.NewPartFromBytes([]byte{1}, "image/svg+xml"), false},
		{interfaces.NewPartFromBytes([]byte{1}, "audio/x-m4a"), true},
	}
	for _, c := range cases {
		if got := supportedPart(c.part); got != c.want {
			t.Errorf("supportedPart(%s %+v) = %v, want %v", c.part.MIMEType(), c.part.FileData, got, c.want)
		}
	}
}

func TestUserTurnsWithoutMemoryIsTheParts(t *testing.T) {
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(interfaces.NewPartFromText("hi"))(params)
	turns, err := userTurns(context.Background(), params)
	if err != nil || len(turns) != 1 || len(turns[0]) != 1 {
		t.Errorf("userTurns() = %v, %v; want one turn with one part", turns, err)
	}
}

func TestUserTurnsFromMemorySkipsTextOnlyAndAssistantMessages(t *testing.T) {
	mem := stubMemory{messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "text only"},
		{Role: interfaces.MessageRoleUser, Parts: []*interfaces.Part{interfaces.NewPartFromText("a")}},
		{Role: interfaces.MessageRoleAssistant, Parts: []*interfaces.Part{interfaces.NewPartFromText("b")}},
		{Role: interfaces.MessageRoleUser, Parts: []*interfaces.Part{interfaces.NewPartFromText("c"), interfaces.NewPartFromText("d")}},
	}}
	turns, err := userTurns(context.Background(), &interfaces.GenerateOptions{Memory: mem})
	if err != nil {
		t.Fatalf("userTurns() error = %v", err)
	}
	if len(turns) != 2 || len(turns[0]) != 1 || len(turns[1]) != 2 {
		t.Errorf("turns = %v, want the two user turns with parts", turns)
	}
}

func TestUserTurnsReturnsMemoryError(t *testing.T) {
	_, err := userTurns(context.Background(), &interfaces.GenerateOptions{Memory: failingMemory(0)})
	if err == nil || !strings.Contains(err.Error(), "failed to read memory") {
		t.Errorf("userTurns() error = %v, want a memory error", err)
	}
}

func TestCheckContentReturnsMemoryError(t *testing.T) {
	// The first read is for CheckMedia; the second, for the user turns,
	// fails.
	mem := failingMemory(1, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "hi"})
	err := checkContent(context.Background(), &interfaces.GenerateOptions{Memory: mem})
	if err == nil || !strings.Contains(err.Error(), "memory down") {
		t.Errorf("checkContent() error = %v, want the memory error", err)
	}

	err = checkContent(context.Background(), &interfaces.GenerateOptions{Memory: failingMemory(0)})
	if err == nil || !strings.Contains(err.Error(), "memory down") {
		t.Errorf("checkContent() error = %v, want the memory error", err)
	}
}

func TestCheckContentRefusesDuplicateNamesAcrossTurns(t *testing.T) {
	doc := func(name string) *interfaces.Part {
		p := interfaces.NewPartFromBytes([]byte{1}, "application/pdf")
		p.InlineData.DisplayName = name
		return p
	}
	mem := stubMemory{messages: []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Parts: []*interfaces.Part{doc("report.pdf"), interfaces.NewPartFromText("read")}},
		{Role: interfaces.MessageRoleAssistant, Content: "done"},
		{Role: interfaces.MessageRoleUser, Parts: []*interfaces.Part{doc("report_pdf"), interfaces.NewPartFromText("again")}},
	}}
	err := checkContent(context.Background(), &interfaces.GenerateOptions{Memory: mem})
	if err == nil || !strings.Contains(err.Error(), "unique") {
		t.Errorf("checkContent() error = %v, want a duplicate name error", err)
	}
}

func TestCheckContentAcceptsDocumentsWithDistinctNamesAndImagesWithoutText(t *testing.T) {
	a := interfaces.NewPartFromBytes([]byte{1}, "application/pdf")
	a.InlineData.DisplayName = "a"
	b := interfaces.NewPartFromURI("s3://bucket/b.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	b.FileData.DisplayName = "b"
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(a, b, interfaces.NewPartFromText("compare"))(params)
	if err := checkContent(context.Background(), params); err != nil {
		t.Errorf("checkContent(two documents) = %v", err)
	}

	imageOnly := &interfaces.GenerateOptions{}
	interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "image/png"))(imageOnly)
	if err := checkContent(context.Background(), imageOnly); err != nil {
		t.Errorf("checkContent(image only) = %v, want nil: only documents need text", err)
	}
}

func TestCheckContentRefusesDocumentWithWhitespaceText(t *testing.T) {
	pdf := interfaces.NewPartFromBytes([]byte{1}, "application/pdf")
	pdf.InlineData.DisplayName = "report"
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(pdf, interfaces.NewPartFromText("  \n "))(params)
	if err := checkContent(context.Background(), params); err == nil || !strings.Contains(err.Error(), "text part") {
		t.Errorf("checkContent() = %v, want a missing text error", err)
	}
}

func TestCheckContentNamesBadDocumentName(t *testing.T) {
	pdf := interfaces.NewPartFromBytes([]byte{1}, "application/pdf")
	pdf.InlineData.DisplayName = "???"
	params := &interfaces.GenerateOptions{}
	interfaces.WithParts(pdf, interfaces.NewPartFromText("read"))(params)
	err := checkContent(context.Background(), params)
	if !errors.Is(err, errDocumentName) || !strings.Contains(err.Error(), `"???"`) {
		t.Errorf("checkContent() = %v, want errDocumentName naming the input", err)
	}
}

func TestAppendAlternating(t *testing.T) {
	user := types.ConversationRoleUser
	assistant := types.ConversationRoleAssistant
	image := &types.ContentBlockMemberImage{Value: types.ImageBlock{Format: types.ImageFormatPng}}

	var messages []types.Message
	messages = appendAlternating(messages, user, "", image)
	if len(messages) != 1 || len(messages[0].Content) != 1 {
		t.Fatalf("messages = %+v, want one turn with only the image", messages)
	}

	// Text merged into a turn that starts with a non-text block goes first.
	messages = appendAlternating(messages, user, "describe")
	if len(messages) != 1 || len(messages[0].Content) != 2 {
		t.Fatalf("messages = %+v, want the text added to the same turn", messages)
	}
	if text, ok := messages[0].Content[0].(*types.ContentBlockMemberText); !ok || text.Value != "describe" {
		t.Errorf("block 0 = %+v, want the text first", messages[0].Content[0])
	}

	// Text merged into a turn that starts with text joins that text.
	messages = appendAlternating(messages, user, "more")
	if text := messages[0].Content[0].(*types.ContentBlockMemberText); text.Value != "describe\n\nmore" {
		t.Errorf("block 0 = %q, want the joined text", text.Value)
	}

	// Extra blocks merged into the same role go last.
	messages = appendAlternating(messages, user, "", image)
	if len(messages[0].Content) != 3 {
		t.Errorf("turn has %d blocks, want 3", len(messages[0].Content))
	}

	// A new role starts a new turn.
	messages = appendAlternating(messages, assistant, "a cat")
	if len(messages) != 2 || messages[1].Role != assistant || textOf(t, messages[1]) != "a cat" {
		t.Errorf("messages = %+v, want a new assistant turn", messages)
	}
}

func TestBuildMessagesContinuesWhenMemoryFails(t *testing.T) {
	messages := newTestClient().buildMessages(context.Background(), "hi", failingMemory(0), nil)
	if len(messages) != 1 || textOf(t, messages[0]) != "hi" {
		t.Errorf("messages = %+v, want only the prompt", messages)
	}
}

func TestBuildMessagesIgnoresPartsWithMemory(t *testing.T) {
	mem := stubMemory{messages: []interfaces.Message{{Role: interfaces.MessageRoleUser, Content: "hi"}}}
	parts := []*interfaces.Part{interfaces.NewPartFromBytes([]byte{1}, "image/png")}
	messages := newTestClient().buildMessages(context.Background(), "follow up", mem, parts)
	for _, block := range messages[0].Content {
		if _, ok := block.(*types.ContentBlockMemberImage); ok {
			t.Fatalf("messages = %+v, want no image from the parts when memory is set", messages)
		}
	}
	if textOf(t, messages[0]) != "hi\n\nfollow up" {
		t.Errorf("text = %q, want the history and the prompt", textOf(t, messages[0]))
	}
}

func TestAssistantMessageAndTokenUsageNil(t *testing.T) {
	if _, ok := assistantMessage(nil); ok {
		t.Error("assistantMessage(nil) ok = true, want false")
	}
	if tokenUsage(nil) != nil {
		t.Error("tokenUsage(nil) != nil")
	}
	u := tokenUsage(&types.TokenUsage{InputTokens: aws.Int32(1), OutputTokens: aws.Int32(2), TotalTokens: aws.Int32(9)})
	if u.TotalTokens != 9 {
		t.Errorf("TotalTokens = %d, want the reported 9", u.TotalTokens)
	}
}

func TestInferenceConfigNil(t *testing.T) {
	ic := inferenceConfig(nil)
	if aws.ToInt32(ic.MaxTokens) != DefaultMaxTokens || ic.Temperature != nil || ic.TopP != nil {
		t.Errorf("inferenceConfig(nil) = %+v, want only the default max tokens", ic)
	}
}

func TestWithResponseFormatInstructionsWithoutSystem(t *testing.T) {
	got := withResponseFormatInstructions("", &interfaces.ResponseFormat{Schema: interfaces.JSONSchema{"type": "object"}})
	if !strings.HasPrefix(got, "You must respond with valid JSON") {
		t.Errorf("got %q, want only the instructions", got)
	}
}

func TestToolInputJSON(t *testing.T) {
	if got := toolInputJSON(nil); got != "{}" {
		t.Errorf("toolInputJSON(nil) = %q, want {}", got)
	}
	if got := toolInputJSON(brdoc.NewLazyDocument([]int{1})); got != "{}" {
		t.Errorf("toolInputJSON(list) = %q, want {} for input that is not an object", got)
	}
}

func TestExecuteToolNotFoundAndError(t *testing.T) {
	c := newTestClient()
	use := types.ToolUseBlock{Name: aws.String("missing"), ToolUseId: aws.String("1")}
	block := c.executeTool(context.Background(), use, nil).(*types.ContentBlockMemberToolResult)
	if block.Value.Status != types.ToolResultStatusError {
		t.Errorf("status = %q, want error", block.Value.Status)
	}
	if text := block.Value.Content[0].(*types.ToolResultContentBlockMemberText).Value; !strings.Contains(text, `"missing" not found`) {
		t.Errorf("content = %q, want a not found message", text)
	}

	failing := &fixedTool{name: "boom", err: errors.New("exploded")}
	use = types.ToolUseBlock{Name: aws.String("boom"), ToolUseId: aws.String("2")}
	block = c.executeTool(context.Background(), use, []interfaces.Tool{failing}).(*types.ContentBlockMemberToolResult)
	if block.Value.Status != types.ToolResultStatusError {
		t.Errorf("status = %q, want error", block.Value.Status)
	}
	if text := block.Value.Content[0].(*types.ToolResultContentBlockMemberText).Value; text != "Error executing tool: exploded" {
		t.Errorf("content = %q, want the tool error", text)
	}
	if failing.args[0] != "{}" {
		t.Errorf("tool args = %q, want {} for no input", failing.args[0])
	}
}

func TestExecuteToolsParallelKeepsOrder(t *testing.T) {
	c := newTestClient()
	tools := []interfaces.Tool{&fixedTool{name: "a", result: "A"}, &fixedTool{name: "b", result: "B"}}
	uses := []types.ToolUseBlock{
		{Name: aws.String("b"), ToolUseId: aws.String("1")},
		{Name: aws.String("a"), ToolUseId: aws.String("2")},
	}
	results := c.executeToolsParallel(context.Background(), uses, tools)
	for i, want := range []string{"B", "A"} {
		got := results[i].(*types.ContentBlockMemberToolResult).Value.Content[0].(*types.ToolResultContentBlockMemberText).Value
		if got != want {
			t.Errorf("result %d = %q, want %q", i, got, want)
		}
	}
}

func TestConvertToolsIncludesEnumsDefaultsAndItems(t *testing.T) {
	cfg := convertTools([]interfaces.Tool{&fixedTool{name: "tagger"}})
	spec := cfg.Tools[0].(*types.ToolMemberToolSpec).Value
	raw, err := spec.InputSchema.(*types.ToolInputSchemaMemberJson).Value.MarshalSmithyDocument()
	if err != nil {
		t.Fatalf("MarshalSmithyDocument() error = %v", err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema %s: %v", raw, err)
	}
	tags := schema["properties"].(map[string]interface{})["tags"].(map[string]interface{})
	for _, key := range []string{"type", "description", "default", "enum", "items"} {
		if _, ok := tags[key]; !ok {
			t.Errorf("tags property = %v, want key %q", tags, key)
		}
	}
	if items := tags["items"].(map[string]interface{}); items["enum"] == nil {
		t.Errorf("items = %v, want an enum", items)
	}
	if _, ok := schema["required"]; ok {
		t.Errorf("schema = %v, want no required list", schema)
	}
}
