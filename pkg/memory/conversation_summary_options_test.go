package memory_test

import (
	"context"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/stretchr/testify/require"
)

func TestConversationSummaryGetMessagesOptions(t *testing.T) {
	llm := &testutil.FakeLLM{DefaultResponse: "earlier conversation"}
	mem := memory.NewConversationSummary(llm, memory.WithMaxBufferSize(4))
	ctx := memory.WithConversationID(multitenancy.WithOrgID(context.Background(), "org"), "conversation")

	for range 4 {
		require.NoError(t, mem.AddMessage(ctx, interfaces.Message{
			Role: interfaces.MessageRoleUser, Content: "earlier turn",
		}))
	}
	require.Equal(t, 1, llm.Calls(), "the retrieval cases must include an actual generated summary")

	require.NoError(t, mem.AddMessage(ctx, interfaces.Message{
		Role: interfaces.MessageRoleUser, Content: "recent question",
	}))
	require.NoError(t, mem.AddMessage(ctx, interfaces.Message{
		Role: interfaces.MessageRoleAssistant, Content: "recent answer",
	}))

	all := []string{"system:earlier conversation", "user:recent question", "assistant:recent answer"}
	tests := []struct {
		name    string
		options []interfaces.GetMessagesOption
		want    []string
	}{
		{name: "unfiltered", want: all},
		{name: "user role excludes summary", options: []interfaces.GetMessagesOption{interfaces.WithRoles("user")}, want: []string{"user:recent question"}},
		{name: "system role includes summary", options: []interfaces.GetMessagesOption{interfaces.WithRoles("system")}, want: []string{"system:earlier conversation"}},
		{name: "unmatched role", options: []interfaces.GetMessagesOption{interfaces.WithRoles("tool")}},
		{name: "limit one", options: []interfaces.GetMessagesOption{interfaces.WithLimit(1)}, want: []string{"assistant:recent answer"}},
		{name: "limit two", options: []interfaces.GetMessagesOption{interfaces.WithLimit(2)}, want: []string{"user:recent question", "assistant:recent answer"}},
		{name: "limit includes summary", options: []interfaces.GetMessagesOption{interfaces.WithLimit(3)}, want: all},
		{name: "zero limit", options: []interfaces.GetMessagesOption{interfaces.WithLimit(0)}, want: all},
		{name: "negative limit", options: []interfaces.GetMessagesOption{interfaces.WithLimit(-1)}, want: all},
		{name: "filter before limit", options: []interfaces.GetMessagesOption{interfaces.WithRoles("system", "user"), interfaces.WithLimit(1)}, want: []string{"user:recent question"}},
		{name: "summary only with limit", options: []interfaces.GetMessagesOption{interfaces.WithRoles("system"), interfaces.WithLimit(1)}, want: []string{"system:earlier conversation"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := mem.GetMessages(ctx, tt.options...)
			require.NoError(t, err)
			var got []string
			for _, message := range messages {
				got = append(got, string(message.Role)+":"+message.Content)
			}
			require.Equal(t, tt.want, got)
		})
	}

	// Retrieval options must not remove the summary or buffered messages from memory.
	messages, err := mem.GetMessages(ctx)
	require.NoError(t, err)
	require.Len(t, messages, len(all))
	require.Equal(t, "earlier conversation", messages[0].Content)
	require.Equal(t, "recent question", messages[1].Content)
	require.Equal(t, "recent answer", messages[2].Content)
}

func TestConversationSummaryGetMessagesOptionsBeforeSummarization(t *testing.T) {
	llm := &testutil.FakeLLM{DefaultResponse: "unused"}
	mem := memory.NewConversationSummary(llm, memory.WithMaxBufferSize(4))
	ctx := memory.WithConversationID(multitenancy.WithOrgID(context.Background(), "org"), "conversation")
	for _, message := range []interfaces.Message{
		{Role: interfaces.MessageRoleUser, Content: "first question"},
		{Role: interfaces.MessageRoleUser, Content: "latest question"},
		{Role: interfaces.MessageRoleAssistant, Content: "answer"},
	} {
		require.NoError(t, mem.AddMessage(ctx, message))
	}
	require.Zero(t, llm.Calls())

	messages, err := mem.GetMessages(ctx, interfaces.WithRoles("user"), interfaces.WithLimit(1))
	require.NoError(t, err)
	require.Equal(t, []interfaces.Message{{Role: interfaces.MessageRoleUser, Content: "latest question"}}, messages)

	messages, err = mem.GetMessages(ctx, interfaces.WithRoles("system"))
	require.NoError(t, err)
	require.Empty(t, messages)
}
