-- name: CreateSnapshot :execlastid
INSERT INTO snapshots (application, name, trigger_component, trigger_git_sha, trigger_pipeline_run, tests_passed, released, release_blocked_reason, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: SnapshotExistsByName :one
SELECT COUNT(*) FROM snapshots WHERE name = ?;

-- name: GetSnapshotRow :one
SELECT id, application, name, trigger_component, trigger_git_sha, trigger_pipeline_run,
       tests_passed, released, release_blocked_reason, created_at
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
SELECT id, application, name, trigger_component, trigger_git_sha, trigger_pipeline_run,
       tests_passed, released, release_blocked_reason, created_at
FROM snapshots
ORDER BY id DESC LIMIT ? OFFSET ?;

-- name: ListSnapshotsByApplication :many
SELECT id, application, name, trigger_component, trigger_git_sha, trigger_pipeline_run,
       tests_passed, released, release_blocked_reason, created_at
FROM snapshots
WHERE application = ?
ORDER BY id DESC LIMIT ? OFFSET ?;

-- name: LatestSnapshotPerApplication :many
SELECT s.id, s.application, s.name, s.trigger_component, s.trigger_git_sha, s.trigger_pipeline_run,
       s.tests_passed, s.released, s.release_blocked_reason, s.created_at, CAST(counts.cnt AS INTEGER) AS cnt
FROM snapshots s
JOIN (
    SELECT application, MAX(id) AS max_id, COUNT(*) AS cnt
    FROM snapshots
    GROUP BY application
) counts ON s.id = counts.max_id
ORDER BY s.application;

-- name: CreateSnapshotTestResult :exec
INSERT INTO snapshot_test_results (snapshot_id, scenario, status, pipeline_run, total, passed, failed, skipped, duration_sec)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListSnapshotTestResults :many
SELECT id, snapshot_id, scenario, status, pipeline_run, total, passed, failed, skipped, duration_sec, created_at
FROM snapshot_test_results
WHERE snapshot_id = ?
ORDER BY scenario;
