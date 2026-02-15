package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"time"

	"github.com/quay/release-readiness/internal/db"
	"github.com/quay/release-readiness/internal/jira"
	"github.com/quay/release-readiness/internal/model"
	s3client "github.com/quay/release-readiness/internal/s3"
	"github.com/quay/release-readiness/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "dashboard.db", "SQLite database path")

	// S3 flags
	s3Endpoint := flag.String("s3-endpoint", os.Getenv("S3_ENDPOINT"), "S3 endpoint URL (e.g. http://localhost:3900)")
	s3Region := flag.String("s3-region", envOrDefault("S3_REGION", "us-east-1"), "S3 region")
	s3Bucket := flag.String("s3-bucket", os.Getenv("S3_BUCKET"), "S3 bucket name")
	s3AccessKey := flag.String("s3-access-key", os.Getenv("AWS_ACCESS_KEY_ID"), "S3 access key")
	s3SecretKey := flag.String("s3-secret-key", os.Getenv("AWS_SECRET_ACCESS_KEY"), "S3 secret key")
	s3PollInterval := flag.Duration("s3-poll-interval", 30*time.Second, "S3 sync poll interval")

	// JIRA flags
	jiraURL := flag.String("jira-url", envOrDefault("JIRA_URL", "https://issues.redhat.com"), "JIRA server URL")
	jiraToken := flag.String("jira-token", os.Getenv("JIRA_TOKEN"), "JIRA personal access token")
	jiraProject := flag.String("jira-project", envOrDefault("JIRA_PROJECT", "PROJQUAY"), "JIRA project key")
	jiraTargetVersionField := flag.String("jira-target-version-field", envOrDefault("JIRA_TARGET_VERSION_FIELD", "customfield_12319940"), "JIRA custom field name for Target Version")
	jiraPollInterval := flag.Duration("jira-poll-interval", 5*time.Minute, "JIRA sync poll interval")

	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(*dbPath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()

	var wg sync.WaitGroup

	var s3c *s3client.Client
	if *s3Bucket != "" {
		s3Log := logger.With("component", "s3-sync")
		s3c, err = s3client.New(ctx, s3client.Config{
			Endpoint:  *s3Endpoint,
			Region:    *s3Region,
			Bucket:    *s3Bucket,
			AccessKey: *s3AccessKey,
			SecretKey: *s3SecretKey,
		}, s3Log)
		if err != nil {
			logger.Error("create s3 client", "error", err)
			os.Exit(1)
		}
		logger.Info("s3 sync enabled", "bucket", *s3Bucket, "endpoint", *s3Endpoint, "interval", *s3PollInterval)
		wg.Add(1)
		go func() {
			defer wg.Done()
			runS3SyncLoop(ctx, database, s3c, *s3PollInterval, s3Log)
		}()
	}

	// Start JIRA sync if token is configured
	if *jiraToken != "" {
		jiraClient := jira.New(jira.Config{
			BaseURL:            *jiraURL,
			Token:              *jiraToken,
			Project:            *jiraProject,
			TargetVersionField: *jiraTargetVersionField,
		})
		jiraLog := logger.With("component", "jira-sync")
		logger.Info("jira sync enabled", "url", *jiraURL, "project", *jiraProject, "interval", *jiraPollInterval)
		syncer := jira.NewSyncer(jiraClient, database, jiraLog)
		wg.Add(1)
		go func() {
			defer wg.Done()
			syncer.Run(ctx, *jiraPollInterval)
		}()
	}

	srv := server.New(database, s3c, *addr, *jiraURL, logger)
	if err := srv.Run(ctx); err != nil {
		logger.Error("server", "error", err)
		os.Exit(1)
	}

	wg.Wait()
	logger.Info("all background tasks stopped")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// --- S3 Sync ---

func runS3SyncLoop(ctx context.Context, database *db.DB, s3c *s3client.Client, interval time.Duration, logger *slog.Logger) {
	syncS3Once(ctx, database, s3c, logger)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("stopping")
			return
		case <-ticker.C:
			syncS3Once(ctx, database, s3c, logger)
		}
	}
}

func syncS3Once(ctx context.Context, database *db.DB, s3c *s3client.Client, logger *slog.Logger) {
	apps, err := s3c.ListApplications(ctx)
	if err != nil {
		logger.Error("list applications", "error", err)
		return
	}

	for _, app := range apps {
		keys, err := s3c.ListSnapshots(ctx, app)
		if err != nil {
			logger.Error("list snapshots", "application", app, "error", err)
			continue
		}

		for _, key := range keys {
			snap, err := s3c.GetSnapshot(ctx, key)
			if err != nil {
				logger.Error("get snapshot", "key", key, "error", err)
				continue
			}

			exists, err := database.SnapshotExistsByName(ctx, snap.Snapshot)
			if err != nil {
				logger.Error("check snapshot", "snapshot", snap.Snapshot, "error", err)
				continue
			}
			if exists {
				continue
			}

			logger.Info("new snapshot", "snapshot", snap.Snapshot, "application", app)

			if err := ingestSnapshot(ctx, database, s3c, key, snap, logger); err != nil {
				logger.Error("ingest snapshot", "snapshot", snap.Snapshot, "error", err)
			}
		}
	}
}

func ingestSnapshot(ctx context.Context, database *db.DB, s3c *s3client.Client, key string, snap *model.Snapshot, logger *slog.Logger) error {
	// Derive the snapshot directory prefix from the key.
	// key is like "{app}/snapshots/{snapshot-name}/snapshot.json"
	// We need "{app}/snapshots/{snapshot-name}/" as the base for JUnit paths.
	snapshotDir := path.Dir(key) + "/"

	// Fetch JUnit data for each test result before writing to DB.
	for i, tr := range snap.TestResults {
		junitPrefix := snapshotDir + "junit/" + tr.Scenario + "/"
		result, err := s3c.GetTestResults(ctx, junitPrefix)
		if err != nil {
			logger.Debug("no junit data", "scenario", tr.Scenario, "path", junitPrefix)
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

	snapshotRecord, err := database.CreateSnapshot(
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
		if _, err := database.EnsureComponent(ctx, comp.Name); err != nil {
			return fmt.Errorf("ensure component %s: %w", comp.Name, err)
		}

		if err := database.CreateSnapshotComponent(ctx, snapshotRecord.ID, comp.Name, comp.GitRevision, comp.ContainerImage, comp.GitURL); err != nil {
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

		if err := database.CreateSnapshotTestResult(
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

