-- Migration 004: Dependency graph support
-- Note: actual migration is done in Go code because SQLite ALTER TABLE
-- ADD COLUMN has no IF NOT EXISTS syntax. This file is documentation only.

-- Store parsed go.mod data per repo
-- ALTER TABLE repos ADD COLUMN module_info_json TEXT;

-- Store import summary per repo
-- ALTER TABLE repos ADD COLUMN import_summary_json TEXT;

-- Extend files table for config file content and structured data
-- ALTER TABLE files ADD COLUMN content TEXT;
-- ALTER TABLE files ADD COLUMN structured_json TEXT;
-- ALTER TABLE files ADD COLUMN config_type TEXT;

INSERT OR IGNORE INTO schema_migrations (version) VALUES (4);
