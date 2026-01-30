-- MCP Repo Context Server - File Hash Tracking for Incremental Indexing
-- This migration adds support for tracking file hashes to enable efficient change detection.

-- File hashes table for incremental indexing
-- Tracks SHA256 hashes of files to detect changes without loading full context
CREATE TABLE IF NOT EXISTS file_hashes (
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    hash TEXT NOT NULL,
    last_analyzed TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    file_size INTEGER,
    PRIMARY KEY (repo_id, file_path),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);

-- Index for efficient lookups by repo
CREATE INDEX IF NOT EXISTS idx_file_hashes_repo_id ON file_hashes(repo_id);

-- Index for finding stale files (by last_analyzed timestamp)
CREATE INDEX IF NOT EXISTS idx_file_hashes_last_analyzed ON file_hashes(last_analyzed);

-- Update schema version
INSERT OR IGNORE INTO schema_migrations (version) VALUES (2);
