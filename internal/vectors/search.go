package vectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// SemanticSearch provides semantic search capabilities over code.
type SemanticSearch struct {
	embedder        Embedder
	store           *SQLiteVectorStore
	hashStore       FunctionHashStore
	vocabStore      OrgVocabularyStore
	embedderFactory func() Embedder // creates fresh embedder instances for org-scoped operations
}

// NewSemanticSearch creates a new semantic search service.
func NewSemanticSearch(embedder Embedder, store *SQLiteVectorStore) *SemanticSearch {
	return &SemanticSearch{
		embedder: embedder,
		store:    store,
	}
}

// SetEmbedderFactory sets a factory function for creating fresh embedder instances.
func (s *SemanticSearch) SetEmbedderFactory(factory func() Embedder) {
	s.embedderFactory = factory
}

// newEmbedder creates a fresh embedder using the factory, or returns the default.
func (s *SemanticSearch) newEmbedder() Embedder {
	if s.embedderFactory != nil {
		return s.embedderFactory()
	}
	return NewLocalEmbedder(DefaultConfig())
}

// IsAvailable returns true if semantic search is properly initialized.
func (s *SemanticSearch) IsAvailable() bool {
	return s != nil && s.store != nil
}

// Performance characteristics (brute-force cosine similarity with O(n log n) sort):
//   <1K vectors:   <10ms   - suitable for all use cases
//   1K-10K vectors: 10-100ms - suitable for interactive use
//   10K-50K vectors: 100ms-1s - acceptable for batch/async
//   >50K vectors:   >1s    - consider specialized vector DB
//
// Vectors are stored as JSON-encoded float64 arrays in SQLite.
// All similarity computations are done in-memory.

// IndexRepository indexes all functions and types from a repository.
func (s *SemanticSearch) IndexRepository(ctx context.Context, repo *ctxpkg.RepoContext) error {
	start := time.Now()
	defer func() {
		logTiming("IndexRepository", start,
			fmt.Sprintf("repo=%s", repo.ID),
			fmt.Sprintf("files=%d", len(repo.Files)),
		)
	}()

	if repo == nil {
		return fmt.Errorf("repository context is nil")
	}

	// Collect all documents for vocabulary building
	var documents []string

	for _, fc := range repo.Files {
		for _, fn := range fc.Functions {
			doc := buildFunctionDocument(fn, fc.Path)
			documents = append(documents, doc)
		}
		for _, t := range fc.Types {
			doc := buildTypeDocument(t, fc.Path)
			documents = append(documents, doc)
		}
	}

	// Build vocabulary from all documents
	if localEmb, ok := s.embedder.(*LocalEmbedder); ok {
		localEmb.BuildVocabulary(documents)
	}

	// Generate and store embeddings
	var records []VectorRecord

	for path, fc := range repo.Files {
		for _, fn := range fc.Functions {
			doc := buildFunctionDocument(fn, path)
			vec := s.embedder.Embed(doc)

			record := VectorRecord{
				ID:       fmt.Sprintf("%s:func:%s:%s", repo.ID, path, fn.Name),
				RepoID:   repo.ID,
				Type:     "function",
				Name:     fn.Name,
				FilePath: path,
				Vector:   vec,
				Metadata: map[string]string{
					"signature":   fn.Signature,
					"description": fn.Description,
					"is_public":   fmt.Sprintf("%v", fn.IsPublic),
				},
			}

			if fn.Behavior != nil && fn.Behavior.Summary != "" {
				record.Metadata["summary"] = fn.Behavior.Summary
			}

			records = append(records, record)
		}

		for _, t := range fc.Types {
			doc := buildTypeDocument(t, path)
			vec := s.embedder.Embed(doc)

			record := VectorRecord{
				ID:       fmt.Sprintf("%s:type:%s:%s", repo.ID, path, t.Name),
				RepoID:   repo.ID,
				Type:     "type",
				Name:     t.Name,
				FilePath: path,
				Vector:   vec,
				Metadata: map[string]string{
					"kind":        t.Kind,
					"description": t.Description,
					"is_public":   fmt.Sprintf("%v", t.IsPublic),
				},
			}

			records = append(records, record)
		}
	}

	// Store all records
	return s.store.StoreBatch(ctx, records)
}

// SearchFunctions searches for functions similar to the query.
func (s *SemanticSearch) SearchFunctions(ctx context.Context, query string, repoID string, limit int) ([]FunctionSearchResult, error) {
	start := time.Now()
	defer func() {
		logTiming("SearchFunctions", start,
			fmt.Sprintf("repo=%s", repoID),
			fmt.Sprintf("query=%q", truncateQuery(query, 50)),
		)
	}()

	queryVec := s.embedder.Embed(query)

	results, err := s.store.SearchByType(ctx, queryVec, repoID, "function", limit)
	if err != nil {
		return nil, err
	}

	var functionResults []FunctionSearchResult
	for _, r := range results {
		functionResults = append(functionResults, FunctionSearchResult{
			Name:       r.Record.Name,
			FilePath:   r.Record.FilePath,
			Signature:  r.Record.Metadata["signature"],
			Summary:    r.Record.Metadata["summary"],
			Similarity: r.Similarity,
		})
	}

	return functionResults, nil
}

// SearchTypes searches for types similar to the query.
func (s *SemanticSearch) SearchTypes(ctx context.Context, query string, repoID string, limit int) ([]TypeSearchResult, error) {
	queryVec := s.embedder.Embed(query)

	results, err := s.store.SearchByType(ctx, queryVec, repoID, "type", limit)
	if err != nil {
		return nil, err
	}

	var typeResults []TypeSearchResult
	for _, r := range results {
		typeResults = append(typeResults, TypeSearchResult{
			Name:        r.Record.Name,
			FilePath:    r.Record.FilePath,
			Kind:        r.Record.Metadata["kind"],
			Description: r.Record.Metadata["description"],
			Similarity:  r.Similarity,
		})
	}

	return typeResults, nil
}

// SearchAll searches across all indexed items.
func (s *SemanticSearch) SearchAll(ctx context.Context, query string, repoID string, limit int) ([]SearchResult, error) {
	queryVec := s.embedder.Embed(query)
	return s.store.Search(ctx, queryVec, repoID, limit)
}

// FunctionSearchResult represents a function search result.
type FunctionSearchResult struct {
	Name       string  `json:"name"`
	FilePath   string  `json:"file_path"`
	Signature  string  `json:"signature,omitempty"`
	Summary    string  `json:"summary,omitempty"`
	Similarity float64 `json:"similarity"`
}

// TypeSearchResult represents a type search result.
type TypeSearchResult struct {
	Name        string  `json:"name"`
	FilePath    string  `json:"file_path"`
	Kind        string  `json:"kind,omitempty"`
	Description string  `json:"description,omitempty"`
	Similarity  float64 `json:"similarity"`
}

// buildFunctionDocument creates a searchable document from a function.
func buildFunctionDocument(fn ctxpkg.FunctionDef, filePath string) string {
	doc := fn.Name + " " + fn.Signature

	if fn.Description != "" {
		doc += " " + fn.Description
	}

	if fn.Behavior != nil {
		if fn.Behavior.Summary != "" {
			doc += " " + fn.Behavior.Summary
		}
		for _, step := range fn.Behavior.Steps {
			doc += " " + step
		}
	}

	// Add file path for context
	doc += " " + filePath

	// Add side effects
	for _, effect := range fn.SideEffects {
		doc += " " + effect
	}

	return doc
}

// buildTypeDocument creates a searchable document from a type.
func buildTypeDocument(t ctxpkg.TypeDef, filePath string) string {
	doc := t.Name + " " + t.Kind

	if t.Description != "" {
		doc += " " + t.Description
	}

	// Add field names
	for _, field := range t.Fields {
		doc += " " + field.Name + " " + field.Type
	}

	// Add file path
	doc += " " + filePath

	return doc
}

// CleanupStaleVectors removes vectors for functions that no longer exist in a file.
// currentFunctions is the set of function/type names currently present in the file.
// Returns the number of vectors removed.
func (s *SemanticSearch) CleanupStaleVectors(ctx context.Context, repoID, filePath string, currentFunctions map[string]bool) (int, error) {
	records, err := s.store.GetVectorsByFile(ctx, repoID, filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to get vectors for file: %w", err)
	}

	removed := 0
	for _, r := range records {
		if !currentFunctions[r.Name] {
			if err := s.store.Delete(ctx, r.ID); err != nil {
				return removed, fmt.Errorf("failed to delete stale vector %s: %w", r.ID, err)
			}
			removed++
		}
	}
	return removed, nil
}

// CleanupDeletedFileVectors removes all vectors and function hashes for a deleted file.
func (s *SemanticSearch) CleanupDeletedFileVectors(ctx context.Context, repoID, filePath string) error {
	if err := s.store.DeleteByFile(ctx, repoID, filePath); err != nil {
		return fmt.Errorf("failed to delete vectors for file %s: %w", filePath, err)
	}
	if s.hashStore != nil {
		if err := s.hashStore.DeleteFunctionHashes(ctx, repoID, filePath); err != nil {
			return fmt.Errorf("failed to delete function hashes for file %s: %w", filePath, err)
		}
	}
	return nil
}

// CleanupRepoFromOrg removes org-tagged vectors for a repo and marks vocabulary stale.
func (s *SemanticSearch) CleanupRepoFromOrg(ctx context.Context, orgID, repoID string) error {
	return s.store.DeleteByOrgAndRepo(ctx, orgID, repoID)
}

// ClearRepository removes all vectors for a repository.
func (s *SemanticSearch) ClearRepository(ctx context.Context, repoID string) error {
	return s.store.DeleteByRepo(ctx, repoID)
}

// Count returns the number of indexed items for a repository.
func (s *SemanticSearch) Count(ctx context.Context, repoID string) (int, error) {
	return s.store.Count(ctx, repoID)
}

// IndexRepositoryWithOrg indexes a repository and tags all vectors with org_id.
// When vocabStore is configured, it loads the org vocabulary and uses a fresh
// embedder with imported vocabulary. All vectors are tagged with vocab_version.
func (s *SemanticSearch) IndexRepositoryWithOrg(ctx context.Context, repo *ctxpkg.RepoContext, orgID string) error {
	if repo == nil {
		return fmt.Errorf("repository context is nil")
	}

	// Determine embedder and vocab version
	embedder := s.embedder
	vocabVersion := ""

	if orgID != "" && s.vocabStore != nil {
		rec, err := s.vocabStore.GetOrgVocabulary(ctx, orgID)
		if err != nil {
			return fmt.Errorf("failed to load org vocabulary: %w", err)
		}
		if rec != nil {
			var wordIDF map[string]float64
			if jsonErr := json.Unmarshal([]byte(rec.VocabularyJSON), &wordIDF); jsonErr == nil {
				localEmb := s.newEmbedder()
				vocabData := &VocabularyData{
					WordIDF:     wordIDF,
					DocCount:    rec.DocCount,
					VersionHash: rec.VersionHash,
				}
				if va, ok := localEmb.(VocabularyAwareEmbedder); ok {
					if importErr := va.ImportVocabulary(vocabData); importErr == nil {
						embedder = localEmb
						vocabVersion = rec.VersionHash
					}
				}
			}
		}
	}

	// If no org vocabulary was loaded, build per-repo vocabulary
	if vocabVersion == "" {
		var documents []string
		for _, fc := range repo.Files {
			for _, fn := range fc.Functions {
				documents = append(documents, buildFunctionDocument(fn, fc.Path))
			}
			for _, t := range fc.Types {
				documents = append(documents, buildTypeDocument(t, fc.Path))
			}
		}
		if localEmb, ok := embedder.(*LocalEmbedder); ok {
			localEmb.BuildVocabulary(documents)
		}
	}

	var records []VectorRecord

	for path, fc := range repo.Files {
		for _, fn := range fc.Functions {
			doc := buildFunctionDocument(fn, path)
			vec := embedder.Embed(doc)

			record := VectorRecord{
				ID:           fmt.Sprintf("%s:func:%s:%s", repo.ID, path, fn.Name),
				RepoID:       repo.ID,
				OrgID:        orgID,
				Type:         "function",
				Name:         fn.Name,
				FilePath:     path,
				Vector:       vec,
				VocabVersion: vocabVersion,
				Metadata: map[string]string{
					"signature":   fn.Signature,
					"description": fn.Description,
					"is_public":   fmt.Sprintf("%v", fn.IsPublic),
				},
			}
			if fn.Behavior != nil && fn.Behavior.Summary != "" {
				record.Metadata["summary"] = fn.Behavior.Summary
			}
			records = append(records, record)
		}

		for _, t := range fc.Types {
			doc := buildTypeDocument(t, path)
			vec := embedder.Embed(doc)
			record := VectorRecord{
				ID:           fmt.Sprintf("%s:type:%s:%s", repo.ID, path, t.Name),
				RepoID:       repo.ID,
				OrgID:        orgID,
				Type:         "type",
				Name:         t.Name,
				FilePath:     path,
				Vector:       vec,
				VocabVersion: vocabVersion,
				Metadata: map[string]string{
					"kind":        t.Kind,
					"description": t.Description,
					"is_public":   fmt.Sprintf("%v", t.IsPublic),
				},
			}
			records = append(records, record)
		}
	}

	return s.store.StoreBatch(ctx, records)
}

// SearchByOrg searches across all repos in an org.
// It loads the org vocabulary before embedding the query for consistent results.
func (s *SemanticSearch) SearchByOrg(ctx context.Context, query string, orgID string, limit int) ([]SearchResult, error) {
	embedder := s.embedder

	// Try to load org vocabulary for consistent query embedding
	if s.vocabStore != nil && orgID != "" {
		rec, err := s.vocabStore.GetOrgVocabulary(ctx, orgID)
		if err == nil && rec != nil {
			var wordIDF map[string]float64
			if jsonErr := json.Unmarshal([]byte(rec.VocabularyJSON), &wordIDF); jsonErr == nil {
				localEmb := s.newEmbedder()
				vocabData := &VocabularyData{
					WordIDF:     wordIDF,
					DocCount:    rec.DocCount,
					VersionHash: rec.VersionHash,
				}
				if va, ok := localEmb.(VocabularyAwareEmbedder); ok {
					if importErr := va.ImportVocabulary(vocabData); importErr == nil {
						embedder = localEmb
					}
				}
			}
		} else if err != nil {
			log.Printf("WARNING: failed to load org vocabulary for search: %v", err)
		}
	}

	queryVec := embedder.Embed(query)
	return s.store.SearchByOrg(ctx, queryVec, orgID, limit)
}

// CountByOrg returns the number of indexed items for an org.
func (s *SemanticSearch) CountByOrg(ctx context.Context, orgID string) (int, error) {
	return s.store.CountByOrg(ctx, orgID)
}

// ClearOrg removes all vectors for an org.
func (s *SemanticSearch) ClearOrg(ctx context.Context, orgID string) error {
	return s.store.DeleteByOrg(ctx, orgID)
}

// BuildOrgVocabulary builds a shared vocabulary from multiple repo contexts.
// It processes repos one at a time to conserve memory, builds vocabulary,
// exports it, and returns the VocabularyData.
func (s *SemanticSearch) BuildOrgVocabulary(repos []*ctxpkg.RepoContext) (*VocabularyData, error) {
	var allDocuments []string

	for _, repo := range repos {
		if repo == nil {
			continue
		}
		for _, fc := range repo.Files {
			for _, fn := range fc.Functions {
				allDocuments = append(allDocuments, buildFunctionDocument(fn, fc.Path))
			}
			for _, t := range fc.Types {
				allDocuments = append(allDocuments, buildTypeDocument(t, fc.Path))
			}
		}
	}

	if len(allDocuments) == 0 {
		return &VocabularyData{
			WordIDF:     make(map[string]float64),
			DocCount:    0,
			VersionHash: ComputeVersionHash(make(map[string]float64)),
		}, nil
	}

	// Build vocabulary on a fresh embedder to get clean state
	localEmb, ok := s.embedder.(*LocalEmbedder)
	if !ok {
		return nil, fmt.Errorf("BuildOrgVocabulary requires a LocalEmbedder")
	}

	localEmb.BuildVocabulary(allDocuments)
	return localEmb.ExportVocabulary(), nil
}
