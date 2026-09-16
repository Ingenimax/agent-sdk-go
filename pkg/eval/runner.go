package eval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// RunnerOptions configures suite execution. Durations of zero mean no extra
// deadline. Concurrency defaults to one.
type RunnerOptions struct {
	Concurrency       int
	CaseTimeout       time.Duration
	GradingTimeout    time.Duration
	CleanupTimeout    time.Duration
	Recorder          RecorderOptions
	ConfigFingerprint string
	BuildRevision     string
	Now               func() time.Time
	AttemptID         func() string
}

// Runner executes validated cases through freshly constructed targets.
type Runner struct {
	Factory    TargetFactory
	Evaluators Evaluators
	Options    RunnerOptions
}

// Run validates the complete suite before creating a target, then executes
// cases with bounded concurrency. Per-case failures are represented in Report;
// returned errors are reserved for invalid suite configuration or cancellation.
func (r *Runner) Run(ctx context.Context, dataset Dataset) (Report, error) {
	now := r.Options.Now
	if now == nil {
		now = time.Now
	}
	report := Report{
		SchemaVersion:     ReportSchemaVersion,
		ConfigFingerprint: r.Options.ConfigFingerprint,
		BuildRevision:     r.Options.BuildRevision,
		StartedAt:         now().UTC(),
	}
	if r.Factory == nil {
		report.EndedAt = now().UTC()
		return report, errors.New("eval: target factory is nil")
	}

	resolved := cloneDataset(dataset)
	evaluators := mergedEvaluators(r.Evaluators)
	if err := ValidateDataset(&resolved, evaluators); err != nil {
		report.EndedAt = now().UTC()
		return report, err
	}
	digest, err := DatasetDigest(resolved)
	if err != nil {
		report.EndedAt = now().UTC()
		return report, err
	}
	report.DatasetID = resolved.ID
	report.DatasetDigest = digest
	report.ResolvedDataset = &resolved
	report.Cases = make([]CaseResult, len(resolved.Cases))
	for i, evalCase := range resolved.Cases {
		report.Cases[i] = notStartedResult(evalCase.ID)
	}

	concurrency := r.Options.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(resolved.Cases) {
		concurrency = len(resolved.Cases)
	}

	type indexedResult struct {
		index  int
		result CaseResult
	}
	jobs := make(chan int)
	results := make(chan indexedResult, concurrency)
	var workers sync.WaitGroup
	workers.Add(concurrency)
	for worker := 0; worker < concurrency; worker++ {
		go func() {
			defer workers.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					results <- indexedResult{index: index, result: notStartedResult(resolved.Cases[index].ID)}
					continue
				}
				results <- indexedResult{
					index:  index,
					result: r.runCase(ctx, resolved.Cases[index], evaluators),
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for index := range resolved.Cases {
			select {
			case jobs <- index:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	for item := range results {
		report.Cases[item.index] = item.result
	}
	report.EndedAt = now().UTC()
	report.Summary = summarize(report.Cases)
	if err := ctx.Err(); err != nil {
		return report, err
	}
	return report, nil
}

func (r *Runner) runCase(suiteContext context.Context, evalCase Case, evaluators Evaluators) CaseResult {
	totalStart := time.Now()
	attemptID := r.newAttemptID()
	observation := Observation{
		CaseID:    evalCase.ID,
		AttemptID: attemptID,
		Status:    RunStatusError,
		Capabilities: Capabilities{
			ToolCapture: CoverageUnavailable,
			Usage:       CoverageUnavailable,
			Output:      CoverageUnavailable,
		},
		Regradable: true,
	}
	caseResult := CaseResult{CaseID: evalCase.ID, AttemptID: attemptID, Observation: observation}

	caseContext := suiteContext
	cancelCase := func() {}
	if r.Options.CaseTimeout > 0 {
		caseContext, cancelCase = context.WithTimeout(suiteContext, r.Options.CaseTimeout)
	}
	defer cancelCase()

	recorder := NewRecorder(r.Options.Recorder)
	setupStart := time.Now()
	target, setupErr := callFactory(caseContext, r.Factory, evalCase, recorder)
	caseResult.Durations.SetupMS = elapsedMilliseconds(setupStart)
	if setupErr != nil {
		observation.Error = pointerErrorDetail(errorDetail("setup", setupErr))
		if errors.Is(setupErr, context.Canceled) || errors.Is(setupErr, context.DeadlineExceeded) {
			observation.Status = RunStatusCancelled
		}
		observation.Trace = recorder.Snapshot()
		caseResult = r.closeAndGrade(suiteContext, evalCase, target, observation, evaluators, caseResult.Durations)
		caseResult.Durations.TotalMS = elapsedMilliseconds(totalStart)
		return caseResult
	}
	if target.Subject == nil {
		setupErr = errors.New("target factory returned a nil subject")
		observation.Error = pointerErrorDetail(errorDetail("setup", setupErr))
		observation.Trace = recorder.Snapshot()
		caseResult = r.closeAndGrade(suiteContext, evalCase, target, observation, evaluators, caseResult.Durations)
		caseResult.Durations.TotalMS = elapsedMilliseconds(totalStart)
		return caseResult
	}
	observation.Capabilities = normalizeCapabilities(target.Capabilities)

	executionStart := time.Now()
	response, executionErr := callSubject(caseContext, target.Subject, evalCase.Input)
	observation.ExecutionDuration = elapsedMilliseconds(executionStart)
	caseResult.Durations.ExecutionMS = observation.ExecutionDuration
	observation.Trace = recorder.Snapshot()
	if executionErr != nil {
		observation.Status = RunStatusError
		if errors.Is(executionErr, context.Canceled) || errors.Is(executionErr, context.DeadlineExceeded) {
			observation.Status = RunStatusCancelled
		}
		observation.Error = pointerErrorDetail(errorDetail("execution", executionErr))
	} else if response == nil {
		observation.Status = RunStatusError
		observation.Error = pointerErrorDetail(ErrorDetail{
			Stage:   "execution",
			Type:    "nil_response",
			Message: "subject returned a nil response without an error",
		})
	} else {
		observation.Status = RunStatusCompleted
		output, truncated := retainOutput(response.Content, r.Options.Recorder.MaxContentBytes)
		observation.Output = &output
		observation.Capabilities.Output = CoverageComplete
		if truncated {
			observation.Capabilities.Output = CoveragePartial
			observation.Regradable = false
		}
		observation.AgentName = response.AgentName
		observation.Model = response.Model
		observation.Usage = cloneUsage(response.Usage)
		observation.ExecutionSummary = cloneExecutionSummary(response.ExecutionSummary)
		if response.Usage == nil && observation.Capabilities.Usage == CoverageComplete {
			observation.Capabilities.Usage = CoverageUnavailable
		}
	}

	caseResult = r.closeAndGrade(suiteContext, evalCase, target, observation, evaluators, caseResult.Durations)
	caseResult.Durations.TotalMS = elapsedMilliseconds(totalStart)
	return caseResult
}

func (r *Runner) closeAndGrade(
	suiteContext context.Context,
	evalCase Case,
	target Target,
	observation Observation,
	evaluators Evaluators,
	durations Durations,
) CaseResult {
	var cleanupError *ErrorDetail
	if target.Close != nil {
		cleanupStart := time.Now()
		cleanupContext := context.Background()
		cancelCleanup := func() {}
		if r.Options.CleanupTimeout > 0 {
			cleanupContext, cancelCleanup = context.WithTimeout(cleanupContext, r.Options.CleanupTimeout)
		}
		err := callClose(cleanupContext, target.Close)
		cancelCleanup()
		durations.CleanupMS = elapsedMilliseconds(cleanupStart)
		if err != nil {
			detail := errorDetail("cleanup", err)
			cleanupError = &detail
		}
	}
	result := r.gradeCase(suiteContext, evalCase, observation, evaluators, durations)
	if cleanupError != nil {
		result.Errors = append(result.Errors, *cleanupError)
		result.Status = CaseStatusError
	}
	return result
}

func (r *Runner) gradeCase(
	suiteContext context.Context,
	evalCase Case,
	observation Observation,
	evaluators Evaluators,
	durations Durations,
) CaseResult {
	gradingContext := suiteContext
	cancelGrading := func() {}
	if r.Options.GradingTimeout > 0 {
		gradingContext, cancelGrading = context.WithTimeout(suiteContext, r.Options.GradingTimeout)
	}
	gradingStart := time.Now()
	result, err := Grade(gradingContext, evalCase, observation, evaluators)
	cancelGrading()
	durations.GradingMS = elapsedMilliseconds(gradingStart)
	result.Durations = durations
	if err != nil {
		result.Status = CaseStatusError
		if len(result.Errors) == 0 || result.Errors[len(result.Errors)-1].Message != err.Error() {
			result.Errors = append(result.Errors, errorDetail("grading", err))
		}
	}
	return result
}

func callFactory(ctx context.Context, factory TargetFactory, evalCase Case, recorder *Recorder) (target Target, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("target factory panic: %v", recovered)
		}
	}()
	return factory(ctx, evalCase, recorder)
}

func callSubject(ctx context.Context, subject Subject, input string) (response *interfaces.AgentResponse, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("subject panic: %v", recovered)
		}
	}()
	return subject.RunDetailed(ctx, input)
}

func callClose(ctx context.Context, closeTarget func(context.Context) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("target cleanup panic: %v", recovered)
		}
	}()
	return closeTarget(ctx)
}

func mergedEvaluators(custom Evaluators) Evaluators {
	merged := BuiltinEvaluators()
	for name, evaluator := range custom {
		merged[name] = evaluator
	}
	return merged
}

func normalizeCapabilities(capabilities Capabilities) Capabilities {
	if !validCoverage(capabilities.ToolCapture) {
		capabilities.ToolCapture = CoverageUnavailable
	}
	if !validCoverage(capabilities.Usage) {
		capabilities.Usage = CoverageUnavailable
	}
	if !validCoverage(capabilities.Output) {
		capabilities.Output = CoverageUnavailable
	}
	return capabilities
}

func validCoverage(coverage Coverage) bool {
	return coverage == CoverageComplete || coverage == CoveragePartial || coverage == CoverageUnavailable
}

func cloneDataset(dataset Dataset) Dataset {
	clone := dataset
	clone.Cases = append([]Case(nil), dataset.Cases...)
	for i := range dataset.Cases {
		clone.Cases[i].Tags = append([]string(nil), dataset.Cases[i].Tags...)
		clone.Cases[i].Checks = append([]Check(nil), dataset.Cases[i].Checks...)
		for j := range dataset.Cases[i].Checks {
			clone.Cases[i].Checks[j].Config = append([]byte(nil), dataset.Cases[i].Checks[j].Config...)
			if dataset.Cases[i].Checks[j].Threshold != nil {
				value := *dataset.Cases[i].Checks[j].Threshold
				clone.Cases[i].Checks[j].Threshold = &value
			}
			if dataset.Cases[i].Checks[j].Required != nil {
				value := *dataset.Cases[i].Checks[j].Required
				clone.Cases[i].Checks[j].Required = &value
			}
		}
		if dataset.Cases[i].Reference != nil {
			value := *dataset.Cases[i].Reference
			clone.Cases[i].Reference = &value
		}
	}
	return clone
}

func cloneUsage(usage *interfaces.TokenUsage) *TokenUsage {
	if usage == nil {
		return nil
	}
	return &TokenUsage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		TotalTokens:              usage.TotalTokens,
		ReasoningTokens:          usage.ReasoningTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
	}
}

func cloneExecutionSummary(summary interfaces.ExecutionSummary) ExecutionSummary {
	clone := ExecutionSummary{
		LLMCalls:        summary.LLMCalls,
		ToolCalls:       summary.ToolCalls,
		SubAgentCalls:   summary.SubAgentCalls,
		ExecutionTimeMS: summary.ExecutionTimeMs,
		UsedTools:       append([]string(nil), summary.UsedTools...),
		UsedSubAgents:   append([]string(nil), summary.UsedSubAgents...),
	}
	if summary.UsageByModel != nil {
		clone.UsageByModel = make(map[string]TokenUsage, len(summary.UsageByModel))
		for model, usage := range summary.UsageByModel {
			clone.UsageByModel[model] = *cloneUsage(&usage)
		}
	}
	return clone
}

func notStartedResult(caseID string) CaseResult {
	return CaseResult{
		CaseID: caseID,
		Status: CaseStatusNotStarted,
		Observation: Observation{
			CaseID: caseID,
			Status: RunStatusNotStarted,
			Capabilities: Capabilities{
				ToolCapture: CoverageUnavailable,
				Usage:       CoverageUnavailable,
				Output:      CoverageUnavailable,
			},
		},
	}
}

func errorDetail(stage string, err error) ErrorDetail {
	detail := ErrorDetail{Stage: stage, Message: err.Error()}
	if typed := reflect.TypeOf(err); typed != nil {
		detail.Type = typed.String()
	}
	return detail
}

func pointerErrorDetail(detail ErrorDetail) *ErrorDetail { return &detail }

func elapsedMilliseconds(start time.Time) int64 {
	duration := time.Since(start)
	if duration <= 0 {
		return 0
	}
	return duration.Milliseconds()
}

func retainOutput(value string, maximum int) (string, bool) {
	if maximum <= 0 {
		maximum = DefaultMaxContentBytes
	}
	if len(value) <= maximum {
		return value, false
	}
	retained := []byte(value[:maximum])
	for len(retained) > 0 && !utf8.Valid(retained) {
		retained = retained[:len(retained)-1]
	}
	return string(retained), true
}

var fallbackAttemptID atomic.Uint64

func (r *Runner) newAttemptID() string {
	if r.Options.AttemptID != nil {
		return r.Options.AttemptID()
	}
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	return fmt.Sprintf("attempt-%d", fallbackAttemptID.Add(1))
}
