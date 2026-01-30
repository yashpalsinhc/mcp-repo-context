-- MCP Repo Context Server - Initial Schema
-- Portable schema without FTS5 (uses LIKE queries for search)

-- Enable foreign keys
PRAGMA foreign_keys = ON;

-- ============================================================================
-- Core Tables
-- ============================================================================

-- Repositories table
CREATE TABLE IF NOT EXISTS repos (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    branch TEXT NOT NULL,
    commit_hash TEXT NOT NULL,
    analyzed_at TIMESTAMP NOT NULL,
    file_count INTEGER DEFAULT 0,
    total_lines INTEGER DEFAULT 0,
    function_count INTEGER DEFAULT 0,
    type_count INTEGER DEFAULT 0,
    stats_json TEXT,
    ai_summary_json TEXT,
    architecture_json TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_repos_url ON repos(url);
CREATE INDEX IF NOT EXISTS idx_repos_analyzed_at ON repos(analyzed_at);

-- Files table
CREATE TABLE IF NOT EXISTS files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    hash TEXT NOT NULL,
    language TEXT,
    package TEXT,
    size INTEGER,
    line_count INTEGER,
    purpose TEXT,
    concepts_json TEXT,
    imports_json TEXT,
    routes_json TEXT,
    analyzed_at TIMESTAMP,
    UNIQUE(repo_id, path)
);

CREATE INDEX IF NOT EXISTS idx_files_repo_id ON files(repo_id);
CREATE INDEX IF NOT EXISTS idx_files_language ON files(language);
CREATE INDEX IF NOT EXISTS idx_files_package ON files(package);
CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);

-- Functions table
CREATE TABLE IF NOT EXISTS functions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    name_lower TEXT NOT NULL,  -- For case-insensitive search
    signature TEXT,
    description TEXT,
    receiver TEXT,
    is_public BOOLEAN DEFAULT 0,
    line_start INTEGER,
    line_end INTEGER,
    complexity INTEGER DEFAULT 1,
    parameters_json TEXT,
    returns_json TEXT,
    behavior_json TEXT,
    error_handling_json TEXT,
    api_flow_json TEXT,
    uses_imports_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_functions_file_id ON functions(file_id);
CREATE INDEX IF NOT EXISTS idx_functions_name ON functions(name);
CREATE INDEX IF NOT EXISTS idx_functions_name_lower ON functions(name_lower);
CREATE INDEX IF NOT EXISTS idx_functions_is_public ON functions(is_public);
CREATE INDEX IF NOT EXISTS idx_functions_receiver ON functions(receiver);

-- Function calls (call graph edges)
CREATE TABLE IF NOT EXISTS function_calls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    caller_id INTEGER NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
    callee_name TEXT NOT NULL,
    callee_package TEXT,
    callee_file TEXT,
    line INTEGER,
    call_type TEXT
);

CREATE INDEX IF NOT EXISTS idx_function_calls_caller ON function_calls(caller_id);
CREATE INDEX IF NOT EXISTS idx_function_calls_callee ON function_calls(callee_name);

-- Types table
CREATE TABLE IF NOT EXISTS types (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    name_lower TEXT NOT NULL,
    kind TEXT,
    description TEXT,
    is_public BOOLEAN DEFAULT 0,
    line_start INTEGER,
    line_end INTEGER,
    fields_json TEXT,
    methods_json TEXT,
    implements_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_types_file_id ON types(file_id);
CREATE INDEX IF NOT EXISTS idx_types_name ON types(name);
CREATE INDEX IF NOT EXISTS idx_types_name_lower ON types(name_lower);
CREATE INDEX IF NOT EXISTS idx_types_kind ON types(kind);

-- Constants table
CREATE TABLE IF NOT EXISTS constants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT,
    value TEXT,
    is_public BOOLEAN DEFAULT 0,
    line_start INTEGER
);

CREATE INDEX IF NOT EXISTS idx_constants_file_id ON constants(file_id);

-- ============================================================================
-- Lookup Tables for Fast Filtering
-- ============================================================================

-- Side effects index
CREATE TABLE IF NOT EXISTS side_effects (
    function_id INTEGER NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
    effect TEXT NOT NULL,
    PRIMARY KEY (function_id, effect)
);

CREATE INDEX IF NOT EXISTS idx_side_effects_effect ON side_effects(effect);

-- Concepts index
CREATE TABLE IF NOT EXISTS concepts (
    function_id INTEGER NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
    concept TEXT NOT NULL,
    PRIMARY KEY (function_id, concept)
);

CREATE INDEX IF NOT EXISTS idx_concepts_concept ON concepts(concept);

-- File concepts
CREATE TABLE IF NOT EXISTS file_concepts (
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    concept TEXT NOT NULL,
    PRIMARY KEY (file_id, concept)
);

CREATE INDEX IF NOT EXISTS idx_file_concepts_concept ON file_concepts(concept);

-- ============================================================================
-- Schema Version Tracking
-- ============================================================================

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO schema_migrations (version) VALUES (1);
