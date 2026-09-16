package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ExportOptions controls how much execution content is written to JSON.
// Observations are omitted by default. Any redaction or truncation makes the
// affected observation ineligible for offline regrading.
type ExportOptions struct {
	IncludeObservations bool
	MaxContentBytes     int
	Redact              func(field, value string) string
}

// WriteJSON writes a versioned report. It never mutates the in-memory report.
func WriteJSON(w io.Writer, report Report, options ExportOptions) error {
	if w == nil {
		return errors.New("eval: report writer is nil")
	}
	if report.SchemaVersion != ReportSchemaVersion {
		return fmt.Errorf("eval: unsupported report schema_version %q", report.SchemaVersion)
	}
	prepared, err := prepareReportForExport(report, options)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(prepared); err != nil {
		return fmt.Errorf("eval: write JSON report: %w", err)
	}
	return nil
}

// ReadReport reads one strict JSON report document.
func ReadReport(r io.Reader) (Report, error) {
	if r == nil {
		return Report{}, errors.New("eval: report reader is nil")
	}
	data, err := io.ReadAll(io.LimitReader(r, DefaultMaxDatasetBytes*10+1))
	if err != nil {
		return Report{}, fmt.Errorf("eval: read report: %w", err)
	}
	if int64(len(data)) > DefaultMaxDatasetBytes*10 {
		return Report{}, errors.New("eval: report exceeds maximum size")
	}
	if !utf8.Valid(data) {
		return Report{}, errors.New("eval: report is not valid UTF-8")
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return Report{}, fmt.Errorf("eval: invalid report JSON: %w", err)
	}
	var report Report
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&report); err != nil {
		return Report{}, fmt.Errorf("eval: decode report: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Report{}, fmt.Errorf("eval: decode report: %w", err)
	}
	if report.SchemaVersion != ReportSchemaVersion {
		return Report{}, fmt.Errorf("eval: unsupported report schema_version %q", report.SchemaVersion)
	}
	return report, nil
}

// WriteJSONFile writes a report through a temporary file and atomically renames
// it into place on filesystems that support atomic rename.
func WriteJSONFile(path string, report Report, options ExportOptions) error {
	return writeFileAtomically(path, func(w io.Writer) error {
		return WriteJSON(w, report, options)
	})
}

// Regrade applies evaluators to complete saved observations without executing
// an agent. Dataset identity is checked before any grading occurs.
func Regrade(ctx context.Context, dataset Dataset, previous Report, evaluators Evaluators) (Report, error) {
	if previous.SchemaVersion != ReportSchemaVersion {
		return Report{}, fmt.Errorf("eval: unsupported report schema_version %q", previous.SchemaVersion)
	}
	resolved := cloneDataset(dataset)
	registry := mergedEvaluators(evaluators)
	if err := ValidateDataset(&resolved, registry); err != nil {
		return Report{}, err
	}
	digest, err := DatasetDigest(resolved)
	if err != nil {
		return Report{}, err
	}
	if previous.DatasetID != resolved.ID || previous.DatasetDigest != digest {
		return Report{}, errors.New("eval: report dataset identity does not match the supplied dataset")
	}

	byCase := make(map[string]CaseResult, len(previous.Cases))
	for _, result := range previous.Cases {
		if _, duplicate := byCase[result.CaseID]; duplicate {
			return Report{}, fmt.Errorf("eval: report contains duplicate case %q", result.CaseID)
		}
		byCase[result.CaseID] = result
	}
	if len(byCase) != len(resolved.Cases) {
		return Report{}, errors.New("eval: report case set does not match the supplied dataset")
	}
	startedAt := time.Now().UTC()
	regarded := Report{
		SchemaVersion:     ReportSchemaVersion,
		DatasetID:         resolved.ID,
		DatasetDigest:     digest,
		ConfigFingerprint: previous.ConfigFingerprint,
		BuildRevision:     previous.BuildRevision,
		StartedAt:         startedAt,
		ResolvedDataset:   &resolved,
		Cases:             make([]CaseResult, 0, len(resolved.Cases)),
	}
	for _, evalCase := range resolved.Cases {
		old, ok := byCase[evalCase.ID]
		if !ok {
			return Report{}, fmt.Errorf("eval: report has no observation for case %q", evalCase.ID)
		}
		if old.Observation.CaseID != evalCase.ID {
			return Report{}, fmt.Errorf("eval: observation identity for case %q does not match", evalCase.ID)
		}
		if !old.Observation.Regradable {
			return Report{}, fmt.Errorf("eval: observation for case %q is incomplete or redacted and cannot be regraded", evalCase.ID)
		}
		gradingStart := time.Now()
		result, err := Grade(ctx, evalCase, old.Observation, registry)
		result.Durations = old.Durations
		result.Durations.GradingMS = elapsedMilliseconds(gradingStart)
		if err != nil {
			return Report{}, fmt.Errorf("eval: regrade case %q: %w", evalCase.ID, err)
		}
		for _, detail := range old.Errors {
			if detail.Stage == "cleanup" {
				result.Errors = append(result.Errors, detail)
				result.Status = CaseStatusError
			}
		}
		regarded.Cases = append(regarded.Cases, result)
	}
	regarded.EndedAt = time.Now().UTC()
	regarded.Summary = summarize(regarded.Cases)
	return regarded, nil
}

type junitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name      string      `xml:"name,attr"`
	Tests     int         `xml:"tests,attr"`
	Failures  int         `xml:"failures,attr"`
	Errors    int         `xml:"errors,attr"`
	Skipped   int         `xml:"skipped,attr"`
	Time      string      `xml:"time,attr"`
	TestCases []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitMessage `xml:"failure,omitempty"`
	Error     *junitMessage `xml:"error,omitempty"`
	Skipped   *junitMessage `xml:"skipped,omitempty"`
}

type junitMessage struct {
	Message string `xml:"message,attr,omitempty"`
	Body    string `xml:",chardata"`
}

// WriteJUnit writes one testcase per evaluation case.
func WriteJUnit(w io.Writer, report Report) error {
	if w == nil {
		return errors.New("eval: JUnit writer is nil")
	}
	suite := junitSuite{
		Name:  xmlSafe(report.DatasetID),
		Tests: len(report.Cases),
	}
	var totalMilliseconds int64
	for _, result := range report.Cases {
		totalMilliseconds += result.Durations.TotalMS
		entry := junitCase{
			Name:      xmlSafe(result.CaseID),
			Classname: xmlSafe(report.DatasetID),
			Time:      secondsString(result.Durations.TotalMS),
		}
		switch result.Status {
		case CaseStatusFail:
			suite.Failures++
			entry.Failure = &junitMessage{Message: "evaluation expectations failed", Body: xmlSafe(metricMessages(result, MetricStatusFail))}
		case CaseStatusError:
			suite.Errors++
			entry.Error = &junitMessage{Message: "evaluation could not complete", Body: xmlSafe(caseErrorMessages(result))}
		case CaseStatusNotStarted:
			suite.Skipped++
			entry.Skipped = &junitMessage{Message: "case was not started"}
		}
		suite.TestCases = append(suite.TestCases, entry)
	}
	suite.Time = secondsString(totalMilliseconds)
	document := junitSuites{Suites: []junitSuite{suite}}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return fmt.Errorf("eval: write JUnit header: %w", err)
	}
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("eval: write JUnit report: %w", err)
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return fmt.Errorf("eval: finish JUnit report: %w", err)
	}
	return nil
}

// WriteJUnitFile writes a JUnit report atomically.
func WriteJUnitFile(path string, report Report) error {
	return writeFileAtomically(path, func(w io.Writer) error { return WriteJUnit(w, report) })
}

func prepareReportForExport(report Report, options ExportOptions) (Report, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return Report{}, fmt.Errorf("eval: clone report for export: %w", err)
	}
	var clone Report
	if err := json.Unmarshal(data, &clone); err != nil {
		return Report{}, fmt.Errorf("eval: clone report for export: %w", err)
	}
	budget := options.MaxContentBytes
	if budget <= 0 {
		budget = DefaultMaxContentBytes
	}
	used := 0
	var redactionErr error
	retain := func(field, value string) (string, bool) {
		if options.Redact != nil {
			var err error
			value, err = callRedactor(options.Redact, field, value)
			if err != nil {
				redactionErr = err
				return "", true
			}
		}
		remaining := budget - used
		if remaining <= 0 {
			return "", value != ""
		}
		if len(value) <= remaining {
			used += len(value)
			return value, false
		}
		bytes := []byte(value[:remaining])
		for len(bytes) > 0 && !utf8.Valid(bytes) {
			bytes = bytes[:len(bytes)-1]
		}
		used += len(bytes)
		return string(bytes), true
	}
	var retainValue func(string, any) (any, bool)
	retainValue = func(field string, value any) (any, bool) {
		switch typed := value.(type) {
		case string:
			return retain(field, typed)
		case map[string]any:
			truncated := false
			for key, child := range typed {
				retained, childTruncated := retainValue(field+"."+key, child)
				typed[key] = retained
				truncated = truncated || childTruncated
			}
			return typed, truncated
		case []any:
			truncated := false
			for index, child := range typed {
				retained, childTruncated := retainValue(fmt.Sprintf("%s[%d]", field, index), child)
				typed[index] = retained
				truncated = truncated || childTruncated
			}
			return typed, truncated
		default:
			return value, false
		}
	}

	if !options.IncludeObservations || options.Redact != nil {
		clone.ResolvedDataset = nil
	}
	for caseIndex := range clone.Cases {
		observation := &clone.Cases[caseIndex].Observation
		observationChanged := options.Redact != nil
		if !options.IncludeObservations {
			observation.Output = nil
			observation.Regradable = false
			for spanIndex := range observation.Trace.Spans {
				span := &observation.Trace.Spans[spanIndex]
				span.Arguments = ""
				span.Result = ""
				span.Error = ""
			}
		} else {
			if observation.Output != nil {
				value, truncated := retain("output", *observation.Output)
				observation.Output = &value
				observationChanged = observationChanged || truncated
			}
			for spanIndex := range observation.Trace.Spans {
				span := &observation.Trace.Spans[spanIndex]
				spanTruncated := false
				var truncated bool
				span.Arguments, truncated = retain("tool.arguments", span.Arguments)
				spanTruncated = spanTruncated || truncated
				observationChanged = observationChanged || truncated
				span.Result, truncated = retain("tool.result", span.Result)
				spanTruncated = spanTruncated || truncated
				observationChanged = observationChanged || truncated
				span.Error, truncated = retain("tool.error", span.Error)
				spanTruncated = spanTruncated || truncated
				observationChanged = observationChanged || truncated
				span.ContentTruncated = span.ContentTruncated || spanTruncated
			}
		}
		if observation.Error != nil {
			value, truncated := retain("error", observation.Error.Message)
			observation.Error.Message = value
			observationChanged = observationChanged || truncated
		}
		if observationChanged {
			observation.Regradable = false
			observation.Trace.Incomplete = true
			if observation.Trace.Reason == "" {
				observation.Trace.Reason = "export content was redacted or truncated"
			}
		}

		for errorIndex := range clone.Cases[caseIndex].Errors {
			detail := &clone.Cases[caseIndex].Errors[errorIndex]
			detail.Message, _ = retain("case.error", detail.Message)
		}
		for metricIndex := range clone.Cases[caseIndex].Metrics {
			metric := &clone.Cases[caseIndex].Metrics[metricIndex]
			metric.Message, _ = retain("metric.message", metric.Message)
			if metric.Evidence != nil {
				retained, _ := retainValue("metric.evidence", metric.Evidence)
				metric.Evidence = retained.(map[string]any)
			}
		}
	}
	if redactionErr != nil {
		return Report{}, redactionErr
	}
	return clone, nil
}

func callRedactor(redact func(string, string) string, field, value string) (result string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("eval: report redactor panic: %v", recovered)
		}
	}()
	return redact(field, value), nil
}

func writeFileAtomically(path string, write func(io.Writer) error) error {
	if path == "" {
		return errors.New("eval: output path is empty")
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".agent-eval-*")
	if err != nil {
		return fmt.Errorf("eval: create temporary report: %w", err)
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := write(temporary); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("eval: sync report: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("eval: close report: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("eval: replace report: %w", err)
	}
	keep = true
	return nil
}

func metricMessages(result CaseResult, status MetricStatus) string {
	var messages []string
	for _, metric := range result.Metrics {
		if metric.Status == status {
			messages = append(messages, fmt.Sprintf("%s (%s): %s", metric.CheckID, metric.Evaluator, metric.Message))
		}
	}
	return strings.Join(messages, "\n")
}

func caseErrorMessages(result CaseResult) string {
	var messages []string
	for _, detail := range result.Errors {
		messages = append(messages, fmt.Sprintf("%s: %s", detail.Stage, detail.Message))
	}
	for _, metric := range result.Metrics {
		if metric.Status == MetricStatusError || (metric.Required && metric.Status == MetricStatusUnavailable) {
			messages = append(messages, fmt.Sprintf("%s (%s): %s", metric.CheckID, metric.Evaluator, metric.Message))
		}
	}
	return strings.Join(messages, "\n")
}

func secondsString(milliseconds int64) string {
	return strconv.FormatFloat(float64(milliseconds)/1000, 'f', 3, 64)
}

func xmlSafe(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' ||
			(r >= 0x20 && r <= 0xD7FF) ||
			(r >= 0xE000 && r <= 0xFFFD) ||
			(r >= 0x10000 && r <= 0x10FFFF) {
			return r
		}
		return -1
	}, value)
}
