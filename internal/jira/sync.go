package jira

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/quay/release-readiness/internal/model"
	"github.com/quay/release-readiness/internal/syncstatus"
	"github.com/quay/release-readiness/internal/workpool"
)

// Store is the subset of the database layer needed by the JIRA syncer.
type Store interface {
	UpsertReleaseVersion(ctx context.Context, v *model.ReleaseVersion) error
	UpsertJiraIssue(ctx context.Context, issue *model.JiraIssueRecord) error
	DeleteJiraIssuesNotIn(ctx context.Context, fixVersion string, keys []string) error
	ListAllReleaseVersions(ctx context.Context) ([]model.ReleaseVersion, error)
}

// TxFunc wraps a function in a database transaction, passing a tx-scoped Store.
type TxFunc func(ctx context.Context, fn func(Store) error) error

// Syncer orchestrates periodic JIRA synchronisation into a Store.
type Syncer struct {
	client *Client
	store  Store
	withTx TxFunc
	logger *slog.Logger
	// Status receives each pass's outcome; nil reports nowhere.
	Status  *syncstatus.Source
	Workers int
}

// releasedDiscoveryWindow bounds how far back closed release tickets are
// discovered each pass: old z-streams beyond it stay undiscovered.
const releasedDiscoveryWindow = 180 * 24 * time.Hour

// NewSyncer creates a Syncer that uses client to fetch data and store to persist it.
func NewSyncer(client *Client, store Store, withTx TxFunc, logger *slog.Logger) *Syncer {
	return &Syncer{client: client, store: store, withTx: withTx, logger: logger, Workers: 2}
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

// SyncOnce discovers active releases and syncs their issues.
func (s *Syncer) SyncOnce(ctx context.Context) {
	var pass syncstatus.Pass
	defer func() { s.Status.Report(authError(pass.Err())) }()

	releases, err := s.client.DiscoverActiveReleases(ctx)
	if err != nil {
		s.logger.Error("discover releases", "error", err)
		pass.Add(err)
		return
	}

	s.logger.Info("discovered active releases", "count", len(releases))

	versions, versionsErr := s.client.ListVersions(ctx)
	if versionsErr != nil {
		s.logger.Error("list versions", "error", versionsErr)
		pass.Add(fmt.Errorf("list versions: %w", versionsErr))
		// Stand in the stored flags so this pass does not zero them.
		stored, err := s.store.ListAllReleaseVersions(ctx)
		if err != nil {
			s.logger.Error("list db versions", "error", err)
			pass.Add(fmt.Errorf("list db versions: %w", err))
			return
		}
		versions = make(map[string]VersionField, len(stored))
		for _, v := range stored {
			vf := VersionField{Name: v.Name, Released: v.Released, Archived: v.Archived}
			if v.ReleaseDate != nil {
				vf.ReleaseDate = v.ReleaseDate.Format(time.DateOnly)
			}
			versions[v.Name] = vf
		}
	}

	activeSet := make(map[string]bool, len(releases))
	var scopes []string

	for _, rel := range releases {
		activeSet[rel.FixVersion] = true

		rv := &model.ReleaseVersion{
			Name:                  rel.FixVersion,
			ReleaseTicketKey:      rel.ReleaseTicketKey,
			ReleaseTicketAssignee: rel.Assignee,
			KonfluxApplication:    rel.KonfluxApplication,
			DueDate:               rel.DueDate,
		}

		versionInfo, ok := versions[rel.FixVersion]
		if !ok {
			// A fixVersion missing from JIRA is a data problem on one ticket, not sync health.
			if versionsErr == nil {
				s.logger.Warn("version not found in project", "version", rel.FixVersion)
			}
		} else {
			rv.Released = versionInfo.Released
			rv.Archived = versionInfo.Archived
			if t, err := time.Parse(time.DateOnly, versionInfo.ReleaseDate); err == nil {
				rv.ReleaseDate = &t
			}
		}

		if err := s.store.UpsertReleaseVersion(ctx, rv); err != nil {
			s.logger.Error("upsert version", "version", rel.FixVersion, "error", err)
			pass.Add(fmt.Errorf("upsert version %s: %w", rel.FixVersion, err))
		}

		if !slices.Contains(scopes, rel.FixVersion) {
			scopes = append(scopes, rel.FixVersion)
		}
	}

	// Backfill versions whose release ticket closed before this app first
	// ran: without a row, the reconciliation below cannot see them, and the
	// portfolio's history and available stage stay empty on a fresh
	// database. The JIRA project version list stays the authority for the
	// released flag; a closed ticket is the evidence when it lacks the
	// version.
	released, releasedErr := s.client.DiscoverReleasedReleases(ctx, releasedDiscoveryWindow)
	if releasedErr != nil {
		s.logger.Error("discover released releases", "error", releasedErr)
		pass.Add(fmt.Errorf("discover released releases: %w", releasedErr))
	} else {
		s.logger.Info("discovered released releases", "count", len(released))
		for _, rel := range released {
			if activeSet[rel.FixVersion] {
				continue
			}
			rv := &model.ReleaseVersion{
				Name:                  rel.FixVersion,
				ReleaseTicketKey:      rel.ReleaseTicketKey,
				ReleaseTicketAssignee: rel.Assignee,
				KonfluxApplication:    rel.KonfluxApplication,
			}
			// A closed release ticket is the release evidence. The project
			// version list often leaves shipped versions unreleased, so it
			// only contributes metadata here, never a veto.
			rv.Released = true
			if versionInfo, ok := versions[rel.FixVersion]; ok {
				rv.Archived = versionInfo.Archived
				if t, err := time.Parse(time.DateOnly, versionInfo.ReleaseDate); err == nil {
					rv.ReleaseDate = &t
				}
			} else {
				// The reconciliation below syncs issues only for versions
				// the project list knows; this one exists by its closed
				// ticket alone, so scope it here.
				scopes = append(scopes, rel.FixVersion)
			}
			if err := s.store.UpsertReleaseVersion(ctx, rv); err != nil {
				s.logger.Error("upsert released version", "version", rel.FixVersion, "error", err)
				pass.Add(fmt.Errorf("upsert version %s: %w", rel.FixVersion, err))
			}
		}
	}

	// Keep syncing versions whose tracking ticket closed (dropped from
	// DiscoverActiveReleases): a shipped release still lists its issues,
	// and JIRA often leaves a shipped version unreleased. Archived ones stay frozen.
	dbVersions, err := s.store.ListAllReleaseVersions(ctx)
	if err != nil {
		s.logger.Error("list db versions", "error", err)
		pass.Add(fmt.Errorf("list db versions: %w", err))
		return
	}
	for _, dbv := range dbVersions {
		if dbv.Archived || activeSet[dbv.Name] {
			continue
		}
		versionInfo, ok := versions[dbv.Name]
		if !ok {
			continue
		}
		dbv.Released = dbv.Released || versionInfo.Released
		dbv.Archived = versionInfo.Archived
		if t, err := time.Parse(time.DateOnly, versionInfo.ReleaseDate); err == nil {
			dbv.ReleaseDate = &t
		}
		if err := s.store.UpsertReleaseVersion(ctx, &dbv); err != nil {
			s.logger.Error("upsert version", "version", dbv.Name, "error", err)
			pass.Add(fmt.Errorf("upsert version %s: %w", dbv.Name, err))
		}
		scopes = append(scopes, dbv.Name)
	}
	pass.Add(workpool.Run(ctx, scopes, s.Workers, func(ctx context.Context, version string) ([]Issue, error) {
		issues, err := s.client.SearchIssues(ctx, version)
		if err != nil {
			return nil, fmt.Errorf("search issues %s: %w", version, err)
		}
		return issues, nil
	}, func(version string, issues []Issue) error { return s.storeVersion(ctx, version, issues) }))
}

// authError marks a rejected token, which every later pass will hit too.
func authError(err error) error {
	if err != nil && (strings.Contains(err.Error(), "returned 401") || strings.Contains(err.Error(), "returned 403")) {
		return fmt.Errorf("JIRA authentication failed: %w", err)
	}
	return err
}

// storeVersion atomically replaces a completely fetched issue scope.
func (s *Syncer) storeVersion(ctx context.Context, fixVersion string, issues []Issue) error {
	if err := s.withTx(ctx, func(txStore Store) error {
		var keys []string
		for _, issue := range issues {
			keys = append(keys, issue.Key)

			jiraURL := fmt.Sprintf("%s/browse/%s", s.client.SiteURL(), issue.Key)

			record := &model.JiraIssueRecord{
				Key:        issue.Key,
				Summary:    issue.Fields.Summary,
				FixVersion: fixVersion,
				IssueType:  issue.Fields.IssueType.Name,
				Status:     issue.Fields.Status.Name,
				Link:       jiraURL,
			}

			if err := txStore.UpsertJiraIssue(ctx, record); err != nil {
				return fmt.Errorf("upsert issue %s: %w", issue.Key, err)
			}
		}

		if err := txStore.DeleteJiraIssuesNotIn(ctx, fixVersion, keys); err != nil {
			return fmt.Errorf("cleanup issues: %w", err)
		}
		return nil
	}); err != nil {
		s.logger.Error("sync version", "version", fixVersion, "error", err)
		return fmt.Errorf("sync version %s: %w", fixVersion, err)
	}

	s.logger.Info("synced issues", "count", len(issues), "version", fixVersion)
	return nil
}
