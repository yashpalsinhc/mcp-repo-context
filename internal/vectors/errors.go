package vectors

import "errors"

var (
	// ErrVectorStoreUnavailable indicates the vector store could not be initialized.
	ErrVectorStoreUnavailable = errors.New("vector store is not available")

	// ErrNotIndexed indicates a repository has not been indexed for semantic search.
	ErrNotIndexed = errors.New("repository is not indexed")

	// ErrNotAnalyzed indicates a repository has not been analyzed yet.
	ErrNotAnalyzed = errors.New("repository has not been analyzed")
)
