package queue

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yashpalc/mcp-repo-context/internal/logging"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
	"github.com/yashpalc/mcp-repo-context/internal/org"
)

const schema = `
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    org_id TEXT,
    repo_id TEXT,
    payload TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    result TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME,
    completed_at DATETIME,
    error TEXT,
    retries INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status, created_at);
CREATE INDEX IF NOT EXISTS idx_jobs_org_repo ON jobs(org_id, repo_id, created_at);
`

// JobQueue manages asynchronous jobs using a SQLite-backed queue.
type JobQueue struct {
	db         *sql.DB
	manager    orchestrator.Manager
	orgManager org.Manager
	workers    int
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	logger     *logging.Logger
}

// NewJobQueue creates a new job queue with its own SQLite database.
func NewJobQueue(dbPath string, manager orchestrator.Manager, orgManager org.Manager, workers int, logger *logging.Logger) (*JobQueue, error) {
	if logger == nil {
		logger = logging.Default()
	}
	if workers < 1 {
		workers = 3
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(workers + 3) // workers + sweeper + cleanup + enqueue
	db.SetMaxIdleConns(workers + 1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	return &JobQueue{
		db:         db,
		manager:    manager,
		orgManager: orgManager,
		workers:    workers,
		logger:     logger,
	}, nil
}

// Enqueue adds a new job to the queue. Handles deduplication: if a job with the
// same (repo_id, type) was created within the last 5 minutes and is not failed,
// the existing job ID is returned.
func (q *JobQueue) Enqueue(ctx context.Context, jobType, orgID, repoID, payload string) (string, error) {
	// Dedup check
	if repoID != "" {
		var existingID string
		err := q.db.QueryRowContext(ctx,
			`SELECT id FROM jobs WHERE repo_id = ? AND type = ? AND status != 'failed'
			 AND created_at > datetime('now', '-5 minutes') LIMIT 1`,
			repoID, jobType).Scan(&existingID)
		if err == nil && existingID != "" {
			return existingID, nil
		}
	}

	id := generateUUID()
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO jobs (id, type, org_id, repo_id, payload, status, max_retries) VALUES (?, ?, ?, ?, ?, 'pending', 3)`,
		id, jobType, orgID, repoID, payload)
	if err != nil {
		return "", err
	}
	return id, nil
}

// GetJob retrieves a job by ID.
func (q *JobQueue) GetJob(ctx context.Context, jobID string) (*Job, error) {
	row := q.db.QueryRowContext(ctx,
		`SELECT id, type, org_id, repo_id, payload, status, result, created_at, started_at, completed_at, error, retries, max_retries
		 FROM jobs WHERE id = ?`, jobID)
	return scanJob(row)
}

// ListJobs returns a paginated list of jobs with optional status filter.
func (q *JobQueue) ListJobs(ctx context.Context, orgID, status string, limit, offset int) ([]Job, int, error) {
	var total int
	var args []interface{}
	where := "1=1"

	if orgID != "" {
		where += " AND org_id = ?"
		args = append(args, orgID)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}

	err := q.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE "+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	queryArgs := append(args, limit, offset)
	rows, err := q.db.QueryContext(ctx,
		"SELECT id, type, org_id, repo_id, payload, status, result, created_at, started_at, completed_at, error, retries, max_retries FROM jobs WHERE "+where+" ORDER BY created_at DESC LIMIT ? OFFSET ?",
		queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		j, err := scanJobRows(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, total, nil
}

// Stats returns the count of pending and running jobs.
func (q *JobQueue) Stats(ctx context.Context) (pending, running int) {
	q.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE status = 'pending'").Scan(&pending)
	q.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE status = 'running'").Scan(&running)
	return
}

// Start starts worker goroutines, sweeper, and cleanup.
func (q *JobQueue) Start(ctx context.Context) {
	workerCtx, cancel := context.WithCancel(ctx)
	q.cancel = cancel

	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.workerLoop(workerCtx, i)
	}

	q.wg.Add(1)
	go q.sweepLoop(workerCtx)

	q.wg.Add(1)
	go q.cleanupLoop(workerCtx)
}

// Stop cancels workers and waits for them to finish.
func (q *JobQueue) Stop() {
	if q.cancel != nil {
		q.cancel()
	}
	q.wg.Wait()
	q.db.Close()
}

func scanJob(row *sql.Row) (*Job, error) {
	j := &Job{}
	var orgID, repoID, result, errMsg sql.NullString
	var startedAt, completedAt sql.NullTime
	err := row.Scan(&j.ID, &j.Type, &orgID, &repoID, &j.Payload, &j.Status, &result,
		&j.CreatedAt, &startedAt, &completedAt, &errMsg, &j.Retries, &j.MaxRetries)
	if err != nil {
		return nil, err
	}
	j.OrgID = orgID.String
	j.RepoID = repoID.String
	j.Result = result.String
	j.Error = errMsg.String
	if startedAt.Valid {
		j.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}
	return j, nil
}

func scanJobRows(rows *sql.Rows) (*Job, error) {
	j := &Job{}
	var orgID, repoID, result, errMsg sql.NullString
	var startedAt, completedAt sql.NullTime
	err := rows.Scan(&j.ID, &j.Type, &orgID, &repoID, &j.Payload, &j.Status, &result,
		&j.CreatedAt, &startedAt, &completedAt, &errMsg, &j.Retries, &j.MaxRetries)
	if err != nil {
		return nil, err
	}
	j.OrgID = orgID.String
	j.RepoID = repoID.String
	j.Result = result.String
	j.Error = errMsg.String
	if startedAt.Valid {
		j.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}
	return j, nil
}

func generateUUID() string {
	// Simple UUID v4 without external dependency
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant is 10
	return formatUUID(b)
}

func formatUUID(b []byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
