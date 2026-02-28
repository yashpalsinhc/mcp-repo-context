package vectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// cachedVocab holds a cached vocabulary for a repo.
type cachedVocab struct {
	vocabData *VocabularyData
	version   int
}

// SemanticSearch provides semantic search capabilities over code.
type SemanticSearch struct {
	embedder        Embedder
	store           *SQLiteVectorStore
	hashStore       FunctionHashStore
	vocabStore      OrgVocabularyStore
	embedderFactory func() Embedder // creates fresh embedder instances for org-scoped operations

	// Vocabulary cache per repo
	vocabCache   map[string]*cachedVocab
	vocabCacheMu sync.RWMutex
}

// NewSemanticSearch creates a new semantic search service.
func NewSemanticSearch(embedder Embedder, store *SQLiteVectorStore) *SemanticSearch {
	return &SemanticSearch{
		embedder:   embedder,
		store:      store,
		vocabCache: make(map[string]*cachedVocab),
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
	if err := s.store.StoreBatch(ctx, records); err != nil {
		return err
	}

	// Persist vocabulary for incremental updates and search
	if localEmb, ok := s.embedder.(*LocalEmbedder); ok {
		vocabData := localEmb.ExportVocabulary()
		vocabJSON, err := json.Marshal(vocabData.WordIDF)
		if err == nil {
			idfJSON, err2 := json.Marshal(vocabData.WordSlots)
			if err2 == nil {
				if storeErr := s.store.StoreVocabulary(ctx, repo.ID, vocabJSON, idfJSON, 0); storeErr != nil {
					log.Printf("Warning: failed to persist vocabulary for %s: %v", repo.ID, storeErr)
				}
			}
		}
		// Clear cache so next load picks up fresh vocabulary
		s.vocabCacheMu.Lock()
		delete(s.vocabCache, repo.ID)
		s.vocabCacheMu.Unlock()
	}

	return nil
}

// FileContext holds functions and types for a single file, used by RefreshFile.
type FileContext struct {
	Functions []ctxpkg.FunctionDef
	Types     []ctxpkg.TypeDef
}

// RefreshFile updates vectors for a single file using the repo's persisted vocabulary.
// It deletes all vectors for the file and re-generates them from the provided functions/types.
func (s *SemanticSearch) RefreshFile(ctx context.Context, repoID, filePath string, functions []ctxpkg.FunctionDef, types []ctxpkg.TypeDef) error {
	start := time.Now()
	defer func() {
		logTiming("RefreshFile", start,
			fmt.Sprintf("repo=%s", repoID),
			fmt.Sprintf("file=%s", filePath),
		)
	}()

	// Load vocabulary for this repo
	if err := s.ensureVocabulary(ctx, repoID); err != nil {
		log.Printf("Warning: vocabulary load failed for %s, using hash fallback: %v", repoID, err)
	}

	// Delete existing vectors for this file
	if err := s.store.DeleteByFile(ctx, repoID, filePath); err != nil {
		return fmt.Errorf("failed to delete vectors for file %s: %w", filePath, err)
	}

	// Generate new vectors
	var records []VectorRecord
	for _, fn := range functions {
		doc := buildFunctionDocument(fn, filePath)
		vec := s.embedder.Embed(doc)
		record := VectorRecord{
			ID:       fmt.Sprintf("%s:func:%s:%s", repoID, filePath, fn.Name),
			RepoID:   repoID,
			Type:     "function",
			Name:     fn.Name,
			FilePath: filePath,
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
	for _, t := range types {
		doc := buildTypeDocument(t, filePath)
		vec := s.embedder.Embed(doc)
		record := VectorRecord{
			ID:       fmt.Sprintf("%s:type:%s:%s", repoID, filePath, t.Name),
			RepoID:   repoID,
			Type:     "type",
			Name:     t.Name,
			FilePath: filePath,
			Vector:   vec,
			Metadata: map[string]string{
				"kind":        t.Kind,
				"description": t.Description,
				"is_public":   fmt.Sprintf("%v", t.IsPublic),
			},
		}
		records = append(records, record)
	}

	if len(records) > 0 {
		if err := s.store.StoreBatch(ctx, records); err != nil {
			return fmt.Errorf("failed to store vectors: %w", err)
		}
	}

	// Increment vocabulary version
	version, err := s.store.IncrementVocabularyVersion(ctx, repoID)
	if err != nil {
		// Not fatal, just log
		log.Printf("Warning: failed to increment vocabulary version for %s: %v", repoID, err)
	} else if version >= 50 {
		log.Printf("Warning: vocabulary for %s is stale (version=%d). Run index_repository(force=true) to rebuild.", repoID, version)
	}

	return nil
}

// RefreshFiles updates vectors for multiple files in a single batch.
func (s *SemanticSearch) RefreshFiles(ctx context.Context, repoID string, files map[string]FileContext) error {
	// Load vocabulary once for all files
	if err := s.ensureVocabulary(ctx, repoID); err != nil {
		log.Printf("Warning: vocabulary load failed for %s, using hash fallback: %v", repoID, err)
	}

	var allRecords []VectorRecord

	for filePath, fc := range files {
		// Delete existing vectors for this file
		if err := s.store.DeleteByFile(ctx, repoID, filePath); err != nil {
			return fmt.Errorf("failed to delete vectors for file %s: %w", filePath, err)
		}

		for _, fn := range fc.Functions {
			doc := buildFunctionDocument(fn, filePath)
			vec := s.embedder.Embed(doc)
			record := VectorRecord{
				ID:       fmt.Sprintf("%s:func:%s:%s", repoID, filePath, fn.Name),
				RepoID:   repoID,
				Type:     "function",
				Name:     fn.Name,
				FilePath: filePath,
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
			allRecords = append(allRecords, record)
		}
		for _, t := range fc.Types {
			doc := buildTypeDocument(t, filePath)
			vec := s.embedder.Embed(doc)
			record := VectorRecord{
				ID:       fmt.Sprintf("%s:type:%s:%s", repoID, filePath, t.Name),
				RepoID:   repoID,
				Type:     "type",
				Name:     t.Name,
				FilePath: filePath,
				Vector:   vec,
				Metadata: map[string]string{
					"kind":        t.Kind,
					"description": t.Description,
					"is_public":   fmt.Sprintf("%v", t.IsPublic),
				},
			}
			allRecords = append(allRecords, record)
		}
	}

	if len(allRecords) > 0 {
		if err := s.store.StoreBatch(ctx, allRecords); err != nil {
			return fmt.Errorf("failed to store vectors: %w", err)
		}
	}

	// Increment vocabulary version once
	version, err := s.store.IncrementVocabularyVersion(ctx, repoID)
	if err != nil {
		log.Printf("Warning: failed to increment vocabulary version for %s: %v", repoID, err)
	} else if version >= 50 {
		log.Printf("Warning: vocabulary for %s is stale (version=%d). Run index_repository(force=true) to rebuild.", repoID, version)
	}

	return nil
}

// ensureVocabulary loads the vocabulary for a repo into the embedder if not already loaded.
// This fixes the "vocabulary lost on server restart" bug.
func (s *SemanticSearch) ensureVocabulary(ctx context.Context, repoID string) error {
	// Check if embedder already has vocabulary
	if localEmb, ok := s.embedder.(*LocalEmbedder); ok {
		if localEmb.VocabularySize() > 0 {
			return nil
		}
	}

	return s.loadVocabularyForRepo(ctx, repoID)
}

// loadVocabularyForRepo loads vocabulary from cache or DB and imports it into the embedder.
func (s *SemanticSearch) loadVocabularyForRepo(ctx context.Context, repoID string) error {
	// Check cache first
	s.vocabCacheMu.RLock()
	cached, ok := s.vocabCache[repoID]
	s.vocabCacheMu.RUnlock()

	if ok && cached.vocabData != nil {
		if va, ok := s.embedder.(VocabularyAwareEmbedder); ok {
			return va.ImportVocabulary(cached.vocabData)
		}
		return nil
	}

	// Load from DB
	if s.store == nil {
		return fmt.Errorf("vector store not available")
	}

	vocabJSON, idfJSON, version, err := s.store.LoadVocabulary(ctx, repoID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no vocabulary found for repo %s", repoID)
		}
		return fmt.Errorf("failed to load vocabulary: %w", err)
	}

	// Parse vocabulary data
	var wordIDF map[string]float64
	if err := json.Unmarshal(vocabJSON, &wordIDF); err != nil {
		return fmt.Errorf("failed to unmarshal vocabulary: %w", err)
	}

	var wordSlots map[string]int
	if err := json.Unmarshal(idfJSON, &wordSlots); err != nil {
		// Fallback: idfJSON might not be word slots in older format
		wordSlots = nil
	}

	vocabData := &VocabularyData{
		WordIDF:     wordIDF,
		WordSlots:   wordSlots,
		DocCount:    len(wordIDF),
		VersionHash: ComputeVersionHash(wordIDF),
	}

	// Import into embedder
	if va, ok := s.embedder.(VocabularyAwareEmbedder); ok {
		if err := va.ImportVocabulary(vocabData); err != nil {
			return fmt.Errorf("failed to import vocabulary: %w", err)
		}
	}

	// Cache
	s.vocabCacheMu.Lock()
	s.vocabCache[repoID] = &cachedVocab{
		vocabData: vocabData,
		version:   version,
	}
	s.vocabCacheMu.Unlock()

	return nil
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

	// Ensure vocabulary is loaded for correct query embedding
	if err := s.ensureVocabulary(ctx, repoID); err != nil {
		log.Printf("Warning: vocabulary load failed for search on %s: %v", repoID, err)
	}

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
	// Ensure vocabulary is loaded for correct query embedding
	if err := s.ensureVocabulary(ctx, repoID); err != nil {
		log.Printf("Warning: vocabulary load failed for search on %s: %v", repoID, err)
	}

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
