package ai

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryer_SuccessFirstTry(t *testing.T) {
	r := NewRetryer(DefaultRetryConfig())
	ctx := context.Background()

	var attempts int32
	err := r.Do(ctx, func() error {
		atomic.AddInt32(&attempts, 1)
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("Expected 1 attempt, got %d", attempts)
	}
}

func TestRetryer_SuccessAfterRetry(t *testing.T) {
	config := DefaultRetryConfig()
	config.InitialBackoff = 1 * time.Millisecond // Fast for testing
	r := NewRetryer(config)
	ctx := context.Background()

	var attempts int32
	err := r.Do(ctx, func() error {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			return NewAPIError("test", http.StatusTooManyRequests, "rate limited")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryer_ExhaustedRetries(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxRetries = 2
	config.InitialBackoff = 1 * time.Millisecond
	r := NewRetryer(config)
	ctx := context.Background()

	var attempts int32
	err := r.Do(ctx, func() error {
		atomic.AddInt32(&attempts, 1)
		return NewAPIError("test", http.StatusTooManyRequests, "rate limited")
	})

	if err == nil {
		t.Error("Expected error after exhausted retries")
	}
	// Initial attempt + 2 retries = 3 attempts
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryer_NonRetryableError(t *testing.T) {
	config := DefaultRetryConfig()
	config.InitialBackoff = 1 * time.Millisecond
	r := NewRetryer(config)
	ctx := context.Background()

	var attempts int32
	err := r.Do(ctx, func() error {
		atomic.AddInt32(&attempts, 1)
		return NewAPIError("test", http.StatusBadRequest, "invalid request") // 400 is not retryable
	})

	if err == nil {
		t.Error("Expected error for non-retryable status")
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("Expected 1 attempt (no retries), got %d", attempts)
	}
}

func TestRetryer_ContextCancellation(t *testing.T) {
	config := DefaultRetryConfig()
	config.InitialBackoff = 100 * time.Millisecond
	r := NewRetryer(config)

	ctx, cancel := context.WithCancel(context.Background())

	var attempts int32
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := r.Do(ctx, func() error {
		atomic.AddInt32(&attempts, 1)
		return NewAPIError("test", http.StatusTooManyRequests, "rate limited")
	})

	if err == nil {
		t.Error("Expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(ctx.Err(), context.Canceled) {
		t.Errorf("Expected context cancelled error, got: %v", err)
	}
}

func TestRetryer_RetryableStatusCodes(t *testing.T) {
	config := DefaultRetryConfig()
	r := NewRetryer(config)

	retryableCodes := []int{429, 500, 502, 503, 504}
	for _, code := range retryableCodes {
		err := NewAPIError("test", code, "error")
		if !r.shouldRetry(err) {
			t.Errorf("Status %d should be retryable", code)
		}
	}

	nonRetryableCodes := []int{400, 401, 403, 404, 422}
	for _, code := range nonRetryableCodes {
		err := NewAPIError("test", code, "error")
		if r.shouldRetry(err) {
			t.Errorf("Status %d should not be retryable", code)
		}
	}
}

func TestRetryer_RetryableErrorMessages(t *testing.T) {
	r := NewRetryer(DefaultRetryConfig())

	retryableErrors := []string{
		"connection refused",
		"timeout exceeded",
		"rate limit exceeded",
		"service unavailable",
	}

	for _, msg := range retryableErrors {
		err := errors.New(msg)
		if !r.shouldRetry(err) {
			t.Errorf("Error '%s' should be retryable", msg)
		}
	}
}

func TestRetryer_OnRetryCallback(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxRetries = 2
	config.InitialBackoff = 1 * time.Millisecond

	var callbackCalls int32
	config.OnRetry = func(attempt int, err error, waitTime time.Duration) {
		atomic.AddInt32(&callbackCalls, 1)
	}

	r := NewRetryer(config)
	ctx := context.Background()

	r.Do(ctx, func() error {
		return NewAPIError("test", http.StatusTooManyRequests, "rate limited")
	})

	if atomic.LoadInt32(&callbackCalls) != 2 {
		t.Errorf("Expected 2 callback calls, got %d", callbackCalls)
	}
}

func TestDoWithResult(t *testing.T) {
	config := DefaultRetryConfig()
	config.InitialBackoff = 1 * time.Millisecond
	r := NewRetryer(config)
	ctx := context.Background()

	var attempts int32
	result, err := DoWithResult(ctx, r, func() (string, error) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 2 {
			return "", NewAPIError("test", http.StatusTooManyRequests, "rate limited")
		}
		return "success", nil
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got '%s'", result)
	}
}

func TestAPIError(t *testing.T) {
	err := NewAPIError("anthropic", 429, "rate limited")

	expected := "anthropic API error (status 429): rate limited"
	if err.Error() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, err.Error())
	}

	if err.StatusCode != 429 {
		t.Errorf("Expected status 429, got %d", err.StatusCode)
	}
}

func TestRetryOperation(t *testing.T) {
	var attempts int32
	ctx := context.Background()

	err := RetryOperation(ctx, 2, func() error {
		count := atomic.AddInt32(&attempts, 1)
		if count < 2 {
			return errors.New("timeout")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestIsRetryable(t *testing.T) {
	if !IsRetryable(NewAPIError("test", 429, "")) {
		t.Error("429 should be retryable")
	}
	if IsRetryable(NewAPIError("test", 400, "")) {
		t.Error("400 should not be retryable")
	}
}

func TestCalculateBackoff(t *testing.T) {
	config := RetryConfig{
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        1 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0, // No jitter for predictable testing
	}
	r := NewRetryer(config)

	// Attempt 0: 100ms
	backoff0 := r.calculateBackoff(0)
	if backoff0 != 100*time.Millisecond {
		t.Errorf("Expected 100ms, got %v", backoff0)
	}

	// Attempt 1: 200ms
	backoff1 := r.calculateBackoff(1)
	if backoff1 != 200*time.Millisecond {
		t.Errorf("Expected 200ms, got %v", backoff1)
	}

	// Attempt 2: 400ms
	backoff2 := r.calculateBackoff(2)
	if backoff2 != 400*time.Millisecond {
		t.Errorf("Expected 400ms, got %v", backoff2)
	}

	// Attempt 4: 1600ms -> capped at 1000ms
	backoff4 := r.calculateBackoff(4)
	if backoff4 != 1*time.Second {
		t.Errorf("Expected 1s (capped), got %v", backoff4)
	}
}
