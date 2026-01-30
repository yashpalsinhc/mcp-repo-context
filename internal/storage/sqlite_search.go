package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// SearchFunctions searches for functions using LIKE queries.
func (s *SQLiteStore) SearchFunctions(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error) {
	// Prepare search pattern
	searchPattern := "%" + strings.ToLower(query) + "%"

	rows, err := s.db.QueryContext(ctx, `
		SELECT f.name, f.signature, fi.path, f.line_start,
		       CASE WHEN f.behavior_json IS NOT NULL
		            THEN json_extract(f.behavior_json, '$.summary')
		            ELSE '' END as summary
		FROM functions f
		JOIN files fi ON fi.id = f.file_id
		WHERE (f.name_lower LIKE ? OR f.signature LIKE ? OR f.description LIKE ?)
		AND fi.repo_id = ?
		ORDER BY
		    CASE WHEN f.name_lower LIKE ? THEN 0 ELSE 1 END,
		    f.name
		LIMIT 50`, searchPattern, searchPattern, searchPattern, repoID, searchPattern)

	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()

	return scanFunctionRefs(rows)
}

// SearchBySideEffect finds functions with specific side effects.
func (s *SQLiteStore) SearchBySideEffect(ctx context.Context, repoID, effect string) ([]ctxpkg.FunctionRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.name, f.signature, fi.path, f.line_start,
		       CASE WHEN f.behavior_json IS NOT NULL
		            THEN json_extract(f.behavior_json, '$.summary')
		            ELSE '' END as summary
		FROM side_effects se
		JOIN functions f ON f.id = se.function_id
		JOIN files fi ON fi.id = f.file_id
		WHERE se.effect = ?
		AND fi.repo_id = ?
		ORDER BY fi.path, f.line_start
		LIMIT 100`, strings.ToLower(effect), repoID)

	if err != nil {
		return nil, fmt.Errorf("side effect search failed: %w", err)
	}
	defer rows.Close()

	return scanFunctionRefs(rows)
}

// SearchByConcept finds functions related to a concept.
func (s *SQLiteStore) SearchByConcept(ctx context.Context, repoID, concept string) ([]ctxpkg.FunctionRef, error) {
	// Map common search terms to concept names
	conceptMapping := map[string]string{
		"auth":     "authentication",
		"login":    "authentication",
		"validate": "validation",
		"db":       "database",
		"sql":      "database",
		"api":      "handler",
		"endpoint": "handler",
		"route":    "handler",
	}

	searchConcept := strings.ToLower(concept)
	if mapped, ok := conceptMapping[searchConcept]; ok {
		searchConcept = mapped
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT f.name, f.signature, fi.path, f.line_start,
		       CASE WHEN f.behavior_json IS NOT NULL
		            THEN json_extract(f.behavior_json, '$.summary')
		            ELSE '' END as summary
		FROM concepts c
		JOIN functions f ON f.id = c.function_id
		JOIN files fi ON fi.id = f.file_id
		WHERE c.concept = ?
		AND fi.repo_id = ?
		ORDER BY fi.path, f.line_start
		LIMIT 100`, searchConcept, repoID)

	if err != nil {
		return nil, fmt.Errorf("concept search failed: %w", err)
	}
	defer rows.Close()

	return scanFunctionRefs(rows)
}

// SearchFiles searches for files using LIKE queries.
func (s *SQLiteStore) SearchFiles(ctx context.Context, repoID, query string) ([]FileSearchResult, error) {
	searchPattern := "%" + strings.ToLower(query) + "%"

	rows, err := s.db.QueryContext(ctx, `
		SELECT fi.path, fi.language, fi.purpose, fi.line_count
		FROM files fi
		WHERE (LOWER(fi.path) LIKE ? OR LOWER(fi.purpose) LIKE ? OR LOWER(fi.package) LIKE ?)
		AND fi.repo_id = ?
		ORDER BY fi.path
		LIMIT 50`, searchPattern, searchPattern, searchPattern, repoID)

	if err != nil {
		return nil, fmt.Errorf("file search failed: %w", err)
	}
	defer rows.Close()

	var results []FileSearchResult
	for rows.Next() {
		var r FileSearchResult
		if err := rows.Scan(&r.Path, &r.Language, &r.Purpose, &r.LineCount); err != nil {
			return nil, err
		}
		results = append(results, r)
	}

	return results, nil
}

// FileSearchResult represents a file search result.
type FileSearchResult struct {
	Path      string `json:"path"`
	Language  string `json:"language"`
	Purpose   string `json:"purpose"`
	LineCount int    `json:"line_count"`
}

// SearchTypes searches for types using LIKE queries.
func (s *SQLiteStore) SearchTypes(ctx context.Context, repoID, query string) ([]TypeSearchResult, error) {
	searchPattern := "%" + strings.ToLower(query) + "%"

	rows, err := s.db.QueryContext(ctx, `
		SELECT t.name, t.kind, t.description, fi.path, t.line_start
		FROM types t
		JOIN files fi ON fi.id = t.file_id
		WHERE (t.name_lower LIKE ? OR t.description LIKE ?)
		AND fi.repo_id = ?
		ORDER BY t.name
		LIMIT 50`, searchPattern, searchPattern, repoID)

	if err != nil {
		return nil, fmt.Errorf("type search failed: %w", err)
	}
	defer rows.Close()

	var results []TypeSearchResult
	for rows.Next() {
		var r TypeSearchResult
		var desc sql.NullString
		if err := rows.Scan(&r.Name, &r.Kind, &desc, &r.File, &r.Line); err != nil {
			return nil, err
		}
		if desc.Valid {
			r.Description = desc.String
		}
		results = append(results, r)
	}

	return results, nil
}

// TypeSearchResult represents a type search result.
type TypeSearchResult struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description,omitempty"`
	File        string `json:"file"`
	Line        int    `json:"line"`
}

// GetFunctionCallers finds all functions that call the given function.
func (s *SQLiteStore) GetFunctionCallers(ctx context.Context, repoID, funcName string) ([]ctxpkg.FunctionRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT f.name, f.signature, fi.path, f.line_start,
		       CASE WHEN f.behavior_json IS NOT NULL
		            THEN json_extract(f.behavior_json, '$.summary')
		            ELSE '' END as summary
		FROM function_calls fc
		JOIN functions f ON f.id = fc.caller_id
		JOIN files fi ON fi.id = f.file_id
		WHERE fc.callee_name = ?
		AND fi.repo_id = ?
		ORDER BY fi.path, f.line_start`, funcName, repoID)

	if err != nil {
		return nil, fmt.Errorf("callers search failed: %w", err)
	}
	defer rows.Close()

	return scanFunctionRefs(rows)
}

// GetFunctionCallees finds all functions called by the given function.
func (s *SQLiteStore) GetFunctionCallees(ctx context.Context, repoID, filePath, funcName string) ([]ctxpkg.CallRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT fc.callee_name, fc.callee_package, fc.callee_file, fc.line, fc.call_type
		FROM function_calls fc
		JOIN functions f ON f.id = fc.caller_id
		JOIN files fi ON fi.id = f.file_id
		WHERE f.name = ?
		AND fi.path = ?
		AND fi.repo_id = ?
		ORDER BY fc.line`, funcName, filePath, repoID)

	if err != nil {
		return nil, fmt.Errorf("callees search failed: %w", err)
	}
	defer rows.Close()

	var results []ctxpkg.CallRef
	for rows.Next() {
		var call ctxpkg.CallRef
		var pkg, file, callType sql.NullString
		var line sql.NullInt64

		if err := rows.Scan(&call.Function, &pkg, &file, &line, &callType); err != nil {
			return nil, err
		}

		if pkg.Valid {
			call.Package = pkg.String
		}
		if file.Valid {
			call.File = file.String
		}
		if line.Valid {
			call.Line = int(line.Int64)
		}
		if callType.Valid {
			call.Type = callType.String
		}

		results = append(results, call)
	}

	return results, nil
}

// GetRepoStats returns statistics for a repository.
func (s *SQLiteStore) GetRepoStats(ctx context.Context, repoID string) (*RepoStats, error) {
	var stats RepoStats
	stats.RepoID = repoID

	// Get basic counts
	err := s.db.QueryRowContext(ctx, `
		SELECT file_count, total_lines, function_count, type_count
		FROM repos WHERE id = ?`, repoID,
	).Scan(&stats.FileCount, &stats.TotalLines, &stats.FunctionCount, &stats.TypeCount)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Get language breakdown
	rows, err := s.db.QueryContext(ctx, `
		SELECT language, COUNT(*) as count
		FROM files WHERE repo_id = ?
		GROUP BY language
		ORDER BY count DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats.Languages = make(map[string]int)
	for rows.Next() {
		var lang string
		var count int
		if err := rows.Scan(&lang, &count); err != nil {
			return nil, err
		}
		stats.Languages[lang] = count
	}

	// Get side effect counts
	rows, err = s.db.QueryContext(ctx, `
		SELECT se.effect, COUNT(*) as count
		FROM side_effects se
		JOIN functions f ON f.id = se.function_id
		JOIN files fi ON fi.id = f.file_id
		WHERE fi.repo_id = ?
		GROUP BY se.effect
		ORDER BY count DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats.SideEffects = make(map[string]int)
	for rows.Next() {
		var effect string
		var count int
		if err := rows.Scan(&effect, &count); err != nil {
			return nil, err
		}
		stats.SideEffects[effect] = count
	}

	return &stats, nil
}

// RepoStats holds repository statistics.
type RepoStats struct {
	RepoID        string         `json:"repo_id"`
	FileCount     int            `json:"file_count"`
	TotalLines    int            `json:"total_lines"`
	FunctionCount int            `json:"function_count"`
	TypeCount     int            `json:"type_count"`
	Languages     map[string]int `json:"languages"`
	SideEffects   map[string]int `json:"side_effects"`
}

// HybridSearch performs combined keyword + concept search.
func (s *SQLiteStore) HybridSearch(ctx context.Context, repoID, query string) ([]ctxpkg.FunctionRef, error) {
	// 1. FTS5 keyword search
	keywordResults, err := s.SearchFunctions(ctx, repoID, query)
	if err != nil {
		return nil, err
	}

	// 2. Extract concepts from query
	concepts := extractQueryConcepts(query)

	// 3. Concept search
	var conceptResults []ctxpkg.FunctionRef
	for _, concept := range concepts {
		results, err := s.SearchByConcept(ctx, repoID, concept)
		if err != nil {
			continue
		}
		conceptResults = append(conceptResults, results...)
	}

	// 4. Merge with deduplication
	return mergeResults(keywordResults, conceptResults), nil
}

// Helper functions

func scanFunctionRefs(rows *sql.Rows) ([]ctxpkg.FunctionRef, error) {
	var results []ctxpkg.FunctionRef
	for rows.Next() {
		var ref ctxpkg.FunctionRef
		var summary sql.NullString
		if err := rows.Scan(&ref.Function, &ref.Signature, &ref.File, &ref.Line, &summary); err != nil {
			return nil, err
		}
		if summary.Valid {
			ref.Summary = summary.String
		}
		results = append(results, ref)
	}
	return results, nil
}


func extractQueryConcepts(query string) []string {
	query = strings.ToLower(query)
	concepts := make(map[string]bool)

	conceptKeywords := map[string]string{
		"auth":       "authentication",
		"login":      "authentication",
		"logout":     "authentication",
		"validate":   "validation",
		"check":      "validation",
		"verify":     "validation",
		"database":   "database",
		"db":         "database",
		"sql":        "database",
		"query":      "database",
		"http":       "http",
		"api":        "handler",
		"endpoint":   "handler",
		"handler":    "handler",
		"error":      "error",
		"exception":  "error",
		"cache":      "cache",
		"store":      "cache",
		"create":     "crud",
		"read":       "crud",
		"update":     "crud",
		"delete":     "crud",
		"get":        "crud",
		"set":        "crud",
		"list":       "crud",
		"find":       "crud",
		"middleware": "middleware",
		"intercept":  "middleware",
	}

	for keyword, concept := range conceptKeywords {
		if strings.Contains(query, keyword) {
			concepts[concept] = true
		}
	}

	result := make([]string, 0, len(concepts))
	for c := range concepts {
		result = append(result, c)
	}
	return result
}

func mergeResults(primary, secondary []ctxpkg.FunctionRef) []ctxpkg.FunctionRef {
	seen := make(map[string]bool)
	var merged []ctxpkg.FunctionRef

	// Add primary results first
	for _, ref := range primary {
		key := fmt.Sprintf("%s:%s:%d", ref.File, ref.Function, ref.Line)
		if !seen[key] {
			seen[key] = true
			merged = append(merged, ref)
		}
	}

	// Add secondary results that aren't duplicates
	for _, ref := range secondary {
		key := fmt.Sprintf("%s:%s:%d", ref.File, ref.Function, ref.Line)
		if !seen[key] {
			seen[key] = true
			merged = append(merged, ref)
		}
	}

	// Limit total results
	if len(merged) > 50 {
		merged = merged[:50]
	}

	return merged
}
