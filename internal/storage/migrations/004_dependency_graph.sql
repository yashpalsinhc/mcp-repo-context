-- Migration 004: Dependency graph support

-- Store parsed go.mod data per repo
ALTER TABLE repos ADD COLUMN module_info_json TEXT;

-- Store import summary per repo
ALTER TABLE repos ADD COLUMN import_summary_json TEXT;

-- Store config files as JSON per repo
ALTER TABLE repos ADD COLUMN config_files_json TEXT;

INSERT OR IGNORE INTO schema_migrations (version) VALUES (4);
