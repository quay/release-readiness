package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/quay/build-dashboard/internal/junit"
	"github.com/quay/build-dashboard/internal/model"
)

type ResultsReport struct {
	Server      string
	BuildID     int64
	Suite       string
	Environment string
	PipelineRun string
	Files       []string
}

func ReportResults(r ResultsReport) error {
	var results []*junit.Result
	for _, f := range r.Files {
		res, err := junit.ParseFile(f)
		if err != nil {
			return fmt.Errorf("parse %s: %w", f, err)
		}
		results = append(results, res)
	}

	merged := junit.MergeResults(results...)

	status := "passed"
	if merged.Failed > 0 {
		status = "failed"
	}

	body := struct {
		Suite       string           `json:"suite"`
		Environment string           `json:"environment"`
		PipelineRun string           `json:"pipeline_run"`
		Total       int              `json:"total"`
		Passed      int              `json:"passed"`
		Failed      int              `json:"failed"`
		Skipped     int              `json:"skipped"`
		DurationSec float64          `json:"duration_sec"`
		TestCases   []model.TestCase `json:"test_cases"`
	}{
		Suite:       r.Suite,
		Environment: r.Environment,
		PipelineRun: r.PipelineRun,
		Total:       merged.Total,
		Passed:      merged.Passed,
		Failed:      merged.Failed,
		Skipped:     merged.Skipped,
		DurationSec: merged.DurationSec,
		TestCases:   merged.TestCases,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/builds/%d/results", r.Server, r.BuildID)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("POST results: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("server returned %d: %v", resp.StatusCode, result["error"])
	}

	fmt.Printf("Test run created: id=%v (%s: %d passed, %d failed, %d skipped)\n",
		result["id"], status, merged.Passed, merged.Failed, merged.Skipped)
	return nil
}
