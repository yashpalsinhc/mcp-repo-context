package vectors

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// FuncHashEntry mirrors storage.FunctionHashInfo for cross-package use.
type FuncHashEntry struct {
	Name        string
	Type        string // "function" or "type"
	ContentHash string
	VectorID    string
}

// FunctionHashStore provides function hash tracking for incremental updates.
type FunctionHashStore interface {
	GetFunctionHashes(ctx context.Context, repoID, filePath string) (map[string]FuncHashEntry, error)
	UpdateFunctionHashes(ctx context.Context, repoID, filePath string, hashes []FuncHashEntry) error
	DeleteFunctionHashes(ctx context.Context, repoID, filePath string) error
	GetChangedFunctions(ctx context.Context, repoID, filePath string, current map[string]string) (added, modified, removed []string, err error)
}

// OrgVocabularyStore provides org vocabulary operations for incremental updates.
type OrgVocabularyStore interface {
	GetOrgVocabulary(ctx context.Context, orgID string) (*OrgVocabRecord, error)
}

// OrgVocabRecord mirrors storage.OrgVocabularyRecord for cross-package use.
type OrgVocabRecord struct {
	OrgID          string
	VocabularyJSON string
	VersionHash    string
	DocCount       int
	IsStale        bool
}

// RefreshResult summarizes the outcome of an incremental vector refresh.
type RefreshResult struct {
	Added    int      `json:"added"`
	Modified int      `json:"modified"`
	Removed  int      `json:"removed"`
	Errors   []string `json:"errors,omitempty"`
}

// AggregateRefreshResult combines results from multiple file refreshes.
type AggregateRefreshResult struct {
	FilesProcessed int      `json:"files_processed"`
	TotalAdded     int      `json:"total_added"`
	TotalModified  int      `json:"total_modified"`
	TotalRemoved   int      `json:"total_removed"`
	Errors         []string `json:"errors,omitempty"`
}

// SetHashStore sets the function hash store for incremental updates.
func (s *SemanticSearch) SetHashStore(store FunctionHashStore) {
	s.hashStore = store
}

// SetVocabStore sets the org vocabulary store for incremental updates.
func (s *SemanticSearch) SetVocabStore(store OrgVocabularyStore) {
	s.vocabStore = store
}

// RefreshFileVectors incrementally updates vector embeddings for a single file.
func (s *SemanticSearch) RefreshFileVectors(ctx context.Context, repoID, filePath string, file *ctxpkg.FileContext, orgID string) (*RefreshResult, error) {
	if s.hashStore == nil {
		return nil, fmt.Errorf("function hash store not configured; incremental updates unavailable")
	}

	result := &RefreshResult{}

	// Step 1: Compute current function hashes
	currentHashes := make(map[string]string)        // "name:type" -> content_hash
	funcLookup := make(map[string]ctxpkg.FunctionDef) // "name:function" -> FunctionDef
	typeLookup := make(map[string]ctxpkg.TypeDef)    // "name:type" -> TypeDef

	for _, fn := range file.Functions {
		qualName := qualifyFuncName(fn)
		key := qualName + ":function"
		hash := computeFuncHash(fn)
		currentHashes[key] = hash
		funcLookup[key] = fn
	}
	for _, t := range file.Types {
		key := t.Name + ":type"
		hash := computeTypeHash(t)
		currentHashes[key] = hash
		typeLookup[key] = t
	}

	// Step 2: Get changed functions
	added, modified, removed, err := s.hashStore.GetChangedFunctions(ctx, repoID, filePath, currentHashes)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed functions: %w", err)
	}

	// Step 3: Prepare embedder
	embedder := s.embedder
	vocabVersion := ""

	if orgID != "" && s.vocabStore != nil {
		rec, err := s.vocabStore.GetOrgVocabulary(ctx, orgID)
		if err == nil && rec != nil {
			var wordIDF map[string]float64
			if jsonErr := json.Unmarshal([]byte(rec.VocabularyJSON), &wordIDF); jsonErr == nil {
				localEmb := NewLocalEmbedder(DefaultConfig())
				vocabData := &VocabularyData{
					WordIDF:     wordIDF,
					DocCount:    rec.DocCount,
					VersionHash: rec.VersionHash,
				}
				if importErr := localEmb.ImportVocabulary(vocabData); importErr == nil {
					embedder = localEmb
					vocabVersion = rec.VersionHash
				}
			}
		}
	}

	// Step 4: Process added functions
	hashEntries := make(map[string]FuncHashEntry) // collect all entries for bulk update
	for _, key := range added {
		record, buildErr := s.buildVectorRecord(key, filePath, repoID, orgID, vocabVersion, funcLookup, typeLookup, embedder)
		if buildErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("add %s: %v", key, buildErr))
			continue
		}
		if storeErr := s.store.Store(ctx, *record); storeErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("store %s: %v", key, storeErr))
			continue
		}
		hashEntries[key] = FuncHashEntry{
			Name:        nameFromKey(key),
			Type:        typeFromKey(key),
			ContentHash: currentHashes[key],
			VectorID:    record.ID,
		}
		result.Added++
	}

	// Step 5: Process modified functions
	for _, key := range modified {
		oldVectorID := fmt.Sprintf("%s:%s:%s:%s", repoID, vectorTypeFromKey(key), filePath, nameFromKey(key))
		if delErr := s.store.Delete(ctx, oldVectorID); delErr != nil {
			log.Printf("WARNING: failed to delete old vector %s: %v", oldVectorID, delErr)
		}

		record, buildErr := s.buildVectorRecord(key, filePath, repoID, orgID, vocabVersion, funcLookup, typeLookup, embedder)
		if buildErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("modify %s: %v", key, buildErr))
			continue
		}
		if storeErr := s.store.Store(ctx, *record); storeErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("store %s: %v", key, storeErr))
			continue
		}
		hashEntries[key] = FuncHashEntry{
			Name:        nameFromKey(key),
			Type:        typeFromKey(key),
			ContentHash: currentHashes[key],
			VectorID:    record.ID,
		}
		result.Modified++
	}

	// Step 6: Process removed functions
	for _, key := range removed {
		vectorID := fmt.Sprintf("%s:%s:%s:%s", repoID, vectorTypeFromKey(key), filePath, nameFromKey(key))
		if delErr := s.store.Delete(ctx, vectorID); delErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("remove %s: %v", key, delErr))
			continue
		}
		result.Removed++
	}

	// Step 7: Update function hashes (all current entries)
	var allHashes []FuncHashEntry
	for key, hash := range currentHashes {
		if entry, ok := hashEntries[key]; ok {
			allHashes = append(allHashes, entry)
		} else {
			// Unchanged function - preserve existing hash entry
			allHashes = append(allHashes, FuncHashEntry{
				Name:        nameFromKey(key),
				Type:        typeFromKey(key),
				ContentHash: hash,
			})
		}
	}
	if err := s.hashStore.UpdateFunctionHashes(ctx, repoID, filePath, allHashes); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("update hashes: %v", err))
	}

	return result, nil
}

// buildVectorRecord creates a VectorRecord for a function or type.
func (s *SemanticSearch) buildVectorRecord(key, filePath, repoID, orgID, vocabVersion string,
	funcLookup map[string]ctxpkg.FunctionDef, typeLookup map[string]ctxpkg.TypeDef, embedder Embedder) (*VectorRecord, error) {

	name := nameFromKey(key)
	itemType := typeFromKey(key)

	var doc string
	var vectorType string

	if itemType == "function" {
		fn, ok := funcLookup[key]
		if !ok {
			return nil, fmt.Errorf("function %s not found in file context", key)
		}
		doc = buildFunctionDocument(fn, filePath)
		vectorType = "func"
	} else {
		t, ok := typeLookup[key]
		if !ok {
			return nil, fmt.Errorf("type %s not found in file context", key)
		}
		doc = buildTypeDocument(t, filePath)
		vectorType = "type"
	}

	vec := embedder.Embed(doc)
	record := &VectorRecord{
		ID:           fmt.Sprintf("%s:%s:%s:%s", repoID, vectorType, filePath, name),
		RepoID:       repoID,
		OrgID:        orgID,
		Type:         itemType,
		Name:         name,
		FilePath:     filePath,
		Vector:       vec,
		VocabVersion: vocabVersion,
		Metadata:     make(map[string]string),
	}

	if itemType == "function" {
		if fn, ok := funcLookup[key]; ok {
			record.Metadata["signature"] = fn.Signature
			record.Metadata["description"] = fn.Description
			record.Metadata["is_public"] = fmt.Sprintf("%v", fn.IsPublic)
		}
	} else {
		if t, ok := typeLookup[key]; ok {
			record.Metadata["kind"] = t.Kind
			record.Metadata["description"] = t.Description
			record.Metadata["is_public"] = fmt.Sprintf("%v", t.IsPublic)
		}
	}

	return record, nil
}

// computeFuncHash computes a SHA256 content hash for a function.
func computeFuncHash(fn ctxpkg.FunctionDef) string {
	// Build deterministic string from function properties
	parts := []string{fn.Receiver, fn.Name, fn.Signature, fn.Description}
	if fn.Behavior != nil {
		parts = append(parts, fn.Behavior.Summary)
		parts = append(parts, fn.Behavior.Steps...)
	}
	content := strings.Join(parts, "|")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
}

// computeTypeHash computes a SHA256 content hash for a type definition.
func computeTypeHash(t ctxpkg.TypeDef) string {
	parts := []string{t.Name, t.Kind, t.Description}
	// Sort fields for determinism
	fieldNames := make([]string, 0, len(t.Fields))
	for _, f := range t.Fields {
		fieldNames = append(fieldNames, f.Name+":"+f.Type)
	}
	sort.Strings(fieldNames)
	parts = append(parts, fieldNames...)
	content := strings.Join(parts, "|")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
}

// qualifyFuncName returns the receiver-qualified name for a function.
func qualifyFuncName(fn ctxpkg.FunctionDef) string {
	if fn.Receiver != "" {
		return fmt.Sprintf("(%s).%s", fn.Receiver, fn.Name)
	}
	return fn.Name
}

// nameFromKey extracts the name from a "name:type" key.
func nameFromKey(key string) string {
	idx := strings.LastIndex(key, ":")
	if idx < 0 {
		return key
	}
	return key[:idx]
}

// typeFromKey extracts the type from a "name:type" key.
func typeFromKey(key string) string {
	idx := strings.LastIndex(key, ":")
	if idx < 0 {
		return ""
	}
	return key[idx+1:]
}

// vectorTypeFromKey converts "function" -> "func", "type" -> "type" for vector ID format.
func vectorTypeFromKey(key string) string {
	t := typeFromKey(key)
	if t == "function" {
		return "func"
	}
	return t
}
