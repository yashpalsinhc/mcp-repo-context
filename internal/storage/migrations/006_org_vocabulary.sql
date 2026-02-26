-- Org-wide vocabulary tracking for semantic search
-- Stores serialized vocabulary data with version hashes for cache invalidation.

CREATE TABLE IF NOT EXISTS org_vocabulary (
    org_id TEXT PRIMARY KEY,
    vocabulary_json TEXT NOT NULL,
    version_hash TEXT NOT NULL,
    doc_count INTEGER DEFAULT 0,
    built_at TIMESTAMP,
    repo_count INTEGER DEFAULT 0,
    is_stale BOOLEAN DEFAULT 0
);

-- Record migration version
INSERT OR IGNORE INTO schema_migrations (version) VALUES (6);
