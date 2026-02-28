# Section 03: Job Queue

## Overview

SQLite-backed job queue for async analysis operations. Provides reliable job enqueue, atomic dequeue, worker execution, retry with exponential backoff, stale job recovery, old job cleanup, and webhook deduplication. Uses a **separate SQLite database** to avoid single-writer contention with the main storage.

## Dependencies

- External: `github.com/mattn/go-sqlite3` (already used in project)
- Internal: `internal/orchestrator` (Manager, for executing analysis jobs)

## New Package

`internal/queue/`

## Separate Database

The job queue uses its own SQLite file (`jobs.db`) to avoid contention with the main storage database during concurrent analysis operations. Both databases use WAL mode.

## Schema

```sql
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
```

## Job Struct

```
type Job struct {
    ID          string
    Type        string    // "analyze_repo", "analyze_org", "pr_context"
    OrgID       string    // optional
    RepoID      string    // optional
    Payload     string    // JSON blob
    Status      string    // "pending", "running", "completed", "failed"
    Result      string    // JSON blob on completion
    CreatedAt   time.Time
    StartedAt   *time.Time
    CompletedAt *time.Time
    Error       string
    Retries     int
    MaxRetries  int
}
```

## JobQueue Struct

```
type JobQueue struct {
    db         *sql.DB
    manager    orchestrator.Manager
    orgManager org.Manager
    workers    int
    cancel     context.CancelFunc
    wg         sync.WaitGroup
    logger     *logging.Logger
}
```

Constructor: `NewJobQueue(dbPath string, manager, orgManager, workers int, logger) (*JobQueue, error)` — opens SQLite, runs migrations, sets WAL mode, returns queue.

## Atomic Job Dequeue

Workers claim jobs using an atomic UPDATE-with-subquery pattern. This is critical for preventing two workers from claiming the same job:

```sql
BEGIN IMMEDIATE;
UPDATE jobs SET status = 'running', started_at = CURRENT_TIMESTAMP
WHERE id = (
    SELECT id FROM jobs
    WHERE status = 'pending' AND created_at <= CURRENT_TIMESTAMP
    ORDER BY created_at ASC
    LIMIT 1
);
-- Check changes() — if 0, no job available
COMMIT;
```

The `IMMEDIATE` transaction ensures exclusive write access. After UPDATE, read back the claimed job by querying for `status='running' AND started_at` matching.

The `created_at <= CURRENT_TIMESTAMP` condition supports exponential backoff: failed jobs are rescheduled by setting `created_at` to a future time.

## Methods

### Enqueue

```
func (q *JobQueue) Enqueue(ctx context.Context, jobType, orgID, repoID, payload string) (string, error)
```

1. Generate UUID for job ID.
2. Check for dedup: query for existing job with same `(repo_id, type)` created within last 5 minutes that is not failed. If found, return existing job ID (no new insert).
3. Insert into jobs table with status "pending".
4. Return job ID.

### GetJob

```
func (q *JobQueue) GetJob(ctx context.Context, jobID string) (*Job, error)
```

Query job by ID. Return nil and error if not found.

### ListJobs

```
func (q *JobQueue) ListJobs(ctx context.Context, orgID, status string, limit, offset int) ([]Job, int, error)
```

Paginated list with optional filters. Returns jobs, total count, error.

### Start

```
func (q *JobQueue) Start(ctx context.Context)
```

1. Create a derived context with cancel.
2. Launch `q.workers` goroutines (default 3), each running `workerLoop`.
3. Launch 1 goroutine for stale job recovery (sweeper).
4. Launch 1 goroutine for old job cleanup.

### Stop

```
func (q *JobQueue) Stop()
```

1. Cancel context (stops workers from picking new jobs).
2. Wait for all worker goroutines to finish current job (via WaitGroup).
3. Close the database.

## Worker Loop

Each worker goroutine:
1. Try to claim a job (atomic dequeue).
2. If no job available, sleep 1 second, retry (respect context cancellation).
3. If job claimed, execute it based on `job.Type`:
   - `analyze_repo`: call `manager.AnalyzeRepo(repoURL)` (extract URL from payload JSON)
   - `analyze_org`: call `orgManager.AnalyzeOrg(orgID)`
   - `pr_context`: call `manager.GetPRContext(repoID, changedFiles)`
4. On success: update job status to "completed", set result JSON and completed_at.
5. On failure: increment retries. If retries < max_retries, set status back to "pending" and set created_at to `now + 2^retries * 30s` (exponential backoff). If retries >= max_retries, set status to "failed" with error message.

## Stale Job Recovery (Sweeper)

A goroutine that runs every 5 minutes:
1. Find jobs with `status='running'` and `started_at` older than 30 minutes.
2. For each: increment retries. If retries < max_retries, set back to "pending". Otherwise set to "failed" with error "job timed out after 30 minutes".

## Old Job Cleanup

A goroutine that runs every hour:
1. Delete jobs with `completed_at` older than 7 days.
2. Delete failed jobs with `created_at` older than 7 days.

## Webhook Deduplication

The `Enqueue` method handles dedup:
- Before inserting, check if a job with the same `(repo_id, type)` exists with `created_at` within the last 5 minutes and `status` not "failed".
- If found, return the existing job ID without creating a new job.
- This persists across restarts (DB-backed, not in-memory).

## Tests

### `internal/queue/queue_test.go`

**Test: Enqueue creates pending job**
- Enqueue("analyze_repo", "org1", "repo1", payload)
- Assert returned job ID non-empty
- GetJob: assert status="pending"

**Test: Worker picks up and completes job**
- Enqueue job with a test handler
- Start queue with 1 worker
- Wait for job to complete (poll GetJob)
- Assert status="completed", result non-nil

**Test: Failed job has error message**
- Enqueue job that will fail
- Wait for completion
- Assert status="failed", error non-empty

**Test: Retry on failure**
- Enqueue job with max_retries=3
- First attempt fails
- Assert retries incremented
- Assert job back to pending (with future created_at)

**Test: Max retries exhausted**
- Enqueue job with max_retries=1
- Fail twice
- Assert status="failed" (not retried again)

**Test: Concurrent workers claim different jobs**
- Enqueue 10 jobs
- Start queue with 5 workers
- Wait for all to complete
- Assert each job processed exactly once (no duplicates)

**Test: Stale job recovery**
- Enqueue job, manually set status="running", started_at=30min ago
- Run sweeper
- Assert job back to pending

**Test: Job cleanup removes old jobs**
- Enqueue job, manually set completed_at=8 days ago
- Run cleanup
- Assert job deleted

**Test: Dedup prevents duplicate within 5 min**
- Enqueue job for repo1
- Enqueue same repo1 job again
- Assert second returns existing job ID

**Test: Dedup allows after 5 min**
- Enqueue job for repo1 with created_at=10min ago
- Enqueue same repo1 again
- Assert new job created

**Test: ListJobs paginated**
- Enqueue 5 jobs
- ListJobs(limit=2, offset=0)
- Assert 2 returned, total=5

**Test: ListJobs filtered by status**
- Enqueue 3 jobs, complete 1
- ListJobs(status="completed")
- Assert 1 returned

## File Inventory

| File | Purpose |
|------|---------|
| `internal/queue/queue.go` | JobQueue struct, constructor, Start, Stop |
| `internal/queue/job.go` | Job struct and type constants |
| `internal/queue/worker.go` | Worker loop, job execution |
| `internal/queue/sweeper.go` | Stale recovery and cleanup goroutines |
| `internal/queue/queue_test.go` | All queue tests |

## Acceptance Criteria

1. Jobs can be enqueued and retrieved by ID
2. Workers claim and execute jobs atomically (no double-processing)
3. Failed jobs retry with exponential backoff up to max_retries
4. Stale jobs (running > 30min) are recovered
5. Old jobs (> 7 days) are cleaned up
6. Duplicate enqueue within 5 min returns existing job ID
7. Pagination and status filtering work on ListJobs
8. All 12 tests pass
