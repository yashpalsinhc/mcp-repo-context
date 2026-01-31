package analytics

import (
	"context"
	"time"

	"github.com/yashpalc/mcp-repo-context/internal/tokens"
)

// ToolExecutor represents a function that executes a tool.
type ToolExecutor func(ctx context.Context, args map[string]interface{}) (result interface{}, isError bool, err error)

// TrackedToolExecutor wraps a tool executor with usage tracking.
type TrackedToolExecutor struct {
	tracker *UsageTracker
	counter *tokens.TokenCounter
}

// NewTrackedToolExecutor creates a new tracked tool executor.
func NewTrackedToolExecutor(tracker *UsageTracker) *TrackedToolExecutor {
	return &TrackedToolExecutor{
		tracker: tracker,
		counter: tokens.NewTokenCounter(),
	}
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	Content     string
	IsError     bool
	InputTokens int
	OutputTokens int
	DurationMs  int64
}

// Wrap wraps a tool executor with usage tracking.
func (t *TrackedToolExecutor) Wrap(toolName string, executor ToolExecutor) ToolExecutor {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, bool, error) {
		// Count input tokens from arguments
		inputTokens := t.counter.CountJSON(args)

		// Track execution time
		start := time.Now()

		// Execute the tool
		result, isError, err := executor(ctx, args)

		// Calculate duration
		durationMs := time.Since(start).Milliseconds()

		// Count output tokens from result
		outputTokens := t.counter.CountJSON(result)

		// Record usage
		usage := ToolUsage{
			Tool:         toolName,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			DurationMs:   durationMs,
			Success:      !isError && err == nil,
			Timestamp:    time.Now(),
		}
		if err != nil {
			usage.ErrorMsg = err.Error()
		}

		// Record asynchronously to not block tool execution
		go func() {
			_ = t.tracker.Record(usage)
		}()

		return result, isError, err
	}
}

// RecordToolCall is a simpler interface for recording tool usage after execution.
// Use this when you can't wrap the executor directly.
func (t *TrackedToolExecutor) RecordToolCall(ctx context.Context, toolName string, inputArgs map[string]interface{}, outputContent string, isError bool, durationMs int64, errMsg string) {
	inputTokens := t.counter.CountJSON(inputArgs)
	outputTokens := t.counter.Count(outputContent)

	usage := ToolUsage{
		Tool:         toolName,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		DurationMs:   durationMs,
		Success:      !isError,
		ErrorMsg:     errMsg,
		Timestamp:    time.Now(),
	}

	// Record asynchronously
	go func() {
		_ = t.tracker.RecordWithContext(context.Background(), usage)
	}()
}

// TrackingMiddleware provides middleware for tracking tool execution.
type TrackingMiddleware struct {
	tracker *UsageTracker
	counter *tokens.TokenCounter
}

// NewTrackingMiddleware creates a new tracking middleware.
func NewTrackingMiddleware(tracker *UsageTracker) *TrackingMiddleware {
	return &TrackingMiddleware{
		tracker: tracker,
		counter: tokens.NewTokenCounter(),
	}
}

// BeforeExecution should be called before tool execution.
// Returns a function that should be called after execution completes.
func (m *TrackingMiddleware) BeforeExecution(toolName string, args map[string]interface{}) func(outputContent string, isError bool, errMsg string) {
	inputTokens := m.counter.CountJSON(args)
	start := time.Now()

	return func(outputContent string, isError bool, errMsg string) {
		durationMs := time.Since(start).Milliseconds()
		outputTokens := m.counter.Count(outputContent)

		usage := ToolUsage{
			Tool:         toolName,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			DurationMs:   durationMs,
			Success:      !isError,
			ErrorMsg:     errMsg,
			Timestamp:    time.Now(),
		}

		// Record asynchronously
		go func() {
			_ = m.tracker.Record(usage)
		}()
	}
}

// Track is a convenience method that combines BeforeExecution pattern.
// Usage:
//   done := middleware.Track("tool_name", args)
//   result := executeToolLogic(ctx, args)
//   done(result.Text, result.IsError, "")
func (m *TrackingMiddleware) Track(toolName string, args map[string]interface{}) func(outputContent string, isError bool, errMsg string) {
	return m.BeforeExecution(toolName, args)
}

// TrackSync records tool usage synchronously. Use this for testing or when
// you need to ensure the record is persisted before continuing.
func (m *TrackingMiddleware) TrackSync(ctx context.Context, toolName string, args map[string]interface{}, outputContent string, isError bool, durationMs int64, errMsg string) error {
	inputTokens := m.counter.CountJSON(args)
	outputTokens := m.counter.Count(outputContent)

	usage := ToolUsage{
		Tool:         toolName,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		DurationMs:   durationMs,
		Success:      !isError,
		ErrorMsg:     errMsg,
		Timestamp:    time.Now(),
	}

	return m.tracker.RecordWithContext(ctx, usage)
}
