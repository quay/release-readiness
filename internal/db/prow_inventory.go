package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/quay/release-readiness/internal/artbuild"
	"github.com/quay/release-readiness/internal/db/sqlc"
	"github.com/quay/release-readiness/internal/prow"
)

// ProwApplicationActivity marks streams with any unreleased, non-archived
// JIRA version as active. Unknown streams are deliberately absent: callers
// must prioritize them until release discovery establishes their lifecycle.
func (d *DB) ProwApplicationActivity(ctx context.Context) (map[string]bool, error) {
	versions, err := d.ListAllReleaseVersions(ctx)
	if err != nil {
		return nil, err
	}
	active := map[string]bool{}
	for _, v := range versions {
		if v.KonfluxApplication != "" {
			active[v.KonfluxApplication] = active[v.KonfluxApplication] || (!v.Released && !v.Archived)
		}
	}
	return active, nil
}

// CandidateWindowStart uses ART build time, never the local observation time.
// Versions without a dated FBC do not schedule CI lookups.
func (d *DB) CandidateWindowStart(ctx context.Context, app string) (time.Time, error) {
	parts := strings.Split(app, "-")
	if len(parts) != 3 {
		return time.Time{}, nil
	}
	versions, err := d.ListAllReleaseVersions(ctx)
	if err != nil {
		return time.Time{}, err
	}
	var names []string
	for _, v := range versions {
		if v.KonfluxApplication == app && !v.Released && !v.Archived {
			names = append(names, v.Name)
		}
	}
	return d.CandidateWindowStartForVersions(ctx, app, names)
}

func (d *DB) CandidateWindowStartForVersions(ctx context.Context, app string, names []string) (time.Time, error) {
	parts := strings.Split(app, "-")
	if len(parts) != 3 {
		return time.Time{}, nil
	}
	active := map[string]bool{}
	for _, name := range names {
		active[strings.TrimPrefix(name, "quay-v")] = false
	}
	if len(active) == 0 {
		return time.Time{}, nil
	}
	rows, err := d.queries().ListCIWindowFBCs(ctx, "quay-"+parts[1]+"."+parts[2])
	if err != nil {
		return time.Time{}, err
	}
	var earliest time.Time
	for _, row := range rows {
		_, version, _, ok := artbuild.ParseFBC(row.Nvr)
		if !ok {
			continue
		}
		if _, ok = active[version]; !ok {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, row.StartTime)
		if err != nil {
			// ART also emits UTC without an explicit zone. This is build
			// start time, not publication time; CI uses a bounded lookback.
			at, err = time.Parse("2006-01-02T15:04:05", row.StartTime)
		}
		if err != nil {
			continue
		}
		active[version] = true
		if earliest.IsZero() || at.Before(earliest) {
			earliest = at
		}
	}
	return earliest, nil
}

// SaveJobInventory publishes a complete revision atomically. Missing jobs are
// retained as removed for audit, but are excluded from coverage by the plan
// registry that consumes the inventory.
func (d *DB) SaveJobInventory(ctx context.Context, jobs []prow.JobDefinition, revision string) error {
	if len(jobs) == 0 || revision == "" {
		return fmt.Errorf("refusing empty job inventory")
	}
	seen := map[string]bool{}
	for _, j := range jobs {
		if j.Name == "" || j.Application == "" || j.Target == "" || j.Revision != revision || seen[j.Name] {
			return fmt.Errorf("invalid job inventory entry %q", j.Name)
		}
		seen[j.Name] = true
	}
	return d.InTx(ctx, func(tx *DB) error {
		q := tx.queries()
		if err := q.MarkProwJobsRemoved(ctx); err != nil {
			return err
		}
		for _, j := range jobs {
			if err := q.UpsertProwJobConfig(ctx, dbsqlc.UpsertProwJobConfigParams{JobName: j.Name, Application: j.Application, Target: j.Target, Revision: j.Revision, SourceUrl: j.SourceURL, Ocp: j.OCP, Cloud: j.Cloud}); err != nil {
				return err
			}
		}
		return q.UpsertProwInventoryRevision(ctx, revision)
	})
}

func (d *DB) JobInventory(ctx context.Context) ([]prow.JobDefinition, string, error) {
	rows, err := d.queries().ListProwJobConfigs(ctx)
	if err != nil {
		return nil, "", err
	}
	var jobs []prow.JobDefinition
	revision := ""
	for _, r := range rows {
		revision = r.InventoryRevision
		jobs = append(jobs, prow.JobDefinition{Name: r.JobName, Application: r.Application, Target: r.Target, Revision: r.Revision, SourceURL: r.SourceUrl, OCP: r.Ocp, Cloud: r.Cloud, Removed: r.Removed != 0})
	}
	return jobs, revision, nil
}
