-- MCP Repo Context Server - Organization Tables
-- Adds organization management tables for grouping repos under orgs.

-- Organizations table
CREATE TABLE IF NOT EXISTS orgs (
    id TEXT PRIMARY KEY,
    config_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Organization-repository junction table
CREATE TABLE IF NOT EXISTS org_repos (
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    repo_id TEXT NOT NULL,
    config_override_json TEXT,
    added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (org_id, repo_id)
);

-- Index for reverse lookups (find which org a repo belongs to)
CREATE INDEX IF NOT EXISTS idx_org_repos_repo_id ON org_repos(repo_id);

-- Trigger to auto-update updated_at on orgs (WHEN guard prevents recursion)
CREATE TRIGGER IF NOT EXISTS update_orgs_timestamp
AFTER UPDATE ON orgs
WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE orgs SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

-- Record migration version
INSERT OR IGNORE INTO schema_migrations (version) VALUES (3);
