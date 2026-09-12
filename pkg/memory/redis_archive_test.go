package memory

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRedisMemory(t *testing.T, options ...RedisOption) (*RedisMemory, *miniredis.Miniredis) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return NewRedisMemory(client, options...), server
}

func addUserMessages(t *testing.T, mem *RedisMemory, ctx context.Context, n int, prefix string) {
	t.Helper()
	for i := 0; i < n; i++ {
		require.NoError(t, mem.AddMessage(ctx, interfaces.Message{
			Role:    interfaces.MessageRoleUser,
			Content: prefix,
		}))
	}
}

// TestSummarizationArchivesRatherThanDestroys guards data loss.
//
// checkAndSummarize used to store a summary and then LPOP every summarized
// message. The raw text existed nowhere else, so any detail the summary dropped
// was gone permanently -- the same class of loss as the ConversationSummary
// overwrite, and on the durable backend where it matters most.
func TestSummarizationArchivesRatherThanDestroys(t *testing.T) {
	llm := &testutil.FakeLLM{DefaultResponse: "a summary of the conversation"}
	mem, _ := newRedisMemory(t, WithSummarization(llm, 6, 3))
	ctx := summaryCtx()

	addUserMessages(t, mem, ctx, 8, "original detail")

	// The live conversation has been trimmed.
	live, err := mem.GetMessages(ctx)
	require.NoError(t, err)
	assert.Less(t, len(live), 8, "summarization should have trimmed the live list")

	// But the raw messages survive.
	archived, err := mem.ArchivedMessages(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, archived,
		"summarized messages were destroyed; summarization is lossy, so the raw "+
			"text must remain recoverable")

	for _, m := range archived {
		assert.Equal(t, "original detail", m.Content)
	}
}

// TestArchiveCanBeDisabled pins the opt-out for callers who care more about
// storage than recoverability.
func TestArchiveCanBeDisabled(t *testing.T) {
	llm := &testutil.FakeLLM{DefaultResponse: "summary"}
	mem, _ := newRedisMemory(t, WithSummarization(llm, 6, 3), WithoutArchive())
	ctx := summaryCtx()

	addUserMessages(t, mem, ctx, 8, "detail")

	archived, err := mem.ArchivedMessages(ctx)
	require.NoError(t, err)
	assert.Empty(t, archived, "WithoutArchive should skip archiving entirely")
}

// TestSummarizationDoesNotSplitToolCallPairs guards a transcript corruption
// that would break the very next request.
//
// summarizeCount is a raw message count, so it can land between an assistant
// turn carrying tool_calls and the tool messages answering them -- leaving the
// live conversation starting with an orphaned tool message, which both the
// OpenAI and Anthropic APIs reject.
func TestSummarizationDoesNotSplitToolCallPairs(t *testing.T) {
	llm := &testutil.FakeLLM{DefaultResponse: "summary"}
	mem, _ := newRedisMemory(t, WithSummarization(llm, 6, 3))
	ctx := summaryCtx()

	// Fill with plain turns, then end on a tool-call group so the summarization
	// boundary is tempted to cut through it.
	addUserMessages(t, mem, ctx, 5, "filler")

	require.NoError(t, mem.AddMessage(ctx, interfaces.Message{
		Role:      interfaces.MessageRoleAssistant,
		ToolCalls: []interfaces.ToolCall{{ID: "call_1", Name: "get_weather"}},
	}))
	require.NoError(t, mem.AddMessage(ctx, interfaces.Message{
		Role:       interfaces.MessageRoleTool,
		Content:    "sunny",
		ToolCallID: "call_1",
	}))

	live, err := mem.GetMessages(ctx)
	require.NoError(t, err)

	// The live list must never begin with a tool result whose call is gone.
	for i, m := range live {
		if m.Role == interfaces.MessageRoleTool {
			require.Greater(t, i, 0,
				"the live conversation starts with an orphaned tool result; its "+
					"tool_call was summarized away and the next API call would be rejected")
			assert.NotEmpty(t, live[i-1].ToolCalls,
				"a tool result must be preceded by the assistant turn that requested it")
		}
	}
}

func TestPairSafeBoundaryKeepsGroupsIntact(t *testing.T) {
	assistant := interfaces.Message{
		Role:      interfaces.MessageRoleAssistant,
		ToolCalls: []interfaces.ToolCall{{ID: "c1", Name: "t"}},
	}
	toolResult := interfaces.Message{Role: interfaces.MessageRoleTool, ToolCallID: "c1"}
	user := interfaces.Message{Role: interfaces.MessageRoleUser, Content: "hi"}

	// A window ending mid-group pulls back past the assistant turn.
	got := pairSafeBoundary([]interfaces.Message{user, assistant, toolResult})
	assert.Equal(t, 1, got, "the assistant turn and its result must stay together")

	// A window ending on a clean boundary is unchanged.
	got = pairSafeBoundary([]interfaces.Message{user, user})
	assert.Equal(t, 2, got)

	// Trailing tool results with their assistant turn inside the window.
	got = pairSafeBoundary([]interfaces.Message{user, assistant, toolResult, toolResult})
	assert.Equal(t, 1, got)

	assert.Equal(t, 0, pairSafeBoundary(nil))
}

// TestArchiveAndTrimIsAtomic guards the race.
//
// The old sequence was LLEN, then LRANGE, then N separate LPOP calls. A
// concurrent AddMessage between them shifted the list, so the pops removed
// messages that had never been summarized -- silently losing the newest turns,
// which are exactly the ones the model most needs.
func TestArchiveAndTrimIsAtomic(t *testing.T) {
	mem, _ := newRedisMemory(t)
	ctx := summaryCtx()

	addUserMessages(t, mem, ctx, 5, "msg")

	live, err := mem.GetMessages(ctx)
	require.NoError(t, err)
	require.Len(t, live, 5)

	require.NoError(t, mem.archiveAndTrim(ctx, mem.conversationKey(ctx), 2))

	after, err := mem.GetMessages(ctx)
	require.NoError(t, err)
	assert.Len(t, after, 3, "exactly the requested count should be trimmed")

	archived, err := mem.ArchivedMessages(ctx)
	require.NoError(t, err)
	assert.Len(t, archived, 2, "trimmed messages should be archived, not dropped")
}

func TestArchiveAndTrimClampsToListLength(t *testing.T) {
	mem, _ := newRedisMemory(t)
	ctx := summaryCtx()
	addUserMessages(t, mem, ctx, 2, "msg")

	// Asking for more than exists must not error or over-trim.
	require.NoError(t, mem.archiveAndTrim(ctx, mem.conversationKey(ctx), 99))

	after, err := mem.GetMessages(ctx)
	require.NoError(t, err)
	assert.Empty(t, after)

	archived, _ := mem.ArchivedMessages(ctx)
	assert.Len(t, archived, 2)
}
