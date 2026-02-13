package server

import (
	"bytes"
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
	return New(database, ":0")
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

func TestBuildLifecycle(t *testing.T) {
	srv := setupTestServer(t)

	// Create a build
	body := `{"component":"quay","version":"3.16.2","git_sha":"abc123def456","image_url":"quay.io/test"}`
	req := httptest.NewRequest("POST", "/api/v1/builds", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create build: got %d, body: %s", w.Code, w.Body.String())
	}

	var build model.Build
	json.NewDecoder(w.Body).Decode(&build)
	if build.Component != "quay" {
		t.Errorf("component: got %q", build.Component)
	}
	if build.Status != "pending" {
		t.Errorf("status: got %q, want pending", build.Status)
	}

	// Get build
	req = httptest.NewRequest("GET", "/api/v1/builds/1", nil)
	w = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get build: got %d", w.Code)
	}

	// Submit test results
	results := `{
		"suite":"ui-cypress",
		"total":3,"passed":2,"failed":1,"skipped":0,"duration_sec":10.5,
		"test_cases":[
			{"name":"test1","status":"passed","duration_sec":3},
			{"name":"test2","status":"passed","duration_sec":4},
			{"name":"test3","status":"failed","failure_msg":"oops","duration_sec":3.5}
		]
	}`
	req = httptest.NewRequest("POST", "/api/v1/builds/1/results", bytes.NewBufferString(results))
	w = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("submit results: got %d, body: %s", w.Code, w.Body.String())
	}

	// Verify build status updated to failed
	req = httptest.NewRequest("GET", "/api/v1/builds/1", nil)
	w = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&build)
	if build.Status != "failed" {
		t.Errorf("status after results: got %q, want failed", build.Status)
	}
	if len(build.TestRuns) != 1 {
		t.Fatalf("test runs: got %d, want 1", len(build.TestRuns))
	}
	if build.TestRuns[0].Failed != 1 {
		t.Errorf("failed count: got %d, want 1", build.TestRuns[0].Failed)
	}
}

func TestListBuildsWithFilters(t *testing.T) {
	srv := setupTestServer(t)

	// Create two builds for different components
	for _, comp := range []string{"quay", "clair"} {
		body, _ := json.Marshal(map[string]string{
			"component": comp, "version": "3.16.2",
			"git_sha": "abc123", "image_url": "quay.io/" + comp,
		})
		req := httptest.NewRequest("POST", "/api/v1/builds", bytes.NewReader(body))
		w := httptest.NewRecorder()
		srv.http.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", comp, w.Code, w.Body.String())
		}
	}

	// List all
	req := httptest.NewRequest("GET", "/api/v1/builds", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	var builds []model.Build
	json.NewDecoder(w.Body).Decode(&builds)
	if len(builds) != 2 {
		t.Errorf("all builds: got %d, want 2", len(builds))
	}

	// Filter by component
	req = httptest.NewRequest("GET", "/api/v1/builds?component=quay", nil)
	w = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&builds)
	if len(builds) != 1 {
		t.Errorf("filtered builds: got %d, want 1", len(builds))
	}
}

func TestComponentsAndSuites(t *testing.T) {
	srv := setupTestServer(t)

	// List seeded components
	req := httptest.NewRequest("GET", "/api/v1/components", nil)
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	var components []model.Component
	json.NewDecoder(w.Body).Decode(&components)
	if len(components) != 10 {
		t.Errorf("components: got %d, want 10", len(components))
	}

	// List seeded suites
	req = httptest.NewRequest("GET", "/api/v1/suites", nil)
	w = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	var suites []model.Suite
	json.NewDecoder(w.Body).Decode(&suites)
	if len(suites) != 5 {
		t.Errorf("suites: got %d, want 5", len(suites))
	}

	// Map suite to component
	body := `{"suite":"ui-cypress","required":true}`
	req = httptest.NewRequest("POST", "/api/v1/components/quay/suites", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("map suite: got %d, body: %s", w.Code, w.Body.String())
	}
}
