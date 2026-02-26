package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// normalizeStoredPath applies basic normalization to endpoint paths at storage time:
// strips trailing slash (unless root) so SQL exact match works consistently.
func normalizeStoredPath(p string) string {
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		return strings.TrimRight(p, "/")
	}
	return p
}

// Endpoint represents a server-side HTTP route stored in normalized form.
type Endpoint struct {
	ID          int64
	RepoID      string
	FilePath    string
	HandlerName string
	Method      string // GET, POST, ANY, gRPC
	Path        string // normalized with {param}
	RawPath     string // original as written
	Framework   string // gorilla/mux, chi, net/http, gin, echo
	Line        int
}

// ServiceCall represents a client-side call (HTTP, gRPC, Kafka) stored in normalized form.
type ServiceCall struct {
	ID               int64
	RepoID           string
	FilePath         string
	FunctionName     string
	CallType         string // "http", "grpc", "kafka_produce", "kafka_consume"
	Method           string // GET, POST, gRPC, produce, consume
	Target           string // URL template or topic name
	TargetExpression string // original expression for dynamic targets
	ServiceHint      string // guessed destination service
	Line             int
}

// StoreEndpoints stores normalized endpoints for a repo (delete-then-insert).
func (s *SQLiteStore) StoreEndpoints(ctx context.Context, repoID string, endpoints []Endpoint) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := s.storeEndpointsTx(ctx, tx, repoID, endpoints); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SQLiteStore) storeEndpointsTx(ctx context.Context, tx *sql.Tx, repoID string, endpoints []Endpoint) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM endpoints WHERE repo_id = ?", repoID); err != nil {
		return fmt.Errorf("failed to delete existing endpoints: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO endpoints (repo_id, file_path, handler_name, method, path, raw_path, framework, line)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare endpoint insert: %w", err)
	}
	defer stmt.Close()

	for _, ep := range endpoints {
		storePath := normalizeStoredPath(ep.Path)
		if _, err := stmt.ExecContext(ctx, repoID, ep.FilePath, ep.HandlerName, ep.Method, storePath, ep.RawPath, ep.Framework, ep.Line); err != nil {
			return fmt.Errorf("failed to insert endpoint %s %s: %w", ep.Method, ep.Path, err)
		}
	}

	return nil
}

// StoreServiceCalls stores normalized service calls for a repo (delete-then-insert).
func (s *SQLiteStore) StoreServiceCalls(ctx context.Context, repoID string, calls []ServiceCall) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := s.storeServiceCallsTx(ctx, tx, repoID, calls); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SQLiteStore) storeServiceCallsTx(ctx context.Context, tx *sql.Tx, repoID string, calls []ServiceCall) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM service_calls WHERE repo_id = ?", repoID); err != nil {
		return fmt.Errorf("failed to delete existing service calls: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO service_calls (repo_id, file_path, function_name, call_type, method, target, target_expression, service_hint, line)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare service call insert: %w", err)
	}
	defer stmt.Close()

	for _, sc := range calls {
		if _, err := stmt.ExecContext(ctx, repoID, sc.FilePath, sc.FunctionName, sc.CallType, sc.Method, sc.Target, sc.TargetExpression, sc.ServiceHint, sc.Line); err != nil {
			return fmt.Errorf("failed to insert service call %s %s: %w", sc.CallType, sc.Target, err)
		}
	}

	return nil
}

// GetEndpoints retrieves all endpoints for a repo.
func (s *SQLiteStore) GetEndpoints(ctx context.Context, repoID string) ([]Endpoint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repo_id, file_path, handler_name, method, path, raw_path, framework, line
		FROM endpoints WHERE repo_id = ? ORDER BY path, method`, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to query endpoints: %w", err)
	}
	defer rows.Close()

	var results []Endpoint
	for rows.Next() {
		var ep Endpoint
		if err := rows.Scan(&ep.ID, &ep.RepoID, &ep.FilePath, &ep.HandlerName, &ep.Method, &ep.Path, &ep.RawPath, &ep.Framework, &ep.Line); err != nil {
			return nil, fmt.Errorf("failed to scan endpoint: %w", err)
		}
		results = append(results, ep)
	}
	return results, rows.Err()
}

// GetServiceCalls retrieves all service calls for a repo.
func (s *SQLiteStore) GetServiceCalls(ctx context.Context, repoID string) ([]ServiceCall, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repo_id, file_path, function_name, call_type, method, target, target_expression, service_hint, line
		FROM service_calls WHERE repo_id = ? ORDER BY call_type, target`, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to query service calls: %w", err)
	}
	defer rows.Close()

	var results []ServiceCall
	for rows.Next() {
		var sc ServiceCall
		if err := rows.Scan(&sc.ID, &sc.RepoID, &sc.FilePath, &sc.FunctionName, &sc.CallType, &sc.Method, &sc.Target, &sc.TargetExpression, &sc.ServiceHint, &sc.Line); err != nil {
			return nil, fmt.Errorf("failed to scan service call: %w", err)
		}
		results = append(results, sc)
	}
	return results, rows.Err()
}

// FindMatchingEndpoints finds endpoints matching a method and path across all repos.
func (s *SQLiteStore) FindMatchingEndpoints(ctx context.Context, method, path string) ([]Endpoint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repo_id, file_path, handler_name, method, path, raw_path, framework, line
		FROM endpoints WHERE path = ? AND (method = ? OR method = 'ANY')
		ORDER BY repo_id`, path, method)
	if err != nil {
		return nil, fmt.Errorf("failed to query matching endpoints: %w", err)
	}
	defer rows.Close()

	var results []Endpoint
	for rows.Next() {
		var ep Endpoint
		if err := rows.Scan(&ep.ID, &ep.RepoID, &ep.FilePath, &ep.HandlerName, &ep.Method, &ep.Path, &ep.RawPath, &ep.Framework, &ep.Line); err != nil {
			return nil, fmt.Errorf("failed to scan endpoint: %w", err)
		}
		results = append(results, ep)
	}
	return results, rows.Err()
}

// FindMatchingConsumers finds Kafka consumers by topic across all repos.
func (s *SQLiteStore) FindMatchingConsumers(ctx context.Context, topic string) ([]ServiceCall, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repo_id, file_path, function_name, call_type, method, target, target_expression, service_hint, line
		FROM service_calls WHERE call_type = 'kafka_consume' AND target = ?
		ORDER BY repo_id`, topic)
	if err != nil {
		return nil, fmt.Errorf("failed to query matching consumers: %w", err)
	}
	defer rows.Close()

	var results []ServiceCall
	for rows.Next() {
		var sc ServiceCall
		if err := rows.Scan(&sc.ID, &sc.RepoID, &sc.FilePath, &sc.FunctionName, &sc.CallType, &sc.Method, &sc.Target, &sc.TargetExpression, &sc.ServiceHint, &sc.Line); err != nil {
			return nil, fmt.Errorf("failed to scan service call: %w", err)
		}
		results = append(results, sc)
	}
	return results, rows.Err()
}

// FindMatchingProducers finds Kafka producers by topic across all repos.
func (s *SQLiteStore) FindMatchingProducers(ctx context.Context, topic string) ([]ServiceCall, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, repo_id, file_path, function_name, call_type, method, target, target_expression, service_hint, line
		FROM service_calls WHERE call_type = 'kafka_produce' AND target = ?
		ORDER BY repo_id`, topic)
	if err != nil {
		return nil, fmt.Errorf("failed to query matching producers: %w", err)
	}
	defer rows.Close()

	var results []ServiceCall
	for rows.Next() {
		var sc ServiceCall
		if err := rows.Scan(&sc.ID, &sc.RepoID, &sc.FilePath, &sc.FunctionName, &sc.CallType, &sc.Method, &sc.Target, &sc.TargetExpression, &sc.ServiceHint, &sc.Line); err != nil {
			return nil, fmt.Errorf("failed to scan service call: %w", err)
		}
		results = append(results, sc)
	}
	return results, rows.Err()
}

// storeEndpointIndex populates the normalized endpoint and service_calls tables
// from the repo context data. Called within StoreRepoContext transaction.
func (s *SQLiteStore) storeEndpointIndex(ctx context.Context, tx *sql.Tx, repoID string, repoCtx *ctxpkg.RepoContext) error {
	var endpoints []Endpoint
	var serviceCalls []ServiceCall

	for filePath, fileCtx := range repoCtx.Files {
		// Collect routes → endpoints
		for _, route := range fileCtx.Routes {
			endpoints = append(endpoints, Endpoint{
				RepoID:      repoID,
				FilePath:    filePath,
				HandlerName: route.Handler,
				Method:      route.Method,
				Path:        route.Path,
				RawPath:     route.RawPath,
				Framework:   route.Framework,
				Line:        route.Line,
			})
		}

		// Collect external calls and async calls → service calls
		for _, fn := range fileCtx.Functions {
			if fn.APIFlow != nil {
				for _, ec := range fn.APIFlow.ExternalCalls {
					callType := "http"
					if ec.Method == "gRPC" {
						callType = "grpc"
					}
					serviceCalls = append(serviceCalls, ServiceCall{
						RepoID:           repoID,
						FilePath:         filePath,
						FunctionName:     fn.Name,
						CallType:         callType,
						Method:           ec.Method,
						Target:           ec.URL,
						TargetExpression: ec.URLExpression,
						ServiceHint:      ec.ServiceHint,
						Line:             ec.Line,
					})
				}
			}

			for _, ac := range fn.AsyncCalls {
				callType := "kafka_" + ac.Direction
				serviceCalls = append(serviceCalls, ServiceCall{
					RepoID:       repoID,
					FilePath:     filePath,
					FunctionName: fn.Name,
					CallType:     callType,
					Method:       ac.Direction,
					Target:       ac.Topic,
					Line:         ac.Line,
				})
			}
		}
	}

	if err := s.storeEndpointsTx(ctx, tx, repoID, endpoints); err != nil {
		return err
	}
	if err := s.storeServiceCallsTx(ctx, tx, repoID, serviceCalls); err != nil {
		return err
	}

	return nil
}
