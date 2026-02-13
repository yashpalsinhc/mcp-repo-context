package vectors

import (
	"context"
	"fmt"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// SemanticSearch provides semantic search capabilities over code.
type SemanticSearch struct {
	embedder Embedder
	store    *SQLiteVectorStore
}

// NewSemanticSearch creates a new semantic search service.
func NewSemanticSearch(embedder Embedder, store *SQLiteVectorStore) *SemanticSearch {
	return &SemanticSearch{
		embedder: embedder,
		store:    store,
	}
}

// IndexRepository indexes all functions and types from a repository.
func (s *SemanticSearch) IndexRepository(ctx context.Context, repo *ctxpkg.RepoContext) error {
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

// ClearRepository removes all vectors for a repository.
func (s *SemanticSearch) ClearRepository(ctx context.Context, repoID string) error {
	return s.store.DeleteByRepo(ctx, repoID)
}

// Count returns the number of indexed items for a repository.
func (s *SemanticSearch) Count(ctx context.Context, repoID string) (int, error) {
	return s.store.Count(ctx, repoID)
}

// IndexRepositoryWithOrg indexes a repository and tags all vectors with org_id.
func (s *SemanticSearch) IndexRepositoryWithOrg(ctx context.Context, repo *ctxpkg.RepoContext, orgID string) error {
	if repo == nil {
		return fmt.Errorf("repository context is nil")
	}

	var documents []string
	for _, fc := range repo.Files {
		for _, fn := range fc.Functions {
			documents = append(documents, buildFunctionDocument(fn, fc.Path))
		}
		for _, t := range fc.Types {
			documents = append(documents, buildTypeDocument(t, fc.Path))
		}
	}

	if localEmb, ok := s.embedder.(*LocalEmbedder); ok {
		localEmb.BuildVocabulary(documents)
	}

	var records []VectorRecord

	for path, fc := range repo.Files {
		for _, fn := range fc.Functions {
			doc := buildFunctionDocument(fn, path)
			vec := s.embedder.Embed(doc)

			record := VectorRecord{
				ID:       fmt.Sprintf("%s:func:%s:%s", repo.ID, path, fn.Name),
				RepoID:   repo.ID,
				OrgID:    orgID,
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
				OrgID:    orgID,
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

	return s.store.StoreBatch(ctx, records)
}

// SearchByOrg searches across all repos in an org.
func (s *SemanticSearch) SearchByOrg(ctx context.Context, query string, orgID string, limit int) ([]SearchResult, error) {
	queryVec := s.embedder.Embed(query)
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
