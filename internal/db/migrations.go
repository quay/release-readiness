package db

import "fmt"

const schema = `
CREATE TABLE IF NOT EXISTS components (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS test_suites (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS component_suites (
    component_id INTEGER NOT NULL REFERENCES components(id) ON DELETE CASCADE,
    suite_id     INTEGER NOT NULL REFERENCES test_suites(id) ON DELETE CASCADE,
    required     INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (component_id, suite_id)
);

CREATE TABLE IF NOT EXISTS builds (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    component_id  INTEGER NOT NULL REFERENCES components(id),
    version       TEXT NOT NULL,
    git_sha       TEXT NOT NULL,
    git_branch    TEXT NOT NULL DEFAULT '',
    image_url     TEXT NOT NULL,
    image_digest  TEXT NOT NULL DEFAULT '',
    pipeline_run  TEXT NOT NULL DEFAULT '',
    snapshot_name TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'pending',
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_builds_component ON builds(component_id);
CREATE INDEX IF NOT EXISTS idx_builds_version ON builds(version);
CREATE INDEX IF NOT EXISTS idx_builds_created ON builds(created_at DESC);

CREATE TABLE IF NOT EXISTS test_runs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    build_id     INTEGER NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    suite_id     INTEGER NOT NULL REFERENCES test_suites(id),
    total        INTEGER NOT NULL DEFAULT 0,
    passed       INTEGER NOT NULL DEFAULT 0,
    failed       INTEGER NOT NULL DEFAULT 0,
    skipped      INTEGER NOT NULL DEFAULT 0,
    duration_sec REAL NOT NULL DEFAULT 0.0,
    status       TEXT NOT NULL DEFAULT 'unknown',
    environment  TEXT NOT NULL DEFAULT '',
    pipeline_run TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_test_runs_build ON test_runs(build_id);

CREATE TABLE IF NOT EXISTS test_cases (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    test_run_id  INTEGER NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    classname    TEXT NOT NULL DEFAULT '',
    duration_sec REAL NOT NULL DEFAULT 0.0,
    status       TEXT NOT NULL,
    failure_msg  TEXT NOT NULL DEFAULT '',
    failure_text TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_test_cases_run ON test_cases(test_run_id);
`

var seedComponents = []struct {
	name        string
	description string
}{
	{"quay", "Quay container registry"},
	{"clair", "Clair vulnerability scanner"},
	{"quay-operator", "Quay Operator"},
	{"quay-operator-bundle", "Quay Operator OLM bundle"},
	{"quay-bridge-operator", "Quay Bridge Operator"},
	{"quay-bridge-operator-bundle", "Quay Bridge Operator OLM bundle"},
	{"container-security-operator", "Container Security Operator"},
	{"container-security-operator-bundle", "Container Security Operator OLM bundle"},
	{"quay-builder", "Quay Builder"},
	{"quay-builder-qemu", "Quay Builder QEMU"},
}

var seedSuites = []struct {
	name        string
	description string
}{
	{"operator-ginkgo", "Operator Ginkgo test suite"},
	{"ui-cypress", "New UI Cypress tests"},
	{"api-cypress", "API Cypress tests"},
	{"frontend-cypress", "Frontend Cypress tests"},
	{"enterprise-contract", "Enterprise Contract verification"},
}

func (d *DB) migrate() error {
	if _, err := d.Exec(schema); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}

	for _, c := range seedComponents {
		_, err := d.Exec(
			`INSERT OR IGNORE INTO components (name, description) VALUES (?, ?)`,
			c.name, c.description,
		)
		if err != nil {
			return fmt.Errorf("seed component %q: %w", c.name, err)
		}
	}

	for _, s := range seedSuites {
		_, err := d.Exec(
			`INSERT OR IGNORE INTO test_suites (name, description) VALUES (?, ?)`,
			s.name, s.description,
		)
		if err != nil {
			return fmt.Errorf("seed suite %q: %w", s.name, err)
		}
	}

	return nil
}
