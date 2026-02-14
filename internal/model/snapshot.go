package model

import "time"

// Snapshot represents the full state of a Konflux Snapshot stored in S3.
// Each file contains everything needed to render the dashboard for that
// point in time: components, test results, and release status.
type Snapshot struct {
	Application string              `json:"application"`
	Snapshot    string              `json:"snapshot"`
	CreatedAt   time.Time           `json:"created_at"`
	Trigger     Trigger             `json:"trigger"`
	Components  []SnapshotComponent `json:"components"`
	TestResults []TestResult        `json:"test_results"`
	Releases    []Release           `json:"releases"`
	Readiness   Readiness           `json:"readiness"`
}

// Trigger identifies the component build that created this snapshot.
type Trigger struct {
	Component   string `json:"component"`
	GitSHA      string `json:"git_sha"`
	PipelineRun string `json:"pipeline_run"`
}

// SnapshotComponent is a single component image captured in the snapshot.
type SnapshotComponent struct {
	Name           string `json:"name"`
	ContainerImage string `json:"container_image"`
	GitRevision    string `json:"git_revision"`
	GitURL         string `json:"git_url"`
}

// TestResult holds the high-level outcome of one IntegrationTestScenario run.
type TestResult struct {
	Scenario       string       `json:"scenario"`
	Status         string       `json:"status"` // passed, failed, invalid
	PipelineRun    string       `json:"pipeline_run,omitempty"`
	StartTime      *time.Time   `json:"start_time,omitempty"`
	CompletionTime *time.Time   `json:"completion_time,omitempty"`
	Details        string       `json:"details,omitempty"`
	JUnitPath      *string      `json:"junit_path"`
	Summary        *TestSummary `json:"summary"`
}

// TestSummary contains pre-computed aggregate counts from JUnit XML.
type TestSummary struct {
	Total       int     `json:"total"`
	Passed      int     `json:"passed"`
	Failed      int     `json:"failed"`
	Skipped     int     `json:"skipped"`
	DurationSec float64 `json:"duration_sec"`
}

// Release represents a Release CR associated with a snapshot.
type Release struct {
	Name           string      `json:"name"`
	ReleasePlan    string      `json:"release_plan"`
	Target         string      `json:"target"`
	Status         string      `json:"status"` // succeeded, failed, progressing
	Reason         string      `json:"reason,omitempty"`
	StartTime      *time.Time  `json:"start_time,omitempty"`
	CompletionTime *time.Time  `json:"completion_time,omitempty"`
	JiraIssues     []JiraIssue `json:"jira_issues,omitempty"`
}

// JiraIssue is a Jira issue referenced in a release's release notes.
type JiraIssue struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

// Readiness summarizes whether this snapshot is ready for release.
type Readiness struct {
	TestsPassed          bool   `json:"tests_passed"`
	Released             bool   `json:"released"`
	ReleaseBlockedReason string `json:"release_blocked_reason,omitempty"`
}
