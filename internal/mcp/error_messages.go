package mcp

import "fmt"

// formatStoreUnavailableMessage returns guidance when vector store isn't available.
func formatStoreUnavailableMessage() string {
	return `## Vector Store Unavailable

The server could not initialize the vector database.

**To fix:**
1. Check the ` + "`MCP_VECTOR_STORE_PATH`" + ` environment variable
2. Ensure the directory has write permissions
3. Check available disk space
4. Restart the server`
}

// formatNotIndexedMessage returns guidance when a repo hasn't been indexed.
func formatNotIndexedMessage(repoID string) string {
	return fmt.Sprintf(`## Repository Not Indexed

Repository `+"`%s`"+` has not been indexed for semantic search.

**To index, run:**
`+"```"+`
index_repository(repo_id: "%s")
`+"```"+`

This generates vector embeddings for all functions and types,
enabling similarity-based search. Indexing is fast (~1-5 seconds for most repos).`, repoID, repoID)
}

// formatNotAnalyzedMessage returns guidance when a repo hasn't been analyzed.
func formatNotAnalyzedMessage(repoID string) string {
	return fmt.Sprintf(`## Repository Not Analyzed

Repository `+"`%s`"+` has not been analyzed yet.

**Analyze first, then index:**
`+"```"+`
analyze_repo(repo_url: "https://github.com/...")
`+"```"+`
or for local directories:
`+"```"+`
analyze_local(path: "/path/to/repo")
`+"```"+`

After analysis, run `+"`index_repository`"+` to enable semantic search.`, repoID)
}

// formatStoreUnavailableForIndexMessage returns guidance when vector store isn't available during indexing.
func formatStoreUnavailableForIndexMessage() string {
	return `## Vector Store Unavailable

Cannot index: the vector database is not available.

**To fix:**
1. Check the ` + "`MCP_VECTOR_STORE_PATH`" + ` environment variable
2. Ensure the path is writable
3. Restart the server`
}
