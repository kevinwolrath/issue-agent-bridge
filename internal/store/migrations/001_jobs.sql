CREATE TABLE jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL,
    delivery_id TEXT NOT NULL,
    repository TEXT NOT NULL,
    issue_number INTEGER NOT NULL CHECK (issue_number > 0),
    issue_url TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','succeeded','failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(provider, delivery_id)
);
CREATE INDEX jobs_queue ON jobs(status, id);
