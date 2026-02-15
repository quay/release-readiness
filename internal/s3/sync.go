package s3

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"time"

	"github.com/quay/release-readiness/internal/model"
)

// Store is the subset of the database layer needed by the S3 syncer.
type Store interface {
	SnapshotExistsByName(ctx context.Context, name string) (bool, error)
	CreateSnapshot(ctx context.Context, application, name, triggerComponent, triggerGitSHA, triggerPipelineRun string, testsPassed, released bool, releaseBlockedReason string, createdAt time.Time) (*model.SnapshotRecord, error)
	EnsureComponent(ctx context.Context, name string) (*model.Component, error)
	CreateSnapshotComponent(ctx context.Context, snapshotID int64, component, gitSHA, imageURL, gitURL string) error
	CreateSnapshotTestResult(ctx context.Context, snapshotID int64, scenario, status, pipelineRun string, total, passed, failed, skipped int, durationSec float64) error
}

// Syncer orchestrates periodic S3 snapshot synchronisation into a Store.
type Syncer struct {
	client *Client
	store  Store
	logger *slog.Logger
}

// NewSyncer creates a Syncer that uses client to fetch data and store to persist it.
func NewSyncer(client *Client, store Store, logger *slog.Logger) *Syncer {
	return &Syncer{client: client, store: store, logger: logger}
}

// Run performs an immediate sync and then repeats every interval until ctx is cancelled.
func (s *Syncer) Run(ctx context.Context, interval time.Duration) {
	s.SyncOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("stopping")
			return
		case <-ticker.C:
			s.SyncOnce(ctx)
		}
	}
}

// SyncOnce discovers all applications and ingests any new snapshots.
func (s *Syncer) SyncOnce(ctx context.Context) {
	apps, err := s.client.ListApplications(ctx)
	if err != nil {
		s.logger.Error("list applications", "error", err)
		return
	}

	for _, app := range apps {
		keys, err := s.client.ListSnapshots(ctx, app)
		if err != nil {
			s.logger.Error("list snapshots", "application", app, "error", err)
			continue
		}

		for _, key := range keys {
			snap, err := s.client.GetSnapshot(ctx, key)
			if err != nil {
				s.logger.Error("get snapshot", "key", key, "error", err)
				continue
			}

			exists, err := s.store.SnapshotExistsByName(ctx, snap.Snapshot)
			if err != nil {
				s.logger.Error("check snapshot", "snapshot", snap.Snapshot, "error", err)
				continue
			}
			if exists {
				continue
			}

			s.logger.Info("new snapshot", "snapshot", snap.Snapshot, "application", app)

			if err := s.ingest(ctx, key, snap); err != nil {
				s.logger.Error("ingest snapshot", "snapshot", snap.Snapshot, "error", err)
			}
		}
	}
}

// ingest persists a single snapshot and its components/test results into the store.
func (s *Syncer) ingest(ctx context.Context, key string, snap *model.Snapshot) error {
	// Derive the snapshot directory prefix from the key.
	// key is like "{app}/snapshots/{snapshot-name}/snapshot.json"
	// We need "{app}/snapshots/{snapshot-name}/" as the base for JUnit paths.
	snapshotDir := path.Dir(key) + "/"

	// Fetch JUnit data for each test result before writing to DB.
	for i, tr := range snap.TestResults {
		junitPrefix := snapshotDir + "junit/" + tr.Scenario + "/"
		result, err := s.client.GetTestResults(ctx, junitPrefix)
		if err != nil {
			s.logger.Debug("no junit data", "scenario", tr.Scenario, "path", junitPrefix)
			continue
		}
		snap.TestResults[i].Summary = &model.TestSummary{
			Total:       result.Total,
			Passed:      result.Passed,
			Failed:      result.Failed,
			Skipped:     result.Skipped,
			DurationSec: result.DurationSec,
		}
	}

	snapshotRecord, err := s.store.CreateSnapshot(
		ctx,
		snap.Application,
		snap.Snapshot,
		snap.Trigger.Component,
		snap.Trigger.GitSHA,
		snap.Trigger.PipelineRun,
		snap.Readiness.TestsPassed,
		snap.Readiness.Released,
		snap.Readiness.ReleaseBlockedReason,
		snap.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	for _, comp := range snap.Components {
		if _, err := s.store.EnsureComponent(ctx, comp.Name); err != nil {
			return fmt.Errorf("ensure component %s: %w", comp.Name, err)
		}

		if err := s.store.CreateSnapshotComponent(ctx, snapshotRecord.ID, comp.Name, comp.GitRevision, comp.ContainerImage, comp.GitURL); err != nil {
			return fmt.Errorf("create snapshot component %s: %w", comp.Name, err)
		}
	}

	for _, tr := range snap.TestResults {
		total, passed, failed, skipped := 0, 0, 0, 0
		var durationSec float64
		if tr.Summary != nil {
			total = tr.Summary.Total
			passed = tr.Summary.Passed
			failed = tr.Summary.Failed
			skipped = tr.Summary.Skipped
			durationSec = tr.Summary.DurationSec
		}

		if err := s.store.CreateSnapshotTestResult(
			ctx,
			snapshotRecord.ID,
			tr.Scenario,
			tr.Status,
			tr.PipelineRun,
			total, passed, failed, skipped,
			durationSec,
		); err != nil {
			return fmt.Errorf("create snapshot test result %s: %w", tr.Scenario, err)
		}
	}

	return nil
}
