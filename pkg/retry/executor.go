package retry

import (
	"context"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// Executor handles the execution of operations with retries
type Executor struct {
	policy *Policy
	logger logging.Logger
}

// NewExecutor creates a new retry executor with the given policy
func NewExecutor(policy *Policy) *Executor {
	return &Executor{
		policy: policy,
		logger: logging.New(),
	}
}

// Execute executes the given operation with retries based on the policy
func (e *Executor) Execute(ctx context.Context, operation func() error) error {
	var lastErr error
	attempt := int32(0)
	currentInterval := e.policy.InitialInterval

	for attempt < e.policy.MaximumAttempts {
		select {
		case <-ctx.Done():
			e.logger.Debug(ctx, "Context cancelled during retry", map[string]interface{}{
				"attempt": attempt,
				"error":   ctx.Err(),
			})
			return ctx.Err()
		default:
			e.logger.Debug(ctx, "Attempting operation", map[string]interface{}{
				"attempt":      attempt + 1,
				"max_attempts": e.policy.MaximumAttempts,
			})

			if err := operation(); err == nil {
				e.logger.Debug(ctx, "Operation succeeded", map[string]interface{}{
					"attempt": attempt + 1,
				})
				return nil
			} else {
				lastErr = err
				attempt++

				if attempt >= e.policy.MaximumAttempts {
					e.logger.Debug(ctx, "Maximum attempts reached", map[string]interface{}{
						"attempt": attempt,
						"error":   err.Error(),
					})
					break
				}

				// Calculate next backoff interval
				nextInterval := time.Duration(float64(currentInterval) * e.policy.BackoffCoefficient)
				if nextInterval > e.policy.MaximumInterval {
					nextInterval = e.policy.MaximumInterval
				}

				e.logger.Debug(ctx, "Operation failed, scheduling retry", map[string]interface{}{
					"attempt":          attempt,
					"error":            err.Error(),
					"current_interval": currentInterval,
					"next_interval":    nextInterval,
				})

				select {
				case <-ctx.Done():
					e.logger.Debug(ctx, "Context cancelled during retry delay", map[string]interface{}{
						"attempt": attempt,
						"error":   ctx.Err(),
					})
					return ctx.Err()
				case <-time.After(currentInterval):
					currentInterval = nextInterval
				}
			}
		}
	}

	return lastErr
}
