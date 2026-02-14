package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/quay/release-readiness/internal/db"
	"github.com/quay/release-readiness/internal/jira"
	"github.com/quay/release-readiness/internal/model"
	s3client "github.com/quay/release-readiness/internal/s3"
	"github.com/quay/release-readiness/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: release-readiness <command>

Commands:
  serve    Start the HTTP server
`)
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address")
	dbPath := fs.String("db", "dashboard.db", "SQLite database path")

	// S3 flags
	s3Endpoint := fs.String("s3-endpoint", "", "S3 endpoint URL (e.g. http://localhost:3900)")
	s3Region := fs.String("s3-region", "us-east-1", "S3 region")
	s3Bucket := fs.String("s3-bucket", "", "S3 bucket name")
	s3AccessKey := fs.String("s3-access-key", "", "S3 access key")
	s3SecretKey := fs.String("s3-secret-key", "", "S3 secret key")
	s3PollInterval := fs.Duration("s3-poll-interval", 30*time.Second, "S3 sync poll interval")

	// JIRA flags
	jiraURL := fs.String("jira-url", envOrDefault("JIRA_URL", "https://issues.redhat.com"), "JIRA server URL")
	jiraToken := fs.String("jira-token", os.Getenv("JIRA_TOKEN"), "JIRA personal access token")
	jiraProject := fs.String("jira-project", envOrDefault("JIRA_PROJECT", "PROJQUAY"), "JIRA project key")
	jiraTargetVersionField := fs.String("jira-target-version-field", envOrDefault("JIRA_TARGET_VERSION_FIELD", "customfield_12319940"), "JIRA custom field name for Target Version")
	jiraPollInterval := fs.Duration("jira-poll-interval", 5*time.Minute, "JIRA sync poll interval")

	fs.Parse(args)

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	var s3c *s3client.Client
	if *s3Bucket != "" {
		ctx := context.Background()
		s3c, err = s3client.New(ctx, s3client.Config{
			Endpoint:  *s3Endpoint,
			Region:    *s3Region,
			Bucket:    *s3Bucket,
			AccessKey: *s3AccessKey,
			SecretKey: *s3SecretKey,
		})
		if err != nil {
			log.Fatalf("create s3 client: %v", err)
		}
		log.Printf("s3 sync enabled: bucket=%s endpoint=%s interval=%s", *s3Bucket, *s3Endpoint, *s3PollInterval)
		go runS3SyncLoop(database, s3c, *s3PollInterval)
	}

	// Start JIRA sync if token is configured
	if *jiraToken != "" {
		jiraClient := jira.New(jira.Config{
			BaseURL:            *jiraURL,
			Token:              *jiraToken,
			Project:            *jiraProject,
			TargetVersionField: *jiraTargetVersionField,
		})
		log.Printf("jira sync enabled: url=%s project=%s interval=%s", *jiraURL, *jiraProject, *jiraPollInterval)
		go runJiraSyncLoop(database, jiraClient, *jiraPollInterval)
	}

	srv := server.New(database, s3c, *addr, *jiraURL)
	if err := srv.Run(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// --- S3 Sync ---

func runS3SyncLoop(database *db.DB, s3c *s3client.Client, interval time.Duration) {
	syncS3Once(database, s3c)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		syncS3Once(database, s3c)
	}
}

func syncS3Once(database *db.DB, s3c *s3client.Client) {
	ctx := context.Background()

	apps, err := s3c.ListApplications(ctx)
	if err != nil {
		log.Printf("s3-sync: list applications: %v", err)
		return
	}

	for _, app := range apps {
		keys, err := s3c.ListSnapshots(ctx, app)
		if err != nil {
			log.Printf("s3-sync: list snapshots for %s: %v", app, err)
			continue
		}

		for _, key := range keys {
			snap, err := s3c.GetSnapshot(ctx, key)
			if err != nil {
				log.Printf("s3-sync: get snapshot %s: %v", key, err)
				continue
			}

			exists, err := database.SnapshotExistsByName(snap.Snapshot)
			if err != nil {
				log.Printf("s3-sync: check snapshot %s: %v", snap.Snapshot, err)
				continue
			}
			if exists {
				continue
			}

			log.Printf("s3-sync: new snapshot %s for %s", snap.Snapshot, app)

			if err := ingestSnapshot(database, snap); err != nil {
				log.Printf("s3-sync: ingest snapshot %s: %v", snap.Snapshot, err)
			}
		}
	}
}

func ingestSnapshot(database *db.DB, snap *model.Snapshot) error {
	snapshotRecord, err := database.CreateSnapshot(
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
		if _, err := database.EnsureComponent(comp.Name); err != nil {
			return fmt.Errorf("ensure component %s: %w", comp.Name, err)
		}

		if err := database.CreateSnapshotComponent(snapshotRecord.ID, comp.Name, comp.GitRevision, comp.ContainerImage, comp.GitURL); err != nil {
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

func runJiraSyncLoop(database *db.DB, jiraClient *jira.Client, interval time.Duration) {
	syncJiraOnce(database, jiraClient)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		syncJiraOnce(database, jiraClient)
	}
}

func syncJiraOnce(database *db.DB, jiraClient *jira.Client) {
	ctx := context.Background()

	// Discover active releases from JIRA using the -area/release component
	releases, err := jiraClient.DiscoverActiveReleases(ctx)
	if err != nil {
		log.Printf("jira-sync: discover releases: %v", err)
		return
	}

	log.Printf("jira-sync: discovered %d active releases", len(releases))

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
			S3Application:        rel.S3Application,
			DueDate:              rel.DueDate,
		}

		// Try to get version metadata from JIRA (release date, description, etc.)
		versionInfo, err := jiraClient.GetVersion(ctx, rel.FixVersion)
		if err != nil {
			log.Printf("jira-sync: get version %s: %v (continuing without version metadata)", rel.FixVersion, err)
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

		if err := database.UpsertReleaseVersion(rv); err != nil {
			log.Printf("jira-sync: upsert version %s: %v", rel.FixVersion, err)
		}

		// Sync issues for this fixVersion
		syncJiraVersion(ctx, database, jiraClient, rel.FixVersion)
	}

	// Reconcile unreleased versions in DB that may have been released in
	// JIRA after their tracking ticket was closed (and thus dropped from
	// DiscoverActiveReleases).
	dbVersions, err := database.ListActiveReleaseVersions()
	if err != nil {
		log.Printf("jira-sync: list active db versions: %v", err)
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
				database.UpsertReleaseVersion(&dbv)
				syncJiraVersion(ctx, database, jiraClient, dbv.Name)
				log.Printf("jira-sync: reconciled version %s (released=%v)", dbv.Name, versionInfo.Released)
			}
		}
	}
}

func syncJiraVersion(ctx context.Context, database *db.DB, jiraClient *jira.Client, fixVersion string) {
	issues, err := jiraClient.SearchIssues(ctx, fixVersion)
	if err != nil {
		log.Printf("jira-sync: search issues for %s: %v", fixVersion, err)
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

		if err := database.UpsertJiraIssue(record); err != nil {
			log.Printf("jira-sync: upsert issue %s: %v", issue.Key, err)
		}
	}

	// Remove issues no longer in this fixVersion
	if err := database.DeleteJiraIssuesNotIn(fixVersion, keys); err != nil {
		log.Printf("jira-sync: cleanup issues for %s: %v", fixVersion, err)
	}

	log.Printf("jira-sync: synced %d issues for fixVersion %s", len(issues), fixVersion)
}
