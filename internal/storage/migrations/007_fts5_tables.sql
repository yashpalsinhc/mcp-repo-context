-- FTS5 full-text search for functions (standalone, not content-synced)
-- We use a standalone table because the "summary" data lives in behavior_json, not a real column.

CREATE VIRTUAL TABLE IF NOT EXISTS functions_fts USING fts5(
    name, signature, description,
    content='',
    contentless_delete=1
);

-- INSERT trigger: auto-populate FTS when new functions are added
CREATE TRIGGER IF NOT EXISTS functions_fts_insert AFTER INSERT ON functions BEGIN
    INSERT INTO functions_fts(rowid, name, signature, description)
    VALUES (new.id, new.name, new.signature, COALESCE(new.description, ''));
END;

-- UPDATE trigger: keep FTS in sync when functions are updated
CREATE TRIGGER IF NOT EXISTS functions_fts_update AFTER UPDATE ON functions BEGIN
    DELETE FROM functions_fts WHERE rowid = old.id;
    INSERT INTO functions_fts(rowid, name, signature, description)
    VALUES (new.id, new.name, new.signature, COALESCE(new.description, ''));
END;

-- DELETE trigger: remove from FTS when functions are explicitly deleted
CREATE TRIGGER IF NOT EXISTS functions_fts_delete AFTER DELETE ON functions BEGIN
    DELETE FROM functions_fts WHERE rowid = old.id;
END;
