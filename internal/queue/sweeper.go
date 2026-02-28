package queue

import (
	"context"
	"time"
)

// sweepLoop recovers stale jobs (running > 30 min) every 5 minutes.
func (q *JobQueue) sweepLoop(ctx context.Context) {
	defer q.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.recoverStaleJobs(ctx)
		}
	}
}

// recoverStaleJobs finds running jobs older than 30 minutes and resets them.
func (q *JobQueue) recoverStaleJobs(ctx context.Context) {
	rows, err := q.db.QueryContext(ctx,
		`SELECT id, retries, max_retries FROM jobs
		 WHERE status = 'running' AND started_at < datetime('now', '-30 minutes')`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var retries, maxRetries int
		if err := rows.Scan(&id, &retries, &maxRetries); err != nil {
			continue
		}

		newRetries := retries + 1
		if newRetries < maxRetries {
			q.db.ExecContext(ctx,
				`UPDATE jobs SET status = 'pending', retries = ?, error = 'recovered from stale state', started_at = NULL WHERE id = ?`,
				newRetries, id)
			q.logger.Warn("recovered stale job %s (retry %d/%d)", id, newRetries, maxRetries)
		} else {
			q.db.ExecContext(ctx,
				`UPDATE jobs SET status = 'failed', retries = ?, error = 'job timed out after 30 minutes', completed_at = CURRENT_TIMESTAMP WHERE id = ?`,
				newRetries, id)
			q.logger.Error("stale job %s failed permanently (max retries exhausted)", id)
		}
	}
}

// cleanupLoop removes old completed/failed jobs every hour.
func (q *JobQueue) cleanupLoop(ctx context.Context) {
	defer q.wg.Done()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.cleanupOldJobs(ctx)
		}
	}
}

// cleanupOldJobs deletes jobs older than 7 days.
func (q *JobQueue) cleanupOldJobs(ctx context.Context) {
	result, err := q.db.ExecContext(ctx,
		`DELETE FROM jobs WHERE
		 (status = 'completed' AND completed_at < datetime('now', '-7 days'))
		 OR (status = 'failed' AND created_at < datetime('now', '-7 days'))`)
	if err != nil {
		return
	}
	if rows, err := result.RowsAffected(); err == nil && rows > 0 {
		q.logger.Info("cleaned up %d old jobs", rows)
	}
}
