package analytics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewUsageTracker(t *testing.T) {
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

	// Verify database file was created
	dbPath := filepath.Join(tmpDir, "usage_analytics.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}

	// Verify tracker is enabled by default
	if !tracker.IsEnabled() {
		t.Error("tracker should be enabled by default")
	}
}

func TestUsageTracker_Record(t *testing.T) {
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

	// Record a usage event
	usage := ToolUsage{
		Tool:         "analyze_repo",
		InputTokens:  100,
		OutputTokens: 500,
		DurationMs:   1500,
		Success:      true,
	}

	err = tracker.Record(usage)
	if err != nil {
		t.Fatalf("failed to record usage: %v", err)
	}

	// Verify it was recorded
	ctx := context.Background()
	usages, err := tracker.GetRecentUsage(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}

	if usages[0].Tool != "analyze_repo" {
		t.Errorf("expected tool analyze_repo, got %s", usages[0].Tool)
	}
	if usages[0].InputTokens != 100 {
		t.Errorf("expected input tokens 100, got %d", usages[0].InputTokens)
	}
	if usages[0].OutputTokens != 500 {
		t.Errorf("expected output tokens 500, got %d", usages[0].OutputTokens)
	}
	if usages[0].DurationMs != 1500 {
		t.Errorf("expected duration 1500ms, got %d", usages[0].DurationMs)
	}
	if !usages[0].Success {
		t.Error("expected success to be true")
	}
}

func TestUsageTracker_RecordFailure(t *testing.T) {
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

	// Record a failed usage event
	usage := ToolUsage{
		Tool:         "get_context",
		InputTokens:  50,
		OutputTokens: 10,
		DurationMs:   100,
		Success:      false,
		ErrorMsg:     "repository not found",
	}

	err = tracker.Record(usage)
	if err != nil {
		t.Fatalf("failed to record usage: %v", err)
	}

	ctx := context.Background()
	usages, err := tracker.GetRecentUsage(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}

	if usages[0].Success {
		t.Error("expected success to be false")
	}
	if usages[0].ErrorMsg != "repository not found" {
		t.Errorf("expected error msg 'repository not found', got '%s'", usages[0].ErrorMsg)
	}
}

func TestUsageTracker_GetStats(t *testing.T) {
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

	// Record multiple usage events for different tools
	usages := []ToolUsage{
		{Tool: "analyze_repo", InputTokens: 100, OutputTokens: 500, DurationMs: 1000, Success: true},
		{Tool: "analyze_repo", InputTokens: 150, OutputTokens: 600, DurationMs: 1200, Success: true},
		{Tool: "get_context", InputTokens: 50, OutputTokens: 1000, DurationMs: 200, Success: true},
		{Tool: "get_context", InputTokens: 60, OutputTokens: 800, DurationMs: 150, Success: false, ErrorMsg: "not found"},
		{Tool: "search_context", InputTokens: 30, OutputTokens: 200, DurationMs: 100, Success: true},
	}

	for _, u := range usages {
		if err := tracker.Record(u); err != nil {
			t.Fatalf("failed to record usage: %v", err)
		}
	}

	ctx := context.Background()
	stats, err := tracker.GetStats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	// Verify totals
	if stats.TotalCalls != 5 {
		t.Errorf("expected 5 total calls, got %d", stats.TotalCalls)
	}

	expectedInputTokens := int64(100 + 150 + 50 + 60 + 30)
	if stats.TotalInputTokens != expectedInputTokens {
		t.Errorf("expected %d total input tokens, got %d", expectedInputTokens, stats.TotalInputTokens)
	}

	expectedOutputTokens := int64(500 + 600 + 1000 + 800 + 200)
	if stats.TotalOutputTokens != expectedOutputTokens {
		t.Errorf("expected %d total output tokens, got %d", expectedOutputTokens, stats.TotalOutputTokens)
	}

	// Verify per-tool stats
	if len(stats.ToolStats) != 3 {
		t.Fatalf("expected 3 tool stats, got %d", len(stats.ToolStats))
	}

	// Find analyze_repo stats (should be first since it has most calls)
	var analyzeStats *ToolStats
	for i := range stats.ToolStats {
		if stats.ToolStats[i].Tool == "analyze_repo" {
			analyzeStats = &stats.ToolStats[i]
			break
		}
	}

	if analyzeStats == nil {
		t.Fatal("analyze_repo stats not found")
	}

	if analyzeStats.TotalCalls != 2 {
		t.Errorf("expected 2 calls for analyze_repo, got %d", analyzeStats.TotalCalls)
	}
	if analyzeStats.SuccessfulCalls != 2 {
		t.Errorf("expected 2 successful calls for analyze_repo, got %d", analyzeStats.SuccessfulCalls)
	}
	if analyzeStats.FailedCalls != 0 {
		t.Errorf("expected 0 failed calls for analyze_repo, got %d", analyzeStats.FailedCalls)
	}

	// Check averages
	expectedAvgInput := float64(100+150) / 2
	if analyzeStats.AvgInputTokens != expectedAvgInput {
		t.Errorf("expected avg input tokens %.2f, got %.2f", expectedAvgInput, analyzeStats.AvgInputTokens)
	}

	// Find get_context stats
	var getCtxStats *ToolStats
	for i := range stats.ToolStats {
		if stats.ToolStats[i].Tool == "get_context" {
			getCtxStats = &stats.ToolStats[i]
			break
		}
	}

	if getCtxStats == nil {
		t.Fatal("get_context stats not found")
	}

	if getCtxStats.FailedCalls != 1 {
		t.Errorf("expected 1 failed call for get_context, got %d", getCtxStats.FailedCalls)
	}
}

func TestUsageTracker_GetStatsSince(t *testing.T) {
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

	// Record events at different times
	oldUsage := ToolUsage{
		Tool:         "old_tool",
		InputTokens:  100,
		OutputTokens: 500,
		DurationMs:   1000,
		Success:      true,
		Timestamp:    time.Now().Add(-2 * time.Hour),
	}

	newUsage := ToolUsage{
		Tool:         "new_tool",
		InputTokens:  200,
		OutputTokens: 800,
		DurationMs:   500,
		Success:      true,
		Timestamp:    time.Now(),
	}

	if err := tracker.Record(oldUsage); err != nil {
		t.Fatalf("failed to record old usage: %v", err)
	}
	if err := tracker.Record(newUsage); err != nil {
		t.Fatalf("failed to record new usage: %v", err)
	}

	ctx := context.Background()

	// Get stats for last hour only
	stats, err := tracker.GetStatsSince(ctx, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("failed to get stats since: %v", err)
	}

	if stats.TotalCalls != 1 {
		t.Errorf("expected 1 call in last hour, got %d", stats.TotalCalls)
	}

	if len(stats.ToolStats) != 1 {
		t.Fatalf("expected 1 tool stat, got %d", len(stats.ToolStats))
	}

	if stats.ToolStats[0].Tool != "new_tool" {
		t.Errorf("expected new_tool, got %s", stats.ToolStats[0].Tool)
	}
}

func TestUsageTracker_GetToolUsage(t *testing.T) {
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

	// Record multiple tools
	usages := []ToolUsage{
		{Tool: "analyze_repo", InputTokens: 100, OutputTokens: 500, DurationMs: 1000, Success: true},
		{Tool: "get_context", InputTokens: 50, OutputTokens: 200, DurationMs: 100, Success: true},
		{Tool: "analyze_repo", InputTokens: 150, OutputTokens: 600, DurationMs: 1200, Success: true},
	}

	for _, u := range usages {
		if err := tracker.Record(u); err != nil {
			t.Fatalf("failed to record usage: %v", err)
		}
	}

	ctx := context.Background()
	analyzeUsages, err := tracker.GetToolUsage(ctx, "analyze_repo", 10)
	if err != nil {
		t.Fatalf("failed to get tool usage: %v", err)
	}

	if len(analyzeUsages) != 2 {
		t.Fatalf("expected 2 usages for analyze_repo, got %d", len(analyzeUsages))
	}

	for _, u := range analyzeUsages {
		if u.Tool != "analyze_repo" {
			t.Errorf("expected tool analyze_repo, got %s", u.Tool)
		}
	}
}

func TestUsageTracker_Cleanup(t *testing.T) {
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

	// Record old and new events
	oldUsage := ToolUsage{
		Tool:         "old_tool",
		InputTokens:  100,
		OutputTokens: 500,
		DurationMs:   1000,
		Success:      true,
		Timestamp:    time.Now().Add(-48 * time.Hour),
	}

	newUsage := ToolUsage{
		Tool:         "new_tool",
		InputTokens:  200,
		OutputTokens: 800,
		DurationMs:   500,
		Success:      true,
		Timestamp:    time.Now(),
	}

	if err := tracker.Record(oldUsage); err != nil {
		t.Fatalf("failed to record old usage: %v", err)
	}
	if err := tracker.Record(newUsage); err != nil {
		t.Fatalf("failed to record new usage: %v", err)
	}

	ctx := context.Background()

	// Clean up records older than 24 hours
	deleted, err := tracker.Cleanup(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("failed to cleanup: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 record deleted, got %d", deleted)
	}

	// Verify only new record remains
	usages, err := tracker.GetRecentUsage(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage remaining, got %d", len(usages))
	}

	if usages[0].Tool != "new_tool" {
		t.Errorf("expected new_tool to remain, got %s", usages[0].Tool)
	}
}

func TestUsageTracker_EnableDisable(t *testing.T) {
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

	// Disable tracking
	tracker.Disable()
	if tracker.IsEnabled() {
		t.Error("tracker should be disabled")
	}

	// Record should be a no-op when disabled
	usage := ToolUsage{
		Tool:         "test_tool",
		InputTokens:  100,
		OutputTokens: 500,
		DurationMs:   1000,
		Success:      true,
	}

	err = tracker.Record(usage)
	if err != nil {
		t.Fatalf("Record should succeed silently when disabled: %v", err)
	}

	ctx := context.Background()
	usages, err := tracker.GetRecentUsage(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}

	if len(usages) != 0 {
		t.Errorf("expected 0 usages when disabled, got %d", len(usages))
	}

	// Re-enable and verify recording works
	tracker.Enable()
	if !tracker.IsEnabled() {
		t.Error("tracker should be enabled")
	}

	err = tracker.Record(usage)
	if err != nil {
		t.Fatalf("failed to record after re-enable: %v", err)
	}

	usages, err = tracker.GetRecentUsage(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}

	if len(usages) != 1 {
		t.Errorf("expected 1 usage after re-enable, got %d", len(usages))
	}
}

func TestUsageTracker_WithEnabledOption(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "analytics_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create tracker with disabled option
	tracker, err := NewUsageTracker(tmpDir, WithEnabled(false))
	if err != nil {
		t.Fatalf("failed to create tracker: %v", err)
	}
	defer tracker.Close()

	if tracker.IsEnabled() {
		t.Error("tracker should be disabled via option")
	}
}

func TestUsageTracker_EmptyStats(t *testing.T) {
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

	ctx := context.Background()
	stats, err := tracker.GetStats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.TotalCalls != 0 {
		t.Errorf("expected 0 total calls, got %d", stats.TotalCalls)
	}
	if stats.TotalInputTokens != 0 {
		t.Errorf("expected 0 total input tokens, got %d", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 0 {
		t.Errorf("expected 0 total output tokens, got %d", stats.TotalOutputTokens)
	}
	if len(stats.ToolStats) != 0 {
		t.Errorf("expected 0 tool stats, got %d", len(stats.ToolStats))
	}
}

func TestUsageTracker_RecentUsageLimit(t *testing.T) {
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

	// Record more than limit
	for i := 0; i < 30; i++ {
		usage := ToolUsage{
			Tool:         "test_tool",
			InputTokens:  i,
			OutputTokens: i * 2,
			DurationMs:   int64(i * 10),
			Success:      true,
		}
		if err := tracker.Record(usage); err != nil {
			t.Fatalf("failed to record usage: %v", err)
		}
	}

	ctx := context.Background()

	// Default limit should work
	usages, err := tracker.GetRecentUsage(ctx, 0) // 0 defaults to 20
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}
	if len(usages) != 20 {
		t.Errorf("expected 20 usages with default limit, got %d", len(usages))
	}

	// Custom limit
	usages, err = tracker.GetRecentUsage(ctx, 5)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}
	if len(usages) != 5 {
		t.Errorf("expected 5 usages, got %d", len(usages))
	}

	// Max limit should be capped at 100
	usages, err = tracker.GetRecentUsage(ctx, 200)
	if err != nil {
		t.Fatalf("failed to get recent usage: %v", err)
	}
	if len(usages) != 30 { // We only have 30 records
		t.Errorf("expected 30 usages (all records), got %d", len(usages))
	}
}
