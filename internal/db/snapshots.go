package db

import (
	"time"

	"github.com/quay/release-readiness/internal/model"
)

func (d *DB) CreateSnapshot(application, name, triggerComponent, triggerGitSHA, triggerPipelineRun string, testsPassed, released bool, releaseBlockedReason string, createdAt time.Time) (*model.SnapshotRecord, error) {
	tp, rel := 0, 0
	if testsPassed {
		tp = 1
	}
	if released {
		rel = 1
	}

	res, err := d.Exec(`
		INSERT INTO snapshots (application, name, trigger_component, trigger_git_sha, trigger_pipeline_run, tests_passed, released, release_blocked_reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		application, name, triggerComponent, triggerGitSHA, triggerPipelineRun, tp, rel, releaseBlockedReason, createdAt.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	id, _ := res.LastInsertId()
	return &model.SnapshotRecord{
		ID:                   id,
		Application:          application,
		Name:                 name,
		TriggerComponent:     triggerComponent,
		TriggerGitSHA:        triggerGitSHA,
		TriggerPipelineRun:   triggerPipelineRun,
		TestsPassed:          testsPassed,
		Released:             released,
		ReleaseBlockedReason: releaseBlockedReason,
		CreatedAt:            createdAt.UTC(),
	}, nil
}

func (d *DB) SnapshotExistsByName(name string) (bool, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM snapshots WHERE name = ?`, name).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *DB) GetSnapshotByName(name string) (*model.SnapshotRecord, error) {
	var s model.SnapshotRecord
	var ts string
	var tp, rel int
	err := d.QueryRow(`
		SELECT id, application, name, trigger_component, trigger_git_sha, trigger_pipeline_run,
		       tests_passed, released, release_blocked_reason, created_at
		FROM snapshots WHERE name = ?`, name).
		Scan(&s.ID, &s.Application, &s.Name, &s.TriggerComponent, &s.TriggerGitSHA,
			&s.TriggerPipelineRun, &tp, &rel, &s.ReleaseBlockedReason, &ts)
	if err != nil {
		return nil, err
	}
	s.TestsPassed = tp == 1
	s.Released = rel == 1
	s.CreatedAt, _ = time.Parse(time.RFC3339, ts)

	components, err := d.listSnapshotComponents(s.ID)
	if err != nil {
		return nil, err
	}
	s.Components = components

	results, err := d.ListSnapshotTestResults(s.ID)
	if err != nil {
		return nil, err
	}
	s.TestResults = results

	return &s, nil
}

func (d *DB) CreateSnapshotComponent(snapshotID int64, component, gitSHA, imageURL, gitURL string) error {
	_, err := d.Exec(`
		INSERT INTO snapshot_components (snapshot_id, component, git_sha, image_url, git_url)
		VALUES (?, ?, ?, ?, ?)`,
		snapshotID, component, gitSHA, imageURL, gitURL)
	return err
}

func (d *DB) listSnapshotComponents(snapshotID int64) ([]model.ComponentRecord, error) {
	rows, err := d.Query(`
		SELECT id, snapshot_id, component, git_sha, image_url, git_url
		FROM snapshot_components
		WHERE snapshot_id = ?
		ORDER BY component`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var components []model.ComponentRecord
	for rows.Next() {
		var c model.ComponentRecord
		if err := rows.Scan(&c.ID, &c.SnapshotID, &c.Component, &c.GitSHA, &c.ImageURL, &c.GitURL); err != nil {
			return nil, err
		}
		components = append(components, c)
	}
	return components, rows.Err()
}

func (d *DB) ListSnapshots(application string, limit, offset int) ([]model.SnapshotRecord, error) {
	query := `SELECT id, application, name, trigger_component, trigger_git_sha, trigger_pipeline_run,
	                 tests_passed, released, release_blocked_reason, created_at
	          FROM snapshots`
	var args []interface{}
	if application != "" {
		query += ` WHERE application = ?`
		args = append(args, application)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []model.SnapshotRecord
	for rows.Next() {
		var s model.SnapshotRecord
		var ts string
		var tp, rel int
		if err := rows.Scan(&s.ID, &s.Application, &s.Name, &s.TriggerComponent, &s.TriggerGitSHA,
			&s.TriggerPipelineRun, &tp, &rel, &s.ReleaseBlockedReason, &ts); err != nil {
			return nil, err
		}
		s.TestsPassed = tp == 1
		s.Released = rel == 1
		s.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		snapshots = append(snapshots, s)
	}
	return snapshots, rows.Err()
}

func (d *DB) LatestSnapshotPerApplication() ([]model.ApplicationSummary, error) {
	rows, err := d.Query(`
		SELECT s.id, s.application, s.name, s.trigger_component, s.trigger_git_sha, s.trigger_pipeline_run,
		       s.tests_passed, s.released, s.release_blocked_reason, s.created_at, counts.cnt
		FROM snapshots s
		JOIN (
			SELECT application, MAX(id) AS max_id, COUNT(*) AS cnt
			FROM snapshots
			GROUP BY application
		) counts ON s.id = counts.max_id
		ORDER BY s.application`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []model.ApplicationSummary
	for rows.Next() {
		var s model.SnapshotRecord
		var ts string
		var tp, rel int
		var count int
		if err := rows.Scan(&s.ID, &s.Application, &s.Name, &s.TriggerComponent, &s.TriggerGitSHA,
			&s.TriggerPipelineRun, &tp, &rel, &s.ReleaseBlockedReason, &ts, &count); err != nil {
			return nil, err
		}
		s.TestsPassed = tp == 1
		s.Released = rel == 1
		s.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		summaries = append(summaries, model.ApplicationSummary{
			Application:    s.Application,
			LatestSnapshot: &s,
			SnapshotCount:  count,
		})
	}
	return summaries, rows.Err()
}

func (d *DB) CreateSnapshotTestResult(snapshotID int64, scenario, status, pipelineRun string, total, passed, failed, skipped int, durationSec float64) error {
	_, err := d.Exec(`
		INSERT INTO snapshot_test_results (snapshot_id, scenario, status, pipeline_run, total, passed, failed, skipped, duration_sec)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshotID, scenario, status, pipelineRun, total, passed, failed, skipped, durationSec)
	return err
}

func (d *DB) ListSnapshotTestResults(snapshotID int64) ([]model.SnapshotTestResult, error) {
	rows, err := d.Query(`
		SELECT id, snapshot_id, scenario, status, pipeline_run, total, passed, failed, skipped, duration_sec, created_at
		FROM snapshot_test_results
		WHERE snapshot_id = ?
		ORDER BY scenario`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.SnapshotTestResult
	for rows.Next() {
		var r model.SnapshotTestResult
		var ts string
		if err := rows.Scan(&r.ID, &r.SnapshotID, &r.Scenario, &r.Status, &r.PipelineRun,
			&r.Total, &r.Passed, &r.Failed, &r.Skipped, &r.DurationSec, &ts); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		results = append(results, r)
	}
	return results, rows.Err()
}
