package vectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
)

// VectorRecord represents a stored vector with metadata.
type VectorRecord struct {
	ID       string    `json:"id"`
	RepoID   string    `json:"repo_id"`
	Type     string    `json:"type"` // "function", "type", "file"
	Name     string    `json:"name"`
	FilePath string    `json:"file_path,omitempty"`
	Vector   []float64 `json:"vector"`
	Metadata map[string]string `json:"metadata,omitempty"`
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
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		file_path TEXT,
		vector BLOB NOT NULL,
		metadata TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_vectors_repo ON vectors(repo_id);
	CREATE INDEX IF NOT EXISTS idx_vectors_type ON vectors(type);
	CREATE INDEX IF NOT EXISTS idx_vectors_name ON vectors(name);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Store saves a vector record.
func (s *SQLiteVectorStore) Store(ctx context.Context, record VectorRecord) error {
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

	query := `
	INSERT OR REPLACE INTO vectors (id, repo_id, type, name, file_path, vector, metadata)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		record.ID,
		record.RepoID,
		record.Type,
		record.Name,
		record.FilePath,
		vectorBytes,
		metadataBytes,
	)

	return err
}

// StoreBatch saves multiple vector records.
func (s *SQLiteVectorStore) StoreBatch(ctx context.Context, records []VectorRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO vectors (id, repo_id, type, name, file_path, vector, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?)
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

		_, err = stmt.ExecContext(ctx,
			record.ID,
			record.RepoID,
			record.Type,
			record.Name,
			record.FilePath,
			vectorBytes,
			metadataBytes,
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

	query := `SELECT id, repo_id, type, name, file_path, vector, metadata FROM vectors WHERE id = ?`

	var record VectorRecord
	var vectorBytes, metadataBytes []byte
	var filePath sql.NullString

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&record.ID,
		&record.RepoID,
		&record.Type,
		&record.Name,
		&filePath,
		&vectorBytes,
		&metadataBytes,
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

// Search finds similar vectors using brute-force cosine similarity.
// For larger datasets, consider using a specialized vector database.
func (s *SQLiteVectorStore) Search(ctx context.Context, query []float64, repoID string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	// Load all vectors for the repo (brute force)
	sqlQuery := `SELECT id, repo_id, type, name, file_path, vector, metadata FROM vectors WHERE repo_id = ?`

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

	sqlQuery := `SELECT id, repo_id, type, name, file_path, vector, metadata FROM vectors WHERE repo_id = ? AND type = ?`

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
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Similarity > results[i].Similarity {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}
