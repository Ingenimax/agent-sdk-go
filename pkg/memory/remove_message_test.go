package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

var (
	_ interfaces.MessageRemover = (*ConversationBuffer)(nil)
	_ interfaces.MessageRemover = (*ConversationSummary)(nil)
	_ interfaces.MessageRemover = (*VectorStoreRetriever)(nil)
	_ interfaces.MessageRemover = (*RedisMemory)(nil)
)

type removableMemory interface {
	interfaces.Memory
	interfaces.MessageRemover
}

func removeCtx(conversationID string) context.Context {
	return WithConversationID(multitenancy.WithOrgID(context.Background(), "org1"), conversationID)
}

func withContent(content string) func(interfaces.Message) bool {
	return func(m interfaces.Message) bool { return m.Content == content }
}

// Every memory in this package can take back a stored message, which the agent
// relies on to drop a user message the model refused.
func TestRemoveLastMessage(t *testing.T) {
	memories := map[string]func(t *testing.T) removableMemory{
		"buffer": func(*testing.T) removableMemory { return NewConversationBuffer() },
		"summary": func(*testing.T) removableMemory {
			return NewConversationSummary(testutil.NewFakeLLM(), WithMaxBufferSize(50))
		},
		"vector": func(*testing.T) removableMemory {
			return NewVectorStoreRetriever(&testutil.FakeVectorStore{})
		},
		"redis": func(t *testing.T) removableMemory {
			mem, _ := newRedisMemory(t)
			return mem
		},
	}

	pdf := interfaces.Message{
		Role:    interfaces.MessageRoleUser,
		Content: "summarize this",
		Parts: []*interfaces.Part{
			interfaces.NewPartFromBytes([]byte("%PDF-1.7"), "application/pdf"),
			interfaces.NewPartFromText("summarize this"),
		},
	}

	for name, newMemory := range memories {
		t.Run(name, func(t *testing.T) {
			mem := newMemory(t)
			ctx := removeCtx("conv1")
			other := removeCtx("conv2")

			for _, m := range []interfaces.Message{
				{Role: interfaces.MessageRoleUser, Content: "hi"},
				{Role: interfaces.MessageRoleAssistant, Content: "ok"},
				pdf,
				{Role: interfaces.MessageRoleUser, Content: "hi"},
			} {
				require.NoError(t, mem.AddMessage(ctx, m))
			}
			require.NoError(t, mem.AddMessage(other, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "hi"}))

			// The most recent match goes, not the first one.
			removed, err := mem.RemoveLastMessage(ctx, withContent("hi"))
			require.NoError(t, err)
			assert.True(t, removed)

			// A message with parts survives storage well enough to be matched.
			removed, err = mem.RemoveLastMessage(ctx, func(m interfaces.Message) bool {
				return len(m.Parts) == 2 && m.Parts[0].MIMEType() == "application/pdf"
			})
			require.NoError(t, err)
			assert.True(t, removed)

			removed, err = mem.RemoveLastMessage(ctx, withContent("never stored"))
			require.NoError(t, err)
			assert.False(t, removed)

			msgs, err := mem.GetMessages(ctx)
			require.NoError(t, err)
			require.Len(t, msgs, 2)
			assert.Equal(t, "hi", msgs[0].Content)
			assert.Equal(t, "ok", msgs[1].Content)

			otherMsgs, err := mem.GetMessages(other)
			require.NoError(t, err)
			assert.Len(t, otherMsgs, 1, "another conversation must be left alone")
		})
	}
}

func TestConversationBufferRemoveLastMessageDropsEmptyConversation(t *testing.T) {
	buffer := NewConversationBuffer()
	ctx := removeCtx("conv1")
	require.NoError(t, buffer.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: "only"}))

	removed, err := buffer.RemoveLastMessage(ctx, withContent("only"))
	require.NoError(t, err)
	require.True(t, removed)

	conversations, err := buffer.GetAllConversations(ctx)
	require.NoError(t, err)
	assert.Empty(t, conversations, "a conversation emptied by removal should not be listed")
}

// GetMessages returns the stored slice itself, so a removal must not shift
// elements under a caller still holding an earlier result.
func TestConversationBufferRemoveLastMessageKeepsEarlierReads(t *testing.T) {
	buffer := NewConversationBuffer()
	ctx := removeCtx("conv1")
	for _, content := range []string{"a", "b", "c"} {
		require.NoError(t, buffer.AddMessage(ctx, interfaces.Message{Role: interfaces.MessageRoleUser, Content: content}))
	}
	earlier, err := buffer.GetMessages(ctx)
	require.NoError(t, err)

	removed, err := buffer.RemoveLastMessage(ctx, withContent("a"))
	require.NoError(t, err)
	require.True(t, removed)

	assert.Equal(t, []string{"a", "b", "c"}, []string{earlier[0].Content, earlier[1].Content, earlier[2].Content})
}

func TestRemoveLastMessageNeedsConversationID(t *testing.T) {
	buffer := NewConversationBuffer()
	_, err := buffer.RemoveLastMessage(context.Background(), withContent("hi"))
	assert.Error(t, err)

	redisMem, _ := newRedisMemory(t)
	_, err = redisMem.RemoveLastMessage(context.Background(), withContent("hi"))
	assert.Error(t, err)
}
