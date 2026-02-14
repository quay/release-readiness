package db

import (
	"context"
	"time"

	"github.com/quay/release-readiness/internal/db/sqlc"
	"github.com/quay/release-readiness/internal/model"
)

func (d *DB) CreateSnapshot(ctx context.Context, application, name, triggerComponent, triggerGitSHA, triggerPipelineRun string, testsPassed, released bool, releaseBlockedReason string, createdAt time.Time) (*model.SnapshotRecord, error) {
	id, err := d.queries().CreateSnapshot(ctx, dbsqlc.CreateSnapshotParams{
		Application:          application,
		Name:                 name,
		TriggerComponent:     triggerComponent,
		TriggerGitSha:        triggerGitSHA,
		TriggerPipelineRun:   triggerPipelineRun,
		TestsPassed:          boolToInt64(testsPassed),
		Released:             boolToInt64(released),
		ReleaseBlockedReason: releaseBlockedReason,
		CreatedAt:            createdAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
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

func (d *DB) SnapshotExistsByName(ctx context.Context, name string) (bool, error) {
	count, err := d.queries().SnapshotExistsByName(ctx, name)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *DB) GetSnapshotByName(ctx context.Context, name string) (*model.SnapshotRecord, error) {
	row, err := d.queries().GetSnapshotRow(ctx, name)
	if err != nil {
		return nil, err
	}
	s := toSnapshotRecord(row)

	components, err := d.listSnapshotComponents(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	s.Components = components

	results, err := d.ListSnapshotTestResults(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	s.TestResults = results

	return &s, nil
}

func (d *DB) CreateSnapshotComponent(ctx context.Context, snapshotID int64, component, gitSHA, imageURL, gitURL string) error {
	return d.queries().CreateSnapshotComponent(ctx, dbsqlc.CreateSnapshotComponentParams{
		SnapshotID: snapshotID,
		Component:  component,
		GitSha:     gitSHA,
		ImageUrl:   imageURL,
		GitUrl:     gitURL,
	})
}

func (d *DB) listSnapshotComponents(ctx context.Context, snapshotID int64) ([]model.ComponentRecord, error) {
	rows, err := d.queries().ListSnapshotComponents(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	components := make([]model.ComponentRecord, len(rows))
	for i, r := range rows {
		components[i] = model.ComponentRecord{
			ID:         r.ID,
			SnapshotID: r.SnapshotID,
			Component:  r.Component,
			GitSHA:     r.GitSha,
			ImageURL:   r.ImageUrl,
			GitURL:     r.GitUrl,
		}
	}
	return components, nil
}

func (d *DB) ListSnapshots(ctx context.Context, application string, limit, offset int) ([]model.SnapshotRecord, error) {
	var rows []dbsqlc.Snapshot
	var err error
	if application != "" {
		rows, err = d.queries().ListSnapshotsByApplication(ctx, dbsqlc.ListSnapshotsByApplicationParams{
			Application: application,
			Limit:       int64(limit),
			Offset:      int64(offset),
		})
	} else {
		rows, err = d.queries().ListAllSnapshots(ctx, dbsqlc.ListAllSnapshotsParams{
			Limit:  int64(limit),
			Offset: int64(offset),
		})
	}
	if err != nil {
		return nil, err
	}
	snapshots := make([]model.SnapshotRecord, len(rows))
	for i, r := range rows {
		snapshots[i] = toSnapshotRecord(r)
	}
	return snapshots, nil
}

func (d *DB) LatestSnapshotPerApplication(ctx context.Context) ([]model.ApplicationSummary, error) {
	rows, err := d.queries().LatestSnapshotPerApplication(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]model.ApplicationSummary, len(rows))
	for i, r := range rows {
		s := model.SnapshotRecord{
			ID:                   r.ID,
			Application:          r.Application,
			Name:                 r.Name,
			TriggerComponent:     r.TriggerComponent,
			TriggerGitSHA:        r.TriggerGitSha,
			TriggerPipelineRun:   r.TriggerPipelineRun,
			TestsPassed:          r.TestsPassed == 1,
			Released:             r.Released == 1,
			ReleaseBlockedReason: r.ReleaseBlockedReason,
			CreatedAt:            parseTime(r.CreatedAt),
		}
		summaries[i] = model.ApplicationSummary{
			Application:    r.Application,
			LatestSnapshot: &s,
			SnapshotCount:  int(r.Cnt),
		}
	}
	return summaries, nil
}

func (d *DB) CreateSnapshotTestResult(ctx context.Context, snapshotID int64, scenario, status, pipelineRun string, total, passed, failed, skipped int, durationSec float64) error {
	return d.queries().CreateSnapshotTestResult(ctx, dbsqlc.CreateSnapshotTestResultParams{
		SnapshotID:  snapshotID,
		Scenario:    scenario,
		Status:      status,
		PipelineRun: pipelineRun,
		Total:       int64(total),
		Passed:      int64(passed),
		Failed:      int64(failed),
		Skipped:     int64(skipped),
		DurationSec: durationSec,
	})
}

func (d *DB) ListSnapshotTestResults(ctx context.Context, snapshotID int64) ([]model.SnapshotTestResult, error) {
	rows, err := d.queries().ListSnapshotTestResults(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	results := make([]model.SnapshotTestResult, len(rows))
	for i, r := range rows {
		results[i] = model.SnapshotTestResult{
			ID:          r.ID,
			SnapshotID:  r.SnapshotID,
			Scenario:    r.Scenario,
			Status:      r.Status,
			PipelineRun: r.PipelineRun,
			Total:       int(r.Total),
			Passed:      int(r.Passed),
			Failed:      int(r.Failed),
			Skipped:     int(r.Skipped),
			DurationSec: r.DurationSec,
			CreatedAt:   parseTime(r.CreatedAt),
		}
	}
	return results, nil
}

func toSnapshotRecord(r dbsqlc.Snapshot) model.SnapshotRecord {
	return model.SnapshotRecord{
		ID:                   r.ID,
		Application:          r.Application,
		Name:                 r.Name,
		TriggerComponent:     r.TriggerComponent,
		TriggerGitSHA:        r.TriggerGitSha,
		TriggerPipelineRun:   r.TriggerPipelineRun,
		TestsPassed:          r.TestsPassed == 1,
		Released:             r.Released == 1,
		ReleaseBlockedReason: r.ReleaseBlockedReason,
		CreatedAt:            parseTime(r.CreatedAt),
	}
}
