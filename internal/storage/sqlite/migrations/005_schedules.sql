ALTER TABLE workflows ADD COLUMN automatic_generation INTEGER NOT NULL DEFAULT 0 CHECK(automatic_generation>=0);
CREATE TABLE schedules (
 workflow_name TEXT NOT NULL REFERENCES workflows(name),
 trigger_id TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 expression TEXT NOT NULL,
 timezone TEXT NOT NULL,
 missed_run TEXT NOT NULL CHECK(missed_run IN ('skip','catch_up_once')),
 cursor_at TEXT NOT NULL,
 checked_minute INTEGER NOT NULL,
 pending_at TEXT,
 retry_at INTEGER NOT NULL,
 version INTEGER NOT NULL CHECK(version>=0),
 last_failure TEXT NOT NULL,
 PRIMARY KEY(workflow_name,trigger_id),
 FOREIGN KEY(workflow_name,revision_id) REFERENCES workflow_revisions(workflow_name,id)
);
CREATE INDEX schedules_due ON schedules(retry_at,workflow_name,trigger_id);
CREATE TABLE scheduled_executions (
 execution_id TEXT PRIMARY KEY REFERENCES executions(id),
 workflow_name TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 trigger_id TEXT NOT NULL,
 scheduled_at TEXT NOT NULL,
 automatic_generation INTEGER NOT NULL CHECK(automatic_generation>=0),
 UNIQUE(workflow_name,revision_id,trigger_id,scheduled_at),
 FOREIGN KEY(workflow_name,revision_id) REFERENCES workflow_revisions(workflow_name,id)
);
CREATE TRIGGER scheduled_executions_no_update BEFORE UPDATE ON scheduled_executions
BEGIN SELECT RAISE(ABORT,'scheduled admission identity is immutable'); END;
CREATE TRIGGER scheduled_executions_no_delete BEFORE DELETE ON scheduled_executions
BEGIN SELECT RAISE(ABORT,'scheduled admission identity is immutable'); END;
CREATE TABLE schedule_events (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,
 workflow_name TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 trigger_id TEXT NOT NULL,
 occurred_at TEXT NOT NULL,
 scheduled_at TEXT,
 code TEXT NOT NULL,
 execution_id TEXT REFERENCES executions(id),
 skipped INTEGER NOT NULL CHECK(skipped>=0 AND skipped<=1441),
 FOREIGN KEY(workflow_name,revision_id) REFERENCES workflow_revisions(workflow_name,id)
);
CREATE INDEX schedule_events_workflow ON schedule_events(workflow_name,sequence);
CREATE TRIGGER schedule_events_no_update BEFORE UPDATE ON schedule_events
BEGIN SELECT RAISE(ABORT,'schedule events are immutable'); END;
CREATE TRIGGER schedule_events_no_delete BEFORE DELETE ON schedule_events
BEGIN SELECT RAISE(ABORT,'schedule events are immutable'); END;
