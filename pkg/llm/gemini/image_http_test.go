package gemini

import (
	"encoding/base64"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

var pngBytes = []byte{0x89, 'P', 'N', 'G', 1, 2, 3}

func TestImageCapabilityHelpers(t *testing.T) {
	tests := []struct {
		model       string
		imageGen    bool
		multiTurn   bool
		formats     []string
		sizes       []string
		formatsSeen []string
	}{
		{ModelGemini15Flash, false, false, nil, nil, []string{"png", "jpeg"}},
		{ModelGemini20FlashPreviewImageGen, true, false, []string{"png", "jpeg"}, nil, []string{"png", "jpeg"}},
		{ModelGemini25FlashImage, true, true, []string{"png", "jpeg"}, []string{"1K", "2K"}, []string{"png", "jpeg"}},
		{ModelGemini3ProImagePreview, true, true, []string{"png", "jpeg"}, []string{"1K", "2K", "4K"}, []string{"png", "jpeg"}},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.imageGen, SupportsImageGeneration(tt.model))
			assert.Equal(t, tt.multiTurn, SupportsMultiTurnImageEditing(tt.model))
			assert.Equal(t, tt.formats, GetSupportedOutputFormats(tt.model))
			assert.Equal(t, tt.sizes, GetSupportedImageSizes(tt.model))

			c := &GeminiClient{model: tt.model}
			assert.Equal(t, tt.imageGen, c.SupportsImageGeneration())
			assert.Equal(t, tt.multiTurn, c.SupportsMultiTurnImageEditing())
			// The client falls back to png and jpeg when the model lists none.
			assert.Equal(t, tt.formatsSeen, c.SupportedImageFormats())
		})
	}
}

func TestGenerateImageSuccess(t *testing.T) {
	api := newFakeAPI(t, reply{body: responseJSON(map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": []any{textPart("a cat"), imagePart("image/png", pngBytes), imagePart("", pngBytes)}},
			"finishReason": "STOP",
		}},
		"usageMetadata": map[string]any{"promptTokenCount": 4, "candidatesTokenCount": 9},
	})})
	client := api.client(t, WithModel(ModelGemini25FlashImage))

	resp, err := client.GenerateImage(t.Context(), interfaces.ImageGenerationRequest{
		Prompt:         "draw a cat",
		ReferenceImage: &interfaces.ImageData{Base64: base64.StdEncoding.EncodeToString([]byte("ref"))},
		Options:        &interfaces.ImageGenerationOptions{OutputFormat: "jpeg"},
	})
	require.NoError(t, err)

	require.Len(t, resp.Images, 2)
	assert.Equal(t, pngBytes, resp.Images[0].Data)
	assert.Equal(t, base64.StdEncoding.EncodeToString(pngBytes), resp.Images[0].Base64)
	assert.Equal(t, "image/png", resp.Images[0].MimeType)
	assert.Equal(t, "STOP", resp.Images[0].FinishReason)
	// A missing MIME type comes from the requested output format.
	assert.Equal(t, "image/jpeg", resp.Images[1].MimeType)
	assert.Equal(t, &interfaces.ImageUsage{InputTokens: 4, OutputTokens: 9, ImagesGenerated: 2}, resp.Usage)
	assert.Equal(t, "a cat", resp.Metadata["text_response"])
	assert.Equal(t, ModelGemini25FlashImage, resp.Metadata["model"])

	body := api.requests()[0].body
	assert.Equal(t, "draw a cat", dig(body, "contents", 0, "parts", 0, "text"))
	assert.Equal(t, "image/png", dig(body, "contents", 0, "parts", 1, "inlineData", "mimeType"))
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("ref")), dig(body, "contents", 0, "parts", 1, "inlineData", "data"))
	assert.Equal(t, []any{"IMAGE"}, dig(body, "generationConfig", "responseModalities"))
}

func TestGenerateImageUsesDefaultsAndRawReference(t *testing.T) {
	api := newFakeAPI(t, okReply(imagePart("", pngBytes)))
	client := api.client(t, WithModel(ModelGemini3ProImagePreview))

	resp, err := client.GenerateImage(t.Context(), interfaces.ImageGenerationRequest{
		Prompt:         "edit",
		ReferenceImage: &interfaces.ImageData{Data: []byte("raw"), MimeType: "image/webp"},
	})
	require.NoError(t, err)
	require.Len(t, resp.Images, 1)
	assert.Equal(t, "image/png", resp.Images[0].MimeType)
	assert.Nil(t, resp.Usage)

	body := api.requests()[0].body
	assert.Equal(t, "image/webp", dig(body, "contents", 0, "parts", 1, "inlineData", "mimeType"))
}

func TestGenerateImageErrors(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		request interfaces.ImageGenerationRequest
		replies []reply
		wantIs  error
		wantErr string
	}{
		{
			name:    "model without image support",
			model:   ModelGemini15Flash,
			request: interfaces.ImageGenerationRequest{Prompt: "x"},
			wantIs:  interfaces.ErrImageGenerationNotSupported,
		},
		{
			name:   "empty prompt",
			model:  ModelGemini25FlashImage,
			wantIs: interfaces.ErrInvalidPrompt,
		},
		{
			name:    "bad reference base64",
			model:   ModelGemini25FlashImage,
			request: interfaces.ImageGenerationRequest{Prompt: "x", ReferenceImage: &interfaces.ImageData{Base64: "!!"}},
			wantErr: "failed to decode reference image",
		},
		{
			name:    "content blocked",
			model:   ModelGemini25FlashImage,
			request: interfaces.ImageGenerationRequest{Prompt: "x"},
			replies: []reply{errReply(http.StatusBadRequest, "prompt blocked by SAFETY")},
			wantIs:  interfaces.ErrContentBlocked,
		},
		{
			name:    "rate limited",
			model:   ModelGemini25FlashImage,
			request: interfaces.ImageGenerationRequest{Prompt: "x"},
			replies: []reply{errReply(http.StatusTooManyRequests, "quota exceeded")},
			wantIs:  interfaces.ErrRateLimitExceeded,
		},
		{
			name:    "other API error",
			model:   ModelGemini25FlashImage,
			request: interfaces.ImageGenerationRequest{Prompt: "x"},
			replies: []reply{errReply(http.StatusBadRequest, "bad")},
			wantErr: "image generation failed",
		},
		{
			name:    "no candidates",
			model:   ModelGemini25FlashImage,
			request: interfaces.ImageGenerationRequest{Prompt: "x"},
			replies: []reply{{body: `{"candidates":[]}`}},
			wantErr: "no candidates in response",
		},
		{
			name:    "text instead of image",
			model:   ModelGemini25FlashImage,
			request: interfaces.ImageGenerationRequest{Prompt: "x"},
			replies: []reply{okReply(textPart("I cannot draw"))},
			wantErr: "model returned text: I cannot draw",
		},
		{
			name:    "no image and no text",
			model:   ModelGemini25FlashImage,
			request: interfaces.ImageGenerationRequest{Prompt: "x"},
			replies: []reply{{body: `{"candidates":[{"finishReason":"STOP"}]}`}},
			wantErr: "no images generated in response",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, tt.replies...)
			resp, err := api.client(t, WithModel(tt.model)).GenerateImage(t.Context(), tt.request)
			require.Error(t, err)
			assert.Nil(t, resp)
			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs)
			}
			if tt.wantErr != "" {
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestErrorClassifiers(t *testing.T) {
	tests := []struct {
		msg       string
		blocked   bool
		rateLimit bool
	}{
		{"request blocked", true, false},
		{"SAFETY filter", true, false},
		{"rate limit hit", false, true},
		{"RESOURCE_EXHAUSTED", false, true},
		{"status 429", false, true},
		{"plain failure", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			err := errors.New(tt.msg)
			assert.Equal(t, tt.blocked, isContentBlockedError(err))
			assert.Equal(t, tt.rateLimit, isRateLimitError(err))
		})
	}
	assert.False(t, isContentBlockedError(nil))
	assert.False(t, isRateLimitError(nil))
	assert.True(t, contains("abc", "abc"))
	assert.False(t, contains("ab", "abc"))
}

func TestImageEditSessionConversation(t *testing.T) {
	api := newFakeAPI(t,
		reply{body: responseJSON(map[string]any{
			"candidates": []any{map[string]any{
				"content":      map[string]any{"role": "model", "parts": []any{textPart("here"), textPart("it is"), imagePart("", pngBytes)}},
				"finishReason": "STOP",
			}},
			"usageMetadata": map[string]any{"promptTokenCount": 2, "candidatesTokenCount": 3},
		})},
		okReply(imagePart("image/jpeg", pngBytes)),
	)
	// The client model cannot edit images, so the session falls back to the default edit model.
	client := api.client(t, WithModel(ModelGemini15Flash))

	session, err := client.CreateImageEditSession(t.Context(), &interfaces.ImageEditSessionOptions{SystemInstruction: "you draw"})
	require.NoError(t, err)

	first, err := session.SendMessage(t.Context(), "draw a cat", nil)
	require.NoError(t, err)
	assert.Equal(t, "here\nit is", first.Text)
	require.Len(t, first.Images, 1)
	assert.Equal(t, "image/png", first.Images[0].MimeType)
	assert.Equal(t, "STOP", first.Images[0].FinishReason)
	assert.Equal(t, &interfaces.ImageUsage{InputTokens: 2, OutputTokens: 3, ImagesGenerated: 1}, first.Usage)
	assert.Equal(t, DefaultImageEditModel, first.Metadata["model"])

	second, err := session.SendMessageWithImage(t.Context(), "make it blue",
		&interfaces.ImageData{Base64: base64.StdEncoding.EncodeToString([]byte("img"))}, nil)
	require.NoError(t, err)
	require.Len(t, second.Images, 1)
	assert.Equal(t, "image/jpeg", second.Images[0].MimeType)
	assert.Nil(t, second.Usage)

	reqs := api.requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, "/v1beta/models/"+DefaultImageEditModel+":generateContent", reqs[0].path)
	assert.Equal(t, "you draw", dig(reqs[0].body, "systemInstruction", "parts", 0, "text"))
	assert.Equal(t, []any{"TEXT", "IMAGE"}, dig(reqs[0].body, "generationConfig", "responseModalities"))

	// The second request carries the whole conversation so far plus the new image.
	contents := dig(reqs[1].body, "contents").([]any)
	require.Len(t, contents, 3)
	assert.Equal(t, "make it blue", dig(contents[2], "parts", 0, "text"))
	assert.Equal(t, "image/png", dig(contents[2], "parts", 1, "inlineData", "mimeType"))

	history := session.GetHistory()
	require.Len(t, history, 4)
	assert.Equal(t, "user", history[0].Role)
	assert.Equal(t, "draw a cat", history[0].Message)
	assert.Equal(t, "model", history[1].Role)
	assert.Equal(t, "here\nit is", history[1].Message)
	require.Len(t, history[1].Images, 1)
	assert.Equal(t, pngBytes, history[1].Images[0].Data)
	assert.Equal(t, "make it blue", history[2].Message)
	require.Len(t, history[2].Images, 1)
	assert.Equal(t, []byte("img"), history[2].Images[0].Data)

	assert.NoError(t, session.Close())
}

func TestImageEditSessionUsesRequestedModel(t *testing.T) {
	api := newFakeAPI(t, okReply(textPart("ok")))
	client := api.client(t)

	session, err := client.CreateImageEditSession(t.Context(), &interfaces.ImageEditSessionOptions{Model: ModelGemini25FlashImage})
	require.NoError(t, err)

	// A raw image with no message is allowed and keeps its MIME type.
	resp, err := session.SendMessageWithImage(t.Context(), "", &interfaces.ImageData{Data: []byte("raw"), MimeType: "image/webp"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Text)
	assert.Empty(t, resp.Images)

	req := api.requests()[0]
	assert.Equal(t, "/v1beta/models/"+ModelGemini25FlashImage+":generateContent", req.path)
	assert.Equal(t, "image/webp", dig(req.body, "contents", 0, "parts", 0, "inlineData", "mimeType"))
	assert.Nil(t, dig(req.body, "systemInstruction"))
}

func TestImageEditSessionErrors(t *testing.T) {
	newSession := func(t *testing.T, replies ...reply) interfaces.ImageEditSession {
		t.Helper()
		api := newFakeAPI(t, replies...)
		session, err := api.client(t).CreateImageEditSession(t.Context(), nil)
		require.NoError(t, err)
		return session
	}

	t.Run("empty message", func(t *testing.T) {
		_, err := newSession(t).SendMessage(t.Context(), "", nil)
		assert.ErrorIs(t, err, interfaces.ErrInvalidPrompt)
	})
	t.Run("no message and no image", func(t *testing.T) {
		_, err := newSession(t).SendMessageWithImage(t.Context(), "", nil, nil)
		assert.ErrorIs(t, err, interfaces.ErrInvalidPrompt)
	})
	t.Run("bad image base64", func(t *testing.T) {
		_, err := newSession(t).SendMessageWithImage(t.Context(), "x", &interfaces.ImageData{Base64: "!!"}, nil)
		assert.ErrorContains(t, err, "failed to decode image base64")
	})
	t.Run("no candidates", func(t *testing.T) {
		_, err := newSession(t, reply{body: `{"candidates":[]}`}).SendMessage(t.Context(), "x", nil)
		assert.ErrorContains(t, err, "no candidates in response")
	})

	sendErrors := []struct {
		name    string
		reply   reply
		wantIs  error
		wantErr string
	}{
		{"blocked", errReply(http.StatusBadRequest, "blocked"), interfaces.ErrContentBlocked, ""},
		{"rate limited", errReply(http.StatusTooManyRequests, "quota"), interfaces.ErrRateLimitExceeded, ""},
		{"other", errReply(http.StatusBadRequest, "bad"), nil, "failed"},
	}
	for _, tt := range sendErrors {
		t.Run("SendMessage "+tt.name, func(t *testing.T) {
			_, err := newSession(t, tt.reply).SendMessage(t.Context(), "x", nil)
			require.Error(t, err)
			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs)
			}
			assert.Contains(t, err.Error(), tt.wantErr)
		})
		t.Run("SendMessageWithImage "+tt.name, func(t *testing.T) {
			_, err := newSession(t, tt.reply).SendMessageWithImage(t.Context(), "x", &interfaces.ImageData{Data: []byte("i")}, nil)
			require.Error(t, err)
			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs)
			}
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
