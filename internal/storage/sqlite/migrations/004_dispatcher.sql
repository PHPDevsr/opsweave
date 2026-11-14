DROP INDEX executions_active_workflow;
CREATE INDEX executions_active_workflow ON executions(workflow_name,state)
 WHERE state IN ('pending','queued','running');
CREATE TABLE dispatcher (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 generation INTEGER NOT NULL CHECK(generation>=0),
 owner TEXT NOT NULL
);
INSERT INTO dispatcher VALUES(1,0,'');
CREATE TABLE execution_queue (
 execution_id TEXT PRIMARY KEY REFERENCES executions(id),
 available_at TEXT NOT NULL,
 expires_at TEXT NOT NULL,
 timeout_ns INTEGER NOT NULL CHECK(timeout_ns>0),
 concurrency INTEGER NOT NULL CHECK(concurrency BETWEEN 1 AND 256),
 generation INTEGER NOT NULL CHECK(generation>=0)
);
CREATE INDEX execution_queue_due ON execution_queue(available_at,execution_id);
CREATE TABLE dispatcher_audit (
 generation INTEGER PRIMARY KEY,
 adopted_unmanaged INTEGER NOT NULL CHECK(adopted_unmanaged IN (0,1)),
 started_at TEXT NOT NULL
);
