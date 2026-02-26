package vectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
)

// VectorRecord represents a stored vector with metadata.
type VectorRecord struct {
	ID           string            `json:"id"`
	RepoID       string            `json:"repo_id"`
	OrgID        string            `json:"org_id,omitempty"`     // org ID when indexed via index_org
	Type         string            `json:"type"`                 // "function", "type", "file"
	Name         string            `json:"name"`
	FilePath     string            `json:"file_path,omitempty"`
	Vector       []float64         `json:"vector"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	VocabVersion string            `json:"vocab_version,omitempty"` // vocabulary version hash used for embedding
}

// VectorStore stores and retrieves vector embeddings.
type VectorStore interface {
	// Store saves a vector record.
	Store(ctx context.Context, record VectorRecord) error

	// StoreBatch saves multiple vector records.
	StoreBatch(ctx context.Context, records []VectorRecord) error

	// Get retrieves a vector by ID.
	Get(ctx context.Context, id string) (*VectorRecord, error)

	// Delete removes a vector by ID.
	Delete(ctx context.Context, id string) error

	// DeleteByRepo removes all vectors for a repository.
	DeleteByRepo(ctx context.Context, repoID string) error

	// DeleteByFile removes all vectors for a specific file.
	DeleteByFile(ctx context.Context, repoID, filePath string) error

	// Search finds similar vectors.
	Search(ctx context.Context, query []float64, repoID string, limit int) ([]SearchResult, error)

	// Count returns the number of vectors for a repository.
	Count(ctx context.Context, repoID string) (int, error)
}

// SearchResult represents a search result with similarity score.
type SearchResult struct {
	Record     VectorRecord `json:"record"`
	Similarity float64      `json:"similarity"`
}

// SQLiteVectorStore implements VectorStore using SQLite.
type SQLiteVectorStore struct {
	db        *sql.DB
	dimension int
	mu        sync.RWMutex
}

// NewSQLiteVectorStore creates a new SQLite-backed vector store.
func NewSQLiteVectorStore(dbPath string, dimension int) (*SQLiteVectorStore, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &SQLiteVectorStore{
		db:        db,
		dimension: dimension,
	}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// NewSQLiteVectorStoreWithDB creates a vector store with an existing DB connection.
func NewSQLiteVectorStoreWithDB(db *sql.DB, dimension int) (*SQLiteVectorStore, error) {
	store := &SQLiteVectorStore{
		db:        db,
		dimension: dimension,
	}

	if err := store.initSchema(); err != nil {
		return nil, err
	}

	return store, nil
}

// initSchema creates the vectors table if it doesn't exist.
func (s *SQLiteVectorStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS vectors (
		id TEXT PRIMARY KEY,
		repo_id TEXT NOT NULL,
		org_id TEXT NOT NULL DEFAULT '',
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		file_path TEXT,
		vector BLOB NOT NULL,
		metadata TEXT,
		vocab_version TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_vectors_repo ON vectors(repo_id);
	CREATE INDEX IF NOT EXISTS idx_vectors_org ON vectors(org_id);
	CREATE INDEX IF NOT EXISTS idx_vectors_type ON vectors(type);
	CREATE INDEX IF NOT EXISTS idx_vectors_name ON vectors(name);
	CREATE INDEX IF NOT EXISTS idx_vectors_file ON vectors(repo_id, file_path);

	CREATE TABLE IF NOT EXISTS vector_metadata (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS vocabularies (
		repo_id TEXT PRIMARY KEY,
		vocabulary_json TEXT NOT NULL,
		idf_json TEXT NOT NULL,
		version INTEGER DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// Migration: add org_id if missing (existing DBs)
	if _, err := s.db.Exec("SELECT org_id FROM vectors LIMIT 1"); err != nil {
		s.db.Exec("ALTER TABLE vectors ADD COLUMN org_id TEXT NOT NULL DEFAULT ''")
		s.db.Exec("CREATE INDEX IF NOT EXISTS idx_vectors_org ON vectors(org_id)")
	}

	// Migration: add vocab_version if missing (existing DBs)
	if _, err := s.db.Exec("SELECT vocab_version FROM vectors LIMIT 1"); err != nil {
		s.db.Exec("ALTER TABLE vectors ADD COLUMN vocab_version TEXT NOT NULL DEFAULT ''")
	}

	// Store dimension in metadata
	_, err := s.db.Exec(`INSERT OR REPLACE INTO vector_metadata(key, value) VALUES ('dimension', ?)`,
		fmt.Sprintf("%d", s.dimension))
	if err != nil {
		return fmt.Errorf("failed to store dimension metadata: %w", err)
	}

	// Run dimension migration
	if err := s.migrateDimension(s.dimension); err != nil {
		return fmt.Errorf("dimension migration failed: %w", err)
	}

	return nil
}

// detectStoredDimension checks the dimension of existing vectors in the store.
// Returns 0 if no vectors exist.
func (s *SQLiteVectorStore) detectStoredDimension() (int, error) {
	var vectorBytes []byte
	err := s.db.QueryRow("SELECT vector FROM vectors LIMIT 1").Scan(&vectorBytes)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var vec []float64
	if err := json.Unmarshal(vectorBytes, &vec); err != nil {
		return 0, fmt.Errorf("failed to unmarshal vector: %w", err)
	}
	return len(vec), nil
}

// migrateDimension detects stored vector dimensions and clears mismatched vectors.
func (s *SQLiteVectorStore) migrateDimension(expectedDim int) error {
	storedDim, err := s.detectStoredDimension()
	if err != nil {
		return err
	}
	if storedDim == 0 || storedDim == expectedDim {
		return nil
	}
	log.Printf("WARNING: Vector dimension mismatch (stored=%d, expected=%d). Clearing all vectors for re-indexing.", storedDim, expectedDim)
	_, err = s.db.Exec("DELETE FROM vectors")
	return err
}

// Store saves a vector record.
func (s *SQLiteVectorStore) Store(ctx context.Context, record VectorRecord) error {
	if len(record.Vector) != s.dimension {
		return fmt.Errorf("vector dimension %d does not match store dimension %d", len(record.Vector), s.dimension)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	vectorBytes, err := json.Marshal(record.Vector)
	if err != nil {
		return fmt.Errorf("failed to marshal vector: %w", err)
	}

	metadataBytes, err := json.Marshal(record.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	orgID := record.OrgID
	if orgID == "" {
		orgID = ""
	}
	query := `
	INSERT OR REPLACE INTO vectors (id, repo_id, org_id, type, name, file_path, vector, metadata, vocab_version)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		record.ID,
		record.RepoID,
		orgID,
		record.Type,
		record.Name,
		record.FilePath,
		vectorBytes,
		metadataBytes,
		record.VocabVersion,
	)

	return err
}

// StoreBatch saves multiple vector records.
func (s *SQLiteVectorStore) StoreBatch(ctx context.Context, records []VectorRecord) error {
	for _, r := range records {
		if len(r.Vector) != s.dimension {
			return fmt.Errorf("vector dimension %d does not match store dimension %d", len(r.Vector), s.dimension)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO vectors (id, repo_id, org_id, type, name, file_path, vector, metadata, vocab_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, record := range records {
		vectorBytes, err := json.Marshal(record.Vector)
		if err != nil {
			return fmt.Errorf("failed to marshal vector: %w", err)
		}

		metadataBytes, err := json.Marshal(record.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		orgID := record.OrgID
		_, err = stmt.ExecContext(ctx,
			record.ID,
			record.RepoID,
			orgID,
			record.Type,
			record.Name,
			record.FilePath,
			vectorBytes,
			metadataBytes,
			record.VocabVersion,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Get retrieves a vector by ID.
func (s *SQLiteVectorStore) Get(ctx context.Context, id string) (*VectorRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, repo_id, org_id, type, name, file_path, vector, metadata, vocab_version FROM vectors WHERE id = ?`

	var record VectorRecord
	var vectorBytes, metadataBytes []byte
	var filePath sql.NullString

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&record.ID,
		&record.RepoID,
		&record.OrgID,
		&record.Type,
		&record.Name,
		&filePath,
		&vectorBytes,
		&metadataBytes,
		&record.VocabVersion,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if filePath.Valid {
		record.FilePath = filePath.String
	}

	if err := json.Unmarshal(vectorBytes, &record.Vector); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vector: %w", err)
	}

	if len(metadataBytes) > 0 {
		if err := json.Unmarshal(metadataBytes, &record.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &record, nil
}

// Delete removes a vector by ID.
func (s *SQLiteVectorStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "DELETE FROM vectors WHERE id = ?", id)
	return err
}

// DeleteByRepo removes all vectors for a repository.
func (s *SQLiteVectorStore) DeleteByRepo(ctx context.Context, repoID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "DELETE FROM vectors WHERE repo_id = ?", repoID)
	return err
}

// DeleteByFile removes all vectors for a specific file in a repository.
func (s *SQLiteVectorStore) DeleteByFile(ctx context.Context, repoID, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "DELETE FROM vectors WHERE repo_id = ? AND file_path = ?", repoID, filePath)
	return err
}

// Search finds similar vectors using brute-force cosine similarity.
// For larger datasets, consider using a specialized vector database.
func (s *SQLiteVectorStore) Search(ctx context.Context, query []float64, repoID string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	// Load all vectors for the repo (brute force)
	sqlQuery := `SELECT id, repo_id, org_id, type, name, file_path, vector, metadata FROM vectors WHERE repo_id = ?`

	rows, err := s.db.QueryContext(ctx, sqlQuery, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult

	for rows.Next() {
		var record VectorRecord
		var vectorBytes, metadataBytes []byte
		var filePath sql.NullString

		err := rows.Scan(
			&record.ID,
			&record.RepoID,
			&record.OrgID,
			&record.Type,
			&record.Name,
			&filePath,
			&vectorBytes,
			&metadataBytes,
		)
		if err != nil {
			return nil, err
		}

		if filePath.Valid {
			record.FilePath = filePath.String
		}

		if err := json.Unmarshal(vectorBytes, &record.Vector); err != nil {
			continue // Skip invalid vectors
		}

		if len(metadataBytes) > 0 {
			json.Unmarshal(metadataBytes, &record.Metadata)
		}

		// Calculate cosine similarity
		similarity := CosineSimilarity(query, record.Vector)
		if similarity > 0 {
			results = append(results, SearchResult{
				Record:     record,
				Similarity: similarity,
			})
		}
	}

	// Sort by similarity descending
	sortBySimilarity(results)

	// Limit results
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// SearchByType searches only vectors of a specific type.
func (s *SQLiteVectorStore) SearchByType(ctx context.Context, query []float64, repoID, vectorType string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	sqlQuery := `SELECT id, repo_id, org_id, type, name, file_path, vector, metadata FROM vectors WHERE repo_id = ? AND type = ?`

	rows, err := s.db.QueryContext(ctx, sqlQuery, repoID, vectorType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult

	for rows.Next() {
		var record VectorRecord
		var vectorBytes, metadataBytes []byte
		var filePath sql.NullString

		err := rows.Scan(
			&record.ID,
			&record.RepoID,
			&record.OrgID,
			&record.Type,
			&record.Name,
			&filePath,
			&vectorBytes,
			&metadataBytes,
		)
		if err != nil {
			return nil, err
		}

		if filePath.Valid {
			record.FilePath = filePath.String
		}

		if err := json.Unmarshal(vectorBytes, &record.Vector); err != nil {
			continue
		}

		if len(metadataBytes) > 0 {
			json.Unmarshal(metadataBytes, &record.Metadata)
		}

		similarity := CosineSimilarity(query, record.Vector)
		if similarity > 0 {
			results = append(results, SearchResult{
				Record:     record,
				Similarity: similarity,
			})
		}
	}

	sortBySimilarity(results)

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// SearchByOrg finds similar vectors across all repos in an org.
func (s *SQLiteVectorStore) SearchByOrg(ctx context.Context, query []float64, orgID string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	sqlQuery := `SELECT id, repo_id, org_id, type, name, file_path, vector, metadata FROM vectors WHERE org_id = ?`

	rows, err := s.db.QueryContext(ctx, sqlQuery, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult

	for rows.Next() {
		var record VectorRecord
		var vectorBytes, metadataBytes []byte
		var filePath sql.NullString

		err := rows.Scan(
			&record.ID,
			&record.RepoID,
			&record.OrgID,
			&record.Type,
			&record.Name,
			&filePath,
			&vectorBytes,
			&metadataBytes,
		)
		if err != nil {
			return nil, err
		}

		if filePath.Valid {
			record.FilePath = filePath.String
		}

		if err := json.Unmarshal(vectorBytes, &record.Vector); err != nil {
			continue
		}

		if len(metadataBytes) > 0 {
			json.Unmarshal(metadataBytes, &record.Metadata)
		}

		similarity := CosineSimilarity(query, record.Vector)
		if similarity > 0 {
			results = append(results, SearchResult{
				Record:     record,
				Similarity: similarity,
			})
		}
	}

	sortBySimilarity(results)

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// DeleteByOrgAndRepo removes vectors for a specific repo within an org.
func (s *SQLiteVectorStore) DeleteByOrgAndRepo(ctx context.Context, orgID, repoID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "DELETE FROM vectors WHERE org_id = ? AND repo_id = ?", orgID, repoID)
	return err
}

// GetVectorsByFile returns all vector records for a specific file in a repo.
func (s *SQLiteVectorStore) GetVectorsByFile(ctx context.Context, repoID, filePath string) ([]VectorRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, repo_id, org_id, type, name, file_path, metadata FROM vectors WHERE repo_id = ? AND file_path = ?`,
		repoID, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []VectorRecord
	for rows.Next() {
		var r VectorRecord
		var fp sql.NullString
		var metadataBytes []byte
		if err := rows.Scan(&r.ID, &r.RepoID, &r.OrgID, &r.Type, &r.Name, &fp, &metadataBytes); err != nil {
			return nil, err
		}
		if fp.Valid {
			r.FilePath = fp.String
		}
		if len(metadataBytes) > 0 {
			json.Unmarshal(metadataBytes, &r.Metadata)
		}
		records = append(records, r)
	}
	return records, nil
}

// DeleteByOrg removes all vectors for an org.
func (s *SQLiteVectorStore) DeleteByOrg(ctx context.Context, orgID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "DELETE FROM vectors WHERE org_id = ?", orgID)
	return err
}

// CountByOrg returns the number of vectors for an org.
func (s *SQLiteVectorStore) CountByOrg(ctx context.Context, orgID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM vectors WHERE org_id = ?", orgID).Scan(&count)
	return count, err
}

// Count returns the number of vectors for a repository.
func (s *SQLiteVectorStore) Count(ctx context.Context, repoID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM vectors WHERE repo_id = ?", repoID).Scan(&count)
	return count, err
}

// Close closes the database connection.
func (s *SQLiteVectorStore) Close() error {
	return s.db.Close()
}

// sortBySimilarity sorts results by similarity in descending order.
func sortBySimilarity(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})
}

// Dimension returns the store's configured vector dimension.
func (s *SQLiteVectorStore) Dimension() int {
	return s.dimension
}

// StoreVocabulary persists vocabulary data for a repository.
func (s *SQLiteVectorStore) StoreVocabulary(ctx context.Context, repoID string, vocabJSON, idfJSON []byte, version int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO vocabularies(repo_id, vocabulary_json, idf_json, version, updated_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		repoID, string(vocabJSON), string(idfJSON), version)
	return err
}

// LoadVocabulary loads persisted vocabulary for a repository.
// Returns sql.ErrNoRows if not found.
func (s *SQLiteVectorStore) LoadVocabulary(ctx context.Context, repoID string) (vocabJSON, idfJSON []byte, version int, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var vJSON, iJSON string
	err = s.db.QueryRowContext(ctx,
		`SELECT vocabulary_json, idf_json, version FROM vocabularies WHERE repo_id = ?`,
		repoID).Scan(&vJSON, &iJSON, &version)
	if err != nil {
		return nil, nil, 0, err
	}
	return []byte(vJSON), []byte(iJSON), version, nil
}

// IncrementVocabularyVersion increments the vocabulary version for a repo.
func (s *SQLiteVectorStore) IncrementVocabularyVersion(ctx context.Context, repoID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var version int
	err := s.db.QueryRowContext(ctx,
		`UPDATE vocabularies SET version = version + 1, updated_at = CURRENT_TIMESTAMP
		 WHERE repo_id = ? RETURNING version`,
		repoID).Scan(&version)
	return version, err
}
