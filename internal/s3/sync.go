package s3

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"time"

	"github.com/quay/release-readiness/internal/ctrf"
	"github.com/quay/release-readiness/internal/model"
)

// Store is the subset of the database layer needed by the S3 syncer.
type Store interface {
	SnapshotExistsByName(ctx context.Context, name string) (bool, error)
	CreateSnapshot(ctx context.Context, application, name string, testsPassed bool, createdAt time.Time) (*model.SnapshotRecord, error)
	EnsureComponent(ctx context.Context, name string) (*model.Component, error)
	CreateSnapshotComponent(ctx context.Context, snapshotID int64, component, gitSHA, imageURL, gitURL string) error
	CreateTestSuite(ctx context.Context, snapshotID int64, name, status, pipelineRun, toolName, toolVersion string, tests, passed, failed, skipped, pending, other, flaky int, startTime, stopTime, durationMs int64) (int64, error)
	CreateTestCase(ctx context.Context, testSuiteID int64, name, status string, durationMs float64, message, trace, filePath, suite string, retries int, flaky bool) error
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
				s.logger.Debug("skipping snapshot", "key", key, "error", err)
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

type suiteData struct {
	name   string
	report *ctrf.Report
}

// ingest persists a single snapshot and its components/test results into the store.
func (s *Syncer) ingest(ctx context.Context, key string, snap *model.Snapshot) error {
	// Derive the snapshot directory prefix from the key.
	// key is like "{app}/snapshots/{snapshot-name}/snapshot.json"
	snapshotDir := path.Dir(key) + "/"

	// Discover test suites from S3 and fetch CTRF reports to determine testsPassed.
	suiteNames, err := s.client.ListTestSuites(ctx, snapshotDir)
	if err != nil {
		s.logger.Debug("no test suites found", "snapshot", snap.Snapshot, "error", err)
	}

	var suites []suiteData
	testsPassed := len(suiteNames) > 0
	for _, name := range suiteNames {
		ctrfPath := snapshotDir + name + "/results/ctrf-report.json"
		report, err := s.client.GetCTRFReport(ctx, ctrfPath)
		if err != nil {
			s.logger.Debug("failed to fetch ctrf report", "suite", name, "error", err)
			continue
		}
		suites = append(suites, suiteData{name: name, report: report})
		if report.Results.Summary.Failed > 0 {
			testsPassed = false
		}
	}
	if len(suites) == 0 {
		testsPassed = false
	}

	snapshotRecord, err := s.store.CreateSnapshot(
		ctx,
		snap.Application,
		snap.Snapshot,
		testsPassed,
		time.Now().UTC(),
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

	for _, sd := range suites {
		status := "passed"
		if sd.report.Results.Summary.Failed > 0 {
			status = "failed"
		}

		sum := sd.report.Results.Summary
		suiteID, err := s.store.CreateTestSuite(
			ctx, snapshotRecord.ID,
			sd.name, status, "",
			sd.report.Results.Tool.Name, sd.report.Results.Tool.Version,
			sum.Tests, sum.Passed, sum.Failed, sum.Skipped,
			sum.Pending, sum.Other, sum.Flaky,
			sum.Start, sum.Stop, sum.Stop-sum.Start,
		)
		if err != nil {
			return fmt.Errorf("create test suite %s: %w", sd.name, err)
		}

		for _, tc := range sd.report.Results.Tests {
			if err := s.store.CreateTestCase(
				ctx, suiteID,
				tc.Name, tc.Status, tc.Duration,
				tc.Message, tc.Trace, tc.FilePath, tc.Suite,
				tc.Retries, tc.Flaky,
			); err != nil {
				return fmt.Errorf("create test case %s: %w", tc.Name, err)
			}
		}
	}

	return nil
}
