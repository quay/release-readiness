package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/quay/release-readiness/internal/db"
	"github.com/quay/release-readiness/internal/model"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		database.Close()
		os.Remove(dbPath)
	})
	return New(database, nil, ":0", "https://issues.redhat.com")
}

func TestHealthEndpoint(t *testing.T) {
	srv := setupTestServer(t)
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "healthy" {
		t.Errorf("status: got %q, want healthy", resp["status"])
	}
}

func TestListComponents(t *testing.T) {
	srv := setupTestServer(t)

	// Ensure a component exists
	if _, err := srv.db.EnsureComponent("quay"); err != nil {
		t.Fatalf("ensure component: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/components", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	var components []model.Component
	json.NewDecoder(w.Body).Decode(&components)
	if len(components) < 1 {
		t.Errorf("components: got %d, want >= 1", len(components))
	}
}

func TestListSnapshots(t *testing.T) {
	srv := setupTestServer(t)

	_, err := srv.db.CreateSnapshot("quay-v3-17", "quay-v3-17-20260213-000", "quay", "abc123", "pr-1", true, false, "", time.Now())
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/snapshots", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list snapshots: got %d, body: %s", w.Code, w.Body.String())
	}

	var snapshots []model.SnapshotRecord
	json.NewDecoder(w.Body).Decode(&snapshots)
	if len(snapshots) != 1 {
		t.Errorf("snapshots: got %d, want 1", len(snapshots))
	}
	if snapshots[0].Application != "quay-v3-17" {
		t.Errorf("application: got %q, want %q", snapshots[0].Application, "quay-v3-17")
	}
}

func TestGetSnapshot(t *testing.T) {
	srv := setupTestServer(t)

	snap, err := srv.db.CreateSnapshot("quay-v3-17", "quay-v3-17-20260213-001", "quay", "def456", "pr-2", false, false, "tests failing", time.Now())
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	if err := srv.db.CreateSnapshotTestResult(snap.ID, "operator-ginkgo", "failed", "pr-run-1", 100, 90, 10, 0, 120.5); err != nil {
		t.Fatalf("create test result: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/snapshots/quay-v3-17-20260213-001", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get snapshot: got %d, body: %s", w.Code, w.Body.String())
	}

	var got model.SnapshotRecord
	json.NewDecoder(w.Body).Decode(&got)
	if got.Name != "quay-v3-17-20260213-001" {
		t.Errorf("name: got %q, want %q", got.Name, "quay-v3-17-20260213-001")
	}
	if got.TestsPassed {
		t.Error("tests_passed: got true, want false")
	}
	if len(got.TestResults) != 1 {
		t.Fatalf("test results: got %d, want 1", len(got.TestResults))
	}
	if got.TestResults[0].Scenario != "operator-ginkgo" {
		t.Errorf("scenario: got %q, want %q", got.TestResults[0].Scenario, "operator-ginkgo")
	}
}

func TestListApplications(t *testing.T) {
	srv := setupTestServer(t)

	_, err := srv.db.CreateSnapshot("quay-v3-17", "snap-1", "quay", "abc", "pr-1", true, false, "", time.Now())
	if err != nil {
		t.Fatalf("create snapshot 1: %v", err)
	}
	_, err = srv.db.CreateSnapshot("quay-v3-17", "snap-2", "quay", "def", "pr-2", false, false, "", time.Now())
	if err != nil {
		t.Fatalf("create snapshot 2: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/applications", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list applications: got %d, body: %s", w.Code, w.Body.String())
	}

	var summaries []model.ApplicationSummary
	json.NewDecoder(w.Body).Decode(&summaries)
	if len(summaries) != 1 {
		t.Fatalf("applications: got %d, want 1", len(summaries))
	}
	if summaries[0].SnapshotCount != 2 {
		t.Errorf("snapshot count: got %d, want 2", summaries[0].SnapshotCount)
	}
	if summaries[0].LatestSnapshot.Name != "snap-2" {
		t.Errorf("latest snapshot: got %q, want %q", summaries[0].LatestSnapshot.Name, "snap-2")
	}
}

func TestListReleases(t *testing.T) {
	srv := setupTestServer(t)

	dueDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	err := srv.db.UpsertReleaseVersion(&model.ReleaseVersion{
		Name:             "3.16.3",
		Description:      "z-stream",
		ReleaseTicketKey: "PROJQUAY-10276",
		S3Application:    "quay-v3-16",
		DueDate:          &dueDate,
	})
	if err != nil {
		t.Fatalf("upsert release: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/releases", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list releases: got %d, body: %s", w.Code, w.Body.String())
	}

	var releases []model.ReleaseVersion
	json.NewDecoder(w.Body).Decode(&releases)
	if len(releases) != 1 {
		t.Fatalf("releases: got %d, want 1", len(releases))
	}
	if releases[0].Name != "3.16.3" {
		t.Errorf("name: got %q, want 3.16.3", releases[0].Name)
	}
	if releases[0].ReleaseTicketKey != "PROJQUAY-10276" {
		t.Errorf("ticket: got %q, want PROJQUAY-10276", releases[0].ReleaseTicketKey)
	}
	if releases[0].S3Application != "quay-v3-16" {
		t.Errorf("s3_app: got %q, want quay-v3-16", releases[0].S3Application)
	}
}

func TestGetReleaseSnapshot(t *testing.T) {
	srv := setupTestServer(t)

	// Create a snapshot for the S3 application
	_, err := srv.db.CreateSnapshot("quay-v3-16", "quay-v3-16-snap-1", "quay", "abc123", "pr-1", true, false, "", time.Now())
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	// Create the release version pointing to this S3 app
	err = srv.db.UpsertReleaseVersion(&model.ReleaseVersion{
		Name:          "3.16.3",
		S3Application: "quay-v3-16",
	})
	if err != nil {
		t.Fatalf("upsert release: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/releases/3.16.3/snapshot", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get release snapshot: got %d, body: %s", w.Code, w.Body.String())
	}

	var snap model.SnapshotRecord
	json.NewDecoder(w.Body).Decode(&snap)
	if snap.Name != "quay-v3-16-snap-1" {
		t.Errorf("snapshot name: got %q, want quay-v3-16-snap-1", snap.Name)
	}
}

func TestGetReleaseReadiness(t *testing.T) {
	srv := setupTestServer(t)

	// Create a release with a future due date
	dueDate := time.Now().Add(10 * 24 * time.Hour)
	err := srv.db.UpsertReleaseVersion(&model.ReleaseVersion{
		Name:          "3.16.3",
		S3Application: "quay-v3-16",
		DueDate:       &dueDate,
	})
	if err != nil {
		t.Fatalf("upsert release: %v", err)
	}

	// Create a passing snapshot
	_, err = srv.db.CreateSnapshot("quay-v3-16", "quay-v3-16-snap-1", "quay", "abc123", "pr-1", true, false, "", time.Now())
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/releases/3.16.3/readiness", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get readiness: got %d, body: %s", w.Code, w.Body.String())
	}

	var readiness ReadinessResponse
	json.NewDecoder(w.Body).Decode(&readiness)
	if readiness.Signal != "green" {
		t.Errorf("signal: got %q, want green", readiness.Signal)
	}
}
