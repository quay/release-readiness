-- name: MarkProwJobsRemoved :exec
UPDATE prow_job_configs SET removed = 1;

-- name: UpsertProwJobConfig :exec
INSERT INTO prow_job_configs(job_name, application, target, revision, source_url, ocp, cloud, removed)
VALUES (?, ?, ?, ?, ?, ?, ?, 0)
ON CONFLICT(job_name) DO UPDATE SET application=excluded.application, target=excluded.target,
 revision=excluded.revision, source_url=excluded.source_url, ocp=excluded.ocp,
 cloud=excluded.cloud, removed=0;

-- name: UpsertProwInventoryRevision :exec
INSERT INTO prow_inventory_state(id, revision) VALUES(1, ?)
ON CONFLICT(id) DO UPDATE SET revision=excluded.revision;

-- name: ListProwJobConfigs :many
SELECT c.*, s.revision AS inventory_revision
FROM prow_job_configs c CROSS JOIN prow_inventory_state s
WHERE s.id = 1 ORDER BY c.job_name;

-- name: ListCIWindowFBCs :many
SELECT nvr, start_time FROM art_history_records
WHERE group_name = ? AND build_type = 'fbc' ORDER BY start_time;
