package imageedit

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

var sessionIDRe = regexp.MustCompile(`Session ID: (\S+)`)

func sessionIDFrom(t *testing.T, out string) string {
	t.Helper()
	m := sessionIDRe.FindStringSubmatch(out)
	require.Len(t, m, 2, "no session ID in output: %s", out)
	return m[1]
}

func image(mime string) interfaces.GeneratedImage {
	return interfaces.GeneratedImage{Data: []byte("abcd"), Base64: "YWJjZA==", MimeType: mime}
}

func respondWith(resp *interfaces.ImageEditResponse) func() *testutil.FakeImageEditSession {
	return func() *testutil.FakeImageEditSession { return testutil.NewFakeImageEditSession(resp, nil) }
}

func failWith(err error) func() *testutil.FakeImageEditSession {
	return func() *testutil.FakeImageEditSession { return testutil.NewFakeImageEditSession(nil, err) }
}

func TestNewDefaultsAndOptions(t *testing.T) {
	tool := New(&testutil.FakeImageEditor{}, nil)
	assert.Equal(t, 2000, tool.maxPromptLen)
	assert.Equal(t, 30*time.Minute, tool.sessionTimeout)
	assert.Equal(t, 10, tool.maxSessions)
	assert.Empty(t, tool.defaultModel)

	tool = New(&testutil.FakeImageEditor{}, nil,
		WithMaxPromptLength(5),
		WithSessionTimeout(time.Second),
		WithMaxSessions(2),
		WithDefaultModel("model-x"),
	)
	assert.Equal(t, 5, tool.maxPromptLen)
	assert.Equal(t, time.Second, tool.sessionTimeout)
	assert.Equal(t, 2, tool.maxSessions)
	assert.Equal(t, "model-x", tool.defaultModel)
}

func TestMetadata(t *testing.T) {
	tool := New(&testutil.FakeImageEditor{}, nil)
	assert.Equal(t, "edit_image", tool.Name())
	assert.Equal(t, "Image Editor", tool.DisplayName())
	assert.Contains(t, tool.Description(), "start_session")
	assert.False(t, tool.Internal())

	params := tool.Parameters()
	require.Contains(t, params, "action")
	assert.True(t, params["action"].Required)
	assert.Equal(t, []any{"start_session", "edit", "end_session"}, params["action"].Enum)
	for _, key := range []string{"session_id", "prompt", "aspect_ratio", "image_size"} {
		assert.Contains(t, params, key)
	}
}

func TestExecuteValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr string
	}{
		{name: "invalid JSON", args: `{bad`, wantErr: "failed to parse arguments"},
		{name: "unknown action", args: `{"action": "paint"}`, wantErr: "unknown action: paint"},
		{name: "edit without session", args: `{"action": "edit", "prompt": "p"}`, wantErr: "session_id is required for 'edit' action"},
		{name: "edit without prompt", args: `{"action": "edit", "session_id": "s"}`, wantErr: "prompt is required for 'edit' action"},
		{name: "end without session", args: `{"action": "end_session"}`, wantErr: "session_id is required for 'end_session' action"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(&testutil.FakeImageEditor{}, nil).Run(context.Background(), tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestStartSessionWithoutPrompt(t *testing.T) {
	editor := &testutil.FakeImageEditor{}
	tool := New(editor, nil, WithDefaultModel("model-x"))

	out, err := tool.Execute(context.Background(), `{"action": "start_session"}`)
	require.NoError(t, err)

	assert.Contains(t, out, "Image editing session started successfully.")
	id := sessionIDFrom(t, out)
	assert.Contains(t, tool.sessions, id)
	assert.Equal(t, 1, tool.GetActiveSessions())
	require.NotNil(t, editor.LastOptions())
	assert.Equal(t, "model-x", editor.LastOptions().Model)
	sessions := editor.Sessions()
	require.Len(t, sessions, 1)
	assert.Empty(t, sessions[0].Prompts(), "no image is generated without a prompt")
}

func TestStartSessionWithPrompt(t *testing.T) {
	editor := &testutil.FakeImageEditor{NewSession: respondWith(&interfaces.ImageEditResponse{
		Text:   "here you go",
		Images: []interfaces.GeneratedImage{image("image/png")},
		Usage:  &interfaces.ImageUsage{InputTokens: 3, OutputTokens: 7},
	})}
	tool := New(editor, nil)

	out, err := tool.Execute(context.Background(), `{"action": "start_session", "prompt": "a cat", "aspect_ratio": "16:9", "image_size": "2K"}`)
	require.NoError(t, err)

	session := editor.Sessions()[0]
	assert.Equal(t, []string{"a cat"}, session.Prompts())
	assert.Equal(t, &interfaces.ImageEditOptions{AspectRatio: "16:9", ImageSize: "2K"}, session.LastOptions())

	assert.True(t, strings.HasPrefix(out, "Image editing session started with initial image."))
	id := sessionIDFrom(t, out)
	assert.Contains(t, out, "Model response: here you go")
	assert.Contains(t, out, "![Generated image](data:image/png;base64,YWJjZA==)")
	assert.Contains(t, out, "Format: image/png")
	assert.Contains(t, out, "Size: 4 bytes")
	assert.Contains(t, out, "Tokens used: 3 input, 7 output")
	assert.Contains(t, out, "session_id='"+id+"'")
	assert.Equal(t, 1, tool.GetActiveSessions())
}

func TestStartSessionFailures(t *testing.T) {
	t.Run("editor error", func(t *testing.T) {
		tool := New(&testutil.FakeImageEditor{Err: errors.New("quota")}, nil)
		_, err := tool.Execute(context.Background(), `{"action": "start_session"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create session: quota")
		assert.Zero(t, tool.GetActiveSessions())
	})

	t.Run("prompt too long", func(t *testing.T) {
		editor := &testutil.FakeImageEditor{}
		tool := New(editor, nil, WithMaxPromptLength(3))
		_, err := tool.Execute(context.Background(), `{"action": "start_session", "prompt": "abcd"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prompt exceeds maximum length of 3 characters")
		assert.Zero(t, tool.GetActiveSessions())
		for _, s := range editor.Sessions() {
			assert.True(t, s.Closed(), "no session is left open")
		}
	})

	t.Run("initial generation error", func(t *testing.T) {
		editor := &testutil.FakeImageEditor{NewSession: failWith(errors.New("blocked"))}
		tool := New(editor, nil)
		_, err := tool.Execute(context.Background(), `{"action": "start_session", "prompt": "a cat"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate initial image: blocked")
		assert.Zero(t, tool.GetActiveSessions())
		assert.True(t, editor.Sessions()[0].Closed())
	})
}

func TestSessionLimitIsPerOrg(t *testing.T) {
	tool := New(&testutil.FakeImageEditor{}, nil, WithMaxSessions(1))
	orgA := multitenancy.WithOrgID(context.Background(), "org-a")
	orgB := multitenancy.WithOrgID(context.Background(), "org-b")

	_, err := tool.Execute(orgA, `{"action": "start_session"}`)
	require.NoError(t, err)

	_, err = tool.Execute(orgA, `{"action": "start_session"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum number of concurrent sessions (1) reached")

	_, err = tool.Execute(orgB, `{"action": "start_session"}`)
	require.NoError(t, err)

	assert.Equal(t, 1, tool.GetActiveSessionsForOrg("org-a"))
	assert.Equal(t, 1, tool.GetActiveSessionsForOrg("org-b"))
	assert.Equal(t, 0, tool.GetActiveSessionsForOrg("org-c"))
	assert.Equal(t, 2, tool.GetActiveSessions())
}

func startSession(t *testing.T, tool *Tool, ctx context.Context) string {
	t.Helper()
	out, err := tool.Execute(ctx, `{"action": "start_session"}`)
	require.NoError(t, err)
	return sessionIDFrom(t, out)
}

func TestEditWithStorage(t *testing.T) {
	editor := &testutil.FakeImageEditor{NewSession: respondWith(&interfaces.ImageEditResponse{
		Images: []interfaces.GeneratedImage{image("image/png"), image("image/jpeg")},
	})}
	store := &testutil.FakeImageStorage{}
	tool := New(editor, store)
	ctx := multitenancy.WithOrgID(context.Background(), "org-a")
	id := startSession(t, tool, ctx)

	out, err := tool.Execute(ctx, `{"action": "edit", "session_id": "`+id+`", "prompt": "make it blue"}`)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(out, "Image edited successfully."))
	assert.Contains(t, out, "![Generated image](https://cdn.test/image/png)")
	assert.Contains(t, out, "--- Image 2 ---")
	assert.Contains(t, out, "![Generated image](https://cdn.test/image/jpeg)")
	assert.NotContains(t, out, "Model response:")
	assert.NotContains(t, out, "Tokens used")

	metas := store.Metadata()
	require.Len(t, metas, 2)
	assert.Equal(t, "make it blue", metas[0].Prompt)
	assert.Equal(t, "org-a", metas[0].OrgID)
	assert.False(t, metas[0].CreatedAt.IsZero())
}

func TestEditStorageFailureFallsBackToBase64(t *testing.T) {
	editor := &testutil.FakeImageEditor{NewSession: respondWith(&interfaces.ImageEditResponse{
		Images: []interfaces.GeneratedImage{image("image/png"), image("image/png")},
	})}
	tool := New(editor, &testutil.FakeImageStorage{Err: errors.New("bucket missing")})
	id := startSession(t, tool, context.Background())

	out, err := tool.Execute(context.Background(), `{"action": "edit", "session_id": "`+id+`", "prompt": "p"}`)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(out, "data:image/png;base64,YWJjZA=="))
	assert.Contains(t, out, "--- Image 2 ---")
}

func TestEditWithoutImages(t *testing.T) {
	editor := &testutil.FakeImageEditor{NewSession: respondWith(&interfaces.ImageEditResponse{Text: "cannot do that"})}
	tool := New(editor, nil)
	id := startSession(t, tool, context.Background())

	out, err := tool.Execute(context.Background(), `{"action": "edit", "session_id": "`+id+`", "prompt": "p"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "Model response: cannot do that")
	assert.Contains(t, out, "No image was generated in this turn.")
}

func TestEditFailures(t *testing.T) {
	t.Run("unknown session", func(t *testing.T) {
		tool := New(&testutil.FakeImageEditor{}, nil)
		_, err := tool.Execute(context.Background(), `{"action": "edit", "session_id": "nope", "prompt": "p"}`)
		require.ErrorIs(t, err, interfaces.ErrSessionNotFound)
	})

	t.Run("prompt too long", func(t *testing.T) {
		tool := New(&testutil.FakeImageEditor{}, nil, WithMaxPromptLength(2))
		id := startSession(t, tool, context.Background())
		_, err := tool.Execute(context.Background(), `{"action": "edit", "session_id": "`+id+`", "prompt": "abc"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "prompt exceeds maximum length of 2 characters")
	})

	t.Run("expired session", func(t *testing.T) {
		editor := &testutil.FakeImageEditor{}
		tool := New(editor, nil, WithSessionTimeout(time.Minute))
		id := startSession(t, tool, context.Background())
		tool.sessionsMu.Lock()
		tool.sessions[id].lastUsed = time.Now().Add(-time.Hour)
		tool.sessionsMu.Unlock()

		_, err := tool.Execute(context.Background(), `{"action": "edit", "session_id": "`+id+`", "prompt": "p"}`)
		require.ErrorIs(t, err, interfaces.ErrSessionExpired)
		assert.Zero(t, tool.GetActiveSessions())
		assert.True(t, editor.Sessions()[0].Closed())
	})

	t.Run("send error", func(t *testing.T) {
		editor := &testutil.FakeImageEditor{NewSession: failWith(errors.New("blocked"))}
		tool := New(editor, nil)
		id := startSession(t, tool, context.Background())

		_, err := tool.Execute(context.Background(), `{"action": "edit", "session_id": "`+id+`", "prompt": "p"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to edit image: blocked")
		assert.Equal(t, 1, tool.GetActiveSessions(), "session stays open after a failed edit")
	})
}

func TestEndSession(t *testing.T) {
	editor := &testutil.FakeImageEditor{NewSession: func() *testutil.FakeImageEditSession {
		s := testutil.NewFakeImageEditSession(nil, nil)
		s.SetHistory(make([]interfaces.ImageEditTurn, 4))
		return s
	}}
	tool := New(editor, nil)
	id := startSession(t, tool, context.Background())

	out, err := tool.Execute(context.Background(), `{"action": "end_session", "session_id": "`+id+`"}`)
	require.NoError(t, err)
	assert.Contains(t, out, "Session "+id+" closed successfully.")
	assert.Contains(t, out, "Total turns: 2")
	assert.True(t, editor.Sessions()[0].Closed())
	assert.Zero(t, tool.GetActiveSessions())

	_, err = tool.Execute(context.Background(), `{"action": "end_session", "session_id": "`+id+`"}`)
	require.ErrorIs(t, err, interfaces.ErrSessionNotFound)
}
