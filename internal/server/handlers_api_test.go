package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/quay/build-dashboard/internal/db"
	"github.com/quay/build-dashboard/internal/model"
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
	return New(database, nil, ":0")
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

	_, err := srv.db.CreateSnapshot("quay-v3-17", "quay-v3-17-20260213-000", "quay", "abc123", "pr-1", true, false, "")
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

	snap, err := srv.db.CreateSnapshot("quay-v3-17", "quay-v3-17-20260213-001", "quay", "def456", "pr-2", false, false, "tests failing")
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

	_, err := srv.db.CreateSnapshot("quay-v3-17", "snap-1", "quay", "abc", "pr-1", true, false, "")
	if err != nil {
		t.Fatalf("create snapshot 1: %v", err)
	}
	_, err = srv.db.CreateSnapshot("quay-v3-17", "snap-2", "quay", "def", "pr-2", false, false, "")
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
