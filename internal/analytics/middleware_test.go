package analytics

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestTrackedToolExecutor_Wrap(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "analytics_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker, err := NewUsageTracker(tmpDir)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}
	defer tracker.Close()

	executor := NewTrackedToolExecutor(tracker)

	// Create a mock tool executor
	mockExecutor := func(ctx context.Context, args map[string]interface{}) (interface{}, bool, error) {
		time.Sleep(10 * time.Millisecond) // Simulate some work
		return map[string]string{"result": "success"}, false, nil
	}

	// Wrap the executor
	wrappedExecutor := executor.Wrap("test_tool", mockExecutor)

	// Execute the wrapped function
	ctx := context.Background()
	result, isError, err := wrappedExecutor(ctx, map[string]interface{}{
		"arg1": "value1",
		"arg2": 123,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isError {
		t.Error("expected isError to be false")
	}
	if result == nil {
		t.Error("expected result to not be nil")
	}

	// Wait for async recording
	time.Sleep(50 * time.Millisecond)

	// Verify the usage was recorded
	usages, err := tracker.GetRecentUsage(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}

	usage := usages[0]
	if usage.Tool != "test_tool" {
		t.Errorf("expected tool test_tool, got %s", usage.Tool)
	}
	if !usage.Success {
		t.Error("expected success to be true")
	}
	if usage.DurationMs < 10 {
		t.Errorf("expected duration >= 10ms, got %d", usage.DurationMs)
	}
	if usage.InputTokens == 0 {
		t.Error("expected non-zero input tokens")
	}
	if usage.OutputTokens == 0 {
		t.Error("expected non-zero output tokens")
	}
}

func TestTrackedToolExecutor_WrapError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "analytics_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker, err := NewUsageTracker(tmpDir)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}
	defer tracker.Close()

	executor := NewTrackedToolExecutor(tracker)

	// Create a mock tool executor that returns an error
	mockExecutor := func(ctx context.Context, args map[string]interface{}) (interface{}, bool, error) {
		return map[string]string{"error": "something went wrong"}, true, nil
	}

	wrappedExecutor := executor.Wrap("failing_tool", mockExecutor)

	ctx := context.Background()
	_, isError, _ := wrappedExecutor(ctx, map[string]interface{}{"arg": "value"})

	if !isError {
		t.Error("expected isError to be true")
	}

	// Wait for async recording
	time.Sleep(50 * time.Millisecond)

	usages, err := tracker.GetRecentUsage(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}

	if usages[0].Success {
		t.Error("expected success to be false for error result")
	}
}

func TestTrackedToolExecutor_RecordToolCall(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "analytics_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker, err := NewUsageTracker(tmpDir)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}
	defer tracker.Close()

	executor := NewTrackedToolExecutor(tracker)

	ctx := context.Background()
	executor.RecordToolCall(
		ctx,
		"manual_tool",
		map[string]interface{}{"key": "value"},
		"This is the output content with some text",
		false,
		250,
		"",
	)

	// Wait for async recording
	time.Sleep(50 * time.Millisecond)

	usages, err := tracker.GetRecentUsage(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}

	usage := usages[0]
	if usage.Tool != "manual_tool" {
		t.Errorf("expected tool manual_tool, got %s", usage.Tool)
	}
	if usage.DurationMs != 250 {
		t.Errorf("expected duration 250ms, got %d", usage.DurationMs)
	}
	if !usage.Success {
		t.Error("expected success to be true")
	}
}

func TestTrackingMiddleware_Track(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "analytics_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker, err := NewUsageTracker(tmpDir)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}
	defer tracker.Close()

	middleware := NewTrackingMiddleware(tracker)

	// Simulate tool execution with Track pattern
	args := map[string]interface{}{
		"query": "test query",
		"limit": 10,
	}

	done := middleware.Track("search_tool", args)

	// Simulate some work
	time.Sleep(20 * time.Millisecond)
	outputContent := "Found 5 results matching your query"

	// Complete tracking
	done(outputContent, false, "")

	// Wait for async recording
	time.Sleep(50 * time.Millisecond)

	ctx := context.Background()
	usages, err := tracker.GetRecentUsage(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}

	usage := usages[0]
	if usage.Tool != "search_tool" {
		t.Errorf("expected tool search_tool, got %s", usage.Tool)
	}
	if usage.DurationMs < 20 {
		t.Errorf("expected duration >= 20ms, got %d", usage.DurationMs)
	}
	if !usage.Success {
		t.Error("expected success to be true")
	}
}

func TestTrackingMiddleware_TrackWithError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "analytics_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker, err := NewUsageTracker(tmpDir)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}
	defer tracker.Close()

	middleware := NewTrackingMiddleware(tracker)

	args := map[string]interface{}{"id": "nonexistent"}

	done := middleware.Track("get_item", args)
	time.Sleep(5 * time.Millisecond)
	done("Error: item not found", true, "item not found")

	// Wait for async recording
	time.Sleep(50 * time.Millisecond)

	ctx := context.Background()
	usages, err := tracker.GetRecentUsage(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}

	usage := usages[0]
	if usage.Success {
		t.Error("expected success to be false")
	}
	if usage.ErrorMsg != "item not found" {
		t.Errorf("expected error msg 'item not found', got '%s'", usage.ErrorMsg)
	}
}

func TestTrackingMiddleware_TrackSync(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "analytics_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker, err := NewUsageTracker(tmpDir)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}
	defer tracker.Close()

	middleware := NewTrackingMiddleware(tracker)

	ctx := context.Background()
	args := map[string]interface{}{"param": "value"}
	outputContent := "Sync result"

	err = middleware.TrackSync(ctx, "sync_tool", args, outputContent, false, 100, "")
	if err != nil {
		t.Fatalf("failed to track sync: %v", err)
	}

	// No need to wait - TrackSync is synchronous
	usages, err := tracker.GetRecentUsage(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}

	usage := usages[0]
	if usage.Tool != "sync_tool" {
		t.Errorf("expected tool sync_tool, got %s", usage.Tool)
	}
	if usage.DurationMs != 100 {
		t.Errorf("expected duration 100ms, got %d", usage.DurationMs)
	}
}

func TestTrackingMiddleware_MultipleTools(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "analytics_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker, err := NewUsageTracker(tmpDir)
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}
	defer tracker.Close()

	middleware := NewTrackingMiddleware(tracker)
	ctx := context.Background()

	// Track multiple tools
	tools := []struct {
		name    string
		args    map[string]interface{}
		output  string
		isError bool
	}{
		{"tool1", map[string]interface{}{"a": 1}, "output1", false},
		{"tool2", map[string]interface{}{"b": 2}, "output2", false},
		{"tool1", map[string]interface{}{"a": 3}, "output3", false},
		{"tool3", map[string]interface{}{"c": 4}, "error output", true},
	}

	for _, tool := range tools {
		err := middleware.TrackSync(ctx, tool.name, tool.args, tool.output, tool.isError, 50, "")
		if err != nil {
			t.Fatalf("failed to track %s: %v", tool.name, err)
		}
	}

	stats, err := tracker.GetStats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.TotalCalls != 4 {
		t.Errorf("expected 4 total calls, got %d", stats.TotalCalls)
	}

	// Check tool1 has 2 calls
	var tool1Stats *ToolStats
	for i := range stats.ToolStats {
		if stats.ToolStats[i].Tool == "tool1" {
			tool1Stats = &stats.ToolStats[i]
			break
		}
	}

	if tool1Stats == nil {
		t.Fatal("tool1 stats not found")
	}
	if tool1Stats.TotalCalls != 2 {
		t.Errorf("expected 2 calls for tool1, got %d", tool1Stats.TotalCalls)
	}

	// Check tool3 has 1 failed call
	var tool3Stats *ToolStats
	for i := range stats.ToolStats {
		if stats.ToolStats[i].Tool == "tool3" {
			tool3Stats = &stats.ToolStats[i]
			break
		}
	}

	if tool3Stats == nil {
		t.Fatal("tool3 stats not found")
	}
	if tool3Stats.FailedCalls != 1 {
		t.Errorf("expected 1 failed call for tool3, got %d", tool3Stats.FailedCalls)
	}
}
