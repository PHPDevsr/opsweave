CREATE TABLE event_routes (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,
 workflow_name TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 trigger_id TEXT NOT NULL,
 source_type TEXT NOT NULL CHECK(source_type IN ('startup','internal','webhook')),
 event_name TEXT NOT NULL,
 UNIQUE(workflow_name,trigger_id),
 FOREIGN KEY(workflow_name,revision_id) REFERENCES workflow_revisions(workflow_name,id)
);
CREATE INDEX event_routes_name ON event_routes(source_type,event_name,sequence);
CREATE TABLE event_executions (
 execution_id TEXT PRIMARY KEY REFERENCES executions(id),
 workflow_name TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 trigger_id TEXT NOT NULL,
 source_type TEXT NOT NULL CHECK(source_type IN ('startup','internal','webhook')),
 automatic_generation INTEGER NOT NULL CHECK(automatic_generation>=0),
 FOREIGN KEY(workflow_name,revision_id) REFERENCES workflow_revisions(workflow_name,id)
);
CREATE TABLE event_dedup (
 source_type TEXT NOT NULL CHECK(source_type IN ('internal','webhook')),
 workflow_name TEXT NOT NULL,
 trigger_id TEXT NOT NULL,
 event_digest TEXT NOT NULL CHECK(length(event_digest)=64),
 fingerprint TEXT NOT NULL CHECK(length(fingerprint)=64),
 expires_at INTEGER NOT NULL,
 execution_id TEXT NOT NULL REFERENCES event_executions(execution_id),
 PRIMARY KEY(source_type,workflow_name,trigger_id,event_digest)
);
CREATE TABLE startup_scan (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 generation INTEGER NOT NULL,
 cursor INTEGER NOT NULL,
 upper_bound INTEGER NOT NULL,
 started_at TEXT NOT NULL,
 deferred_sequence INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE event_history (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,
 workflow_name TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 trigger_id TEXT NOT NULL,
 source_type TEXT NOT NULL CHECK(source_type IN ('startup','internal','webhook')),
 occurred_at TEXT NOT NULL,
 code TEXT NOT NULL,
 execution_id TEXT REFERENCES event_executions(execution_id),
 FOREIGN KEY(workflow_name,revision_id) REFERENCES workflow_revisions(workflow_name,id)
);
CREATE INDEX event_history_workflow ON event_history(workflow_name,sequence);
CREATE TRIGGER event_executions_no_update BEFORE UPDATE ON event_executions
BEGIN SELECT RAISE(ABORT,'event execution origin is immutable'); END;
CREATE TRIGGER event_executions_no_delete BEFORE DELETE ON event_executions
BEGIN SELECT RAISE(ABORT,'event execution origin is immutable'); END;
CREATE TRIGGER event_history_no_update BEFORE UPDATE ON event_history
BEGIN SELECT RAISE(ABORT,'event history is immutable'); END;
CREATE TRIGGER event_history_no_delete BEFORE DELETE ON event_history
BEGIN SELECT RAISE(ABORT,'event history is immutable'); END;
