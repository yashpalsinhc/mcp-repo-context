package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

//go:embed migrations/001_initial_schema.sql
var initialSchema string

//go:embed migrations/003_org_tables.sql
var orgTablesMigration string

// Ensure SQLiteStore implements ContextStore interface.
var _ ContextStore = (*SQLiteStore)(nil)

// SQLiteStore implements ContextStore using SQLite with FTS5.
type SQLiteStore struct {
	db   *sql.DB
	path string
}

// NewSQLiteStore creates a new SQLite-based store.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Open database with WAL mode for better concurrency
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=ON", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(0)

	store, err := NewSQLiteStoreWithDB(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	store.path = path

	return store, nil
}

// NewSQLiteStoreWithDB creates a store using a pre-opened *sql.DB.
// This enables sharing the same database connection between stores.
// The caller retains ownership of db and is responsible for closing it.
func NewSQLiteStoreWithDB(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db must not be nil")
	}

	// Ensure foreign keys are enabled regardless of how the DB was opened
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	store := &SQLiteStore{db: db}

	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return store, nil
}

// migrate applies database migrations.
func (s *SQLiteStore) migrate() error {
	// Run initial schema migration
	_, err := s.db.Exec(initialSchema)
	if err != nil {
		return fmt.Errorf("failed to apply initial schema: %w", err)
	}

	// Run file hashes migration (for incremental indexing support)
	if err := s.migrateFileHashes(); err != nil {
		return fmt.Errorf("failed to apply file hashes migration: %w", err)
	}

	// Run org tables migration
	if err := s.migrateOrgTables(); err != nil {
		return fmt.Errorf("failed to apply org tables migration: %w", err)
	}

	return nil
}

// migrateOrgTables applies the org tables migration.
func (s *SQLiteStore) migrateOrgTables() error {
	var version int
	err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return fmt.Errorf("failed to check schema version: %w", err)
	}

	if version >= 3 {
		return nil // Already migrated
	}

	_, err = s.db.Exec(orgTablesMigration)
	if err != nil {
		return fmt.Errorf("failed to run org tables migration: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// StoreRepoContext persists complete repository context.
func (s *SQLiteStore) StoreRepoContext(ctx context.Context, repoID string, repoCtx *ctxpkg.RepoContext) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete existing repo data (cascade will delete related records)
	if _, err := tx.ExecContext(ctx, "DELETE FROM repos WHERE id = ?", repoID); err != nil {
		return fmt.Errorf("failed to delete existing repo: %w", err)
	}

	// Insert repo
	statsJSON, _ := json.Marshal(repoCtx.Statistics)
	aiSummaryJSON, _ := json.Marshal(repoCtx.AISummary)
	archJSON, _ := json.Marshal(repoCtx.Architecture)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO repos (id, url, branch, commit_hash, analyzed_at, file_count, total_lines,
		                   function_count, type_count, stats_json, ai_summary_json, architecture_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		repoCtx.ID, repoCtx.URL, repoCtx.Branch, repoCtx.CommitHash, repoCtx.AnalyzedAt,
		repoCtx.Statistics.TotalFiles, repoCtx.Statistics.TotalLines,
		repoCtx.Statistics.FunctionCount, repoCtx.Statistics.TypeCount,
		string(statsJSON), string(aiSummaryJSON), string(archJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to insert repo: %w", err)
	}

	// Insert files and their contents
	for path, fileCtx := range repoCtx.Files {
		if err := ctx.Err(); err != nil {
			return err
		}

		fileID, err := s.insertFile(ctx, tx, repoID, path, fileCtx)
		if err != nil {
			return fmt.Errorf("failed to insert file %s: %w", path, err)
		}

		// Insert functions
		for _, fn := range fileCtx.Functions {
			if err := s.insertFunction(ctx, tx, fileID, &fn); err != nil {
				return fmt.Errorf("failed to insert function %s: %w", fn.Name, err)
			}
		}

		// Insert types
		for _, t := range fileCtx.Types {
			if err := s.insertType(ctx, tx, fileID, &t); err != nil {
				return fmt.Errorf("failed to insert type %s: %w", t.Name, err)
			}
		}

		// Insert constants
		for _, c := range fileCtx.Constants {
			if err := s.insertConstant(ctx, tx, fileID, &c); err != nil {
				return fmt.Errorf("failed to insert constant %s: %w", c.Name, err)
			}
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) insertFile(ctx context.Context, tx *sql.Tx, repoID, path string, fileCtx *ctxpkg.FileContext) (int64, error) {
	conceptsJSON, _ := json.Marshal(fileCtx.Concepts)
	importsJSON, _ := json.Marshal(fileCtx.Imports)
	routesJSON, _ := json.Marshal(fileCtx.Routes)

	result, err := tx.ExecContext(ctx, `
		INSERT INTO files (repo_id, path, hash, language, package, size, line_count, purpose,
		                   concepts_json, imports_json, routes_json, analyzed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		repoID, path, fileCtx.Hash, fileCtx.Language, fileCtx.Package, fileCtx.Size,
		fileCtx.LineCount, fileCtx.Purpose, string(conceptsJSON), string(importsJSON),
		string(routesJSON), fileCtx.AnalyzedAt,
	)
	if err != nil {
		return 0, err
	}

	fileID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Insert file concepts for fast lookup
	for _, concept := range fileCtx.Concepts {
		_, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO file_concepts (file_id, concept) VALUES (?, ?)`,
			fileID, strings.ToLower(concept),
		)
		if err != nil {
			return 0, err
		}
	}

	return fileID, nil
}

func (s *SQLiteStore) insertFunction(ctx context.Context, tx *sql.Tx, fileID int64, fn *ctxpkg.FunctionDef) error {
	paramsJSON, _ := json.Marshal(fn.Parameters)
	returnsJSON, _ := json.Marshal(fn.Returns)
	behaviorJSON, _ := json.Marshal(fn.Behavior)
	errorHandlingJSON, _ := json.Marshal(fn.ErrorHandling)
	apiFlowJSON, _ := json.Marshal(fn.APIFlow)
	usesImportsJSON, _ := json.Marshal(fn.UsesImports)

	result, err := tx.ExecContext(ctx, `
		INSERT INTO functions (file_id, name, name_lower, signature, description, receiver, is_public,
		                       line_start, line_end, complexity, parameters_json, returns_json,
		                       behavior_json, error_handling_json, api_flow_json, uses_imports_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fileID, fn.Name, strings.ToLower(fn.Name), fn.Signature, fn.Description, fn.Receiver, fn.IsPublic,
		fn.LineStart, fn.LineEnd, fn.Complexity, string(paramsJSON), string(returnsJSON),
		string(behaviorJSON), string(errorHandlingJSON), string(apiFlowJSON), string(usesImportsJSON),
	)
	if err != nil {
		return err
	}

	funcID, err := result.LastInsertId()
	if err != nil {
		return err
	}

	// Insert function calls
	for _, call := range fn.Calls {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO function_calls (caller_id, callee_name, callee_package, callee_file, line, call_type)
			VALUES (?, ?, ?, ?, ?, ?)`,
			funcID, call.Function, call.Package, call.File, call.Line, call.Type,
		)
		if err != nil {
			return err
		}
	}

	// Insert side effects
	for _, effect := range fn.SideEffects {
		_, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO side_effects (function_id, effect) VALUES (?, ?)`,
			funcID, strings.ToLower(effect),
		)
		if err != nil {
			return err
		}
	}

	// Extract and insert concepts from function name and behavior
	concepts := extractFunctionConcepts(fn)
	for _, concept := range concepts {
		_, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO concepts (function_id, concept) VALUES (?, ?)`,
			funcID, strings.ToLower(concept),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *SQLiteStore) insertType(ctx context.Context, tx *sql.Tx, fileID int64, t *ctxpkg.TypeDef) error {
	fieldsJSON, _ := json.Marshal(t.Fields)
	methodsJSON, _ := json.Marshal(t.Methods)
	implementsJSON, _ := json.Marshal(t.Implements)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO types (file_id, name, name_lower, kind, description, is_public, line_start, line_end,
		                   fields_json, methods_json, implements_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fileID, t.Name, strings.ToLower(t.Name), t.Kind, t.Description, t.IsPublic, t.LineStart, t.LineEnd,
		string(fieldsJSON), string(methodsJSON), string(implementsJSON),
	)
	return err
}

func (s *SQLiteStore) insertConstant(ctx context.Context, tx *sql.Tx, fileID int64, c *ctxpkg.ConstantDef) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO constants (file_id, name, type, value, is_public, line_start)
		VALUES (?, ?, ?, ?, ?, ?)`,
		fileID, c.Name, c.Type, c.Value, c.IsPublic, c.LineStart,
	)
	return err
}

// GetRepoContext retrieves full repository context.
func (s *SQLiteStore) GetRepoContext(ctx context.Context, repoID string) (*ctxpkg.RepoContext, error) {
	// Get repo metadata
	var repo ctxpkg.RepoContext
	var statsJSON, aiSummaryJSON, archJSON sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, url, branch, commit_hash, analyzed_at, stats_json, ai_summary_json, architecture_json
		FROM repos WHERE id = ?`, repoID,
	).Scan(&repo.ID, &repo.URL, &repo.Branch, &repo.CommitHash, &repo.AnalyzedAt,
		&statsJSON, &aiSummaryJSON, &archJSON)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get repo: %w", err)
	}

	// Parse JSON fields
	if statsJSON.Valid {
		json.Unmarshal([]byte(statsJSON.String), &repo.Statistics)
	}
	if aiSummaryJSON.Valid && aiSummaryJSON.String != "null" {
		repo.AISummary = &ctxpkg.AISummary{}
		json.Unmarshal([]byte(aiSummaryJSON.String), repo.AISummary)
	}
	if archJSON.Valid && archJSON.String != "null" {
		repo.Architecture = &ctxpkg.ArchitectureContext{}
		json.Unmarshal([]byte(archJSON.String), repo.Architecture)
	}

	// Get files
	repo.Files = make(map[string]*ctxpkg.FileContext)
	fileRows, err := s.db.QueryContext(ctx, `
		SELECT id, path, hash, language, package, size, line_count, purpose,
		       concepts_json, imports_json, routes_json, analyzed_at
		FROM files WHERE repo_id = ?`, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}
	defer fileRows.Close()

	fileIDs := make(map[int64]string) // fileID -> path

	for fileRows.Next() {
		var fileID int64
		var fileCtx ctxpkg.FileContext
		var conceptsJSON, importsJSON, routesJSON sql.NullString

		err := fileRows.Scan(&fileID, &fileCtx.Path, &fileCtx.Hash, &fileCtx.Language,
			&fileCtx.Package, &fileCtx.Size, &fileCtx.LineCount, &fileCtx.Purpose,
			&conceptsJSON, &importsJSON, &routesJSON, &fileCtx.AnalyzedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}

		if conceptsJSON.Valid {
			json.Unmarshal([]byte(conceptsJSON.String), &fileCtx.Concepts)
		}
		if importsJSON.Valid {
			json.Unmarshal([]byte(importsJSON.String), &fileCtx.Imports)
		}
		if routesJSON.Valid {
			json.Unmarshal([]byte(routesJSON.String), &fileCtx.Routes)
		}

		fileIDs[fileID] = fileCtx.Path
		repo.Files[fileCtx.Path] = &fileCtx
	}

	// Get functions for each file
	for fileID, path := range fileIDs {
		functions, err := s.getFunctionsForFile(ctx, fileID)
		if err != nil {
			return nil, fmt.Errorf("failed to get functions for %s: %w", path, err)
		}
		repo.Files[path].Functions = functions

		types, err := s.getTypesForFile(ctx, fileID)
		if err != nil {
			return nil, fmt.Errorf("failed to get types for %s: %w", path, err)
		}
		repo.Files[path].Types = types

		constants, err := s.getConstantsForFile(ctx, fileID)
		if err != nil {
			return nil, fmt.Errorf("failed to get constants for %s: %w", path, err)
		}
		repo.Files[path].Constants = constants
	}

	return &repo, nil
}

func (s *SQLiteStore) getFunctionsForFile(ctx context.Context, fileID int64) ([]ctxpkg.FunctionDef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, signature, description, receiver, is_public, line_start, line_end,
		       complexity, parameters_json, returns_json, behavior_json, error_handling_json,
		       api_flow_json, uses_imports_json
		FROM functions WHERE file_id = ?`, fileID)
	if err != nil {
		return nil, err
	}

	// Collect function data first, then close rows before making nested queries
	type funcData struct {
		fn     ctxpkg.FunctionDef
		funcID int64
	}
	var funcList []funcData

	for rows.Next() {
		var fd funcData
		var paramsJSON, returnsJSON, behaviorJSON, errorJSON, apiFlowJSON, usesImportsJSON sql.NullString

		err := rows.Scan(&fd.funcID, &fd.fn.Name, &fd.fn.Signature, &fd.fn.Description, &fd.fn.Receiver,
			&fd.fn.IsPublic, &fd.fn.LineStart, &fd.fn.LineEnd, &fd.fn.Complexity,
			&paramsJSON, &returnsJSON, &behaviorJSON, &errorJSON, &apiFlowJSON, &usesImportsJSON)
		if err != nil {
			rows.Close()
			return nil, err
		}

		if paramsJSON.Valid {
			json.Unmarshal([]byte(paramsJSON.String), &fd.fn.Parameters)
		}
		if returnsJSON.Valid {
			json.Unmarshal([]byte(returnsJSON.String), &fd.fn.Returns)
		}
		if behaviorJSON.Valid && behaviorJSON.String != "null" {
			fd.fn.Behavior = &ctxpkg.FunctionBehavior{}
			json.Unmarshal([]byte(behaviorJSON.String), fd.fn.Behavior)
		}
		if errorJSON.Valid && errorJSON.String != "null" {
			fd.fn.ErrorHandling = &ctxpkg.ErrorHandling{}
			json.Unmarshal([]byte(errorJSON.String), fd.fn.ErrorHandling)
		}
		if apiFlowJSON.Valid && apiFlowJSON.String != "null" {
			fd.fn.APIFlow = &ctxpkg.APIFlow{}
			json.Unmarshal([]byte(apiFlowJSON.String), fd.fn.APIFlow)
		}
		if usesImportsJSON.Valid {
			json.Unmarshal([]byte(usesImportsJSON.String), &fd.fn.UsesImports)
		}

		funcList = append(funcList, fd)
	}
	rows.Close() // Close rows BEFORE making nested queries

	// Now query nested data with rows closed
	functions := make([]ctxpkg.FunctionDef, 0, len(funcList))
	for _, fd := range funcList {
		fd.fn.Calls, _ = s.getFunctionCalls(ctx, fd.funcID)
		fd.fn.SideEffects, _ = s.getSideEffects(ctx, fd.funcID)
		functions = append(functions, fd.fn)
	}

	return functions, nil
}

func (s *SQLiteStore) getFunctionCalls(ctx context.Context, funcID int64) ([]ctxpkg.CallRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT callee_name, callee_package, callee_file, line, call_type
		FROM function_calls WHERE caller_id = ?`, funcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calls []ctxpkg.CallRef
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

		calls = append(calls, call)
	}

	return calls, nil
}

func (s *SQLiteStore) getSideEffects(ctx context.Context, funcID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT effect FROM side_effects WHERE function_id = ?`, funcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var effects []string
	for rows.Next() {
		var effect string
		if err := rows.Scan(&effect); err != nil {
			return nil, err
		}
		effects = append(effects, effect)
	}

	return effects, nil
}

func (s *SQLiteStore) getTypesForFile(ctx context.Context, fileID int64) ([]ctxpkg.TypeDef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, kind, description, is_public, line_start, line_end,
		       fields_json, methods_json, implements_json
		FROM types WHERE file_id = ?`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []ctxpkg.TypeDef
	for rows.Next() {
		var t ctxpkg.TypeDef
		var fieldsJSON, methodsJSON, implementsJSON sql.NullString

		err := rows.Scan(&t.Name, &t.Kind, &t.Description, &t.IsPublic,
			&t.LineStart, &t.LineEnd, &fieldsJSON, &methodsJSON, &implementsJSON)
		if err != nil {
			return nil, err
		}

		if fieldsJSON.Valid {
			json.Unmarshal([]byte(fieldsJSON.String), &t.Fields)
		}
		if methodsJSON.Valid {
			json.Unmarshal([]byte(methodsJSON.String), &t.Methods)
		}
		if implementsJSON.Valid {
			json.Unmarshal([]byte(implementsJSON.String), &t.Implements)
		}

		types = append(types, t)
	}

	return types, nil
}

func (s *SQLiteStore) getConstantsForFile(ctx context.Context, fileID int64) ([]ctxpkg.ConstantDef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, type, value, is_public, line_start
		FROM constants WHERE file_id = ?`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var constants []ctxpkg.ConstantDef
	for rows.Next() {
		var c ctxpkg.ConstantDef
		var typeName, value sql.NullString

		err := rows.Scan(&c.Name, &typeName, &value, &c.IsPublic, &c.LineStart)
		if err != nil {
			return nil, err
		}

		if typeName.Valid {
			c.Type = typeName.String
		}
		if value.Valid {
			c.Value = value.String
		}

		constants = append(constants, c)
	}

	return constants, nil
}

// GetFileContext retrieves context for a specific file.
func (s *SQLiteStore) GetFileContext(ctx context.Context, repoID, filePath string) (*ctxpkg.FileContext, error) {
	var fileID int64
	var fileCtx ctxpkg.FileContext
	var conceptsJSON, importsJSON, routesJSON sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, path, hash, language, package, size, line_count, purpose,
		       concepts_json, imports_json, routes_json, analyzed_at
		FROM files WHERE repo_id = ? AND path = ?`, repoID, filePath,
	).Scan(&fileID, &fileCtx.Path, &fileCtx.Hash, &fileCtx.Language,
		&fileCtx.Package, &fileCtx.Size, &fileCtx.LineCount, &fileCtx.Purpose,
		&conceptsJSON, &importsJSON, &routesJSON, &fileCtx.AnalyzedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if conceptsJSON.Valid {
		json.Unmarshal([]byte(conceptsJSON.String), &fileCtx.Concepts)
	}
	if importsJSON.Valid {
		json.Unmarshal([]byte(importsJSON.String), &fileCtx.Imports)
	}
	if routesJSON.Valid {
		json.Unmarshal([]byte(routesJSON.String), &fileCtx.Routes)
	}

	fileCtx.Functions, _ = s.getFunctionsForFile(ctx, fileID)
	fileCtx.Types, _ = s.getTypesForFile(ctx, fileID)
	fileCtx.Constants, _ = s.getConstantsForFile(ctx, fileID)

	return &fileCtx, nil
}

// ContextExists checks if context exists and is within maxAge.
func (s *SQLiteStore) ContextExists(ctx context.Context, repoID string, maxAge time.Duration) (bool, time.Time, error) {
	var analyzedAt time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT analyzed_at FROM repos WHERE id = ?`, repoID,
	).Scan(&analyzedAt)

	if err == sql.ErrNoRows {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}

	if maxAge > 0 && time.Since(analyzedAt) > maxAge {
		return false, analyzedAt, nil
	}

	return true, analyzedAt, nil
}

// DeleteContext removes all context for a repository.
func (s *SQLiteStore) DeleteContext(ctx context.Context, repoID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM repos WHERE id = ?", repoID)
	return err
}

// ListContexts returns metadata for all stored contexts.
func (s *SQLiteStore) ListContexts(ctx context.Context) ([]ctxpkg.ContextMetadata, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, url, branch, commit_hash, file_count, analyzed_at
		FROM repos ORDER BY analyzed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ctxpkg.ContextMetadata
	for rows.Next() {
		var meta ctxpkg.ContextMetadata
		err := rows.Scan(&meta.RepoID, &meta.URL, &meta.Branch, &meta.CommitHash,
			&meta.FileCount, &meta.AnalyzedAt)
		if err != nil {
			return nil, err
		}
		results = append(results, meta)
	}

	return results, nil
}

// extractFunctionConcepts extracts concepts from function definition.
func extractFunctionConcepts(fn *ctxpkg.FunctionDef) []string {
	concepts := make(map[string]bool)

	// Extract from function name
	name := strings.ToLower(fn.Name)
	conceptPatterns := map[string][]string{
		"authentication": {"auth", "login", "logout", "session", "token"},
		"validation":     {"valid", "check", "verify", "sanitize"},
		"handler":        {"handler", "handle", "serve", "endpoint"},
		"crud":           {"create", "read", "update", "delete", "get", "set", "list", "find"},
		"middleware":     {"middleware", "intercept", "filter"},
		"error":          {"error", "err", "fail", "panic"},
		"database":       {"db", "query", "insert", "select", "repository", "repo"},
		"http":           {"http", "request", "response", "api", "rest"},
		"cache":          {"cache", "memo", "store"},
		"config":         {"config", "setting", "option", "env"},
	}

	for concept, patterns := range conceptPatterns {
		for _, pattern := range patterns {
			if strings.Contains(name, pattern) {
				concepts[concept] = true
				break
			}
		}
	}

	// Add from behavior if available
	if fn.Behavior != nil {
		for _, pattern := range fn.Behavior.Patterns {
			concepts[strings.ToLower(pattern)] = true
		}
	}

	result := make([]string, 0, len(concepts))
	for c := range concepts {
		result = append(result, c)
	}
	return result
}
