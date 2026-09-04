package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingLLM captures every summarization prompt it is asked to answer, so
// tests can assert on what the summarizer was actually shown.
type recordingLLM struct {
	mu      sync.Mutex
	prompts []string
	calls   int
}

func (m *recordingLLM) Name() string            { return "recording" }
func (m *recordingLLM) SupportsStreaming() bool { return false }

func (m *recordingLLM) Generate(_ context.Context, prompt string, _ ...interfaces.GenerateOption) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.prompts = append(m.prompts, prompt)
	return fmt.Sprintf("summary-%d", m.calls), nil
}

func (m *recordingLLM) GenerateWithTools(context.Context, string, []interfaces.Tool, ...interfaces.GenerateOption) (string, error) {
	return "", nil
}

func (m *recordingLLM) GenerateDetailed(context.Context, string, ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return &interfaces.LLMResponse{}, nil
}

func (m *recordingLLM) GenerateWithToolsDetailed(context.Context, string, []interfaces.Tool, ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	return &interfaces.LLMResponse{}, nil
}

func (m *recordingLLM) promptAt(i int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.prompts[i]
}

func (m *recordingLLM) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// summaryCtx builds a context carrying the org and conversation IDs that
// memory scoping requires; getConversationID hard-fails without both.
func summaryCtx() context.Context {
	ctx := multitenancy.WithOrgID(context.Background(), "org1")
	return WithConversationID(ctx, "conv1")
}

func addMessages(t *testing.T, mem interfaces.Memory, ctx context.Context, n int, prefix string) {
	t.Helper()
	for i := 0; i < n; i++ {
		require.NoError(t, mem.AddMessage(ctx, interfaces.Message{
			Role:    interfaces.MessageRoleUser,
			Content: fmt.Sprintf("%s-%d", prefix, i),
		}))
	}
}

// TestConversationSummary_TriggerFiresAboveHardcodedBufferSize guards the dead
// trigger.
//
// NewConversationSummary used to build its inner buffer with a bare
// NewConversationBuffer(), which hardcodes maxSize 100 and trims from the front
// on every add. The summarization trigger compares len(messages) against
// maxBufferSize using that same trimmed buffer, so any caller passing
// WithMaxBufferSize above 100 had a summarizer that could never reach its own
// threshold. The feature was off, with no error and no warning.
func TestConversationSummary_TriggerFiresAboveHardcodedBufferSize(t *testing.T) {
	llm := &recordingLLM{}
	mem := NewConversationSummary(llm, WithMaxBufferSize(120))
	ctx := summaryCtx()

	// One short of the threshold: nothing should have been summarized.
	addMessages(t, mem, ctx, 119, "msg")
	require.Equal(t, 0, llm.callCount(), "summarized before reaching the threshold")

	// Crossing it must trigger exactly once.
	addMessages(t, mem, ctx, 1, "trigger")
	assert.Equal(t, 1, llm.callCount(),
		"summarization never fired at maxBufferSize=120: the inner buffer is capped below the threshold")

	msgs, err := mem.GetMessages(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, msgs)
	assert.Equal(t, "summary-1", msgs[0].Content)
}

// TestConversationSummary_PriorSummaryIsFoldedIn guards against silent history
// loss.
//
// The new summary used to replace the old one with a plain map assignment while
// summarize() only ever saw the current buffer. Because the raw messages had
// already been cleared from the only store, every summarization after the first
// destroyed all earlier history irrecoverably.
func TestConversationSummary_PriorSummaryIsFoldedIn(t *testing.T) {
	llm := &recordingLLM{}
	mem := NewConversationSummary(llm, WithMaxBufferSize(4))
	ctx := summaryCtx()

	// First round: fills the buffer and summarizes.
	addMessages(t, mem, ctx, 4, "first")
	require.Equal(t, 1, llm.callCount())
	assert.NotContains(t, llm.promptAt(0), "Summary of the conversation so far",
		"there was no prior summary to fold in on the first round")

	// Second round: must carry the first summary into the prompt.
	addMessages(t, mem, ctx, 4, "second")
	require.Equal(t, 2, llm.callCount())

	second := llm.promptAt(1)
	assert.Contains(t, second, "Summary of the conversation so far",
		"second summarization did not include the prior summary, so the first round's history is lost")
	assert.Contains(t, second, "summary-1",
		"the prior summary's content must reach the summarizer, not just a header")
	assert.Contains(t, second, "second-0", "the new messages must also be summarized")
}

// TestConversationSummary_CountAccumulates asserts the summary reports how many
// raw messages it stands for across every round folded into it, rather than
// only the most recent window.
func TestConversationSummary_CountAccumulates(t *testing.T) {
	llm := &recordingLLM{}
	mem := NewConversationSummary(llm, WithMaxBufferSize(4))
	ctx := summaryCtx()

	addMessages(t, mem, ctx, 4, "first")
	addMessages(t, mem, ctx, 4, "second")

	msgs, err := mem.GetMessages(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, msgs)

	count, ok := msgs[0].Metadata["count"].(int)
	require.True(t, ok, "summary should carry an int count")
	assert.Equal(t, 8, count, "count should span both summarized rounds, not just the last window")
}

// TestConversationSummary_RendersToolCalls asserts that assistant turns which
// carry only tool calls are described rather than rendered as a bare role name
// with empty content, which told the summarizer nothing about what the agent did.
func TestConversationSummary_RendersToolCalls(t *testing.T) {
	llm := &recordingLLM{}
	mem := NewConversationSummary(llm, WithMaxBufferSize(2))
	ctx := summaryCtx()

	require.NoError(t, mem.AddMessage(ctx, interfaces.Message{
		Role:    interfaces.MessageRoleUser,
		Content: "what is the weather",
	}))
	require.NoError(t, mem.AddMessage(ctx, interfaces.Message{
		Role:      interfaces.MessageRoleAssistant,
		Content:   "",
		ToolCalls: []interfaces.ToolCall{{Name: "get_weather", Arguments: `{"city":"Oslo"}`}},
	}))

	require.Equal(t, 1, llm.callCount())
	assert.True(t, strings.Contains(llm.promptAt(0), "get_weather"),
		"a tool-call-only assistant turn must name the tools it called; got:\n%s", llm.promptAt(0))
}
