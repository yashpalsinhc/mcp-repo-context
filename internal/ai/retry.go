package ai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// RetryConfig configures the retry behavior for AI API calls.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (0 = no retries).
	MaxRetries int

	// InitialBackoff is the initial wait time before the first retry.
	InitialBackoff time.Duration

	// MaxBackoff is the maximum wait time between retries.
	MaxBackoff time.Duration

	// BackoffMultiplier is the factor by which backoff increases each retry.
	BackoffMultiplier float64

	// Jitter adds randomness to backoff to prevent thundering herd.
	// Value between 0 and 1, where 0.1 means +/- 10% jitter.
	Jitter float64

	// RetryableStatusCodes are HTTP status codes that should trigger a retry.
	RetryableStatusCodes []int

	// OnRetry is called before each retry attempt (optional).
	OnRetry func(attempt int, err error, waitTime time.Duration)
}

// DefaultRetryConfig returns a sensible default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.1,
		RetryableStatusCodes: []int{
			http.StatusTooManyRequests,     // 429 - Rate limited
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout,      // 504
		},
	}
}

// Retryer handles retry logic for operations.
type Retryer struct {
	config RetryConfig
}

// NewRetryer creates a new retryer with the given configuration.
func NewRetryer(config RetryConfig) *Retryer {
	return &Retryer{config: config}
}

// Do executes the given operation with retry logic.
// The operation should return an error if it fails, or nil if it succeeds.
func (r *Retryer) Do(ctx context.Context, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// Check context before each attempt
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled: %w", err)
		}

		// Execute the operation
		err := operation()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if we should retry this error
		if !r.shouldRetry(err) {
			return err // Non-retryable error
		}

		// Check if we have retries left
		if attempt >= r.config.MaxRetries {
			break
		}

		// Calculate backoff with jitter
		backoff := r.calculateBackoff(attempt)

		// Call OnRetry callback if configured
		if r.config.OnRetry != nil {
			r.config.OnRetry(attempt+1, err, backoff)
		}

		// Wait before retry, respecting context cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry backoff: %w", ctx.Err())
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", r.config.MaxRetries, lastErr)
}

// DoWithResult executes an operation that returns a result.
func DoWithResult[T any](ctx context.Context, r *Retryer, operation func() (T, error)) (T, error) {
	var result T
	var lastErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("context cancelled: %w", err)
		}

		res, err := operation()
		if err == nil {
			return res, nil
		}

		lastErr = err
		result = res // Keep last result even if error

		if !r.shouldRetry(err) {
			return result, err
		}

		if attempt >= r.config.MaxRetries {
			break
		}

		backoff := r.calculateBackoff(attempt)

		if r.config.OnRetry != nil {
			r.config.OnRetry(attempt+1, err, backoff)
		}

		select {
		case <-ctx.Done():
			return result, fmt.Errorf("context cancelled during retry backoff: %w", ctx.Err())
		case <-time.After(backoff):
		}
	}

	return result, fmt.Errorf("operation failed after %d retries: %w", r.config.MaxRetries, lastErr)
}

// shouldRetry determines if an error is retryable.
func (r *Retryer) shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Check for API errors with status codes
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		for _, code := range r.config.RetryableStatusCodes {
			if apiErr.StatusCode == code {
				return true
			}
		}
		return false
	}

	// Check for common transient error patterns
	errMsg := strings.ToLower(err.Error())
	transientPatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"too many requests",
		"rate limit",
		"service unavailable",
		"server overloaded",
		"try again",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// calculateBackoff calculates the backoff duration for a given attempt.
func (r *Retryer) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff
	backoff := float64(r.config.InitialBackoff) * math.Pow(r.config.BackoffMultiplier, float64(attempt))

	// Apply max backoff cap
	if backoff > float64(r.config.MaxBackoff) {
		backoff = float64(r.config.MaxBackoff)
	}

	// Apply jitter
	if r.config.Jitter > 0 {
		jitterRange := backoff * r.config.Jitter
		backoff = backoff - jitterRange + (rand.Float64() * 2 * jitterRange)
	}

	return time.Duration(backoff)
}

// APIError represents an error from an AI API call.
type APIError struct {
	StatusCode int
	Message    string
	Provider   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s API error (status %d): %s", e.Provider, e.StatusCode, e.Message)
}

// NewAPIError creates a new API error.
func NewAPIError(provider string, statusCode int, message string) *APIError {
	return &APIError{
		Provider:   provider,
		StatusCode: statusCode,
		Message:    message,
	}
}

// IsRetryable returns true if the error is retryable with default config.
func IsRetryable(err error) bool {
	retryer := NewRetryer(DefaultRetryConfig())
	return retryer.shouldRetry(err)
}

// RetryOperation is a convenience function for one-off retry operations.
func RetryOperation(ctx context.Context, maxRetries int, operation func() error) error {
	config := DefaultRetryConfig()
	config.MaxRetries = maxRetries
	return NewRetryer(config).Do(ctx, operation)
}
