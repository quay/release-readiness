package model

import "time"

type Component struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	Suites      []Suite   `json:"suites,omitempty"`
}

type Suite struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	Required    bool      `json:"required,omitempty"`
}

type Build struct {
	ID           int64     `json:"id"`
	ComponentID  int64     `json:"component_id"`
	Component    string    `json:"component,omitempty"`
	Version      string    `json:"version"`
	GitSHA       string    `json:"git_sha"`
	GitBranch    string    `json:"git_branch"`
	ImageURL     string    `json:"image_url"`
	ImageDigest  string    `json:"image_digest"`
	PipelineRun  string    `json:"pipeline_run"`
	SnapshotName string    `json:"snapshot_name"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	TestRuns     []TestRun `json:"test_runs,omitempty"`
}

type TestRun struct {
	ID          int64      `json:"id"`
	BuildID     int64      `json:"build_id"`
	SuiteID     int64      `json:"suite_id"`
	Suite       string     `json:"suite,omitempty"`
	Total       int        `json:"total"`
	Passed      int        `json:"passed"`
	Failed      int        `json:"failed"`
	Skipped     int        `json:"skipped"`
	DurationSec float64    `json:"duration_sec"`
	Status      string     `json:"status"`
	Environment string     `json:"environment"`
	PipelineRun string     `json:"pipeline_run"`
	CreatedAt   time.Time  `json:"created_at"`
	TestCases   []TestCase `json:"test_cases,omitempty"`
}

type TestCase struct {
	ID          int64   `json:"id"`
	TestRunID   int64   `json:"test_run_id"`
	Name        string  `json:"name"`
	ClassName   string  `json:"classname"`
	DurationSec float64 `json:"duration_sec"`
	Status      string  `json:"status"`
	FailureMsg  string  `json:"failure_msg,omitempty"`
	FailureText string  `json:"failure_text,omitempty"`
}

type ReadinessCell struct {
	SuiteName string `json:"suite_name"`
	SuiteID   int64  `json:"suite_id"`
	Required  bool   `json:"required"`
	Status    string `json:"status"` // passed|failed|pending|not_configured
	TestRunID int64  `json:"test_run_id,omitempty"`
}

type ReadinessRow struct {
	Component Component       `json:"component"`
	Build     *Build          `json:"build,omitempty"`
	Cells     []ReadinessCell `json:"cells"`
	Ready     bool            `json:"ready"`
}

type ReadinessMatrix struct {
	Version    string         `json:"version"`
	Suites     []Suite        `json:"suites"`
	Rows       []ReadinessRow `json:"rows"`
	Ready      bool           `json:"ready"`
	BlockCount int            `json:"block_count"`
}

type VersionSummary struct {
	Version    string `json:"version"`
	BuildCount int    `json:"build_count"`
	Ready      bool   `json:"ready"`
	BlockCount int    `json:"block_count"`
}
