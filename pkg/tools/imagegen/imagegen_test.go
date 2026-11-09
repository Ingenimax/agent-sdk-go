package imagegen

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage"
)

// fakeGenerator records the last request and returns a fixed response.
type fakeGenerator struct {
	resp    *interfaces.ImageGenerationResponse
	err     error
	lastReq interfaces.ImageGenerationRequest
}

func (g *fakeGenerator) GenerateImage(_ context.Context, req interfaces.ImageGenerationRequest) (*interfaces.ImageGenerationResponse, error) {
	g.lastReq = req
	return g.resp, g.err
}

func (g *fakeGenerator) SupportsImageGeneration() bool   { return true }
func (g *fakeGenerator) SupportedImageFormats() []string { return []string{"png"} }

func image(mime string) interfaces.GeneratedImage {
	return interfaces.GeneratedImage{Data: []byte("abcd"), Base64: "YWJjZA==", MimeType: mime}
}

func oneImage() *interfaces.ImageGenerationResponse {
	return &interfaces.ImageGenerationResponse{
		Images: []interfaces.GeneratedImage{image("image/png")},
		Usage:  &interfaces.ImageUsage{InputTokens: 2, OutputTokens: 9},
	}
}

func respondWith(resp *interfaces.ImageEditResponse) func() *testutil.FakeImageEditSession {
	return func() *testutil.FakeImageEditSession { return testutil.NewFakeImageEditSession(resp, nil) }
}

func TestNewDefaultsAndOptions(t *testing.T) {
	tool := New(&fakeGenerator{}, nil)
	assert.Equal(t, 2000, tool.maxPromptLen)
	assert.Equal(t, "1:1", tool.defaultAspect)
	assert.Equal(t, "png", tool.defaultFormat)
	assert.Equal(t, 30*time.Minute, tool.sessionTimeout)
	assert.Equal(t, 10, tool.maxSessionsPerOrg)
	assert.False(t, tool.multiTurnEnabled)

	tool = New(&fakeGenerator{}, nil,
		WithMaxPromptLength(50),
		WithDefaultAspectRatio("16:9"),
		WithDefaultFormat("jpeg"),
		WithMultiTurnModel("m"),
		WithSessionTimeout(time.Second),
		WithMaxSessionsPerOrg(3),
	)
	assert.Equal(t, 50, tool.maxPromptLen)
	assert.Equal(t, "16:9", tool.defaultAspect)
	assert.Equal(t, "jpeg", tool.defaultFormat)
	assert.Equal(t, "m", tool.multiTurnModel)
	assert.Equal(t, time.Second, tool.sessionTimeout)
	assert.Equal(t, 3, tool.maxSessionsPerOrg)
}

func TestWithMultiTurnEditor(t *testing.T) {
	tests := []struct {
		name   string
		editor interfaces.MultiTurnImageEditor
		want   bool
	}{
		{name: "nil editor", editor: nil, want: false},
		{name: "unsupported editor", editor: &testutil.FakeImageEditor{Unsupported: true}, want: false},
		{name: "supported editor", editor: &testutil.FakeImageEditor{}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(tt.editor))
			assert.Equal(t, tt.want, tool.multiTurnEnabled)
			assert.Equal(t, tt.want, tool.sessions != nil)
		})
	}
}

func TestMetadataSingleShot(t *testing.T) {
	tool := New(&fakeGenerator{}, nil, WithDefaultFormat("jpeg"))
	assert.Equal(t, "generate_image", tool.Name())
	assert.Equal(t, "Image Generator", tool.DisplayName())
	assert.False(t, tool.Internal())
	assert.True(t, strings.HasPrefix(tool.Description(), "Generate images from text descriptions"))

	params := tool.Parameters()
	assert.True(t, params["prompt"].Required)
	assert.Equal(t, "1:1", params["aspect_ratio"].Default)
	require.Contains(t, params, "output_format")
	assert.Equal(t, "jpeg", params["output_format"].Default)
	assert.NotContains(t, params, "action")
	assert.NotContains(t, params, "image_size")
	assert.Zero(t, tool.GetActiveSessions())
}

func TestMetadataMultiTurn(t *testing.T) {
	tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(&testutil.FakeImageEditor{}))
	assert.Contains(t, tool.Description(), "Generate and edit images through conversation")

	params := tool.Parameters()
	require.Contains(t, params, "action")
	assert.Equal(t, "generate", params["action"].Default)
	require.Contains(t, params, "image_size")
	assert.NotContains(t, params, "output_format")
}

func TestExecuteInvalidJSON(t *testing.T) {
	_, err := New(&fakeGenerator{}, nil).Run(context.Background(), `{bad`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse arguments")
}

func TestSingleShotRequest(t *testing.T) {
	gen := &fakeGenerator{resp: oneImage()}
	tool := New(gen, nil, WithDefaultAspectRatio("4:3"), WithDefaultFormat("jpeg"))

	_, err := tool.Execute(context.Background(), `{"prompt": "a fox"}`)
	require.NoError(t, err)
	assert.Equal(t, "a fox", gen.lastReq.Prompt)
	assert.Equal(t, &interfaces.ImageGenerationOptions{NumberOfImages: 1, AspectRatio: "4:3", OutputFormat: "jpeg"}, gen.lastReq.Options)

	_, err = tool.Execute(context.Background(), `{"prompt": "a fox", "aspect_ratio": "1:1", "output_format": "png"}`)
	require.NoError(t, err)
	assert.Equal(t, "1:1", gen.lastReq.Options.AspectRatio)
	assert.Equal(t, "png", gen.lastReq.Options.OutputFormat)
}

func TestSingleShotWithStorage(t *testing.T) {
	gen := &fakeGenerator{resp: oneImage()}
	store := &testutil.FakeImageStorage{}
	tool := New(gen, store)

	out, err := tool.Execute(context.Background(), `{"prompt": "a fox"}`)
	require.NoError(t, err)

	assert.Contains(t, out, `Successfully generated image for prompt: "a fox"`)
	assert.Contains(t, out, "![Generated image](https://cdn.test/image/png)")
	assert.Contains(t, out, "Format: image/png")
	assert.Contains(t, out, "Size: 4 bytes")
	assert.Contains(t, out, "Tokens used: 2 input, 9 output")
	assert.Equal(t, "https://cdn.test/image/png", gen.resp.Images[0].URL)
	metas := store.Metadata()
	require.Len(t, metas, 1)
	assert.Equal(t, "a fox", metas[0].Prompt)
}

func TestSingleShotWithStorageNoUsage(t *testing.T) {
	gen := &fakeGenerator{resp: &interfaces.ImageGenerationResponse{Images: []interfaces.GeneratedImage{image("image/png")}}}
	out, err := New(gen, &testutil.FakeImageStorage{}).Execute(context.Background(), `{"prompt": "a fox"}`)
	require.NoError(t, err)
	assert.NotContains(t, out, "Tokens used")
}

func TestSingleShotBase64(t *testing.T) {
	tests := []struct {
		name  string
		store storage.ImageStorage
	}{
		{name: "no storage", store: nil},
		{name: "storage failure", store: &testutil.FakeImageStorage{Err: errors.New("bucket missing")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := &fakeGenerator{resp: oneImage()}
			out, err := New(gen, tt.store).Execute(context.Background(), `{"prompt": "a fox"}`)
			require.NoError(t, err)
			assert.Contains(t, out, "![Generated image](data:image/png;base64,YWJjZA==)")
			assert.Contains(t, out, "Size: 4 bytes")
			assert.NotContains(t, out, "Tokens used")
		})
	}
}

func TestSingleShotTruncatesLongPromptInOutput(t *testing.T) {
	prompt := strings.Repeat("x", 150)
	gen := &fakeGenerator{resp: oneImage()}
	out, err := New(gen, nil).Execute(context.Background(), `{"prompt": "`+prompt+`"}`)
	require.NoError(t, err)
	assert.Contains(t, out, `"`+strings.Repeat("x", 100)+`..."`)
	assert.Equal(t, prompt, gen.lastReq.Prompt, "the generator still gets the full prompt")
}

func TestSingleShotErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		gen     *fakeGenerator
		wantErr string
	}{
		{name: "missing prompt", args: `{}`, gen: &fakeGenerator{}, wantErr: "prompt is required"},
		{name: "prompt too long", args: `{"prompt": "abcdef"}`, gen: &fakeGenerator{}, wantErr: "prompt exceeds maximum length of 5 characters"},
		{name: "generator error", args: `{"prompt": "a"}`, gen: &fakeGenerator{err: errors.New("quota")}, wantErr: "image generation failed: quota"},
		{name: "no images", args: `{"prompt": "a"}`, gen: &fakeGenerator{resp: &interfaces.ImageGenerationResponse{}}, wantErr: "no images were generated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.gen, nil, WithMaxPromptLength(5)).Execute(context.Background(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMultiTurnGenerate(t *testing.T) {
	editor := &testutil.FakeImageEditor{NewSession: respondWith(&interfaces.ImageEditResponse{
		Text:   "done",
		Images: []interfaces.GeneratedImage{image("image/png")},
		Usage:  &interfaces.ImageUsage{InputTokens: 1, OutputTokens: 2},
	})}
	tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(editor), WithMultiTurnModel("m"))

	out, err := tool.Execute(context.Background(), `{"prompt": "a fox", "aspect_ratio": "16:9"}`)
	require.NoError(t, err)

	assert.Equal(t, "m", editor.LastOptions().Model)
	sessions := editor.Sessions()
	require.Len(t, sessions, 1)
	assert.Equal(t, []string{"a fox"}, sessions[0].Prompts())
	assert.Equal(t, &interfaces.ImageEditOptions{AspectRatio: "16:9", ImageSize: "1K"}, sessions[0].LastOptions())

	assert.True(t, strings.HasPrefix(out, "Image generated successfully."))
	assert.Contains(t, out, "Model: done")
	assert.Contains(t, out, "![Generated image](data:image/png;base64,YWJjZA==)")
	assert.Contains(t, out, "Tokens used: 1 input, 2 output")
	assert.Contains(t, out, "action='edit'")
	assert.Equal(t, 1, tool.GetActiveSessions())
	assert.Contains(t, tool.sessions, "default")
}

func TestMultiTurnGenerateReplacesSession(t *testing.T) {
	editor := &testutil.FakeImageEditor{}
	tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(editor))
	ctx := multitenancy.WithOrgID(context.Background(), "org-a")

	_, err := tool.Execute(ctx, `{"action": "generate", "prompt": "one", "image_size": "4K"}`)
	require.NoError(t, err)
	_, err = tool.Execute(ctx, `{"action": "generate", "prompt": "two"}`)
	require.NoError(t, err)

	sessions := editor.Sessions()
	require.Len(t, sessions, 2)
	assert.True(t, sessions[0].Closed())
	assert.False(t, sessions[1].Closed())
	assert.Equal(t, "4K", sessions[0].LastOptions().ImageSize)
	assert.Equal(t, 1, tool.GetActiveSessions())
	assert.Contains(t, tool.sessions, "org-a")
}

func TestMultiTurnGenerateErrors(t *testing.T) {
	t.Run("missing prompt", func(t *testing.T) {
		tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(&testutil.FakeImageEditor{}))
		_, err := tool.Execute(context.Background(), `{"action": "generate"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prompt is required for generating an image")
	})

	t.Run("prompt too long", func(t *testing.T) {
		tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(&testutil.FakeImageEditor{}), WithMaxPromptLength(2))
		_, err := tool.Execute(context.Background(), `{"prompt": "abc"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prompt exceeds maximum length of 2 characters")
	})

	t.Run("create session error", func(t *testing.T) {
		tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(&testutil.FakeImageEditor{Err: errors.New("quota")}))
		_, err := tool.Execute(context.Background(), `{"prompt": "a"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create editing session: quota")
	})

	t.Run("initial message error", func(t *testing.T) {
		editor := &testutil.FakeImageEditor{NewSession: func() *testutil.FakeImageEditSession {
			return testutil.NewFakeImageEditSession(nil, errors.New("blocked"))
		}}
		tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(editor))
		_, err := tool.Execute(context.Background(), `{"prompt": "a"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate initial image: blocked")
		assert.Zero(t, tool.GetActiveSessions())
		assert.True(t, editor.Sessions()[0].Closed())
	})

	t.Run("session limit", func(t *testing.T) {
		tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(&testutil.FakeImageEditor{}), WithMaxSessionsPerOrg(1))
		// Seed a session under another key that belongs to the same org.
		tool.sessionsMu.Lock()
		tool.sessions["other"] = &sessionEntry{session: testutil.NewFakeImageEditSession(nil, nil), orgID: "org-a", lastUsed: time.Now()}
		tool.sessionsMu.Unlock()

		ctx := multitenancy.WithOrgID(context.Background(), "org-a")
		_, err := tool.Execute(ctx, `{"prompt": "a"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum number of concurrent sessions (1) reached")
	})

	t.Run("unknown action", func(t *testing.T) {
		tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(&testutil.FakeImageEditor{}))
		_, err := tool.Execute(context.Background(), `{"action": "paint", "prompt": "a"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown action: paint")
	})
}

func TestMultiTurnEdit(t *testing.T) {
	editor := &testutil.FakeImageEditor{NewSession: respondWith(&interfaces.ImageEditResponse{
		Images: []interfaces.GeneratedImage{image("image/png"), image("image/jpeg")},
	})}
	store := &testutil.FakeImageStorage{}
	tool := New(&fakeGenerator{}, store, WithMultiTurnEditor(editor))
	ctx := multitenancy.WithOrgID(context.Background(), "org-a")

	_, err := tool.Execute(ctx, `{"prompt": "a fox"}`)
	require.NoError(t, err)

	out, err := tool.Execute(ctx, `{"action": "edit", "prompt": "make it red", "image_size": "2K"}`)
	require.NoError(t, err)

	sessions := editor.Sessions()
	require.Len(t, sessions, 1, "edit reuses the active session")
	assert.Equal(t, []string{"a fox", "make it red"}, sessions[0].Prompts())
	assert.Equal(t, "2K", sessions[0].LastOptions().ImageSize)

	assert.True(t, strings.HasPrefix(out, "Image edited successfully."))
	assert.Contains(t, out, "![Generated image](https://cdn.test/image/png)")
	assert.Contains(t, out, "--- Image 2 ---")
	assert.Contains(t, out, "![Generated image](https://cdn.test/image/jpeg)")
	metas := store.Metadata()
	require.NotEmpty(t, metas)
	assert.Equal(t, "org-a", metas[len(metas)-1].OrgID)
	assert.Equal(t, "make it red", metas[len(metas)-1].Prompt)
}

func TestMultiTurnEditStartsSessionWhenMissing(t *testing.T) {
	editor := &testutil.FakeImageEditor{}
	tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(editor))

	out, err := tool.Execute(context.Background(), `{"action": "edit", "prompt": "a fox"}`)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "Image generated successfully."))
	assert.Contains(t, out, "No image was generated in this turn.")
	assert.Len(t, editor.Sessions(), 1)
}

func TestMultiTurnEditRestartsExpiredSession(t *testing.T) {
	editor := &testutil.FakeImageEditor{}
	tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(editor), WithSessionTimeout(time.Minute))

	_, err := tool.Execute(context.Background(), `{"prompt": "one"}`)
	require.NoError(t, err)
	tool.sessionsMu.Lock()
	tool.sessions["default"].lastUsed = time.Now().Add(-time.Hour)
	tool.sessionsMu.Unlock()

	out, err := tool.Execute(context.Background(), `{"action": "edit", "prompt": "two"}`)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "Image generated successfully."))
	sessions := editor.Sessions()
	require.Len(t, sessions, 2)
	assert.True(t, sessions[0].Closed())
	assert.Equal(t, 1, tool.GetActiveSessions())
}

func TestMultiTurnEditErrors(t *testing.T) {
	t.Run("missing prompt", func(t *testing.T) {
		tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(&testutil.FakeImageEditor{}))
		_, err := tool.Execute(context.Background(), `{"action": "edit"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prompt is required for editing")
	})

	t.Run("prompt too long", func(t *testing.T) {
		tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(&testutil.FakeImageEditor{}), WithMaxPromptLength(2))
		_, err := tool.Execute(context.Background(), `{"action": "edit", "prompt": "abc"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prompt exceeds maximum length of 2 characters")
	})

	t.Run("send error", func(t *testing.T) {
		editor := &testutil.FakeImageEditor{}
		tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(editor))
		_, err := tool.Execute(context.Background(), `{"prompt": "a"}`)
		require.NoError(t, err)
		editor.Sessions()[0].SetErr(errors.New("blocked"))

		_, err = tool.Execute(context.Background(), `{"action": "edit", "prompt": "b"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to edit image: blocked")
		assert.Equal(t, 1, tool.GetActiveSessions())
	})
}

func TestMultiTurnStorageFailureFallsBackToBase64(t *testing.T) {
	editor := &testutil.FakeImageEditor{NewSession: respondWith(&interfaces.ImageEditResponse{
		Images: []interfaces.GeneratedImage{image("image/png")},
	})}
	tool := New(&fakeGenerator{}, &testutil.FakeImageStorage{Err: errors.New("bucket missing")}, WithMultiTurnEditor(editor))

	out, err := tool.Execute(context.Background(), `{"prompt": "a"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "data:image/png;base64,YWJjZA==")
}

func TestMultiTurnEndSession(t *testing.T) {
	editor := &testutil.FakeImageEditor{NewSession: func() *testutil.FakeImageEditSession {
		s := testutil.NewFakeImageEditSession(nil, nil)
		s.SetHistory(make([]interfaces.ImageEditTurn, 6))
		return s
	}}
	tool := New(&fakeGenerator{}, nil, WithMultiTurnEditor(editor))

	out, err := tool.Execute(context.Background(), `{"action": "end_session"}`)
	require.NoError(t, err)
	assert.Equal(t, "No active editing session to close.", out)

	_, err = tool.Execute(context.Background(), `{"prompt": "a"}`)
	require.NoError(t, err)

	out, err = tool.Execute(context.Background(), `{"action": "end_session"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "Editing session closed.")
	assert.Contains(t, out, "Total turns: 3")
	assert.True(t, editor.Sessions()[0].Closed())
	assert.Zero(t, tool.GetActiveSessions())
}

func TestFormatImageBase64(t *testing.T) {
	tool := New(&fakeGenerator{}, nil)

	t.Run("small image is embedded", func(t *testing.T) {
		out := tool.formatImageBase64(&interfaces.GeneratedImage{Data: []byte("ab"), Base64: "YWI=", MimeType: "image/png"}, 0)
		assert.Equal(t, "![Generated image](data:image/png;base64,YWI=)\n\nFormat: image/png\nSize: 2 bytes\n", out)
	})

	t.Run("large image is not embedded", func(t *testing.T) {
		img := &interfaces.GeneratedImage{
			Data:     make([]byte, 2048),
			Base64:   strings.Repeat("A", 50001),
			MimeType: "image/png",
		}
		out := tool.formatImageBase64(img, 1)
		assert.True(t, strings.HasPrefix(out, "\n--- Image 2 ---\n[Image generated successfully]"))
		assert.Contains(t, out, "Size: 2048 bytes (2.0 KB)")
		assert.Contains(t, out, "too large to display inline")
		assert.NotContains(t, out, "data:image/png")
	})
}

func TestTruncateString(t *testing.T) {
	assert.Equal(t, "abc", truncateString("abc", 3))
	assert.Equal(t, "ab...", truncateString("abc", 2))
	assert.Equal(t, "", truncateString("", 2))
}

func TestGetImageReference(t *testing.T) {
	before := time.Now()
	ref := GetImageReference(&interfaces.GeneratedImage{URL: "https://cdn.test/x", MimeType: "image/png"}, "a fox")

	assert.Equal(t, "https://cdn.test/x", ref.URL)
	assert.Equal(t, "image/png", ref.MimeType)
	assert.Equal(t, "a fox", ref.Prompt)
	assert.False(t, ref.CreatedAt.Before(before))
}
