CREATE TABLE executions (
    id TEXT PRIMARY KEY,
    workflow_name TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK(state IN ('pending','queued','running','succeeded','failed','cancelled','timed_out')),
    version INTEGER NOT NULL CHECK(version >= 0),
    owner TEXT NOT NULL,
    created_at TEXT NOT NULL,
    body TEXT NOT NULL,
    FOREIGN KEY(workflow_name,revision_id) REFERENCES workflow_revisions(workflow_name,id)
);
CREATE UNIQUE INDEX executions_active_workflow ON executions(workflow_name)
    WHERE state IN ('pending','queued','running');
CREATE TABLE execution_steps (
    execution_id TEXT NOT NULL REFERENCES executions(id),
    step_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK(ordinal >= 0 AND ordinal < 256),
    body TEXT NOT NULL,
    PRIMARY KEY(execution_id,step_id),
    UNIQUE(execution_id,ordinal)
);
CREATE TABLE execution_events (
    execution_id TEXT NOT NULL REFERENCES executions(id),
    sequence INTEGER NOT NULL CHECK(sequence > 0 AND sequence <= 1024),
    body TEXT NOT NULL,
    PRIMARY KEY(execution_id,sequence)
);
CREATE TRIGGER execution_events_no_update BEFORE UPDATE ON execution_events
BEGIN SELECT RAISE(ABORT,'execution events are immutable'); END;
CREATE TRIGGER execution_events_no_delete BEFORE DELETE ON execution_events
BEGIN SELECT RAISE(ABORT,'execution events are immutable'); END;
