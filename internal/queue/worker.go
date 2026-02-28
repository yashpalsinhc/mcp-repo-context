package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
)

// workerLoop continuously claims and executes jobs until context is cancelled.
func (q *JobQueue) workerLoop(ctx context.Context, workerID int) {
	defer q.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := q.claimJob(ctx)
		if err != nil || job == nil {
			// No job available, wait before retrying
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
			continue
		}

		q.logger.Info("worker %d: executing job %s (type=%s)", workerID, job.ID, job.Type)
		q.executeJob(ctx, job)
	}
}

// claimJob atomically claims a pending job using an IMMEDIATE transaction.
func (q *JobQueue) claimJob(ctx context.Context) (*Job, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// First, select the ID of the job we want to claim
	var jobID string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM jobs
		 WHERE status = 'pending' AND created_at <= CURRENT_TIMESTAMP
		 ORDER BY created_at ASC
		 LIMIT 1`).Scan(&jobID)
	if err != nil {
		return nil, nil // No job available (includes sql.ErrNoRows)
	}

	// Claim it by updating status
	result, err := tx.ExecContext(ctx,
		`UPDATE jobs SET status = 'running', started_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status = 'pending'`, jobID)
	if err != nil {
		return nil, err
	}

	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return nil, nil // Another worker got it
	}

	// Read back the claimed job by its specific ID
	row := tx.QueryRowContext(ctx,
		`SELECT id, type, org_id, repo_id, payload, status, result, created_at, started_at, completed_at, error, retries, max_retries
		 FROM jobs WHERE id = ?`, jobID)

	job := &Job{}
	var orgIDVal, repoIDVal, jobResult, errMsg *string
	var startedAt, completedAt *time.Time
	err = row.Scan(&job.ID, &job.Type, &orgIDVal, &repoIDVal, &job.Payload, &job.Status, &jobResult,
		&job.CreatedAt, &startedAt, &completedAt, &errMsg, &job.Retries, &job.MaxRetries)
	if err != nil {
		return nil, err
	}
	if orgIDVal != nil {
		job.OrgID = *orgIDVal
	}
	if repoIDVal != nil {
		job.RepoID = *repoIDVal
	}
	if startedAt != nil {
		job.StartedAt = startedAt
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return job, nil
}

// executeJob runs the job based on its type and updates status.
func (q *JobQueue) executeJob(ctx context.Context, job *Job) {
	var result string
	var jobErr error

	switch job.Type {
	case JobTypeAnalyzeRepo:
		result, jobErr = q.executeAnalyzeRepo(ctx, job)
	case JobTypeAnalyzeOrg:
		result, jobErr = q.executeAnalyzeOrg(ctx, job)
	case JobTypePRContext:
		result, jobErr = q.executePRContext(ctx, job)
	default:
		jobErr = fmt.Errorf("unknown job type: %s", job.Type)
	}

	if jobErr != nil {
		q.handleJobFailure(ctx, job, jobErr)
		return
	}

	// Mark as completed
	now := time.Now()
	q.db.ExecContext(ctx,
		`UPDATE jobs SET status = 'completed', result = ?, completed_at = ?, error = NULL WHERE id = ?`,
		result, now, job.ID)
	q.logger.Info("job %s completed", job.ID)
}

func (q *JobQueue) executeAnalyzeRepo(ctx context.Context, job *Job) (string, error) {
	var payload struct {
		RepoURL string `json:"repo_url"`
		Branch  string `json:"branch"`
	}
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return "", fmt.Errorf("invalid payload: %w", err)
	}

	analyzeResult, err := q.manager.AnalyzeRepo(ctx, payload.RepoURL, orchestrator.AnalyzeOptions{
		Branch: payload.Branch,
		Force:  true,
	})
	if err != nil {
		return "", err
	}

	resultJSON, _ := json.Marshal(analyzeResult)
	return string(resultJSON), nil
}

func (q *JobQueue) executeAnalyzeOrg(ctx context.Context, job *Job) (string, error) {
	if q.orgManager == nil {
		return "", fmt.Errorf("org manager not configured")
	}

	result, err := q.orgManager.AnalyzeOrg(ctx, job.OrgID, false, 2)
	if err != nil {
		return "", err
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

func (q *JobQueue) executePRContext(ctx context.Context, job *Job) (string, error) {
	var payload struct {
		RepoID       string                       `json:"repo_id"`
		ChangedFiles []orchestrator.ChangedFile    `json:"changed_files"`
	}
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return "", fmt.Errorf("invalid payload: %w", err)
	}

	result, err := q.manager.GetPRContext(ctx, payload.RepoID, payload.ChangedFiles)
	if err != nil {
		return "", err
	}

	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// handleJobFailure increments retries and either reschedules or marks as failed.
func (q *JobQueue) handleJobFailure(ctx context.Context, job *Job, jobErr error) {
	newRetries := job.Retries + 1

	if newRetries < job.MaxRetries {
		// Exponential backoff: 2^retries * 30s
		backoff := time.Duration(math.Pow(2, float64(newRetries))) * 30 * time.Second
		newCreatedAt := time.Now().Add(backoff)
		q.db.ExecContext(ctx,
			`UPDATE jobs SET status = 'pending', retries = ?, created_at = ?, error = ?, started_at = NULL WHERE id = ?`,
			newRetries, newCreatedAt, jobErr.Error(), job.ID)
		q.logger.Warn("job %s failed (retry %d/%d): %v", job.ID, newRetries, job.MaxRetries, jobErr)
	} else {
		q.db.ExecContext(ctx,
			`UPDATE jobs SET status = 'failed', retries = ?, error = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`,
			newRetries, jobErr.Error(), job.ID)
		q.logger.Error("job %s failed permanently after %d retries: %v", job.ID, newRetries, jobErr)
	}
}
