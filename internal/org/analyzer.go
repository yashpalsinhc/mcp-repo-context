package org

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
)

// Analyzer orchestrates concurrent analysis of all repos in an org.
type Analyzer struct {
	orgManager Manager
	orch       orchestrator.Manager
}

// NewAnalyzer creates an Analyzer.
func NewAnalyzer(orgManager Manager, orch orchestrator.Manager) *Analyzer {
	return &Analyzer{orgManager: orgManager, orch: orch}
}

// AnalyzeOrg analyzes all repos in an org with bounded concurrency and single-retry.
func (a *Analyzer) AnalyzeOrg(ctx context.Context, orgID string, force bool, concurrency int) (*AnalysisResult, error) {
	// Clamp concurrency
	if concurrency <= 0 {
		concurrency = 3
	}
	if concurrency > 10 {
		concurrency = 10
	}

	org, err := a.orgManager.Get(ctx, orgID)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	result := &AnalysisResult{
		OrgID: orgID,
		Total: len(org.Repos),
	}

	if len(org.Repos) == 0 {
		result.Duration = time.Since(start)
		return result, nil
	}

	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, repoID := range org.Repos {
		// Check cancellation before launching — break, don't return,
		// so we fall through to wg.Wait() for in-flight goroutines.
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(repoID string) {
			defer wg.Done()

			// Acquire semaphore or bail on cancellation
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }() // release

			err := a.analyzeRepo(ctx, repoID, force)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.Failed++
				result.Errors = append(result.Errors, RepoError{
					RepoID: repoID,
					Error:  err.Error(),
				})
			} else {
				result.Succeeded++
			}
		}(repoID)
	}

	wg.Wait()
	result.Duration = time.Since(start)
	return result, nil
}

// analyzeRepo analyzes a single repo with single-retry for transient errors.
func (a *Analyzer) analyzeRepo(ctx context.Context, repoID string, force bool) error {
	err := a.callOrchestrator(ctx, repoID, force)
	if err == nil {
		return nil
	}

	// Skip retry for non-retryable errors
	if isNonRetryable(err) {
		return err
	}

	// Wait and retry once (use NewTimer to avoid leak on cancel)
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return err
	case <-timer.C:
	}

	return a.callOrchestrator(ctx, repoID, force)
}

// callOrchestrator routes to AnalyzeLocal or AnalyzeRepo based on prefix.
func (a *Analyzer) callOrchestrator(ctx context.Context, repoID string, force bool) error {
	if path, ok := strings.CutPrefix(repoID, "local:"); ok {
		_, err := a.orch.AnalyzeLocal(ctx, path, orchestrator.AnalyzeLocalOptions{Force: force})
		return err
	}
	_, err := a.orch.AnalyzeRepo(ctx, repoID, orchestrator.AnalyzeOptions{Force: force})
	return err
}

// isNonRetryable returns true for errors that should not be retried.
func isNonRetryable(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "invalid")
}
