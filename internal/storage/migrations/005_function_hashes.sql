-- Function-level hash tracking for incremental vector updates
-- Tracks SHA256 hashes of individual functions/types to detect changes
-- at function granularity rather than file granularity.

CREATE TABLE IF NOT EXISTS function_hashes (
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    name TEXT NOT NULL,         -- Fully qualified: includes receiver, e.g. "(*Foo).Bar"
    type TEXT NOT NULL,          -- "function" or "type"
    content_hash TEXT NOT NULL,  -- SHA256 of raw source code (line range from file)
    vector_id TEXT,              -- Application-level reference to vectors table ID (NOT a real FK)
    PRIMARY KEY (repo_id, file_path, name, type)
);

-- Index for efficient lookups by repo_id + file_path (common query pattern)
CREATE INDEX IF NOT EXISTS idx_function_hashes_repo_file
    ON function_hashes(repo_id, file_path);

-- Record migration version
INSERT OR IGNORE INTO schema_migrations (version) VALUES (5);
