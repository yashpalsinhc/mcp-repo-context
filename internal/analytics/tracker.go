// Package analytics provides usage tracking for MCP tools.
package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ToolUsage represents a single tool usage record.
type ToolUsage struct {
	ID           int64     `json:"id,omitempty"`
	Tool         string    `json:"tool"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	DurationMs   int64     `json:"duration_ms"`
	Timestamp    time.Time `json:"timestamp"`
	Success      bool      `json:"success"`
	ErrorMsg     string    `json:"error_msg,omitempty"`
}

// ToolStats represents aggregated statistics for a tool.
type ToolStats struct {
	Tool             string  `json:"tool"`
	TotalCalls       int64   `json:"total_calls"`
	SuccessfulCalls  int64   `json:"successful_calls"`
	FailedCalls      int64   `json:"failed_calls"`
	TotalInputTokens int64   `json:"total_input_tokens"`
	TotalOutputTokens int64  `json:"total_output_tokens"`
	AvgInputTokens   float64 `json:"avg_input_tokens"`
	AvgOutputTokens  float64 `json:"avg_output_tokens"`
	AvgDurationMs    float64 `json:"avg_duration_ms"`
	MinDurationMs    int64   `json:"min_duration_ms"`
	MaxDurationMs    int64   `json:"max_duration_ms"`
	LastUsed         time.Time `json:"last_used"`
}

// UsageStats represents overall usage statistics.
type UsageStats struct {
	TotalCalls        int64       `json:"total_calls"`
	TotalInputTokens  int64       `json:"total_input_tokens"`
	TotalOutputTokens int64       `json:"total_output_tokens"`
	ToolStats         []ToolStats `json:"tool_stats"`
	Period            string      `json:"period,omitempty"`
}

// UsageTracker tracks tool usage with SQLite storage.
type UsageTracker struct {
	db       *sql.DB
	dbPath   string
	mu       sync.RWMutex
	enabled  bool
}

// TrackerOption configures the UsageTracker.
type TrackerOption func(*UsageTracker)

// WithEnabled sets whether tracking is enabled.
func WithEnabled(enabled bool) TrackerOption {
	return func(t *UsageTracker) {
		t.enabled = enabled
	}
}

// NewUsageTracker creates a new usage tracker with SQLite storage.
func NewUsageTracker(dataDir string, opts ...TrackerOption) (*UsageTracker, error) {
	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "usage_analytics.db")

	// Open database with WAL mode for better concurrency
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open analytics database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(0)

	tracker := &UsageTracker{
		db:      db,
		dbPath:  dbPath,
		enabled: true, // enabled by default
	}

	// Apply options
	for _, opt := range opts {
		opt(tracker)
	}

	// Initialize schema
	if err := tracker.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return tracker, nil
}

// initSchema creates the usage tracking table.
func (t *UsageTracker) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tool_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tool TEXT NOT NULL,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		duration_ms INTEGER DEFAULT 0,
		success BOOLEAN DEFAULT 1,
		error_msg TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_usage_tool ON tool_usage(tool);
	CREATE INDEX IF NOT EXISTS idx_usage_timestamp ON tool_usage(timestamp);
	CREATE INDEX IF NOT EXISTS idx_usage_tool_timestamp ON tool_usage(tool, timestamp);
	`

	_, err := t.db.Exec(schema)
	return err
}

// Record records a tool usage event.
func (t *UsageTracker) Record(usage ToolUsage) error {
	if !t.enabled {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if usage.Timestamp.IsZero() {
		usage.Timestamp = time.Now()
	}

	_, err := t.db.Exec(`
		INSERT INTO tool_usage (tool, input_tokens, output_tokens, duration_ms, success, error_msg, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		usage.Tool, usage.InputTokens, usage.OutputTokens, usage.DurationMs,
		usage.Success, usage.ErrorMsg, usage.Timestamp,
	)
	return err
}

// RecordWithContext records a tool usage event with context support.
func (t *UsageTracker) RecordWithContext(ctx context.Context, usage ToolUsage) error {
	if !t.enabled {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if usage.Timestamp.IsZero() {
		usage.Timestamp = time.Now()
	}

	_, err := t.db.ExecContext(ctx, `
		INSERT INTO tool_usage (tool, input_tokens, output_tokens, duration_ms, success, error_msg, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		usage.Tool, usage.InputTokens, usage.OutputTokens, usage.DurationMs,
		usage.Success, usage.ErrorMsg, usage.Timestamp,
	)
	return err
}

// GetStats returns aggregated usage statistics per tool.
func (t *UsageTracker) GetStats(ctx context.Context) (*UsageStats, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Get overall totals
	var stats UsageStats
	err := t.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(COUNT(*), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0)
		FROM tool_usage
	`).Scan(&stats.TotalCalls, &stats.TotalInputTokens, &stats.TotalOutputTokens)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get total stats: %w", err)
	}

	// Get per-tool statistics
	rows, err := t.db.QueryContext(ctx, `
		SELECT
			tool,
			COUNT(*) as total_calls,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successful_calls,
			SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failed_calls,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(AVG(input_tokens), 0) as avg_input_tokens,
			COALESCE(AVG(output_tokens), 0) as avg_output_tokens,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms,
			COALESCE(MIN(duration_ms), 0) as min_duration_ms,
			COALESCE(MAX(duration_ms), 0) as max_duration_ms,
			MAX(timestamp) as last_used
		FROM tool_usage
		GROUP BY tool
		ORDER BY total_calls DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ts ToolStats
		var lastUsedStr sql.NullString
		err := rows.Scan(
			&ts.Tool,
			&ts.TotalCalls,
			&ts.SuccessfulCalls,
			&ts.FailedCalls,
			&ts.TotalInputTokens,
			&ts.TotalOutputTokens,
			&ts.AvgInputTokens,
			&ts.AvgOutputTokens,
			&ts.AvgDurationMs,
			&ts.MinDurationMs,
			&ts.MaxDurationMs,
			&lastUsedStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tool stats: %w", err)
		}
		if lastUsedStr.Valid {
			// Parse SQLite datetime format
			ts.LastUsed, _ = time.Parse("2006-01-02 15:04:05", lastUsedStr.String)
		}
		stats.ToolStats = append(stats.ToolStats, ts)
	}

	return &stats, nil
}

// GetStatsSince returns statistics since a specific time.
func (t *UsageTracker) GetStatsSince(ctx context.Context, since time.Time) (*UsageStats, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var stats UsageStats
	stats.Period = fmt.Sprintf("since %s", since.Format(time.RFC3339))

	// Get totals since
	err := t.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(COUNT(*), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0)
		FROM tool_usage
		WHERE timestamp >= ?
	`, since).Scan(&stats.TotalCalls, &stats.TotalInputTokens, &stats.TotalOutputTokens)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get total stats: %w", err)
	}

	// Get per-tool statistics since
	rows, err := t.db.QueryContext(ctx, `
		SELECT
			tool,
			COUNT(*) as total_calls,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successful_calls,
			SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failed_calls,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(AVG(input_tokens), 0) as avg_input_tokens,
			COALESCE(AVG(output_tokens), 0) as avg_output_tokens,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms,
			COALESCE(MIN(duration_ms), 0) as min_duration_ms,
			COALESCE(MAX(duration_ms), 0) as max_duration_ms,
			MAX(timestamp) as last_used
		FROM tool_usage
		WHERE timestamp >= ?
		GROUP BY tool
		ORDER BY total_calls DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ts ToolStats
		var lastUsedStr sql.NullString
		err := rows.Scan(
			&ts.Tool,
			&ts.TotalCalls,
			&ts.SuccessfulCalls,
			&ts.FailedCalls,
			&ts.TotalInputTokens,
			&ts.TotalOutputTokens,
			&ts.AvgInputTokens,
			&ts.AvgOutputTokens,
			&ts.AvgDurationMs,
			&ts.MinDurationMs,
			&ts.MaxDurationMs,
			&lastUsedStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tool stats: %w", err)
		}
		if lastUsedStr.Valid {
			// Parse SQLite datetime format
			ts.LastUsed, _ = time.Parse("2006-01-02 15:04:05", lastUsedStr.String)
		}
		stats.ToolStats = append(stats.ToolStats, ts)
	}

	return &stats, nil
}

// GetRecentUsage returns the most recent usage records for debugging.
func (t *UsageTracker) GetRecentUsage(ctx context.Context, limit int) ([]ToolUsage, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	rows, err := t.db.QueryContext(ctx, `
		SELECT id, tool, input_tokens, output_tokens, duration_ms, success, error_msg, timestamp
		FROM tool_usage
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent usage: %w", err)
	}
	defer rows.Close()

	var usages []ToolUsage
	for rows.Next() {
		var u ToolUsage
		var errorMsg sql.NullString
		err := rows.Scan(&u.ID, &u.Tool, &u.InputTokens, &u.OutputTokens,
			&u.DurationMs, &u.Success, &errorMsg, &u.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage: %w", err)
		}
		if errorMsg.Valid {
			u.ErrorMsg = errorMsg.String
		}
		usages = append(usages, u)
	}

	return usages, nil
}

// GetToolUsage returns usage records for a specific tool.
func (t *UsageTracker) GetToolUsage(ctx context.Context, toolName string, limit int) ([]ToolUsage, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	rows, err := t.db.QueryContext(ctx, `
		SELECT id, tool, input_tokens, output_tokens, duration_ms, success, error_msg, timestamp
		FROM tool_usage
		WHERE tool = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, toolName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool usage: %w", err)
	}
	defer rows.Close()

	var usages []ToolUsage
	for rows.Next() {
		var u ToolUsage
		var errorMsg sql.NullString
		err := rows.Scan(&u.ID, &u.Tool, &u.InputTokens, &u.OutputTokens,
			&u.DurationMs, &u.Success, &errorMsg, &u.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage: %w", err)
		}
		if errorMsg.Valid {
			u.ErrorMsg = errorMsg.String
		}
		usages = append(usages, u)
	}

	return usages, nil
}

// Cleanup removes old usage records older than the specified duration.
func (t *UsageTracker) Cleanup(ctx context.Context, olderThan time.Duration) (int64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	result, err := t.db.ExecContext(ctx, `
		DELETE FROM tool_usage WHERE timestamp < ?
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old records: %w", err)
	}

	return result.RowsAffected()
}

// Enable enables usage tracking.
func (t *UsageTracker) Enable() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.enabled = true
}

// Disable disables usage tracking.
func (t *UsageTracker) Disable() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.enabled = false
}

// IsEnabled returns whether tracking is enabled.
func (t *UsageTracker) IsEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.enabled
}

// Close closes the database connection.
func (t *UsageTracker) Close() error {
	return t.db.Close()
}
