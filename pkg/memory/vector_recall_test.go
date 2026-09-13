package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func recallCtx() context.Context { return summaryCtx() }

func userMsg(content string) interfaces.Message {
	return interfaces.Message{Role: interfaces.MessageRoleUser, Content: content}
}

// TestVectorStoreIsWriteOnlyWithoutAutoRecall pins the default, and documents
// the trap.
//
// Every LLM provider builds its request with a bare, optionless GetMessages
// call, and interfaces.WithQuery has no caller anywhere in the module. So
// opts.Query is always "" and the similarity search is unreachable: callers pay
// to embed and store every message into a store that nothing ever reads.
//
// That default is preserved (turning recall on silently would change what every
// existing agent sends to its model), which makes the opt-in below the only way
// to get value from the store.
func TestVectorStoreIsWriteOnlyWithoutAutoRecall(t *testing.T) {
	store := &testutil.FakeVectorStore{
		Results: []interfaces.SearchResult{{
			Document: interfaces.Document{
				Content:  "the deploy key is rotated every 90 days",
				Metadata: map[string]interface{}{"role": "assistant"},
			},
			Score: 0.9,
		}},
	}

	mem := NewVectorStoreRetriever(store)
	ctx := recallCtx()

	require.NoError(t, mem.AddMessage(ctx, userMsg("how often is the deploy key rotated?")))

	msgs, err := mem.GetMessages(ctx)
	require.NoError(t, err)

	assert.NotEmpty(t, store.Stored(), "messages should still be written to the store")
	assert.Empty(t, store.Searches(), "without auto-recall the store is never searched")
	for _, m := range msgs {
		assert.NotContains(t, m.Content, "90 days",
			"recalled content must not appear when auto-recall is off")
	}
}

// TestAutoRecallSearchesUsingTheLatestUserMessage asserts the store becomes
// readable, using the question actually being asked as the query.
func TestAutoRecallSearchesUsingTheLatestUserMessage(t *testing.T) {
	store := &testutil.FakeVectorStore{
		Results: []interfaces.SearchResult{{
			Document: interfaces.Document{
				Content:  "the deploy key is rotated every 90 days",
				Metadata: map[string]interface{}{"role": "assistant"},
			},
			Score: 0.9,
		}},
	}

	mem := NewVectorStoreRetriever(store, WithAutoRecall())
	ctx := recallCtx()

	require.NoError(t, mem.AddMessage(ctx, userMsg("what about the deploy key?")))

	msgs, err := mem.GetMessages(ctx)
	require.NoError(t, err)

	require.Len(t, store.Searches(), 1, "the store should have been searched exactly once")
	assert.Equal(t, "what about the deploy key?", store.Searches()[0],
		"the recall query should be the message the agent is being asked to answer")

	require.NotEmpty(t, msgs)
	assert.Contains(t, msgs[0].Content, "90 days", "recalled content should reach the model")
	assert.Equal(t, interfaces.MessageRoleSystem, msgs[0].Role)
}

// TestAutoRecallDoesNotReorderTheTranscript is the correctness guarantee that
// matters most here.
//
// A similarity search returns results that are neither contiguous nor
// chronological. Splicing them into the message list would separate
// tool_call / tool_result pairs, which both the OpenAI and Anthropic APIs
// reject outright. Recalled content is therefore delivered as a single leading
// context message and the real sequence is left untouched.
func TestAutoRecallDoesNotReorderTheTranscript(t *testing.T) {
	store := &testutil.FakeVectorStore{
		Results: []interfaces.SearchResult{{
			Document: interfaces.Document{
				Content:  "some older relevant thing",
				Metadata: map[string]interface{}{"role": "assistant"},
			},
			Score: 0.8,
		}},
	}

	mem := NewVectorStoreRetriever(store, WithAutoRecall())
	ctx := recallCtx()

	// An assistant turn carrying only a tool call, followed by its result.
	require.NoError(t, mem.AddMessage(ctx, userMsg("check the weather")))
	require.NoError(t, mem.AddMessage(ctx, interfaces.Message{
		Role:      interfaces.MessageRoleAssistant,
		ToolCalls: []interfaces.ToolCall{{ID: "call_1", Name: "get_weather"}},
	}))
	require.NoError(t, mem.AddMessage(ctx, interfaces.Message{
		Role:       interfaces.MessageRoleTool,
		Content:    "sunny",
		ToolCallID: "call_1",
	}))

	msgs, err := mem.GetMessages(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(msgs), 4, "context message plus the three real turns")

	// The recalled context is prepended; everything after it is the original
	// sequence, in order.
	assert.Equal(t, interfaces.MessageRoleSystem, msgs[0].Role)

	tail := msgs[1:]
	require.Len(t, tail, 3)
	assert.Equal(t, interfaces.MessageRoleUser, tail[0].Role)
	assert.Equal(t, interfaces.MessageRoleAssistant, tail[1].Role)
	require.Len(t, tail[1].ToolCalls, 1, "the tool call must survive intact")
	assert.Equal(t, interfaces.MessageRoleTool, tail[2].Role)
	assert.Equal(t, "call_1", tail[2].ToolCallID,
		"the tool result must still immediately follow its call")
}

// TestAutoRecallSkipsContentAlreadyPresent asserts the model is not told the
// same thing twice when a search returns something already in the window.
func TestAutoRecallSkipsContentAlreadyPresent(t *testing.T) {
	store := &testutil.FakeVectorStore{
		Results: []interfaces.SearchResult{{
			Document: interfaces.Document{
				Content:  "what about the deploy key?",
				Metadata: map[string]interface{}{"role": "user"},
			},
			Score: 0.99,
		}},
	}

	mem := NewVectorStoreRetriever(store, WithAutoRecall())
	ctx := recallCtx()

	require.NoError(t, mem.AddMessage(ctx, userMsg("what about the deploy key?")))

	msgs, err := mem.GetMessages(ctx)
	require.NoError(t, err)

	for _, m := range msgs {
		if m.Role == interfaces.MessageRoleSystem && strings.Contains(m.Content, "Relevant excerpts") {
			t.Errorf("a context message was added for content already in the window: %q", m.Content)
		}
	}
}

// TestAutoRecallSurvivesSearchFailure asserts a failing vector store degrades to
// the plain transcript rather than failing the turn. Recall is an enhancement;
// losing it must not lose the conversation.
func TestAutoRecallSurvivesSearchFailure(t *testing.T) {
	store := &testutil.FakeVectorStore{SearchErr: assertAnError{}}

	mem := NewVectorStoreRetriever(store, WithAutoRecall())
	ctx := recallCtx()

	require.NoError(t, mem.AddMessage(ctx, userMsg("hello")))

	msgs, err := mem.GetMessages(ctx)
	require.NoError(t, err, "a recall failure must not fail the turn")
	require.Len(t, msgs, 1)
	assert.Equal(t, "hello", msgs[0].Content)
}

type assertAnError struct{}

func (assertAnError) Error() string { return "vector store unavailable" }
