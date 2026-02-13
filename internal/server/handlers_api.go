package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/quay/build-dashboard/internal/model"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Ping(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// --- Components ---

func (s *Server) handleListComponents(w http.ResponseWriter, r *http.Request) {
	components, err := s.db.ListComponents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, components)
}

func (s *Server) handleCreateComponent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	comp, err := s.db.CreateComponent(req.Name, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, comp)
}

func (s *Server) handleMapSuite(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Suite    string `json:"suite"`
		Required bool   `json:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Suite == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("suite is required"))
		return
	}
	if err := s.db.MapSuiteToComponent(name, req.Suite, req.Required); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Suites ---

func (s *Server) handleListSuites(w http.ResponseWriter, r *http.Request) {
	suites, err := s.db.ListSuites()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, suites)
}

func (s *Server) handleCreateSuite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	suite, err := s.db.CreateSuite(req.Name, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, suite)
}

// --- Builds ---

func (s *Server) handleCreateBuild(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Component    string `json:"component"`
		Version      string `json:"version"`
		GitSHA       string `json:"git_sha"`
		GitBranch    string `json:"git_branch"`
		ImageURL     string `json:"image_url"`
		ImageDigest  string `json:"image_digest"`
		PipelineRun  string `json:"pipeline_run"`
		SnapshotName string `json:"snapshot_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Component == "" || req.Version == "" || req.GitSHA == "" || req.ImageURL == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("component, version, git_sha, and image_url are required"))
		return
	}
	build, err := s.db.CreateBuild(req.Component, req.Version, req.GitSHA, req.GitBranch,
		req.ImageURL, req.ImageDigest, req.PipelineRun, req.SnapshotName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, build)
}

func (s *Server) handleListBuilds(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	builds, err := s.db.ListBuilds(q.Get("component"), q.Get("version"), q.Get("status"), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, builds)
}

func (s *Server) handleGetBuild(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid build id"))
		return
	}
	build, err := s.db.GetBuild(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("build not found"))
		return
	}
	writeJSON(w, http.StatusOK, build)
}

func (s *Server) handleLatestBuild(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	component := q.Get("component")
	version := q.Get("version")
	if component == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("component is required"))
		return
	}
	build, err := s.db.GetLatestBuild(component, version)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("build not found"))
		return
	}
	writeJSON(w, http.StatusOK, build)
}

// --- Test Results ---

func (s *Server) handleSubmitResults(w http.ResponseWriter, r *http.Request) {
	buildID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid build id"))
		return
	}
	var req struct {
		Suite       string           `json:"suite"`
		Environment string           `json:"environment"`
		PipelineRun string           `json:"pipeline_run"`
		Total       int              `json:"total"`
		Passed      int              `json:"passed"`
		Failed      int              `json:"failed"`
		Skipped     int              `json:"skipped"`
		DurationSec float64          `json:"duration_sec"`
		TestCases   []model.TestCase `json:"test_cases"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Suite == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("suite is required"))
		return
	}

	status := "passed"
	if req.Failed > 0 {
		status = "failed"
	}

	run, err := s.db.CreateTestRun(buildID, req.Suite, req.Total, req.Passed, req.Failed,
		req.Skipped, req.DurationSec, status, req.Environment, req.PipelineRun, req.TestCases)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Recompute build status
	if err := s.db.RecomputeBuildStatus(buildID); err != nil {
		log.Printf("recompute build status: %v", err)
	}

	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) handleGetResults(w http.ResponseWriter, r *http.Request) {
	buildID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid build id"))
		return
	}
	runs, err := s.db.ListTestRuns(buildID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleGetTestRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid test run id"))
		return
	}
	run, err := s.db.GetTestRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("test run not found"))
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// --- Readiness ---

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	matrix, err := s.db.GetReadiness(version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, matrix)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
