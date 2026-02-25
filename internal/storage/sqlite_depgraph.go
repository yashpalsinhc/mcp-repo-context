package storage

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// migrateDependencyGraph applies migration 004 for dependency graph columns.
func (s *SQLiteStore) migrateDependencyGraph() error {
	var version int
	err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return fmt.Errorf("failed to check schema version: %w", err)
	}

	if version >= 4 {
		return nil
	}

	// Check and add columns idempotently using PRAGMA table_info
	repoColumns := map[string]bool{}
	rows, err := s.db.Query("PRAGMA table_info(repos)")
	if err != nil {
		return fmt.Errorf("failed to get repos table info: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue *string
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		repoColumns[name] = true
	}

	if !repoColumns["module_info_json"] {
		if _, err := s.db.Exec("ALTER TABLE repos ADD COLUMN module_info_json TEXT"); err != nil {
			return fmt.Errorf("failed to add module_info_json: %w", err)
		}
	}
	if !repoColumns["import_summary_json"] {
		if _, err := s.db.Exec("ALTER TABLE repos ADD COLUMN import_summary_json TEXT"); err != nil {
			return fmt.Errorf("failed to add import_summary_json: %w", err)
		}
	}

	fileColumns := map[string]bool{}
	rows2, err := s.db.Query("PRAGMA table_info(files)")
	if err != nil {
		return fmt.Errorf("failed to get files table info: %w", err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue *string
		if err := rows2.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		fileColumns[name] = true
	}

	if !fileColumns["content"] {
		if _, err := s.db.Exec("ALTER TABLE files ADD COLUMN content TEXT"); err != nil {
			return fmt.Errorf("failed to add content column: %w", err)
		}
	}
	if !fileColumns["structured_json"] {
		if _, err := s.db.Exec("ALTER TABLE files ADD COLUMN structured_json TEXT"); err != nil {
			return fmt.Errorf("failed to add structured_json: %w", err)
		}
	}
	if !fileColumns["config_type"] {
		if _, err := s.db.Exec("ALTER TABLE files ADD COLUMN config_type TEXT"); err != nil {
			return fmt.Errorf("failed to add config_type: %w", err)
		}
	}

	_, err = s.db.Exec("INSERT OR IGNORE INTO schema_migrations (version) VALUES (4)")
	if err != nil {
		return fmt.Errorf("failed to record migration version: %w", err)
	}

	return nil
}

// StoreModuleInfo persists ModuleInfo for a repo.
func (s *SQLiteStore) StoreModuleInfo(repoID string, info *ctxpkg.ModuleInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal module info: %w", err)
	}
	_, err = s.db.Exec("UPDATE repos SET module_info_json = ? WHERE id = ?", string(data), repoID)
	return err
}

// GetModuleInfo retrieves ModuleInfo for a repo.
func (s *SQLiteStore) GetModuleInfo(repoID string) (*ctxpkg.ModuleInfo, error) {
	var data *string
	err := s.db.QueryRow("SELECT module_info_json FROM repos WHERE id = ?", repoID).Scan(&data)
	if err != nil {
		return nil, err
	}
	if data == nil || *data == "" {
		return nil, nil
	}
	var info ctxpkg.ModuleInfo
	if err := json.Unmarshal([]byte(*data), &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal module info: %w", err)
	}
	return &info, nil
}

// GetModuleInfoBatch retrieves ModuleInfo for multiple repos in one query.
func (s *SQLiteStore) GetModuleInfoBatch(repoIDs []string) (map[string]*ctxpkg.ModuleInfo, error) {
	if len(repoIDs) == 0 {
		return map[string]*ctxpkg.ModuleInfo{}, nil
	}

	placeholders := make([]string, len(repoIDs))
	args := make([]interface{}, len(repoIDs))
	for i, id := range repoIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf("SELECT id, module_info_json FROM repos WHERE id IN (%s)", strings.Join(placeholders, ","))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*ctxpkg.ModuleInfo)
	for rows.Next() {
		var id string
		var data *string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		if data == nil || *data == "" {
			continue
		}
		var info ctxpkg.ModuleInfo
		if err := json.Unmarshal([]byte(*data), &info); err != nil {
			continue
		}
		result[id] = &info
	}

	return result, rows.Err()
}

// StoreImportSummary persists ImportSummary for a repo.
func (s *SQLiteStore) StoreImportSummary(repoID string, summary *ctxpkg.ImportSummary) error {
	data, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal import summary: %w", err)
	}
	_, err = s.db.Exec("UPDATE repos SET import_summary_json = ? WHERE id = ?", string(data), repoID)
	return err
}

// GetImportSummary retrieves ImportSummary for a repo.
func (s *SQLiteStore) GetImportSummary(repoID string) (*ctxpkg.ImportSummary, error) {
	var data *string
	err := s.db.QueryRow("SELECT import_summary_json FROM repos WHERE id = ?", repoID).Scan(&data)
	if err != nil {
		return nil, err
	}
	if data == nil || *data == "" {
		return nil, nil
	}
	var summary ctxpkg.ImportSummary
	if err := json.Unmarshal([]byte(*data), &summary); err != nil {
		return nil, fmt.Errorf("failed to unmarshal import summary: %w", err)
	}
	return &summary, nil
}

// isSensitiveFile checks if a file may contain secrets and should not store raw content.
func isSensitiveFile(filePath string) bool {
	base := filepath.Base(filePath)
	lower := strings.ToLower(base)

	// .env files
	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return true
	}

	// Key/certificate files
	if strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") {
		return true
	}

	// Files with sensitive names
	for _, word := range []string{"secret", "credential", "token"} {
		if strings.Contains(lower, word) {
			return true
		}
	}

	return false
}

// StoreConfigFileContent stores config file content and structured data.
func (s *SQLiteStore) StoreConfigFileContent(repoID, filePath string, content string, structuredJSON json.RawMessage, configType string) error {
	// Apply security blocklist
	storedContent := content
	if isSensitiveFile(filePath) {
		storedContent = ""
	}

	var structuredStr *string
	if structuredJSON != nil {
		s := string(structuredJSON)
		structuredStr = &s
	}

	_, err := s.db.Exec(
		"UPDATE files SET content = ?, structured_json = ?, config_type = ? WHERE repo_id = ? AND path = ?",
		storedContent, structuredStr, configType, repoID, filePath,
	)
	return err
}

// GetConfigFiles retrieves all config files for a repo.
func (s *SQLiteStore) GetConfigFiles(repoID string) ([]ctxpkg.ConfigFile, error) {
	rows, err := s.db.Query(
		"SELECT path, config_type, content, structured_json FROM files WHERE repo_id = ? AND config_type IS NOT NULL",
		repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []ctxpkg.ConfigFile
	for rows.Next() {
		var cf ctxpkg.ConfigFile
		var content, structured *string
		if err := rows.Scan(&cf.Path, &cf.Type, &content, &structured); err != nil {
			return nil, err
		}
		if content != nil {
			cf.Content = *content
		}
		if structured != nil {
			cf.StructuredJSON = json.RawMessage(*structured)
		}
		files = append(files, cf)
	}

	return files, rows.Err()
}
