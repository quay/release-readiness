package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/quay/release-readiness/internal/model"
	"github.com/quay/release-readiness/internal/prow"
)

func TestCIHistoryScopesAndPagination(t *testing.T) {
	s := setupTestServer(t)
	const version = "quay-v3.18.1"
	const app = "quay-3-18"
	if err := s.db.UpsertReleaseVersion(t.Context(), &model.ReleaseVersion{Name: version, KonfluxApplication: app}); err != nil {
		t.Fatal(err)
	}
	selected := seedHistoryFBC(t, s, "quay-operator-fbc-3.18.1-20261003000000.ocp4.20", "quay-operator-metadata-container-3.18.1.a")
	seedStagedFBC(t, s, "fbc-quay-3-18", "3.18.1", "quay-stage-3-18-1-fbc-20261003", "2026-10-03T01:00:00Z", []string{selected}, "Succeeded")
	started := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	for _, entry := range []struct{ id, job, application, catalog, artifact string }{
		{"1", "job-a", app, selected, prow.ArtifactPresent},
		{"2", "job-a", app, selected, prow.ArtifactPresent},
		{"3", "job-a", app, "quay.io/example/fbc@sha256:other", prow.ArtifactPresent},
		{"4", "job-a", app, "", prow.ArtifactMissing},
		{"5", "job-b", app, selected, prow.ArtifactPresent},
		{"6", "job-a", "other-app", selected, prow.ArtifactPresent},
		{"7", "job-a", app, selected, prow.ArtifactMissing},
	} {
		err := s.db.UpsertProwRun(t.Context(), &prow.Run{JobName: entry.job, BuildID: entry.id, Application: entry.application, CatalogRef: entry.catalog, ArtifactState: entry.artifact, StartedAt: &started, CompletedAt: &started, State: "success", ProwURL: "https://prow.example/" + entry.id})
		if err != nil {
			t.Fatal(err)
		}
	}
	const base = "/api/v1/releases/" + version + "/ci-history"
	var first historyPage
	getJSON(t, s, base+"?job=job-a&limit=1", http.StatusOK, &first)
	if len(first.Runs) != 1 || first.Runs[0].BuildID != "2" || first.Runs[0].FBCMatch != "exact" || first.NextCursor == "" {
		t.Fatalf("first page: %+v", first)
	}
	var second historyPage
	getJSON(t, s, base+"?job=job-a&limit=1&cursor="+url.QueryEscape(first.NextCursor), http.StatusOK, &second)
	if len(second.Runs) != 1 || second.Runs[0].BuildID != "1" || second.NextCursor != "" {
		t.Fatalf("second page: %+v", second)
	}
	var all historyPage
	getJSON(t, s, base+"?job=job-a&scope=all", http.StatusOK, &all)
	if len(all.Runs) != 5 {
		t.Fatalf("all scope: %+v", all)
	}
	matches := map[string]string{}
	for _, run := range all.Runs {
		matches[run.BuildID] = run.FBCMatch
	}
	if matches["3"] != "different" || matches["4"] != "unverified" || matches["7"] != "unverified" {
		t.Fatalf("identity labels: %+v", matches)
	}
	var releaseWide historyPage
	getJSON(t, s, base, http.StatusOK, &releaseWide)
	if len(releaseWide.Runs) != 3 {
		t.Fatalf("selected scope: %+v", releaseWide)
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?limit=no", "?cursor=bad", "?scope=invalid", "?cursor=" + strings.Repeat("a", 4097)} {
		getJSON(t, s, base+query, http.StatusBadRequest, nil)
	}
	getJSON(t, s, "/api/v1/releases/missing/ci-history", http.StatusNotFound, nil)
}

func TestCIHistoryWithoutCandidate(t *testing.T) {
	s := setupTestServer(t)
	if err := s.db.UpsertReleaseVersion(t.Context(), &model.ReleaseVersion{Name: "quay-v5.0.0", KonfluxApplication: "quay-5-0"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpsertProwRun(t.Context(), &prow.Run{JobName: "job", BuildID: "1", Application: "quay-5-0", State: "pending", ArtifactState: prow.ArtifactMissing}); err != nil {
		t.Fatal(err)
	}
	var page historyPage
	getJSON(t, s, "/api/v1/releases/quay-v5.0.0/ci-history", http.StatusOK, &page)
	if len(page.Runs) != 0 || len(page.SelectedFBCs) != 0 {
		t.Fatalf("invented candidate evidence: %+v", page)
	}
	getJSON(t, s, "/api/v1/releases/quay-v5.0.0/ci-history?scope=all", http.StatusOK, &page)
	if len(page.Runs) != 1 || page.Runs[0].FBCMatch != "unverified" || page.Runs[0].State != "pending" {
		t.Fatalf("missing unknown run: %+v", page)
	}
}
