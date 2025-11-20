package interfaces

type AgentResponse struct {
	Content          string
	Usage            *TokenUsage
	AgentName        string
	Model            string
	ExecutionSummary ExecutionSummary
	Metadata         map[string]interface{}
}

type ExecutionSummary struct {
	LLMCalls        int
	ToolCalls       int
	SubAgentCalls   int
	ExecutionTimeMs int64
	UsedTools       []string
	UsedSubAgents   []string
}
