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
	ID          int64             `json:"id"`
	Application string            `json:"application"`
	Name        string            `json:"name"`
	TestsPassed bool              `json:"tests_passed"`
	HasTests    bool              `json:"has_tests"`
	CreatedAt   time.Time         `json:"created_at"`
	Components  []ComponentRecord `json:"components,omitempty"`
	TestSuites  []TestSuite       `json:"test_suites,omitempty"`
}

type TestSuite struct {
	ID          int64      `json:"id"`
	SnapshotID  int64      `json:"snapshot_id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	PipelineRun string     `json:"pipeline_run"`
	ToolName    string     `json:"tool_name"`
	ToolVersion string     `json:"tool_version"`
	Tests       int        `json:"tests"`
	Passed      int        `json:"passed"`
	Failed      int        `json:"failed"`
	Skipped     int        `json:"skipped"`
	Pending     int        `json:"pending"`
	Other       int        `json:"other"`
	Flaky       int        `json:"flaky"`
	StartTime   int64      `json:"start_time"`
	StopTime    int64      `json:"stop_time"`
	DurationMs  int64      `json:"duration_ms"`
	CreatedAt   time.Time  `json:"created_at"`
	TestCases   []TestCase `json:"test_cases,omitempty"`
}

type TestCase struct {
	ID          int64   `json:"id"`
	TestSuiteID int64   `json:"test_suite_id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	DurationMs  float64 `json:"duration_ms"`
	Message     string  `json:"message,omitempty"`
	Trace       string  `json:"trace,omitempty"`
	FilePath    string  `json:"file_path,omitempty"`
	Suite       string  `json:"suite,omitempty"`
	Retries     int     `json:"retries"`
	Flaky       bool    `json:"flaky"`
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

// ReleaseOverview is a combined view of a release with its issue summary,
// readiness signal, and latest snapshot metadata.
type ReleaseOverview struct {
	Release      ReleaseVersion    `json:"release"`
	IssueSummary *IssueSummary     `json:"issue_summary,omitempty"`
	Readiness    ReadinessResponse `json:"readiness"`
	Snapshot     *SnapshotRecord   `json:"snapshot,omitempty"`
}

// ReadinessResponse represents the computed readiness signal for a release.
type ReadinessResponse struct {
	Signal  string `json:"signal"`  // "green", "yellow", "red"
	Message string `json:"message"` // human-readable reason
}

// ReleaseVersion represents a JIRA fixVersion with release metadata.
type ReleaseVersion struct {
	Name                  string     `json:"name"`
	Description           string     `json:"description"`
	ReleaseDate           *time.Time `json:"release_date,omitempty"`
	Released              bool       `json:"released"`
	Archived              bool       `json:"archived"`
	ReleaseTicketKey      string     `json:"release_ticket_key,omitempty"`
	ReleaseTicketAssignee string     `json:"release_ticket_assignee,omitempty"`
	S3Application         string     `json:"s3_application,omitempty"`
	DueDate               *time.Time `json:"due_date,omitempty"`
}
