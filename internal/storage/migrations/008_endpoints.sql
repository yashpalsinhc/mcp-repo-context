-- Migration 008: Normalized endpoint and service call storage for cross-repo matching

CREATE TABLE IF NOT EXISTS endpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    handler_name TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    raw_path TEXT DEFAULT '',
    framework TEXT DEFAULT '',
    line INTEGER DEFAULT 0,
    UNIQUE(repo_id, file_path, handler_name, method, path)
);
CREATE INDEX IF NOT EXISTS idx_endpoints_path ON endpoints(path);
CREATE INDEX IF NOT EXISTS idx_endpoints_repo ON endpoints(repo_id);

CREATE TABLE IF NOT EXISTS service_calls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    function_name TEXT NOT NULL,
    call_type TEXT NOT NULL,
    method TEXT DEFAULT '',
    target TEXT NOT NULL,
    target_expression TEXT DEFAULT '',
    service_hint TEXT DEFAULT '',
    line INTEGER DEFAULT 0,
    UNIQUE(repo_id, file_path, function_name, call_type, target, line)
);
CREATE INDEX IF NOT EXISTS idx_service_calls_target ON service_calls(target);
CREATE INDEX IF NOT EXISTS idx_service_calls_repo ON service_calls(repo_id);
CREATE INDEX IF NOT EXISTS idx_service_calls_type ON service_calls(call_type);

INSERT OR IGNORE INTO schema_migrations (version) VALUES (8);
