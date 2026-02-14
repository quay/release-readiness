package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

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

// --- Snapshots ---

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	snapshots, err := s.db.ListSnapshots(q.Get("application"), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshots)
}

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	snap, err := s.db.GetSnapshotByName(name)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("snapshot not found"))
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// --- Applications ---

func (s *Server) handleListApplications(w http.ResponseWriter, r *http.Request) {
	summaries, err := s.db.LatestSnapshotPerApplication()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, summaries)
}

// --- JIRA / Releases ---

func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	fixVersion := s.appToFixVersion(app)
	if fixVersion == "" {
		writeError(w, http.StatusNotFound, fmt.Errorf("no fixVersion mapping for %q", app))
		return
	}

	q := r.URL.Query()
	issues, err := s.db.ListJiraIssues(fixVersion, q.Get("type"), q.Get("status"), q.Get("label"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if issues == nil {
		issues = []model.JiraIssueRecord{}
	}
	writeJSON(w, http.StatusOK, issues)
}

func (s *Server) handleGetIssueSummary(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	fixVersion := s.appToFixVersion(app)
	if fixVersion == "" {
		writeError(w, http.StatusNotFound, fmt.Errorf("no fixVersion mapping for %q", app))
		return
	}

	summary, err := s.db.GetIssueSummary(fixVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleGetReleaseVersion(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("app")
	fixVersion := s.appToFixVersion(app)
	if fixVersion == "" {
		writeError(w, http.StatusNotFound, fmt.Errorf("no fixVersion mapping for %q", app))
		return
	}

	version, err := s.db.GetReleaseVersion(fixVersion)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("version %q not found", fixVersion))
		return
	}
	writeJSON(w, http.StatusOK, version)
}

// appToFixVersion derives a JIRA fixVersion from an S3 application prefix.
// Convention: "quay-v3-16" → "3.16", "quay-v3-16-2" → "3.16.2"
// Falls back to the configured version mapping if available.
func (s *Server) appToFixVersion(app string) string {
	if s.fixVersions != nil {
		if v, ok := s.fixVersions[app]; ok {
			return v
		}
	}
	return deriveFixVersion(app)
}

// deriveFixVersion extracts a version string from an application prefix.
// Examples: "quay-v3-16" → "3.16", "quay-v3-16-2" → "3.16.2"
func deriveFixVersion(app string) string {
	// Find "v" followed by version digits
	parts := strings.Split(app, "-")
	var versionParts []string
	inVersion := false
	for _, p := range parts {
		if !inVersion {
			if len(p) > 0 && p[0] == 'v' {
				inVersion = true
				versionParts = append(versionParts, p[1:])
			}
			continue
		}
		// Only include numeric parts
		if isNumeric(p) {
			versionParts = append(versionParts, p)
		} else {
			break
		}
	}
	if len(versionParts) == 0 {
		return ""
	}
	return strings.Join(versionParts, ".")
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
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
