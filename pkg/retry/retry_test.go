package retry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPolicyDefaults(t *testing.T) {
	p := NewPolicy()

	assert.Equal(t, time.Second, p.InitialInterval)
	assert.Equal(t, 2.0, p.BackoffCoefficient)
	assert.Equal(t, 100*time.Second, p.MaximumInterval)
	assert.Equal(t, int32(3), p.MaximumAttempts)
}

func TestNewPolicyOptions(t *testing.T) {
	p := NewPolicy(
		WithInitialInterval(5*time.Millisecond),
		WithBackoffCoefficient(1.5),
		WithMaximumInterval(20*time.Millisecond),
		WithMaxAttempts(7),
	)

	assert.Equal(t, 5*time.Millisecond, p.InitialInterval)
	assert.Equal(t, 1.5, p.BackoffCoefficient)
	assert.Equal(t, 20*time.Millisecond, p.MaximumInterval)
	assert.Equal(t, int32(7), p.MaximumAttempts)
}

// fastPolicy keeps retry delays short so the tests stay fast.
func fastPolicy(attempts int32) *Policy {
	return NewPolicy(
		WithInitialInterval(time.Millisecond),
		WithBackoffCoefficient(2),
		WithMaximumInterval(2*time.Millisecond),
		WithMaxAttempts(attempts),
	)
}

func TestExecute(t *testing.T) {
	errFail := errors.New("fail")

	tests := []struct {
		name         string
		attempts     int32
		failures     int
		wantErr      error
		wantAttempts int
	}{
		{name: "succeeds first try", attempts: 3, failures: 0, wantErr: nil, wantAttempts: 1},
		{name: "succeeds after retries", attempts: 3, failures: 2, wantErr: nil, wantAttempts: 3},
		{name: "fails after max attempts", attempts: 3, failures: 10, wantErr: errFail, wantAttempts: 3},
		{name: "single attempt failure", attempts: 1, failures: 10, wantErr: errFail, wantAttempts: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			op := func() error {
				calls++
				if calls <= tt.failures {
					return errFail
				}
				return nil
			}

			err := NewExecutor(fastPolicy(tt.attempts)).Execute(context.Background(), op)

			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, tt.wantErr)
			}
			assert.Equal(t, tt.wantAttempts, calls)
		})
	}
}

func TestExecuteReturnsLastError(t *testing.T) {
	calls := 0
	err := NewExecutor(fastPolicy(3)).Execute(context.Background(), func() error {
		calls++
		return fmt.Errorf("attempt %d", calls)
	})

	require.Error(t, err)
	assert.Equal(t, "attempt 3", err.Error())
}

func TestExecuteCancelledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := NewExecutor(fastPolicy(3)).Execute(ctx, func() error {
		calls++
		return nil
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, calls)
}

func TestExecuteCancelledDuringDelay(t *testing.T) {
	// A long delay ensures the cancel lands while the executor waits.
	policy := NewPolicy(WithInitialInterval(time.Hour), WithMaxAttempts(5))
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := NewExecutor(policy).Execute(ctx, func() error {
		calls++
		cancel()
		return errors.New("fail")
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
}

func TestExecuteBackoffIsCappedByMaximumInterval(t *testing.T) {
	// With a cap of 10ms, the three delays are 10ms, 10ms, 10ms
	// instead of 10ms, 1s, 100s.
	policy := NewPolicy(
		WithInitialInterval(10*time.Millisecond),
		WithBackoffCoefficient(100),
		WithMaximumInterval(10*time.Millisecond),
		WithMaxAttempts(4),
	)

	start := time.Now()
	err := NewExecutor(policy).Execute(context.Background(), func() error {
		return errors.New("fail")
	})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.GreaterOrEqual(t, elapsed, 30*time.Millisecond)
	assert.Less(t, elapsed, time.Second)
}
