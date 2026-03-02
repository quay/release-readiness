-- name: CreateSnapshot :execlastid
INSERT INTO snapshots (application, name, tests_passed, created_at)
VALUES (?, ?, ?, ?);

-- name: SnapshotExistsByName :one
SELECT COUNT(*) FROM snapshots WHERE name = ?;

-- name: GetSnapshotRow :one
SELECT id, application, name, tests_passed, created_at
FROM snapshots WHERE name = ?;

-- name: CreateSnapshotComponent :exec
INSERT INTO snapshot_components (snapshot_id, component, git_sha, image_url, git_url)
VALUES (?, ?, ?, ?, ?);

-- name: ListSnapshotComponents :many
SELECT id, snapshot_id, component, git_sha, image_url, git_url
FROM snapshot_components
WHERE snapshot_id = ?
ORDER BY component;

-- name: ListAllSnapshots :many
SELECT id, application, name, tests_passed, created_at
FROM snapshots
ORDER BY id DESC LIMIT ? OFFSET ?;

-- name: ListSnapshotsByApplication :many
SELECT id, application, name, tests_passed, created_at
FROM snapshots
WHERE application = ?
ORDER BY id DESC LIMIT ? OFFSET ?;

-- name: LatestSnapshotPerApplication :many
SELECT s.id, s.application, s.name, s.tests_passed, s.created_at, CAST(counts.cnt AS INTEGER) AS cnt,
       (SELECT COUNT(*) FROM test_suites WHERE snapshot_id = s.id) AS test_count
FROM snapshots s
JOIN (
    SELECT application, MAX(id) AS max_id, COUNT(*) AS cnt
    FROM snapshots
    GROUP BY application
) counts ON s.id = counts.max_id
ORDER BY s.application;

-- name: CreateTestSuite :execlastid
INSERT INTO test_suites (snapshot_id, name, status, pipeline_run, tool_name, tool_version, tests, passed, failed, skipped, pending, other, flaky, start_time, stop_time, duration_ms)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: CreateTestCase :exec
INSERT INTO test_cases (test_suite_id, name, status, duration_ms, message, trace, file_path, suite, retries, flaky)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListTestSuitesBySnapshot :many
SELECT id, snapshot_id, name, status, pipeline_run, tool_name, tool_version, tests, passed, failed, skipped, pending, other, flaky, start_time, stop_time, duration_ms, created_at
FROM test_suites
WHERE snapshot_id = ?
ORDER BY name;

-- name: ListTestCasesBySuite :many
SELECT id, test_suite_id, name, status, duration_ms, message, trace, file_path, suite, retries, flaky
FROM test_cases
WHERE test_suite_id = ?
ORDER BY name;
