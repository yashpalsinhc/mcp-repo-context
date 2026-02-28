package queue

import "time"

// Job type constants.
const (
	JobTypeAnalyzeRepo = "analyze_repo"
	JobTypeAnalyzeOrg  = "analyze_org"
	JobTypePRContext   = "pr_context"
)

// Job status constants.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Job represents a queued work item.
type Job struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	OrgID       string     `json:"org_id,omitempty"`
	RepoID      string     `json:"repo_id,omitempty"`
	Payload     string     `json:"payload"`
	Status      string     `json:"status"`
	Result      string     `json:"result,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Error       string     `json:"error,omitempty"`
	Retries     int        `json:"retries"`
	MaxRetries  int        `json:"max_retries"`
}
