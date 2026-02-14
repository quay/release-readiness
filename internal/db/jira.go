package db

import (
	"strings"
	"time"

	"github.com/quay/release-readiness/internal/model"
)

// UpsertJiraIssue inserts or updates a JIRA issue by key.
func (d *DB) UpsertJiraIssue(issue *model.JiraIssueRecord) error {
	_, err := d.Exec(`
		INSERT INTO jira_issues (key, summary, status, priority, labels, fix_version, assignee, issue_type, resolution, link, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key, fix_version) DO UPDATE SET
			summary=excluded.summary,
			status=excluded.status,
			priority=excluded.priority,
			labels=excluded.labels,
			assignee=excluded.assignee,
			issue_type=excluded.issue_type,
			resolution=excluded.resolution,
			link=excluded.link,
			updated_at=excluded.updated_at`,
		issue.Key, issue.Summary, issue.Status, issue.Priority, issue.Labels,
		issue.FixVersion, issue.Assignee, issue.IssueType, issue.Resolution,
		issue.Link, issue.UpdatedAt.UTC().Format(time.RFC3339))
	return err
}

// ListJiraIssues returns issues for a fixVersion with optional filters.
func (d *DB) ListJiraIssues(fixVersion string, issueType, status, label string) ([]model.JiraIssueRecord, error) {
	query := `SELECT id, key, summary, status, priority, labels, fix_version, assignee, issue_type, resolution, link, updated_at
		FROM jira_issues WHERE fix_version = ?`
	args := []interface{}{fixVersion}

	if issueType != "" {
		query += ` AND issue_type = ?`
		args = append(args, issueType)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	if label != "" {
		query += ` AND labels LIKE ?`
		args = append(args, "%"+label+"%")
	}
	query += ` ORDER BY key`

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []model.JiraIssueRecord
	for rows.Next() {
		var i model.JiraIssueRecord
		var ts string
		if err := rows.Scan(&i.ID, &i.Key, &i.Summary, &i.Status, &i.Priority,
			&i.Labels, &i.FixVersion, &i.Assignee, &i.IssueType, &i.Resolution,
			&i.Link, &ts); err != nil {
			return nil, err
		}
		i.UpdatedAt, _ = time.Parse(time.RFC3339, ts)
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

// GetIssueSummary returns aggregate counts for a fixVersion.
func (d *DB) GetIssueSummary(fixVersion string) (*model.IssueSummary, error) {
	var s model.IssueSummary
	err := d.QueryRow(`
		SELECT
			COUNT(*) AS total,
			SUM(CASE WHEN LOWER(status) IN ('closed', 'verified', 'done') THEN 1 ELSE 0 END) AS verified,
			SUM(CASE WHEN LOWER(status) NOT IN ('closed', 'verified', 'done') THEN 1 ELSE 0 END) AS open,
			SUM(CASE WHEN LOWER(issue_type) = 'cve' OR LOWER(labels) LIKE '%cve%' THEN 1 ELSE 0 END) AS cves,
			SUM(CASE WHEN LOWER(issue_type) = 'bug' THEN 1 ELSE 0 END) AS bugs
		FROM jira_issues
		WHERE fix_version = ?`, fixVersion).
		Scan(&s.Total, &s.Verified, &s.Open, &s.CVEs, &s.Bugs)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpsertReleaseVersion inserts or updates a release version by name.
func (d *DB) UpsertReleaseVersion(v *model.ReleaseVersion) error {
	relDate := ""
	if v.ReleaseDate != nil {
		relDate = v.ReleaseDate.UTC().Format(time.RFC3339)
	}
	dueDate := ""
	if v.DueDate != nil {
		dueDate = v.DueDate.UTC().Format(time.RFC3339)
	}
	rel, arch := 0, 0
	if v.Released {
		rel = 1
	}
	if v.Archived {
		arch = 1
	}
	_, err := d.Exec(`
		INSERT INTO release_versions (name, description, release_date, released, archived, release_ticket_key, release_ticket_assignee, s3_application, due_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			description=excluded.description,
			release_date=excluded.release_date,
			released=excluded.released,
			archived=excluded.archived,
			release_ticket_key=excluded.release_ticket_key,
			release_ticket_assignee=excluded.release_ticket_assignee,
			s3_application=excluded.s3_application,
			due_date=excluded.due_date`,
		v.Name, v.Description, relDate, rel, arch, v.ReleaseTicketKey, v.ReleaseTicketAssignee, v.S3Application, dueDate)
	return err
}

// GetReleaseVersion returns a release version by name.
func (d *DB) GetReleaseVersion(name string) (*model.ReleaseVersion, error) {
	var v model.ReleaseVersion
	var relDate, dueDate string
	var rel, arch int
	err := d.QueryRow(`
		SELECT name, description, release_date, released, archived, release_ticket_key, release_ticket_assignee, s3_application, due_date
		FROM release_versions WHERE name = ?`, name).
		Scan(&v.Name, &v.Description, &relDate, &rel, &arch, &v.ReleaseTicketKey, &v.ReleaseTicketAssignee, &v.S3Application, &dueDate)
	if err != nil {
		return nil, err
	}
	v.Released = rel == 1
	v.Archived = arch == 1
	if relDate != "" {
		t, err := time.Parse(time.RFC3339, relDate)
		if err == nil {
			v.ReleaseDate = &t
		}
	}
	if dueDate != "" {
		t, err := time.Parse(time.RFC3339, dueDate)
		if err == nil {
			v.DueDate = &t
		}
	}
	return &v, nil
}

// ListActiveReleaseVersions returns all release versions that are not released or archived.
func (d *DB) ListActiveReleaseVersions() ([]model.ReleaseVersion, error) {
	rows, err := d.Query(`
		SELECT name, description, release_date, released, archived, release_ticket_key, release_ticket_assignee, s3_application, due_date
		FROM release_versions
		WHERE released = 0 AND archived = 0
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []model.ReleaseVersion
	for rows.Next() {
		var v model.ReleaseVersion
		var relDate, dueDate string
		var rel, arch int
		if err := rows.Scan(&v.Name, &v.Description, &relDate, &rel, &arch, &v.ReleaseTicketKey, &v.ReleaseTicketAssignee, &v.S3Application, &dueDate); err != nil {
			return nil, err
		}
		v.Released = rel == 1
		v.Archived = arch == 1
		if relDate != "" {
			t, err := time.Parse(time.RFC3339, relDate)
			if err == nil {
				v.ReleaseDate = &t
			}
		}
		if dueDate != "" {
			t, err := time.Parse(time.RFC3339, dueDate)
			if err == nil {
				v.DueDate = &t
			}
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// ListAllReleaseVersions returns all release versions.
func (d *DB) ListAllReleaseVersions() ([]model.ReleaseVersion, error) {
	rows, err := d.Query(`
		SELECT name, description, release_date, released, archived, release_ticket_key, release_ticket_assignee, s3_application, due_date
		FROM release_versions
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []model.ReleaseVersion
	for rows.Next() {
		var v model.ReleaseVersion
		var relDate, dueDate string
		var rel, arch int
		if err := rows.Scan(&v.Name, &v.Description, &relDate, &rel, &arch, &v.ReleaseTicketKey, &v.ReleaseTicketAssignee, &v.S3Application, &dueDate); err != nil {
			return nil, err
		}
		v.Released = rel == 1
		v.Archived = arch == 1
		if relDate != "" {
			t, err := time.Parse(time.RFC3339, relDate)
			if err == nil {
				v.ReleaseDate = &t
			}
		}
		if dueDate != "" {
			t, err := time.Parse(time.RFC3339, dueDate)
			if err == nil {
				v.DueDate = &t
			}
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// DeleteJiraIssuesNotIn removes issues for a fixVersion that are not in the given keys slice.
// This handles issues that have been moved to a different fixVersion.
func (d *DB) DeleteJiraIssuesNotIn(fixVersion string, keys []string) error {
	if len(keys) == 0 {
		_, err := d.Exec(`DELETE FROM jira_issues WHERE fix_version = ?`, fixVersion)
		return err
	}
	placeholders := make([]string, len(keys))
	args := make([]interface{}, 0, len(keys)+1)
	args = append(args, fixVersion)
	for i, k := range keys {
		placeholders[i] = "?"
		args = append(args, k)
	}
	query := `DELETE FROM jira_issues WHERE fix_version = ? AND key NOT IN (` + strings.Join(placeholders, ",") + `)`
	_, err := d.Exec(query, args...)
	return err
}
