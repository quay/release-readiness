package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/quay/release-readiness/internal/model"
	"github.com/quay/release-readiness/internal/prow"
	"github.com/quay/release-readiness/internal/release"
)

func TestDynamicInventoryFreshnessAndZeroRunCoverage(t *testing.T) {
	s := setupTestServer(t)
	s.InventoryPlans = release.NewPlanRegistry()
	s.InventoryPlans.Update([]prow.JobDefinition{{Name: "new-job", Application: "quay-3-18"}}, "rev")
	for _, name := range []string{"prow", "konflux", "art-history", "fbc-render"} {
		s.syncStatus.Track(name, time.Minute).Report(nil)
	}
	c := &model.ReleaseCandidate{Verdict: model.Verdict{State: "ready"}}
	v := s.validation("quay-3-18", c, nil)
	if v.SourceFresh || v.Total != 1 || v.Missing != 1 || v.Ready {
		t.Fatalf("unrefreshed inventory: %+v", v)
	}
	s.syncStatus.Track("prow-inventory", time.Minute).Report(nil)
	v = s.validation("quay-3-18", c, nil)
	if !v.SourceFresh || v.Total != 1 || v.Missing != 1 || v.Ready {
		t.Fatalf("new job without runs: %+v", v)
	}
}

func TestMissingValidationRelease(t *testing.T) {
	var response map[string]any
	getJSON(t, setupTestServer(t), "/api/v1/releases/quay-v99.0.0/validation", http.StatusNotFound, &response)
}

func TestValidationRequiresSourceSuccess(t *testing.T) {
	s := setupTestServer(t)
	v := s.validation("quay-3-18", &model.ReleaseCandidate{Verdict: model.Verdict{State: "ready"}}, nil)
	if v.Ready || v.SourceFresh || v.PlanKnown {
		t.Fatalf("empty instance looks ready: %+v", v)
	}
}

func TestJiraReleaseDoesNotConfirmCustomerPublication(t *testing.T) {
	s := setupTestServer(t)
	const name = "quay-v3.18.1"
	if err := s.db.UpsertReleaseVersion(t.Context(), &model.ReleaseVersion{
		Name: name, KonfluxApplication: "quay-3-18", Released: true,
	}); err != nil {
		t.Fatal(err)
	}
	var rows []model.ReleaseOverview
	getJSON(t, s, "/api/v1/releases/overview?portfolio=1", http.StatusOK, &rows)
	if len(rows) != 1 || !rows[0].Shipped || rows[0].Published || rows[0].Portfolio == nil || rows[0].Portfolio.Stage == "available" {
		t.Fatalf("JIRA-only release advertised as published: %+v", rows)
	}
	var detail struct {
		Published bool `json:"published"`
	}
	getJSON(t, s, "/api/v1/releases/"+name, http.StatusOK, &detail)
	if detail.Published {
		t.Fatal("detail marks JIRA-only release published")
	}
}
