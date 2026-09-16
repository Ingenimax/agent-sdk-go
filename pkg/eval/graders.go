package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

const (
	EvaluatorOutputExact      = "output_exact"
	EvaluatorOutputContains   = "output_contains"
	EvaluatorOutputRegex      = "output_regex"
	EvaluatorOutputJSONSchema = "output_json_schema"
	EvaluatorToolTrajectory   = "tool_trajectory"
	EvaluatorMaxToolCalls     = "max_tool_calls"
	EvaluatorMaxLatencyMS     = "max_latency_ms"
	EvaluatorMaxTotalTokens   = "max_total_tokens"
)

// BuiltinEvaluators returns a fresh registry of deterministic evaluators.
func BuiltinEvaluators() Evaluators {
	return Evaluators{
		EvaluatorOutputExact:      textEvaluator{name: EvaluatorOutputExact},
		EvaluatorOutputContains:   textEvaluator{name: EvaluatorOutputContains},
		EvaluatorOutputRegex:      regexEvaluator{},
		EvaluatorOutputJSONSchema: jsonSchemaEvaluator{},
		EvaluatorToolTrajectory:   trajectoryEvaluator{},
		EvaluatorMaxToolCalls:     maxToolCallsEvaluator{},
		EvaluatorMaxLatencyMS:     maxLatencyEvaluator{},
		EvaluatorMaxTotalTokens:   maxTokensEvaluator{},
	}
}

type textConfig struct {
	Value         string `json:"value"`
	Normalization string `json:"normalization,omitempty"`
}

type textEvaluator struct{ name string }

func (e textEvaluator) Name() string { return e.name }

func (e textEvaluator) Validate(check Check) error {
	var config textConfig
	if err := decodeConfig(check.Config, &config); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	return validateNormalization(config.Normalization)
}

func (e textEvaluator) Evaluate(_ context.Context, in Evaluation) (MetricResult, error) {
	var config textConfig
	if err := decodeConfig(in.Check.Config, &config); err != nil {
		return MetricResult{}, err
	}
	result := metricBase(in.Check, e.Name())
	if unavailable := requireOutput(result, in.Observation); unavailable != nil {
		return *unavailable, nil
	}
	actual := normalizeText(*in.Observation.Output, config.Normalization)
	expected := normalizeText(config.Value, config.Normalization)
	matched := actual == expected
	if e.name == EvaluatorOutputContains {
		matched = strings.Contains(actual, expected)
	}
	result.Evidence = map[string]any{
		"normalization":  resolvedNormalization(config.Normalization),
		"expected_bytes": len(expected),
		"actual_bytes":   len(actual),
	}
	return binaryMetric(result, matched, "output matched", "output did not match"), nil
}

type regexConfig struct {
	Pattern       string `json:"pattern"`
	Normalization string `json:"normalization,omitempty"`
}

type regexEvaluator struct{}

func (regexEvaluator) Name() string { return EvaluatorOutputRegex }

func (regexEvaluator) Validate(check Check) error {
	var config regexConfig
	if err := decodeConfig(check.Config, &config); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if config.Pattern == "" {
		return errors.New("pattern is required")
	}
	if err := validateNormalization(config.Normalization); err != nil {
		return err
	}
	if _, err := regexp.Compile(config.Pattern); err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}
	return nil
}

func (regexEvaluator) Evaluate(_ context.Context, in Evaluation) (MetricResult, error) {
	var config regexConfig
	if err := decodeConfig(in.Check.Config, &config); err != nil {
		return MetricResult{}, err
	}
	pattern, err := regexp.Compile(config.Pattern)
	if err != nil {
		return MetricResult{}, err
	}
	result := metricBase(in.Check, EvaluatorOutputRegex)
	if unavailable := requireOutput(result, in.Observation); unavailable != nil {
		return *unavailable, nil
	}
	actual := normalizeText(*in.Observation.Output, config.Normalization)
	result.Evidence = map[string]any{
		"pattern":       config.Pattern,
		"normalization": resolvedNormalization(config.Normalization),
		"actual_bytes":  len(actual),
	}
	return binaryMetric(result, pattern.MatchString(actual), "output matched pattern", "output did not match pattern"), nil
}

type jsonSchemaConfig struct {
	Schema json.RawMessage `json:"schema"`
}

type jsonSchemaEvaluator struct{}

func (jsonSchemaEvaluator) Name() string { return EvaluatorOutputJSONSchema }

func (jsonSchemaEvaluator) Validate(check Check) error {
	_, err := resolveCheckSchema(check)
	return err
}

func (jsonSchemaEvaluator) Evaluate(_ context.Context, in Evaluation) (MetricResult, error) {
	resolved, err := resolveCheckSchema(in.Check)
	if err != nil {
		return MetricResult{}, err
	}
	result := metricBase(in.Check, EvaluatorOutputJSONSchema)
	if unavailable := requireOutput(result, in.Observation); unavailable != nil {
		return *unavailable, nil
	}
	instance, err := decodeOneJSON([]byte(*in.Observation.Output), false)
	if err != nil {
		result.Evidence = map[string]any{"parse_error": err.Error()}
		return binaryMetric(result, false, "", "output is not one valid JSON value"), nil
	}
	if err := resolved.Validate(instance); err != nil {
		result.Evidence = map[string]any{"schema_valid": false}
		return binaryMetric(result, false, "", "output did not satisfy JSON Schema"), nil
	}
	return binaryMetric(result, true, "output satisfied JSON Schema", ""), nil
}

func resolveCheckSchema(check Check) (*jsonschema.Resolved, error) {
	var config jsonSchemaConfig
	if err := decodeConfig(check.Config, &config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if len(bytes.TrimSpace(config.Schema)) == 0 || bytes.Equal(bytes.TrimSpace(config.Schema), []byte("null")) {
		return nil, errors.New("schema must be a JSON object")
	}
	if err := rejectDuplicateJSONKeys(config.Schema); err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(config.Schema, &schema); err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}
	resolved, err := schema.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		return nil, fmt.Errorf("invalid or externally referenced schema: %w", err)
	}
	return resolved, nil
}

type expectedToolCall struct {
	Name         string          `json:"name"`
	Arguments    json.RawMessage `json:"arguments,omitempty"`
	ArgumentsRaw *string         `json:"arguments_raw,omitempty"`
}

type trajectoryConfig struct {
	Layer         ToolLayer          `json:"layer,omitempty"`
	AgentPath     string             `json:"agent_path,omitempty"`
	Mode          string             `json:"mode,omitempty"`
	ArgumentMatch string             `json:"argument_match,omitempty"`
	Calls         []expectedToolCall `json:"calls"`
}

type trajectoryEvaluator struct{}

func (trajectoryEvaluator) Name() string { return EvaluatorToolTrajectory }

func (trajectoryEvaluator) Validate(check Check) error {
	config, err := decodeTrajectoryConfig(check)
	if err != nil {
		return err
	}
	for i, call := range config.Calls {
		if call.Name == "" {
			return fmt.Errorf("calls[%d].name is required", i)
		}
		if len(call.Arguments) > 0 && call.ArgumentsRaw != nil {
			return fmt.Errorf("calls[%d] cannot set both arguments and arguments_raw", i)
		}
		if config.ArgumentMatch == "raw" {
			if len(call.Arguments) > 0 {
				return fmt.Errorf("calls[%d].arguments is not valid with raw argument matching", i)
			}
			continue
		}
		if call.ArgumentsRaw != nil {
			return fmt.Errorf("calls[%d].arguments_raw requires raw argument matching", i)
		}
		if len(call.Arguments) > 0 {
			if _, err := decodeOneJSON(call.Arguments, true); err != nil {
				return fmt.Errorf("calls[%d].arguments: %w", i, err)
			}
		}
	}
	return nil
}

func (trajectoryEvaluator) Evaluate(_ context.Context, in Evaluation) (MetricResult, error) {
	config, err := decodeTrajectoryConfig(in.Check)
	if err != nil {
		return MetricResult{}, err
	}
	result := metricBase(in.Check, EvaluatorToolTrajectory)
	if unavailable := requireToolTrace(result, in.Observation); unavailable != nil {
		return *unavailable, nil
	}
	observed := filterSpans(in.Observation.Trace, config.Layer, config.AgentPath)
	matched := matchTrajectory(config, observed)
	names := make([]string, len(observed))
	for i := range observed {
		names[i] = observed[i].Tool
	}
	result.Evidence = map[string]any{
		"layer":          config.Layer,
		"agent_path":     config.AgentPath,
		"mode":           config.Mode,
		"expected_calls": len(config.Calls),
		"observed_calls": names,
	}
	return binaryMetric(result, matched, "tool trajectory matched", "tool trajectory did not match"), nil
}

func decodeTrajectoryConfig(check Check) (trajectoryConfig, error) {
	var config trajectoryConfig
	if err := decodeConfig(check.Config, &config); err != nil {
		return config, fmt.Errorf("invalid config: %w", err)
	}
	if config.Layer == "" {
		config.Layer = ToolLayerExecution
	}
	if config.AgentPath == "" {
		config.AgentPath = RootAgentPath
	}
	if config.Mode == "" {
		config.Mode = "exact"
	}
	if config.ArgumentMatch == "" {
		config.ArgumentMatch = "full"
	}
	if config.Layer != ToolLayerAttempt && config.Layer != ToolLayerExecution {
		return config, fmt.Errorf("layer must be %q or %q", ToolLayerAttempt, ToolLayerExecution)
	}
	if config.Mode != "exact" && config.Mode != "subsequence" && config.Mode != "unordered" {
		return config, errors.New("mode must be exact, subsequence, or unordered")
	}
	if config.ArgumentMatch != "full" && config.ArgumentMatch != "subset" && config.ArgumentMatch != "raw" {
		return config, errors.New("argument_match must be full, subset, or raw")
	}
	return config, nil
}

type scopedMaximumConfig struct {
	Layer     ToolLayer `json:"layer,omitempty"`
	AgentPath string    `json:"agent_path,omitempty"`
	Maximum   *int      `json:"maximum"`
}

type maxToolCallsEvaluator struct{}

func (maxToolCallsEvaluator) Name() string { return EvaluatorMaxToolCalls }

func (maxToolCallsEvaluator) Validate(check Check) error {
	_, err := decodeScopedMaximum(check)
	return err
}

func (maxToolCallsEvaluator) Evaluate(_ context.Context, in Evaluation) (MetricResult, error) {
	config, err := decodeScopedMaximum(in.Check)
	if err != nil {
		return MetricResult{}, err
	}
	result := metricBase(in.Check, EvaluatorMaxToolCalls)
	if unavailable := requireToolTrace(result, in.Observation); unavailable != nil {
		return *unavailable, nil
	}
	count := len(filterSpans(in.Observation.Trace, config.Layer, config.AgentPath))
	result.Evidence = map[string]any{
		"layer":      config.Layer,
		"agent_path": config.AgentPath,
		"maximum":    *config.Maximum,
		"observed":   count,
	}
	return binaryMetric(result, count <= *config.Maximum, "tool call count was within limit", "tool call count exceeded limit"), nil
}

func decodeScopedMaximum(check Check) (scopedMaximumConfig, error) {
	var config scopedMaximumConfig
	if err := decodeConfig(check.Config, &config); err != nil {
		return config, fmt.Errorf("invalid config: %w", err)
	}
	if config.Layer == "" {
		config.Layer = ToolLayerExecution
	}
	if config.AgentPath == "" {
		config.AgentPath = RootAgentPath
	}
	if config.Layer != ToolLayerAttempt && config.Layer != ToolLayerExecution {
		return config, fmt.Errorf("layer must be %q or %q", ToolLayerAttempt, ToolLayerExecution)
	}
	if config.Maximum == nil {
		return config, errors.New("maximum is required")
	}
	if *config.Maximum < 0 {
		return config, errors.New("maximum cannot be negative")
	}
	return config, nil
}

type maximumConfig struct {
	Maximum *int64 `json:"maximum"`
}

type maxLatencyEvaluator struct{}

func (maxLatencyEvaluator) Name() string { return EvaluatorMaxLatencyMS }

func (maxLatencyEvaluator) Validate(check Check) error {
	_, err := decodeMaximum(check)
	return err
}

func (maxLatencyEvaluator) Evaluate(_ context.Context, in Evaluation) (MetricResult, error) {
	config, err := decodeMaximum(in.Check)
	if err != nil {
		return MetricResult{}, err
	}
	result := metricBase(in.Check, EvaluatorMaxLatencyMS)
	if in.Observation.Status == RunStatusNotStarted {
		return unavailableMetric(result, "execution did not start"), nil
	}
	result.Evidence = map[string]any{
		"maximum_ms":  *config.Maximum,
		"observed_ms": in.Observation.ExecutionDuration,
	}
	return binaryMetric(result, in.Observation.ExecutionDuration <= *config.Maximum, "execution latency was within limit", "execution latency exceeded limit"), nil
}

type maxTokensEvaluator struct{}

func (maxTokensEvaluator) Name() string { return EvaluatorMaxTotalTokens }

func (maxTokensEvaluator) Validate(check Check) error {
	_, err := decodeMaximum(check)
	return err
}

func (maxTokensEvaluator) Evaluate(_ context.Context, in Evaluation) (MetricResult, error) {
	config, err := decodeMaximum(in.Check)
	if err != nil {
		return MetricResult{}, err
	}
	result := metricBase(in.Check, EvaluatorMaxTotalTokens)
	if in.Observation.Capabilities.Usage != CoverageComplete || in.Observation.Usage == nil {
		return unavailableMetric(result, "complete token usage is unavailable"), nil
	}
	observed := int64(in.Observation.Usage.TotalTokens)
	result.Evidence = map[string]any{
		"maximum":  *config.Maximum,
		"observed": observed,
	}
	return binaryMetric(result, observed <= *config.Maximum, "token usage was within limit", "token usage exceeded limit"), nil
}

func decodeMaximum(check Check) (maximumConfig, error) {
	var config maximumConfig
	if err := decodeConfig(check.Config, &config); err != nil {
		return config, fmt.Errorf("invalid config: %w", err)
	}
	if config.Maximum == nil {
		return config, errors.New("maximum is required")
	}
	if *config.Maximum < 0 {
		return config, errors.New("maximum cannot be negative")
	}
	return config, nil
}

func metricBase(check Check, evaluator string) MetricResult {
	return MetricResult{
		CheckID:   check.ID,
		Evaluator: evaluator,
		Threshold: check.ThresholdValue(),
		Required:  check.RequiredValue(),
	}
}

func binaryMetric(result MetricResult, matched bool, passMessage, failMessage string) MetricResult {
	value := 0.0
	result.Status = MetricStatusFail
	result.Message = failMessage
	if matched {
		value = 1
		result.Message = passMessage
	}
	result.Score = score(value)
	if value >= result.Threshold {
		result.Status = MetricStatusPass
	}
	return result
}

func unavailableMetric(result MetricResult, message string) MetricResult {
	result.Status = MetricStatusUnavailable
	result.Message = message
	result.Score = nil
	return result
}

func validateNormalization(normalization string) error {
	if normalization != "" && normalization != "none" && normalization != "trim_space" {
		return errors.New("normalization must be none or trim_space")
	}
	return nil
}

func resolvedNormalization(normalization string) string {
	if normalization == "" {
		return "none"
	}
	return normalization
}

func normalizeText(value, normalization string) string {
	if normalization == "trim_space" {
		return strings.TrimSpace(value)
	}
	return value
}

func requireToolTrace(base MetricResult, observation Observation) *MetricResult {
	if observation.Capabilities.ToolCapture != CoverageComplete {
		result := unavailableMetric(base, "complete tool capture is unavailable")
		return &result
	}
	if observation.Trace.Incomplete {
		result := unavailableMetric(base, "tool trace is incomplete")
		return &result
	}
	return nil
}

func requireOutput(base MetricResult, observation Observation) *MetricResult {
	if observation.Capabilities.Output != CoverageComplete || observation.Output == nil {
		result := unavailableMetric(base, "complete agent output is unavailable")
		return &result
	}
	return nil
}

func filterSpans(trace Trace, layer ToolLayer, agentPath string) []ToolSpan {
	spans := make([]ToolSpan, 0)
	for _, span := range trace.Spans {
		if span.Layer == layer && span.AgentPath == agentPath {
			spans = append(spans, span)
		}
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Sequence < spans[j].Sequence })
	return spans
}

func matchTrajectory(config trajectoryConfig, observed []ToolSpan) bool {
	switch config.Mode {
	case "exact":
		if len(config.Calls) != len(observed) {
			return false
		}
		for i := range config.Calls {
			if !matchToolCall(config.Calls[i], observed[i], config.ArgumentMatch) {
				return false
			}
		}
		return true
	case "subsequence":
		next := 0
		for _, span := range observed {
			if next < len(config.Calls) && matchToolCall(config.Calls[next], span, config.ArgumentMatch) {
				next++
			}
		}
		return next == len(config.Calls)
	case "unordered":
		if len(config.Calls) != len(observed) {
			return false
		}
		matches := make([]int, len(observed))
		for i := range matches {
			matches[i] = -1
		}
		for expectedIndex := range config.Calls {
			seen := make([]bool, len(observed))
			if !augmentToolMatch(expectedIndex, config, observed, seen, matches) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func augmentToolMatch(expectedIndex int, config trajectoryConfig, observed []ToolSpan, seen []bool, matches []int) bool {
	for observedIndex := range observed {
		if seen[observedIndex] || !matchToolCall(config.Calls[expectedIndex], observed[observedIndex], config.ArgumentMatch) {
			continue
		}
		seen[observedIndex] = true
		if matches[observedIndex] == -1 || augmentToolMatch(matches[observedIndex], config, observed, seen, matches) {
			matches[observedIndex] = expectedIndex
			return true
		}
	}
	return false
}

func matchToolCall(expected expectedToolCall, observed ToolSpan, argumentMatch string) bool {
	if expected.Name != observed.Tool {
		return false
	}
	if argumentMatch == "raw" {
		return expected.ArgumentsRaw == nil || *expected.ArgumentsRaw == observed.Arguments
	}
	if len(expected.Arguments) == 0 {
		return true
	}
	expectedValue, err := decodeOneJSON(expected.Arguments, true)
	if err != nil {
		return false
	}
	observedValue, err := decodeOneJSON([]byte(observed.Arguments), true)
	if err != nil {
		return false
	}
	return jsonValueMatches(expectedValue, observedValue, argumentMatch == "subset")
}

func decodeOneJSON(data []byte, useNumber bool) (any, error) {
	if !json.Valid(data) {
		return nil, errors.New("invalid JSON")
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if useNumber {
		decoder.UseNumber()
	}
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	return value, nil
}

func jsonValueMatches(expected, actual any, subset bool) bool {
	switch expectedValue := expected.(type) {
	case json.Number:
		actualValue, ok := actual.(json.Number)
		if !ok {
			return false
		}
		expectedNumber, expectedErr := canonicalizeJSONNumber(expectedValue.String())
		actualNumber, actualErr := canonicalizeJSONNumber(actualValue.String())
		return expectedErr == nil && actualErr == nil && expectedNumber == actualNumber
	case map[string]any:
		actualValue, ok := actual.(map[string]any)
		if !ok || (!subset && len(expectedValue) != len(actualValue)) {
			return false
		}
		for key, expectedChild := range expectedValue {
			actualChild, exists := actualValue[key]
			if !exists || !jsonValueMatches(expectedChild, actualChild, subset) {
				return false
			}
		}
		return true
	case []any:
		actualValue, ok := actual.([]any)
		if !ok || len(expectedValue) != len(actualValue) {
			return false
		}
		for i := range expectedValue {
			// Arrays remain exact even when object subset matching is enabled.
			if !jsonValueMatches(expectedValue[i], actualValue[i], subset) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(expected, actual)
	}
}
