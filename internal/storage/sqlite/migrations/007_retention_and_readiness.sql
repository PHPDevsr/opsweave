ALTER TABLE dispatcher ADD COLUMN server_state TEXT NOT NULL DEFAULT 'unavailable'
 CHECK(server_state IN ('unavailable','ready','stopping'));

CREATE TABLE pruned_schedule_occurrences (
 workflow_name TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 trigger_id TEXT NOT NULL,
 scheduled_at TEXT NOT NULL,
 PRIMARY KEY(workflow_name,revision_id,trigger_id,scheduled_at),
 FOREIGN KEY(workflow_name,revision_id) REFERENCES workflow_revisions(workflow_name,id)
);

-- Authorization lasts only inside the transaction deleting the complete record.
CREATE TABLE execution_prune_targets (id TEXT PRIMARY KEY);

DROP TRIGGER execution_events_no_delete;
CREATE TRIGGER execution_events_no_delete BEFORE DELETE ON execution_events
WHEN NOT EXISTS (SELECT 1 FROM execution_prune_targets WHERE id=OLD.execution_id)
BEGIN SELECT RAISE(ABORT,'execution events are immutable'); END;

DROP TRIGGER attempts_no_delete;
CREATE TRIGGER attempts_no_delete BEFORE DELETE ON execution_attempts
WHEN NOT EXISTS (SELECT 1 FROM execution_prune_targets WHERE id=OLD.execution_id)
BEGIN SELECT RAISE(ABORT,'attempt history is immutable'); END;

DROP TRIGGER scheduled_executions_no_delete;
CREATE TRIGGER scheduled_executions_no_delete BEFORE DELETE ON scheduled_executions
WHEN NOT EXISTS (SELECT 1 FROM execution_prune_targets WHERE id=OLD.execution_id)
BEGIN SELECT RAISE(ABORT,'scheduled admission identity is immutable'); END;

DROP TRIGGER schedule_events_no_delete;
CREATE TRIGGER schedule_events_no_delete BEFORE DELETE ON schedule_events
WHEN NOT EXISTS (SELECT 1 FROM execution_prune_targets WHERE id=OLD.execution_id)
BEGIN SELECT RAISE(ABORT,'schedule events are immutable'); END;

DROP TRIGGER event_executions_no_delete;
CREATE TRIGGER event_executions_no_delete BEFORE DELETE ON event_executions
WHEN NOT EXISTS (SELECT 1 FROM execution_prune_targets WHERE id=OLD.execution_id)
BEGIN SELECT RAISE(ABORT,'event execution origin is immutable'); END;

DROP TRIGGER event_history_no_delete;
CREATE TRIGGER event_history_no_delete BEFORE DELETE ON event_history
WHEN NOT EXISTS (SELECT 1 FROM execution_prune_targets WHERE id=OLD.execution_id)
BEGIN SELECT RAISE(ABORT,'event history is immutable'); END;

CREATE INDEX execution_policies_parent ON execution_policies(parent_id);
CREATE INDEX execution_admissions_execution ON execution_admissions(execution_id);
CREATE INDEX event_dedup_execution ON event_dedup(execution_id);
CREATE INDEX event_history_execution ON event_history(execution_id);
CREATE INDEX schedule_events_execution ON schedule_events(execution_id);
