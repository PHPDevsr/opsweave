ALTER TABLE dispatcher ADD COLUMN worker_capacity INTEGER NOT NULL DEFAULT 0
 CHECK(worker_capacity BETWEEN 0 AND 64);
CREATE INDEX execution_attempts_running ON execution_attempts(execution_id) WHERE state='running';
CREATE INDEX executions_queued ON executions(id) WHERE state='queued';
