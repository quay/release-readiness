package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"strings"
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
		wg.Add(1)
		go func() {
			defer wg.Done()
			runJiraSyncLoop(ctx, database, jiraClient, *jiraPollInterval, jiraLog)
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

// --- JIRA Sync ---

func runJiraSyncLoop(ctx context.Context, database *db.DB, jiraClient *jira.Client, interval time.Duration, logger *slog.Logger) {
	syncJiraOnce(ctx, database, jiraClient, logger)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("stopping")
			return
		case <-ticker.C:
			syncJiraOnce(ctx, database, jiraClient, logger)
		}
	}
}

func syncJiraOnce(ctx context.Context, database *db.DB, jiraClient *jira.Client, logger *slog.Logger) {
	// Discover active releases from JIRA using the -area/release component
	releases, err := jiraClient.DiscoverActiveReleases(ctx)
	if err != nil {
		logger.Error("discover releases", "error", err)
		return
	}

	logger.Info("discovered active releases", "count", len(releases))

	// Track which versions were actively synced this cycle
	activeSet := make(map[string]bool, len(releases))

	// Process each discovered release sequentially to avoid rate limiting
	for _, rel := range releases {
		activeSet[rel.FixVersion] = true

		// Store the discovered release version
		rv := &model.ReleaseVersion{
			Name:                  rel.FixVersion,
			ReleaseTicketKey:      rel.ReleaseTicketKey,
			ReleaseTicketAssignee: rel.Assignee,
			S3Application:         rel.S3Application,
			DueDate:               rel.DueDate,
		}

		// Try to get version metadata from JIRA (release date, description, etc.)
		versionInfo, err := jiraClient.GetVersion(ctx, rel.FixVersion)
		if err != nil {
			logger.Warn("get version metadata", "version", rel.FixVersion, "error", err)
		} else {
			rv.Description = versionInfo.Description
			rv.Released = versionInfo.Released
			rv.Archived = versionInfo.Archived
			if versionInfo.ReleaseDate != "" {
				t, err := time.Parse("2006-01-02", versionInfo.ReleaseDate)
				if err == nil {
					rv.ReleaseDate = &t
				}
			}
		}

		if err := database.UpsertReleaseVersion(ctx, rv); err != nil {
			logger.Error("upsert version", "version", rel.FixVersion, "error", err)
		}

		// Sync issues for this fixVersion
		syncJiraVersion(ctx, database, jiraClient, rel.FixVersion, logger)
	}

	// Reconcile unreleased versions in DB that may have been released in
	// JIRA after their tracking ticket was closed (and thus dropped from
	// DiscoverActiveReleases).
	dbVersions, err := database.ListActiveReleaseVersions(ctx)
	if err != nil {
		logger.Error("list active db versions", "error", err)
	} else {
		for _, dbv := range dbVersions {
			if activeSet[dbv.Name] {
				continue // already synced this cycle
			}
			versionInfo, err := jiraClient.GetVersion(ctx, dbv.Name)
			if err != nil {
				continue // version may not exist in JIRA
			}
			if versionInfo.Released || versionInfo.Archived {
				dbv.Released = versionInfo.Released
				dbv.Archived = versionInfo.Archived
				if versionInfo.ReleaseDate != "" {
					t, err := time.Parse("2006-01-02", versionInfo.ReleaseDate)
					if err == nil {
						dbv.ReleaseDate = &t
					}
				}
				if err := database.UpsertReleaseVersion(ctx, &dbv); err != nil {
					logger.Error("upsert version", "version", dbv.Name, "error", err)
				}
				syncJiraVersion(ctx, database, jiraClient, dbv.Name, logger)
				logger.Info("reconciled version", "version", dbv.Name, "released", versionInfo.Released)
			}
		}
	}
}

func syncJiraVersion(ctx context.Context, database *db.DB, jiraClient *jira.Client, fixVersion string, logger *slog.Logger) {
	issues, err := jiraClient.SearchIssues(ctx, fixVersion)
	if err != nil {
		logger.Error("search issues", "version", fixVersion, "error", err)
		return
	}

	var keys []string
	for _, issue := range issues {
		keys = append(keys, issue.Key)

		labels := strings.Join(issue.Fields.Labels, ",")
		assignee := ""
		if issue.Fields.Assignee != nil {
			assignee = issue.Fields.Assignee.DisplayName
		}
		resolution := ""
		if issue.Fields.Resolution != nil {
			resolution = issue.Fields.Resolution.Name
		}

		updatedAt, _ := time.Parse("2006-01-02T15:04:05.000-0700", issue.Fields.Updated)
		if updatedAt.IsZero() {
			updatedAt = time.Now().UTC()
		}

		jiraURL := ""
		if jiraClient != nil {
			jiraURL = fmt.Sprintf("%s/browse/%s", strings.TrimRight(jiraClient.BaseURL(), "/"), issue.Key)
		}

		record := &model.JiraIssueRecord{
			Key:        issue.Key,
			Summary:    issue.Fields.Summary,
			Status:     issue.Fields.Status.Name,
			Priority:   issue.Fields.Priority.Name,
			Labels:     labels,
			FixVersion: fixVersion,
			Assignee:   assignee,
			IssueType:  issue.Fields.IssueType.Name,
			Resolution: resolution,
			Link:       jiraURL,
			UpdatedAt:  updatedAt,
		}

		if err := database.UpsertJiraIssue(ctx, record); err != nil {
			logger.Error("upsert issue", "key", issue.Key, "error", err)
		}
	}

	// Remove issues no longer in this fixVersion
	if err := database.DeleteJiraIssuesNotIn(ctx, fixVersion, keys); err != nil {
		logger.Error("cleanup issues", "version", fixVersion, "error", err)
	}

	logger.Info("synced issues", "count", len(issues), "version", fixVersion)
}
