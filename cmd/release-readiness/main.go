package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/quay/release-readiness/internal/db"
	"github.com/quay/release-readiness/internal/jira"
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
		syncer := s3client.NewSyncer(s3c, database, s3Log)
		wg.Add(1)
		go func() {
			defer wg.Done()
			syncer.Run(ctx, *s3PollInterval)
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
