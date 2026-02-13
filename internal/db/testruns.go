package db

import (
	"fmt"
	"time"

	"github.com/quay/build-dashboard/internal/model"
)

func (d *DB) CreateTestRun(buildID int64, suiteName string, total, passed, failed, skipped int,
	durationSec float64, status, environment, pipelineRun string, testCases []model.TestCase) (*model.TestRun, error) {

	suite, err := d.GetSuiteByName(suiteName)
	if err != nil {
		return nil, fmt.Errorf("suite %q not found: %w", suiteName, err)
	}

	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT INTO test_runs (build_id, suite_id, total, passed, failed, skipped, duration_sec, status, environment, pipeline_run)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		buildID, suite.ID, total, passed, failed, skipped, durationSec, status, environment, pipelineRun)
	if err != nil {
		return nil, err
	}

	runID, _ := res.LastInsertId()

	for _, tc := range testCases {
		_, err := tx.Exec(`
			INSERT INTO test_cases (test_run_id, name, classname, duration_sec, status, failure_msg, failure_text)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			runID, tc.Name, tc.ClassName, tc.DurationSec, tc.Status, tc.FailureMsg, tc.FailureText)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &model.TestRun{
		ID:          runID,
		BuildID:     buildID,
		SuiteID:     suite.ID,
		Suite:       suiteName,
		Total:       total,
		Passed:      passed,
		Failed:      failed,
		Skipped:     skipped,
		DurationSec: durationSec,
		Status:      status,
		Environment: environment,
		PipelineRun: pipelineRun,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

func (d *DB) ListTestRuns(buildID int64) ([]model.TestRun, error) {
	rows, err := d.Query(`
		SELECT tr.id, tr.build_id, tr.suite_id, ts.name, tr.total, tr.passed, tr.failed,
		       tr.skipped, tr.duration_sec, tr.status, tr.environment, tr.pipeline_run, tr.created_at
		FROM test_runs tr
		JOIN test_suites ts ON ts.id = tr.suite_id
		WHERE tr.build_id = ?
		ORDER BY ts.name`, buildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []model.TestRun
	for rows.Next() {
		var r model.TestRun
		var ts string
		if err := rows.Scan(&r.ID, &r.BuildID, &r.SuiteID, &r.Suite, &r.Total, &r.Passed,
			&r.Failed, &r.Skipped, &r.DurationSec, &r.Status, &r.Environment,
			&r.PipelineRun, &ts); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (d *DB) GetTestRun(id int64) (*model.TestRun, error) {
	var r model.TestRun
	var ts string
	err := d.QueryRow(`
		SELECT tr.id, tr.build_id, tr.suite_id, ts.name, tr.total, tr.passed, tr.failed,
		       tr.skipped, tr.duration_sec, tr.status, tr.environment, tr.pipeline_run, tr.created_at
		FROM test_runs tr
		JOIN test_suites ts ON ts.id = tr.suite_id
		WHERE tr.id = ?`, id).
		Scan(&r.ID, &r.BuildID, &r.SuiteID, &r.Suite, &r.Total, &r.Passed,
			&r.Failed, &r.Skipped, &r.DurationSec, &r.Status, &r.Environment,
			&r.PipelineRun, &ts)
	if err != nil {
		return nil, err
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, ts)

	cases, err := d.listTestCases(id)
	if err != nil {
		return nil, err
	}
	r.TestCases = cases
	return &r, nil
}

func (d *DB) listTestCases(testRunID int64) ([]model.TestCase, error) {
	rows, err := d.Query(`
		SELECT id, test_run_id, name, classname, duration_sec, status, failure_msg, failure_text
		FROM test_cases
		WHERE test_run_id = ?
		ORDER BY CASE WHEN status = 'failed' THEN 0 WHEN status = 'error' THEN 1 ELSE 2 END, name`,
		testRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cases []model.TestCase
	for rows.Next() {
		var tc model.TestCase
		if err := rows.Scan(&tc.ID, &tc.TestRunID, &tc.Name, &tc.ClassName, &tc.DurationSec,
			&tc.Status, &tc.FailureMsg, &tc.FailureText); err != nil {
			return nil, err
		}
		cases = append(cases, tc)
	}
	return cases, rows.Err()
}

func (d *DB) GetReadiness(version string) (*model.ReadinessMatrix, error) {
	components, err := d.ListComponents()
	if err != nil {
		return nil, err
	}

	suites, err := d.ListSuites()
	if err != nil {
		return nil, err
	}

	matrix := &model.ReadinessMatrix{
		Version: version,
		Suites:  suites,
		Ready:   true,
	}

	for _, comp := range components {
		row := model.ReadinessRow{
			Component: comp,
			Ready:     true,
		}

		build, err := d.GetLatestBuild(comp.Name, version)
		if err != nil {
			// No build for this component/version
			row.Ready = false
			matrix.Ready = false
			matrix.BlockCount++
			for _, s := range suites {
				row.Cells = append(row.Cells, model.ReadinessCell{
					SuiteName: s.Name,
					SuiteID:   s.ID,
					Status:    "pending",
				})
			}
			matrix.Rows = append(matrix.Rows, row)
			continue
		}
		row.Build = build

		runs, err := d.ListTestRuns(build.ID)
		if err != nil {
			return nil, err
		}

		// Build a map of suite -> test run
		runMap := make(map[int64]*model.TestRun)
		for i := range runs {
			runMap[runs[i].SuiteID] = &runs[i]
		}

		// Build required suites set
		requiredSuites := make(map[int64]bool)
		for _, s := range comp.Suites {
			if s.Required {
				requiredSuites[s.ID] = true
			}
		}

		for _, s := range suites {
			cell := model.ReadinessCell{
				SuiteName: s.Name,
				SuiteID:   s.ID,
			}

			// Check if this suite is mapped to this component
			isMapped := false
			for _, cs := range comp.Suites {
				if cs.ID == s.ID {
					isMapped = true
					cell.Required = cs.Required
					break
				}
			}

			if !isMapped {
				cell.Status = "not_configured"
			} else if run, ok := runMap[s.ID]; ok {
				cell.Status = run.Status
				cell.TestRunID = run.ID
				if run.Status == "failed" && cell.Required {
					row.Ready = false
				}
			} else {
				cell.Status = "pending"
				if cell.Required {
					row.Ready = false
				}
			}

			row.Cells = append(row.Cells, cell)
		}

		if !row.Ready {
			matrix.Ready = false
			matrix.BlockCount++
		}
		matrix.Rows = append(matrix.Rows, row)
	}

	return matrix, nil
}
