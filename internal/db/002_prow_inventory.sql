CREATE TABLE prow_job_configs (
    job_name TEXT PRIMARY KEY,
    application TEXT NOT NULL,
    target TEXT NOT NULL,
    revision TEXT NOT NULL,
    source_url TEXT NOT NULL,
    ocp TEXT NOT NULL DEFAULT '',
    cloud TEXT NOT NULL DEFAULT '',
    removed INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_prow_job_configs_application ON prow_job_configs(application);
CREATE TABLE prow_inventory_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    revision TEXT NOT NULL
);
