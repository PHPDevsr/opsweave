CREATE TABLE workflows (
    name TEXT PRIMARY KEY,
    enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
    archived INTEGER NOT NULL CHECK(archived IN (0,1)),
    current_revision TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (name,current_revision) REFERENCES workflow_revisions(workflow_name,id)
        DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE workflow_revisions (
    id TEXT PRIMARY KEY,
    workflow_name TEXT NOT NULL REFERENCES workflows(name),
    number INTEGER NOT NULL CHECK(number > 0),
    source BLOB NOT NULL,
    normalized TEXT NOT NULL,
    normalization_version INTEGER NOT NULL CHECK(normalization_version = 1),
    bindings TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(workflow_name,number),
    UNIQUE(workflow_name,id)
);
CREATE TRIGGER revisions_no_update BEFORE UPDATE ON workflow_revisions
BEGIN SELECT RAISE(ABORT,'workflow revisions are immutable'); END;
CREATE TRIGGER revisions_no_delete BEFORE DELETE ON workflow_revisions
BEGIN SELECT RAISE(ABORT,'workflow revisions are immutable'); END;
