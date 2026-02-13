package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/quay/build-dashboard/internal/model"
)

func (d *DB) CreateBuild(component, version, gitSHA, gitBranch, imageURL, imageDigest, pipelineRun, snapshotName string) (*model.Build, error) {
	comp, err := d.GetComponentByName(component)
	if err != nil {
		return nil, fmt.Errorf("component %q not found: %w", component, err)
	}

	res, err := d.Exec(`
		INSERT INTO builds (component_id, version, git_sha, git_branch, image_url, image_digest, pipeline_run, snapshot_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		comp.ID, version, gitSHA, gitBranch, imageURL, imageDigest, pipelineRun, snapshotName)
	if err != nil {
		return nil, err
	}

	id, _ := res.LastInsertId()
	return &model.Build{
		ID:           id,
		ComponentID:  comp.ID,
		Component:    comp.Name,
		Version:      version,
		GitSHA:       gitSHA,
		GitBranch:    gitBranch,
		ImageURL:     imageURL,
		ImageDigest:  imageDigest,
		PipelineRun:  pipelineRun,
		SnapshotName: snapshotName,
		Status:       "pending",
		CreatedAt:    time.Now().UTC(),
	}, nil
}

func (d *DB) GetBuild(id int64) (*model.Build, error) {
	b, err := d.scanBuild(d.QueryRow(`
		SELECT b.id, b.component_id, c.name, b.version, b.git_sha, b.git_branch,
		       b.image_url, b.image_digest, b.pipeline_run, b.snapshot_name, b.status, b.created_at
		FROM builds b
		JOIN components c ON c.id = b.component_id
		WHERE b.id = ?`, id))
	if err != nil {
		return nil, err
	}

	runs, err := d.ListTestRuns(id)
	if err != nil {
		return nil, err
	}
	b.TestRuns = runs
	return b, nil
}

func (d *DB) GetLatestBuild(component, version string) (*model.Build, error) {
	query := `
		SELECT b.id, b.component_id, c.name, b.version, b.git_sha, b.git_branch,
		       b.image_url, b.image_digest, b.pipeline_run, b.snapshot_name, b.status, b.created_at
		FROM builds b
		JOIN components c ON c.id = b.component_id
		WHERE c.name = ?`
	args := []interface{}{component}
	if version != "" {
		query += ` AND b.version = ?`
		args = append(args, version)
	}
	query += ` ORDER BY b.created_at DESC LIMIT 1`
	return d.scanBuild(d.QueryRow(query, args...))
}

func (d *DB) ListBuilds(component, version, status string, limit, offset int) ([]model.Build, error) {
	var where []string
	var args []interface{}

	if component != "" {
		where = append(where, "c.name = ?")
		args = append(args, component)
	}
	if version != "" {
		where = append(where, "b.version = ?")
		args = append(args, version)
	}
	if status != "" {
		where = append(where, "b.status = ?")
		args = append(args, status)
	}

	query := `
		SELECT b.id, b.component_id, c.name, b.version, b.git_sha, b.git_branch,
		       b.image_url, b.image_digest, b.pipeline_run, b.snapshot_name, b.status, b.created_at
		FROM builds b
		JOIN components c ON c.id = b.component_id`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY b.created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var builds []model.Build
	for rows.Next() {
		var b model.Build
		var ts string
		if err := rows.Scan(&b.ID, &b.ComponentID, &b.Component, &b.Version, &b.GitSHA,
			&b.GitBranch, &b.ImageURL, &b.ImageDigest, &b.PipelineRun, &b.SnapshotName,
			&b.Status, &ts); err != nil {
			return nil, err
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		builds = append(builds, b)
	}
	return builds, rows.Err()
}

func (d *DB) LatestBuildsPerComponent(version string) ([]model.Build, error) {
	query := `
		SELECT b.id, b.component_id, c.name, b.version, b.git_sha, b.git_branch,
		       b.image_url, b.image_digest, b.pipeline_run, b.snapshot_name, b.status, b.created_at
		FROM builds b
		JOIN components c ON c.id = b.component_id
		WHERE b.id IN (
			SELECT MAX(b2.id) FROM builds b2
			WHERE b2.version = ?
			GROUP BY b2.component_id
		)
		ORDER BY c.name`
	rows, err := d.Query(query, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var builds []model.Build
	for rows.Next() {
		var b model.Build
		var ts string
		if err := rows.Scan(&b.ID, &b.ComponentID, &b.Component, &b.Version, &b.GitSHA,
			&b.GitBranch, &b.ImageURL, &b.ImageDigest, &b.PipelineRun, &b.SnapshotName,
			&b.Status, &ts); err != nil {
			return nil, err
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		builds = append(builds, b)
	}
	return builds, rows.Err()
}

func (d *DB) RecomputeBuildStatus(buildID int64) error {
	var total, failed int
	err := d.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) FROM test_runs WHERE build_id = ?`, buildID).
		Scan(&total, &failed)
	if err != nil {
		return err
	}

	status := "pending"
	if total > 0 {
		if failed > 0 {
			status = "failed"
		} else {
			status = "passed"
		}
	}

	_, err = d.Exec(`UPDATE builds SET status = ? WHERE id = ?`, status, buildID)
	return err
}

func (d *DB) ActiveVersions() ([]string, error) {
	rows, err := d.Query(`SELECT DISTINCT version FROM builds ORDER BY version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (d *DB) VersionSummaries() ([]model.VersionSummary, error) {
	versions, err := d.ActiveVersions()
	if err != nil {
		return nil, err
	}

	var summaries []model.VersionSummary
	for _, v := range versions {
		var buildCount int
		d.QueryRow(`SELECT COUNT(*) FROM builds WHERE version = ?`, v).Scan(&buildCount)

		matrix, err := d.GetReadiness(v)
		if err != nil {
			summaries = append(summaries, model.VersionSummary{
				Version:    v,
				BuildCount: buildCount,
			})
			continue
		}
		summaries = append(summaries, model.VersionSummary{
			Version:    v,
			BuildCount: buildCount,
			Ready:      matrix.Ready,
			BlockCount: matrix.BlockCount,
		})
	}
	return summaries, nil
}

func (d *DB) scanBuild(row *sql.Row) (*model.Build, error) {
	var b model.Build
	var ts string
	if err := row.Scan(&b.ID, &b.ComponentID, &b.Component, &b.Version, &b.GitSHA,
		&b.GitBranch, &b.ImageURL, &b.ImageDigest, &b.PipelineRun, &b.SnapshotName,
		&b.Status, &ts); err != nil {
		return nil, err
	}
	b.CreatedAt, _ = time.Parse(time.RFC3339, ts)
	return &b, nil
}
