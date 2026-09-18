package eval

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// Grade evaluates one saved observation without running an agent.
func Grade(ctx context.Context, evalCase Case, observation Observation, evaluators Evaluators) (CaseResult, error) {
	result := CaseResult{
		CaseID:      evalCase.ID,
		AttemptID:   observation.AttemptID,
		Observation: observation,
		Status:      CaseStatusPass,
	}
	if observation.Status != RunStatusCompleted {
		result.Status = CaseStatusError
		if observation.Error != nil {
			result.Errors = append(result.Errors, *observation.Error)
		} else {
			result.Errors = append(result.Errors, ErrorDetail{
				Stage:   "execution",
				Message: fmt.Sprintf("execution status is %s", observation.Status),
			})
		}
	}

	for _, check := range evalCase.Checks {
		if err := ctx.Err(); err != nil {
			result.Status = CaseStatusError
			result.Errors = append(result.Errors, errorDetail("grading", err))
			return result, err
		}

		evaluator, ok := evaluators[check.Type]
		if !ok || evaluator == nil {
			metric := metricBase(check, check.Type)
			metric.Status = MetricStatusError
			metric.Message = "evaluator is not registered"
			result.Metrics = append(result.Metrics, metric)
			if check.RequiredValue() {
				result.Status = CaseStatusError
			}
			continue
		}

		evaluatorName := safeEvaluatorName(evaluator, check.Type)
		metric, err := callEvaluator(ctx, evaluator, Evaluation{
			Case:        evalCase,
			Check:       check,
			Observation: observation,
		})
		if err != nil {
			metric = metricBase(check, evaluatorName)
			metric.Status = MetricStatusError
			metric.Message = err.Error()
		}
		metric.CheckID = check.ID
		metric.Evaluator = evaluatorName
		metric.Threshold = check.ThresholdValue()
		metric.Required = check.RequiredValue()
		if err := validateMetric(metric); err != nil {
			metric.Status = MetricStatusError
			metric.Score = nil
			metric.Message = fmt.Sprintf("invalid evaluator result: %v", err)
		}
		result.Metrics = append(result.Metrics, metric)

		if !metric.Required {
			continue
		}
		switch metric.Status {
		case MetricStatusFail:
			if result.Status == CaseStatusPass {
				result.Status = CaseStatusFail
			}
		case MetricStatusUnavailable, MetricStatusError:
			result.Status = CaseStatusError
		}
	}

	return result, nil
}

func callEvaluator(ctx context.Context, evaluator Evaluator, input Evaluation) (result MetricResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("evaluator panic: %v", recovered)
		}
	}()
	return evaluator.Evaluate(ctx, input)
}

func safeEvaluatorName(evaluator Evaluator, fallback string) (name string) {
	name = fallback
	defer func() {
		if recover() != nil {
			name = fallback
		}
	}()
	if candidate := evaluator.Name(); candidate != "" {
		name = candidate
	}
	return name
}

func validateMetric(metric MetricResult) error {
	switch metric.Status {
	case MetricStatusPass, MetricStatusFail:
		if metric.Score == nil {
			return fmt.Errorf("status %s requires a score", metric.Status)
		}
		if math.IsNaN(*metric.Score) || math.IsInf(*metric.Score, 0) || *metric.Score < 0 || *metric.Score > 1 {
			return fmt.Errorf("score must be between 0 and 1")
		}
		passes := *metric.Score >= metric.Threshold
		if passes != (metric.Status == MetricStatusPass) {
			return fmt.Errorf("status %s is inconsistent with score %v and threshold %v", metric.Status, *metric.Score, metric.Threshold)
		}
	case MetricStatusUnavailable, MetricStatusError:
		if metric.Score != nil {
			return fmt.Errorf("status %s cannot include a score", metric.Status)
		}
	default:
		return fmt.Errorf("unknown metric status %q", metric.Status)
	}
	return nil
}

func summarize(results []CaseResult) Summary {
	summary := Summary{Total: len(results)}
	type metricAccumulator struct {
		scored      int
		unavailable int
		errors      int
		total       float64
	}
	metrics := make(map[string]*metricAccumulator)
	for _, result := range results {
		switch result.Status {
		case CaseStatusPass:
			summary.Passed++
		case CaseStatusFail:
			summary.Failed++
		case CaseStatusError:
			summary.Errors++
		case CaseStatusNotStarted:
			summary.NotStarted++
		}
		for _, metric := range result.Metrics {
			acc := metrics[metric.Evaluator]
			if acc == nil {
				acc = &metricAccumulator{}
				metrics[metric.Evaluator] = acc
			}
			switch metric.Status {
			case MetricStatusPass, MetricStatusFail:
				if metric.Score != nil {
					acc.scored++
					acc.total += *metric.Score
				}
			case MetricStatusUnavailable:
				acc.unavailable++
			case MetricStatusError:
				acc.errors++
			}
		}
	}
	completed := summary.Passed + summary.Failed + summary.Errors
	if completed > 0 {
		summary.PassRate = float64(summary.Passed) / float64(completed)
	}
	names := make([]string, 0, len(metrics))
	for name := range metrics {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		acc := metrics[name]
		metricSummary := MetricSummary{
			Evaluator:   name,
			Scored:      acc.scored,
			Unavailable: acc.unavailable,
			Errors:      acc.errors,
		}
		if acc.scored > 0 {
			metricSummary.MeanScore = acc.total / float64(acc.scored)
		}
		summary.Metrics = append(summary.Metrics, metricSummary)
	}
	return summary
}
