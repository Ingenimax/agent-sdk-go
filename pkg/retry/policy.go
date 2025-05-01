package retry

import "time"

// Policy defines the retry policy configuration
type Policy struct {
	InitialInterval    time.Duration
	BackoffCoefficient float64
	MaximumInterval    time.Duration
	MaximumAttempts    int32
}

// Option represents a retry policy option
type Option func(*Policy)

// WithInitialInterval sets the initial interval for retries
func WithInitialInterval(interval time.Duration) Option {
	return func(p *Policy) {
		p.InitialInterval = interval
	}
}

// WithBackoffCoefficient sets the backoff coefficient
func WithBackoffCoefficient(coefficient float64) Option {
	return func(p *Policy) {
		p.BackoffCoefficient = coefficient
	}
}

// WithMaximumInterval sets the maximum interval between retries
func WithMaximumInterval(interval time.Duration) Option {
	return func(p *Policy) {
		p.MaximumInterval = interval
	}
}

// WithMaxAttempts sets the maximum number of retry attempts
func WithMaxAttempts(attempts int32) Option {
	return func(p *Policy) {
		p.MaximumAttempts = attempts
	}
}

// NewPolicy creates a new retry policy with default values
func NewPolicy(opts ...Option) *Policy {
	policy := &Policy{
		InitialInterval:    time.Second,       // Default 1s
		BackoffCoefficient: 2.0,               // Default exponential backoff
		MaximumInterval:    time.Second * 100, // Default 100s
		MaximumAttempts:    3,                 // Default 3 attempts
	}

	for _, opt := range opts {
		opt(policy)
	}

	return policy
}
