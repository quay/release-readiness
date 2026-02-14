package model

import "time"

type Component struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type ComponentRecord struct {
	ID         int64  `json:"id"`
	SnapshotID int64  `json:"snapshot_id"`
	Component  string `json:"component"`
	GitSHA     string `json:"git_sha"`
	ImageURL   string `json:"image_url"`
	GitURL     string `json:"git_url"`
}

type SnapshotRecord struct {
	ID                   int64                `json:"id"`
	Application          string               `json:"application"`
	Name                 string               `json:"name"`
	TriggerComponent     string               `json:"trigger_component"`
	TriggerGitSHA        string               `json:"trigger_git_sha"`
	TriggerPipelineRun   string               `json:"trigger_pipeline_run"`
	TestsPassed          bool                 `json:"tests_passed"`
	Released             bool                 `json:"released"`
	ReleaseBlockedReason string               `json:"release_blocked_reason,omitempty"`
	CreatedAt            time.Time            `json:"created_at"`
	Components           []ComponentRecord    `json:"components,omitempty"`
	TestResults          []SnapshotTestResult `json:"test_results,omitempty"`
}

type SnapshotTestResult struct {
	ID          int64     `json:"id"`
	SnapshotID  int64     `json:"snapshot_id"`
	Scenario    string    `json:"scenario"`
	Status      string    `json:"status"`
	PipelineRun string    `json:"pipeline_run"`
	Total       int       `json:"total"`
	Passed      int       `json:"passed"`
	Failed      int       `json:"failed"`
	Skipped     int       `json:"skipped"`
	DurationSec float64   `json:"duration_sec"`
	CreatedAt   time.Time `json:"created_at"`
}

type ApplicationSummary struct {
	Application    string          `json:"application"`
	LatestSnapshot *SnapshotRecord `json:"latest_snapshot,omitempty"`
	SnapshotCount  int             `json:"snapshot_count"`
}

// JiraIssueRecord represents a JIRA issue cached in the database.
type JiraIssueRecord struct {
	ID         int64     `json:"id"`
	Key        string    `json:"key"`
	Summary    string    `json:"summary"`
	Status     string    `json:"status"`
	Priority   string    `json:"priority"`
	Labels     string    `json:"labels"` // comma-separated
	FixVersion string    `json:"fix_version"`
	Assignee   string    `json:"assignee"`
	IssueType  string    `json:"issue_type"`
	Resolution string    `json:"resolution"`
	Link       string    `json:"link"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// IssueSummary provides aggregate counts of JIRA issues for a release.
type IssueSummary struct {
	Total    int `json:"total"`
	Verified int `json:"verified"`
	Open     int `json:"open"`
	CVEs     int `json:"cves"`
	Bugs     int `json:"bugs"`
}

// ReleaseVersion represents a JIRA fixVersion with release metadata.
type ReleaseVersion struct {
	Name                   string     `json:"name"`
	Description            string     `json:"description"`
	ReleaseDate            *time.Time `json:"release_date,omitempty"`
	Released               bool       `json:"released"`
	Archived               bool       `json:"archived"`
	ReleaseTicketKey       string     `json:"release_ticket_key,omitempty"`
	ReleaseTicketAssignee  string     `json:"release_ticket_assignee,omitempty"`
	S3Application          string     `json:"s3_application,omitempty"`
	DueDate                *time.Time `json:"due_date,omitempty"`
}
