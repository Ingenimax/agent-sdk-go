// Package eval provides provider-neutral evaluation of agents.
package eval

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

const (
	// DatasetSchemaVersion is the dataset format understood by this package.
	DatasetSchemaVersion = "1"
	// ReportSchemaVersion is the report format emitted by this package.
	ReportSchemaVersion = "1"
	// RootAgentPath is the conventional path used for the evaluated root agent.
	RootAgentPath = "root"
)

// Dataset is an ordered collection of evaluation cases.
type Dataset struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	Cases         []Case `json:"cases"`
}

// Case is one independent agent execution and its checks.
type Case struct {
	ID        string   `json:"id"`
	Input     string   `json:"input"`
	Reference *string  `json:"reference,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Checks    []Check  `json:"checks"`
}

// Check configures one evaluator. Config is interpreted by the evaluator named
// in Type. Pointer fields preserve the distinction between omitted values and
// explicit false/zero values while loading a dataset.
type Check struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Config    json.RawMessage `json:"config"`
	Threshold *float64        `json:"threshold,omitempty"`
	Required  *bool           `json:"required,omitempty"`
}

// ThresholdValue returns the resolved threshold. The default is 1.
func (c Check) ThresholdValue() float64 {
	if c.Threshold == nil {
		return 1
	}
	return *c.Threshold
}

// RequiredValue returns whether a check gates the case. The default is true.
func (c Check) RequiredValue() bool {
	if c.Required == nil {
		return true
	}
	return *c.Required
}

// Coverage describes how completely an observation captured a capability.
type Coverage string

const (
	CoverageComplete    Coverage = "complete"
	CoveragePartial     Coverage = "partial"
	CoverageUnavailable Coverage = "unavailable"
)

// Capabilities describes which parts of an execution are trustworthy enough
// to grade.
type Capabilities struct {
	ToolCapture Coverage `json:"tool_capture"`
	Usage       Coverage `json:"usage"`
	Output      Coverage `json:"output"`
}

// RunStatus is the terminal execution state of an observation.
type RunStatus string

const (
	RunStatusCompleted  RunStatus = "completed"
	RunStatusError      RunStatus = "error"
	RunStatusCancelled  RunStatus = "cancelled"
	RunStatusNotStarted RunStatus = "not_started"
)

// ErrorDetail is a serializable error. Stage identifies where the failure
// happened without exposing implementation-specific error values.
type ErrorDetail struct {
	Stage   string `json:"stage"`
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
}

// Durations separates agent execution from setup, grading, and cleanup.
type Durations struct {
	SetupMS     int64 `json:"setup_ms,omitempty"`
	ExecutionMS int64 `json:"execution_ms,omitempty"`
	GradingMS   int64 `json:"grading_ms,omitempty"`
	CleanupMS   int64 `json:"cleanup_ms,omitempty"`
	TotalMS     int64 `json:"total_ms,omitempty"`
}

// TokenUsage is the stable report representation of interfaces.TokenUsage.
type TokenUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	TotalTokens              int `json:"total_tokens"`
	ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// ExecutionSummary is the stable report representation of an agent execution
// summary. It intentionally excludes arbitrary response metadata.
type ExecutionSummary struct {
	LLMCalls        int                   `json:"llm_calls"`
	ToolCalls       int                   `json:"tool_calls"`
	SubAgentCalls   int                   `json:"sub_agent_calls"`
	ExecutionTimeMS int64                 `json:"execution_time_ms"`
	UsedTools       []string              `json:"used_tools,omitempty"`
	UsedSubAgents   []string              `json:"used_sub_agents,omitempty"`
	UsageByModel    map[string]TokenUsage `json:"usage_by_model,omitempty"`
}

// ToolLayer identifies whether a span represents a provider attempt or an
// actual execution after policy decorators.
type ToolLayer string

const (
	ToolLayerAttempt   ToolLayer = "attempt"
	ToolLayerExecution ToolLayer = "execution"
)

// ToolSpanStatus is the terminal state of a tool span.
type ToolSpanStatus string

const (
	ToolSpanCompleted      ToolSpanStatus = "completed"
	ToolSpanError          ToolSpanStatus = "error"
	ToolSpanPanicked       ToolSpanStatus = "panicked"
	ToolSpanShortCircuited ToolSpanStatus = "short_circuited"
	ToolSpanIncomplete     ToolSpanStatus = "incomplete"
)

// ToolSpan records one invocation observed at one decorator layer.
type ToolSpan struct {
	ID               string         `json:"id"`
	ParentID         string         `json:"parent_id,omitempty"`
	AgentPath        string         `json:"agent_path"`
	Layer            ToolLayer      `json:"layer"`
	Sequence         uint64         `json:"sequence"`
	Tool             string         `json:"tool"`
	Method           string         `json:"method"`
	Arguments        string         `json:"arguments,omitempty"`
	Result           string         `json:"result,omitempty"`
	Error            string         `json:"error,omitempty"`
	StartedAt        time.Time      `json:"started_at"`
	EndedAt          *time.Time     `json:"ended_at,omitempty"`
	Status           ToolSpanStatus `json:"status"`
	ContentTruncated bool           `json:"content_truncated,omitempty"`
}

// Trace is an immutable snapshot of recorded tool spans.
type Trace struct {
	Spans      []ToolSpan `json:"spans"`
	Incomplete bool       `json:"incomplete,omitempty"`
	Reason     string     `json:"reason,omitempty"`
}

// Observation is everything graders may inspect about one execution.
type Observation struct {
	CaseID            string           `json:"case_id"`
	AttemptID         string           `json:"attempt_id"`
	Status            RunStatus        `json:"status"`
	Output            *string          `json:"output,omitempty"`
	Error             *ErrorDetail     `json:"error,omitempty"`
	AgentName         string           `json:"agent_name,omitempty"`
	Model             string           `json:"model,omitempty"`
	Usage             *TokenUsage      `json:"usage,omitempty"`
	ExecutionSummary  ExecutionSummary `json:"execution_summary"`
	Trace             Trace            `json:"trace"`
	Capabilities      Capabilities     `json:"capabilities"`
	ExecutionDuration int64            `json:"execution_duration_ms"`
	Regradable        bool             `json:"regradable"`
}

// MetricStatus is the outcome of evaluating one check.
type MetricStatus string

const (
	MetricStatusPass        MetricStatus = "pass"
	MetricStatusFail        MetricStatus = "fail"
	MetricStatusUnavailable MetricStatus = "unavailable"
	MetricStatusError       MetricStatus = "error"
)

// MetricResult is the result of one configured check.
type MetricResult struct {
	CheckID   string         `json:"check_id"`
	Evaluator string         `json:"evaluator"`
	Status    MetricStatus   `json:"status"`
	Score     *float64       `json:"score,omitempty"`
	Threshold float64        `json:"threshold"`
	Required  bool           `json:"required"`
	Message   string         `json:"message,omitempty"`
	Evidence  map[string]any `json:"evidence,omitempty"`
}

// CaseStatus is the suite-level result for a case.
type CaseStatus string

const (
	CaseStatusPass       CaseStatus = "pass"
	CaseStatusFail       CaseStatus = "fail"
	CaseStatusError      CaseStatus = "error"
	CaseStatusNotStarted CaseStatus = "not_started"
)

// CaseResult combines an observation and its metric results.
type CaseResult struct {
	CaseID      string         `json:"case_id"`
	AttemptID   string         `json:"attempt_id,omitempty"`
	Status      CaseStatus     `json:"status"`
	Observation Observation    `json:"observation"`
	Metrics     []MetricResult `json:"metrics,omitempty"`
	Errors      []ErrorDetail  `json:"errors,omitempty"`
	Durations   Durations      `json:"durations"`
}

// MetricSummary aggregates one evaluator without hiding unavailable results.
type MetricSummary struct {
	Evaluator   string  `json:"evaluator"`
	Scored      int     `json:"scored"`
	Unavailable int     `json:"unavailable"`
	Errors      int     `json:"errors"`
	MeanScore   float64 `json:"mean_score,omitempty"`
}

// Summary aggregates the suite result.
type Summary struct {
	Total      int             `json:"total"`
	Passed     int             `json:"passed"`
	Failed     int             `json:"failed"`
	Errors     int             `json:"errors"`
	NotStarted int             `json:"not_started"`
	PassRate   float64         `json:"pass_rate"`
	Metrics    []MetricSummary `json:"metrics,omitempty"`
}

// Report is the versioned result of a dataset run.
type Report struct {
	SchemaVersion     string       `json:"schema_version"`
	DatasetID         string       `json:"dataset_id"`
	DatasetDigest     string       `json:"dataset_digest"`
	ConfigFingerprint string       `json:"config_fingerprint,omitempty"`
	BuildRevision     string       `json:"build_revision,omitempty"`
	StartedAt         time.Time    `json:"started_at"`
	EndedAt           time.Time    `json:"ended_at"`
	ResolvedDataset   *Dataset     `json:"resolved_dataset,omitempty"`
	Cases             []CaseResult `json:"cases"`
	Summary           Summary      `json:"summary"`
}

// Subject is the minimum execution API required by Runner.
type Subject interface {
	RunDetailed(context.Context, string) (*interfaces.AgentResponse, error)
}

// Target owns a freshly constructed subject and its cleanup callback.
type Target struct {
	Subject      Subject
	Close        func(context.Context) error
	Capabilities Capabilities
}

// TargetFactory builds isolated state for one case attempt.
type TargetFactory func(context.Context, Case, *Recorder) (Target, error)

// Evaluation is the input to one evaluator invocation.
type Evaluation struct {
	Case        Case
	Check       Check
	Observation Observation
}

// Evaluator validates and evaluates one check type.
type Evaluator interface {
	Name() string
	Validate(Check) error
	Evaluate(context.Context, Evaluation) (MetricResult, error)
}

// Evaluators maps check types to their implementations.
type Evaluators map[string]Evaluator

// score returns a stable pointer for result construction.
func score(value float64) *float64 { return &value }
