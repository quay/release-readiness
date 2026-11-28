package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quay/release-readiness/internal/model"
	"github.com/quay/release-readiness/internal/prow"
)

func TestValidationRequiresFullPlanAndExactIdentity(t *testing.T) {
	plan := ValidationPlan{Revision: "reviewed", Jobs: []model.ValidationRow{{Job: "a"}, {Job: "b"}}}
	start := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	later := start.Add(time.Hour)
	candidate := &model.ReleaseCandidate{Verdict: model.Verdict{State: "ready"}, FBCCandidate: &model.FBCCandidate{Operators: []model.OperatorFBC{{Members: []model.FBCMember{{CI: &model.FBCCI{Exact: []model.CIJob{{JobName: "a", Runs: []model.CIRun{{ProwURL: "pass"}, {ProwURL: "fail"}, {ProwURL: "running"}}}}, Equivalent: []model.CIJob{{JobName: "b", Runs: []model.CIRun{{ProwURL: "other"}}}}}}}}}}}
	runs := []prow.Run{{JobName: "a", State: "success", CompletedAt: &start, ProwURL: "pass"}, {JobName: "b", State: "success", CompletedAt: &start, ProwURL: "other"}}
	v := EvaluateValidation(plan, candidate, runs, true)
	if v.Ready || v.Total != 2 || v.Passed != 1 || v.Missing != 1 {
		t.Fatalf("partial/equivalent evidence: %+v", v)
	}
	runs = append(runs, prow.Run{JobName: "a", State: "running", ProwURL: "running"})
	if v = EvaluateValidation(plan, candidate, runs, true); v.Passed != 1 {
		t.Fatal("running rerun replaced completed pass")
	}
	runs = append(runs, prow.Run{JobName: "a", State: "failure", CompletedAt: &later, ProwURL: "fail"})
	if v = EvaluateValidation(plan, candidate, runs, true); v.Failed != 1 || v.Passed != 0 {
		t.Fatal("new completed failure did not replace pass")
	}
	plan.Jobs = plan.Jobs[:1]
	runs = runs[:1]
	if !EvaluateValidation(plan, candidate, runs, true).Ready {
		t.Fatal("complete exact plan should be ready")
	}
	if EvaluateValidation(plan, candidate, runs, false).Ready {
		t.Fatal("stale sources authorized readiness")
	}
	candidate.Verdict.State = "not_ready"
	if EvaluateValidation(plan, candidate, runs, true).Ready {
		t.Fatal("FBC blocker bypassed")
	}
	if EvaluateValidation(ValidationPlan{}, candidate, runs, true).Ready {
		t.Fatal("unknown plan authorized readiness")
	}
	runs[0].InstallFailed = true
	if v = EvaluateValidation(plan, candidate, runs, true); v.Failed != 1 {
		t.Fatal("installation failure treated as green")
	}
}

func TestLoadValidationPlans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	for _, body := range []string{`{"quay-3-18":{"jobs":[{"job":"a"}]}}`, `{"quay-3-18":{"revision":"rev","jobs":[]}}`, `{"quay-3-18":{"revision":"rev","jobs":[{"job":"a"},{"job":"a"}]}}`} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadValidationPlans(path); err == nil {
			t.Fatalf("accepted invalid plan: %s", body)
		}
	}
}

func TestValidationExplainsMissingEvidenceWithoutPassing(t *testing.T) {
	plan := ValidationPlan{Revision: "periodics:rev", Jobs: []model.ValidationRow{{Job: "required"}}}
	candidate := &model.ReleaseCandidate{Verdict: model.Verdict{State: "ready"}, FBCCandidate: &model.FBCCandidate{Operators: []model.OperatorFBC{{Members: []model.FBCMember{{Digest: "sha256:selected"}}}}}}
	for _, tc := range []struct {
		name    string
		run     *prow.Run
		message string
	}{
		{"no runs", nil, "No run has been ingested"},
		{"missing artifact", &prow.Run{ArtifactState: prow.ArtifactNone}, "image evidence is unavailable"},
		{"legacy", &prow.Run{ArtifactState: prow.ArtifactPresent}, "without an exact FBC identity"},
		{"other FBC", &prow.Run{ArtifactState: prow.ArtifactPresent, CatalogRef: "quay.io/catalog@sha256:other"}, "different FBC"},
		{"unresolved match", &prow.Run{ArtifactState: prow.ArtifactPresent, CatalogRef: "quay.io/catalog@sha256:selected"}, "correlation is not yet complete"},
		{"running", &prow.Run{State: "running"}, "in progress"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var runs []prow.Run
			if tc.run != nil {
				tc.run.JobName = "required"
				tc.run.ProwURL = "https://prow.example/observed"
				runs = append(runs, *tc.run)
			}
			v := EvaluateValidation(plan, candidate, runs, true)
			if v.Ready || v.Passed != 0 || v.Missing != 1 || !strings.Contains(v.Rows[0].Evidence, tc.message) {
				t.Fatalf("wrong evidence: %+v", v)
			}
			if tc.run != nil && v.Rows[0].RunURL != tc.run.ProwURL {
				t.Fatal("observed run cannot be inspected")
			}
		})
	}
}

func TestValidationRunningRequiresExactFBCAndUsesNewestRun(t *testing.T) {
	plan := ValidationPlan{Revision: "periodics:rev", Jobs: []model.ValidationRow{{Job: "a"}}}
	candidate := &model.ReleaseCandidate{Verdict: model.Verdict{State: "ready"}, FBCCandidate: &model.FBCCandidate{Operators: []model.OperatorFBC{{Members: []model.FBCMember{{CI: &model.FBCCI{Exact: []model.CIJob{{JobName: "a", Runs: []model.CIRun{{ProwURL: "done"}, {ProwURL: "new"}, {ProwURL: "old"}}}}}}}}}}}
	at := time.Now().UTC()
	older, newer := at.Add(-time.Hour), at.Add(time.Hour)
	runs := []prow.Run{{JobName: "a", State: "running", StartedAt: &newer, ProwURL: "new"}, {JobName: "a", State: "pending", StartedAt: &older, ProwURL: "old"}}
	v := EvaluateValidation(plan, candidate, runs, true)
	if v.Running != 1 || v.Missing != 0 || v.Ready || v.Rows[0].RunURL != "new" {
		t.Fatalf("active exact run: %+v", v)
	}
	runs = append(runs, prow.Run{JobName: "a", State: "success", CompletedAt: &at, ProwURL: "done"})
	if v = EvaluateValidation(plan, candidate, runs, true); v.Running != 1 || v.Ready {
		t.Fatalf("new rerun hidden by old success: %+v", v)
	}
	runs = runs[1:]
	if v = EvaluateValidation(plan, candidate, runs, true); v.Passed != 1 {
		t.Fatalf("old active run hid later success: %+v", v)
	}
	runs = []prow.Run{{JobName: "a", State: "running", StartedAt: &newer, ProwURL: "unverified"}}
	if v = EvaluateValidation(plan, candidate, runs, true); v.Missing != 1 || v.Running != 0 {
		t.Fatalf("unverified run counted: %+v", v)
	}
}
