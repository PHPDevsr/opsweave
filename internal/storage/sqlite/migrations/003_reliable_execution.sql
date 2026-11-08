DROP TRIGGER execution_events_no_update;
DROP TRIGGER execution_events_no_delete;
ALTER TABLE execution_events RENAME TO execution_events_v1;
CREATE TABLE execution_events (
    execution_id TEXT NOT NULL REFERENCES executions(id),
    sequence INTEGER NOT NULL CHECK(sequence > 0 AND sequence <= 16384),
    body TEXT NOT NULL,
    PRIMARY KEY(execution_id,sequence)
);
INSERT INTO execution_events SELECT execution_id,sequence,body FROM execution_events_v1;
DROP TABLE execution_events_v1;
CREATE TRIGGER execution_events_no_update BEFORE UPDATE ON execution_events
BEGIN SELECT RAISE(ABORT,'execution events are immutable'); END;
CREATE TRIGGER execution_events_no_delete BEFORE DELETE ON execution_events
BEGIN SELECT RAISE(ABORT,'execution events are immutable'); END;

CREATE TABLE execution_policies (
    execution_id TEXT PRIMARY KEY REFERENCES executions(id),
    format INTEGER NOT NULL CHECK(format=2),
    event_limit INTEGER NOT NULL CHECK(event_limit=16384),
    parent_id TEXT REFERENCES executions(id),
    acknowledged_uncertain INTEGER NOT NULL CHECK(acknowledged_uncertain IN (0,1))
);
CREATE TABLE execution_attempts (
    execution_id TEXT NOT NULL,
    step_id TEXT NOT NULL,
    number INTEGER NOT NULL CHECK(number BETWEEN 1 AND 10),
    state TEXT NOT NULL CHECK(state IN ('running','succeeded','failed','cancelled','timed_out')),
    body TEXT NOT NULL,
    PRIMARY KEY(execution_id,step_id,number),
    FOREIGN KEY(execution_id,step_id) REFERENCES execution_steps(execution_id,step_id)
);
CREATE TRIGGER attempts_terminal_immutable BEFORE UPDATE ON execution_attempts
WHEN OLD.state<>'running' OR NEW.execution_id<>OLD.execution_id OR NEW.step_id<>OLD.step_id
    OR NEW.number<>OLD.number OR NEW.state='running'
BEGIN SELECT RAISE(ABORT,'terminal attempts are immutable'); END;
CREATE TRIGGER attempts_no_delete BEFORE DELETE ON execution_attempts
BEGIN SELECT RAISE(ABORT,'attempt history is immutable'); END;
CREATE TABLE execution_admissions (
    scope TEXT NOT NULL,
    digest TEXT NOT NULL CHECK(length(digest)=64),
    fingerprint TEXT NOT NULL CHECK(length(fingerprint)=64),
    execution_id TEXT NOT NULL REFERENCES executions(id),
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY(scope,digest)
);
