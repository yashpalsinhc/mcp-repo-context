package storage

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// MetadataIndex provides fast access to repository metadata without
// reading the full context files. This solves the O(n) problem of
// ListContexts reading all multi-MB JSON files.
type MetadataIndex struct {
	mu       sync.RWMutex
	path     string
	entries  map[string]*MetadataEntry
	modified bool
}

// MetadataEntry contains lightweight metadata for a single repository.
type MetadataEntry struct {
	RepoID       string    `json:"repo_id"`
	URL          string    `json:"url"`
	Branch       string    `json:"branch"`
	CommitHash   string    `json:"commit_hash"`
	FileCount    int       `json:"file_count"`
	TotalLines   int       `json:"total_lines"`
	AnalyzedAt   time.Time `json:"analyzed_at"`
	ContextPath  string    `json:"context_path"`
	HasAISummary bool      `json:"has_ai_summary"`
	Languages    []string  `json:"languages,omitempty"`
}

// metadataIndexFile is the on-disk format.
type metadataIndexFile struct {
	Version   int                       `json:"version"`
	UpdatedAt time.Time                 `json:"updated_at"`
	Entries   map[string]*MetadataEntry `json:"entries"`
}

// NewMetadataIndex creates a new metadata index.
func NewMetadataIndex(path string) *MetadataIndex {
	return &MetadataIndex{
		path:    path,
		entries: make(map[string]*MetadataEntry),
	}
}

// Load reads the index from disk.
func (idx *MetadataIndex) Load() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	data, err := os.ReadFile(idx.path)
	if err != nil {
		if os.IsNotExist(err) {
			idx.entries = make(map[string]*MetadataEntry)
			return nil
		}
		return err
	}

	var file metadataIndexFile
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}

	idx.entries = file.Entries
	if idx.entries == nil {
		idx.entries = make(map[string]*MetadataEntry)
	}
	idx.modified = false

	return nil
}

// Save writes the index to disk.
func (idx *MetadataIndex) Save() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if !idx.modified {
		return nil
	}

	file := metadataIndexFile{
		Version:   1,
		UpdatedAt: time.Now(),
		Entries:   idx.entries,
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(idx.path, data, 0644); err != nil {
		return err
	}

	idx.modified = false
	return nil
}

// Update updates or adds an entry to the index.
func (idx *MetadataIndex) Update(repoID string, ctx *ctxpkg.RepoContext) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Extract languages from statistics
	var languages []string
	for lang := range ctx.Statistics.LanguageBreakdown {
		languages = append(languages, lang)
	}

	idx.entries[repoID] = &MetadataEntry{
		RepoID:       ctx.ID,
		URL:          ctx.URL,
		Branch:       ctx.Branch,
		CommitHash:   ctx.CommitHash,
		FileCount:    len(ctx.Files),
		TotalLines:   ctx.Statistics.TotalLines,
		AnalyzedAt:   ctx.AnalyzedAt,
		HasAISummary: ctx.AISummary != nil,
		Languages:    languages,
	}
	idx.modified = true
}

// UpdateEntry updates or adds a metadata entry directly.
func (idx *MetadataIndex) UpdateEntry(entry *MetadataEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.entries[entry.RepoID] = entry
	idx.modified = true
}

// Get retrieves metadata for a specific repository.
func (idx *MetadataIndex) Get(repoID string) (*MetadataEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.entries[repoID]
	return entry, ok
}

// Delete removes a repository from the index.
func (idx *MetadataIndex) Delete(repoID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	delete(idx.entries, repoID)
	idx.modified = true
}

// List returns metadata for all repositories.
// This is O(1) since it just returns the in-memory map.
func (idx *MetadataIndex) List() []ctxpkg.ContextMetadata {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	results := make([]ctxpkg.ContextMetadata, 0, len(idx.entries))
	for _, entry := range idx.entries {
		results = append(results, ctxpkg.ContextMetadata{
			RepoID:     entry.RepoID,
			URL:        entry.URL,
			Branch:     entry.Branch,
			CommitHash: entry.CommitHash,
			FileCount:  entry.FileCount,
			AnalyzedAt: entry.AnalyzedAt,
		})
	}

	return results
}

// Exists checks if a repository exists in the index.
func (idx *MetadataIndex) Exists(repoID string) (bool, time.Time) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if entry, ok := idx.entries[repoID]; ok {
		return true, entry.AnalyzedAt
	}
	return false, time.Time{}
}

// Count returns the number of repositories in the index.
func (idx *MetadataIndex) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}

// NeedsSave returns true if the index has unsaved changes.
func (idx *MetadataIndex) NeedsSave() bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.modified
}

// Rebuild reconstructs the index from all context files.
// This is expensive but ensures consistency.
func (idx *MetadataIndex) Rebuild(store ContextStore) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Clear existing entries
	idx.entries = make(map[string]*MetadataEntry)

	// Use the store's list method (this is the slow path)
	// We only call this once during rebuild
	ctx, cancel := ctxWithTimeout(5 * time.Minute)
	defer cancel()

	metas, err := store.ListContexts(ctx)
	if err != nil {
		return err
	}

	for _, meta := range metas {
		idx.entries[meta.RepoID] = &MetadataEntry{
			RepoID:     meta.RepoID,
			URL:        meta.URL,
			Branch:     meta.Branch,
			CommitHash: meta.CommitHash,
			FileCount:  meta.FileCount,
			AnalyzedAt: meta.AnalyzedAt,
		}
	}

	idx.modified = true
	return nil
}

// ctxWithTimeout creates a context with timeout for internal operations.
func ctxWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
